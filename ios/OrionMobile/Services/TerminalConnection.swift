import Foundation

@Observable
final class TerminalConnection {
    private(set) var terminalId: String
    let tmuxSession: String
    private(set) var isConnected = false
    private(set) var connectionState = ConnectionState.disconnected
    /// Set to true after scrolling (tmux copy mode). Cleared when exitCopyMode() is called.
    var inCopyMode = false
    var onOutput: (([UInt8]) -> Void)?
    var onExit: (() -> Void)?

    private var host: String?
    private var token: String?
    private var webSocket: URLSessionWebSocketTask?
    private var session: URLSession?
    private var pendingResize: (cols: Int, rows: Int)?
    private var reconnectTask: Task<Void, Never>?
    private var pingTask: Task<Void, Never>?

    init(terminalId: String, tmuxSession: String) {
        self.terminalId = terminalId
        self.tmuxSession = tmuxSession
    }

    deinit { disconnect() }

    func connect(host: String, token: String) {
        self.host = host
        self.token = token
        // Clean up old WebSocket if any
        pingTask?.cancel(); pingTask = nil
        webSocket?.cancel(with: .normalClosure, reason: nil); webSocket = nil
        session?.invalidateAndCancel(); session = nil
        inCopyMode = false
        connectWebSocket(host: host, token: token)
    }

    private func connectWebSocket(host: String, token: String) {
        let encoded = token.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? token
        let encodedTmux = tmuxSession.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? tmuxSession
        guard let url = URL(string: "ws://\(host)/ws/terminal/\(terminalId)?token=\(encoded)&tmux=\(encodedTmux)") else {
            print("[Orion Terminal] Invalid URL for \(terminalId)")
            return
        }
        print("[Orion Terminal] Connecting: \(terminalId) → \(tmuxSession)")
        let config = URLSessionConfiguration.default
        config.shouldUseExtendedBackgroundIdleMode = true
        session = URLSession(configuration: config)
        webSocket = session?.webSocketTask(with: url)
        webSocket?.resume()
        isConnected = true
        connectionState = .connected
        receiveLoop()
        startPing()
        if let resize = pendingResize { sendResize(cols: resize.cols, rows: resize.rows); pendingResize = nil }
    }

    func disconnect() {
        cancelReconnect()
        pingTask?.cancel()
        pingTask = nil
        webSocket?.cancel(with: .normalClosure, reason: nil)
        webSocket = nil
        session?.invalidateAndCancel()
        session = nil
        isConnected = false
        connectionState = .disconnected
    }

    func sendInput(_ data: [UInt8]) { send(WSMessage(type: "input", data: Data(data).base64EncodedString())) }
    func sendResize(cols: Int, rows: Int) {
        guard isConnected else { pendingResize = (cols, rows); return }
        send(WSMessage(type: "resize", cols: cols, rows: rows))
    }
    func sendScroll(direction: String, lines: Int) { inCopyMode = true; send(WSMessage(type: "scroll", data: direction, cols: lines)) }

    /// Exit tmux copy mode and scroll to bottom. Only fires when we know we're in copy mode.
    func exitCopyMode() {
        guard inCopyMode else { return }
        inCopyMode = false
        // Send Escape first (cancels any copy-mode sub-prompt like goto-line),
        // then q to exit copy mode entirely. At a normal prompt this never fires
        // because inCopyMode is only true after scrolling.
        sendInput([0x1B])
        sendInput([UInt8]("q".utf8))
    }

    private func send(_ message: WSMessage) {
        guard let data = try? JSONEncoder().encode(message), let string = String(data: data, encoding: .utf8) else { return }
        webSocket?.send(.string(string)) { _ in }
    }

    private func receiveLoop() {
        webSocket?.receive { [weak self] result in
            guard let self else { return }
            switch result {
            case .success(let message): self.handleMessage(message); self.receiveLoop()
            case .failure:
                DispatchQueue.main.async {
                    self.isConnected = false
                    self.attemptReconnect()
                }
            }
        }
    }

    private func handleMessage(_ message: URLSessionWebSocketTask.Message) {
        guard case .string(let text) = message, let data = text.data(using: .utf8),
              let msg = try? JSONDecoder().decode(WSMessage.self, from: data) else { return }
        switch msg.type {
        case "output":
            if let b64 = msg.data, let decoded = Data(base64Encoded: b64) {
                DispatchQueue.main.async { self.onOutput?([UInt8](decoded)) }
            }
        case "exit":
            DispatchQueue.main.async { self.isConnected = false; self.connectionState = .disconnected; self.onExit?() }
        case "pong":
            break // Ignore pong responses
        default: break
        }
    }

    // MARK: - Auto-Reconnect

    private func attemptReconnect() {
        guard let host, let token else {
            connectionState = .failed
            onExit?()
            return
        }

        connectionState = .reconnecting
        reconnectTask = Task { [weak self] in
            let maxAttempts = 5
            var attempt = 0

            while attempt < maxAttempts {
                guard let self, !Task.isCancelled else { return }
                attempt += 1
                let delay = UInt64(pow(2.0, Double(attempt - 1))) // 1, 2, 4, 8, 16 seconds
                try? await Task.sleep(nanoseconds: delay * 1_000_000_000)
                guard !Task.isCancelled else { return }

                do {
                    // Request a new terminal ID from the server
                    var request = URLRequest(url: URL(string: "http://\(host)/api/terminal")!)
                    request.httpMethod = "POST"
                    request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
                    request.setValue("application/json", forHTTPHeaderField: "Content-Type")
                    request.httpBody = try JSONEncoder().encode(["tmuxSession": self.tmuxSession])
                    let (data, _) = try await URLSession.shared.data(for: request)
                    let resp = try JSONDecoder().decode(CreateTerminalResponse.self, from: data)

                    guard !Task.isCancelled else { return }

                    // Disconnect old WebSocket
                    await MainActor.run {
                        self.webSocket?.cancel(with: .normalClosure, reason: nil)
                        self.webSocket = nil
                        self.session?.invalidateAndCancel()
                        self.session = nil

                        // Update terminal ID and reconnect
                        self.terminalId = resp.terminalId
                        self.connectWebSocket(host: host, token: token)
                    }
                    return // Success
                } catch {
                    continue // Try again
                }
            }

            // All attempts failed
            guard let self, !Task.isCancelled else { return }
            await MainActor.run {
                self.connectionState = .failed
                self.onExit?()
            }
        }
    }

    private func cancelReconnect() {
        reconnectTask?.cancel()
        reconnectTask = nil
    }

    // MARK: - Client Ping

    private func startPing() {
        pingTask?.cancel()
        pingTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 20 * 1_000_000_000) // 20 seconds
                guard let self, !Task.isCancelled, self.isConnected else { return }
                self.send(WSMessage(type: "ping"))
            }
        }
    }
}
