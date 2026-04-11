import SwiftUI

struct ConnectionView: View {
    @Environment(AppState.self) private var state
    @State private var host = ""
    @State private var token = ""
    @State private var isConnecting = false
    @State private var didAutoConnect = false

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
                            host = discovered.address
                            if let saved = KeychainService.getToken(for: discovered.address) { token = saved }
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
                TextField("Host (e.g. 192.168.1.100:9867)", text: $host).textFieldStyle(OrionTextFieldStyle()).textInputAutocapitalization(.never).autocorrectionDisabled().keyboardType(.URL)
                TextField("Auth token", text: $token).textFieldStyle(OrionTextFieldStyle()).textInputAutocapitalization(.never).autocorrectionDisabled()
                if let error = state.connectionError { Text(error).font(.system(size: 13)).foregroundStyle(OrionTheme.accentRed) }
                Button { Task { await connectTapped() } } label: {
                    if isConnecting { ProgressView().tint(.black).frame(maxWidth: .infinity).frame(height: 48) }
                    else { Text("Connect").font(.system(size: 16, weight: .semibold)).frame(maxWidth: .infinity).frame(height: 48) }
                }.background(OrionTheme.accentBlue).foregroundStyle(.black).clipShape(RoundedRectangle(cornerRadius: 8))
                .disabled(host.isEmpty || token.isEmpty || isConnecting).opacity(host.isEmpty || token.isEmpty ? 0.4 : 1)
            }.padding(.horizontal, 24)

            let saved = KeychainService.loadConnections()
            if !saved.isEmpty {
                VStack(alignment: .leading, spacing: 8) {
                    Text("Saved Connections").font(.system(size: 11, weight: .medium)).foregroundStyle(OrionTheme.textDim).textCase(.uppercase)
                    ForEach(saved) { conn in
                        Button { host = conn.host; token = conn.token; Task { await connectTapped() } } label: {
                            HStack { Text(conn.host).font(.system(size: 14, design: .monospaced)).foregroundStyle(OrionTheme.textSecondary); Spacer() }
                            .padding(12).background(OrionTheme.bgSurface).clipShape(RoundedRectangle(cornerRadius: 8))
                        }
                    }
                }.padding(.horizontal, 24).padding(.top, 24)
            }
            Spacer()
        }.background(OrionTheme.bgPrimary)
        .onAppear {
            state.bonjour.startBrowsing()

            // Load last connection from UserDefaults (survives simulator rebuilds)
            let defaults = UserDefaults.standard
            if let savedHost = defaults.string(forKey: "lastHost"), !savedHost.isEmpty,
               let savedToken = defaults.string(forKey: "lastToken"), !savedToken.isEmpty {
                host = savedHost
                token = savedToken
                // Auto-connect if we have saved credentials
                if !didAutoConnect {
                    didAutoConnect = true
                    Task { await connectTapped() }
                }
            } else if let first = KeychainService.loadConnections().first {
                host = first.host
                token = first.token
            }
        }
    }

    private func connectTapped() async {
        isConnecting = true; state.connectionError = nil
        do {
            try await state.connect(host: host, token: token)
            // Persist to UserDefaults for quick reconnect after rebuild
            UserDefaults.standard.set(host, forKey: "lastHost")
            UserDefaults.standard.set(token, forKey: "lastToken")
        } catch {
            state.connectionError = error.localizedDescription
        }
        isConnecting = false
    }
}

struct OrionTextFieldStyle: TextFieldStyle {
    func _body(configuration: TextField<Self._Label>) -> some View {
        configuration.padding(14).background(OrionTheme.bgSurface).clipShape(RoundedRectangle(cornerRadius: 8))
            .overlay(RoundedRectangle(cornerRadius: 8).stroke(OrionTheme.border, lineWidth: 1)).font(.system(size: 16)).foregroundStyle(OrionTheme.textPrimary)
    }
}
