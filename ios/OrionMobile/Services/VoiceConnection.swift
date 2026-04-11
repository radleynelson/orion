import Foundation

/// Connects to Orion's /ws/voice WebSocket to receive Claude Code responses for TTS.
@Observable
final class VoiceConnection {
    private(set) var isConnected = false
    /// Callback with (text, tmuxSession) — session may be empty
    var onVoiceText: ((String, String) -> Void)?

    private var webSocket: URLSessionWebSocketTask?
    private var session: URLSession?

    func connect(host: String, token: String) {
        disconnect()
        let encoded = token.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? token
        guard let url = URL(string: "ws://\(host)/ws/voice?token=\(encoded)") else { return }
        let config = URLSessionConfiguration.default
        config.shouldUseExtendedBackgroundIdleMode = true
        session = URLSession(configuration: config)
        webSocket = session?.webSocketTask(with: url)
        webSocket?.resume()
        isConnected = true
        receiveLoop()
    }

    func disconnect() {
        webSocket?.cancel(with: .normalClosure, reason: nil)
        webSocket = nil
        session?.invalidateAndCancel()
        session = nil
        isConnected = false
    }

    private func receiveLoop() {
        webSocket?.receive { [weak self] result in
            guard let self else { return }
            switch result {
            case .success(let message):
                self.handleMessage(message)
                self.receiveLoop()
            case .failure:
                DispatchQueue.main.async { self.isConnected = false }
            }
        }
    }

    private func handleMessage(_ message: URLSessionWebSocketTask.Message) {
        guard case .string(let text) = message,
              let data = text.data(using: .utf8),
              let msg = try? JSONDecoder().decode(VoiceMessage.self, from: data) else { return }
        if msg.type == "voice", let voiceText = msg.text {
            DispatchQueue.main.async { self.onVoiceText?(voiceText, msg.session ?? "") }
        }
    }
}

private struct VoiceMessage: Codable {
    let type: String
    var text: String?
    var session: String?
}
