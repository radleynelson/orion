import AVFoundation
import Speech

@Observable
final class SpeechService: NSObject {
    var isListening = false
    var isSpeaking = false
    var isAuthorized = false
    var dictatedText = ""
    var onDictationResult: ((String) -> Void)?
    var openAIApiKey = ""

    private let synthesizer = AVSpeechSynthesizer()
    private var recognizer: SFSpeechRecognizer?
    private var recognitionRequest: SFSpeechAudioBufferRecognitionRequest?
    private var recognitionTask: SFSpeechRecognitionTask?
    private let audioEngine = AVAudioEngine()
    private var sentenceQueue: [String] = []
    private var audioPlayer: AVAudioPlayer?
    private var ttsTask: Task<Void, Never>?

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

    /// Speak a Claude response. Routes to OpenAI TTS or Apple TTS based on settings.
    func speakResponse(_ text: String, rate: Float = 0.52) {
        guard !text.isEmpty else { return }
        if isSpeaking { stopSpeaking() }

        let filtered = VoiceContentFilter.filter(text)
        guard !filtered.isEmpty else { return }

        let provider = UserDefaults.standard.string(forKey: "ttsProvider") ?? "apple"
        print("[Orion TTS] Provider: \(provider), API key: \(openAIApiKey.isEmpty ? "EMPTY" : "SET (\(openAIApiKey.count) chars)"), text length: \(filtered.count)")
        if provider == "openai" && !openAIApiKey.isEmpty {
            print("[Orion TTS] Using OpenAI TTS")
            speakWithOpenAI(filtered)
        } else {
            print("[Orion TTS] Using Apple TTS (provider=\(provider), keyEmpty=\(openAIApiKey.isEmpty))")
            speakWithApple(filtered, rate: rate)
        }
    }

    func stopSpeaking() {
        ttsTask?.cancel()
        ttsTask = nil
        audioPlayer?.stop()
        audioPlayer = nil
        synthesizer.stopSpeaking(at: .immediate)
        sentenceQueue.removeAll()
        isSpeaking = false
    }

    // MARK: - Apple TTS

    private func speakWithApple(_ text: String, rate: Float) {
        sentenceQueue = splitIntoSentences(text)
        guard !sentenceQueue.isEmpty else { return }
        do {
            try AVAudioSession.sharedInstance().setCategory(.playback, mode: .voicePrompt)
            try AVAudioSession.sharedInstance().setActive(true)
        } catch {}
        speakNextSentence(rate: rate)
    }

    private func speakNextSentence(rate: Float = 0.52) {
        guard !sentenceQueue.isEmpty else { isSpeaking = false; return }
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
            if let s = sub?.trimmingCharacters(in: .whitespacesAndNewlines), !s.isEmpty { sentences.append(s) }
        }
        if sentences.isEmpty && !text.isEmpty { sentences = [text] }
        return sentences
    }

    // MARK: - OpenAI TTS

    private func speakWithOpenAI(_ text: String) {
        isSpeaking = true
        ttsTask = Task { [weak self] in
            guard let self else { return }
            // Chunk text at 4096 char limit if needed
            let chunks = self.chunkText(text, maxLength: 4000)
            for chunk in chunks {
                if Task.isCancelled { break }
                await self.playOpenAIChunk(chunk)
            }
            await MainActor.run { self.isSpeaking = false }
        }
    }

    private func playOpenAIChunk(_ text: String) async {
        let voice = UserDefaults.standard.string(forKey: "openaiVoice") ?? "nova"
        let instructions = "Speak naturally and conversationally, like a colleague explaining what they worked on. Keep a moderate pace. When you encounter technical terms or variable names, pronounce them clearly."

        var request = URLRequest(url: URL(string: "https://api.openai.com/v1/audio/speech")!)
        request.httpMethod = "POST"
        request.setValue("Bearer \(openAIApiKey)", forHTTPHeaderField: "Authorization")
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.timeoutInterval = 30

        let body: [String: Any] = [
            "model": "gpt-4o-mini-tts",
            "input": text,
            "voice": voice,
            "response_format": "mp3",
            "instructions": instructions
        ]
        request.httpBody = try? JSONSerialization.data(withJSONObject: body)

        do {
            let (data, response) = try await URLSession.shared.data(for: request)
            guard !Task.isCancelled else { return }
            guard let http = response as? HTTPURLResponse else { return }
            print("[Orion TTS] OpenAI response: HTTP \(http.statusCode), \(data.count) bytes")
            guard http.statusCode == 200 else {
                if let errorText = String(data: data, encoding: .utf8) { print("[Orion TTS] OpenAI error: \(errorText)") }
                return
            }

            try AVAudioSession.sharedInstance().setCategory(.playback, mode: .voicePrompt)
            try AVAudioSession.sharedInstance().setActive(true)

            let player = try AVAudioPlayer(data: data)
            await MainActor.run { self.audioPlayer = player }
            player.delegate = self
            player.play()

            // Wait for playback to finish
            while player.isPlaying && !Task.isCancelled {
                try await Task.sleep(for: .milliseconds(100))
            }
        } catch {}
    }

    private func chunkText(_ text: String, maxLength: Int) -> [String] {
        guard text.count > maxLength else { return [text] }
        var chunks: [String] = []
        var remaining = text
        while !remaining.isEmpty {
            if remaining.count <= maxLength {
                chunks.append(remaining)
                break
            }
            // Find the last sentence boundary before maxLength
            let endIndex = remaining.index(remaining.startIndex, offsetBy: maxLength)
            let searchRange = remaining.startIndex..<endIndex
            var splitAt = endIndex
            // Look for sentence-ending punctuation
            for char: Character in [".", "!", "?", "\n"] {
                if let idx = remaining[searchRange].lastIndex(of: char) {
                    splitAt = remaining.index(after: idx)
                    break
                }
            }
            chunks.append(String(remaining[remaining.startIndex..<splitAt]).trimmingCharacters(in: .whitespacesAndNewlines))
            remaining = String(remaining[splitAt...]).trimmingCharacters(in: .whitespacesAndNewlines)
        }
        return chunks.filter { !$0.isEmpty }
    }
}

// MARK: - Delegates

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

extension SpeechService: AVAudioPlayerDelegate {
    func audioPlayerDidFinishPlaying(_ player: AVAudioPlayer, successfully flag: Bool) {
        // Playback completion is handled by the polling loop in playOpenAIChunk
    }
}

// MARK: - Voice Content Filter

enum VoiceContentFilter {
    static func filter(_ text: String) -> String {
        var result: [String] = []
        var inCodeBlock = false
        var codeBlockLines = 0
        var codeBlockLang = ""

        for line in text.components(separatedBy: "\n") {
            let trimmed = line.trimmingCharacters(in: .whitespaces)

            if trimmed.hasPrefix("```") {
                if inCodeBlock {
                    if codeBlockLines > 0 {
                        let langLabel = codeBlockLang.isEmpty ? "code" : codeBlockLang
                        result.append("[\(codeBlockLines) lines of \(langLabel)]")
                    }
                    inCodeBlock = false; codeBlockLines = 0; codeBlockLang = ""
                } else {
                    inCodeBlock = true; codeBlockLines = 0
                    codeBlockLang = String(trimmed.dropFirst(3)).trimmingCharacters(in: .whitespaces)
                }
                continue
            }

            if inCodeBlock { codeBlockLines += 1; continue }
            if trimmed.isEmpty { continue }

            var cleaned = trimmed
            cleaned = cleaned.replacingOccurrences(of: "**", with: "")
            cleaned = cleaned.replacingOccurrences(of: "__", with: "")
            if cleaned.hasPrefix("# ") { cleaned = String(cleaned.dropFirst(2)) }
            else if cleaned.hasPrefix("## ") { cleaned = String(cleaned.dropFirst(3)) }
            else if cleaned.hasPrefix("### ") { cleaned = String(cleaned.dropFirst(4)) }
            if cleaned.hasPrefix("- ") { cleaned = String(cleaned.dropFirst(2)) }
            else if cleaned.hasPrefix("* ") { cleaned = String(cleaned.dropFirst(2)) }
            cleaned = cleaned.replacingOccurrences(of: "`", with: "")
            cleaned = abbreviatePaths(cleaned)

            if !cleaned.isEmpty { result.append(cleaned) }
        }

        if inCodeBlock && codeBlockLines > 0 {
            let langLabel = codeBlockLang.isEmpty ? "code" : codeBlockLang
            result.append("[\(codeBlockLines) lines of \(langLabel)]")
        }

        return result.joined(separator: " ")
    }

    private static func abbreviatePaths(_ text: String) -> String {
        let pattern = #"(?:\/[\w.-]+){3,}"#
        guard let regex = try? NSRegularExpression(pattern: pattern) else { return text }
        let range = NSRange(text.startIndex..., in: text)
        var result = text
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
