import PhotosUI
import SwiftUI

struct ContentView: View {
    @Environment(AppState.self) private var state
    var body: some View { if state.isConnected { MainView() } else { ConnectionView() } }
}

struct MainView: View {
    @Environment(AppState.self) private var state
    var body: some View {
        VStack(spacing: 0) {
            HeaderBar()
            if !state.showHome && (state.activeWorkspace != nil || !state.visibleTabs.isEmpty) { TabStrip() }
            if state.isReconnecting {
                HStack(spacing: 6) {
                    ProgressView().controlSize(.mini).tint(OrionTheme.accentYellow)
                    Text("Reconnecting...").font(.caption).foregroundStyle(OrionTheme.accentYellow)
                }
                .frame(maxWidth: .infinity).padding(.vertical, 4)
                .background(OrionTheme.accentYellow.opacity(0.1))
            }
            if let error = state.transientError {
                HStack(spacing: 6) {
                    Image(systemName: "exclamationmark.triangle.fill").font(.caption)
                    Text(error).font(.caption).lineLimit(2)
                }
                .foregroundStyle(OrionTheme.accentRed)
                .frame(maxWidth: .infinity).padding(.vertical, 6).padding(.horizontal, 12)
                .background(OrionTheme.accentRed.opacity(0.15))
            }
            ZStack {
                OrionTheme.bgTerminal.ignoresSafeArea()
                if let activeSession = state.activeSession,
                   state.activeSessionShowsChat,
                   let connection = state.activeChatConnection,
                   connection.sessionId == activeSession.chatConnectionId {
                    CodexChatView(connection: connection)
                    if connection.connectionState == .failed {
                        VStack(spacing: 10) {
                            Image(systemName: "wifi.exclamationmark")
                                .font(.system(size: 26))
                                .foregroundStyle(OrionTheme.accentYellow)
                            Text("Connection lost")
                                .font(.headline)
                                .foregroundStyle(OrionTheme.textPrimary)
                            Button("Reconnect") {
                                connection.connect(host: state.host, token: state.token)
                            }
                            .buttonStyle(.borderedProminent)
                            .tint(OrionTheme.accentBlue)
                        }
                        .padding(16)
                        .background(OrionTheme.bgSecondary.opacity(0.94))
                        .clipShape(RoundedRectangle(cornerRadius: 8))
                    }
                } else if let activeSession = state.activeSession,
                   let connection = state.activeConnection,
                   connection.tmuxSession == activeSession.terminalTmuxSession {
                    TerminalContainerView(connection: connection)
                    if connection.connectionState == .failed {
                        VStack(spacing: 10) {
                            Image(systemName: "wifi.exclamationmark")
                                .font(.system(size: 26))
                                .foregroundStyle(OrionTheme.accentYellow)
                            Text("Connection lost")
                                .font(.headline)
                                .foregroundStyle(OrionTheme.textPrimary)
                            Button("Reconnect") {
                                connection.connect(host: state.host, token: state.token)
                            }
                            .buttonStyle(.borderedProminent)
                            .tint(OrionTheme.accentBlue)
                        }
                        .padding(16)
                        .background(OrionTheme.bgSecondary.opacity(0.94))
                        .clipShape(RoundedRectangle(cornerRadius: 8))
                    }
                } else if state.activeSession != nil {
                    ProgressView()
                        .controlSize(.regular)
                        .tint(OrionTheme.accentBlue)
                } else if let workspace = state.activeWorkspace {
                    WorkspaceEmptyView(workspace: workspace)
                } else {
                    WorkspaceEmptyView(workspace: nil)
                }
            }
            if !state.showHome && state.activeSession != nil && !state.activeSessionShowsChat { TerminalToolbar() }
        }
        .background(OrionTheme.bgPrimary)
        .sheet(isPresented: Binding(get: { state.showWorkspaces }, set: { state.showWorkspaces = $0 })) {
            WorkspaceSheet().presentationDetents([.height(420), .large]).presentationDragIndicator(.visible)
        }
        .sheet(isPresented: Binding(get: { state.showSettings }, set: { state.showSettings = $0 })) {
            SettingsView().presentationDetents([.medium]).presentationDragIndicator(.visible)
        }
        .sheet(isPresented: Binding(get: { state.showDiffReview }, set: { state.showDiffReview = $0 })) {
            DiffReviewSheet().presentationDetents([.large]).presentationDragIndicator(.visible)
        }
        .confirmationDialog(
            state.pendingKillSession.map { "Close \($0.label)?" } ?? "Close Session?",
            isPresented: Binding(
                get: { state.pendingKillSession != nil },
                set: { if !$0 { state.pendingKillSession = nil } }
            ),
            titleVisibility: .visible
        ) {
            if let session = state.pendingKillSession {
                Button("Kill Session", role: .destructive) {
                    state.pendingKillSession = nil
                    Task { await state.killSession(session) }
                }
            }
            Button("Keep Running", role: .cancel) {
                state.pendingKillSession = nil
            }
        } message: {
            if let session = state.pendingKillSession {
                Text("\(session.label) will be stopped and removed from this workspace.")
            }
        }
    }

}

// MARK: - Header with Project Switcher

struct HeaderBar: View {
    @Environment(AppState.self) private var state
    @State private var serverStatuses: [ServerStatus] = []
    @State private var isChangingServers = false
    @State private var diffStats: DiffStats?
    @State private var isLoadingDiffStats = false

    private var activeWorkspace: Workspace? { state.activeWorkspace }
    private var runningServers: [ServerStatus] { serverStatuses.filter(\.running) }
    private var serversRunning: Bool { !runningServers.isEmpty }

    var body: some View {
        HStack(spacing: 10) {
            Button {
                state.showWorkspaces = true
            } label: {
                Image(systemName: "sidebar.left")
                    .font(.system(size: 18))
                    .foregroundStyle(OrionTheme.textSecondary)
                    .frame(width: 32, height: 32)
            }
            .buttonStyle(.plain)

            projectSwitcher
                .frame(maxWidth: 148, alignment: .leading)

            Spacer(minLength: 4)

            Button {
                state.showDiffReview = true
            } label: {
                DiffHeaderPill(stats: diffStats, isLoading: isLoadingDiffStats)
            }
            .buttonStyle(.plain)

            if activeWorkspace != nil {
                Button {
                    Task { await toggleServers() }
                } label: {
                    ServerHeaderPill(
                        systemImage: serversRunning ? "stop.fill" : "play.fill",
                        tint: serversRunning ? OrionTheme.accentRed : OrionTheme.accentGreen,
                        isLoading: isChangingServers
                    )
                }
                .buttonStyle(.plain)
                .disabled(isChangingServers)
            }

            Button { state.showSettings = true } label: {
                Image(systemName: "gearshape")
                    .font(.system(size: 15))
                    .foregroundStyle(OrionTheme.textSecondary)
                    .frame(width: 30, height: 30)
            }
            .buttonStyle(.plain)
        }
        .padding(.horizontal, 12)
        .frame(height: 54)
        .background(OrionTheme.bgPrimary)
        .overlay(alignment: .bottom) { OrionTheme.border.frame(height: 0.5) }
        .task(id: state.activeWorkspacePath) {
            await reloadServerStatuses()
            await reloadDiffStats()
        }
        .onChange(of: state.sessions.count) { _, _ in
            Task { await reloadServerStatuses() }
        }
        .onChange(of: state.showDiffReview) { _, isShowing in
            guard !isShowing else { return }
            Task { await reloadDiffStats() }
        }
    }

    @ViewBuilder
    private var projectSwitcher: some View {
        if state.projects.count > 1 {
            Menu {
                ForEach(state.projects, id: \.self) { p in
                    Button {
                        Task { try? await state.selectProject(p) }
                    } label: {
                        Label((p as NSString).lastPathComponent,
                              systemImage: p == state.selectedProject ? "checkmark" : "folder")
                    }
                }
            } label: {
                titleView
            }
        } else {
            titleView
        }
    }

    @ViewBuilder
    private var titleView: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 4) {
                Text(state.projectInfo?.name ?? "Orion")
                    .font(.system(size: 16, weight: .semibold))
                    .foregroundStyle(OrionTheme.textPrimary)
                if state.projects.count > 1 {
                    Image(systemName: "chevron.up.chevron.down")
                        .font(.system(size: 9, weight: .medium))
                        .foregroundStyle(OrionTheme.textDim)
                }
            }
            if let workspace = state.activeWorkspace {
                Text(workspaceSubtitle(workspace))
                    .font(.system(size: 11, design: .monospaced))
                    .foregroundStyle(OrionTheme.textDim)
                    .lineLimit(1)
            }
        }
    }

    private func workspaceSubtitle(_ workspace: Workspace) -> String {
        if workspace.isMain {
            return "Main workspace"
        }
        if !workspace.branch.isEmpty {
            return workspace.branch
        }
        return workspace.name
    }

    private func reloadServerStatuses() async {
        guard let workspace = activeWorkspace else {
            serverStatuses = []
            return
        }
        serverStatuses = await state.getServerStatuses(workspace: workspace)
    }

    private func reloadDiffStats() async {
        guard let workspace = activeWorkspace else {
            diffStats = nil
            return
        }
        let workspacePath = workspace.path
        isLoadingDiffStats = true
        defer { isLoadingDiffStats = false }

        do {
            let files = try await state.changedFiles(workspacePath: workspacePath)
            var totals = DiffStats()
            for file in files {
                let diff = try await state.unifiedDiff(for: file, workspacePath: workspacePath)
                totals.add(countDiffChanges(diff))
            }
            guard state.activeWorkspacePath == workspacePath else { return }
            diffStats = totals.isEmpty ? nil : totals
        } catch {
            guard state.activeWorkspacePath == workspacePath else { return }
            diffStats = nil
        }
    }

    private func toggleServers() async {
        guard let workspace = activeWorkspace else { return }
        isChangingServers = true
        if serversRunning {
            await state.stopServers(workspace: workspace)
        } else {
            await state.startServers(workspace: workspace)
        }
        serverStatuses = await state.getServerStatuses(workspace: workspace)
        isChangingServers = false
    }
}

private struct DiffStats: Equatable {
    var added = 0
    var removed = 0

    var isEmpty: Bool { added == 0 && removed == 0 }

    mutating func add(_ other: DiffStats) {
        added += other.added
        removed += other.removed
    }
}

private struct DiffHeaderPill: View {
    let stats: DiffStats?
    var isLoading = false

    var body: some View {
        HStack(spacing: 6) {
            if isLoading {
                ProgressView()
                    .controlSize(.mini)
                    .tint(OrionTheme.accentBlue)
            } else if let stats {
                Text("+\(stats.added)")
                    .foregroundStyle(OrionTheme.accentGreen)
                Text("-\(stats.removed)")
                    .foregroundStyle(OrionTheme.accentRed)
            } else {
                Text("Diff")
                    .foregroundStyle(OrionTheme.textSecondary)
            }
        }
        .font(.system(size: 11, weight: .semibold, design: stats == nil ? .default : .monospaced))
        .padding(.horizontal, 9)
        .frame(height: 28)
        .background(OrionTheme.bgSurface)
        .clipShape(Capsule())
        .overlay(Capsule().stroke(OrionTheme.borderDim, lineWidth: 0.7))
    }
}

private struct ServerHeaderPill: View {
    let systemImage: String
    let tint: Color
    var isLoading = false

    var body: some View {
        HStack(spacing: 0) {
            if isLoading {
                ProgressView()
                    .controlSize(.mini)
                    .tint(tint)
            } else {
                Image(systemName: systemImage)
                    .font(.system(size: 10, weight: .bold))
                    .foregroundStyle(tint)
            }
        }
        .frame(width: 28)
        .frame(height: 28)
        .background(OrionTheme.bgSurface)
        .clipShape(Capsule())
        .overlay(Capsule().stroke(OrionTheme.borderDim, lineWidth: 0.7))
    }
}

// MARK: - Mobile Home

private struct WorkspaceEmptyView: View {
    @Environment(AppState.self) private var state
    let workspace: Workspace?

    var body: some View {
        VStack(spacing: 14) {
            OrionMarkView(size: 44)
            VStack(spacing: 5) {
                Text(workspace?.name ?? "No workspace selected")
                    .font(.system(size: 24, weight: .semibold))
                    .foregroundStyle(OrionTheme.textPrimary)
                    .lineLimit(1)
                    .minimumScaleFactor(0.75)
                Text(subtitle)
                    .font(.system(size: 13, design: .monospaced))
                    .foregroundStyle(OrionTheme.textDim)
                    .multilineTextAlignment(.center)
                    .lineLimit(2)
            }
            if let workspace {
                NewSessionMenu(workspace: workspace, style: .prominent)
            } else {
                Button { state.showWorkspaces = true } label: {
                    Label("Choose workspace", systemImage: "sidebar.left")
                        .font(.system(size: 14, weight: .semibold))
                        .padding(.horizontal, 16)
                        .frame(height: 38)
                }
                .buttonStyle(.borderedProminent)
                .tint(OrionTheme.accentBlue)
            }
        }
        .padding(.horizontal, 28)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private var subtitle: String {
        guard let workspace else { return "Open a workspace to start a session." }
        if workspace.isMain { return "Main workspace" }
        return workspace.branch.isEmpty ? workspace.name : workspace.branch
    }
}

private struct MobileHomeView: View {
    @Environment(AppState.self) private var state
    @State private var serverStatusesByWorkspace: [String: [ServerStatus]] = [:]
    @State private var changedFiles: [GitChangedFile] = []
    @State private var loadingHome = false
    @State private var showQuickAsk = false
    @State private var detailWorkspace: Workspace?

    private var projectSubtitle: String {
        let projectCount = state.workspaces.count
        let projectRoot = state.projectInfo?.root ?? state.selectedProject ?? ""
        let folder = (projectRoot as NSString).lastPathComponent
        if folder.isEmpty { return "\(projectCount) workspace\(projectCount == 1 ? "" : "s")" }
        return "\(folder) · \(projectCount) workspace\(projectCount == 1 ? "" : "s")"
    }

    private var featuredSession: SessionInfo? {
        if let active = state.activeSession, active.isChat { return active }
        if let activeWorkspace = state.activeWorkspacePath,
           let chat = state.sessions.first(where: { $0.workspacePath == activeWorkspace && $0.isChat }) {
            return chat
        }
        return state.sessions.first(where: \.isChat)
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                homeHero
                readyCard
                workspaceList
            }
            .padding(.horizontal, 16)
            .padding(.top, 18)
            .padding(.bottom, 96)
        }
        .background(OrionTheme.bgTerminal)
        .refreshable { await loadHome() }
        .safeAreaInset(edge: .bottom) {
            quickAskBar
        }
        .task { await loadHome() }
        .onChange(of: state.activeWorkspacePath) { _, _ in
            Task { await loadHome() }
        }
        .sheet(isPresented: $showQuickAsk) {
            QuickAskSheet(
                defaultWorkspacePath: state.activeWorkspacePath,
                onCancel: { showQuickAsk = false },
                onDone: {
                    showQuickAsk = false
                    Task { await loadHome() }
                }
            )
            .presentationDetents([.large])
            .presentationDragIndicator(.visible)
        }
        .sheet(item: $detailWorkspace) { workspace in
            WorkspaceDetailSheet(
                workspace: workspace,
                onClose: { detailWorkspace = nil },
                onRefresh: { await loadHome() }
            )
            .presentationDetents([.large])
            .presentationDragIndicator(.visible)
        }
    }

    private var homeHero: some View {
        VStack(alignment: .leading, spacing: 15) {
            HStack(spacing: 6) {
                Circle()
                    .fill(state.isConnected ? OrionTheme.accentGreen : OrionTheme.accentRed)
                    .frame(width: 7, height: 7)
                Text(state.isConnected ? "Connected" : "Offline")
                    .font(.system(size: 13))
                    .foregroundStyle(OrionTheme.textDim)
                Spacer()
                Button {
                    Task { await loadHome() }
                } label: {
                    if loadingHome {
                        ProgressView().controlSize(.mini).tint(OrionTheme.textSecondary)
                    } else {
                        Image(systemName: "arrow.clockwise")
                            .font(.system(size: 13, weight: .medium))
                            .foregroundStyle(OrionTheme.textDim)
                    }
                }
                .buttonStyle(.plain)
            }

            HStack(alignment: .center, spacing: 14) {
                OrionMarkView(size: 46)
                VStack(alignment: .leading, spacing: 3) {
                    Text("orion")
                        .font(.system(size: 31, weight: .bold))
                        .foregroundStyle(OrionTheme.textPrimary)
                    Text(projectSubtitle)
                        .font(.system(size: 14, design: .monospaced))
                        .foregroundStyle(OrionTheme.textDim)
                        .lineLimit(1)
                }
                Spacer()
                Menu {
                    Button { state.showWorkspaces = true } label: {
                        Label("Workspaces", systemImage: "sidebar.left")
                    }
                    Button { state.showDiffReview = true } label: {
                        Label("Review diff", systemImage: "doc.text.magnifyingglass")
                    }
                    Button { showQuickAsk = true } label: {
                        Label("Ask agent", systemImage: "paperplane")
                    }
                } label: {
                    Image(systemName: "ellipsis")
                        .font(.system(size: 16, weight: .bold))
                        .foregroundStyle(OrionTheme.textSecondary)
                        .frame(width: 42, height: 42)
                        .background(OrionTheme.bgSurface)
                        .clipShape(Circle())
                }
                .buttonStyle(.plain)
            }
        }
    }

    private var readyCard: some View {
        let session = featuredSession
        let label = session?.label ?? "Codex"
        return HStack(spacing: 12) {
            AgentSigilView(session?.type ?? "codex-chat", size: 38)
            VStack(alignment: .leading, spacing: 4) {
                Text("\(label) is ready when you are")
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundStyle(OrionTheme.textPrimary)
                    .lineLimit(2)
                    .minimumScaleFactor(0.86)
                Text(readyCardSubtitle)
                    .font(.system(size: 12, design: .monospaced))
                    .foregroundStyle(OrionTheme.textDim)
                    .lineLimit(2)
                    .minimumScaleFactor(0.86)
            }
            Spacer(minLength: 8)
            Button {
                if let session {
                    Task { try? await state.activateSession(session) }
                } else if changedFiles.isEmpty {
                    showQuickAsk = true
                } else {
                    state.showDiffReview = true
                }
            } label: {
                Text(session == nil && changedFiles.isEmpty ? "Ask" : "Review")
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundStyle(Color(hex: 0x18233A))
                    .padding(.horizontal, 17)
                    .frame(height: 34)
                    .background(OrionTheme.accentBlue)
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
            }
            .buttonStyle(.plain)
        }
        .padding(14)
        .background(OrionTheme.bgSurface)
        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 8, style: .continuous).stroke(OrionTheme.borderDim, lineWidth: 0.8))
    }

    private var readyCardSubtitle: String {
        if !changedFiles.isEmpty {
            return "\(changedFiles.count) changed file\(changedFiles.count == 1 ? "" : "s") in \(state.activeWorkspace?.name ?? "workspace")"
        }
        if let session = featuredSession {
            return "\(sessionLabel(session.type)) · \(workspaceName(for: session.workspacePath))"
        }
        return "Start a plan, chat, or shell"
    }

    private var workspaceList: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("Workspaces")
                    .font(.system(size: 13, weight: .bold))
                    .foregroundStyle(OrionTheme.textDim)
                Spacer()
                Text("\(state.workspaces.count)")
                    .font(.system(size: 13, design: .monospaced))
                    .foregroundStyle(OrionTheme.textDim)
            }
            ForEach(state.workspaces) { workspace in
                MobileWorkspaceCard(
                    workspace: workspace,
                    sessions: state.sessions.filter { $0.workspacePath == workspace.path && $0.type != "server" },
                    servers: serverStatusesByWorkspace[workspace.path] ?? [],
                    onOpenDetail: { detailWorkspace = workspace },
                    onRefresh: { await loadHome() }
                )
            }
        }
    }

    private var quickAskBar: some View {
        Button { showQuickAsk = true } label: {
            HStack(spacing: 10) {
                Text("Ask an agent...")
                    .font(.system(size: 15))
                    .foregroundStyle(OrionTheme.textDim)
                    .frame(maxWidth: .infinity, alignment: .leading)
                Image(systemName: "mic")
                    .font(.system(size: 13, weight: .medium))
                    .foregroundStyle(OrionTheme.textDim)
                Image(systemName: "arrow.up")
                    .font(.system(size: 16, weight: .bold))
                    .foregroundStyle(Color(hex: 0x18233A))
                    .frame(width: 42, height: 42)
                    .background(OrionTheme.accentBlue)
                    .clipShape(Circle())
            }
            .padding(.leading, 16)
            .padding(.trailing, 6)
            .frame(height: 58)
            .background(OrionTheme.bgSurface)
            .clipShape(Capsule())
            .overlay(Capsule().stroke(OrionTheme.borderDim, lineWidth: 0.8))
            .padding(.horizontal, 16)
            .padding(.top, 8)
            .padding(.bottom, 8)
            .background(OrionTheme.bgTerminal.opacity(0.96))
        }
        .buttonStyle(.plain)
    }

    private func loadHome() async {
        loadingHome = true
        defer { loadingHome = false }

        await state.refreshSessions()
        var statuses: [String: [ServerStatus]] = [:]
        for workspace in state.workspaces {
            statuses[workspace.path] = await state.getServerStatuses(workspace: workspace)
        }
        serverStatusesByWorkspace = statuses

        do {
            changedFiles = try await state.changedFiles()
        } catch {
            changedFiles = []
        }
    }

    private func workspaceName(for path: String) -> String {
        state.workspaces.first(where: { $0.path == path })?.name ?? "workspace"
    }
}

private struct MobileWorkspaceCard: View {
    @Environment(AppState.self) private var state
    let workspace: Workspace
    let sessions: [SessionInfo]
    let servers: [ServerStatus]
    let onOpenDetail: () -> Void
    let onRefresh: () async -> Void
    @State private var showingCodexOptions = false
    @State private var codexOptions = CodexLaunchOptions()
    @State private var serverBusy = false

    private var isActiveWorkspace: Bool { state.activeWorkspacePath == workspace.path }
    private var activeSessionInWorkspace: Bool { state.activeSession?.workspacePath == workspace.path }
    private var runningServers: [ServerStatus] { servers.filter(\.running) }

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Button {
                onOpenDetail()
            } label: {
                HStack(alignment: .center, spacing: 10) {
                    Circle()
                        .fill(statusDotColor)
                        .frame(width: 9, height: 9)
                        .shadow(color: statusDotColor.opacity(isActiveWorkspace ? 0.55 : 0), radius: 5)
                    VStack(alignment: .leading, spacing: 3) {
                        HStack(spacing: 7) {
                            Text(workspace.name)
                                .font(.system(size: 17, weight: .semibold))
                                .foregroundStyle(OrionTheme.textPrimary)
                                .lineLimit(1)
                            if workspace.isMain {
                                Text("MAIN")
                                    .font(.system(size: 9, weight: .bold, design: .monospaced))
                                    .foregroundStyle(OrionTheme.accentBlue)
                                    .padding(.horizontal, 6)
                                    .padding(.vertical, 2)
                                    .background(OrionTheme.accentBlue.opacity(0.15))
                                    .clipShape(RoundedRectangle(cornerRadius: 4, style: .continuous))
                            }
                        }
                        Text(workspace.branch.isEmpty ? workspace.name : workspace.branch)
                            .font(.system(size: 12, design: .monospaced))
                            .foregroundStyle(OrionTheme.textDim)
                            .lineLimit(1)
                    }
                    Spacer()
                    Text(statusText)
                        .font(.system(size: 12, weight: .medium))
                        .foregroundStyle(statusDotColor)
                }
            }
            .buttonStyle(.plain)

            if sessions.isEmpty {
                HStack(spacing: 9) {
                    AgentSigilView("codex-chat", size: 25)
                    Text("No live sessions")
                        .font(.system(size: 13))
                        .foregroundStyle(OrionTheme.textDim)
                    Spacer()
                }
            } else {
                VStack(spacing: 8) {
                    ForEach(Array(sessions.prefix(3))) { session in
                        Button {
                            Task {
                                do { try await state.activateSession(session) }
                                catch { state.showTransientError("Failed to open session: \(error.localizedDescription)") }
                            }
                        } label: {
                            HomeSessionRow(
                                session: session,
                                isActive: state.activeSession?.id == session.id
                            )
                        }
                        .buttonStyle(.plain)
                    }
                    if sessions.count > 3 {
                        Text("+ \(sessions.count - 3) more session\(sessions.count - 3 == 1 ? "" : "s")")
                            .font(.system(size: 11, design: .monospaced))
                            .foregroundStyle(OrionTheme.textDim)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                }
            }

            if !runningServers.isEmpty {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 8) {
                        ForEach(runningServers) { server in
                            HStack(spacing: 5) {
                                Circle().fill(OrionTheme.accentGreen).frame(width: 6, height: 6)
                                Text("\(server.name):\(server.port)")
                                    .font(.system(size: 11, design: .monospaced))
                                    .foregroundStyle(OrionTheme.textDim)
                            }
                        }
                    }
                }
            }

            HStack(spacing: 10) {
                Menu {
                    Button {
                        Task {
                            do {
                                try await state.launchShell(workspacePath: workspace.path)
                                await onRefresh()
                            } catch {
                                state.showTransientError("Failed to start shell: \(error.localizedDescription)")
                            }
                        }
                    } label: {
                        Label("Shell", systemImage: "terminal")
                    }
                    Button { showingCodexOptions = true } label: {
                        Label("Codex Chat", systemImage: "bubble.left.and.bubble.right")
                    }
                    Button {
                        Task {
                            do {
                                _ = try await state.launchClaudeChat(workspacePath: workspace.path)
                                await onRefresh()
                            } catch {
                                state.showTransientError("Failed to start Claude: \(error.localizedDescription)")
                            }
                        }
                    } label: {
                        Label("Claude Chat", systemImage: "bubble.left.and.bubble.right.fill")
                    }
                    if !state.agentTypes.isEmpty {
                        Divider()
                        ForEach(state.agentTypes) { agent in
                            Button {
                                Task {
                                    do {
                                        try await state.launchAgent(workspacePath: workspace.path, agentType: agent.name)
                                        await onRefresh()
                                    } catch {
                                        state.showTransientError("Failed to start \(agent.label): \(error.localizedDescription)")
                                    }
                                }
                            } label: {
                                Label(agent.label, systemImage: agentIcon(agent.name))
                            }
                        }
                    }
                } label: {
                    Label("New", systemImage: "plus")
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundStyle(OrionTheme.textPrimary)
                        .padding(.horizontal, 11)
                        .frame(height: 32)
                        .background(OrionTheme.bgActive)
                        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                }
                .buttonStyle(.plain)

                Spacer()

                if !servers.isEmpty {
                    Button {
                        Task {
                            serverBusy = true
                            if runningServers.isEmpty {
                                await state.startServers(workspace: workspace)
                            } else {
                                await state.stopServers(workspace: workspace)
                            }
                            await onRefresh()
                            serverBusy = false
                        }
                    } label: {
                        if serverBusy {
                            ProgressView().controlSize(.mini).tint(OrionTheme.textSecondary)
                        } else {
                            Label(runningServers.isEmpty ? "Servers" : "Stop", systemImage: runningServers.isEmpty ? "play.fill" : "stop.fill")
                                .font(.system(size: 12, weight: .semibold))
                                .foregroundStyle(runningServers.isEmpty ? OrionTheme.accentGreen : OrionTheme.accentRed)
                        }
                    }
                    .buttonStyle(.plain)
                    .disabled(serverBusy)
                }
            }
        }
        .padding(14)
        .background(isActiveWorkspace ? OrionTheme.bgSurface : OrionTheme.bgSecondary)
        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 8, style: .continuous)
                .stroke(isActiveWorkspace ? OrionTheme.accentBlue.opacity(0.22) : OrionTheme.borderDim, lineWidth: 0.8)
        )
        .sheet(isPresented: $showingCodexOptions) {
            CodexLaunchOptionsSheet(
                workspaceName: workspace.branch.isEmpty ? workspace.name : workspace.branch,
                options: $codexOptions,
                onCancel: { showingCodexOptions = false },
                onLaunch: {
                    let selected = codexOptions
                    showingCodexOptions = false
                    Task {
                        do {
                            _ = try await state.launchCodexChat(workspacePath: workspace.path, options: selected)
                            await onRefresh()
                        } catch {
                            state.showTransientError("Failed to start Codex: \(error.localizedDescription)")
                        }
                    }
                }
            )
            .presentationDetents([.medium])
            .presentationDragIndicator(.visible)
        }
    }

    private var statusText: String {
        if activeSessionInWorkspace { return "Open" }
        if !sessions.isEmpty { return "\(sessions.count) session\(sessions.count == 1 ? "" : "s")" }
        if !runningServers.isEmpty { return "Servers" }
        if isActiveWorkspace { return "Selected" }
        return "Idle"
    }

    private var statusDotColor: Color {
        if activeSessionInWorkspace { return OrionTheme.accentBlue }
        if !sessions.isEmpty || !runningServers.isEmpty { return OrionTheme.accentGreen }
        if isActiveWorkspace { return OrionTheme.accentBlue }
        return OrionTheme.border
    }
}

private struct HomeSessionRow: View {
    let session: SessionInfo
    let isActive: Bool

    var body: some View {
        HStack(spacing: 9) {
            AgentSigilView(session.type, size: 25)
            VStack(alignment: .leading, spacing: 2) {
                Text(session.label)
                    .font(.system(size: 13.5, weight: .medium))
                    .foregroundStyle(OrionTheme.textSecondary)
                    .lineLimit(1)
                HStack(spacing: 6) {
                    Text(sessionLabel(session.type))
                    if let model = session.model, !model.isEmpty {
                        Text(modelLabel(model))
                    }
                    if let reasoning = session.reasoningEffort, !reasoning.isEmpty {
                        Text(reasoningLabel(reasoning))
                    }
                }
                .font(.system(size: 10.5, design: .monospaced))
                .foregroundStyle(OrionTheme.textDim)
                .lineLimit(1)
            }
            Spacer()
            Circle()
                .fill(isActive ? OrionTheme.accentBlue : OrionTheme.textDim)
                .frame(width: 7, height: 7)
                .opacity(isActive ? 1 : 0.7)
        }
    }
}

private struct QuickAskSheet: View {
    @Environment(AppState.self) private var state
    let onCancel: () -> Void
    let onDone: () -> Void
    @State private var workspacePath: String
    @State private var provider = "codex-chat"
    @State private var prompt = ""
    @State private var codexOptions = CodexLaunchOptions()
    @State private var launching = false

    init(defaultWorkspacePath: String?, onCancel: @escaping () -> Void, onDone: @escaping () -> Void) {
        self.onCancel = onCancel
        self.onDone = onDone
        _workspacePath = State(initialValue: defaultWorkspacePath ?? "")
    }

    private var canLaunch: Bool {
        !workspacePath.isEmpty && !prompt.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && !launching
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("Workspace") {
                    Picker("Workspace", selection: $workspacePath) {
                        ForEach(state.workspaces) { workspace in
                            Text(workspace.name).tag(workspace.path)
                        }
                    }
                }

                Section("Agent") {
                    Picker("Agent", selection: $provider) {
                        Text("Codex Chat").tag("codex-chat")
                        Text("Claude Chat").tag("claude-chat")
                    }
                    .pickerStyle(.segmented)
                    if provider == "codex-chat" {
                        Picker("Model", selection: $codexOptions.model) {
                            ForEach(["gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex", "gpt-5.3-codex-spark", "gpt-5.2"], id: \.self) {
                                Text(modelLabel($0)).tag($0)
                            }
                        }
                        Picker("Reasoning", selection: $codexOptions.reasoningEffort) {
                            ForEach(["low", "medium", "high", "xhigh"], id: \.self) {
                                Text(reasoningLabel($0)).tag($0)
                            }
                        }
                    }
                }

                Section("Prompt") {
                    TextEditor(text: $prompt)
                        .frame(minHeight: 170)
                        .font(.system(size: 15))
                }
            }
            .scrollContentBackground(.hidden)
            .background(OrionTheme.bgPrimary)
            .navigationTitle("Ask agent")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Cancel", action: onCancel)
                        .foregroundStyle(OrionTheme.accentBlue)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        Task { await launch() }
                    } label: {
                        if launching {
                            ProgressView().controlSize(.mini)
                        } else {
                            Text("Start").fontWeight(.semibold)
                        }
                    }
                    .disabled(!canLaunch)
                    .foregroundStyle(canLaunch ? OrionTheme.accentBlue : OrionTheme.textDim)
                }
            }
            .toolbarBackground(OrionTheme.bgSecondary, for: .navigationBar)
            .onAppear {
                if workspacePath.isEmpty {
                    workspacePath = state.activeWorkspacePath ?? state.workspaces.first?.path ?? ""
                }
            }
        }
    }

    private func launch() async {
        guard canLaunch else { return }
        launching = true
        do {
            _ = try await state.launchChatWithPrompt(
                workspacePath: workspacePath,
                provider: provider,
                prompt: prompt,
                codexOptions: codexOptions
            )
            launching = false
            onDone()
        } catch {
            launching = false
            state.showTransientError("Failed to start chat: \(error.localizedDescription)")
        }
    }
}

private struct WorkspaceDetailSheet: View {
    @Environment(AppState.self) private var state
    let workspace: Workspace
    let onClose: () -> Void
    let onRefresh: () async -> Void
    @State private var serverStatuses: [ServerStatus] = []
    @State private var history: [CodexHistoryThread] = []
    @State private var changedFiles: [GitChangedFile] = []
    @State private var loading = false
    @State private var serverBusy = false
    @State private var showingCodexOptions = false
    @State private var codexOptions = CodexLaunchOptions()

    private var sessions: [SessionInfo] {
        state.sessions.filter { $0.workspacePath == workspace.path && $0.type != "server" }
    }

    private var runningServers: [ServerStatus] {
        serverStatuses.filter(\.running)
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    header
                    summaryGrid
                    primaryActions
                    liveSessionsSection
                    recentHistorySection
                }
                .padding(16)
                .padding(.bottom, 28)
            }
            .background(OrionTheme.bgPrimary)
            .navigationTitle("Workspace")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Close", action: onClose)
                        .foregroundStyle(OrionTheme.accentBlue)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button { Task { await load() } } label: {
                        if loading {
                            ProgressView().controlSize(.mini)
                        } else {
                            Image(systemName: "arrow.clockwise")
                        }
                    }
                    .foregroundStyle(OrionTheme.accentBlue)
                }
            }
            .toolbarBackground(OrionTheme.bgSecondary, for: .navigationBar)
            .task { await load() }
            .sheet(isPresented: $showingCodexOptions) {
                CodexLaunchOptionsSheet(
                    workspaceName: workspace.branch.isEmpty ? workspace.name : workspace.branch,
                    options: $codexOptions,
                    onCancel: { showingCodexOptions = false },
                    onLaunch: {
                        let selected = codexOptions
                        showingCodexOptions = false
                        Task {
                            do {
                                _ = try await state.launchCodexChat(workspacePath: workspace.path, options: selected)
                                await onRefresh()
                                onClose()
                            } catch {
                                state.showTransientError("Failed to start Codex: \(error.localizedDescription)")
                            }
                        }
                    }
                )
                .presentationDetents([.medium])
                .presentationDragIndicator(.visible)
            }
        }
    }

    private var header: some View {
        HStack(alignment: .center, spacing: 14) {
            OrionMarkView(size: 46)
            VStack(alignment: .leading, spacing: 5) {
                HStack(spacing: 7) {
                    Text(workspace.name)
                        .font(.system(size: 25, weight: .bold))
                        .foregroundStyle(OrionTheme.textPrimary)
                        .lineLimit(1)
                    if workspace.isMain {
                        Text("MAIN")
                            .font(.system(size: 10, weight: .bold, design: .monospaced))
                            .foregroundStyle(OrionTheme.accentBlue)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(OrionTheme.accentBlue.opacity(0.15))
                            .clipShape(RoundedRectangle(cornerRadius: 4, style: .continuous))
                    }
                }
                Text(workspace.branch.isEmpty ? workspace.name : workspace.branch)
                    .font(.system(size: 13, design: .monospaced))
                    .foregroundStyle(OrionTheme.textDim)
                    .lineLimit(1)
            }
            Spacer()
        }
    }

    private var summaryGrid: some View {
        HStack(spacing: 10) {
            WorkspaceSummaryTile(value: "\(sessions.count)", label: "sessions", tint: sessions.isEmpty ? OrionTheme.textDim : OrionTheme.accentBlue)
            WorkspaceSummaryTile(value: "\(runningServers.count)", label: "servers", tint: runningServers.isEmpty ? OrionTheme.textDim : OrionTheme.accentGreen)
            WorkspaceSummaryTile(value: "\(changedFiles.count)", label: "files", tint: changedFiles.isEmpty ? OrionTheme.textDim : OrionTheme.accentYellow)
        }
    }

    private var primaryActions: some View {
        VStack(spacing: 10) {
            HStack(spacing: 10) {
                Button {
                    if let preferred = sessions.first {
                        Task {
                            do {
                                try await state.activateSession(preferred)
                                onClose()
                            } catch {
                                state.showTransientError("Failed to open session: \(error.localizedDescription)")
                            }
                        }
                    } else {
                        showingCodexOptions = true
                    }
                } label: {
                    Label(sessions.isEmpty ? "Start Codex" : "Open latest", systemImage: sessions.isEmpty ? "plus" : "arrow.up.forward")
                        .font(.system(size: 13, weight: .semibold))
                        .frame(maxWidth: .infinity)
                        .frame(height: 38)
                }
                .buttonStyle(.borderedProminent)
                .tint(OrionTheme.accentBlue)

                Button {
                    Task {
                        await state.activateWorkspace(workspace.path)
                        state.showDiffReview = true
                        onClose()
                    }
                } label: {
                    Label("Diff", systemImage: "doc.text.magnifyingglass")
                        .font(.system(size: 13, weight: .semibold))
                        .frame(maxWidth: .infinity)
                        .frame(height: 38)
                }
                .buttonStyle(.bordered)
                .tint(OrionTheme.accentBlue)
            }

            HStack(spacing: 10) {
                Menu {
                    Button { showingCodexOptions = true } label: {
                        Label("Codex Chat", systemImage: "bubble.left.and.bubble.right")
                    }
                    Button {
                        Task {
                            do {
                                _ = try await state.launchClaudeChat(workspacePath: workspace.path)
                                await onRefresh()
                                onClose()
                            } catch {
                                state.showTransientError("Failed to start Claude: \(error.localizedDescription)")
                            }
                        }
                    } label: {
                        Label("Claude Chat", systemImage: "bubble.left.and.bubble.right.fill")
                    }
                    Button {
                        Task {
                            do {
                                try await state.launchShell(workspacePath: workspace.path)
                                await onRefresh()
                                onClose()
                            } catch {
                                state.showTransientError("Failed to start shell: \(error.localizedDescription)")
                            }
                        }
                    } label: {
                        Label("Shell", systemImage: "terminal")
                    }
                    if !state.agentTypes.isEmpty {
                        Divider()
                        ForEach(state.agentTypes) { agent in
                            Button {
                                Task {
                                    do {
                                        try await state.launchAgent(workspacePath: workspace.path, agentType: agent.name)
                                        await onRefresh()
                                        onClose()
                                    } catch {
                                        state.showTransientError("Failed to start \(agent.label): \(error.localizedDescription)")
                                    }
                                }
                            } label: {
                                Label(agent.label, systemImage: agentIcon(agent.name))
                            }
                        }
                    }
                } label: {
                    Label("New session", systemImage: "plus")
                        .font(.system(size: 13, weight: .semibold))
                        .frame(maxWidth: .infinity)
                        .frame(height: 36)
                }
                .buttonStyle(.bordered)
                .tint(OrionTheme.textSecondary)

                if !serverStatuses.isEmpty {
                    Button {
                        Task {
                            serverBusy = true
                            if runningServers.isEmpty {
                                await state.startServers(workspace: workspace)
                            } else {
                                await state.stopServers(workspace: workspace)
                            }
                            await load()
                            await onRefresh()
                            serverBusy = false
                        }
                    } label: {
                        if serverBusy {
                            ProgressView().controlSize(.mini)
                                .frame(maxWidth: .infinity)
                                .frame(height: 36)
                        } else {
                            Label(runningServers.isEmpty ? "Start servers" : "Stop servers", systemImage: runningServers.isEmpty ? "play.fill" : "stop.fill")
                                .font(.system(size: 13, weight: .semibold))
                                .frame(maxWidth: .infinity)
                                .frame(height: 36)
                        }
                    }
                    .buttonStyle(.bordered)
                    .tint(runningServers.isEmpty ? OrionTheme.accentGreen : OrionTheme.accentRed)
                    .disabled(serverBusy)
                }
            }
        }
    }

    private var liveSessionsSection: some View {
        VStack(alignment: .leading, spacing: 10) {
            sectionTitle("Live sessions", count: sessions.count)
            if sessions.isEmpty {
                EmptyDetailRow(icon: "codex-chat", title: "No live sessions", subtitle: "Start or resume an agent from this workspace.")
            } else {
                ForEach(Array(sessions.prefix(3))) { session in
                    Button {
                        Task {
                            do {
                                try await state.activateSession(session)
                                onClose()
                            } catch {
                                state.showTransientError("Failed to open session: \(error.localizedDescription)")
                            }
                        }
                    } label: {
                        DetailSessionRow(session: session, isActive: state.activeSession?.id == session.id)
                    }
                    .buttonStyle(.plain)
                }
                if sessions.count > 3 {
                    Text("+ \(sessions.count - 3) more session\(sessions.count - 3 == 1 ? "" : "s")")
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(OrionTheme.textDim)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(.leading, 2)
                }
            }
        }
    }

    private var recentHistorySection: some View {
        VStack(alignment: .leading, spacing: 10) {
            sectionTitle("Recent Codex threads", count: history.count)
            if history.isEmpty {
                EmptyDetailRow(icon: "codex-chat", title: "No saved Codex threads", subtitle: "Completed Codex chats for this workspace will appear here.")
            } else {
                ForEach(history) { thread in
                    DetailHistoryRow(
                        thread: thread,
                        liveSession: liveSession(for: thread),
                        onOpen: { open(thread) }
                    )
                }
            }
        }
    }

    private func sectionTitle(_ title: String, count: Int) -> some View {
        HStack {
            Text(title)
                .font(.system(size: 12, weight: .bold))
                .foregroundStyle(OrionTheme.textDim)
            Spacer()
            Text("\(count)")
                .font(.system(size: 12, design: .monospaced))
                .foregroundStyle(OrionTheme.textDim)
        }
    }

    private func load() async {
        loading = true
        defer { loading = false }
        await state.refreshSessions()
        async let loadedServers = state.getServerStatuses(workspace: workspace)
        async let loadedHistory = state.codexHistory(workspace: workspace)
        let loadedChanges = (try? await state.changedFiles(workspacePath: workspace.path)) ?? []
        serverStatuses = await loadedServers
        history = await loadedHistory
        changedFiles = loadedChanges
    }

    private func liveSession(for thread: CodexHistoryThread) -> SessionInfo? {
        sessions.first { $0.threadId == thread.threadId || $0.tmuxName == thread.threadId }
    }

    private func open(_ thread: CodexHistoryThread) {
        Task {
            do {
                if let live = liveSession(for: thread) {
                    try await state.activateSession(live)
                } else {
                    _ = try await state.resumeCodexChat(workspacePath: workspace.path, threadId: thread.threadId)
                }
                await onRefresh()
                onClose()
            } catch {
                state.showTransientError("Failed to resume thread: \(error.localizedDescription)")
            }
        }
    }
}

private struct WorkspaceSummaryTile: View {
    let value: String
    let label: String
    let tint: Color

    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            Text(value)
                .font(.system(size: 19, weight: .bold, design: .monospaced))
                .foregroundStyle(tint)
            Text(label)
                .font(.system(size: 11, design: .monospaced))
                .foregroundStyle(OrionTheme.textDim)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(12)
        .background(OrionTheme.bgSurface)
        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 8, style: .continuous).stroke(OrionTheme.borderDim, lineWidth: 0.8))
    }
}

private struct DetailSessionRow: View {
    let session: SessionInfo
    let isActive: Bool

    var body: some View {
        HStack(spacing: 11) {
            AgentSigilView(session.type, size: 30)
            VStack(alignment: .leading, spacing: 3) {
                Text(session.label)
                    .font(.system(size: 14.5, weight: .semibold))
                    .foregroundStyle(OrionTheme.textPrimary)
                    .lineLimit(1)
                HStack(spacing: 6) {
                    Text(sessionLabel(session.type))
                    if let model = session.model, !model.isEmpty { Text(modelLabel(model)) }
                    if let reasoning = session.reasoningEffort, !reasoning.isEmpty { Text(reasoningLabel(reasoning)) }
                }
                .font(.system(size: 11, design: .monospaced))
                .foregroundStyle(OrionTheme.textDim)
                .lineLimit(1)
            }
            Spacer()
            Text(isActive ? "Active" : "Open")
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(isActive ? OrionTheme.accentBlue : OrionTheme.textDim)
        }
        .padding(12)
        .background(OrionTheme.bgSurface)
        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 8, style: .continuous).stroke(isActive ? OrionTheme.accentBlue.opacity(0.32) : OrionTheme.borderDim, lineWidth: 0.8))
    }
}

private struct DetailHistoryRow: View {
    let thread: CodexHistoryThread
    let liveSession: SessionInfo?
    let onOpen: () -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 11) {
            AgentSigilView("codex-chat", size: 30)
            VStack(alignment: .leading, spacing: 5) {
                HStack(spacing: 7) {
                    Text(shortThreadLabel(thread.threadId))
                        .font(.system(size: 13, weight: .semibold, design: .monospaced))
                        .foregroundStyle(OrionTheme.textPrimary)
                    Text(relativeTimeLabel(thread.updatedAt))
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(OrionTheme.textDim)
                }
                Text(thread.preview?.isEmpty == false ? thread.preview! : "No preview")
                    .font(.system(size: 12.5))
                    .foregroundStyle(OrionTheme.textSecondary)
                    .lineLimit(2)
                HStack(spacing: 8) {
                    if let model = thread.model, !model.isEmpty {
                        Text(modelLabel(model))
                    }
                    Text("\(thread.messageCount) msg\(thread.messageCount == 1 ? "" : "s")")
                }
                .font(.system(size: 10.5, design: .monospaced))
                .foregroundStyle(OrionTheme.textDim)
            }
            Spacer()
            Button(liveSession == nil ? "Resume" : "Open", action: onOpen)
                .font(.system(size: 12, weight: .semibold))
                .foregroundStyle(OrionTheme.accentBlue)
        }
        .padding(12)
        .background(OrionTheme.bgSurface)
        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 8, style: .continuous).stroke(OrionTheme.borderDim, lineWidth: 0.8))
    }
}

private struct EmptyDetailRow: View {
    let icon: String
    let title: String
    let subtitle: String

    var body: some View {
        HStack(spacing: 11) {
            AgentSigilView(icon, size: 30)
            VStack(alignment: .leading, spacing: 3) {
                Text(title)
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundStyle(OrionTheme.textSecondary)
                Text(subtitle)
                    .font(.system(size: 11.5))
                    .foregroundStyle(OrionTheme.textDim)
                    .lineLimit(2)
            }
            Spacer()
        }
        .padding(12)
        .background(OrionTheme.bgSurface)
        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 8, style: .continuous).stroke(OrionTheme.borderDim, lineWidth: 0.8))
    }
}

// MARK: - Workspace Sheet

struct WorkspaceSheet: View {
    @Environment(AppState.self) private var state
    @State private var showingNewWorktree = false

    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 14) {
                HStack {
                    VStack(alignment: .leading, spacing: 3) {
                        Text("Workspaces")
                            .font(.system(size: 22, weight: .semibold))
                            .foregroundStyle(OrionTheme.textPrimary)
                        Text(state.projectInfo?.name ?? "Orion")
                            .font(.system(size: 12, design: .monospaced))
                            .foregroundStyle(OrionTheme.textDim)
                    }
                    Spacer()
                    Button {
                        Task { await state.refreshSessions() }
                    } label: {
                        Image(systemName: "arrow.clockwise")
                            .font(.system(size: 14, weight: .medium))
                            .foregroundStyle(OrionTheme.textSecondary)
                            .frame(width: 34, height: 34)
                            .background(OrionTheme.bgSurface)
                            .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                    }
                    .buttonStyle(.plain)
                    Button { showingNewWorktree = true } label: {
                        Image(systemName: "plus")
                            .font(.system(size: 15, weight: .semibold))
                            .foregroundStyle(OrionTheme.textPrimary)
                            .frame(width: 34, height: 34)
                            .background(OrionTheme.bgSurface)
                            .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                    }
                    .buttonStyle(.plain)
                }

                ScrollView {
                    LazyVStack(spacing: 8) {
                        ForEach(state.workspaces) { workspace in
                            WorkspaceSwitcherRow(workspace: workspace)
                        }
                    }
                    .padding(.bottom, 10)
                }

                Button { showingNewWorktree = true } label: {
                    Label("New workspace", systemImage: "plus")
                        .font(.system(size: 14, weight: .semibold))
                        .foregroundStyle(OrionTheme.textPrimary)
                        .frame(maxWidth: .infinity)
                        .frame(height: 42)
                        .background(OrionTheme.bgSurface)
                        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                        .overlay(RoundedRectangle(cornerRadius: 8, style: .continuous).stroke(OrionTheme.borderDim, lineWidth: 0.8))
                }
                .buttonStyle(.plain)
            }
            .padding(16)
            .background(OrionTheme.bgPrimary.ignoresSafeArea())
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Done") { state.showWorkspaces = false }
                        .foregroundStyle(OrionTheme.accentBlue)
                }
            }
            .toolbarBackground(OrionTheme.bgSecondary, for: .navigationBar)
            .sheet(isPresented: $showingNewWorktree) {
                NewWorktreeSheet(
                    baseRefs: workspaceBaseRefs(mainBranch: state.projectInfo?.mainBranch, workspaces: state.workspaces),
                    onCancel: { showingNewWorktree = false },
                    onCreate: { draft in
                        showingNewWorktree = false
                        Task {
                            do {
                                try await state.createWorkspace(
                                    name: draft.name,
                                    baseRef: draft.baseRef,
                                    startWith: draft.startWith,
                                    firstPrompt: draft.firstPrompt,
                                    codexOptions: draft.codexOptions
                                )
                                state.showWorkspaces = false
                            } catch {
                                state.showTransientError("Failed to create worktree: \(error.localizedDescription)")
                            }
                        }
                    }
                )
                .presentationDetents([.large])
                .presentationDragIndicator(.visible)
            }
        }
    }
}

private struct WorkspaceSwitcherRow: View {
    @Environment(AppState.self) private var state
    let workspace: Workspace

    private var sessions: [SessionInfo] {
        state.sessions.filter { $0.workspacePath == workspace.path && $0.type != "server" }
    }

    private var isActive: Bool { state.activeWorkspacePath == workspace.path }

    var body: some View {
        Button {
            Task {
                await state.activateWorkspace(workspace.path)
                state.showWorkspaces = false
            }
        } label: {
            HStack(spacing: 11) {
                Circle()
                    .fill(isActive ? OrionTheme.accentBlue : sessions.isEmpty ? OrionTheme.border : OrionTheme.accentGreen)
                    .frame(width: 8, height: 8)
                    .shadow(color: isActive ? OrionTheme.accentBlue.opacity(0.45) : .clear, radius: 5)
                VStack(alignment: .leading, spacing: 3) {
                    HStack(spacing: 7) {
                        Text(workspace.name)
                            .font(.system(size: 16, weight: .semibold))
                            .foregroundStyle(OrionTheme.textPrimary)
                            .lineLimit(1)
                        if workspace.isMain {
                            Text("MAIN")
                                .font(.system(size: 9, weight: .bold, design: .monospaced))
                                .foregroundStyle(OrionTheme.accentBlue)
                                .padding(.horizontal, 6)
                                .padding(.vertical, 2)
                                .background(OrionTheme.accentBlue.opacity(0.15))
                                .clipShape(RoundedRectangle(cornerRadius: 4, style: .continuous))
                        }
                    }
                    Text(workspace.branch.isEmpty ? workspace.name : workspace.branch)
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(OrionTheme.textDim)
                        .lineLimit(1)
                }
                Spacer()
                Text(sessions.isEmpty ? "idle" : "\(sessions.count)")
                    .font(.system(size: 12, design: .monospaced))
                    .foregroundStyle(OrionTheme.textDim)
                Image(systemName: isActive ? "checkmark.circle.fill" : "circle")
                    .font(.system(size: 15))
                    .foregroundStyle(isActive ? OrionTheme.accentBlue : OrionTheme.textDim)
            }
            .padding(.horizontal, 12)
            .frame(height: 58)
            .background(isActive ? OrionTheme.bgSurface : OrionTheme.bgSecondary)
            .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 8, style: .continuous).stroke(isActive ? OrionTheme.accentBlue.opacity(0.24) : OrionTheme.borderDim, lineWidth: 0.8))
        }
        .buttonStyle(.plain)
    }
}

struct WorkspaceSection: View {
    @Environment(AppState.self) private var state
    let workspace: Workspace
    @State private var serverStatuses: [ServerStatus] = []
    @State private var loadingServers = false
    @State private var showingCodexOptions = false
    @State private var codexOptions = CodexLaunchOptions()

    // Non-server sessions only (shells, agents)
    private var sessions: [SessionInfo] {
        state.sessions.filter { $0.workspacePath == workspace.path && $0.type != "server" }
    }

    private var runningServers: [ServerStatus] {
        serverStatuses.filter { $0.running }
    }

    private var openTabCount: Int {
        state.sessions.filter { $0.workspacePath == workspace.path && $0.type != "server" }.count
    }

    private var isActiveWorkspace: Bool {
        state.activeWorkspacePath == workspace.path
    }

    var body: some View {
        Section {
            // Sessions — native swipe-to-delete
            ForEach(sessions) { session in
                Button {
                    Task {
                        do { try await state.activateSession(session) }
                        catch { state.showTransientError("Failed to open session: \(error.localizedDescription)") }
                    }
                    state.showWorkspaces = false
                } label: {
                    HStack(spacing: 10) {
                        AgentSigilView(session.type, size: 26)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(session.label).font(.system(size: 14, weight: .medium)).foregroundStyle(OrionTheme.textPrimary)
                            Text(session.type.replacingOccurrences(of: "-chat", with: " chat"))
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundStyle(OrionTheme.textDim)
                        }
                        Spacer()
                        Image(systemName: "chevron.right").font(.system(size: 11)).foregroundStyle(OrionTheme.textDim)
                    }
                    .padding(.vertical, 3)
                }
                .swipeActions(edge: .trailing, allowsFullSwipe: true) {
                    Button(role: .destructive) {
                        Task { await state.killSession(session) }
                    } label: {
                        Label("Kill", systemImage: "xmark.circle")
                    }
                }
                .listRowBackground(OrionTheme.bgSurface)
                .listRowSeparator(.hidden)
            }

            // Only show running servers
            ForEach(runningServers) { srv in
                HStack(spacing: 10) {
                    AgentSigilView("server", size: 24)
                    Text(srv.name.capitalized).font(.system(size: 14, weight: .medium)).foregroundStyle(OrionTheme.textPrimary)
                    Spacer()
                    Text(":\(srv.port)").font(.system(size: 12, design: .monospaced)).foregroundStyle(OrionTheme.textDim)
                }
                .listRowBackground(OrionTheme.bgSurface)
                .listRowSeparator(.hidden)
            }

            // Actions row: New session menu + server controls
            HStack(spacing: 10) {
                Menu {
                    Button {
                        Task {
                            try? await state.launchShell(workspacePath: workspace.path)
                            state.showWorkspaces = false
                        }
                    } label: {
                        Label("Shell", systemImage: "terminal")
                    }

                    Button {
                        showingCodexOptions = true
                    } label: {
                        Label("Codex Chat", systemImage: "bubble.left.and.bubble.right")
                    }

                    Button {
                        Task {
                            _ = try? await state.launchClaudeChat(workspacePath: workspace.path)
                            state.showWorkspaces = false
                        }
                    } label: {
                        Label("Claude Chat", systemImage: "bubble.left.and.bubble.right.fill")
                    }

                    if !state.agentTypes.isEmpty {
                        Divider()
                        ForEach(state.agentTypes) { agent in
                            Button {
                                Task {
                                    try? await state.launchAgent(workspacePath: workspace.path, agentType: agent.name)
                                    state.showWorkspaces = false
                                }
                            } label: {
                                Label(agent.label, systemImage: agentIcon(agent.name))
                            }
                        }
                    }
                } label: {
                    Label("New", systemImage: "plus").font(.system(size: 13))
                }

                Spacer()

                if !serverStatuses.isEmpty {
                    let anyRunning = !runningServers.isEmpty
                    Button {
                        Task {
                            loadingServers = true
                            if anyRunning { await state.stopServers(workspace: workspace) }
                            else { await state.startServers(workspace: workspace) }
                            serverStatuses = await state.getServerStatuses(workspace: workspace)
                            await state.refreshSessions()
                            loadingServers = false
                        }
                    } label: {
                        if loadingServers {
                            ProgressView().controlSize(.small)
                        } else {
                            Label(anyRunning ? "Stop Servers" : "Start Servers",
                                  systemImage: anyRunning ? "stop.fill" : "play.fill")
                                .font(.system(size: 13))
                                .foregroundStyle(anyRunning ? OrionTheme.accentRed : OrionTheme.accentGreen)
                        }
                    }.disabled(loadingServers)
                }
            }
            .listRowBackground(OrionTheme.bgSurface)
            .listRowSeparator(.hidden)
        } header: {
            Button {
                Task {
                    await state.activateWorkspace(workspace.path)
                    state.showWorkspaces = false
                }
            } label: {
                HStack(spacing: 8) {
                    Circle()
                        .fill(isActiveWorkspace ? OrionTheme.accentBlue : openTabCount > 0 ? OrionTheme.accentGreen : OrionTheme.border)
                        .frame(width: 8, height: 8)
                        .shadow(color: isActiveWorkspace ? OrionTheme.accentBlue.opacity(0.45) : .clear, radius: 5)
                    VStack(alignment: .leading, spacing: 2) {
                        HStack(spacing: 6) {
                            Text(workspace.name).font(.system(size: 14, weight: .semibold))
                                .foregroundStyle(OrionTheme.textPrimary)
                            if workspace.isMain {
                                Text("MAIN").font(.system(size: 9, weight: .bold)).padding(.horizontal, 5).padding(.vertical, 1)
                                    .background(OrionTheme.accentBlue.opacity(0.16)).foregroundStyle(OrionTheme.accentBlue).clipShape(RoundedRectangle(cornerRadius: 4))
                            }
                        }
                        Text(workspace.branch.isEmpty ? workspace.name : workspace.branch)
                            .font(.system(size: 11, design: .monospaced))
                            .foregroundStyle(OrionTheme.textDim)
                            .lineLimit(1)
                    }
                    Spacer()
                    if openTabCount > 0 {
                        Text("\(openTabCount) tab\(openTabCount == 1 ? "" : "s")")
                            .font(.system(size: 11))
                            .foregroundStyle(OrionTheme.textDim)
                    }
                    Image(systemName: isActiveWorkspace ? "checkmark.circle.fill" : "circle")
                        .font(.system(size: 13))
                        .foregroundStyle(isActiveWorkspace ? OrionTheme.accentBlue : OrionTheme.textDim)
                }
                .padding(.vertical, 4)
            }
            .buttonStyle(.plain)
        }
        .onAppear { Task { serverStatuses = await state.getServerStatuses(workspace: workspace) } }
        .sheet(isPresented: $showingCodexOptions) {
            CodexLaunchOptionsSheet(
                workspaceName: workspace.branch.isEmpty ? workspace.name : workspace.branch,
                options: $codexOptions,
                onCancel: { showingCodexOptions = false },
                onLaunch: {
                    let selected = codexOptions
                    showingCodexOptions = false
                    Task {
                        _ = try? await state.launchCodexChat(workspacePath: workspace.path, options: selected)
                        state.showWorkspaces = false
                    }
                }
            )
            .presentationDetents([.medium])
            .presentationDragIndicator(.visible)
        }
    }
}

private struct NewWorktreeDraft {
    var name = ""
    var baseRef: String
    var startWith = "codex-chat"
    var firstPrompt = ""
    var codexOptions = CodexLaunchOptions()
}

private struct NewWorktreeSheet: View {
    let baseRefs: [String]
    let onCancel: () -> Void
    let onCreate: (NewWorktreeDraft) -> Void
    @State private var draft: NewWorktreeDraft

    private let starts = [
        ("codex-chat", "Codex Chat"),
        ("claude-chat", "Claude Chat"),
        ("codex", "Codex CLI"),
        ("claude", "Claude CLI"),
        ("shell", "Shell"),
        ("none", "Nothing")
    ]

    init(baseRefs: [String], onCancel: @escaping () -> Void, onCreate: @escaping (NewWorktreeDraft) -> Void) {
        let refs = baseRefs.isEmpty ? ["main"] : baseRefs
        self.baseRefs = refs
        self.onCancel = onCancel
        self.onCreate = onCreate
        _draft = State(initialValue: NewWorktreeDraft(baseRef: refs[0]))
    }

    private var normalizedName: String {
        normalizedWorktreeName(draft.name)
    }

    private var canCreate: Bool {
        !normalizedName.isEmpty
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("Name") {
                    TextField("fix-stripe-webhook", text: $draft.name)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .font(.system(size: 15, design: .monospaced))
                    Text(normalizedName.isEmpty ? "Use lowercase letters, numbers, dots, underscores, and dashes." : "New branch: \(normalizedName)")
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(OrionTheme.textDim)
                }

                Section("Branch from") {
                    Picker("Base", selection: $draft.baseRef) {
                        ForEach(baseRefs, id: \.self) { Text($0).tag($0) }
                    }
                }

                Section("Start with") {
                    Picker("Session", selection: $draft.startWith) {
                        ForEach(starts, id: \.0) { item in
                            Text(item.1).tag(item.0)
                        }
                    }
                    if draft.startWith == "codex-chat" {
                        Picker("Model", selection: $draft.codexOptions.model) {
                            ForEach(["gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex", "gpt-5.3-codex-spark", "gpt-5.2"], id: \.self) {
                                Text(modelLabel($0)).tag($0)
                            }
                        }
                        Picker("Reasoning", selection: $draft.codexOptions.reasoningEffort) {
                            ForEach(["low", "medium", "high", "xhigh"], id: \.self) {
                                Text(reasoningLabel($0)).tag($0)
                            }
                        }
                    }
                }

                Section("First prompt") {
                    TextEditor(text: $draft.firstPrompt)
                        .frame(minHeight: 110)
                        .font(.system(size: 14))
                        .disabled(draft.startWith != "codex-chat" && draft.startWith != "claude-chat")
                    Text("Sent automatically when the worktree starts with a chat session.")
                        .font(.system(size: 11))
                        .foregroundStyle(OrionTheme.textDim)
                }
            }
            .scrollContentBackground(.hidden)
            .background(OrionTheme.bgPrimary)
            .navigationTitle("New worktree")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Cancel", action: onCancel)
                        .foregroundStyle(OrionTheme.accentBlue)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Create") {
                        var normalized = draft
                        normalized.name = normalizedName
                        onCreate(normalized)
                    }
                    .disabled(!canCreate)
                    .foregroundStyle(canCreate ? OrionTheme.accentBlue : OrionTheme.textDim)
                }
            }
            .toolbarBackground(OrionTheme.bgSecondary, for: .navigationBar)
        }
    }
}

private struct DiffReviewSheet: View {
    @Environment(AppState.self) private var state
    @State private var files: [GitChangedFile] = []
    @State private var selectedIndex = 0
    @State private var rawDiff = ""
    @State private var loading = false
    @State private var baseMode = "uncommitted"

    private var baseRef: String {
        baseMode == "main" ? (state.projectInfo?.mainBranch ?? "main") : ""
    }

    private var selectedFile: GitChangedFile? {
        guard !files.isEmpty else { return nil }
        return files[min(selectedIndex, files.count - 1)]
    }

    private var diffLines: [MobileDiffLine] {
        parseMobileDiff(rawDiff)
    }

    private var counts: (added: Int, removed: Int) {
        diffLines.reduce((0, 0)) { partial, line in
            switch line.kind {
            case .add: return (partial.0 + 1, partial.1)
            case .delete: return (partial.0, partial.1 + 1)
            default: return partial
            }
        }
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                Picker("Review base", selection: $baseMode) {
                    Text("Uncommitted").tag("uncommitted")
                    Text("vs \(state.projectInfo?.mainBranch ?? "main")").tag("main")
                }
                .pickerStyle(.segmented)
                .padding(.horizontal, 16)
                .padding(.top, 10)

                if files.isEmpty && !loading {
                    Spacer()
                    VStack(spacing: 10) {
                        Image(systemName: "checkmark.seal")
                            .font(.system(size: 30))
                            .foregroundStyle(OrionTheme.accentGreen)
                        Text("No changes")
                            .font(.system(size: 17, weight: .semibold))
                            .foregroundStyle(OrionTheme.textPrimary)
                        Text(state.activeWorkspace.map(workspaceReviewSubtitle) ?? "Pick a workspace to review.")
                            .font(.system(size: 12, design: .monospaced))
                            .foregroundStyle(OrionTheme.textDim)
                    }
                    Spacer()
                } else {
                    diffHeader
                    fileStrip
                    diffBody
                    diffFooter
                }
            }
            .background(OrionTheme.bgPrimary)
            .navigationTitle("Diff review")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Close") { state.showDiffReview = false }
                        .foregroundStyle(OrionTheme.accentBlue)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button { Task { await loadFiles() } } label: {
                        if loading {
                            ProgressView().controlSize(.mini)
                        } else {
                            Image(systemName: "arrow.clockwise")
                        }
                    }
                    .foregroundStyle(OrionTheme.accentBlue)
                }
            }
            .toolbarBackground(OrionTheme.bgSecondary, for: .navigationBar)
            .task { await loadFiles() }
            .onChange(of: baseMode) { _, _ in Task { await loadFiles() } }
        }
    }

    private var diffHeader: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .firstTextBaseline, spacing: 10) {
                VStack(alignment: .leading, spacing: 3) {
                    Text(files.isEmpty ? "0 files" : "\(min(selectedIndex + 1, files.count)) of \(files.count) files")
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(OrionTheme.textDim)
                        .textCase(.uppercase)
                    Text(selectedFile?.path ?? "Loading")
                        .font(.system(size: 16, weight: .semibold, design: .monospaced))
                        .foregroundStyle(OrionTheme.textPrimary)
                        .lineLimit(2)
                }
                Spacer()
                HStack(spacing: 7) {
                    Text("+\(counts.added)").foregroundStyle(OrionTheme.accentGreen)
                    Text("-\(counts.removed)").foregroundStyle(OrionTheme.accentRed)
                }
                .font(.system(size: 12, design: .monospaced))
            }

            HStack(spacing: 10) {
                AgentSigilView("codex-chat", size: 26)
                VStack(alignment: .leading, spacing: 2) {
                    Text(baseMode == "main" ? "Branch review" : "Workspace changes")
                        .font(.system(size: 12.5, weight: .medium))
                        .foregroundStyle(OrionTheme.textPrimary)
                    Text(state.activeWorkspace.map(workspaceReviewSubtitle) ?? "Active workspace")
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(OrionTheme.textDim)
                        .lineLimit(1)
                }
                Spacer()
                Text(selectedFile?.statusText ?? "")
                    .font(.system(size: 10, weight: .bold))
                    .padding(.horizontal, 7)
                    .padding(.vertical, 3)
                    .foregroundStyle(statusColor(selectedFile?.status ?? ""))
                    .background(statusColor(selectedFile?.status ?? "").opacity(0.12))
                    .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
            }
        }
        .padding(.horizontal, 16)
        .padding(.top, 12)
        .padding(.bottom, 10)
    }

    private var fileStrip: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(Array(files.enumerated()), id: \.element.path) { index, file in
                    Button {
                        selectedIndex = index
                        Task { await loadDiff(file: file) }
                    } label: {
                        HStack(spacing: 7) {
                            Text(file.status)
                                .font(.system(size: 11, weight: .bold, design: .monospaced))
                                .foregroundStyle(statusColor(file.status))
                            Text((file.path as NSString).lastPathComponent)
                                .font(.system(size: 12, design: .monospaced))
                                .lineLimit(1)
                        }
                        .padding(.horizontal, 10)
                        .padding(.vertical, 8)
                        .background(index == selectedIndex ? OrionTheme.accentBlue.opacity(0.16) : OrionTheme.bgSurface)
                        .foregroundStyle(index == selectedIndex ? OrionTheme.textPrimary : OrionTheme.textSecondary)
                        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                        .overlay(RoundedRectangle(cornerRadius: 8, style: .continuous).stroke(index == selectedIndex ? OrionTheme.accentBlue.opacity(0.5) : OrionTheme.borderDim, lineWidth: 0.7))
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.horizontal, 16)
            .padding(.bottom, 10)
        }
    }

    private var diffBody: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                if loading {
                    ProgressView().frame(maxWidth: .infinity).padding(.vertical, 28)
                } else if diffLines.isEmpty {
                    Text("(no textual diff)")
                        .font(.system(size: 12, design: .monospaced))
                        .foregroundStyle(OrionTheme.textDim)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(14)
                } else {
                    ForEach(Array(diffLines.enumerated()), id: \.offset) { _, line in
                        diffLine(line)
                    }
                }
            }
            .background(OrionTheme.bgTerminal)
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 10, style: .continuous).stroke(OrionTheme.borderDim, lineWidth: 0.7))
            .padding(.horizontal, 12)
            .padding(.bottom, 12)
        }
    }

    private var diffFooter: some View {
        HStack(spacing: 10) {
            Button("Previous") {
                let nextIndex = max(selectedIndex - 1, 0)
                selectedIndex = nextIndex
                Task { await loadDiff(file: files[nextIndex]) }
            }
            .disabled(selectedIndex == 0)
            Spacer()
            Button("Next file") {
                let nextIndex = min(selectedIndex + 1, max(files.count - 1, 0))
                selectedIndex = nextIndex
                Task { await loadDiff(file: files[nextIndex]) }
            }
            .disabled(files.isEmpty || selectedIndex >= files.count - 1)
            .buttonStyle(.borderedProminent)
            .tint(OrionTheme.accentBlue)
        }
        .padding(.horizontal, 12)
        .padding(.top, 10)
        .padding(.bottom, 18)
        .background(OrionTheme.bgSecondary)
        .overlay(alignment: .top) { OrionTheme.border.frame(height: 0.5) }
    }

    private func diffLine(_ line: MobileDiffLine) -> some View {
        HStack(alignment: .top, spacing: 0) {
            Text(line.kind.sign)
                .font(.system(size: 11, design: .monospaced))
                .frame(width: 22)
                .foregroundStyle(line.kind.foreground)
            Text(line.text.isEmpty ? " " : line.text)
                .font(.system(size: 11.5, design: .monospaced))
                .foregroundStyle(line.kind == .context ? OrionTheme.textSecondary : OrionTheme.textPrimary)
                .frame(maxWidth: .infinity, alignment: .leading)
                .lineLimit(nil)
        }
        .padding(.vertical, line.kind == .hunk ? 7 : 2)
        .padding(.horizontal, 8)
        .background(line.kind.background)
    }

    private func loadFiles() async {
        loading = true
        defer { loading = false }
        do {
            files = try await state.changedFiles(base: baseRef)
            selectedIndex = min(selectedIndex, max(files.count - 1, 0))
            await loadDiff()
        } catch {
            files = []
            rawDiff = ""
            state.showTransientError("Failed to load diff: \(error.localizedDescription)")
        }
    }

    private func loadDiff(file explicitFile: GitChangedFile? = nil) async {
        guard let file = explicitFile ?? selectedFile else {
            rawDiff = ""
            return
        }
        do {
            rawDiff = try await state.unifiedDiff(for: file, base: baseRef)
        } catch {
            rawDiff = ""
        }
    }
}

private struct CodexLaunchOptionsSheet: View {
    let workspaceName: String
    @Binding var options: CodexLaunchOptions
    let onCancel: () -> Void
    let onLaunch: () -> Void

    private let models = ["gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex", "gpt-5.3-codex-spark", "gpt-5.2"]
    private let efforts = ["low", "medium", "high", "xhigh"]
    private let approvals = ["never", "on-request", "on-failure", "untrusted"]
    private let sandboxes = ["danger-full-access", "workspace-write", "read-only"]
    private let modes = ["default", "plan"]

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Picker("Model", selection: $options.model) {
                        ForEach(models, id: \.self) { Text(modelLabel($0)).tag($0) }
                    }
                    Picker("Reasoning", selection: $options.reasoningEffort) {
                        ForEach(efforts, id: \.self) { Text(reasoningLabel($0)).tag($0) }
                    }
                    Picker("Approvals", selection: $options.approvalPolicy) {
                        ForEach(approvals, id: \.self) { Text(approvalPickerLabel($0)).tag($0) }
                    }
                    Picker("Sandbox", selection: $options.sandboxMode) {
                        ForEach(sandboxes, id: \.self) { Text(sandboxPickerLabel($0)).tag($0) }
                    }
                    Picker("Mode", selection: $options.collaborationMode) {
                        ForEach(modes, id: \.self) { Text($0 == "plan" ? "Plan first" : "Default").tag($0) }
                    }
                } header: {
                    Text(workspaceName)
                } footer: {
                    Text("These options are sent when the Codex app-server thread starts.")
                }
            }
            .scrollContentBackground(.hidden)
            .background(OrionTheme.bgPrimary)
            .navigationTitle("Codex Chat")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Cancel", action: onCancel)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Start", action: onLaunch)
                        .fontWeight(.semibold)
                }
            }
            .toolbarBackground(OrionTheme.bgSecondary, for: .navigationBar)
        }
    }
}

// MARK: - Tabs

private enum NewSessionMenuStyle {
    case tabBar
    case prominent
}

private struct NewSessionMenu: View {
    @Environment(AppState.self) private var state
    let workspace: Workspace
    var style: NewSessionMenuStyle = .tabBar
    @State private var showingCodexOptions = false
    @State private var codexOptions = CodexLaunchOptions()

    var body: some View {
        Menu {
            Button {
                Task { await launch { try await state.launchShell(workspacePath: workspace.path) } }
            } label: {
                Label("Shell", systemImage: "terminal")
            }
            Button { showingCodexOptions = true } label: {
                Label("Codex Chat", systemImage: "bubble.left.and.bubble.right")
            }
            Button {
                Task { await launch { _ = try await state.launchClaudeChat(workspacePath: workspace.path) } }
            } label: {
                Label("Claude Chat", systemImage: "bubble.left.and.bubble.right.fill")
            }
            if !state.agentTypes.isEmpty {
                Divider()
                ForEach(state.agentTypes) { agent in
                    Button {
                        Task { await launch { try await state.launchAgent(workspacePath: workspace.path, agentType: agent.name) } }
                    } label: {
                        Label(agent.label, systemImage: agentIcon(agent.name))
                    }
                }
            }
        } label: {
            menuLabel
        }
        .buttonStyle(.plain)
        .sheet(isPresented: $showingCodexOptions) {
            CodexLaunchOptionsSheet(
                workspaceName: workspace.branch.isEmpty ? workspace.name : workspace.branch,
                options: $codexOptions,
                onCancel: { showingCodexOptions = false },
                onLaunch: {
                    let selected = codexOptions
                    showingCodexOptions = false
                    Task {
                        await launch {
                            _ = try await state.launchCodexChat(workspacePath: workspace.path, options: selected)
                        }
                    }
                }
            )
            .presentationDetents([.medium])
            .presentationDragIndicator(.visible)
        }
    }

    @ViewBuilder
    private var menuLabel: some View {
        switch style {
        case .tabBar:
            Image(systemName: "plus")
                .font(.system(size: 15, weight: .semibold))
                .foregroundStyle(OrionTheme.textPrimary)
                .frame(width: 34, height: 30)
                .background(OrionTheme.bgSurface)
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: 8, style: .continuous).stroke(OrionTheme.borderDim, lineWidth: 0.7))
        case .prominent:
            Label("New session", systemImage: "plus")
                .font(.system(size: 14, weight: .semibold))
                .foregroundStyle(Color(hex: 0x18233A))
                .padding(.horizontal, 17)
                .frame(height: 38)
                .background(OrionTheme.accentBlue)
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
        }
    }

    private func launch(_ operation: () async throws -> Void) async {
        do {
            try await operation()
            state.showWorkspaces = false
        } catch {
            state.showTransientError("Failed to start session: \(error.localizedDescription)")
        }
    }
}

struct TabStrip: View {
    @Environment(AppState.self) private var state
    var body: some View {
        HStack(spacing: 7) {
            if let workspace = state.activeWorkspace {
                NewSessionMenu(workspace: workspace, style: .tabBar)
            } else {
                Button { state.showWorkspaces = true } label: {
                    Image(systemName: "plus")
                        .font(.system(size: 15, weight: .semibold))
                        .foregroundStyle(OrionTheme.textDim)
                        .frame(width: 34, height: 30)
                }
                .buttonStyle(.plain)
            }
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 6) {
                    if state.visibleTabs.isEmpty {
                        Text(state.activeWorkspace?.name ?? "Workspace")
                            .font(.system(size: 12, weight: .medium))
                            .foregroundStyle(OrionTheme.textDim)
                            .lineLimit(1)
                            .padding(.horizontal, 8)
                            .frame(height: 30)
                    } else {
                        ForEach(state.visibleTabs) { tab in
                            TabPill(tab: tab, isActive: tab.id == state.activeTabId)
                        }
                    }
                }
                .padding(.vertical, 6)
            }
        }
        .padding(.horizontal, 10)
        .frame(height: 44)
        .background(OrionTheme.bgSecondary)
        .overlay(alignment: .bottom) { OrionTheme.border.frame(height: 0.5) }
    }
}

struct TabPill: View {
    @Environment(AppState.self) private var state
    let tab: TerminalTab; let isActive: Bool
    var body: some View {
        HStack(spacing: 6) {
            Button { state.activateTab(tab.id) } label: {
                HStack(spacing: 7) {
                    AgentSigilView(tab.type, size: 20)
                    Text(tab.label)
                        .font(.system(size: 12, weight: isActive ? .medium : .regular))
                        .foregroundStyle(isActive ? OrionTheme.textPrimary : OrionTheme.textDim)
                        .lineLimit(1)
                        .frame(maxWidth: 130, alignment: .leading)
                }
                .padding(.leading, 4)
                .padding(.trailing, 6)
                .frame(height: 30)
            }
            .buttonStyle(.plain)
            if isConvertibleSession(tab.type) {
                Button {
                    if let session = state.sessions.first(where: { $0.id == tab.id || $0.tmuxName == tab.tmuxSession }) {
                        Task { await state.convertSession(session) }
                    }
                } label: {
                    Image(systemName: "arrow.triangle.2.circlepath")
                        .font(.system(size: 10, weight: .medium))
                        .foregroundStyle(OrionTheme.textDim)
                        .frame(width: 22, height: 30)
                }
                .buttonStyle(.plain)
            }
            Button { state.requestKillSession(tab.id) } label: {
                Image(systemName: "xmark")
                    .font(.system(size: 10, weight: .medium))
                    .foregroundStyle(OrionTheme.textDim)
                    .frame(width: 22, height: 30)
                    .padding(.trailing, 6)
            }
            .buttonStyle(.plain)
        }
        .frame(height: 30)
        .background(isActive ? OrionTheme.bgSurface : .clear)
        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 10, style: .continuous).stroke(isActive ? OrionTheme.borderDim : .clear, lineWidth: 0.7))
    }
}

// MARK: - Session type icons (matches desktop app)

func sessionIcon(_ type: String) -> String {
    switch type {
    case "claude": return "\u{25C6}"  // ◆
    case "claude-chat": return "\u{25C6}"  // ◆
    case "codex-chat": return "\u{25C8}"  // ◈
    case "codex":  return "\u{25C7}"  // ◇
    case "server": return "\u{25B8}"  // ▸
    default:       return "\u{203A}"  // ›
    }
}

func sessionLabel(_ type: String) -> String {
    switch type {
    case "claude": return "Claude CLI"
    case "claude-chat": return "Claude chat"
    case "codex-chat": return "Codex chat"
    case "codex": return "Codex CLI"
    case "server": return "Server"
    case "shell": return "Shell"
    default: return type.replacingOccurrences(of: "-", with: " ")
    }
}

func isConvertibleSession(_ type: String) -> Bool {
    type == "claude" || type == "codex" || type == "claude-chat" || type == "codex-chat"
}

func agentIcon(_ name: String) -> String {
    switch name {
    case "claude": return "diamond.fill"
    case "codex":  return "diamond"
    default:       return "sparkles"
    }
}

private func workspaceBaseRefs(mainBranch: String?, workspaces: [Workspace]) -> [String] {
    var refs: [String] = []
    func append(_ value: String?) {
        guard let value else { return }
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty, trimmed != "(detached)", !refs.contains(trimmed) else { return }
        refs.append(trimmed)
    }
    append(mainBranch)
    workspaces.forEach { append($0.branch) }
    if refs.isEmpty { refs.append("main") }
    return refs
}

private func normalizedWorktreeName(_ name: String) -> String {
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

private func workspaceReviewSubtitle(_ workspace: Workspace) -> String {
    if workspace.isMain { return "Main workspace" }
    if !workspace.branch.isEmpty { return workspace.branch }
    return workspace.name
}

private func shortThreadLabel(_ threadId: String) -> String {
    let trimmed = threadId.trimmingCharacters(in: .whitespacesAndNewlines)
    if trimmed.count <= 12 { return trimmed }
    return String(trimmed.prefix(8))
}

private func relativeTimeLabel(_ value: String) -> String {
    let fractional = ISO8601DateFormatter()
    fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    let standard = ISO8601DateFormatter()
    let date = fractional.date(from: value) ?? standard.date(from: value)
    guard let date else { return "" }
    let formatter = RelativeDateTimeFormatter()
    formatter.unitsStyle = .abbreviated
    return formatter.localizedString(for: date, relativeTo: Date())
}

private struct MobileDiffLine {
    let kind: MobileDiffKind
    let text: String
}

private enum MobileDiffKind: Equatable {
    case add
    case delete
    case context
    case hunk

    var sign: String {
        switch self {
        case .add: return "+"
        case .delete: return "-"
        case .hunk: return "@"
        case .context: return " "
        }
    }

    var foreground: Color {
        switch self {
        case .add: return OrionTheme.accentGreen
        case .delete: return OrionTheme.accentRed
        case .hunk: return OrionTheme.accentBlue
        case .context: return OrionTheme.textDim
        }
    }

    var background: Color {
        switch self {
        case .add: return OrionTheme.accentGreen.opacity(0.10)
        case .delete: return OrionTheme.accentRed.opacity(0.10)
        case .hunk: return OrionTheme.bgSecondary
        case .context: return .clear
        }
    }
}

private func parseMobileDiff(_ raw: String) -> [MobileDiffLine] {
    raw.split(separator: "\n", omittingEmptySubsequences: false).compactMap { part in
        let line = String(part)
        if line.hasPrefix("diff --git") || line.hasPrefix("index ") || line.hasPrefix("--- ") || line.hasPrefix("+++ ") {
            return nil
        }
        if line.hasPrefix("@@") {
            return MobileDiffLine(kind: .hunk, text: line)
        }
        if line.hasPrefix("+") {
            return MobileDiffLine(kind: .add, text: String(line.dropFirst()))
        }
        if line.hasPrefix("-") {
            return MobileDiffLine(kind: .delete, text: String(line.dropFirst()))
        }
        return MobileDiffLine(kind: .context, text: line.hasPrefix(" ") ? String(line.dropFirst()) : line)
    }
}

private func countDiffChanges(_ raw: String) -> DiffStats {
    raw.split(separator: "\n", omittingEmptySubsequences: false).reduce(DiffStats()) { partial, part in
        let line = String(part)
        var next = partial
        if line.hasPrefix("+"), !line.hasPrefix("+++ ") {
            next.added += 1
        } else if line.hasPrefix("-"), !line.hasPrefix("--- ") {
            next.removed += 1
        }
        return next
    }
}

private func statusColor(_ status: String) -> Color {
    switch status {
    case "A": return OrionTheme.accentGreen
    case "D": return OrionTheme.accentRed
    case "R": return OrionTheme.accentBlue
    case "M": return OrionTheme.accentYellow
    default: return OrionTheme.textDim
    }
}

func sessionColor(_ type: String) -> Color {
    switch type {
    case "claude", "claude-chat": return OrionTheme.accentPurple
    case "codex-chat": return OrionTheme.accentGreen
    case "codex":  return OrionTheme.accentBlue
    case "server": return OrionTheme.accentYellow
    default:       return OrionTheme.textSecondary
    }
}

// MARK: - Codex Chat

private struct CodexChatRow: Identifiable {
    let id: String
    let type: String
    let label: String
    var text: String
    var details: String?
    let toolUseId: String?
    let planPath: String?
    let attachments: [ChatAttachmentPayload]
    var resultText: String?
    var resultDetails: String?
    var toolStatus: String?
    var permissionState: String? = nil
    var planState: String? = nil
    var answerText: String? = nil
}

private struct ChatSessionMetadata: Decodable {
    let provider: String?
    let viewMode: String?
    let model: String?
    let reasoningEffort: String?
    let approvalPolicy: String?
    let sandboxMode: String?
    let permissionMode: String?
    let collaborationMode: String?
    let threadId: String?
}

private struct MobileLiveActivityItem: Identifiable {
    let id: String
    let kind: String
    let label: String
    let value: String?
}

private struct PendingChatImage: Identifiable {
    let id = UUID()
    let name: String
    let mimeType: String
    let data: String
    let preview: UIImage?

    var payload: ChatAttachmentPayload {
        ChatAttachmentPayload(id: id.uuidString, name: name, mimeType: mimeType, data: data)
    }
}

private struct CompactChatChip: View {
    let title: String
    let tint: Color

    var body: some View {
        HStack(spacing: 5) {
            Circle()
                .fill(tint)
                .frame(width: 5, height: 5)
            Text(title)
                .font(.system(size: 10, weight: .semibold))
                .foregroundStyle(OrionTheme.textSecondary)
                .lineLimit(1)
        }
        .padding(.horizontal, 7)
        .frame(height: 22)
        .background(OrionTheme.bgPrimary.opacity(0.55))
        .clipShape(Capsule())
        .overlay(Capsule().stroke(OrionTheme.borderDim, lineWidth: 0.5))
    }
}

struct CodexChatView: View {
    @Environment(AppState.self) private var state
    let connection: CodexChatConnection
    @State private var input = ""
    @State private var answers: [String: String] = [:]
    @State private var submittedAnswers: [String: String] = [:]
    @State private var selectedPhotoItems: [PhotosPickerItem] = []
    @State private var pendingImages: [PendingChatImage] = []
    @State private var isLoadingPhotos = false
    @State private var expandedPlan: CodexChatRow?
    @State private var dismissedKeyboardDuringDrag = false
    @FocusState private var composerFocused: Bool

    private let chatBottomID = "chat-bottom"
    private var assistantName: String { connection.displayName }
    private var assistantColor: Color { sessionColor(connection.sessionType) }
    private var sessionMetadata: ChatSessionMetadata? { chatSessionMetadata(connection.messages) }

    var body: some View {
        VStack(spacing: 0) {
            compactHeader
                .padding(.horizontal, 12)
                .frame(height: 42)
            .background(OrionTheme.bgSecondary)
            .overlay(alignment: .bottom) { OrionTheme.border.frame(height: 0.5) }

            ScrollViewReader { proxy in
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 14) {
                        if chatRows.isEmpty {
                            Text("Ask \(assistantName) to inspect, edit, or explain this workspace.")
                                .font(.system(size: 14))
                                .foregroundStyle(OrionTheme.textDim)
                                .frame(maxWidth: .infinity)
                                .padding(.top, 120)
                        }
                        ForEach(chatRows) { row in
                            chatRow(row)
                                .id(row.id)
                        }
                        Color.clear
                            .frame(height: 1)
                            .id(chatBottomID)
                    }
                    .padding(14)
                }
                .scrollDismissesKeyboard(.interactively)
                .simultaneousGesture(keyboardDismissGesture)
                .background(OrionTheme.bgTerminal)
                .safeAreaInset(edge: .bottom, spacing: 0) {
                    composer
                }
                .onChange(of: connection.messages.count) { _, _ in
                    scrollToChatBottom(proxy)
                }
            }
        }
        .fullScreenCover(item: $expandedPlan) { plan in
            PlanReviewView(
                plan: plan,
                assistantName: assistantName,
                accentColor: assistantColor,
                onClose: { expandedPlan = nil },
                onReviewDiff: {
                    expandedPlan = nil
                    state.showDiffReview = true
                },
                onApprove: {
                    connection.approvePlan()
                    expandedPlan = nil
                }
            )
        }
        .onAppear {
            connection.reconnectOrProbe()
        }
        .onChange(of: selectedPhotoItems) { _, items in
            guard !items.isEmpty else { return }
            Task { await loadPhotoAttachments(items) }
        }
    }

    private var compactHeader: some View {
        HStack(spacing: 8) {
            AgentSigilView(connection.sessionType, size: 24, strong: true)
            Text(assistantName)
                .font(.system(size: 13, weight: .semibold))
                .foregroundStyle(OrionTheme.textPrimary)
                .lineLimit(1)

            if let mode = compactModeLabel(sessionMetadata) {
                CompactChatChip(title: mode, tint: mode == "plan" ? OrionTheme.accentBlue : OrionTheme.textDim)
            }

            if let activity = liveActivityItems.first {
                CompactChatChip(
                    title: activity.value.map { "\(activity.label) \($0)" } ?? activity.label,
                    tint: liveActivityColor(activity.kind)
                )
            } else {
                Text(statusLabel)
                    .font(.system(size: 11))
                    .foregroundStyle(OrionTheme.textDim)
                    .lineLimit(1)
            }

            Spacer(minLength: 6)

            HStack(spacing: 5) {
                Circle()
                    .fill(connectionBadgeColor)
                    .frame(width: 6, height: 6)
                Text(connectionBadgeText)
                    .font(.system(size: 10, weight: .medium))
                    .foregroundStyle(connectionBadgeForeground)
                    .lineLimit(1)
            }
        }
    }

    private var statusLabel: String {
        if let message = connection.messages.reversed().first(where: { $0.type == "status" }),
           let status = message.status {
            switch status {
            case "running": return message.text ?? "\(assistantName) is working"
            case "waiting_input": return "Waiting for your answer"
            case "starting": return "Starting \(assistantName)"
            default: return "Ready"
            }
        }
        return "Ready"
    }

    private var connectionBadgeText: String {
        if connection.queuedMessageCount > 0 {
            return connection.queuedMessageCount == 1 ? "1 queued" : "\(connection.queuedMessageCount) queued"
        }
        switch connection.connectionState {
        case .connected: return "Online"
        case .reconnecting: return "Reconnecting"
        case .failed: return "Reconnect"
        case .disconnected: return "Offline"
        }
    }

    private var connectionBadgeColor: Color {
        if connection.queuedMessageCount > 0 { return OrionTheme.accentYellow }
        switch connection.connectionState {
        case .connected: return OrionTheme.accentGreen
        case .reconnecting: return OrionTheme.accentYellow
        case .failed: return OrionTheme.accentRed
        case .disconnected: return OrionTheme.textDim
        }
    }

    private var connectionBadgeForeground: Color {
        connection.connectionState == .disconnected && connection.queuedMessageCount == 0
            ? OrionTheme.textDim
            : OrionTheme.textSecondary
    }

    private var isAssistantRunning: Bool {
        connection.messages.reversed().first(where: { $0.type == "status" })?.status == "running"
    }

    private var workingLabel: String {
        if let message = connection.messages.reversed().first(where: { $0.type == "status" && $0.status == "running" }),
           let text = message.text,
           !text.isEmpty {
            return text
        }
        return "\(assistantName) is working"
    }

    private var chatRows: [CodexChatRow] {
        mergeChatRows(connection.messages, assistantName: assistantName)
    }

    private var liveActivityItems: [MobileLiveActivityItem] {
        liveChatActivityItems(messages: connection.messages, rows: chatRows, assistantName: assistantName)
    }

    private func compactModeLabel(_ metadata: ChatSessionMetadata?) -> String? {
        guard let metadata else { return nil }
        let rawMode = (metadata.permissionMode ?? metadata.collaborationMode ?? "")
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .lowercased()
        guard !rawMode.isEmpty, rawMode != "default" else { return nil }
        if rawMode == "plan" { return "plan" }
        if rawMode == "approved" { return "approved" }
        return rawMode
    }

    @ViewBuilder
    private func modeStrip(_ metadata: ChatSessionMetadata) -> some View {
        let items: [(String, String?)] = [
            ("model", metadata.model),
            ("reasoning", metadata.reasoningEffort),
            ("approvals", approvalLabel(metadata.approvalPolicy)),
            ("sandbox", sandboxLabel(metadata.sandboxMode)),
            ("mode", modeLabel(metadata.permissionMode ?? metadata.collaborationMode))
        ]
        let visible = items.compactMap { item -> (String, String)? in
            let (label, value) = item
            guard let value, !value.isEmpty else { return nil }
            return (label, value)
        }
        if !visible.isEmpty {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 6) {
                    ForEach(Array(visible.enumerated()), id: \.offset) { _, item in
                        HStack(spacing: 5) {
                            Text(item.0)
                                .foregroundStyle(OrionTheme.textDim)
                            Text(item.1)
                                .fontWeight(.semibold)
                                .foregroundStyle(OrionTheme.textSecondary)
                        }
                        .font(.system(size: 10, design: .monospaced))
                        .padding(.horizontal, 7)
                        .padding(.vertical, 5)
                        .background(OrionTheme.bgActive.opacity(0.72))
                        .overlay(RoundedRectangle(cornerRadius: 7).stroke(OrionTheme.borderDim, lineWidth: 0.5))
                        .clipShape(RoundedRectangle(cornerRadius: 7))
                    }
                }
            }
        }
    }

    private func liveActivityStrip(_ items: [MobileLiveActivityItem]) -> some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 6) {
                ForEach(items) { item in
                    HStack(spacing: 6) {
                        Circle()
                            .fill(liveActivityColor(item.kind))
                            .frame(width: 6, height: 6)
                            .shadow(color: liveActivityColor(item.kind).opacity(0.35), radius: 4)
                        Text(item.label)
                            .fontWeight(.semibold)
                            .foregroundStyle(OrionTheme.textSecondary)
                            .lineLimit(1)
                        if let value = item.value, !value.isEmpty {
                            Text(value)
                                .font(.system(size: 10, design: .monospaced))
                                .foregroundStyle(OrionTheme.textDim)
                                .lineLimit(1)
                        }
                    }
                    .font(.system(size: 11))
                    .padding(.horizontal, 8)
                    .padding(.vertical, 5)
                    .background(OrionTheme.bgPrimary.opacity(0.58))
                    .overlay(Capsule().stroke(OrionTheme.borderDim, lineWidth: 0.5))
                    .clipShape(Capsule())
                }
            }
        }
    }

    @ViewBuilder
    private func chatRow(_ row: CodexChatRow) -> some View {
        if row.type == "plan" {
            planRow(row)
        } else if row.type == "loading" {
            loadingRow(row)
        } else if row.type == "tool" {
            toolRow(row)
        } else if row.type == "thinking_delta" {
            reasoningRow(row)
        } else if row.type == "permission_request" {
            permissionRow(row)
        } else if isActivityRow(row) {
            activityRow(row)
        } else if row.type == "user" {
            HStack {
                Spacer(minLength: 46)
                VStack(alignment: .trailing, spacing: 5) {
                    Text("You")
                        .font(.system(size: 11))
                        .foregroundStyle(OrionTheme.textDim)
                    messageBubble(row)
                }
            }
            .frame(maxWidth: .infinity, alignment: .trailing)
        } else {
            HStack(alignment: .bottom, spacing: 8) {
                AgentSigilView(connection.sessionType, size: 24)
                VStack(alignment: .leading, spacing: 5) {
                    Text(row.type == "permission_request" ? "\(assistantName) needs an answer" : assistantName)
                        .font(.system(size: 11))
                        .foregroundStyle(OrionTheme.textDim)
                    messageBubble(row)
                }
                Spacer(minLength: 46)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func planRow(_ row: CodexChatRow) -> some View {
        let markdown = row.details ?? row.text
        let insights = planInsights(markdown)
        let sections = planSections(markdown)
        let isWaiting = row.planState != "approved"
        let planTint = isWaiting ? OrionTheme.accentBlue : OrionTheme.accentGreen
        return HStack(alignment: .bottom, spacing: 8) {
            AgentSigilView(connection.sessionType, size: 24)
            VStack(alignment: .leading, spacing: 5) {
                Text("\(assistantName) has a plan")
                    .font(.system(size: 11))
                    .foregroundStyle(OrionTheme.textDim)
                VStack(alignment: .leading, spacing: 12) {
                    HStack(alignment: .top) {
                        VStack(alignment: .leading, spacing: 4) {
                            Text(isWaiting ? "PLAN · WAITING FOR YOU" : "PLAN · APPROVED")
                                .font(.system(size: 10, weight: .semibold))
                                .foregroundStyle(planTint)
                            Text(row.text.isEmpty ? "Plan ready" : row.text)
                                .font(.system(size: 15, weight: .semibold))
                                .foregroundStyle(OrionTheme.textPrimary)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                        Spacer(minLength: 8)
                        VStack(alignment: .trailing, spacing: 6) {
                            Text(isWaiting ? "Plan" : "Approved")
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundStyle(planTint)
                                .padding(.horizontal, 8)
                                .padding(.vertical, 4)
                                .overlay(Capsule().stroke(planTint.opacity(0.45), lineWidth: 0.5))
                            if insights.sections > 0 {
                                Text("\(insights.sections) sections")
                                    .font(.system(size: 10, design: .monospaced))
                                    .foregroundStyle(OrionTheme.textDim)
                            }
                        }
                    }

                    if !sections.isEmpty {
                        ScrollView(.horizontal, showsIndicators: false) {
                            HStack(spacing: 6) {
                                ForEach(sections.prefix(4), id: \.self) { section in
                                    Text(section)
                                        .font(.system(size: 11))
                                        .foregroundStyle(OrionTheme.textSecondary)
                                        .lineLimit(1)
                                        .padding(.horizontal, 8)
                                        .padding(.vertical, 4)
                                        .background(OrionTheme.accentBlue.opacity(0.08))
                                        .overlay(Capsule().stroke(OrionTheme.accentBlue.opacity(0.18), lineWidth: 0.5))
                                        .clipShape(Capsule())
                                }
                            }
                        }
                    }

                    VStack(alignment: .leading, spacing: 7) {
                        ForEach(Array(planPreview(markdown).components(separatedBy: .newlines).filter { !$0.isEmpty }.prefix(4).enumerated()), id: \.offset) { _, line in
                            HStack(alignment: .top, spacing: 8) {
                                Image(systemName: "circle")
                                    .font(.system(size: 12, weight: .medium))
                                    .foregroundStyle(OrionTheme.accentBlue)
                                    .frame(width: 20, height: 20)
                                Text(cleanPlanPreviewLine(line))
                                    .font(.system(size: 13))
                                    .foregroundStyle(OrionTheme.textSecondary)
                                    .fixedSize(horizontal: false, vertical: true)
                            }
                        }
                    }

                    HStack(spacing: 8) {
                        Button("Review plan") { expandedPlan = row }
                            .buttonStyle(.bordered)
                        Button("Review diff") { state.showDiffReview = true }
                            .buttonStyle(.bordered)
                    }
                    if isWaiting {
                        Button("Approve & run") { connection.approvePlan() }
                            .buttonStyle(.borderedProminent)
                            .tint(OrionTheme.accentBlue)
                            .frame(maxWidth: .infinity, alignment: .trailing)
                    }
                }
                .padding(12)
                .frame(maxWidth: 340, alignment: .leading)
                .background(OrionTheme.bgSurface)
                .overlay(RoundedRectangle(cornerRadius: 18, style: .continuous).stroke(planTint.opacity(0.28), lineWidth: 0.7))
                .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
            }
            Spacer(minLength: 22)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func loadingRow(_ row: CodexChatRow) -> some View {
        return HStack(alignment: .bottom, spacing: 8) {
            AgentSigilView(connection.sessionType, size: 24)
            VStack(alignment: .leading, spacing: 5) {
                Text(assistantName)
                    .font(.system(size: 11))
                    .foregroundStyle(OrionTheme.textDim)
                HStack(spacing: 8) {
                    Text(row.text.isEmpty ? "\(assistantName) is thinking" : row.text)
                        .font(.system(size: 13))
                        .foregroundStyle(OrionTheme.textSecondary)
                        .fixedSize(horizontal: false, vertical: true)
                    TypingDotsView()
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 10)
                .frame(maxWidth: 300, alignment: .leading)
                .background(OrionTheme.bgSurface)
                .overlay(RoundedRectangle(cornerRadius: 8).stroke(OrionTheme.borderDim, lineWidth: 0.5))
                .clipShape(RoundedRectangle(cornerRadius: 8))
            }
            Spacer(minLength: 46)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func toolRow(_ row: CodexChatRow) -> some View {
        let complete = row.toolStatus == "complete"
        let output = row.resultText ?? row.resultDetails ?? ""
        return HStack(alignment: .bottom, spacing: 8) {
            AgentSigilView(connection.sessionType, size: 24)
            VStack(alignment: .leading, spacing: 5) {
                Text(complete ? "Tool finished" : "Tool running")
                    .font(.system(size: 11))
                    .foregroundStyle(OrionTheme.textDim)
                VStack(alignment: .leading, spacing: 0) {
                    HStack(spacing: 10) {
                        Text(toolIcon(row.label))
                            .font(.system(size: 13, weight: .bold, design: .monospaced))
                            .foregroundStyle(OrionTheme.accentGreen)
                            .frame(width: 28, height: 28)
                            .background(OrionTheme.accentGreen.opacity(0.13))
                            .overlay(RoundedRectangle(cornerRadius: 9).stroke(OrionTheme.accentGreen.opacity(0.28), lineWidth: 0.5))
                            .clipShape(RoundedRectangle(cornerRadius: 9))
                        VStack(alignment: .leading, spacing: 2) {
                            Text(row.label.isEmpty ? "Tool" : row.label)
                                .font(.system(size: 13, weight: .semibold))
                                .foregroundStyle(OrionTheme.textPrimary)
                            if let toolUseId = row.toolUseId {
                                Text(shortID(toolUseId))
                                    .font(.system(size: 10, design: .monospaced))
                                    .foregroundStyle(OrionTheme.textDim)
                            }
                        }
                        Spacer()
                        Text(complete ? "complete" : "running")
                            .font(.system(size: 10, design: .monospaced))
                            .foregroundStyle(complete ? OrionTheme.accentGreen : OrionTheme.accentBlue)
                            .padding(.horizontal, 7)
                            .padding(.vertical, 4)
                            .overlay(Capsule().stroke((complete ? OrionTheme.accentGreen : OrionTheme.accentBlue).opacity(0.42), lineWidth: 0.5))
                    }
                    .padding(12)

                    if !row.text.isEmpty {
                        Text(row.text)
                            .font(.system(size: 12, design: .monospaced))
                            .foregroundStyle(OrionTheme.textSecondary)
                            .textSelection(.enabled)
                            .fixedSize(horizontal: false, vertical: true)
                            .padding(12)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(OrionTheme.bgTerminal.opacity(0.72))
                    }

                    if let details = row.details, !details.isEmpty {
                        detailsView(details)
                            .padding(.horizontal, 12)
                            .padding(.vertical, 8)
                    }

                    if !output.isEmpty {
                        DisclosureGroup {
                            Text(prettyDetails(output))
                                .font(.system(size: 12, design: .monospaced))
                                .foregroundStyle(OrionTheme.textSecondary)
                                .textSelection(.enabled)
                                .fixedSize(horizontal: false, vertical: true)
                                .padding(.top, 6)
                        } label: {
                            Text(toolLooksLikeCommand(row.label) ? "Output" : "Result")
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundStyle(OrionTheme.textDim)
                        }
                        .tint(OrionTheme.textDim)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 8)
                        .overlay(alignment: .top) { OrionTheme.borderDim.frame(height: 0.5) }
                    }
                }
                .frame(maxWidth: 340, alignment: .leading)
                .background(OrionTheme.bgSurface)
                .overlay(RoundedRectangle(cornerRadius: 16, style: .continuous).stroke(OrionTheme.accentGreen.opacity(0.24), lineWidth: 0.7))
                .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
            }
            Spacer(minLength: 22)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func reasoningRow(_ row: CodexChatRow) -> some View {
        HStack(alignment: .bottom, spacing: 8) {
            AgentSigilView(connection.sessionType, size: 24)
            VStack(alignment: .leading, spacing: 5) {
                Text("Reasoning")
                    .font(.system(size: 11))
                    .foregroundStyle(OrionTheme.textDim)
                DisclosureGroup {
                    Text(row.text.isEmpty ? "Reasoning in progress." : row.text)
                        .font(.system(size: 13))
                        .foregroundStyle(OrionTheme.textSecondary)
                        .textSelection(.enabled)
                        .fixedSize(horizontal: false, vertical: true)
                        .padding(.top, 8)
                } label: {
                    HStack(spacing: 8) {
                        Circle()
                            .fill(OrionTheme.accentPurple)
                            .frame(width: 7, height: 7)
                        Text(reasoningActivityTitle(row.text))
                            .font(.system(size: 13))
                            .foregroundStyle(OrionTheme.textSecondary)
                    }
                }
                .tint(OrionTheme.textDim)
                .padding(12)
                .frame(maxWidth: 340, alignment: .leading)
                .background(OrionTheme.bgSurface)
                .overlay(RoundedRectangle(cornerRadius: 14, style: .continuous).stroke(OrionTheme.accentPurple.opacity(0.24), lineWidth: 0.7))
                .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            }
            Spacer(minLength: 22)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func permissionRow(_ row: CodexChatRow) -> some View {
        let state = row.permissionState ?? "waiting"
        let resolved = state == "submitted" || state == "answered"
        let accent = resolved ? OrionTheme.accentGreen : OrionTheme.accentYellow
        let questionText = row.text.isEmpty ? (row.details ?? "") : row.text
        let answerDisplay: String = {
            if let id = row.toolUseId, let optimistic = submittedAnswers[id], !optimistic.isEmpty {
                return optimistic
            }
            if let stored = row.answerText, !stored.isEmpty { return stored }
            return state == "answered" ? "Answer delivered." : "Waiting for the session to continue."
        }()
        return HStack(alignment: .bottom, spacing: 8) {
            AgentSigilView(connection.sessionType, size: 24)
            VStack(alignment: .leading, spacing: 5) {
                Text(resolved ? "\(assistantName) answered" : "\(assistantName) needs input")
                    .font(.system(size: 11))
                    .foregroundStyle(OrionTheme.textDim)
                VStack(alignment: .leading, spacing: 10) {
                    HStack(spacing: 10) {
                        Text(resolved ? "✓" : "?")
                            .font(.system(size: 13, weight: .bold, design: .monospaced))
                            .foregroundStyle(accent)
                            .frame(width: 28, height: 28)
                            .background(accent.opacity(0.13))
                            .overlay(RoundedRectangle(cornerRadius: 9).stroke(accent.opacity(0.34), lineWidth: 0.5))
                            .clipShape(RoundedRectangle(cornerRadius: 9))
                        VStack(alignment: .leading, spacing: 2) {
                            Text(row.label.isEmpty ? "Question" : row.label)
                                .font(.system(size: 13, weight: .semibold))
                                .foregroundStyle(OrionTheme.textPrimary)
                            if !resolved {
                                Text(questionText.isEmpty ? "The session is waiting for your answer." : questionText)
                                    .font(.system(size: 11))
                                    .foregroundStyle(OrionTheme.textDim)
                                    .lineLimit(2)
                            }
                        }
                        Spacer(minLength: 0)
                    }
                    if resolved {
                        VStack(alignment: .leading, spacing: 8) {
                            if !questionText.isEmpty {
                                qaBlock(label: "Question", text: questionText, color: OrionTheme.textSecondary, accent: nil)
                            }
                            qaBlock(label: "Your answer", text: answerDisplay, color: OrionTheme.textPrimary, accent: accent)
                        }
                    } else {
                        Text(row.details ?? row.text)
                            .font(.system(size: 13))
                            .foregroundStyle(OrionTheme.textSecondary)
                            .fixedSize(horizontal: false, vertical: true)
                        if let toolUseId = row.toolUseId {
                            HStack(alignment: .bottom, spacing: 8) {
                                TextField("Answer \(assistantName)...", text: Binding(
                                    get: { answers[toolUseId] ?? "" },
                                    set: { answers[toolUseId] = $0 }
                                ), axis: .vertical)
                                .textFieldStyle(.plain)
                                .font(.system(size: 14))
                                .foregroundStyle(OrionTheme.textPrimary)
                                .padding(8)
                                .background(OrionTheme.bgPrimary)
                                .overlay(RoundedRectangle(cornerRadius: 8).stroke(OrionTheme.border, lineWidth: 0.5))

                                Button("Send") {
                                    let text = (answers[toolUseId] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                                    guard !text.isEmpty else { return }
                                    submittedAnswers[toolUseId] = text
                                    answers[toolUseId] = ""
                                    connection.answer(toolUseId: toolUseId, text: text)
                                }
                                .buttonStyle(.borderedProminent)
                                .tint(OrionTheme.accentBlue)
                            }
                        }
                    }
                }
                .padding(12)
                .frame(maxWidth: 340, alignment: .leading)
                .background(accent.opacity(0.06))
                .overlay(RoundedRectangle(cornerRadius: 16, style: .continuous).stroke(accent.opacity(0.42), lineWidth: 0.7))
                .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
            }
            Spacer(minLength: 22)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    @ViewBuilder
    private func qaBlock(label: String, text: String, color: Color, accent: Color?) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(label.uppercased())
                .font(.system(size: 10, weight: .semibold, design: .monospaced))
                .foregroundStyle(OrionTheme.textDim)
                .tracking(0.4)
            HStack(alignment: .top, spacing: 8) {
                if let accent {
                    Rectangle()
                        .fill(accent.opacity(0.55))
                        .frame(width: 2)
                        .frame(maxHeight: .infinity)
                }
                Text(text)
                    .font(.system(size: 13))
                    .foregroundStyle(color)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
    }

    private var composer: some View {
        VStack(spacing: 7) {
            if connection.queuedMessageCount > 0 {
                HStack(spacing: 8) {
                    Image(systemName: "clock.arrow.circlepath")
                        .font(.system(size: 12, weight: .semibold))
                    Text(connection.queuedMessageCount == 1 ? "Message queued. It will send when chat reconnects." : "\(connection.queuedMessageCount) messages queued. They will send when chat reconnects.")
                        .font(.system(size: 12, weight: .medium))
                    Spacer(minLength: 0)
                }
                .foregroundStyle(OrionTheme.accentYellow)
                .padding(.horizontal, 12)
                .padding(.vertical, 7)
                .background(OrionTheme.accentYellow.opacity(0.08))
                .clipShape(Capsule())
            }

            if isAssistantRunning {
                workingIndicator
                    .transition(.opacity.combined(with: .move(edge: .bottom)))
            }

            if !pendingImages.isEmpty {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 8) {
                        ForEach(pendingImages) { image in
                            pendingImageChip(image)
                        }
                    }
                    .padding(.horizontal, 2)
                }
            }

            VStack(alignment: .leading, spacing: 7) {
                TextField("Message \(assistantName)...", text: $input, axis: .vertical)
                    .textFieldStyle(.plain)
                    .font(.system(size: 16))
                    .foregroundStyle(OrionTheme.textPrimary)
                    .lineLimit(1...5)
                    .focused($composerFocused)
                    .padding(.horizontal, 6)
                    .padding(.top, 4)

                HStack(spacing: 6) {
                    PhotosPicker(selection: $selectedPhotoItems, maxSelectionCount: 4, matching: .images) {
                        composerIcon(systemName: isLoadingPhotos ? "photo.badge.clock" : "photo", isActive: isLoadingPhotos, activeColor: OrionTheme.accentBlue)
                    }
                    .disabled(isLoadingPhotos)
                    .buttonStyle(.plain)

                    chatMicButton
                    chatVoiceModeButton
                    chatSpeakerButton

                    if state.voiceModeEnabled {
                        Text("Voice")
                            .font(.system(size: 11, weight: .semibold))
                            .foregroundStyle(OrionTheme.accentPurple)
                    }

                    Spacer(minLength: 8)

                    Button {
                        sendComposer()
                    } label: {
                        Image(systemName: "arrow.up")
                            .font(.system(size: 16, weight: .bold))
                            .foregroundStyle(canSend ? .black : OrionTheme.textDim)
                            .frame(width: 40, height: 40)
                            .background(canSend ? OrionTheme.accentBlue : OrionTheme.bgActive)
                            .clipShape(Circle())
                    }
                    .disabled(!canSend)
                    .buttonStyle(.plain)
                }
            }
            .padding(8)
            .background(OrionTheme.bgSurface.opacity(0.96))
            .clipShape(RoundedRectangle(cornerRadius: 24, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 24, style: .continuous).stroke(OrionTheme.borderDim, lineWidth: 0.5))
        }
        .padding(.horizontal, 10)
        .padding(.top, 8)
        .padding(.bottom, 8)
        .background(OrionTheme.bgPrimary.opacity(0.98))
        .simultaneousGesture(keyboardDismissGesture)
    }

    private var workingIndicator: some View {
        HStack(spacing: 8) {
            TypingDotsView()
            Text(workingLabel)
                .font(.system(size: 12, weight: .medium))
                .foregroundStyle(OrionTheme.textSecondary)
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 7)
        .background(assistantColor.opacity(0.08))
        .clipShape(Capsule())
    }

    private var canSend: Bool {
        !input.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || !pendingImages.isEmpty
    }

    private var keyboardDismissGesture: some Gesture {
        DragGesture(minimumDistance: 12)
            .onChanged { value in
                guard !dismissedKeyboardDuringDrag else { return }
                guard value.translation.height > 18 else { return }
                guard value.translation.height > abs(value.translation.width) else { return }
                dismissedKeyboardDuringDrag = true
                dismissKeyboard()
            }
            .onEnded { _ in
                dismissedKeyboardDuringDrag = false
            }
    }

    private func dismissKeyboard() {
        composerFocused = false
        UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
    }

    private func scrollToChatBottom(_ proxy: ScrollViewProxy, delay: TimeInterval = 0, animated: Bool = true) {
        let update = {
            if animated {
                withAnimation(.easeOut(duration: 0.2)) {
                    proxy.scrollTo(chatBottomID, anchor: .bottom)
                }
            } else {
                proxy.scrollTo(chatBottomID, anchor: .bottom)
            }
        }
        guard delay > 0 else {
            update()
            return
        }
        DispatchQueue.main.asyncAfter(deadline: .now() + delay) {
            update()
        }
    }

    private var chatMicButton: some View {
        Button {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            if state.speech.isListening {
                state.speech.stopDictation()
                return
            }
            if !state.speech.isAuthorized {
                state.speech.requestAuthorization()
                return
            }
            state.speech.onDictationResult = { text in
                let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
                guard !trimmed.isEmpty else { return }
                Task { @MainActor in
                    if input.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                        input = trimmed
                    } else {
                        input += " " + trimmed
                    }
                }
            }
            state.speech.startDictation()
        } label: {
            composerIcon(systemName: state.speech.isListening ? "mic.fill" : "mic", isActive: state.speech.isListening, activeColor: OrionTheme.accentRed)
        }
        .buttonStyle(.plain)
    }

    private var chatVoiceModeButton: some View {
        Button {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            state.toggleVoiceMode()
        } label: {
            composerIcon(systemName: state.voiceModeEnabled ? "waveform.circle.fill" : "waveform.circle", isActive: state.voiceModeEnabled, activeColor: OrionTheme.accentPurple)
        }
        .buttonStyle(.plain)
    }

    private var chatSpeakerButton: some View {
        Button {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            if state.speech.isSpeaking {
                state.speech.stopSpeaking()
                return
            }
            if let text = state.lastVoiceText, !text.isEmpty {
                let rate = UserDefaults.standard.double(forKey: "ttsRate")
                state.speech.speakResponse(text, rate: Float(rate > 0 ? rate : 0.52))
            }
        } label: {
            composerIcon(
                systemName: state.speech.isSpeaking ? "speaker.wave.3.fill" : "speaker.wave.2",
                isActive: state.speech.isSpeaking,
                activeColor: OrionTheme.accentBlue,
                isDisabled: state.lastVoiceText == nil && !state.speech.isSpeaking
            )
        }
        .buttonStyle(.plain)
    }

    private func composerIcon(systemName: String, isActive: Bool = false, activeColor: Color = OrionTheme.accentBlue, isDisabled: Bool = false) -> some View {
        Image(systemName: systemName)
            .font(.system(size: 17, weight: .semibold))
            .foregroundStyle(isDisabled ? OrionTheme.textDim : isActive ? activeColor : OrionTheme.textSecondary)
            .frame(width: 34, height: 34)
            .contentShape(Rectangle())
    }

    private func sendComposer() {
        let text = input.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty || !pendingImages.isEmpty else { return }
        let attachments = pendingImages.map(\.payload)
        input = ""
        pendingImages = []
        connection.sendInput(text, attachments: attachments)
    }

    private func pendingImageChip(_ image: PendingChatImage) -> some View {
        ZStack(alignment: .topTrailing) {
            HStack(spacing: 8) {
                if let preview = image.preview {
                    Image(uiImage: preview)
                        .resizable()
                        .scaledToFill()
                        .frame(width: 42, height: 42)
                        .clipShape(RoundedRectangle(cornerRadius: 7))
                } else {
                    Image(systemName: "photo")
                        .font(.system(size: 16))
                        .foregroundStyle(OrionTheme.accentBlue)
                        .frame(width: 42, height: 42)
                        .background(OrionTheme.bgActive)
                        .clipShape(RoundedRectangle(cornerRadius: 7))
                }
                Text(image.name)
                    .font(.system(size: 12))
                    .foregroundStyle(OrionTheme.textSecondary)
                    .lineLimit(1)
            }
            .padding(6)
            .padding(.trailing, 18)
            .background(OrionTheme.bgSurface)
            .overlay(RoundedRectangle(cornerRadius: 8).stroke(OrionTheme.border, lineWidth: 0.5))
            .clipShape(RoundedRectangle(cornerRadius: 8))

            Button {
                pendingImages.removeAll { $0.id == image.id }
            } label: {
                Image(systemName: "xmark.circle.fill")
                    .font(.system(size: 17))
                    .foregroundStyle(OrionTheme.textDim)
                    .background(OrionTheme.bgSurface)
                    .clipShape(Circle())
            }
            .offset(x: 4, y: -4)
        }
    }

    private func loadPhotoAttachments(_ items: [PhotosPickerItem]) async {
        isLoadingPhotos = true
        defer {
            isLoadingPhotos = false
            selectedPhotoItems = []
        }
        var loaded: [PendingChatImage] = []
        for (index, item) in items.prefix(4).enumerated() {
            guard let data = try? await item.loadTransferable(type: Data.self),
                  let normalized = normalizedImageData(data) else {
                continue
            }
            let name = "orion-image-\(Int(Date().timeIntervalSince1970))-\(index + 1).jpg"
            loaded.append(PendingChatImage(
                name: name,
                mimeType: "image/jpeg",
                data: normalized.base64EncodedString(),
                preview: UIImage(data: normalized)
            ))
        }
        if !loaded.isEmpty {
            pendingImages = Array((pendingImages + loaded).suffix(6))
        }
    }

    private func normalizedImageData(_ data: Data) -> Data? {
        guard let image = UIImage(data: data) else { return nil }
        let maxDimension: CGFloat = 1600
        let longest = max(image.size.width, image.size.height)
        let scale = longest > maxDimension ? maxDimension / longest : 1
        let targetSize = CGSize(width: image.size.width * scale, height: image.size.height * scale)

        let renderer = UIGraphicsImageRenderer(size: targetSize)
        let rendered = renderer.image { _ in
            image.draw(in: CGRect(origin: .zero, size: targetSize))
        }
        return rendered.jpegData(compressionQuality: 0.82)
    }

    @ViewBuilder
    private func messageBubble(_ row: CodexChatRow) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            if !row.attachments.isEmpty {
                attachmentList(row.attachments)
            }
            if !row.text.isEmpty {
                Text(row.text)
                    .font(.system(size: 15))
                    .fontWeight(row.type == "user" ? .medium : .regular)
                    .foregroundStyle(row.type == "user" ? Color(hex: 0x0B1B3D) : row.type == "error" ? OrionTheme.accentRed : OrionTheme.textPrimary)
                    .textSelection(.enabled)
                    .fixedSize(horizontal: false, vertical: true)
            }
            if let details = row.details, !details.isEmpty {
                detailsView(details)
            }
            if row.type == "permission_request", let toolUseId = row.toolUseId {
                HStack(alignment: .bottom, spacing: 8) {
                    TextField("Answer \(assistantName)...", text: Binding(
                        get: { answers[toolUseId] ?? "" },
                        set: { answers[toolUseId] = $0 }
                    ), axis: .vertical)
                    .textFieldStyle(.plain)
                    .font(.system(size: 14))
                    .foregroundStyle(OrionTheme.textPrimary)
                    .padding(8)
                    .background(OrionTheme.bgPrimary)
                    .overlay(RoundedRectangle(cornerRadius: 8).stroke(OrionTheme.border, lineWidth: 0.5))

                    Button("Send") {
                        let text = (answers[toolUseId] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                        guard !text.isEmpty else { return }
                        submittedAnswers[toolUseId] = text
                        answers[toolUseId] = ""
                        connection.answer(toolUseId: toolUseId, text: text)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(OrionTheme.accentBlue)
                }
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .frame(maxWidth: 320, alignment: .leading)
        .background(rowBackground(row.type))
        .overlay(RoundedRectangle(cornerRadius: row.type == "user" ? 18 : 14, style: .continuous).stroke(rowBorder(row.type), lineWidth: 0.5))
        .clipShape(RoundedRectangle(cornerRadius: row.type == "user" ? 18 : 14, style: .continuous))
    }

    private func attachmentList(_ attachments: [ChatAttachmentPayload]) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            ForEach(Array(attachments.enumerated()), id: \.offset) { _, attachment in
                HStack(spacing: 7) {
                    Image(systemName: "photo")
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundStyle(OrionTheme.accentBlue)
                    Text(attachmentDisplayName(attachment))
                        .font(.system(size: 12))
                        .foregroundStyle(OrionTheme.textSecondary)
                        .lineLimit(1)
                }
                .padding(.horizontal, 9)
                .padding(.vertical, 6)
                .background(OrionTheme.accentBlue.opacity(0.1))
                .overlay(RoundedRectangle(cornerRadius: 8).stroke(OrionTheme.accentBlue.opacity(0.35), lineWidth: 0.5))
                .clipShape(RoundedRectangle(cornerRadius: 8))
            }
        }
    }

    @ViewBuilder
    private func activityRow(_ row: CodexChatRow) -> some View {
        VStack(spacing: 8) {
            HStack(spacing: 7) {
                Circle()
                    .fill(activityDotColor(row.type))
                    .frame(width: 6, height: 6)
                Text(activityLabel(row))
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundStyle(row.type == "error" ? OrionTheme.accentRed : OrionTheme.textSecondary)
                if !row.text.isEmpty {
                    Text(row.text)
                        .font(.system(size: 12))
                        .foregroundStyle(row.type == "error" ? OrionTheme.accentRed : OrionTheme.textDim)
                        .lineLimit(3)
                }
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 6)
            .background(row.type == "error" ? OrionTheme.accentRed.opacity(0.1) : OrionTheme.bgSecondary.opacity(0.9))
            .overlay(RoundedRectangle(cornerRadius: 8).stroke(rowBorder(row.type), lineWidth: 0.5))
            .clipShape(RoundedRectangle(cornerRadius: 8))

            if let details = row.details, !details.isEmpty {
                detailsView(details)
                    .frame(maxWidth: 320, alignment: .leading)
            }
        }
        .frame(maxWidth: .infinity, alignment: .center)
    }

    @ViewBuilder
    private func detailsView(_ details: String) -> some View {
        DisclosureGroup {
            Text(prettyDetails(details))
                .font(.system(size: 12, design: .monospaced))
                .foregroundStyle(OrionTheme.textSecondary)
                .textSelection(.enabled)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, 6)
        } label: {
            Text("Details")
                .font(.system(size: 11, design: .monospaced))
                .foregroundStyle(OrionTheme.textDim)
        }
        .tint(OrionTheme.textDim)
    }

    private func isActivityRow(_ row: CodexChatRow) -> Bool {
        switch row.type {
        case "user", "assistant", "permission_request", "plan", "loading", "tool", "thinking_delta": return false
        default: return true
        }
    }

    private func rowBackground(_ type: String) -> Color {
        switch type {
        case "user": return OrionTheme.accentBlue
        case "permission_request": return OrionTheme.accentYellow.opacity(0.08)
        default: return OrionTheme.bgSurface
        }
    }

    private func rowBorder(_ type: String) -> Color {
        switch type {
        case "permission_request": return OrionTheme.accentYellow.opacity(0.65)
        case "tool", "tool_result": return OrionTheme.accentBlue.opacity(0.45)
        case "error": return OrionTheme.accentRed.opacity(0.65)
        case "user": return OrionTheme.accentBlue.opacity(0.45)
        default: return OrionTheme.borderDim
        }
    }

    private func activityDotColor(_ type: String) -> Color {
        switch type {
        case "tool", "tool_result", "thinking_delta": return OrionTheme.accentBlue
        case "error": return OrionTheme.accentRed
        default: return OrionTheme.textDim
        }
    }

    private func activityLabel(_ row: CodexChatRow) -> String {
        switch row.type {
        case "tool": return row.label.isEmpty ? "Using tool" : "Using \(row.label)"
        case "tool_result": return row.label.isEmpty ? "Tool finished" : "\(row.label) finished"
        case "thinking_delta": return "Thinking"
        case "result": return row.label
        case "error": return "Error"
        case "system": return "System"
        default: return row.label
        }
    }
}

private struct PlanReviewView: View {
    let plan: CodexChatRow
    let assistantName: String
    let accentColor: Color
    let onClose: () -> Void
    let onReviewDiff: () -> Void
    let onApprove: () -> Void

    private var markdown: String {
        let value = plan.details ?? plan.text
        return value.isEmpty ? "Plan ready" : value
    }

    private var reviewBlocks: [PlanReviewBlock] {
        planReviewBlocks(markdown)
    }

    private var isWaiting: Bool {
        plan.planState != "approved"
    }

    var body: some View {
        VStack(spacing: 0) {
            HStack(alignment: .center, spacing: 12) {
                VStack(alignment: .leading, spacing: 4) {
                    Text("\(assistantName) plan")
                        .font(.system(size: 11, weight: .semibold))
                        .foregroundStyle(OrionTheme.textDim)
                    Text(plan.text.isEmpty ? planTitle(markdown) : plan.text)
                        .font(.system(size: 17, weight: .semibold))
                        .foregroundStyle(OrionTheme.textPrimary)
                        .lineLimit(2)
                }
                Spacer()
                Button("Minimize", action: onClose)
                    .buttonStyle(.bordered)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 14)
            .background(OrionTheme.bgSecondary)
            .overlay(alignment: .bottom) { OrionTheme.border.frame(height: 0.5) }

            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    ForEach(Array(reviewBlocks.enumerated()), id: \.offset) { _, block in
                        PlanReviewBlockRow(block: block)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(16)
            }
            .background(OrionTheme.bgPrimary)

            HStack(spacing: 10) {
                Button("Back to chat", action: onClose)
                    .buttonStyle(.bordered)
                Button("Review diff", action: onReviewDiff)
                    .buttonStyle(.bordered)
                if isWaiting {
                    Button("Approve & run", action: onApprove)
                        .buttonStyle(.borderedProminent)
                        .tint(accentColor)
                }
            }
            .frame(maxWidth: .infinity, alignment: .trailing)
            .padding(14)
            .background(OrionTheme.bgSecondary)
            .overlay(alignment: .top) { OrionTheme.border.frame(height: 0.5) }
        }
        .background(OrionTheme.bgPrimary)
    }
}

private struct PlanReviewBlock {
    let kind: String
    let text: String
}

private struct PlanReviewBlockRow: View {
    let block: PlanReviewBlock

    var body: some View {
        switch block.kind {
        case "heading":
            Text(block.text)
                .font(.system(size: 15, weight: .semibold))
                .foregroundStyle(OrionTheme.textPrimary)
                .textSelection(.enabled)
                .padding(.top, 2)
        case "step":
            HStack(alignment: .top, spacing: 9) {
                Image(systemName: block.text.lowercased().contains("[completed]") ? "checkmark.circle" : "circle")
                    .font(.system(size: 14, weight: .medium))
                    .foregroundStyle(OrionTheme.accentBlue)
                    .frame(width: 20, height: 20)
                Text(block.text)
                    .font(.system(size: 14))
                    .foregroundStyle(OrionTheme.textSecondary)
                    .textSelection(.enabled)
                    .fixedSize(horizontal: false, vertical: true)
            }
        default:
            Text(block.text)
                .font(.system(size: 14))
                .foregroundStyle(OrionTheme.textSecondary)
                .textSelection(.enabled)
                .fixedSize(horizontal: false, vertical: true)
        }
    }
}

private struct TypingDotsView: View {
    @State private var activeDot = 0
    private let timer = Timer.publish(every: 0.28, on: .main, in: .common).autoconnect()

    var body: some View {
        HStack(spacing: 4) {
            ForEach(0..<3, id: \.self) { index in
                Circle()
                    .fill(OrionTheme.textDim)
                    .frame(width: 5, height: 5)
                    .opacity(activeDot == index ? 1 : 0.35)
                    .offset(y: activeDot == index ? -2 : 0)
                    .animation(.easeInOut(duration: 0.18), value: activeDot)
            }
        }
        .onReceive(timer) { _ in
            activeDot = (activeDot + 1) % 3
        }
    }
}

private func mergeChatRows(_ messages: [CodexChatMessage], assistantName: String) -> [CodexChatRow] {
    var rows: [CodexChatRow] = []
    var planApprovedSinceLastUser = false
    for message in messages {
        if message.type == "status" { continue }
        if message.type == "user" {
            planApprovedSinceLastUser = false
        }
        if message.type == "permission_submitted" {
            updatePermissionRow(&rows, update: message, state: "submitted")
            continue
        }
        if message.type == "permission_resolved" {
            updatePermissionRow(&rows, update: message, state: "answered")
            continue
        }
        if message.type == "plan_resolved" {
            markLatestPlanRowApproved(&rows)
            planApprovedSinceLastUser = true
            continue
        }
        if shouldHideChatMessage(message) { continue }
        if message.type == "stream_delta" {
            if let lastIndex = rows.indices.last, rows[lastIndex].type == "assistant", rows[lastIndex].id.hasPrefix("assistant-stream") {
                rows[lastIndex].text += message.text ?? ""
            } else {
                rows.append(CodexChatRow(id: "assistant-stream-\(message.id)", type: "assistant", label: assistantName, text: message.text ?? "", details: nil, toolUseId: nil, planPath: nil, attachments: [], resultText: nil, resultDetails: nil, toolStatus: nil))
            }
            continue
        }
        if message.type == "thinking_delta" {
            if let lastIndex = rows.indices.last, rows[lastIndex].type == "thinking_delta" {
                rows[lastIndex].text += message.text ?? ""
            } else {
                rows.append(CodexChatRow(id: message.id, type: message.type, label: "Thinking", text: message.text ?? "", details: nil, toolUseId: nil, planPath: nil, attachments: [], resultText: nil, resultDetails: nil, toolStatus: nil))
            }
            continue
        }
        if message.type == "tool_result" {
            if let index = findOpenToolRow(rows, result: message) {
                rows[index].resultText = message.text
                rows[index].resultDetails = message.details
                rows[index].toolStatus = "complete"
                continue
            }
        }
        if message.type == "tool" {
            rows.append(CodexChatRow(
                id: message.id,
                type: message.type,
                label: chatLabel(message, assistantName: assistantName),
                text: message.text ?? "",
                details: message.details,
                toolUseId: message.toolUseId,
                planPath: message.planPath,
                attachments: message.attachments ?? [],
                resultText: nil,
                resultDetails: nil,
                toolStatus: "running"
            ))
            continue
        }
        if message.type == "permission_request" {
            rows.append(CodexChatRow(
                id: message.id,
                type: message.type,
                label: chatLabel(message, assistantName: assistantName),
                text: message.text ?? "",
                details: message.details,
                toolUseId: message.toolUseId,
                planPath: message.planPath,
                attachments: message.attachments ?? [],
                resultText: nil,
                resultDetails: nil,
                toolStatus: nil,
                permissionState: "waiting"
            ))
            continue
        }
        if message.type == "plan" {
            rows.append(CodexChatRow(
                id: message.id,
                type: message.type,
                label: chatLabel(message, assistantName: assistantName),
                text: message.text ?? "",
                details: message.details,
                toolUseId: message.toolUseId,
                planPath: message.planPath,
                attachments: message.attachments ?? [],
                resultText: nil,
                resultDetails: nil,
                toolStatus: nil,
                planState: planApprovedSinceLastUser ? "approved" : "waiting"
            ))
            continue
        }
        rows.append(CodexChatRow(
            id: message.id,
            type: message.type,
            label: chatLabel(message, assistantName: assistantName),
            text: message.text ?? "",
            details: message.details,
            toolUseId: message.toolUseId,
            planPath: message.planPath,
            attachments: message.attachments ?? [],
            resultText: nil,
            resultDetails: nil,
            toolStatus: nil
        ))
    }
    return rows
}

private func updatePermissionRow(_ rows: inout [CodexChatRow], update: CodexChatMessage, state: String) {
    let requestedToolUseId = update.toolUseId?.trimmingCharacters(in: .whitespacesAndNewlines)
    for index in rows.indices.reversed() {
        guard rows[index].type == "permission_request" else { continue }
        if let requestedToolUseId, !requestedToolUseId.isEmpty, rows[index].toolUseId != requestedToolUseId { continue }
        resolvePermissionRow(&rows[index], update: update, state: state)
        return
    }
    guard let fallbackIndex = rows.indices.reversed().first(where: {
        rows[$0].type == "permission_request" && rows[$0].permissionState == "waiting"
    }) else { return }
    resolvePermissionRow(&rows[fallbackIndex], update: update, state: state)
}

private func resolvePermissionRow(_ row: inout CodexChatRow, update: CodexChatMessage, state: String) {
    row.permissionState = state
    if let text = update.text, !text.isEmpty, !isGenericPermissionAnswer(text) {
        row.answerText = text
    }
}

private func markLatestPlanRowApproved(_ rows: inout [CodexChatRow]) {
    guard let index = rows.indices.reversed().first(where: { rows[$0].type == "plan" }) else {
        return
    }
    rows[index].planState = "approved"
}

private func isGenericPermissionAnswer(_ text: String) -> Bool {
    let normalized = text.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    return normalized == "answered" || normalized == "answer submitted"
}

private func findOpenToolRow(_ rows: [CodexChatRow], result: CodexChatMessage) -> Int? {
    for index in rows.indices.reversed() {
        let row = rows[index]
        guard row.type == "tool" else { continue }
        guard row.resultText == nil && row.resultDetails == nil else { continue }
        if let toolUseId = result.toolUseId, row.toolUseId == toolUseId {
            return index
        }
        if result.toolUseId == nil && row.label == (result.toolName ?? "") {
            return index
        }
    }
    return nil
}

private func chatLabel(_ message: CodexChatMessage, assistantName: String) -> String {
    switch message.type {
    case "user": return "You"
    case "assistant": return assistantName
    case "tool": return message.toolName ?? "Tool"
    case "tool_result": return message.toolName ?? "Tool"
    case "permission_request": return message.toolName ?? "Question"
    case "plan": return "Plan"
    case "loading": return assistantName
    case "result": return message.subtype ?? "Finished"
    case "error": return "Error"
    case "system": return "System"
    default: return message.type
    }
}

private func attachmentDisplayName(_ attachment: ChatAttachmentPayload) -> String {
    if let name = attachment.name, !name.isEmpty {
        return name
    }
    if let path = attachment.path, !path.isEmpty {
        return (path as NSString).lastPathComponent
    }
    return "Image"
}

private func shouldHideChatMessage(_ message: CodexChatMessage) -> Bool {
    if message.type == "permission_resolved" { return true }
    if message.type == "permission_submitted" { return true }
    if message.type == "plan_resolved" { return true }
    if message.type == "result" {
        let value = (message.subtype ?? message.text ?? "").lowercased()
        return value.isEmpty || value == "completed" || value == "success" || value == "ok"
    }
    if message.type == "system" {
        let value = (message.text ?? "").lowercased()
        return value.contains("codex chat ready") || value.contains("claude chat ready")
    }
    return false
}

private func prettyDetails(_ details: String) -> String {
    guard let data = details.data(using: .utf8),
          let object = try? JSONSerialization.jsonObject(with: data),
          let pretty = try? JSONSerialization.data(withJSONObject: object, options: [.prettyPrinted]),
          let string = String(data: pretty, encoding: .utf8) else {
        return details
    }
    return string
}

private func planTitle(_ markdown: String) -> String {
    for line in markdown.components(separatedBy: .newlines) {
        let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty { continue }
        return trimmed.replacingOccurrences(of: #"^#+\s*"#, with: "", options: .regularExpression)
    }
    return "Plan ready"
}

private func planPreview(_ markdown: String) -> String {
    let lines = markdown
        .components(separatedBy: .newlines)
        .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
        .filter { !$0.isEmpty && !$0.hasPrefix("#") }
    let preview = lines.prefix(4).joined(separator: "\n")
    return preview.isEmpty ? "Review the plan before changes start." : preview
}

private func planInsights(_ markdown: String) -> (sections: Int, steps: Int) {
    let lines = markdown
        .components(separatedBy: .newlines)
        .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
        .filter { !$0.isEmpty }
    let sections = lines.filter { $0.range(of: #"^#{1,4}\s+"#, options: .regularExpression) != nil }.count
    let steps = lines.filter { $0.range(of: #"^([-*]|\d+\.)\s+"#, options: .regularExpression) != nil }.count
    return (sections, steps)
}

private func planSections(_ markdown: String) -> [String] {
    markdown
        .components(separatedBy: .newlines)
        .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
        .filter { $0.range(of: #"^#{1,4}\s+"#, options: .regularExpression) != nil }
        .map { $0.replacingOccurrences(of: #"^#{1,4}\s+"#, with: "", options: .regularExpression) }
        .filter { !$0.isEmpty }
}

private func planReviewBlocks(_ markdown: String) -> [PlanReviewBlock] {
    let blocks = markdown
        .components(separatedBy: .newlines)
        .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
        .compactMap { line -> PlanReviewBlock? in
            guard !line.isEmpty else { return nil }
            if line.range(of: #"^#{1,4}\s+"#, options: .regularExpression) != nil {
                let text = line.replacingOccurrences(of: #"^#{1,4}\s+"#, with: "", options: .regularExpression)
                return PlanReviewBlock(kind: "heading", text: text)
            }
            if line.range(of: #"^([-*]|\d+\.)\s+"#, options: .regularExpression) != nil {
                return PlanReviewBlock(kind: "step", text: cleanPlanPreviewLine(line))
            }
            return PlanReviewBlock(kind: "paragraph", text: line)
        }
    return blocks.isEmpty ? [PlanReviewBlock(kind: "paragraph", text: "Plan ready")] : blocks
}

private func cleanPlanPreviewLine(_ line: String) -> String {
    line
        .replacingOccurrences(of: #"^[-*]\s+"#, with: "", options: .regularExpression)
        .replacingOccurrences(of: #"^\d+\.\s+"#, with: "", options: .regularExpression)
}

private func chatSessionMetadata(_ messages: [CodexChatMessage]) -> ChatSessionMetadata? {
    let planApproved = messages.contains { $0.type == "plan_resolved" }
    for message in messages.reversed() where message.type == "system" {
        guard let details = message.details, let data = details.data(using: .utf8) else { continue }
        if let metadata = try? JSONDecoder().decode(ChatSessionMetadata.self, from: data) {
            if metadata.model != nil || metadata.reasoningEffort != nil || metadata.approvalPolicy != nil || metadata.sandboxMode != nil || metadata.permissionMode != nil || metadata.collaborationMode != nil || metadata.threadId != nil {
                if planApproved && (metadata.permissionMode == "plan" || metadata.collaborationMode == "plan") {
                    return ChatSessionMetadata(
                        provider: metadata.provider,
                        viewMode: metadata.viewMode,
                        model: metadata.model,
                        reasoningEffort: metadata.reasoningEffort,
                        approvalPolicy: metadata.approvalPolicy,
                        sandboxMode: metadata.sandboxMode,
                        permissionMode: "approved",
                        collaborationMode: metadata.collaborationMode,
                        threadId: metadata.threadId
                    )
                }
                return metadata
            }
        }
    }
    return nil
}

private func liveChatActivityItems(messages: [CodexChatMessage], rows: [CodexChatRow], assistantName: String) -> [MobileLiveActivityItem] {
    var items: [MobileLiveActivityItem] = []
    let statusMessage = messages.reversed().first { $0.type == "status" }
    let status = statusMessage?.status ?? "idle"
    let openPlan = openPlanMessage(messages)
    let hasWaitingPermission = rows.contains { $0.type == "permission_request" && $0.permissionState == "waiting" }

    switch status {
    case "starting":
        items.append(MobileLiveActivityItem(id: "status-starting", kind: "status", label: "Starting", value: assistantName))
    case "running":
        let value = compactActivityValue(statusMessage?.text ?? "\(assistantName) is working")
        items.append(MobileLiveActivityItem(id: "status-running", kind: "status", label: "Working", value: value))
    case "waiting_input":
        if hasWaitingPermission {
            items.append(MobileLiveActivityItem(id: "status-waiting", kind: "question", label: "Waiting", value: "needs your input"))
        } else if openPlan == nil {
            items.append(MobileLiveActivityItem(id: "status-paused", kind: "status", label: "Ready", value: nil))
        }
    default:
        break
    }

    let runningTools = rows.filter { $0.type == "tool" && $0.toolStatus == "running" }
    if let latestTool = runningTools.last {
        items.append(MobileLiveActivityItem(
            id: "tool-\(latestTool.id)",
            kind: "tool",
            label: "Using",
            value: compactActivityValue(latestTool.label.isEmpty ? latestTool.text : latestTool.label)
        ))
        if runningTools.count > 1 {
            items.append(MobileLiveActivityItem(id: "tool-count", kind: "tool", label: "\(runningTools.count) tools", value: "active"))
        }
    }

    let lastVisibleRow = rows.last
    if let lastReasoning = rows.reversed().first(where: { $0.type == "thinking_delta" }),
       status == "running" || lastVisibleRow?.id == lastReasoning.id {
        items.append(MobileLiveActivityItem(
            id: "reasoning-\(lastReasoning.id)",
            kind: "reasoning",
            label: "Reasoning",
            value: reasoningActivitySummary(lastReasoning.text)
        ))
    }

    if status == "running",
       let lastVisibleRow,
       lastVisibleRow.type == "assistant",
       lastVisibleRow.id.hasPrefix("assistant-stream-") {
        items.append(MobileLiveActivityItem(id: "stream-\(lastVisibleRow.id)", kind: "stream", label: "Streaming", value: "answer"))
    }

    if let plan = openPlan {
        let markdown = plan.details ?? ""
        let title = plan.text?.isEmpty == false ? plan.text! : planTitle(markdown)
        items.append(MobileLiveActivityItem(id: "plan-\(plan.id)", kind: "plan", label: "Plan ready", value: compactActivityValue(title)))
    }

    return Array(uniqueActivityItems(items).prefix(4))
}

private func uniqueActivityItems(_ items: [MobileLiveActivityItem]) -> [MobileLiveActivityItem] {
    var seen = Set<String>()
    return items.filter { item in
        let key = "\(item.kind):\(item.label):\(item.value ?? "")"
        guard !seen.contains(key) else { return false }
        seen.insert(key)
        return true
    }
}

private func openPlanMessage(_ messages: [CodexChatMessage]) -> CodexChatMessage? {
    var lastOpenPlan: CodexChatMessage?
    var planApprovedSinceLastUser = false
    for message in messages {
        switch message.type {
        case "user":
            lastOpenPlan = nil
            planApprovedSinceLastUser = false
        case "plan_resolved":
            lastOpenPlan = nil
            planApprovedSinceLastUser = true
        case "plan":
            if !planApprovedSinceLastUser {
                lastOpenPlan = message
            }
        default:
            break
        }
    }
    return lastOpenPlan
}

private func compactActivityValue(_ value: String) -> String {
    let collapsed = value
        .components(separatedBy: .whitespacesAndNewlines)
        .filter { !$0.isEmpty }
        .joined(separator: " ")
    guard collapsed.count > 48 else { return collapsed }
    return "\(collapsed.prefix(45))..."
}

private func reasoningActivitySummary(_ value: String) -> String {
    let firstLine = value
        .components(separatedBy: .newlines)
        .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
        .first { !$0.isEmpty } ?? "thinking"
    return compactActivityValue(firstLine)
}

private func reasoningActivityTitle(_ value: String) -> String {
    let summary = reasoningActivitySummary(value)
    return summary == "thinking" ? "Thinking through the plan" : summary
}

private func liveActivityColor(_ kind: String) -> Color {
    switch kind {
    case "tool": return OrionTheme.accentGreen
    case "reasoning": return OrionTheme.accentPurple
    case "plan", "question": return OrionTheme.accentYellow
    default: return OrionTheme.accentBlue
    }
}

private func approvalLabel(_ value: String?) -> String? {
    guard let value, !value.isEmpty else { return nil }
    if value == "never" { return "full access" }
    if value == "on-request" { return "ask first" }
    return value.replacingOccurrences(of: "_", with: " ")
}

private func sandboxLabel(_ value: String?) -> String? {
    guard let value, !value.isEmpty else { return nil }
    if value == "danger-full-access" { return "workspace + network" }
    return value.replacingOccurrences(of: "-", with: " ")
}

private func modeLabel(_ value: String?) -> String? {
    guard let value, !value.isEmpty else { return nil }
    switch value {
    case "bypassPermissions", "never":
        return "full access"
    case "dontAsk":
        return "don't ask"
    case "default":
        return "default"
    case "plan":
        return "plan"
    case "approved":
        return "approved"
    default:
        return value
            .replacingOccurrences(of: "_", with: " ")
            .replacingOccurrences(of: "-", with: " ")
    }
}

private func modelLabel(_ value: String) -> String {
    switch value {
    case "gpt-5.4-mini": return "GPT-5.4 Mini"
    case "gpt-5.3-codex": return "GPT-5.3 Codex"
    case "gpt-5.3-codex-spark": return "GPT-5.3 Codex Spark"
    default: return value.uppercased()
    }
}

private func reasoningLabel(_ value: String) -> String {
    switch value {
    case "xhigh": return "Extra high"
    default: return value.capitalized
    }
}

private func approvalPickerLabel(_ value: String) -> String {
    switch value {
    case "never": return "Full access"
    case "on-request": return "Ask first"
    case "on-failure": return "On failure"
    default: return value.capitalized
    }
}

private func sandboxPickerLabel(_ value: String) -> String {
    switch value {
    case "danger-full-access": return "Workspace + network"
    case "workspace-write": return "Workspace write"
    case "read-only": return "Read only"
    default: return value
    }
}

private func toolLooksLikeCommand(_ toolName: String) -> Bool {
    let value = toolName.lowercased()
    return value.contains("bash") || value.contains("command") || value.contains("shell")
}

private func toolIcon(_ toolName: String) -> String {
    let value = toolName.lowercased()
    if value.contains("bash") || value.contains("command") { return "$" }
    if value.contains("file") { return "±" }
    if value.contains("web") { return "⌕" }
    if value.contains("mcp") { return "◆" }
    return "∴"
}

private func shortID(_ id: String) -> String {
    guard id.count > 12 else { return id }
    return "\(id.prefix(6))…\(id.suffix(4))"
}
