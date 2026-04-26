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

struct GitChangedFile: Codable, Identifiable {
    var id: String { path }
    let path: String
    let status: String
    let statusText: String
}

struct GitDiffResponse: Codable {
    let diff: String
}

struct SessionInfo: Codable, Identifiable {
    var id: String { runtimeSessionId ?? threadId ?? tmuxName }
    let tmuxName: String
    let type: String
    let label: String
    let workspacePath: String
    let provider: String?
    let viewMode: String?
    let runtimeSessionId: String?
    let threadId: String?
    let model: String?
    let reasoningEffort: String?
    let approvalPolicy: String?
    let sandboxMode: String?
    let permissionMode: String?
    let collaborationMode: String?

    var isChat: Bool { type == "codex-chat" || type == "claude-chat" }
    var isClaude: Bool { type == "claude" || type == "claude-chat" }
    var chatConnectionId: String { runtimeSessionId ?? threadId ?? tmuxName }
    var terminalTmuxSession: String { tmuxName }

    init(
        tmuxName: String,
        type: String,
        label: String,
        workspacePath: String,
        provider: String? = nil,
        viewMode: String? = nil,
        runtimeSessionId: String? = nil,
        threadId: String? = nil,
        model: String? = nil,
        reasoningEffort: String? = nil,
        approvalPolicy: String? = nil,
        sandboxMode: String? = nil,
        permissionMode: String? = nil,
        collaborationMode: String? = nil
    ) {
        self.tmuxName = tmuxName
        self.type = type
        self.label = label
        self.workspacePath = workspacePath
        self.provider = provider
        self.viewMode = viewMode
        self.runtimeSessionId = runtimeSessionId
        self.threadId = threadId
        self.model = model
        self.reasoningEffort = reasoningEffort
        self.approvalPolicy = approvalPolicy
        self.sandboxMode = sandboxMode
        self.permissionMode = permissionMode
        self.collaborationMode = collaborationMode
    }
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
        self.id = session.id
        self.label = session.label
        self.type = session.type
        self.tmuxSession = session.terminalTmuxSession
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
    let provider: String?
    let model: String?
    let reasoningEffort: String?
    let approvalPolicy: String?
    let sandboxMode: String?
    let permissionMode: String?
    let collaborationMode: String?
    let chatCapable: Bool?
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
    let provider: String?
    let viewMode: String?
    let runtimeSessionId: String?
    let model: String?
    let reasoningEffort: String?
    let approvalPolicy: String?
    let sandboxMode: String?
    let permissionMode: String?
    let collaborationMode: String?
}

typealias LaunchClaudeChatResponse = LaunchCodexChatResponse

struct CodexHistoryThread: Codable, Identifiable {
    var id: String { threadId }
    let threadId: String
    let workspacePath: String?
    let model: String?
    let updatedAt: String
    let messageCount: Int
    let preview: String?
}

struct CodexLaunchOptions: Codable, Equatable {
    var model = "gpt-5.4"
    var reasoningEffort = "xhigh"
    var approvalPolicy = "never"
    var sandboxMode = "danger-full-access"
    var collaborationMode = "default"
}

struct ClaudeLaunchOptions: Codable, Equatable {
    var model: String?
    var reasoningEffort: String?
    var approvalPolicy: String?
    var sandboxMode: String?
    var permissionMode: String?
}

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
