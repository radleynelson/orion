import AVFoundation
import Speech

@Observable
final class SpeechService: NSObject {
    var isListening = false
    var isSpeaking = false
    var isAuthorized = false
    var dictatedText = ""
    var onDictationResult: ((String) -> Void)?

    private let synthesizer = AVSpeechSynthesizer()
    private var recognizer: SFSpeechRecognizer?
    private var recognitionRequest: SFSpeechAudioBufferRecognitionRequest?
    private var recognitionTask: SFSpeechRecognitionTask?
    private let audioEngine = AVAudioEngine()
    private var sentenceQueue: [String] = []

    override init() { super.init(); synthesizer.delegate = self; recognizer = SFSpeechRecognizer(locale: Locale(identifier: "en-US")) }

    func requestAuthorization() {
        SFSpeechRecognizer.requestAuthorization { [weak self] status in DispatchQueue.main.async { self?.isAuthorized = (status == .authorized) } }
        AVAudioApplication.requestRecordPermission { [weak self] granted in DispatchQueue.main.async { if !granted { self?.isAuthorized = false } } }
    }

    func startDictation() {
        guard isAuthorized, !isListening else { return }
        if isSpeaking { stopSpeaking() }
        do {
            try AVAudioSession.sharedInstance().setCategory(.record, mode: .measurement, options: .duckOthers)
            try AVAudioSession.sharedInstance().setActive(true, options: .notifyOthersOnDeactivation)
            recognitionRequest = SFSpeechAudioBufferRecognitionRequest()
            guard let request = recognitionRequest else { return }
            request.shouldReportPartialResults = true; request.requiresOnDeviceRecognition = true
            recognitionTask = recognizer?.recognitionTask(with: request) { [weak self] result, error in
                if let result { self?.dictatedText = result.bestTranscription.formattedString; if result.isFinal { self?.onDictationResult?(self?.dictatedText ?? ""); self?.stopDictation() } }
                if error != nil { self?.stopDictation() }
            }
            let inputNode = audioEngine.inputNode
            inputNode.installTap(onBus: 0, bufferSize: 1024, format: inputNode.outputFormat(forBus: 0)) { [weak self] buffer, _ in self?.recognitionRequest?.append(buffer) }
            audioEngine.prepare(); try audioEngine.start(); isListening = true; dictatedText = ""
        } catch { stopDictation() }
    }

    func stopDictation() {
        audioEngine.stop(); audioEngine.inputNode.removeTap(onBus: 0); recognitionRequest?.endAudio(); recognitionRequest = nil; recognitionTask?.cancel(); recognitionTask = nil
        if !dictatedText.isEmpty { onDictationResult?(dictatedText) }
        isListening = false; dictatedText = ""; try? AVAudioSession.sharedInstance().setActive(false)
    }

    func speak(_ text: String, rate: Float = 0.52) {
        guard !text.isEmpty else { return }; if isSpeaking { stopSpeaking() }
        let utterance = AVSpeechUtterance(string: text); utterance.rate = rate; utterance.voice = AVSpeechSynthesisVoice(language: "en-US")
        do { try AVAudioSession.sharedInstance().setCategory(.playback, mode: .voicePrompt); try AVAudioSession.sharedInstance().setActive(true) } catch {}
        synthesizer.speak(utterance); isSpeaking = true
    }

    /// Speak a long response by breaking it into sentences and queuing them.
    /// This gives a more natural flow for longer Claude responses.
    func speakResponse(_ text: String, rate: Float = 0.52) {
        guard !text.isEmpty else { return }
        if isSpeaking { stopSpeaking() }

        // Filter the text for voice (skip code blocks, abbreviate paths, etc.)
        let filtered = VoiceContentFilter.filter(text)
        guard !filtered.isEmpty else { return }

        // Split into sentences and queue them
        sentenceQueue = splitIntoSentences(filtered)
        guard !sentenceQueue.isEmpty else { return }

        do {
            try AVAudioSession.sharedInstance().setCategory(.playback, mode: .voicePrompt)
            try AVAudioSession.sharedInstance().setActive(true)
        } catch {}

        speakNextSentence(rate: rate)
    }

    func stopSpeaking() {
        synthesizer.stopSpeaking(at: .immediate)
        sentenceQueue.removeAll()
        isSpeaking = false
    }

    private func speakNextSentence(rate: Float = 0.52) {
        guard !sentenceQueue.isEmpty else {
            isSpeaking = false
            return
        }
        let sentence = sentenceQueue.removeFirst()
        let utterance = AVSpeechUtterance(string: sentence)
        utterance.rate = rate
        utterance.voice = AVSpeechSynthesisVoice(language: "en-US")
        utterance.postUtteranceDelay = 0.1
        synthesizer.speak(utterance)
        isSpeaking = true
    }

    private func splitIntoSentences(_ text: String) -> [String] {
        var sentences: [String] = []
        text.enumerateSubstrings(in: text.startIndex..., options: [.bySentences, .localized]) { sub, _, _, _ in
            if let s = sub?.trimmingCharacters(in: .whitespacesAndNewlines), !s.isEmpty {
                sentences.append(s)
            }
        }
        if sentences.isEmpty && !text.isEmpty {
            sentences = [text]
        }
        return sentences
    }
}

extension SpeechService: AVSpeechSynthesizerDelegate {
    func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didFinish utterance: AVSpeechUtterance) {
        DispatchQueue.main.async {
            if self.sentenceQueue.isEmpty {
                self.isSpeaking = false
            } else {
                let rate = UserDefaults.standard.double(forKey: "ttsRate")
                self.speakNextSentence(rate: Float(rate > 0 ? rate : 0.52))
            }
        }
    }
    func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didCancel utterance: AVSpeechUtterance) {
        DispatchQueue.main.async { self.sentenceQueue.removeAll(); self.isSpeaking = false }
    }
}

// MARK: - Voice Content Filter

/// Filters Claude Code markdown responses for TTS readability.
/// Skips code blocks, abbreviates file paths, and cleans up formatting.
enum VoiceContentFilter {
    static func filter(_ text: String) -> String {
        var result: [String] = []
        var inCodeBlock = false
        var codeBlockLines = 0
        var codeBlockLang = ""

        for line in text.components(separatedBy: "\n") {
            let trimmed = line.trimmingCharacters(in: .whitespaces)

            // Detect code block boundaries
            if trimmed.hasPrefix("```") {
                if inCodeBlock {
                    // Closing code block — announce how long it was
                    if codeBlockLines > 0 {
                        let langLabel = codeBlockLang.isEmpty ? "code" : codeBlockLang
                        result.append("[\(codeBlockLines) lines of \(langLabel)]")
                    }
                    inCodeBlock = false
                    codeBlockLines = 0
                    codeBlockLang = ""
                } else {
                    // Opening code block
                    inCodeBlock = true
                    codeBlockLines = 0
                    codeBlockLang = String(trimmed.dropFirst(3)).trimmingCharacters(in: .whitespaces)
                }
                continue
            }

            if inCodeBlock {
                codeBlockLines += 1
                continue
            }

            // Skip empty lines
            if trimmed.isEmpty { continue }

            // Clean up markdown formatting
            var cleaned = trimmed
            // Remove bold/italic markers
            cleaned = cleaned.replacingOccurrences(of: "**", with: "")
            cleaned = cleaned.replacingOccurrences(of: "__", with: "")
            // Remove header markers
            if cleaned.hasPrefix("# ") { cleaned = String(cleaned.dropFirst(2)) }
            else if cleaned.hasPrefix("## ") { cleaned = String(cleaned.dropFirst(3)) }
            else if cleaned.hasPrefix("### ") { cleaned = String(cleaned.dropFirst(4)) }
            // Remove bullet markers
            if cleaned.hasPrefix("- ") { cleaned = String(cleaned.dropFirst(2)) }
            else if cleaned.hasPrefix("* ") { cleaned = String(cleaned.dropFirst(2)) }
            // Clean inline code backticks
            cleaned = cleaned.replacingOccurrences(of: "`", with: "")
            // Abbreviate long file paths: keep just filename or last component
            cleaned = abbreviatePaths(cleaned)

            if !cleaned.isEmpty {
                result.append(cleaned)
            }
        }

        // Handle unclosed code block
        if inCodeBlock && codeBlockLines > 0 {
            let langLabel = codeBlockLang.isEmpty ? "code" : codeBlockLang
            result.append("[\(codeBlockLines) lines of \(langLabel)]")
        }

        return result.joined(separator: " ")
    }

    private static func abbreviatePaths(_ text: String) -> String {
        // Match paths like /Users/foo/bar/baz/file.rb or internal/server/manager.go
        let pattern = #"(?:\/[\w.-]+){3,}"#
        guard let regex = try? NSRegularExpression(pattern: pattern) else { return text }
        let range = NSRange(text.startIndex..., in: text)
        var result = text
        // Process matches in reverse so indices stay valid
        let matches = regex.matches(in: text, range: range).reversed()
        for match in matches {
            guard let matchRange = Range(match.range, in: text) else { continue }
            let path = String(text[matchRange])
            let components = path.split(separator: "/")
            if components.count > 2 {
                let abbreviated = components.suffix(2).joined(separator: "/")
                result = result.replacingCharacters(in: matchRange, with: abbreviated)
            }
        }
        return result
    }
}
