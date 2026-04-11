import SwiftUI

struct WorkspaceListView: View {
    @Environment(AppState.self) private var state
    @State private var expandedWorkspaces: Set<String> = []

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 16) {
                    if state.projects.count > 1 {
                        VStack(alignment: .leading, spacing: 6) {
                            Text("Project").font(.system(size: 11, weight: .medium)).foregroundStyle(OrionTheme.textDim).textCase(.uppercase)
                            Menu {
                                ForEach(state.projects, id: \.self) { p in Button((p as NSString).lastPathComponent) { Task { try? await state.selectProject(p) } } }
                            } label: {
                                HStack {
                                    Text(state.projectInfo?.name ?? "Select...").font(.system(size: 14)).foregroundStyle(OrionTheme.textPrimary); Spacer()
                                    Image(systemName: "chevron.up.chevron.down").font(.system(size: 12)).foregroundStyle(OrionTheme.textDim)
                                }.padding(12).background(OrionTheme.bgSurface).clipShape(RoundedRectangle(cornerRadius: 8)).overlay(RoundedRectangle(cornerRadius: 8).stroke(OrionTheme.border, lineWidth: 1))
                            }
                        }
                    }
                    ForEach(state.workspaces) { ws in
                        let sessions = state.sessions.filter { $0.workspacePath == ws.path }
                        WorkspaceCard(workspace: ws, sessions: sessions, isExpanded: expandedWorkspaces.contains(ws.path),
                            onToggle: { if expandedWorkspaces.contains(ws.path) { expandedWorkspaces.remove(ws.path) } else { expandedWorkspaces.insert(ws.path) } },
                            onOpenSession: { s in Task { try? await state.openSession(s); state.showWorkspaces = false } },
                            onNewShell: { Task { try? await state.launchShell(workspacePath: ws.path); state.showWorkspaces = false } })
                    }
                }.padding(16)
            }.background(OrionTheme.bgPrimary).navigationTitle("Workspaces").navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) { Button("Done") { state.showWorkspaces = false }.foregroundStyle(OrionTheme.accentBlue) }
                ToolbarItem(placement: .topBarLeading) { Button { Task { await state.refreshSessions() } } label: { Image(systemName: "arrow.clockwise").foregroundStyle(OrionTheme.accentBlue) } }
            }
        }.onAppear { for ws in state.workspaces { if !state.sessions.filter({ $0.workspacePath == ws.path }).isEmpty { expandedWorkspaces.insert(ws.path) } } }
    }
}

struct WorkspaceCard: View {
    let workspace: Workspace; let sessions: [SessionInfo]; let isExpanded: Bool
    let onToggle: () -> Void; let onOpenSession: (SessionInfo) -> Void; let onNewShell: () -> Void

    var body: some View {
        VStack(spacing: 0) {
            Button(action: onToggle) {
                HStack {
                    VStack(alignment: .leading, spacing: 2) {
                        HStack(spacing: 8) {
                            Text(workspace.name).font(.system(size: 14, weight: .medium)).foregroundStyle(OrionTheme.textPrimary)
                            if workspace.isMain { Text("MAIN").font(.system(size: 10, weight: .bold)).padding(.horizontal, 6).padding(.vertical, 2).background(OrionTheme.accentBlue).foregroundStyle(.black).clipShape(RoundedRectangle(cornerRadius: 4)) }
                        }
                        HStack(spacing: 4) {
                            Text(workspace.branch).font(.system(size: 12)).foregroundStyle(OrionTheme.textDim)
                            if !sessions.isEmpty { Text("·").foregroundStyle(OrionTheme.textDim); Text("\(sessions.count) session\(sessions.count == 1 ? "" : "s")").font(.system(size: 12)).foregroundStyle(OrionTheme.textDim) }
                        }
                    }; Spacer()
                    Image(systemName: isExpanded ? "chevron.up" : "chevron.down").font(.system(size: 12)).foregroundStyle(OrionTheme.textDim)
                }.padding(12)
            }
            if isExpanded {
                VStack(spacing: 0) {
                    ForEach(sessions) { session in
                        Button { onOpenSession(session) } label: {
                            HStack(spacing: 8) {
                                Text(session.type == "shell" ? "\u{25B8}" : "\u{2726}").font(.system(size: 14)).foregroundStyle(session.type == "shell" ? OrionTheme.textSecondary : OrionTheme.accentPurple)
                                Text(session.label).font(.system(size: 13)).foregroundStyle(OrionTheme.textSecondary); Spacer()
                            }.padding(.horizontal, 12).padding(.vertical, 8).contentShape(Rectangle())
                        }
                    }
                    Button(action: onNewShell) {
                        HStack(spacing: 6) { Image(systemName: "plus").font(.system(size: 12)); Text("Shell").font(.system(size: 12)) }
                        .foregroundStyle(OrionTheme.textSecondary).padding(.horizontal, 12).padding(.vertical, 8).frame(maxWidth: .infinity)
                        .background(OrionTheme.bgHover).clipShape(RoundedRectangle(cornerRadius: 6))
                    }.padding(12)
                }
            }
        }.background(OrionTheme.bgSurface).clipShape(RoundedRectangle(cornerRadius: 8))
    }
}
