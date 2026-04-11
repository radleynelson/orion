import SwiftUI

@main
struct OrionMobileApp: App {
    @State private var appState = AppState()
    var body: some Scene {
        WindowGroup { ContentView().environment(appState).preferredColorScheme(.dark) }
    }
}

@Observable
final class AppState {
    var isConnected = false
    var host = ""
    var token = ""
    var client: OrionClient?
    var projects: [String] = []
    var selectedProject: String?
    var projectInfo: ProjectInfo?
    var workspaces: [Workspace] = []
    var sessions: [SessionInfo] = []
    var tabs: [TerminalTab] = []
    var activeTabId: String?
    var connections: [String: TerminalConnection] = [:]
    let bonjour = BonjourDiscovery()
    let speech = SpeechService()
    var showWorkspaces = false
    var showSettings = false
    var connectionError: String?

    var activeTab: TerminalTab? { tabs.first { $0.id == activeTabId } }

    func connect(host: String, token: String) async throws {
        let client = OrionClient(host: host, token: token)
        let projects = try await client.getProjects()
        self.client = client; self.host = host; self.token = token; self.projects = projects; self.isConnected = true; self.connectionError = nil
        KeychainService.saveToken(token, for: host)
        var saved = KeychainService.loadConnections(); saved.removeAll { $0.host == host }
        saved.insert(SavedConnection(host: host, token: token, name: nil), at: 0)
        if saved.count > 5 { saved = Array(saved.prefix(5)) }; KeychainService.saveConnections(saved)
        if let first = projects.first { try await selectProject(first) }
    }

    func disconnect() {
        for (_, conn) in connections { conn.disconnect() }
        connections.removeAll(); tabs.removeAll(); activeTabId = nil; client = nil; isConnected = false
        selectedProject = nil; projectInfo = nil; workspaces = []; sessions = []
    }

    func selectProject(_ root: String) async throws {
        guard let client else { return }
        selectedProject = root; projectInfo = try await client.getProjectInfo(root: root)
        workspaces = try await client.getWorkspaces(root: root); await refreshSessions()
    }

    func refreshSessions() async {
        guard let client, let info = projectInfo else { return }
        do { sessions = try await client.getSessions(repo: info.name, workspacePaths: workspaces.map(\.path)) } catch {}
    }

    func openSession(_ session: SessionInfo) async throws {
        if let existing = tabs.first(where: { $0.tmuxSession == session.tmuxName }) { activeTabId = existing.id; return }
        guard let client else { return }
        let resp = try await client.createTerminal(tmuxSession: session.tmuxName)
        let tab = TerminalTab(label: session.label, tmuxSession: session.tmuxName, terminalId: resp.terminalId, workspacePath: session.workspacePath)
        let connection = TerminalConnection(terminalId: resp.terminalId, tmuxSession: session.tmuxName)
        connection.onExit = { [weak self] in self?.closeTab(tab.id) }
        connections[resp.terminalId] = connection; tabs.append(tab); activeTabId = tab.id
        connection.connect(host: host, token: token)
    }

    func launchShell(workspacePath: String) async throws {
        guard let client, let root = selectedProject else { return }
        let resp = try await client.launchShell(repoRoot: root, workspacePath: workspacePath)
        await refreshSessions()
        if let session = sessions.first(where: { $0.tmuxName == resp.tmuxSession }) { try await openSession(session) }
    }

    func closeTab(_ tabId: String) {
        guard let tab = tabs.first(where: { $0.id == tabId }) else { return }
        connections[tab.terminalId]?.disconnect(); connections.removeValue(forKey: tab.terminalId)
        tabs.removeAll { $0.id == tabId }
        if activeTabId == tabId { activeTabId = tabs.last?.id }
    }

    func activateTab(_ tabId: String) { activeTabId = tabId }
}
