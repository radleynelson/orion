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
    var agentViewModeBySession: [String: String] = [:]
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
    var suppressNextAutoConnect = false
    /// Transient error message shown as a toast at the top of the main view.
    /// Cleared automatically after 4 seconds.
    var transientError: String?
    var backgroundTaskId: UIBackgroundTaskIdentifier = .invalid
    private var activationGeneration = 0

    var activeWorkspace: Workspace? { workspaces.first { $0.path == activeWorkspacePath } }

    var visibleSessions: [SessionInfo] {
        guard let activeWorkspacePath else { return sessions }
        return sessions.filter { $0.workspacePath == activeWorkspacePath }
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
        activeChatConnection?.connectionState == .reconnecting
    }

    func showsChat(_ session: SessionInfo) -> Bool {
        if session.isChat {
            return true
        }
        guard isTranscriptChatCapable(session) else { return false }
        return agentViewModeBySession[viewModeKey(for: session)] != "terminal"
    }

    func isTranscriptChatCapable(_ session: SessionInfo) -> Bool {
        let type = session.type.lowercased()
        let provider = (session.provider ?? "").lowercased()
        return type == "claude" || type == "codex" || provider == "claude" || provider == "codex"
    }

    func viewModeKey(for session: SessionInfo) -> String {
        session.tmuxName
    }

    func showChatView(for session: SessionInfo) async throws {
        agentViewModeBySession.removeValue(forKey: viewModeKey(for: session))
        try await activateSession(session)
    }

    func showTerminalView(for session: SessionInfo) async {
        if session.isChat {
            await convertSession(session)
            return
        }
        guard isTranscriptChatCapable(session) else { return }
        agentViewModeBySession[viewModeKey(for: session)] = "terminal"
        do {
            try await activateSession(session)
        } catch {
            showTransientError("Failed to open terminal: \(error.localizedDescription)")
        }
    }

    func connect(host: String, token: String, name: String? = nil) async throws {
        let client = OrionClient(host: host, token: token)
        let projects = try await client.getProjects()
        self.client = client; self.host = host; self.token = token; self.projects = projects; self.isConnected = true; self.connectionError = nil
        suppressNextAutoConnect = false
        KeychainService.saveToken(token, for: host)
        var saved = KeychainService.loadConnections()
        let existingName = saved.first(where: { $0.host == host })?.name
        saved.removeAll { $0.host == host }
        let savedName = normalizedConnectionName(name) ?? normalizedConnectionName(existingName)
        saved.insert(SavedConnection(host: host, token: token, name: savedName), at: 0)
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
        suppressNextAutoConnect = true
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
        showWorkspaces = false
        showSettings = false
        showDiffReview = false
        activeWorkspacePath = nil
        activeTabId = nil
        selectedSessionByWorkspace = [:]
        agentViewModeBySession = [:]
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
            agentViewModeBySession = [:]
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

    func unifiedDiff(for file: GitChangedFile, workspacePath explicitWorkspacePath: String? = nil, base: String = "") async throws -> String {
        guard let client, let workspacePath = explicitWorkspacePath ?? activeWorkspacePath else { return "" }
        return try await client.getUnifiedDiff(workspacePath: workspacePath, base: base, filePath: file.path)
    }

    func gitStatus(workspacePath explicitWorkspacePath: String? = nil) async throws -> GitRepositoryStatus? {
        guard let client, let workspacePath = explicitWorkspacePath ?? activeWorkspacePath else { return nil }
        return try await client.getGitStatus(workspacePath: workspacePath)
    }

    func agentCompletions(provider: String, workspacePath: String) async -> [AgentCompletion] {
        guard let client else { return [] }
        do {
            return try await client.getAgentCompletions(provider: provider, workspacePath: workspacePath)
        } catch {
            return []
        }
    }

    func gitFetch(workspacePath: String) async throws -> GitActionResult {
        guard let client else { throw OrionError.invalidResponse }
        return try await client.gitFetch(workspacePath: workspacePath)
    }

    func gitPull(workspacePath: String) async throws -> GitActionResult {
        guard let client else { throw OrionError.invalidResponse }
        return try await client.gitPull(workspacePath: workspacePath)
    }

    func gitPush(workspacePath: String) async throws -> GitActionResult {
        guard let client else { throw OrionError.invalidResponse }
        return try await client.gitPush(workspacePath: workspacePath)
    }

    func refreshSessions() async {
        guard let client, let info = projectInfo, !workspaces.isEmpty else { return }
        do {
            var fetched = try await client.getSessions(repo: info.name, workspacePaths: workspaces.map(\.path))
            // Override with phone-launched session info (correct type/label)
            for i in fetched.indices {
                if let better = phoneLaunchedSessions[fetched[i].id] ?? phoneLaunchedSessions[fetched[i].tmuxName] {
                    fetched[i] = better.withStatus(fetched[i].status ?? better.status)
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
        let agent = agentTypes.first { $0.name == agentType }
        let provider = agentProvider(agent)
        let label = agent?.label ?? String(agentType.prefix(1)).uppercased() + agentType.dropFirst()
        let session = SessionInfo(
            tmuxName: resp.tmuxSession,
            type: provider ?? agentType,
            label: label,
            workspacePath: workspacePath,
            provider: provider,
            icon: agentIcon(agent),
            viewMode: "terminal",
            runtimeSessionId: resp.tmuxSession,
            model: agent?.model,
            reasoningEffort: agent?.reasoningEffort,
            approvalPolicy: agent?.approvalPolicy,
            sandboxMode: agent?.sandboxMode,
            permissionMode: agent?.permissionMode,
            collaborationMode: agent?.collaborationMode
        )
        phoneLaunchedSessions[resp.tmuxSession] = session
        await refreshSessions()
        if let refreshed = sessions.first(where: { $0.tmuxName == resp.tmuxSession }) {
            try await activateSession(refreshed)
        } else {
            try await activateSession(session)
        }
    }

    @discardableResult
    func launchPreferredAgent(workspacePath: String, agent: AgentType) async throws -> SessionInfo? {
        switch agentProvider(agent) {
        case "codex":
            return try await launchCodexChat(workspacePath: workspacePath, options: codexOptions(from: agent), icon: agentIcon(agent))
        case "claude":
            return try await launchClaudeChat(workspacePath: workspacePath, options: claudeOptions(from: agent), icon: agentIcon(agent))
        default:
            try await launchAgent(workspacePath: workspacePath, agentType: agent.name)
            return sessions.first { $0.workspacePath == workspacePath && $0.label == agent.label }
        }
    }

    @discardableResult
    func launchCodexChat(workspacePath: String, options: CodexLaunchOptions? = CodexLaunchOptions(), threadId: String? = nil, icon: String? = nil) async throws -> SessionInfo {
        guard let client, let root = selectedProject else { throw OrionError.invalidResponse }
        let resp = try await client.launchCodexChat(repoRoot: root, workspacePath: workspacePath, threadId: threadId, options: options, icon: icon)
        let session = SessionInfo(
            tmuxName: resp.id,
            type: resp.type,
            label: resp.label,
            workspacePath: resp.workspacePath,
            provider: resp.provider ?? "codex",
            icon: resp.icon ?? icon ?? "codex",
            viewMode: resp.viewMode ?? "chat",
            status: resp.status,
            runtimeSessionId: resp.runtimeSessionId ?? resp.id,
            threadId: resp.threadId,
            model: resp.model,
            reasoningEffort: resp.reasoningEffort,
            approvalPolicy: resp.approvalPolicy,
            sandboxMode: resp.sandboxMode,
            permissionMode: resp.permissionMode,
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
        try await launchCodexChat(workspacePath: workspacePath, options: nil, threadId: threadId)
    }

    @discardableResult
    func launchClaudeChat(workspacePath: String, options: ClaudeLaunchOptions? = nil, icon: String? = nil) async throws -> SessionInfo {
        guard let client, let root = selectedProject else { throw OrionError.invalidResponse }
        let resp = try await client.launchClaudeChat(repoRoot: root, workspacePath: workspacePath, options: options, icon: icon)
        let session = SessionInfo(
            tmuxName: resp.id,
            type: resp.type,
            label: resp.label,
            workspacePath: resp.workspacePath,
            provider: resp.provider ?? "claude",
            icon: resp.icon ?? icon ?? "claude",
            viewMode: resp.viewMode ?? "chat",
            status: resp.status,
            runtimeSessionId: resp.runtimeSessionId ?? resp.id,
            threadId: resp.threadId,
            model: resp.model,
            reasoningEffort: resp.reasoningEffort,
            approvalPolicy: resp.approvalPolicy,
            sandboxMode: resp.sandboxMode,
            permissionMode: resp.permissionMode,
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

    func convertSession(_ session: SessionInfo) async {
        guard let client, let root = selectedProject else { return }
        do {
            if session.isChat {
                let kind = session.type == "claude-chat" ? "claude" : "codex"
                let resp = try await client.convertChatToTerminal(
                    repoRoot: root,
                    workspacePath: session.workspacePath,
                    sessionId: session.chatConnectionId,
                    chatKind: kind
                )
                let label = kind == "claude" ? "Claude" : "Codex"
                let converted = SessionInfo(
                    tmuxName: resp.tmuxSession,
                    type: kind,
                    label: label,
                    workspacePath: session.workspacePath,
                    provider: kind,
                    icon: session.icon ?? kind,
                    viewMode: "terminal",
                    status: session.status,
                    runtimeSessionId: resp.tmuxSession,
                    threadId: session.threadId,
                    model: session.model,
                    reasoningEffort: session.reasoningEffort,
                    approvalPolicy: session.approvalPolicy,
                    sandboxMode: session.sandboxMode,
                    permissionMode: session.permissionMode,
                    collaborationMode: session.collaborationMode
                )
                sessions.removeAll { $0.id == session.id }
                phoneLaunchedSessions.removeValue(forKey: session.id)
                phoneLaunchedSessions[resp.tmuxSession] = converted
                agentViewModeBySession[viewModeKey(for: converted)] = "terminal"
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
                ? try await client.launchClaudeChat(repoRoot: root, workspacePath: session.workspacePath, tmuxSession: session.terminalTmuxSession, options: claudeOptions(from: session), icon: session.icon)
                : try await client.launchCodexChat(repoRoot: root, workspacePath: session.workspacePath, tmuxSession: session.terminalTmuxSession, options: codexOptions(from: session), icon: session.icon)
            let converted = SessionInfo(
                tmuxName: resp.id,
                type: resp.type,
                label: resp.label,
                workspacePath: resp.workspacePath,
                provider: session.type,
                icon: resp.icon ?? session.icon ?? session.type,
                viewMode: "chat",
                status: resp.status,
                runtimeSessionId: resp.runtimeSessionId ?? resp.id,
                threadId: resp.threadId,
                model: resp.model,
                reasoningEffort: resp.reasoningEffort,
                approvalPolicy: resp.approvalPolicy,
                sandboxMode: resp.sandboxMode,
                permissionMode: resp.permissionMode,
                collaborationMode: resp.collaborationMode
            )
            if activeConnection?.tmuxSession == session.tmuxName {
                disconnectActiveTerminal()
            }
            sessions.removeAll { $0.tmuxName == session.tmuxName }
            phoneLaunchedSessions.removeValue(forKey: session.tmuxName)
            phoneLaunchedSessions[converted.id] = converted
            agentViewModeBySession.removeValue(forKey: viewModeKey(for: session))
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
        showHome = false
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
        agentViewModeBySession.removeValue(forKey: viewModeKey(for: session))
        let remoteID = session.isChat ? session.chatConnectionId : session.terminalTmuxSession
        try? await client.killSession(tmuxSession: remoteID)
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
           activeSessionShowsChat {
            activeChatConnection.reconnectOrProbe(force: activeChatConnection.connectionState == .reconnecting)
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
            activeChatConnection?.reconnectOrProbe(force: activeChatConnection?.connectionState == .reconnecting)
            return
        }

        activationGeneration += 1
        let oldTerminal = activeConnection
        let oldChat = activeChatConnection
        let connection = CodexChatConnection(sessionId: session.chatConnectionId, sessionType: session.type, sessionIcon: session.icon, workspacePath: session.workspacePath)
        connection.onPermanentFailure = { [weak self] in
            self?.showTransientError("\(session.label) disconnected. Tap Reconnect to resume.")
        }
        connection.onAssistantVoiceText = { [weak self, weak connection] text in
            guard let self, let connection, self.activeChatConnection === connection else { return }
            self.handleVoiceText(text)
        }
        connection.onStatusChange = { [weak self] status in
            self?.updateSessionStatus(session, status: status)
        }

        activeConnection = nil
        activeChatConnection = connection
        oldTerminal?.disconnect()
        oldChat?.disconnect()
        connection.connect(host: host, token: token)
    }

    private func updateSessionStatus(_ session: SessionInfo, status: String) {
        let keys = Set([session.id, session.tmuxName, session.threadId ?? "", session.runtimeSessionId ?? "", session.chatConnectionId].filter { !$0.isEmpty })
        sessions = sessions.map { current in
            let currentKeys = Set([current.id, current.tmuxName, current.threadId ?? "", current.runtimeSessionId ?? "", current.chatConnectionId].filter { !$0.isEmpty })
            return !keys.isDisjoint(with: currentKeys) ? current.withStatus(status) : current
        }
        for key in keys where phoneLaunchedSessions[key] != nil {
            phoneLaunchedSessions[key] = phoneLaunchedSessions[key]?.withStatus(status)
        }
    }

    private func codexOptions(from session: SessionInfo) -> CodexLaunchOptions? {
        let hasMetadata = [session.model, session.reasoningEffort, session.approvalPolicy, session.sandboxMode, session.collaborationMode]
            .contains { value in
                guard let value else { return false }
                return !value.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            }
        guard hasMetadata else { return nil }
        return CodexLaunchOptions(
            model: session.model ?? CodexLaunchOptions().model,
            reasoningEffort: session.reasoningEffort ?? CodexLaunchOptions().reasoningEffort,
            approvalPolicy: session.approvalPolicy ?? CodexLaunchOptions().approvalPolicy,
            sandboxMode: session.sandboxMode ?? CodexLaunchOptions().sandboxMode,
            collaborationMode: session.collaborationMode ?? CodexLaunchOptions().collaborationMode
        )
    }

    private func codexOptions(from agent: AgentType) -> CodexLaunchOptions {
        CodexLaunchOptions(
            model: agent.model ?? CodexLaunchOptions().model,
            reasoningEffort: agent.reasoningEffort ?? CodexLaunchOptions().reasoningEffort,
            approvalPolicy: agent.approvalPolicy ?? CodexLaunchOptions().approvalPolicy,
            sandboxMode: agent.sandboxMode ?? CodexLaunchOptions().sandboxMode,
            collaborationMode: agent.collaborationMode ?? CodexLaunchOptions().collaborationMode
        )
    }

    private func claudeOptions(from session: SessionInfo) -> ClaudeLaunchOptions? {
        let hasMetadata = [session.model, session.reasoningEffort, session.approvalPolicy, session.sandboxMode, session.permissionMode]
            .contains { value in
                guard let value else { return false }
                return !value.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            }
        guard hasMetadata else { return nil }
        return ClaudeLaunchOptions(
            model: session.model,
            reasoningEffort: session.reasoningEffort,
            approvalPolicy: session.approvalPolicy,
            sandboxMode: session.sandboxMode,
            permissionMode: session.permissionMode
        )
    }

    private func claudeOptions(from agent: AgentType) -> ClaudeLaunchOptions {
        ClaudeLaunchOptions(
            model: agent.model,
            reasoningEffort: agent.reasoningEffort,
            approvalPolicy: agent.approvalPolicy,
            sandboxMode: agent.sandboxMode,
            permissionMode: agent.permissionMode
        )
    }

    private func agentProvider(_ agent: AgentType?) -> String? {
        let provider = (agent?.provider ?? agent?.name ?? "").lowercased()
        return provider == "claude" || provider == "codex" ? provider : nil
    }

    private func agentIcon(_ agent: AgentType?) -> String? {
        agent?.icon ?? agentProvider(agent)
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
        let workspaceSessions = sessions.filter { $0.workspacePath == workspacePath }
        guard !workspaceSessions.isEmpty else { return nil }
        if let selected = selectedSessionByWorkspace[workspacePath],
           let session = workspaceSessions.first(where: { $0.id == selected || $0.tmuxName == selected }) {
            return session
        }
        if let activeSession, activeSession.workspacePath == workspacePath,
           let session = workspaceSessions.first(where: { $0.id == activeSession.id || $0.tmuxName == activeSession.tmuxName }) {
            return session
        }
        return workspaceSessions.first { $0.type != "server" } ?? workspaceSessions.first
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

        if showsChat(preferred) {
            try? await activateSession(preferred, showSession: false)
            return
        }

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
        agentViewModeBySession.removeValue(forKey: tmuxSession)
        sessions.removeAll { $0.tmuxName == tmuxSession }

        Task {
            await ensureWorkspaceSelectionAttached()
        }
    }
}

private func normalizedConnectionName(_ name: String?) -> String? {
    guard let value = name?.trimmingCharacters(in: .whitespacesAndNewlines), !value.isEmpty else {
        return nil
    }
    return value
}
