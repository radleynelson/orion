import SwiftUI

struct TerminalToolbar: View {
    @Environment(AppState.self) private var state
    var body: some View {
        HStack(spacing: 4) {
            ShortcutButton(label: "Ctrl+C") { sendKey("\u{03}") }
            ShortcutButton(label: "Tab") { sendKey("\t") }
            ShortcutButton(label: "\u{25B2}", monospace: false) { sendKey("\u{1B}[A") }
            ShortcutButton(label: "\u{25BC}", monospace: false) { sendKey("\u{1B}[B") }
            ShortcutButton(label: "Esc") { sendKey("\u{1B}") }
            ShortcutButton(label: "Ctrl+D") { sendKey("\u{04}") }
            Divider().frame(height: 24).background(OrionTheme.border)
            KeyboardButton()
            MicButton()
            SpeakerButton()
        }
        .padding(.horizontal, 4).padding(.bottom, safeAreaBottom).frame(minHeight: 44)
        .background(OrionTheme.bgSecondary).overlay(alignment: .top) { OrionTheme.border.frame(height: 0.5) }
    }

    private var safeAreaBottom: CGFloat {
        UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }.first?.windows.first?.safeAreaInsets.bottom ?? 0
    }

    private func sendKey(_ key: String) {
        guard let tab = state.activeTab, let connection = state.connections[tab.terminalId] else { return }
        connection.sendInput([UInt8](key.utf8)); UIImpactFeedbackGenerator(style: .light).impactOccurred()
    }
}

struct ShortcutButton: View {
    let label: String; var monospace: Bool = true; let action: () -> Void
    var body: some View {
        Button(action: action) {
            Text(label).font(monospace ? .system(size: 11, design: .monospaced) : .system(size: 14))
                .foregroundStyle(OrionTheme.textSecondary).frame(maxWidth: .infinity).frame(height: 32)
                .background(OrionTheme.bgSurface).clipShape(RoundedRectangle(cornerRadius: 6))
                .overlay(RoundedRectangle(cornerRadius: 6).stroke(OrionTheme.border, lineWidth: 0.5))
        }
    }
}

struct KeyboardButton: View {
    @State private var isShowingKeyboard = false
    var body: some View {
        Button {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            isShowingKeyboard.toggle()
            NotificationCenter.default.post(name: .orionToggleKeyboard, object: nil)
        } label: {
            Image(systemName: isShowingKeyboard ? "keyboard.fill" : "keyboard").font(.system(size: 14))
                .foregroundStyle(isShowingKeyboard ? OrionTheme.accentBlue : OrionTheme.textSecondary)
                .frame(width: 36, height: 32).background(isShowingKeyboard ? OrionTheme.bgActive : OrionTheme.bgSurface)
                .clipShape(RoundedRectangle(cornerRadius: 6)).overlay(RoundedRectangle(cornerRadius: 6).stroke(isShowingKeyboard ? OrionTheme.accentBlue : OrionTheme.border, lineWidth: 0.5))
        }.onReceive(NotificationCenter.default.publisher(for: UIResponder.keyboardDidHideNotification)) { _ in isShowingKeyboard = false }
    }
}

struct MicButton: View {
    @Environment(AppState.self) private var state
    var body: some View {
        Button {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            if state.speech.isListening { state.speech.stopDictation(); return }
            if !state.speech.isAuthorized { state.speech.requestAuthorization(); return }
            state.speech.onDictationResult = { text in
                guard let tab = state.activeTab, let conn = state.connections[tab.terminalId] else { return }
                conn.sendInput([UInt8](text.utf8))
            }
            state.speech.startDictation()
        } label: {
            Image(systemName: state.speech.isListening ? "mic.fill" : "mic").font(.system(size: 16))
                .foregroundStyle(state.speech.isListening ? OrionTheme.accentRed : OrionTheme.textSecondary)
                .frame(width: 36, height: 32).background(state.speech.isListening ? OrionTheme.bgActive : OrionTheme.bgSurface)
                .clipShape(RoundedRectangle(cornerRadius: 6)).overlay(RoundedRectangle(cornerRadius: 6).stroke(state.speech.isListening ? OrionTheme.accentRed : OrionTheme.border, lineWidth: 0.5))
        }
    }
}

struct SpeakerButton: View {
    @Environment(AppState.self) private var state
    var body: some View {
        Button {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            if state.speech.isSpeaking { state.speech.stopSpeaking(); return }
            if let text = UIPasteboard.general.string, !text.isEmpty { state.speech.speak(text) }
        } label: {
            Image(systemName: state.speech.isSpeaking ? "speaker.wave.3.fill" : "speaker.wave.2").font(.system(size: 14))
                .foregroundStyle(state.speech.isSpeaking ? OrionTheme.accentBlue : OrionTheme.textSecondary)
                .frame(width: 36, height: 32).background(state.speech.isSpeaking ? OrionTheme.bgActive : OrionTheme.bgSurface)
                .clipShape(RoundedRectangle(cornerRadius: 6)).overlay(RoundedRectangle(cornerRadius: 6).stroke(state.speech.isSpeaking ? OrionTheme.accentBlue : OrionTheme.border, lineWidth: 0.5))
        }
    }
}
