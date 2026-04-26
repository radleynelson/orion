import Foundation

@Observable
final class TerminalConnection {
    private(set) var terminalId: String
    let tmuxSession: String
    private(set) var isConnected = false
    private(set) var connectionState = ConnectionState.disconnected
    /// Set to true after scrolling (tmux copy mode). Cleared when exitCopyMode() is called.
    var inCopyMode = false
    var onOutput: (([UInt8]) -> Void)? {
        didSet { flushBufferedOutput() }
    }
    var onExit: (() -> Void)?
    var onPermanentFailure: (() -> Void)?

    private var host: String?
    private var token: String?
    private var webSocket: URLSessionWebSocketTask?
    private var session: URLSession?
    private var pendingResize: (cols: Int, rows: Int)?
    private var reconnectTask: Task<Void, Never>?
    private var pingTask: Task<Void, Never>?
    private var bufferedOutput: [[UInt8]] = []
    private var shouldReconnect = false
    private var connectionGeneration = 0

    init(terminalId: String, tmuxSession: String) {
        self.terminalId = terminalId
        self.tmuxSession = tmuxSession
    }

    deinit { disconnect() }

    func connect(host: String, token: String) {
        self.host = host
        self.token = token
        shouldReconnect = true
        connectionGeneration += 1
        // Clean up old WebSocket if any
        pingTask?.cancel(); pingTask = nil
        webSocket?.cancel(with: .normalClosure, reason: nil); webSocket = nil
        session?.invalidateAndCancel(); session = nil
        inCopyMode = false
        connectWebSocket(host: host, token: token, generation: connectionGeneration)
    }

    private func connectWebSocket(host: String, token: String, generation: Int) {
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
        receiveLoop(generation: generation)
        startPing()
        if let resize = pendingResize { sendResize(cols: resize.cols, rows: resize.rows); pendingResize = nil }
    }

    func disconnect() {
        shouldReconnect = false
        connectionGeneration += 1
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
    /// Uses tmux's official cancel command via the backend (bypasses the PTY for reliability).
    func exitCopyMode() {
        guard inCopyMode else { return }
        inCopyMode = false
        send(WSMessage(type: "cancel-copy-mode"))
    }

    private func send(_ message: WSMessage) {
        guard let data = try? JSONEncoder().encode(message), let string = String(data: data, encoding: .utf8) else { return }
        webSocket?.send(.string(string)) { _ in }
    }

    private func receiveLoop(generation: Int) {
        webSocket?.receive { [weak self] result in
            guard let self else { return }
            guard generation == self.connectionGeneration else { return }
            switch result {
            case .success(let message):
                self.handleMessage(message)
                self.receiveLoop(generation: generation)
            case .failure:
                DispatchQueue.main.async {
                    guard generation == self.connectionGeneration else { return }
                    self.isConnected = false
                    guard self.shouldReconnect else { return }
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
                let bytes = [UInt8](decoded)
                DispatchQueue.main.async { self.deliverOutput(bytes) }
            }
        case "exit":
            DispatchQueue.main.async {
                self.shouldReconnect = false
                self.isConnected = false
                self.connectionState = .disconnected
                self.onExit?()
            }
        case "pong":
            break // Ignore pong responses
        default: break
        }
    }

    // MARK: - Auto-Reconnect

    private func attemptReconnect() {
        guard let host, let token else {
            connectionState = .failed
            onPermanentFailure?()
            return
        }

        connectionState = .reconnecting
        reconnectTask = Task { [weak self] in
            let maxAttempts = 5
            var attempt = 0

            while attempt < maxAttempts {
                guard let self, !Task.isCancelled else { return }
                attempt += 1
                // Exponential backoff with jitter: 1s, 2s, 4s, 8s, 16s × (0.5–1.5)
                let baseDelay = pow(2.0, Double(attempt - 1))
                let jittered = baseDelay * Double.random(in: 0.5...1.5)
                try? await Task.sleep(nanoseconds: UInt64(jittered * 1_000_000_000))
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
                        self.connectionGeneration += 1
                        let generation = self.connectionGeneration
                        self.webSocket?.cancel(with: .normalClosure, reason: nil)
                        self.webSocket = nil
                        self.session?.invalidateAndCancel()
                        self.session = nil

                        // Update terminal ID and reconnect
                        self.terminalId = resp.terminalId
                        self.connectWebSocket(host: host, token: token, generation: generation)
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
                self.onPermanentFailure?()
            }
        }
    }

    private func cancelReconnect() {
        reconnectTask?.cancel()
        reconnectTask = nil
    }

    private func deliverOutput(_ bytes: [UInt8]) {
        if let onOutput {
            onOutput(bytes)
            return
        }
        bufferedOutput.append(bytes)
        if bufferedOutput.count > 256 {
            bufferedOutput.removeFirst(bufferedOutput.count - 256)
        }
    }

    private func flushBufferedOutput() {
        guard let onOutput, !bufferedOutput.isEmpty else { return }
        let pending = bufferedOutput
        bufferedOutput.removeAll(keepingCapacity: true)
        for chunk in pending {
            onOutput(chunk)
        }
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

@Observable
final class CodexChatConnection {
    let sessionId: String
    let sessionType: String
    let sessionIcon: String
    let workspacePath: String
    private(set) var isConnected = false
    private(set) var connectionState = ConnectionState.disconnected
    private(set) var queuedMessageCount = 0
    var messages: [CodexChatMessage] = []
    var onPermanentFailure: (() -> Void)?
    var onAssistantVoiceText: ((String) -> Void)?

    private var host: String?
    private var token: String?
    private var webSocket: URLSessionWebSocketTask?
    private var session: URLSession?
    private var reconnectTask: Task<Void, Never>?
    private var pingTask: Task<Void, Never>?
    private var pongTimeoutTask: Task<Void, Never>?
    private var healthTask: Task<Void, Never>?
    private var shouldReconnect = false
    private var connectionGeneration = 0
    private var connectedAt = Date.distantPast
    private var lastPongAt = Date.distantPast
    private var pendingCodexVoiceText = ""
    private var lastCodexVoiceMessageType = ""
    private var pendingSends: [CodexChatWSMessage] = []
    private let maxPendingSends = 12

    init(sessionId: String, sessionType: String = "codex-chat", sessionIcon: String? = nil, workspacePath: String = "") {
        self.sessionId = sessionId
        self.sessionType = sessionType
        self.sessionIcon = sessionIcon?.isEmpty == false ? sessionIcon! : sessionType
        self.workspacePath = workspacePath
    }

    private var isClaude: Bool { sessionType == "claude" || sessionType == "claude-chat" }
    var displayName: String { isClaude ? "Claude" : "Codex" }
    var avatar: String { isClaude ? "\u{25C6}" : "C" }

    deinit { disconnect() }

    func connect(host: String, token: String) {
        self.host = host
        self.token = token
        shouldReconnect = true
        connectionGeneration += 1
        reconnectTask?.cancel()
        reconnectTask = nil
        pingTask?.cancel(); pingTask = nil
        pongTimeoutTask?.cancel(); pongTimeoutTask = nil
        healthTask?.cancel(); healthTask = nil
        webSocket?.cancel(with: .normalClosure, reason: nil); webSocket = nil
        session?.invalidateAndCancel(); session = nil
        connectWebSocket(host: host, token: token, generation: connectionGeneration)
    }

    func disconnect() {
        shouldReconnect = false
        connectionGeneration += 1
        reconnectTask?.cancel()
        reconnectTask = nil
        pingTask?.cancel()
        pingTask = nil
        pongTimeoutTask?.cancel()
        pongTimeoutTask = nil
        healthTask?.cancel()
        healthTask = nil
        webSocket?.cancel(with: .normalClosure, reason: nil)
        webSocket = nil
        session?.invalidateAndCancel()
        session = nil
        isConnected = false
        connectionState = .disconnected
        pendingSends.removeAll(keepingCapacity: true)
        queuedMessageCount = 0
        resetPendingCodexVoiceText()
    }

    func sendInput(_ text: String, attachments: [ChatAttachmentPayload] = []) {
        send(CodexChatWSMessage(type: "input", text: text, attachments: attachments.isEmpty ? nil : attachments))
    }

    func answer(toolUseId: String, text: String) {
        send(CodexChatWSMessage(type: "answer", text: text, toolUseId: toolUseId))
    }

    func approvePlan() {
        send(CodexChatWSMessage(type: "plan_action", action: "approve"))
    }

    func reconnectOrProbe() {
        guard let host, let token else { return }
        if connectionState == .reconnecting { return }
        if !isConnected || connectionState == .failed {
            connect(host: host, token: token)
            return
        }
        verifyServerReachable()
    }

    private func connectWebSocket(host: String, token: String, generation: Int) {
        let encoded = token.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? token
        let encodedSession = sessionId.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? sessionId
        let route = isClaude ? "claude-chat" : "codex-chat"
        var query = "token=\(encoded)"
        if !workspacePath.isEmpty {
            let encodedWorkspace = workspacePath.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? workspacePath
            query += "&workspacePath=\(encodedWorkspace)"
        }
        guard let url = URL(string: "ws://\(host)/ws/\(route)/\(encodedSession)?\(query)") else {
            print("[Orion \(displayName) Chat] Invalid URL for \(sessionId)")
            return
        }
        print("[Orion \(displayName) Chat] Connecting: \(sessionId)")
        let config = URLSessionConfiguration.default
        config.shouldUseExtendedBackgroundIdleMode = true
        session = URLSession(configuration: config)
        webSocket = session?.webSocketTask(with: url)
        webSocket?.resume()
        isConnected = true
        connectionState = .connected
        connectedAt = Date()
        lastPongAt = Date()
        resetPendingCodexVoiceText()
        receiveLoop(generation: generation)
        startPing()
        flushPendingSends()
    }

    private func send(_ message: CodexChatWSMessage) {
        guard let data = try? JSONEncoder().encode(message),
              let string = String(data: data, encoding: .utf8) else { return }
        guard isConnected, let webSocket else {
            queueForReconnect(message)
            attemptReconnect()
            return
        }
        webSocket.send(.string(string)) { [weak self] error in
            guard let self, error != nil else { return }
            DispatchQueue.main.async {
                self.queueForReconnect(message)
                self.markDisconnectedAndReconnect()
            }
        }
    }

    private func receiveLoop(generation: Int) {
        webSocket?.receive { [weak self] result in
            guard let self else { return }
            guard generation == self.connectionGeneration else { return }
            switch result {
            case .success(let message):
                self.handleMessage(message)
                self.receiveLoop(generation: generation)
            case .failure:
                DispatchQueue.main.async {
                    guard generation == self.connectionGeneration else { return }
                    self.markDisconnectedAndReconnect()
                }
            }
        }
    }

    private func handleMessage(_ message: URLSessionWebSocketTask.Message) {
        guard case .string(let text) = message,
              let data = text.data(using: .utf8) else { return }
        if let chatMessage = try? JSONDecoder().decode(CodexChatMessage.self, from: data) {
            DispatchQueue.main.async {
                if !self.messages.contains(where: { $0.id == chatMessage.id }) {
                    self.messages.append(chatMessage)
                    self.publishAssistantVoiceTextIfNeeded(chatMessage)
                }
            }
            return
        }
        if let control = try? JSONDecoder().decode(CodexChatWSMessage.self, from: data),
           control.type == "pong" {
            DispatchQueue.main.async {
                self.lastPongAt = Date()
                self.pongTimeoutTask?.cancel()
                self.pongTimeoutTask = nil
            }
            return
        }
    }

    private func attemptReconnect() {
        guard reconnectTask == nil else { return }
        guard let host, let token else {
            connectionState = .failed
            onPermanentFailure?()
            return
        }
        connectionState = .reconnecting
        reconnectTask = Task { [weak self] in
            let maxAttempts = 5
            for attempt in 0..<maxAttempts {
                guard let self, !Task.isCancelled else { return }
                let baseDelay = pow(2.0, Double(attempt))
                let jittered = baseDelay * Double.random(in: 0.5...1.5)
                try? await Task.sleep(nanoseconds: UInt64(jittered * 1_000_000_000))
                guard !Task.isCancelled else { return }
                await MainActor.run {
                    self.connectionGeneration += 1
                    let generation = self.connectionGeneration
                    self.webSocket?.cancel(with: .normalClosure, reason: nil)
                    self.webSocket = nil
                    self.session?.invalidateAndCancel()
                    self.session = nil
                    self.reconnectTask = nil
                    self.connectWebSocket(host: host, token: token, generation: generation)
                }
                return
            }
            guard let self, !Task.isCancelled else { return }
            await MainActor.run {
                self.reconnectTask = nil
                self.connectionState = .failed
                self.onPermanentFailure?()
            }
        }
    }

    private func startPing() {
        pingTask?.cancel()
        pingTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 20 * 1_000_000_000)
                guard let self, !Task.isCancelled, self.isConnected else { return }
                self.verifyServerReachable()
            }
        }
    }

    private func verifyServerReachable() {
        guard let host, let token else { return }
        guard let url = URL(string: "http://\(host)/api/projects") else {
            markDisconnectedAndReconnect()
            return
        }
        healthTask?.cancel()
        let generation = connectionGeneration
        healthTask = Task { [weak self] in
            var request = URLRequest(url: url)
            request.timeoutInterval = 3
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            do {
                let (_, response) = try await URLSession.shared.data(for: request)
                guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
                    throw URLError(.badServerResponse)
                }
                guard let self, !Task.isCancelled else { return }
                await MainActor.run {
                    guard generation == self.connectionGeneration else { return }
                    self.healthTask = nil
                    guard self.connectionState == .connected else { return }
                    self.sendPingProbe()
                }
            } catch {
                guard let self, !Task.isCancelled else { return }
                await MainActor.run {
                    guard generation == self.connectionGeneration else { return }
                    self.healthTask = nil
                    self.markDisconnectedAndReconnect()
                }
            }
        }
    }

    private func sendPingProbe() {
        send(CodexChatWSMessage(type: "ping"))
        schedulePongTimeout()
    }

    private func schedulePongTimeout() {
        pongTimeoutTask?.cancel()
        let generation = connectionGeneration
        pongTimeoutTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: 5 * 1_000_000_000)
            guard let self, !Task.isCancelled else { return }
            await MainActor.run {
                guard generation == self.connectionGeneration else { return }
                guard self.connectionState == .connected else { return }
                guard Date().timeIntervalSince(self.lastPongAt) >= 4.5 else { return }
                self.markDisconnectedAndReconnect()
            }
        }
    }

    private func queueForReconnect(_ message: CodexChatWSMessage) {
        guard message.type != "ping" else { return }
        pendingSends.append(message)
        if pendingSends.count > maxPendingSends {
            pendingSends.removeFirst(pendingSends.count - maxPendingSends)
        }
        queuedMessageCount = pendingSends.count
    }

    private func flushPendingSends() {
        guard isConnected, !pendingSends.isEmpty else { return }
        let queued = pendingSends
        pendingSends.removeAll(keepingCapacity: true)
        queuedMessageCount = 0
        for message in queued {
            send(message)
        }
    }

    private func markDisconnectedAndReconnect() {
        pongTimeoutTask?.cancel()
        pongTimeoutTask = nil
        healthTask?.cancel()
        healthTask = nil
        isConnected = false
        guard shouldReconnect else { return }
        attemptReconnect()
    }

    private func publishAssistantVoiceTextIfNeeded(_ message: CodexChatMessage) {
        guard !isClaude else { return }
        guard isLiveMessage(message) else { return }

        switch message.type {
        case "user":
            resetPendingCodexVoiceText()
        case "status":
            if message.status == "running" {
                resetPendingCodexVoiceText()
            } else if message.status == "idle" {
                publishPendingCodexVoiceText()
            }
        case "stream_delta":
            appendPendingCodexVoiceText(message.text)
        case "assistant":
            let text = message.text?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            guard !text.isEmpty else { return }
            resetPendingCodexVoiceText()
            onAssistantVoiceText?(text)
        case "result":
            let value = (message.subtype ?? message.text ?? "").lowercased()
            guard value != "error" && value != "failed" else {
                resetPendingCodexVoiceText()
                return
            }
            publishPendingCodexVoiceText()
        default:
            break
        }
        lastCodexVoiceMessageType = message.type
    }

    private func appendPendingCodexVoiceText(_ text: String?) {
        guard let text, !text.isEmpty else { return }
        if !pendingCodexVoiceText.isEmpty && lastCodexVoiceMessageType != "stream_delta" {
            pendingCodexVoiceText += "\n\n"
        }
        pendingCodexVoiceText += text
    }

    private func publishPendingCodexVoiceText() {
        let text = pendingCodexVoiceText.trimmingCharacters(in: .whitespacesAndNewlines)
        resetPendingCodexVoiceText()
        guard !text.isEmpty else { return }
        onAssistantVoiceText?(text)
    }

    private func resetPendingCodexVoiceText() {
        pendingCodexVoiceText = ""
        lastCodexVoiceMessageType = ""
    }

    private func isLiveMessage(_ message: CodexChatMessage) -> Bool {
        guard let createdAt = Self.parseCreatedAt(message.createdAt) else { return true }
        return createdAt >= connectedAt.addingTimeInterval(-1)
    }

    private static func parseCreatedAt(_ value: String) -> Date? {
        fractionalDateFormatter.date(from: value) ?? plainDateFormatter.date(from: value)
    }

    private static let fractionalDateFormatter: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    private static let plainDateFormatter: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter
    }()
}
