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
            if !state.visibleTabs.isEmpty { TabStrip() }
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
                   connection.sessionId == activeSession.tmuxName {
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
                   connection.tmuxSession == activeSession.tmuxName {
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
                } else {
                    VStack(spacing: 12) {
                        Image(systemName: "terminal").font(.system(size: 40)).foregroundStyle(OrionTheme.textDim)
                        Text(emptyStateTitle).font(.subheadline).foregroundStyle(OrionTheme.textDim)
                        Button("Browse Workspaces") { state.showWorkspaces = true }.buttonStyle(.bordered).tint(OrionTheme.accentBlue)
                    }
                }
            }
            if state.activeSession != nil && !state.activeSessionShowsChat { TerminalToolbar() }
        }
        .background(OrionTheme.bgPrimary)
        .sheet(isPresented: Binding(get: { state.showWorkspaces }, set: { state.showWorkspaces = $0 })) {
            WorkspaceSheet().presentationDetents([.medium, .large]).presentationDragIndicator(.visible)
        }
        .sheet(isPresented: Binding(get: { state.showSettings }, set: { state.showSettings = $0 })) {
            SettingsView().presentationDetents([.medium]).presentationDragIndicator(.visible)
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

    private var emptyStateTitle: String {
        if let workspace = state.activeWorkspace {
            return "Open a session in \(workspace.name)"
        }
        return "Open a session from workspaces"
    }
}

// MARK: - Header with Project Switcher

struct HeaderBar: View {
    @Environment(AppState.self) private var state
    var body: some View {
        HStack(spacing: 12) {
            Button { state.showWorkspaces = true } label: {
                Image(systemName: "sidebar.left").font(.system(size: 18)).foregroundStyle(OrionTheme.textSecondary)
            }
            Spacer()

            // Project switcher in the header
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

            Spacer()
            Circle().fill(state.isConnected ? OrionTheme.accentGreen : OrionTheme.accentRed).frame(width: 8, height: 8)
            Button { state.showSettings = true } label: {
                Image(systemName: "gearshape").font(.system(size: 16)).foregroundStyle(OrionTheme.textSecondary)
            }
        }
        .padding(.horizontal, 12).frame(height: 48).background(OrionTheme.bgSecondary)
        .overlay(alignment: .bottom) { OrionTheme.border.frame(height: 0.5) }
    }

    @ViewBuilder
    private var titleView: some View {
        VStack(spacing: 1) {
            HStack(spacing: 4) {
                Text(state.projectInfo?.name ?? "Orion")
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundStyle(OrionTheme.textPrimary)
                if state.projects.count > 1 {
                    Image(systemName: "chevron.up.chevron.down")
                        .font(.system(size: 9, weight: .medium))
                        .foregroundStyle(OrionTheme.textDim)
                }
            }
            if let workspace = state.activeWorkspace {
                Text(workspaceSubtitle(workspace))
                    .font(.system(size: 11))
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
}

// MARK: - Workspace Sheet (native List with swipe-to-delete)

struct WorkspaceSheet: View {
    @Environment(AppState.self) private var state

    var body: some View {
        NavigationStack {
            List {
                ForEach(state.workspaces) { ws in
                    WorkspaceSection(workspace: ws)
                }
            }
            .listStyle(.insetGrouped)
            .scrollContentBackground(.hidden)
            .background(OrionTheme.bgPrimary)
            .navigationTitle("Workspaces")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Done") { state.showWorkspaces = false }.foregroundStyle(OrionTheme.accentBlue)
                }
                ToolbarItem(placement: .topBarLeading) {
                    Button { Task { await state.refreshSessions() } } label: {
                        Image(systemName: "arrow.clockwise").foregroundStyle(OrionTheme.accentBlue)
                    }
                }
            }
            .toolbarBackground(OrionTheme.bgSecondary, for: .navigationBar)
        }
    }
}

struct WorkspaceSection: View {
    @Environment(AppState.self) private var state
    let workspace: Workspace
    @State private var serverStatuses: [ServerStatus] = []
    @State private var loadingServers = false

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
                    HStack(spacing: 8) {
                        Text(sessionIcon(session.type)).font(.system(size: 14))
                            .foregroundStyle(sessionColor(session.type))
                        Text(session.label).font(.system(size: 14)).foregroundStyle(OrionTheme.textPrimary)
                        Spacer()
                        Image(systemName: "chevron.right").font(.system(size: 11)).foregroundStyle(OrionTheme.textDim)
                    }
                }
                .swipeActions(edge: .trailing, allowsFullSwipe: true) {
                    Button(role: .destructive) {
                        Task { await state.killSession(session) }
                    } label: {
                        Label("Kill", systemImage: "xmark.circle")
                    }
                }
                .listRowBackground(OrionTheme.bgSurface)
            }

            // Only show running servers
            ForEach(runningServers) { srv in
                HStack(spacing: 8) {
                    Circle().fill(OrionTheme.accentGreen).frame(width: 7, height: 7)
                    Text(srv.name.capitalized).font(.system(size: 14)).foregroundStyle(OrionTheme.textPrimary)
                    Spacer()
                    Text(":\(srv.port)").font(.system(size: 12, design: .monospaced)).foregroundStyle(OrionTheme.textDim)
                }
                .listRowBackground(OrionTheme.bgSurface)
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
                        Task {
                            try? await state.launchCodexChat(workspacePath: workspace.path)
                            state.showWorkspaces = false
                        }
                    } label: {
                        Label("Codex Chat", systemImage: "bubble.left.and.bubble.right")
                    }

                    Button {
                        Task {
                            try? await state.launchClaudeChat(workspacePath: workspace.path)
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
        } header: {
            Button {
                Task {
                    await state.activateWorkspace(workspace.path)
                    state.showWorkspaces = false
                }
            } label: {
                HStack(spacing: 6) {
                    Text(workspace.name).font(.system(size: 13, weight: .semibold))
                    if workspace.isMain {
                        Text("MAIN").font(.system(size: 9, weight: .bold)).padding(.horizontal, 5).padding(.vertical, 1)
                            .background(OrionTheme.accentBlue).foregroundStyle(.black).clipShape(RoundedRectangle(cornerRadius: 3))
                    }
                    Spacer()
                    if openTabCount > 0 {
                        Text("\(openTabCount) tab\(openTabCount == 1 ? "" : "s")")
                            .font(.system(size: 11))
                            .foregroundStyle(OrionTheme.textDim)
                    }
                    if !workspace.branch.isEmpty {
                        Text(workspace.branch).font(.system(size: 11)).foregroundStyle(OrionTheme.textDim)
                    }
                    Image(systemName: isActiveWorkspace ? "checkmark.circle.fill" : "circle")
                        .font(.system(size: 13))
                        .foregroundStyle(isActiveWorkspace ? OrionTheme.accentGreen : OrionTheme.textDim)
                }
            }
            .buttonStyle(.plain)
        }
        .onAppear { Task { serverStatuses = await state.getServerStatuses(workspace: workspace) } }
    }
}

// MARK: - Tabs

struct TabStrip: View {
    @Environment(AppState.self) private var state
    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 0) { ForEach(state.visibleTabs) { tab in TabPill(tab: tab, isActive: tab.id == state.activeTabId) } }
        }.frame(height: 36).background(OrionTheme.bgSecondary).overlay(alignment: .bottom) { OrionTheme.border.frame(height: 0.5) }
    }
}

struct TabPill: View {
    @Environment(AppState.self) private var state
    let tab: TerminalTab; let isActive: Bool
    var body: some View {
        HStack(spacing: 6) {
            Button { state.activateTab(tab.id) } label: {
                Text(tab.label).font(.system(size: 13)).foregroundStyle(isActive ? OrionTheme.textPrimary : OrionTheme.textDim)
                    .padding(.leading, 16)
                    .padding(.trailing, 2)
                    .frame(height: 36)
            }
            .buttonStyle(.plain)
            if isConvertibleSession(tab.type) {
                Button {
                    if let session = state.sessions.first(where: { $0.tmuxName == tab.tmuxSession }) {
                        Task { await state.convertSession(session) }
                    }
                } label: {
                    Image(systemName: "arrow.triangle.2.circlepath")
                        .font(.system(size: 10, weight: .medium))
                        .foregroundStyle(OrionTheme.textDim)
                        .frame(width: 24, height: 36)
                }
                .buttonStyle(.plain)
            }
            Button { state.requestKillSession(tab.tmuxSession) } label: {
                Image(systemName: "xmark")
                    .font(.system(size: 10, weight: .medium))
                    .foregroundStyle(OrionTheme.textDim)
                    .frame(width: 24, height: 36)
                    .padding(.trailing, 8)
            }
            .buttonStyle(.plain)
        }
        .frame(height: 36)
        .background(isActive ? OrionTheme.bgPrimary : .clear)
        .overlay(alignment: .trailing) { OrionTheme.borderDim.frame(width: 0.5) }
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

struct CodexChatView: View {
    @Environment(AppState.self) private var state
    let connection: CodexChatConnection
    @State private var input = ""
    @State private var answers: [String: String] = [:]
    @State private var selectedPhotoItems: [PhotosPickerItem] = []
    @State private var pendingImages: [PendingChatImage] = []
    @State private var isLoadingPhotos = false
    @State private var expandedPlan: CodexChatRow?

    private var assistantName: String { connection.displayName }
    private var assistantAvatar: String { connection.avatar }
    private var assistantColor: Color { sessionColor(connection.sessionType) }

    var body: some View {
        VStack(spacing: 0) {
            HStack {
                VStack(alignment: .leading, spacing: 2) {
                    Text(assistantName)
                        .font(.system(size: 15, weight: .semibold))
                        .foregroundStyle(OrionTheme.textPrimary)
                    Text(statusLabel)
                        .font(.system(size: 11))
                        .foregroundStyle(OrionTheme.textDim)
                }
                Spacer()
                HStack(spacing: 6) {
                    Circle()
                        .fill(connection.connectionState == .connected ? OrionTheme.accentGreen : OrionTheme.textDim)
                        .frame(width: 7, height: 7)
                    Text(connection.connectionState == .connected ? "Online" : "Offline")
                        .font(.system(size: 11, weight: .medium))
                        .foregroundStyle(connection.connectionState == .connected ? OrionTheme.textSecondary : OrionTheme.textDim)
                }
                .padding(.horizontal, 8)
                .padding(.vertical, 4)
                .overlay(RoundedRectangle(cornerRadius: 8).stroke(OrionTheme.border, lineWidth: 0.5))
            }
            .padding(.horizontal, 14)
            .frame(height: 52)
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
                    }
                    .padding(14)
                }
                .scrollDismissesKeyboard(.interactively)
                .background(OrionTheme.bgTerminal)
                .onChange(of: connection.messages.count) { _, _ in
                    if let last = chatRows.last {
                        withAnimation(.easeOut(duration: 0.18)) {
                            proxy.scrollTo(last.id, anchor: .bottom)
                        }
                    }
                }
            }

            composer
            .padding(12)
            .background(OrionTheme.bgSecondary)
            .overlay(alignment: .top) { OrionTheme.border.frame(height: 0.5) }
        }
        .fullScreenCover(item: $expandedPlan) { plan in
            PlanReviewView(
                plan: plan,
                assistantName: assistantName,
                accentColor: assistantColor,
                onClose: { expandedPlan = nil },
                onApprove: {
                    connection.approvePlan()
                    expandedPlan = nil
                }
            )
        }
        .onChange(of: selectedPhotoItems) { _, items in
            guard !items.isEmpty else { return }
            Task { await loadPhotoAttachments(items) }
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

    private var chatRows: [CodexChatRow] {
        mergeChatRows(connection.messages, assistantName: assistantName)
    }

    @ViewBuilder
    private func chatRow(_ row: CodexChatRow) -> some View {
        if row.type == "plan" {
            planRow(row)
        } else if row.type == "loading" {
            loadingRow(row)
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
                Text(assistantAvatar)
                    .font(.system(size: 12, weight: .bold))
                    .foregroundStyle(OrionTheme.textPrimary)
                    .frame(width: 24, height: 24)
                    .background(assistantColor.opacity(0.18))
                    .overlay(RoundedRectangle(cornerRadius: 6).stroke(assistantColor.opacity(0.35), lineWidth: 0.5))
                    .clipShape(RoundedRectangle(cornerRadius: 6))
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
        HStack(alignment: .bottom, spacing: 8) {
            Text(assistantAvatar)
                .font(.system(size: 12, weight: .bold))
                .foregroundStyle(OrionTheme.textPrimary)
                .frame(width: 24, height: 24)
                .background(assistantColor.opacity(0.18))
                .overlay(RoundedRectangle(cornerRadius: 6).stroke(assistantColor.opacity(0.35), lineWidth: 0.5))
                .clipShape(RoundedRectangle(cornerRadius: 6))
            VStack(alignment: .leading, spacing: 5) {
                Text("\(assistantName) has a plan")
                    .font(.system(size: 11))
                    .foregroundStyle(OrionTheme.textDim)
                VStack(alignment: .leading, spacing: 12) {
                    HStack(alignment: .top) {
                        VStack(alignment: .leading, spacing: 4) {
                            Text("Waiting for approval")
                                .font(.system(size: 10, weight: .semibold))
                                .foregroundStyle(OrionTheme.textDim)
                            Text(row.text.isEmpty ? "Plan ready" : row.text)
                                .font(.system(size: 15, weight: .semibold))
                                .foregroundStyle(OrionTheme.textPrimary)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                        Spacer(minLength: 8)
                        Text("Plan")
                            .font(.system(size: 11, design: .monospaced))
                            .foregroundStyle(OrionTheme.accentYellow)
                            .padding(.horizontal, 8)
                            .padding(.vertical, 4)
                            .overlay(RoundedRectangle(cornerRadius: 6).stroke(OrionTheme.accentYellow.opacity(0.55), lineWidth: 0.5))
                    }

                    Text(planPreview(row.details ?? row.text))
                        .font(.system(size: 13))
                        .foregroundStyle(OrionTheme.textSecondary)
                        .lineLimit(5)
                        .fixedSize(horizontal: false, vertical: true)

                    HStack(spacing: 8) {
                        Button("Review") { expandedPlan = row }
                            .buttonStyle(.bordered)
                        Button("Approve and run") { connection.approvePlan() }
                            .buttonStyle(.borderedProminent)
                            .tint(OrionTheme.accentBlue)
                    }
                }
                .padding(12)
                .frame(maxWidth: 340, alignment: .leading)
                .background(OrionTheme.accentYellow.opacity(0.08))
                .overlay(RoundedRectangle(cornerRadius: 8).stroke(OrionTheme.accentYellow.opacity(0.55), lineWidth: 0.5))
                .clipShape(RoundedRectangle(cornerRadius: 8))
            }
            Spacer(minLength: 22)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func loadingRow(_ row: CodexChatRow) -> some View {
        HStack(alignment: .bottom, spacing: 8) {
            Text(assistantAvatar)
                .font(.system(size: 12, weight: .bold))
                .foregroundStyle(OrionTheme.textPrimary)
                .frame(width: 24, height: 24)
                .background(assistantColor.opacity(0.18))
                .overlay(RoundedRectangle(cornerRadius: 6).stroke(assistantColor.opacity(0.35), lineWidth: 0.5))
                .clipShape(RoundedRectangle(cornerRadius: 6))
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

    private var composer: some View {
        VStack(spacing: 8) {
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

            VStack(alignment: .leading, spacing: 8) {
                TextField("Message \(assistantName)...", text: $input, axis: .vertical)
                    .textFieldStyle(.plain)
                    .font(.system(size: 16))
                    .foregroundStyle(OrionTheme.textPrimary)
                    .lineLimit(1...5)
                    .padding(.horizontal, 4)
                    .padding(.top, 2)

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
            .padding(10)
            .background(OrionTheme.bgSurface.opacity(0.96))
            .clipShape(RoundedRectangle(cornerRadius: 22))
            .overlay(RoundedRectangle(cornerRadius: 22).stroke(OrionTheme.border, lineWidth: 0.5))
        }
        .padding(.horizontal, 8)
        .padding(.top, 6)
        .padding(.bottom, 6)
        .background(OrionTheme.bgPrimary)
    }

    private var canSend: Bool {
        !input.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || !pendingImages.isEmpty
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
                    .foregroundStyle(row.type == "error" ? OrionTheme.accentRed : OrionTheme.textPrimary)
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
        .overlay(RoundedRectangle(cornerRadius: 8).stroke(rowBorder(row.type), lineWidth: 0.5))
        .clipShape(RoundedRectangle(cornerRadius: 8))
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
        case "user", "assistant", "permission_request", "plan", "loading": return false
        default: return true
        }
    }

    private func rowBackground(_ type: String) -> Color {
        switch type {
        case "user": return OrionTheme.accentBlue.opacity(0.16)
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
    let onApprove: () -> Void

    private var markdown: String {
        let value = plan.details ?? plan.text
        return value.isEmpty ? "Plan ready" : value
    }

    private var renderedPlan: AttributedString {
        (try? AttributedString(markdown: markdown)) ?? AttributedString(markdown)
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
                Text(renderedPlan)
                    .font(.system(size: 14))
                    .foregroundStyle(OrionTheme.textPrimary)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(16)
            }
            .background(OrionTheme.bgPrimary)

            HStack(spacing: 10) {
                Button("Back to chat", action: onClose)
                    .buttonStyle(.bordered)
                Button("Approve and run", action: onApprove)
                    .buttonStyle(.borderedProminent)
                    .tint(accentColor)
            }
            .frame(maxWidth: .infinity, alignment: .trailing)
            .padding(14)
            .background(OrionTheme.bgSecondary)
            .overlay(alignment: .top) { OrionTheme.border.frame(height: 0.5) }
        }
        .background(OrionTheme.bgPrimary)
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
    for message in messages {
        if message.type == "status" { continue }
        if shouldHideChatMessage(message) { continue }
        if message.type == "stream_delta" {
            if let lastIndex = rows.indices.last, rows[lastIndex].type == "assistant", rows[lastIndex].id.hasPrefix("assistant-stream") {
                rows[lastIndex].text += message.text ?? ""
            } else {
                rows.append(CodexChatRow(id: "assistant-stream-\(message.id)", type: "assistant", label: assistantName, text: message.text ?? "", details: nil, toolUseId: nil, planPath: nil, attachments: []))
            }
            continue
        }
        if message.type == "thinking_delta" {
            if let lastIndex = rows.indices.last, rows[lastIndex].type == "thinking_delta" {
                rows[lastIndex].text += message.text ?? ""
            } else {
                rows.append(CodexChatRow(id: message.id, type: message.type, label: "Thinking", text: message.text ?? "", details: nil, toolUseId: nil, planPath: nil, attachments: []))
            }
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
            attachments: message.attachments ?? []
        ))
    }
    if let status = messages.reversed().first(where: { $0.type == "status" }),
       status.status == "running" {
        rows.append(CodexChatRow(
            id: "loading-\(status.id)",
            type: "loading",
            label: assistantName,
            text: status.text ?? "\(assistantName) is thinking",
            details: nil,
            toolUseId: nil,
            planPath: nil,
            attachments: []
        ))
    }
    return rows
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
    return preview.isEmpty ? "Review the plan before Claude starts changing files." : preview
}
