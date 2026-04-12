import Foundation

/// Connects to Orion's /ws/voice WebSocket to receive Claude Code responses for TTS.
@Observable
final class VoiceConnection {
    private(set) var isConnected = false
    private(set) var connectionState = ConnectionState.disconnected
    /// Callback with (text, tmuxSession) — session may be empty
    var onVoiceText: ((String, String) -> Void)?

    private var host: String?
    private var token: String?
    private var webSocket: URLSessionWebSocketTask?
    private var session: URLSession?
    private var reconnectTask: Task<Void, Never>?
    private var pingTask: Task<Void, Never>?

    func connect(host: String, token: String) {
        self.host = host
        self.token = token
        disconnect()
        connectWebSocket(host: host, token: token)
    }

    private func connectWebSocket(host: String, token: String) {
        let encoded = token.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? token
        guard let url = URL(string: "ws://\(host)/ws/voice?token=\(encoded)") else { return }
        let config = URLSessionConfiguration.default
        config.shouldUseExtendedBackgroundIdleMode = true
        session = URLSession(configuration: config)
        webSocket = session?.webSocketTask(with: url)
        webSocket?.resume()
        isConnected = true
        connectionState = .connected
        receiveLoop()
        startPing()
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

    private func receiveLoop() {
        webSocket?.receive { [weak self] result in
            guard let self else { return }
            switch result {
            case .success(let message):
                self.handleMessage(message)
                self.receiveLoop()
            case .failure:
                DispatchQueue.main.async {
                    self.isConnected = false
                    self.attemptReconnect()
                }
            }
        }
    }

    private func handleMessage(_ message: URLSessionWebSocketTask.Message) {
        guard case .string(let text) = message,
              let data = text.data(using: .utf8),
              let msg = try? JSONDecoder().decode(VoiceMessage.self, from: data) else { return }
        switch msg.type {
        case "voice":
            if let voiceText = msg.text {
                DispatchQueue.main.async { self.onVoiceText?(voiceText, msg.session ?? "") }
            }
        case "pong":
            break // Ignore pong responses
        default:
            break
        }
    }

    // MARK: - Auto-Reconnect

    private func attemptReconnect() {
        guard let host, let token else {
            connectionState = .failed
            return
        }

        connectionState = .reconnecting
        reconnectTask = Task { [weak self] in
            let maxAttempts = 5

            for attempt in 1...maxAttempts {
                guard self != nil, !Task.isCancelled else { return }
                let delay = UInt64(pow(2.0, Double(attempt - 1))) // 1, 2, 4, 8, 16 seconds
                try? await Task.sleep(nanoseconds: delay * 1_000_000_000)
                guard let self, !Task.isCancelled else { return }

                // Just reconnect the WebSocket (no new ID needed for voice)
                await MainActor.run {
                    self.webSocket?.cancel(with: .normalClosure, reason: nil)
                    self.webSocket = nil
                    self.session?.invalidateAndCancel()
                    self.session = nil
                    self.connectWebSocket(host: host, token: token)
                }

                // Check if connected after a brief pause
                try? await Task.sleep(nanoseconds: 500_000_000) // 0.5s
                if self.isConnected { return } // Success
            }

            // All attempts failed
            guard let self, !Task.isCancelled else { return }
            await MainActor.run {
                self.connectionState = .failed
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
                guard let data = try? JSONEncoder().encode(["type": "ping"]),
                      let string = String(data: data, encoding: .utf8) else { return }
                self.webSocket?.send(.string(string)) { _ in }
            }
        }
    }
}

private struct VoiceMessage: Codable {
    let type: String
    var text: String?
    var session: String?
}
