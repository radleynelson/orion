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
    var agentTypes: [AgentType] = []
    // Track sessions launched from the phone with their correct types/labels
    // so refreshSessions doesn't overwrite them with "Shell"
    var phoneLaunchedSessions: [String: SessionInfo] = [:] // tmuxName -> SessionInfo
    var tabs: [TerminalTab] = []
    var activeTabId: String?
    var connections: [String: TerminalConnection] = [:]
    let bonjour = BonjourDiscovery()
    let speech = SpeechService()
    let voiceConnection = VoiceConnection()
    var voiceModeEnabled = false
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
        // Connect voice WebSocket and fetch config
        connectVoice()
        do {
            let config = try await client.getConfig()
            let key = config.openaiApiKey ?? ""
            speech.openAIApiKey = key
            print("[Orion Voice] Fetched OpenAI key: \(key.isEmpty ? "EMPTY" : "\(key.prefix(12))... (\(key.count) chars)")")
        } catch {
            print("[Orion Voice] Failed to fetch config: \(error)")
        }
        if let first = projects.first { try await selectProject(first) }
    }

    func disconnect() {
        voiceConnection.disconnect()
        for (_, conn) in connections { conn.disconnect() }
        connections.removeAll(); tabs.removeAll(); activeTabId = nil; client = nil; isConnected = false
        selectedProject = nil; projectInfo = nil; workspaces = []; sessions = []
    }

    func connectVoice() {
        voiceConnection.onVoiceText = { [weak self] text, session in
            guard let self, self.voiceModeEnabled else { return }
            // TODO: Filter by active session once we reliably get tmux session names from hooks.
            // For now, read all Claude responses when voice mode is on.
            let rate = UserDefaults.standard.double(forKey: "ttsRate")
            self.speech.speakResponse(text, rate: Float(rate > 0 ? rate : 0.52))
        }
        voiceConnection.connect(host: host, token: token)
    }

    func toggleVoiceMode() {
        voiceModeEnabled.toggle()
        if !voiceModeEnabled { speech.stopSpeaking() }
        // Reconnect voice WS if disconnected
        if voiceModeEnabled && !voiceConnection.isConnected {
            connectVoice()
        }
    }

    func selectProject(_ root: String) async throws {
        guard let client else { return }
        selectedProject = root
        projectInfo = try await client.getProjectInfo(root: root)
        let ws = try await client.getWorkspaces(root: root)
        workspaces = ws
        // Fetch sessions and agent types
        if let info = projectInfo {
            do { sessions = try await client.getSessions(repo: info.name, workspacePaths: ws.map(\.path)) } catch {}
            do { agentTypes = try await client.getAgentTypes(root: root) } catch {}
        }
    }

    func refreshSessions() async {
        guard let client, let info = projectInfo, !workspaces.isEmpty else { return }
        do {
            var fetched = try await client.getSessions(repo: info.name, workspacePaths: workspaces.map(\.path))
            // Override with phone-launched session info (correct type/label)
            for i in fetched.indices {
                if let better = phoneLaunchedSessions[fetched[i].tmuxName] {
                    fetched[i] = better
                }
            }
            sessions = fetched
        } catch {}
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
        let session = SessionInfo(tmuxName: resp.tmuxSession, type: "shell", label: "Shell", workspacePath: workspacePath)
        phoneLaunchedSessions[resp.tmuxSession] = session
        try await openSession(session)
        await refreshSessions()
    }

    func launchAgent(workspacePath: String, agentType: String) async throws {
        guard let client, let root = selectedProject else { return }
        let resp = try await client.launchAgent(repoRoot: root, workspacePath: workspacePath, agentType: agentType)
        let label = String(agentType.prefix(1)).uppercased() + agentType.dropFirst()
        let session = SessionInfo(tmuxName: resp.tmuxSession, type: agentType, label: label, workspacePath: workspacePath)
        phoneLaunchedSessions[resp.tmuxSession] = session
        try await openSession(session)
        await refreshSessions()
    }

    func closeTab(_ tabId: String) {
        guard let tab = tabs.first(where: { $0.id == tabId }) else { return }
        connections[tab.terminalId]?.disconnect(); connections.removeValue(forKey: tab.terminalId)
        tabs.removeAll { $0.id == tabId }
        if activeTabId == tabId { activeTabId = tabs.last?.id }
    }

    func activateTab(_ tabId: String) { activeTabId = tabId }

    // MARK: - Kill Session

    func killSession(_ session: SessionInfo) async {
        guard let client else { return }
        // Close the tab if it's open
        if let tab = tabs.first(where: { $0.tmuxSession == session.tmuxName }) {
            closeTab(tab.id)
        }
        // Remove from local state FIRST so the List animation stays in sync
        sessions.removeAll { $0.tmuxName == session.tmuxName }
        // Then kill on server and refresh in background
        try? await client.killSession(tmuxSession: session.tmuxName)
        // Small delay to let the List animation finish before refreshing
        try? await Task.sleep(for: .milliseconds(500))
        await refreshSessions()
    }

    // MARK: - Server Management

    func getServerStatuses(workspace: Workspace) async -> [ServerStatus] {
        guard let client, let root = selectedProject else { return [] }
        do { return try await client.getServerStatuses(root: root, workspace: workspace.path) }
        catch { return [] }
    }

    func startServers(workspace: Workspace) async {
        guard let client, let root = selectedProject else { return }
        do { let _ = try await client.startServers(repoRoot: root, workspacePath: workspace.path, isMain: workspace.isMain) }
        catch {}
        await refreshSessions()
    }

    func stopServers(workspace: Workspace) async {
        guard let client else { return }
        try? await client.stopServers(workspacePath: workspace.path)
        await refreshSessions()
    }
}
