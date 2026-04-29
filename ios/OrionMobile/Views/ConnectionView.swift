import SwiftUI

struct ConnectionView: View {
    @Environment(AppState.self) private var state
    @State private var connectionName = ""
    @State private var host = ""
    @State private var token = ""
    @State private var isConnecting = false
    @State private var didAutoConnect = false
    @State private var savedConnections: [SavedConnection] = []
    @State private var renameHost: String?
    @State private var renameDraft = ""

    var body: some View {
        VStack(spacing: 0) {
            Spacer()
            VStack(spacing: 4) {
                Text("ORION").font(.system(size: 28, weight: .bold, design: .monospaced)).foregroundStyle(OrionTheme.accentBlue).kerning(4)
                Text("Mobile Companion").font(.system(size: 13)).foregroundStyle(OrionTheme.textDim)
            }.padding(.bottom, 32)

            if !state.bonjour.discoveredHosts.isEmpty {
                VStack(alignment: .leading, spacing: 8) {
                    Label("Discovered on Network", systemImage: "wifi").font(.system(size: 11, weight: .medium)).foregroundStyle(OrionTheme.textDim).textCase(.uppercase)
                    ForEach(state.bonjour.discoveredHosts) { discovered in
                        Button {
                            selectConnection(
                                host: discovered.address,
                                token: KeychainService.getToken(for: discovered.address) ?? token,
                                name: savedName(for: discovered.address) ?? discovered.name
                            )
                        } label: {
                            HStack {
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(discovered.name).font(.system(size: 14, weight: .medium)).foregroundStyle(OrionTheme.textPrimary)
                                    Text(discovered.address).font(.system(size: 12, design: .monospaced)).foregroundStyle(OrionTheme.textDim)
                                }; Spacer(); Image(systemName: "arrow.right.circle").foregroundStyle(OrionTheme.accentBlue)
                            }.padding(12).background(OrionTheme.bgSurface).clipShape(RoundedRectangle(cornerRadius: 8))
                        }
                    }
                }.padding(.horizontal, 24).padding(.bottom, 20)
            }

            VStack(spacing: 12) {
                TextField("Name (e.g. Mac Studio, Work laptop)", text: $connectionName).textFieldStyle(OrionTextFieldStyle()).textInputAutocapitalization(.words).autocorrectionDisabled()
                TextField("Host (e.g. 192.168.1.100:9867)", text: $host).textFieldStyle(OrionTextFieldStyle()).textInputAutocapitalization(.never).autocorrectionDisabled().keyboardType(.URL)
                TextField("Auth token", text: $token).textFieldStyle(OrionTextFieldStyle()).textInputAutocapitalization(.never).autocorrectionDisabled()
                if let error = state.connectionError { Text(error).font(.system(size: 13)).foregroundStyle(OrionTheme.accentRed) }
                Button { Task { await connectTapped() } } label: {
                    if isConnecting { ProgressView().tint(.black).frame(maxWidth: .infinity).frame(height: 48) }
                    else { Text("Connect").font(.system(size: 16, weight: .semibold)).frame(maxWidth: .infinity).frame(height: 48) }
                }.background(OrionTheme.accentBlue).foregroundStyle(.black).clipShape(RoundedRectangle(cornerRadius: 8))
                .disabled(!canConnect)
                .opacity(trimmedHost.isEmpty || trimmedToken.isEmpty ? 0.4 : 1)
            }.padding(.horizontal, 24)

            if !savedConnections.isEmpty {
                VStack(alignment: .leading, spacing: 8) {
                    Text("Saved Connections").font(.system(size: 11, weight: .medium)).foregroundStyle(OrionTheme.textDim).textCase(.uppercase)
                    ForEach(savedConnections) { conn in
                        HStack(spacing: 8) {
                            Button { connectSaved(conn) } label: {
                                HStack {
                                    VStack(alignment: .leading, spacing: 3) {
                                        Text(conn.displayName)
                                            .font(.system(size: 14, weight: .medium))
                                            .foregroundStyle(OrionTheme.textPrimary)
                                            .lineLimit(1)
                                        if conn.hasCustomName {
                                            Text(conn.host)
                                                .font(.system(size: 12, design: .monospaced))
                                                .foregroundStyle(OrionTheme.textDim)
                                                .lineLimit(1)
                                        }
                                    }
                                    Spacer()
                                }
                            }
                            .buttonStyle(.plain)

                            Button { beginRenaming(conn) } label: {
                                Image(systemName: "pencil")
                                    .font(.system(size: 13, weight: .semibold))
                                    .foregroundStyle(OrionTheme.accentBlue)
                                    .frame(width: 34, height: 34)
                                    .background(OrionTheme.accentBlue.opacity(0.1))
                                    .clipShape(RoundedRectangle(cornerRadius: 8))
                            }
                            .buttonStyle(.plain)
                        }
                        .padding(12)
                        .background(OrionTheme.bgSurface)
                        .clipShape(RoundedRectangle(cornerRadius: 8))
                    }
                }.padding(.horizontal, 24).padding(.top, 24)
            }
            Spacer()
        }.background(OrionTheme.bgPrimary)
        .alert("Rename Connection", isPresented: Binding(
            get: { renameHost != nil },
            set: { if !$0 { renameHost = nil } }
        )) {
            TextField("Name", text: $renameDraft)
            Button("Save") { saveRename() }
            Button("Cancel", role: .cancel) { renameHost = nil }
        } message: {
            if let renameHost {
                Text(renameHost)
            }
        }
        .onAppear {
            state.bonjour.startBrowsing()
            savedConnections = KeychainService.loadConnections()
            let shouldAutoConnect = !state.suppressNextAutoConnect

            let env = ProcessInfo.processInfo.environment
            if let envHost = env["ORION_MOBILE_HOST"], !envHost.isEmpty,
               let envToken = env["ORION_MOBILE_TOKEN"], !envToken.isEmpty {
                selectConnection(host: envHost, token: envToken, name: savedName(for: envHost))
                if shouldAutoConnect && !didAutoConnect {
                    didAutoConnect = true
                    Task { await connectTapped() }
                }
                return
            }

            // Load last host from UserDefaults; token lives in Keychain only
            let defaults = UserDefaults.standard
            if let savedHost = defaults.string(forKey: "lastHost"), !savedHost.isEmpty,
               let savedToken = KeychainService.getToken(for: savedHost), !savedToken.isEmpty {
                selectConnection(host: savedHost, token: savedToken, name: savedName(for: savedHost))
                if shouldAutoConnect && !didAutoConnect {
                    didAutoConnect = true
                    Task { await connectTapped() }
                }
            } else if let first = savedConnections.first {
                selectConnection(host: first.host, token: first.token, name: first.name)
            }
        }
    }

    private var trimmedHost: String {
        host.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var trimmedToken: String {
        token.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var canConnect: Bool {
        !trimmedHost.isEmpty && !trimmedToken.isEmpty && !isConnecting
    }

    private func connectTapped() async {
        let selectedHost = trimmedHost
        let selectedToken = trimmedToken
        guard !selectedHost.isEmpty, !selectedToken.isEmpty else { return }
        isConnecting = true; state.connectionError = nil
        do {
            try await state.connect(host: selectedHost, token: selectedToken, name: connectionName)
            host = selectedHost
            token = selectedToken
            // Persist host only; token is stored in Keychain by state.connect()
            UserDefaults.standard.set(selectedHost, forKey: "lastHost")
            savedConnections = KeychainService.loadConnections()
            connectionName = savedName(for: selectedHost) ?? connectionName.trimmingCharacters(in: .whitespacesAndNewlines)
        } catch {
            state.connectionError = error.localizedDescription
        }
        isConnecting = false
    }

    private func connectSaved(_ connection: SavedConnection) {
        selectConnection(host: connection.host, token: connection.token, name: connection.name)
        Task { await connectTapped() }
    }

    private func selectConnection(host selectedHost: String, token selectedToken: String, name selectedName: String?) {
        host = selectedHost
        token = selectedToken
        connectionName = selectedName?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    }

    private func savedName(for host: String) -> String? {
        savedConnections.first(where: { $0.host == host })?.name
    }

    private func beginRenaming(_ connection: SavedConnection) {
        renameHost = connection.host
        renameDraft = connection.name?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    }

    private func saveRename() {
        guard let renameHost else { return }
        var connections = KeychainService.loadConnections()
        guard let index = connections.firstIndex(where: { $0.host == renameHost }) else {
            self.renameHost = nil
            return
        }
        let connection = connections[index]
        let name = renameDraft.trimmingCharacters(in: .whitespacesAndNewlines)
        connections[index] = SavedConnection(
            host: connection.host,
            token: connection.token,
            name: name.isEmpty ? nil : name
        )
        KeychainService.saveConnections(connections)
        savedConnections = connections
        if host == renameHost {
            connectionName = name
        }
        self.renameHost = nil
    }
}

struct OrionTextFieldStyle: TextFieldStyle {
    func _body(configuration: TextField<Self._Label>) -> some View {
        configuration.padding(14).background(OrionTheme.bgSurface).clipShape(RoundedRectangle(cornerRadius: 8))
            .overlay(RoundedRectangle(cornerRadius: 8).stroke(OrionTheme.border, lineWidth: 1)).font(.system(size: 16)).foregroundStyle(OrionTheme.textPrimary)
    }
}
