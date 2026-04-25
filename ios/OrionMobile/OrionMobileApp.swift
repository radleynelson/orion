import SwiftUI

@main
struct OrionMobileApp: App {
    @State private var appState = AppState()
    @Environment(\.scenePhase) private var scenePhase
    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(appState)
                .preferredColorScheme(.dark)
                .onChange(of: scenePhase) { _, newPhase in
                    appState.handleScenePhaseChange(newPhase)
                }
        }
    }
}

private func normalizeWorkspaceName(_ name: String) -> String {
    let lowercased = name.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    var output = ""
    var previousWasDash = false
    for scalar in lowercased.unicodeScalars {
        let isAllowed = CharacterSet.alphanumerics.contains(scalar) || scalar == "." || scalar == "_" || scalar == "-"
        if isAllowed {
            output.unicodeScalars.append(scalar)
            previousWasDash = scalar == "-"
        } else if !previousWasDash {
            output.append("-")
            previousWasDash = true
        }
    }
    return output.trimmingCharacters(in: CharacterSet(charactersIn: "-"))
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
    var activeTabId: String?
    var activeWorkspacePath: String?
    var selectedSessionByWorkspace: [String: String] = [:]
    var claudeViewModeBySession: [String: String] = [:]
    var activeConnection: TerminalConnection?
    var activeChatConnection: CodexChatConnection?
    var pendingKillSession: SessionInfo?
    let bonjour = BonjourDiscovery()
    let speech = SpeechService()
    let voiceConnection = VoiceConnection()
    var voiceModeEnabled = false
    var lastVoiceText: String?
    var showHome = true
    var showWorkspaces = false
    var showSettings = false
    var showDiffReview = false
    var connectionError: String?
    /// Transient error message shown as a toast at the top of the main view.
    /// Cleared automatically after 4 seconds.
    var transientError: String?
    var backgroundTaskId: UIBackgroundTaskIdentifier = .invalid
    private var activationGeneration = 0

    var activeWorkspace: Workspace? { workspaces.first { $0.path == activeWorkspacePath } }

    var visibleSessions: [SessionInfo] {
        let nonServerSessions = sessions.filter { $0.type != "server" }
        guard let activeWorkspacePath else { return nonServerSessions }
        return nonServerSessions.filter { $0.workspacePath == activeWorkspacePath }
    }

    var visibleTabs: [TerminalTab] {
        visibleSessions.map { TerminalTab(session: $0) }
    }

    var activeSession: SessionInfo? { sessions.first { $0.id == activeTabId || $0.tmuxName == activeTabId } }

    var activeTab: TerminalTab? {
        guard let activeSession else { return nil }
        return TerminalTab(session: activeSession)
    }

    var activeSessionShowsChat: Bool {
        guard let activeSession else { return false }
        return showsChat(activeSession)
    }

    var isReconnecting: Bool {
        activeConnection?.connectionState == .reconnecting ||
        activeChatConnection?.connectionState == .reconnecting ||
        voiceConnection.connectionState == .reconnecting
    }

    func showsChat(_ session: SessionInfo) -> Bool {
        if session.type == "codex-chat" || session.type == "claude-chat" {
            return true
        }
        if session.type == "claude" {
            return claudeViewModeBySession[session.tmuxName] == "chat"
        }
        return false
    }

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
            print("[Orion Voice] OpenAI key: \(key.isEmpty ? "missing" : "loaded (\(key.count) chars)")")
        } catch {
            print("[Orion Voice] Failed to fetch config: \(error)")
        }
        let env = ProcessInfo.processInfo.environment
        let requestedProject = env["ORION_MOBILE_PROJECT"].flatMap { projects.contains($0) ? $0 : nil }
        if let project = requestedProject ?? projects.first {
            try await selectProject(project)
            if let sessionId = env["ORION_MOBILE_SESSION"],
               let session = sessions.first(where: { $0.tmuxName == sessionId }) {
                try? await activateSession(session)
            }
        }
    }

    func disconnect() {
        voiceConnection.disconnect()
        disconnectActiveTerminal()
        client = nil
        isConnected = false
        selectedProject = nil
        projectInfo = nil
        workspaces = []
        sessions = []
        phoneLaunchedSessions = [:]
        showHome = true
        activeWorkspacePath = nil
        activeTabId = nil
        selectedSessionByWorkspace = [:]
        claudeViewModeBySession = [:]
        pendingKillSession = nil
    }

    func connectVoice() {
        voiceConnection.onVoiceText = { [weak self] text, _ in
            guard let self else { return }
            self.handleVoiceText(text)
        }
        voiceConnection.connect(host: host, token: token)
    }

    func handleVoiceText(_ text: String) {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        // Always store the last response for on-demand playback.
        lastVoiceText = trimmed
        // Only auto-speak if voice mode is on.
        guard voiceModeEnabled else { return }
        let rate = UserDefaults.standard.double(forKey: "ttsRate")
        speech.speakResponse(trimmed, rate: Float(rate > 0 ? rate : 0.52))
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
        let switchingProjects = selectedProject != nil && selectedProject != root
        if switchingProjects {
            disconnectActiveTerminal()
            sessions = []
            phoneLaunchedSessions = [:]
            showHome = true
            activeWorkspacePath = nil
            activeTabId = nil
            selectedSessionByWorkspace = [:]
            claudeViewModeBySession = [:]
        }
        selectedProject = root
        projectInfo = try await client.getProjectInfo(root: root)
        let ws = try await client.getWorkspaces(root: root)
        workspaces = ws
        reconcileActiveWorkspaceSelection()
        // Fetch sessions and agent types
        if let info = projectInfo {
            do { sessions = try await client.getSessions(repo: info.name, workspacePaths: ws.map(\.path)) } catch {}
            do { agentTypes = try await client.getAgentTypes(root: root) } catch {}
        }
        await ensureWorkspaceSelectionAttached()
    }

    @discardableResult
    func createWorkspace(name: String, baseRef: String, startWith: String, firstPrompt: String, codexOptions: CodexLaunchOptions = CodexLaunchOptions()) async throws -> Workspace {
        guard let client, let root = selectedProject else { throw OrionError.invalidResponse }
        let normalizedName = normalizeWorkspaceName(name)
        let workspace = try await client.createWorkspace(root: root, name: normalizedName, baseRef: baseRef)
        workspaces = try await client.getWorkspaces(root: root)
        activeWorkspacePath = workspace.path
        let prompt = firstPrompt.trimmingCharacters(in: .whitespacesAndNewlines)

        switch startWith {
        case "codex-chat":
            let session = try await launchCodexChat(workspacePath: workspace.path, options: codexOptions)
            if !prompt.isEmpty { try await client.sendCodexChatMessage(sessionId: session.chatConnectionId, text: prompt) }
        case "claude-chat":
            let session = try await launchClaudeChat(workspacePath: workspace.path)
            if !prompt.isEmpty { try await client.sendClaudeChatMessage(sessionId: session.chatConnectionId, text: prompt) }
        case "codex", "claude":
            try await launchAgent(workspacePath: workspace.path, agentType: startWith)
        case "shell":
            try await launchShell(workspacePath: workspace.path)
        default:
            break
        }

        return workspace
    }

    func changedFiles(workspacePath explicitWorkspacePath: String? = nil, base: String = "") async throws -> [GitChangedFile] {
        guard let client, let workspacePath = explicitWorkspacePath ?? activeWorkspacePath else { return [] }
        return try await client.getChangedFiles(workspacePath: workspacePath, base: base)
    }

    func unifiedDiff(for file: GitChangedFile, base: String = "") async throws -> String {
        guard let client, let workspacePath = activeWorkspacePath else { return "" }
        return try await client.getUnifiedDiff(workspacePath: workspacePath, base: base, filePath: file.path)
    }

    func refreshSessions() async {
        guard let client, let info = projectInfo, !workspaces.isEmpty else { return }
        do {
            var fetched = try await client.getSessions(repo: info.name, workspacePaths: workspaces.map(\.path))
            // Override with phone-launched session info (correct type/label)
            for i in fetched.indices {
                if let better = phoneLaunchedSessions[fetched[i].id] ?? phoneLaunchedSessions[fetched[i].tmuxName] {
                    fetched[i] = better
                }
            }
            sessions = fetched
            let liveSessions = Set(fetched.flatMap { [$0.id, $0.tmuxName, $0.threadId ?? "", $0.runtimeSessionId ?? ""] }.filter { !$0.isEmpty })
            phoneLaunchedSessions = phoneLaunchedSessions.filter { liveSessions.contains($0.key) }
            await ensureWorkspaceSelectionAttached()
        } catch {}
    }

    func activateSession(_ session: SessionInfo, showSession: Bool = true) async throws {
        guard let client else { return }

        if showSession { showHome = false }
        activeWorkspacePath = session.workspacePath
        activeTabId = session.id
        selectedSessionByWorkspace[session.workspacePath] = session.id

        if showsChat(session) {
            connectChatSession(session)
            return
        }

        if activeConnection?.tmuxSession == session.terminalTmuxSession {
            if let activeConnection, !activeConnection.isConnected, activeConnection.connectionState != .reconnecting {
                activeConnection.connect(host: host, token: token)
            }
            return
        }

        disconnectActiveTerminal()
        activationGeneration += 1
        let generation = activationGeneration

        let resp = try await client.createTerminal(tmuxSession: session.terminalTmuxSession)
        guard generation == activationGeneration else { return }

        let connection = TerminalConnection(terminalId: resp.terminalId, tmuxSession: session.terminalTmuxSession)
        connection.onExit = { [weak self] in
            Task { @MainActor in
                self?.handleSessionExit(tmuxSession: session.terminalTmuxSession, workspacePath: session.workspacePath)
            }
        }
        connection.onPermanentFailure = { [weak self] in
            self?.showTransientError("\(session.label) disconnected. Tap Reconnect to resume.")
        }

        activeConnection = connection
        connection.connect(host: host, token: token)
    }

    func launchShell(workspacePath: String) async throws {
        guard let client, let root = selectedProject else { return }
        let resp = try await client.launchShell(repoRoot: root, workspacePath: workspacePath)
        let session = SessionInfo(tmuxName: resp.tmuxSession, type: "shell", label: "Shell", workspacePath: workspacePath)
        phoneLaunchedSessions[resp.tmuxSession] = session
        await refreshSessions()
        if let refreshed = sessions.first(where: { $0.tmuxName == resp.tmuxSession }) {
            try await activateSession(refreshed)
        } else {
            try await activateSession(session)
        }
    }

    func launchAgent(workspacePath: String, agentType: String) async throws {
        guard let client, let root = selectedProject else { return }
        let resp = try await client.launchAgent(repoRoot: root, workspacePath: workspacePath, agentType: agentType)
        let label = String(agentType.prefix(1)).uppercased() + agentType.dropFirst()
        let session = SessionInfo(tmuxName: resp.tmuxSession, type: agentType, label: label, workspacePath: workspacePath)
        phoneLaunchedSessions[resp.tmuxSession] = session
        await refreshSessions()
        if let refreshed = sessions.first(where: { $0.tmuxName == resp.tmuxSession }) {
            try await activateSession(refreshed)
        } else {
            try await activateSession(session)
        }
    }

    @discardableResult
    func launchCodexChat(workspacePath: String, options: CodexLaunchOptions = CodexLaunchOptions(), threadId: String? = nil) async throws -> SessionInfo {
        guard let client, let root = selectedProject else { throw OrionError.invalidResponse }
        let resp = try await client.launchCodexChat(repoRoot: root, workspacePath: workspacePath, threadId: threadId, options: options)
        let session = SessionInfo(
            tmuxName: resp.threadId ?? resp.id,
            type: resp.type,
            label: resp.label,
            workspacePath: resp.workspacePath,
            provider: resp.provider ?? "codex",
            viewMode: resp.viewMode ?? "chat",
            runtimeSessionId: resp.runtimeSessionId ?? resp.id,
            threadId: resp.threadId,
            model: resp.model,
            reasoningEffort: resp.reasoningEffort,
            approvalPolicy: resp.approvalPolicy,
            sandboxMode: resp.sandboxMode,
            collaborationMode: resp.collaborationMode
        )
        phoneLaunchedSessions[session.id] = session
        await refreshSessions()
        if let refreshed = sessions.first(where: { $0.id == session.id || $0.threadId == resp.threadId }) {
            try await activateSession(refreshed)
            return refreshed
        } else {
            sessions.append(session)
            try await activateSession(session)
            return session
        }
    }

    @discardableResult
    func resumeCodexChat(workspacePath: String, threadId: String) async throws -> SessionInfo {
        try await launchCodexChat(workspacePath: workspacePath, threadId: threadId)
    }

    @discardableResult
    func launchClaudeChat(workspacePath: String) async throws -> SessionInfo {
        guard let client, let root = selectedProject else { throw OrionError.invalidResponse }
        let resp = try await client.launchClaudeChat(repoRoot: root, workspacePath: workspacePath)
        let session = SessionInfo(tmuxName: resp.id, type: resp.type, label: resp.label, workspacePath: resp.workspacePath, provider: resp.provider ?? "claude", viewMode: resp.viewMode ?? "chat", runtimeSessionId: resp.runtimeSessionId ?? resp.id, threadId: resp.threadId)
        claudeViewModeBySession[resp.id] = "chat"
        phoneLaunchedSessions[resp.id] = session
        await refreshSessions()
        if let refreshed = sessions.first(where: { $0.tmuxName == resp.id }) {
            try await activateSession(refreshed)
            return refreshed
        } else {
            sessions.append(session)
            try await activateSession(session)
            return session
        }
    }

    func convertSession(_ session: SessionInfo) async {
        guard let client, let root = selectedProject else { return }
        do {
            if session.type == "claude" {
                if showsChat(session) {
                    claudeViewModeBySession[session.tmuxName] = "terminal"
                    disconnectActiveTerminal()
                    try await activateSession(session)
                } else {
                    activeWorkspacePath = session.workspacePath
                    activeTabId = session.tmuxName
                    selectedSessionByWorkspace[session.workspacePath] = session.tmuxName
                    connectChatSession(session)
                    claudeViewModeBySession[session.tmuxName] = "chat"
                }
                return
            }

            if session.isChat {
                let kind = session.type == "claude-chat" ? "claude" : "codex"
                let resp = try await client.convertChatToTerminal(repoRoot: root, workspacePath: session.workspacePath, sessionId: session.chatConnectionId, chatKind: kind)
                let label = kind == "claude" ? "Claude" : "Codex"
                let converted = SessionInfo(tmuxName: resp.tmuxSession, type: kind, label: label, workspacePath: session.workspacePath, provider: kind, viewMode: "terminal", runtimeSessionId: resp.tmuxSession, threadId: session.threadId)
                sessions.removeAll { $0.id == session.id }
                phoneLaunchedSessions.removeValue(forKey: session.id)
                phoneLaunchedSessions[resp.tmuxSession] = converted
                await refreshSessions()
                if let refreshed = sessions.first(where: { $0.tmuxName == resp.tmuxSession }) {
                    try await activateSession(refreshed)
                } else {
                    sessions.append(converted)
                    try await activateSession(converted)
                }
                return
            }

            guard session.type == "claude" || session.type == "codex" else { return }
            let resp = session.type == "claude"
                ? try await client.launchClaudeChat(repoRoot: root, workspacePath: session.workspacePath)
                : try await client.launchCodexChat(repoRoot: root, workspacePath: session.workspacePath, tmuxSession: session.terminalTmuxSession)
            let converted = SessionInfo(
                tmuxName: session.type == "codex" ? (resp.threadId ?? resp.id) : resp.id,
                type: resp.type,
                label: resp.label,
                workspacePath: resp.workspacePath,
                provider: session.type,
                viewMode: "chat",
                runtimeSessionId: resp.runtimeSessionId ?? resp.id,
                threadId: resp.threadId,
                model: resp.model,
                reasoningEffort: resp.reasoningEffort,
                approvalPolicy: resp.approvalPolicy,
                sandboxMode: resp.sandboxMode,
                collaborationMode: resp.collaborationMode
            )
            if activeConnection?.tmuxSession == session.tmuxName {
                disconnectActiveTerminal()
            }
            sessions.removeAll { $0.tmuxName == session.tmuxName }
            phoneLaunchedSessions.removeValue(forKey: session.tmuxName)
            if session.type == "codex" {
                try? await client.killSession(tmuxSession: session.terminalTmuxSession)
            }
            phoneLaunchedSessions[converted.id] = converted
            await refreshSessions()
            if let refreshed = sessions.first(where: { $0.id == converted.id || $0.threadId == converted.threadId }) {
                try await activateSession(refreshed)
            } else {
                sessions.append(converted)
                try await activateSession(converted)
            }
        } catch {
            showTransientError("Failed to convert session: \(error.localizedDescription)")
        }
    }

    func showTransientError(_ message: String) {
        transientError = message
        Task { @MainActor in
            try? await Task.sleep(for: .seconds(4))
            if transientError == message { transientError = nil }
        }
    }

    func requestKillSession(_ sessionId: String) {
        pendingKillSession = sessions.first { $0.id == sessionId || $0.tmuxName == sessionId }
    }

    func activateTab(_ tabId: String) {
        guard let session = sessions.first(where: { $0.id == tabId || $0.tmuxName == tabId }) else { return }
        Task {
            try? await activateSession(session)
        }
    }

    func activateWorkspace(_ path: String) async {
        showHome = true
        activeWorkspacePath = path
        await ensureWorkspaceSelectionAttached()
    }

    @discardableResult
    func launchChatWithPrompt(workspacePath: String, provider: String, prompt: String, codexOptions: CodexLaunchOptions = CodexLaunchOptions()) async throws -> SessionInfo {
        guard let client else { throw OrionError.invalidResponse }
        let trimmed = prompt.trimmingCharacters(in: .whitespacesAndNewlines)
        if provider == "claude-chat" {
            let session = try await launchClaudeChat(workspacePath: workspacePath)
            if !trimmed.isEmpty {
                try await client.sendClaudeChatMessage(sessionId: session.chatConnectionId, text: trimmed)
            }
            return session
        }

        let session = try await launchCodexChat(workspacePath: workspacePath, options: codexOptions)
        if !trimmed.isEmpty {
            try await client.sendCodexChatMessage(sessionId: session.chatConnectionId, text: trimmed)
        }
        return session
    }

    func codexHistory(workspace: Workspace, limit: Int = 20) async -> [CodexHistoryThread] {
        guard let client else { return [] }
        do { return try await client.getCodexHistory(workspacePath: workspace.path, limit: limit) }
        catch { return [] }
    }

    // MARK: - Kill Session

    func killSession(_ session: SessionInfo) async {
        guard let client else { return }

        if activeConnection?.tmuxSession == session.terminalTmuxSession {
            disconnectActiveTerminal()
        }
        if activeChatConnection?.sessionId == session.chatConnectionId {
            disconnectActiveTerminal()
        }
        if activeTabId == session.id {
            activeTabId = nil
        }
        if selectedSessionByWorkspace[session.workspacePath] == session.id {
            selectedSessionByWorkspace.removeValue(forKey: session.workspacePath)
        }
        sessions.removeAll { $0.id == session.id || $0.tmuxName == session.tmuxName }
        phoneLaunchedSessions.removeValue(forKey: session.id)
        phoneLaunchedSessions.removeValue(forKey: session.tmuxName)
        // Then kill on server and refresh in background
        if !session.isChat || session.isClaude {
            try? await client.killSession(tmuxSession: session.terminalTmuxSession)
        }
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

    // MARK: - Scene Phase / Background

    func handleScenePhaseChange(_ phase: ScenePhase) {
        switch phase {
        case .active:
            reconnectDeadConnections()
            // End background task if one was active
            if backgroundTaskId != .invalid {
                UIApplication.shared.endBackgroundTask(backgroundTaskId)
                backgroundTaskId = .invalid
            }
        case .background:
            if voiceModeEnabled {
                backgroundTaskId = UIApplication.shared.beginBackgroundTask { [weak self] in
                    guard let self else { return }
                    if self.backgroundTaskId != .invalid {
                        UIApplication.shared.endBackgroundTask(self.backgroundTaskId)
                        self.backgroundTaskId = .invalid
                    }
                }
            }
        case .inactive:
            break
        @unknown default:
            break
        }
    }

    func reconnectDeadConnections() {
        guard isConnected, !host.isEmpty, !token.isEmpty else { return }

        if let activeConnection,
           activeSession != nil,
           !activeConnection.isConnected,
           activeConnection.connectionState != .reconnecting {
            activeConnection.connect(host: host, token: token)
        }
        if let activeChatConnection,
           activeSessionShowsChat,
           !activeChatConnection.isConnected,
           activeChatConnection.connectionState != .reconnecting {
            activeChatConnection.connect(host: host, token: token)
        } else if let activeChatConnection,
                  activeSessionShowsChat,
                  activeChatConnection.connectionState == .connected {
            activeChatConnection.reconnectOrProbe()
        }

        // Reconnect voice WebSocket if voice mode is on and it's disconnected
        if voiceModeEnabled && !voiceConnection.isConnected && voiceConnection.connectionState == .disconnected {
            connectVoice()
        }
    }

    private func disconnectActiveTerminal() {
        activationGeneration += 1
        activeConnection?.disconnect()
        activeConnection = nil
        activeChatConnection?.disconnect()
        activeChatConnection = nil
    }

    private func connectChatSession(_ session: SessionInfo) {
        if activeChatConnection?.sessionId == session.chatConnectionId {
            if let activeChatConnection, !activeChatConnection.isConnected, activeChatConnection.connectionState != .reconnecting {
                activeChatConnection.connect(host: host, token: token)
            }
            return
        }

        activationGeneration += 1
        let oldTerminal = activeConnection
        let oldChat = activeChatConnection
        let connection = CodexChatConnection(sessionId: session.chatConnectionId, sessionType: session.type, workspacePath: session.workspacePath)
        connection.onPermanentFailure = { [weak self] in
            self?.showTransientError("\(session.label) disconnected. Tap Reconnect to resume.")
        }
        connection.onAssistantVoiceText = { [weak self, weak connection] text in
            guard let self, let connection, self.activeChatConnection === connection else { return }
            self.handleVoiceText(text)
        }

        activeConnection = nil
        activeChatConnection = connection
        oldTerminal?.disconnect()
        oldChat?.disconnect()
        connection.connect(host: host, token: token)
    }

    private func reconcileActiveWorkspaceSelection() {
        if let activeWorkspacePath, workspaces.contains(where: { $0.path == activeWorkspacePath }) {
            return
        }
        if let mainWorkspace = workspaces.first(where: \.isMain) {
            activeWorkspacePath = mainWorkspace.path
            return
        }
        if let firstWorkspace = workspaces.first {
            activeWorkspacePath = firstWorkspace.path
            return
        }
        activeWorkspacePath = nil
        activeTabId = nil
    }

    private func preferredSession(in workspacePath: String) -> SessionInfo? {
        let workspaceSessions = sessions.filter { $0.workspacePath == workspacePath && $0.type != "server" }
        guard !workspaceSessions.isEmpty else { return nil }
        if let selected = selectedSessionByWorkspace[workspacePath],
           let session = workspaceSessions.first(where: { $0.id == selected || $0.tmuxName == selected }) {
            return session
        }
        if let activeSession, activeSession.workspacePath == workspacePath,
           let session = workspaceSessions.first(where: { $0.id == activeSession.id || $0.tmuxName == activeSession.tmuxName }) {
            return session
        }
        return workspaceSessions.first
    }

    private func ensureWorkspaceSelectionAttached() async {
        reconcileActiveWorkspaceSelection()

        guard let activeWorkspacePath else {
            activeTabId = nil
            disconnectActiveTerminal()
            return
        }

        guard let preferred = preferredSession(in: activeWorkspacePath) else {
            activeTabId = nil
            disconnectActiveTerminal()
            return
        }

        if activeTabId != preferred.id {
            activeTabId = preferred.id
        }
        selectedSessionByWorkspace[activeWorkspacePath] = preferred.id

        if let activeConnection, activeConnection.tmuxSession == preferred.terminalTmuxSession {
            if !activeConnection.isConnected && activeConnection.connectionState != .reconnecting {
                activeConnection.connect(host: host, token: token)
            }
            return
        }

        try? await activateSession(preferred, showSession: false)
    }

    private func handleSessionExit(tmuxSession: String, workspacePath: String) {
        if activeConnection?.tmuxSession == tmuxSession {
            disconnectActiveTerminal()
        }
        if activeTabId == tmuxSession {
            activeTabId = nil
        }
        if selectedSessionByWorkspace[workspacePath] == tmuxSession {
            selectedSessionByWorkspace.removeValue(forKey: workspacePath)
        }
        phoneLaunchedSessions.removeValue(forKey: tmuxSession)
        sessions.removeAll { $0.tmuxName == tmuxSession }

        Task {
            await ensureWorkspaceSelectionAttached()
        }
    }
}
