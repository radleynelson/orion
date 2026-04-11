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
}

struct CreateTerminalResponse: Codable {
    let terminalId: String
    let tmuxSession: String
}

struct LaunchShellResponse: Codable {
    let tmuxSession: String
}

// MARK: - App Models

struct TerminalTab: Identifiable {
    let id: String
    let label: String
    let tmuxSession: String
    let terminalId: String
    let workspacePath: String

    init(label: String, tmuxSession: String, terminalId: String, workspacePath: String) {
        self.id = "tab-\(Int(Date().timeIntervalSince1970 * 1000))"
        self.label = label
        self.tmuxSession = tmuxSession
        self.terminalId = terminalId
        self.workspacePath = workspacePath
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

// MARK: - WebSocket Message Types

struct WSMessage: Codable {
    let type: String
    var data: String?
    var cols: Int?
    var rows: Int?
}
