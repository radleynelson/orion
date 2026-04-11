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

    func stopSpeaking() { synthesizer.stopSpeaking(at: .immediate); isSpeaking = false }
}

extension SpeechService: AVSpeechSynthesizerDelegate {
    func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didFinish utterance: AVSpeechUtterance) { DispatchQueue.main.async { self.isSpeaking = false } }
    func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didCancel utterance: AVSpeechUtterance) { DispatchQueue.main.async { self.isSpeaking = false } }
}
