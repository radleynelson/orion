import SwiftUI
import SwiftTerm

struct TerminalContainerView: View {
    let connection: TerminalConnection
    var body: some View {
        ZStack(alignment: .trailing) {
            SwiftTermView(connection: connection)
                .ignoresSafeArea(.keyboard)
            ScrollJoystick(connection: connection)
        }
    }
}

// MARK: - Scroll Joystick (matches PWA's right-edge scroll handle)

struct ScrollJoystick: View {
    let connection: TerminalConnection
    @State private var isDragging = false
    @State private var thumbOffset: CGFloat = 0 // offset from center
    @State private var scrollTimer: Timer?

    private let trackWidth: CGFloat = 28
    private let thumbHeight: CGFloat = 50
    private let deadZone: CGFloat = 30

    var body: some View {
        GeometryReader { geo in
            let centerY = geo.size.height / 2
            ZStack {
                // Track (invisible but tappable)
                Rectangle()
                    .fill(Color.clear)
                    .frame(width: trackWidth)
                    .contentShape(Rectangle())

                // Thumb
                RoundedRectangle(cornerRadius: 8)
                    .fill(isDragging ? Color.white.opacity(0.35) : Color.white.opacity(0.15))
                    .frame(width: 14, height: thumbHeight)
                    .offset(y: thumbOffset)
            }
            .frame(width: trackWidth, height: geo.size.height)
            .gesture(
                DragGesture(minimumDistance: 0)
                    .onChanged { value in
                        if !isDragging {
                            isDragging = true
                            startScrollLoop(trackHeight: geo.size.height)
                        }
                        // Offset from center
                        let dragY = value.location.y - centerY
                        let maxOffset = (geo.size.height / 2) - 20
                        thumbOffset = max(-maxOffset, min(maxOffset, dragY))
                    }
                    .onEnded { _ in
                        isDragging = false
                        thumbOffset = 0
                        stopScrollLoop()
                    }
            )
        }
        .frame(width: trackWidth)
    }

    private func startScrollLoop(trackHeight: CGFloat) {
        scrollTimer = Timer.scheduledTimer(withTimeInterval: 0.08, repeats: true) { _ in
            guard isDragging else { return }

            let offset = thumbOffset
            guard abs(offset) > deadZone else { return }

            let direction = offset > 0 ? "down" : "up"
            let distance = abs(offset) - deadZone
            let maxDistance = (trackHeight / 2) - deadZone
            let ratio = min(1, distance / maxDistance)
            // Quadratic curve — slow at start, fast only at extremes
            let speed = max(1, Int(ratio * ratio * 8))

            connection.sendScroll(direction: direction, lines: speed)
        }
    }

    private func stopScrollLoop() {
        scrollTimer?.invalidate()
        scrollTimer = nil
    }
}

extension Notification.Name {
    static let orionToggleKeyboard = Notification.Name("orionToggleKeyboard")
    static let orionRefocusTerminal = Notification.Name("orionRefocusTerminal")
}

struct SwiftTermView: UIViewRepresentable {
    let connection: TerminalConnection

    func makeUIView(context: Context) -> TerminalView {
        let tv = TerminalView(frame: .zero)
        tv.terminalDelegate = context.coordinator
        context.coordinator.terminalView = tv
        context.coordinator.connection = connection

        tv.nativeBackgroundColor = OrionTheme.Terminal.background
        tv.nativeForegroundColor = OrionTheme.Terminal.foreground
        tv.caretColor = OrionTheme.Terminal.cursor
        tv.selectedTextBackgroundColor = OrionTheme.Terminal.selectionBackground

        let colors: [SwiftTerm.Color] = [
            stColor(0x1E, 0x1E, 0x1E), stColor(0xFF, 0x7B, 0x72), stColor(0x7E, 0xE7, 0x87), stColor(0xD2, 0x99, 0x22),
            stColor(0x6C, 0xB6, 0xFF), stColor(0xD2, 0xA8, 0xFF), stColor(0x76, 0xE3, 0xEA), stColor(0xD4, 0xD4, 0xD4),
            stColor(0x80, 0x80, 0x80), stColor(0xFF, 0x9C, 0x94), stColor(0xA5, 0xF0, 0xB0), stColor(0xE8, 0xC5, 0x47),
            stColor(0x8F, 0xCE, 0xFF), stColor(0xE0, 0xC0, 0xFF), stColor(0x9A, 0xEF, 0xF0), stColor(0xFF, 0xFF, 0xFF),
        ]
        tv.installColors(colors)
        tv.font = UIFont.monospacedSystemFont(ofSize: 14, weight: .regular)
        tv.optionAsMetaKey = false
        tv.allowMouseReporting = false  // prevent SwiftTerm from translating touches into mouse events
        tv.autocorrectionType = .no     // prevent iOS predictive text injection ("Ankerstar" bug)
        tv.autocapitalizationType = .none
        tv.smartInsertDeleteType = .no

        connection.onOutput = { [weak tv] bytes in tv?.feed(byteArray: ArraySlice(bytes)) }

        // Hide SwiftTerm's built-in input accessory bar
        DispatchQueue.main.async {
            if let accessory = tv.inputAccessoryView {
                accessory.isHidden = true; accessory.frame.size.height = 0; tv.reloadInputViews()
            }
        }

        context.coordinator.setupKeyboardSuppression()

        // Scroll gesture — sends to tmux, not SwiftTerm's local buffer
        let panGesture = UIPanGestureRecognizer(target: context.coordinator, action: #selector(Coordinator.handleScroll(_:)))
        panGesture.delegate = context.coordinator
        tv.addGestureRecognizer(panGesture)

        // Make SwiftTerm's gestures yield to our pan
        for gesture in tv.gestureRecognizers ?? [] where gesture !== panGesture {
            gesture.require(toFail: panGesture)
        }

        return tv
    }

    func updateUIView(_ uiView: TerminalView, context: Context) {
        if context.coordinator.connection !== connection {
            context.coordinator.connection = connection; context.coordinator.terminalView = uiView
            connection.onOutput = { [weak uiView] bytes in uiView?.feed(byteArray: ArraySlice(bytes)) }
        }
    }

    func makeCoordinator() -> Coordinator { Coordinator() }

    class Coordinator: NSObject, TerminalViewDelegate, UIGestureRecognizerDelegate {
        weak var terminalView: TerminalView?
        var connection: TerminalConnection?
        var keyboardEnabled = false
        var isScrolling = false
        private var lastScrollTime: TimeInterval = 0

        func setupKeyboardSuppression() {
            NotificationCenter.default.addObserver(self, selector: #selector(keyboardWillShow), name: UIResponder.keyboardWillShowNotification, object: nil)
            NotificationCenter.default.addObserver(self, selector: #selector(keyboardDidHide), name: UIResponder.keyboardDidHideNotification, object: nil)
            NotificationCenter.default.addObserver(self, selector: #selector(toggleKeyboard), name: .orionToggleKeyboard, object: nil)
            NotificationCenter.default.addObserver(self, selector: #selector(refocusTerminal), name: .orionRefocusTerminal, object: nil)
        }

        @objc private func keyboardWillShow(_ n: Notification) { if !keyboardEnabled { DispatchQueue.main.async { self.terminalView?.resignFirstResponder() } } }
        @objc private func keyboardDidHide(_ n: Notification) { keyboardEnabled = false }
        @objc private func toggleKeyboard() {
            guard let tv = terminalView else { return }
            if tv.isFirstResponder && keyboardEnabled { keyboardEnabled = false; tv.resignFirstResponder() }
            else {
                keyboardEnabled = true; tv.becomeFirstResponder()
                // Exit tmux copy mode so keyboard input goes to the shell/Claude, not tmux's command line
                connection?.exitCopyMode()
            }
        }
        /// Reclaim first responder for the terminal after dictation or other interruptions
        @objc private func refocusTerminal() {
            guard let tv = terminalView, !tv.isFirstResponder, keyboardEnabled else { return }
            DispatchQueue.main.async { tv.becomeFirstResponder() }
        }

        @objc func handleScroll(_ gesture: UIPanGestureRecognizer) {
            switch gesture.state {
            case .began: isScrolling = true
            case .ended, .cancelled, .failed: isScrolling = false; return
            default: break
            }
            guard let connection else { return }
            let now = CACurrentMediaTime()
            guard now - lastScrollTime > 0.06 else { return }
            lastScrollTime = now
            let translation = gesture.translation(in: gesture.view)
            gesture.setTranslation(.zero, in: gesture.view)
            let deltaY = translation.y
            guard abs(deltaY) > 3 else { return }
            let direction = deltaY > 0 ? "up" : "down"
            let lines = max(1, Int(abs(deltaY) / 16))
            connection.sendScroll(direction: direction, lines: lines)
        }

        func gestureRecognizer(_ gestureRecognizer: UIGestureRecognizer, shouldRecognizeSimultaneouslyWith otherGestureRecognizer: UIGestureRecognizer) -> Bool { false }

        deinit { NotificationCenter.default.removeObserver(self) }

        func send(source: TerminalView, data: ArraySlice<UInt8>) { guard !isScrolling else { return }; connection?.exitCopyMode(); connection?.sendInput(Array(data)) }
        func sizeChanged(source: TerminalView, newCols: Int, newRows: Int) { connection?.sendResize(cols: newCols, rows: newRows) }
        func setTerminalTitle(source: TerminalView, title: String) {}
        func scrolled(source: TerminalView, position: Double) {}
        func clipboardCopy(source: TerminalView, content: Data) { if let s = String(data: content, encoding: .utf8) { UIPasteboard.general.string = s } }
        func rangeChanged(source: TerminalView, startY: Int, endY: Int) {}
        func hostCurrentDirectoryUpdate(source: TerminalView, directory: String?) {}
        func requestOpenLink(source: TerminalView, link: String, params: [String: String]) { if let url = URL(string: link) { UIApplication.shared.open(url) } }
        func bell(source: TerminalView) { UIImpactFeedbackGenerator(style: .medium).impactOccurred() }
        func iTermContent(source: TerminalView, content: ArraySlice<UInt8>) {}
    }
}

private func stColor(_ r: UInt16, _ g: UInt16, _ b: UInt16) -> SwiftTerm.Color {
    SwiftTerm.Color(red: r * 257, green: g * 257, blue: b * 257)
}
