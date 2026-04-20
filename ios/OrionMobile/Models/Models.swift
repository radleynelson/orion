import Foundation

// MARK: - API Response Models (match Go backend JSON)

struct ProjectInfo: Codable {
    let name: String
    let root: String
    let mainBranch: String
}

struct Workspace: Codable, Identifiable {
    var id: String { path }
    let name: String
    let path: String
    let branch: String
    let isMain: Bool
    let hasAgent: Bool
}

struct SessionInfo: Codable, Identifiable {
    var id: String { tmuxName }
    let tmuxName: String
    let type: String
    let label: String
    let workspacePath: String

    var isChat: Bool { type == "codex-chat" || type == "claude-chat" }
    var isClaude: Bool { type == "claude" || type == "claude-chat" }
}

struct CreateTerminalResponse: Codable {
    let terminalId: String
    let tmuxSession: String
}

struct LaunchShellResponse: Codable {
    let tmuxSession: String
}

// MARK: - Connection State

enum ConnectionState: Equatable {
    case disconnected
    case connected
    case reconnecting
    case failed
}

// MARK: - App Models

struct TerminalTab: Identifiable {
    let id: String
    let label: String
    let type: String
    let tmuxSession: String
    let workspacePath: String

    init(session: SessionInfo) {
        self.id = session.tmuxName
        self.label = session.label
        self.type = session.type
        self.tmuxSession = session.tmuxName
        self.workspacePath = session.workspacePath
    }
}

struct SavedConnection: Codable, Identifiable {
    var id: String { host }
    let host: String
    let token: String
    let name: String?
}

struct DiscoveredHost: Identifiable {
    let id = UUID()
    let name: String
    let host: String
    let port: Int

    var address: String { "\(host):\(port)" }
}

struct ServerStatus: Codable, Identifiable {
    var id: String { name }
    let name: String
    let port: Int
    let running: Bool
    let tmuxSession: String
}

struct AgentType: Codable, Identifiable {
    var id: String { name }
    let name: String
    let label: String
}

struct LaunchAgentResponse: Codable {
    let tmuxSession: String
}

struct LaunchCodexChatResponse: Codable {
    let id: String
    let type: String
    let label: String
    let workspacePath: String
    let status: String
    let threadId: String?
}

typealias LaunchClaudeChatResponse = LaunchCodexChatResponse

struct StartServersRequest: Codable {
    let repoRoot: String
    let workspacePath: String
    let isMain: Bool
}

struct AppConfig: Codable {
    let openaiApiKey: String?
}

// MARK: - WebSocket Message Types

struct WSMessage: Codable {
    let type: String
    var data: String?
    var cols: Int?
    var rows: Int?
}

struct CodexChatMessage: Codable, Identifiable {
    let id: String
    let sessionId: String
    let threadId: String?
    let type: String
    let subtype: String?
    let role: String?
    let text: String?
    let status: String?
    let toolUseId: String?
    let toolName: String?
    let details: String?
    let planPath: String?
    let attachments: [ChatAttachmentPayload]?
    let createdAt: String
}

struct ChatAttachmentPayload: Codable {
    var id: String?
    var name: String?
    var path: String?
    var mimeType: String?
    var size: Int64?
    var data: String?
}

struct CodexChatWSMessage: Codable {
    let type: String
    var text: String?
    var toolUseId: String?
    var action: String?
    var attachments: [ChatAttachmentPayload]?
}
