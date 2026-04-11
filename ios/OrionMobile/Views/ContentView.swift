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
            if !state.tabs.isEmpty { TabStrip() }
            ZStack {
                OrionTheme.bgTerminal.ignoresSafeArea()
                if let tab = state.activeTab, let connection = state.connections[tab.terminalId] {
                    TerminalContainerView(connection: connection)
                } else {
                    VStack(spacing: 12) {
                        Image(systemName: "terminal").font(.system(size: 40)).foregroundStyle(OrionTheme.textDim)
                        Text("Open a session from workspaces").font(.subheadline).foregroundStyle(OrionTheme.textDim)
                        Button("Browse Workspaces") { state.showWorkspaces = true }.buttonStyle(.bordered).tint(OrionTheme.accentBlue)
                    }
                }
            }
            if state.activeTab != nil { TerminalToolbar() }
        }
        .background(OrionTheme.bgPrimary)
        .sheet(isPresented: Binding(get: { state.showWorkspaces }, set: { state.showWorkspaces = $0 })) {
            WorkspaceSheet().presentationDetents([.medium, .large]).presentationDragIndicator(.visible)
        }
        .sheet(isPresented: Binding(get: { state.showSettings }, set: { state.showSettings = $0 })) {
            SettingsView().presentationDetents([.medium]).presentationDragIndicator(.visible)
        }
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
                    HStack(spacing: 4) {
                        Text(state.projectInfo?.name ?? "Orion")
                            .font(.system(size: 15, weight: .semibold))
                            .foregroundStyle(OrionTheme.textPrimary)
                        Image(systemName: "chevron.up.chevron.down")
                            .font(.system(size: 9, weight: .medium))
                            .foregroundStyle(OrionTheme.textDim)
                    }
                }
            } else {
                Text(state.projectInfo?.name ?? "Orion")
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundStyle(OrionTheme.textPrimary)
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

    var body: some View {
        Section {
            // Sessions — native swipe-to-delete
            ForEach(sessions) { session in
                Button {
                    Task { try? await state.openSession(session) }
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
            HStack(spacing: 6) {
                Text(workspace.name).font(.system(size: 13, weight: .semibold))
                if workspace.isMain {
                    Text("MAIN").font(.system(size: 9, weight: .bold)).padding(.horizontal, 5).padding(.vertical, 1)
                        .background(OrionTheme.accentBlue).foregroundStyle(.black).clipShape(RoundedRectangle(cornerRadius: 3))
                    Spacer()
                    Text(workspace.branch).font(.system(size: 11)).foregroundStyle(OrionTheme.textDim)
                }
            }
        }
        .onAppear { Task { serverStatuses = await state.getServerStatuses(workspace: workspace) } }
    }
}

// MARK: - Tabs

struct TabStrip: View {
    @Environment(AppState.self) private var state
    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 0) { ForEach(state.tabs) { tab in TabPill(tab: tab, isActive: tab.id == state.activeTabId) } }
        }.frame(height: 36).background(OrionTheme.bgSecondary).overlay(alignment: .bottom) { OrionTheme.border.frame(height: 0.5) }
    }
}

struct TabPill: View {
    @Environment(AppState.self) private var state
    let tab: TerminalTab; let isActive: Bool
    var body: some View {
        Button { state.activateTab(tab.id) } label: {
            HStack(spacing: 6) {
                Text(tab.label).font(.system(size: 13)).foregroundStyle(isActive ? OrionTheme.textPrimary : OrionTheme.textDim)
                Button { state.closeTab(tab.id) } label: { Image(systemName: "xmark").font(.system(size: 10, weight: .medium)).foregroundStyle(OrionTheme.textDim) }
            }.padding(.horizontal, 16).frame(height: 36).background(isActive ? OrionTheme.bgPrimary : .clear)
        }.overlay(alignment: .trailing) { OrionTheme.borderDim.frame(width: 0.5) }
    }
}

// MARK: - Session type icons (matches desktop app)

func sessionIcon(_ type: String) -> String {
    switch type {
    case "claude": return "\u{25C6}"  // ◆
    case "codex":  return "\u{25C7}"  // ◇
    case "server": return "\u{25B8}"  // ▸
    default:       return "\u{203A}"  // ›
    }
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
    case "claude": return OrionTheme.accentPurple
    case "codex":  return OrionTheme.accentBlue
    case "server": return OrionTheme.accentYellow
    default:       return OrionTheme.textSecondary
    }
}
