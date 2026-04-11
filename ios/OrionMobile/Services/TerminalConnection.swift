import Foundation

@Observable
final class TerminalConnection {
    let terminalId: String
    let tmuxSession: String
    private(set) var isConnected = false
    var onOutput: (([UInt8]) -> Void)?
    var onExit: (() -> Void)?

    private var webSocket: URLSessionWebSocketTask?
    private var session: URLSession?
    private var pendingResize: (cols: Int, rows: Int)?

    init(terminalId: String, tmuxSession: String) {
        self.terminalId = terminalId
        self.tmuxSession = tmuxSession
    }

    deinit { disconnect() }

    func connect(host: String, token: String) {
        let encoded = token.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? token
        let encodedTmux = tmuxSession.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? tmuxSession
        guard let url = URL(string: "ws://\(host)/ws/terminal/\(terminalId)?token=\(encoded)&tmux=\(encodedTmux)") else { return }
        let config = URLSessionConfiguration.default
        config.shouldUseExtendedBackgroundIdleMode = true
        session = URLSession(configuration: config)
        webSocket = session?.webSocketTask(with: url)
        webSocket?.resume()
        isConnected = true
        receiveLoop()
        if let resize = pendingResize { sendResize(cols: resize.cols, rows: resize.rows); pendingResize = nil }
    }

    func disconnect() {
        webSocket?.cancel(with: .normalClosure, reason: nil)
        webSocket = nil
        session?.invalidateAndCancel()
        session = nil
        isConnected = false
    }

    func sendInput(_ data: [UInt8]) { send(WSMessage(type: "input", data: Data(data).base64EncodedString())) }
    func sendResize(cols: Int, rows: Int) {
        guard isConnected else { pendingResize = (cols, rows); return }
        send(WSMessage(type: "resize", cols: cols, rows: rows))
    }
    func sendScroll(direction: String, lines: Int) { send(WSMessage(type: "scroll", data: direction, cols: lines)) }

    private func send(_ message: WSMessage) {
        guard let data = try? JSONEncoder().encode(message), let string = String(data: data, encoding: .utf8) else { return }
        webSocket?.send(.string(string)) { _ in }
    }

    private func receiveLoop() {
        webSocket?.receive { [weak self] result in
            guard let self else { return }
            switch result {
            case .success(let message): self.handleMessage(message); self.receiveLoop()
            case .failure: DispatchQueue.main.async { self.isConnected = false; self.onExit?() }
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
            DispatchQueue.main.async { self.isConnected = false; self.onExit?() }
        default: break
        }
    }
}
