import SwiftUI

struct SettingsView: View {
    @Environment(AppState.self) private var state
    @AppStorage("terminalFontSize") private var fontSize: Double = 14
    @AppStorage("ttsRate") private var ttsRate: Double = 0.52
    @AppStorage("ttsProvider") private var ttsProvider: String = "apple"
    @AppStorage("openaiVoice") private var openaiVoice: String = "nova"

    private let openaiVoices = [
        ("alloy", "Alloy — Neutral"),
        ("ash", "Ash — Warm"),
        ("ballad", "Ballad — Gentle"),
        ("coral", "Coral — Friendly"),
        ("echo", "Echo — Clear"),
        ("fable", "Fable — British"),
        ("nova", "Nova — Natural"),
        ("onyx", "Onyx — Deep"),
        ("sage", "Sage — Calm"),
        ("shimmer", "Shimmer — Bright"),
        ("verse", "Verse — Expressive"),
    ]

    var body: some View {
        NavigationStack {
            List {
                Section("Connection") {
                    LabeledContent("Host", value: state.host)
                    LabeledContent("Status", value: state.isConnected ? "Connected" : "Disconnected")
                    if let info = state.projectInfo { LabeledContent("Project", value: info.name) }
                    Button("Disconnect", role: .destructive) { state.disconnect(); state.showSettings = false }
                }
                Section("Terminal") {
                    VStack(alignment: .leading) { Text("Font Size: \(Int(fontSize))pt"); Slider(value: $fontSize, in: 10...24, step: 1).tint(OrionTheme.accentBlue) }
                }
                Section("Voice Mode") {
                    Toggle("Read Claude Responses", isOn: Binding(get: { state.voiceModeEnabled }, set: { _ in state.toggleVoiceMode() }))
                        .tint(OrionTheme.accentBlue)

                    Picker("TTS Engine", selection: $ttsProvider) {
                        Text("Apple (Free, Offline)").tag("apple")
                        Text("OpenAI (Natural)").tag("openai")
                    }

                    if state.speech.openAIApiKey.isEmpty {
                        Label("OpenAI key not loaded — reconnect", systemImage: "exclamationmark.triangle").font(.caption).foregroundStyle(OrionTheme.accentYellow)
                    } else {
                        Label("OpenAI key: \(String(state.speech.openAIApiKey.prefix(12)))...", systemImage: "checkmark.circle").font(.caption).foregroundStyle(OrionTheme.accentGreen)
                    }

                    if ttsProvider == "openai" {
                        Picker("Voice", selection: $openaiVoice) {
                            ForEach(openaiVoices, id: \.0) { voice in
                                Text(voice.1).tag(voice.0)
                            }
                        }
                    }

                    if ttsProvider == "apple" {
                        VStack(alignment: .leading) {
                            Text("Speech Rate")
                            Slider(value: $ttsRate, in: 0.3...0.7, step: 0.02) { Text("Rate") } minimumValueLabel: { Text("Slow").font(.caption2) } maximumValueLabel: { Text("Fast").font(.caption2) }.tint(OrionTheme.accentBlue)
                        }
                    }

                    HStack {
                        Button("Test Voice") { state.speech.speakResponse("Claude finished refactoring the auth middleware. Four files changed, all tests passing. Here's what I did:\n\n```ruby\ndef authenticate(token)\n  user = User.find_by(token: token)\n  return unauthorized unless user\nend\n```\n\nThe function now validates tokens before checking permissions.", rate: Float(ttsRate)) }
                        Spacer()
                        if state.voiceConnection.isConnected {
                            Label("Connected", systemImage: "antenna.radiowaves.left.and.right").font(.caption2).foregroundStyle(OrionTheme.accentGreen)
                        } else {
                            Label("Disconnected", systemImage: "antenna.radiowaves.left.and.right.slash").font(.caption2).foregroundStyle(OrionTheme.textDim)
                        }
                    }
                }
                Section("About") { LabeledContent("Version", value: "1.0.0") }
            }.navigationTitle("Settings").navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .topBarTrailing) { Button("Done") { state.showSettings = false }.foregroundStyle(OrionTheme.accentBlue) } }
        }
    }
}
