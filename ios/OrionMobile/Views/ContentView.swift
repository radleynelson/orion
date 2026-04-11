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
            WorkspaceListView().presentationDetents([.medium, .large]).presentationDragIndicator(.visible)
        }
        .sheet(isPresented: Binding(get: { state.showSettings }, set: { state.showSettings = $0 })) {
            SettingsView().presentationDetents([.medium]).presentationDragIndicator(.visible)
        }
    }
}

struct HeaderBar: View {
    @Environment(AppState.self) private var state
    var body: some View {
        HStack(spacing: 12) {
            Button { state.showWorkspaces = true } label: { Image(systemName: "sidebar.left").font(.system(size: 18)).foregroundStyle(OrionTheme.textSecondary) }
            Spacer()
            Text(state.projectInfo?.name ?? "Orion").font(.system(size: 15, weight: .semibold)).foregroundStyle(OrionTheme.textPrimary)
            Spacer()
            Circle().fill(state.isConnected ? OrionTheme.accentGreen : OrionTheme.accentRed).frame(width: 8, height: 8)
            Button { state.showSettings = true } label: { Image(systemName: "gearshape").font(.system(size: 16)).foregroundStyle(OrionTheme.textSecondary) }
        }
        .padding(.horizontal, 12).frame(height: 48).background(OrionTheme.bgSecondary)
        .overlay(alignment: .bottom) { OrionTheme.border.frame(height: 0.5) }
    }
}

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
