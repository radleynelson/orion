import SwiftUI
import SwiftTerm

struct TerminalContainerView: View {
    let connection: TerminalConnection
    var body: some View {
        SwiftTermView(connection: connection)
            .ignoresSafeArea(.keyboard)
    }
}

extension Notification.Name {
    static let orionToggleKeyboard = Notification.Name("orionToggleKeyboard")
    static let orionEnableKeyboard = Notification.Name("orionEnableKeyboard")
}

final class OrionTerminalView: TerminalView {
    private let followThreshold: CGFloat = 24
    private let maxDeferredChunks = 512
    var userDetachedFromBottom = false
    private var suppressScrollSync = false
    private var deferredOutput: [[UInt8]] = []

    func configureForRemoteSession() {
        changeScrollback(5000)
        showsVerticalScrollIndicator = true
        indicatorStyle = .white
        keyboardDismissMode = .none
        isScrollEnabled = true
        bounces = true
        alwaysBounceVertical = false
        decelerationRate = .normal
        delaysContentTouches = true
        canCancelContentTouches = true
        isDirectionalLockEnabled = true
    }

    func noteUserScroll() {
        let wasDetached = userDetachedFromBottom
        userDetachedFromBottom = !isNearBottom
        if wasDetached && !userDetachedFromBottom {
            flushDeferredOutputIfNeeded()
        }
    }

    func beginDetachedScroll() {
        userDetachedFromBottom = true
    }

    func resumeLiveFollow() {
        userDetachedFromBottom = false
        applyViewportPosition(1)
        flushDeferredOutputIfNeeded()
    }

    func feedRemoteOutput(_ bytes: [UInt8]) {
        if shouldDeferRemoteOutput {
            deferRemoteOutput(bytes)
            return
        }

        flushDeferredOutputIfNeeded()
        feedVisibleOutput(bytes)
    }

    private func feedVisibleOutput(_ bytes: [UInt8]) {
        let preserveOffset = userDetachedFromBottom || isTracking || isDragging || isDecelerating
        let previousOffset = contentOffset
        let previousMaxOffset = max(0, contentSize.height - bounds.height)
        let previousPosition = previousMaxOffset > 0 ? Double(previousOffset.y / previousMaxOffset) : 1
        feed(byteArray: ArraySlice(bytes))
        guard preserveOffset else { return }
        DispatchQueue.main.async {
            if self.canScroll, previousMaxOffset > 0 {
                self.applyViewportPosition(previousPosition)
            } else {
                let maxOffset = max(0, self.contentSize.height - self.bounds.height)
                let y = min(max(0, previousOffset.y), maxOffset)
                self.performProgrammaticScroll {
                    self.setContentOffset(CGPoint(x: 0, y: y), animated: false)
                }
            }
        }
    }

    private var shouldDeferRemoteOutput: Bool {
        userDetachedFromBottom || isTracking || isDragging || isDecelerating
    }

    private func deferRemoteOutput(_ bytes: [UInt8]) {
        deferredOutput.append(bytes)
        if deferredOutput.count > maxDeferredChunks {
            deferredOutput.removeFirst(deferredOutput.count - maxDeferredChunks)
        }
    }

    private func flushDeferredOutputIfNeeded() {
        guard !shouldDeferRemoteOutput else { return }
        guard !deferredOutput.isEmpty else { return }

        let pending = deferredOutput
        deferredOutput.removeAll(keepingCapacity: true)
        for chunk in pending {
            feedVisibleOutput(chunk)
        }
    }

    private func applyViewportPosition(_ position: Double) {
        let clamped = min(max(position, 0), 1)
        if canScroll {
            performProgrammaticScroll {
                scroll(toPosition: clamped)
            }
            noteUserScroll()
            return
        }

        let maxOffset = max(0, contentSize.height - bounds.height)
        let y = maxOffset * clamped
        performProgrammaticScroll {
            setContentOffset(CGPoint(x: 0, y: y), animated: false)
        }
        noteUserScroll()
    }

    private func performProgrammaticScroll(_ update: () -> Void) {
        guard !suppressScrollSync else {
            update()
            return
        }
        suppressScrollSync = true
        update()
        DispatchQueue.main.async {
            self.suppressScrollSync = false
        }
    }

    private var isNearBottom: Bool {
        let maxOffset = max(0, contentSize.height - bounds.height)
        return maxOffset - contentOffset.y <= followThreshold
    }

    func syncBufferToViewport() {
        guard !suppressScrollSync else { return }
        guard canScroll else { return }
        let maxOffset = max(0, contentSize.height - bounds.height)
        guard maxOffset > 0 else {
            noteUserScroll()
            return
        }
        let position = min(max(Double(contentOffset.y / maxOffset), 0), 1)
        performProgrammaticScroll {
            scroll(toPosition: position)
        }
        noteUserScroll()
    }
}

struct SwiftTermView: UIViewRepresentable {
    let connection: TerminalConnection

    func makeUIView(context: Context) -> OrionTerminalView {
        let tv = OrionTerminalView(frame: .zero)
        tv.terminalDelegate = context.coordinator
        tv.delegate = context.coordinator
        context.coordinator.terminalView = tv
        context.coordinator.connection = connection

        tv.nativeBackgroundColor = OrionTheme.Terminal.background
        tv.nativeForegroundColor = OrionTheme.Terminal.foreground
        tv.caretColor = OrionTheme.Terminal.cursor
        tv.selectedTextBackgroundColor = OrionTheme.Terminal.selectionBackground

        let colors: [SwiftTerm.Color] = [
            stColor(0x13, 0x13, 0x16), stColor(0xE8, 0x91, 0x80), stColor(0x8A, 0xCF, 0xA3), stColor(0xE6, 0xB8, 0x6B),
            stColor(0x7C, 0xA9, 0xF7), stColor(0xB9, 0xA3, 0xEC), stColor(0x9B, 0xC5, 0xFF), stColor(0xEA, 0xEA, 0xEC),
            stColor(0x6E, 0x6E, 0x78), stColor(0xF2, 0xB6, 0xAA), stColor(0xBD, 0xE8, 0xCA), stColor(0xF1, 0xD4, 0x97),
            stColor(0xAF, 0xCB, 0xFA), stColor(0xD4, 0xC4, 0xF4), stColor(0xC5, 0xDC, 0xFF), stColor(0xFF, 0xFF, 0xFF),
        ]
        tv.installColors(colors)
        tv.font = UIFont.monospacedSystemFont(ofSize: 14, weight: .regular)
        tv.optionAsMetaKey = false
        tv.allowMouseReporting = false  // prevent SwiftTerm from translating touches into mouse events
        tv.autocorrectionType = .no     // prevent iOS predictive text injection ("Ankerstar" bug)
        tv.autocapitalizationType = .none
        tv.smartInsertDeleteType = .no
        tv.configureForRemoteSession()

        connection.onOutput = { [weak tv] bytes in tv?.feedRemoteOutput(bytes) }

        // Hide SwiftTerm's built-in input accessory bar
        DispatchQueue.main.async {
            if let accessory = tv.inputAccessoryView {
                accessory.isHidden = true; accessory.frame.size.height = 0; tv.reloadInputViews()
            }
        }

        context.coordinator.setupObservers()
        let panGesture = UIPanGestureRecognizer(target: context.coordinator, action: #selector(Coordinator.handlePan(_:)))
        panGesture.cancelsTouchesInView = false
        panGesture.delegate = context.coordinator
        tv.addGestureRecognizer(panGesture)

        return tv
    }

    func updateUIView(_ uiView: OrionTerminalView, context: Context) {
        if context.coordinator.connection !== connection {
            context.coordinator.connection = connection; context.coordinator.terminalView = uiView
            connection.onOutput = { [weak uiView] bytes in uiView?.feedRemoteOutput(bytes) }
        }
    }

    func makeCoordinator() -> Coordinator { Coordinator() }

    class Coordinator: NSObject, TerminalViewDelegate, UIScrollViewDelegate, UIGestureRecognizerDelegate {
        weak var terminalView: OrionTerminalView?
        var connection: TerminalConnection?
        private var lastPanTranslationY: CGFloat = 0
        private var remoteScrollRemainder: CGFloat = 0
        private let remoteScrollStep: CGFloat = 28

        func setupObservers() {
            NotificationCenter.default.addObserver(self, selector: #selector(toggleKeyboard), name: .orionToggleKeyboard, object: nil)
            NotificationCenter.default.addObserver(self, selector: #selector(enableKeyboard), name: .orionEnableKeyboard, object: nil)
        }

        @objc private func enableKeyboard() {
            guard let tv = terminalView else { return }
            tv.resumeLiveFollow()
            DispatchQueue.main.async { _ = tv.becomeFirstResponder() }
        }

        @objc private func toggleKeyboard() {
            guard let tv = terminalView else { return }
            if tv.isFirstResponder {
                _ = tv.resignFirstResponder()
            } else {
                tv.resumeLiveFollow()
                _ = tv.becomeFirstResponder()
            }
        }

        deinit { NotificationCenter.default.removeObserver(self) }

        func send(source: TerminalView, data: ArraySlice<UInt8>) {
            (source as? OrionTerminalView)?.resumeLiveFollow()
            connection?.exitCopyMode()
            connection?.sendInput(Array(data))
        }
        func sizeChanged(source: TerminalView, newCols: Int, newRows: Int) { connection?.sendResize(cols: newCols, rows: newRows) }
        func setTerminalTitle(source: TerminalView, title: String) {}
        func scrolled(source: TerminalView, position: Double) {}
        func clipboardCopy(source: TerminalView, content: Data) { if let s = String(data: content, encoding: .utf8) { UIPasteboard.general.string = s } }
        func rangeChanged(source: TerminalView, startY: Int, endY: Int) {}
        func hostCurrentDirectoryUpdate(source: TerminalView, directory: String?) {}
        func requestOpenLink(source: TerminalView, link: String, params: [String: String]) { if let url = URL(string: link) { UIApplication.shared.open(url) } }
        func bell(source: TerminalView) { UIImpactFeedbackGenerator(style: .medium).impactOccurred() }
        func iTermContent(source: TerminalView, content: ArraySlice<UInt8>) {}

        @objc func handlePan(_ gesture: UIPanGestureRecognizer) {
            guard let tv = terminalView, let connection else { return }

            switch gesture.state {
            case .began:
                lastPanTranslationY = gesture.translation(in: tv).y
                remoteScrollRemainder = 0
            case .changed:
                let translationY = gesture.translation(in: tv).y
                let deltaY = translationY - lastPanTranslationY
                lastPanTranslationY = translationY
                guard abs(deltaY) > 0 else { return }

                if tv.canScroll {
                    return
                }

                remoteScrollRemainder += deltaY
                let step = max(tv.font.lineHeight * 1.5, remoteScrollStep)
                let lines = Int(abs(remoteScrollRemainder) / step)
                guard lines > 0 else { return }

                let direction = remoteScrollRemainder > 0 ? "up" : "down"
                connection.sendScroll(direction: direction, lines: min(lines, 8))
                let consumed = CGFloat(lines) * step * (remoteScrollRemainder > 0 ? 1 : -1)
                remoteScrollRemainder -= consumed
            case .ended, .cancelled, .failed:
                lastPanTranslationY = 0
                remoteScrollRemainder = 0
                tv.noteUserScroll()
            default:
                break
            }
        }

        func gestureRecognizer(_ gestureRecognizer: UIGestureRecognizer, shouldRecognizeSimultaneouslyWith otherGestureRecognizer: UIGestureRecognizer) -> Bool {
            true
        }

        func gestureRecognizerShouldBegin(_ gestureRecognizer: UIGestureRecognizer) -> Bool {
            guard gestureRecognizer is UIPanGestureRecognizer, let tv = terminalView else { return true }
            return !tv.canScroll
        }

        func scrollViewDidScroll(_ scrollView: UIScrollView) {
            guard let tv = scrollView as? OrionTerminalView else { return }
            guard scrollView.isTracking || scrollView.isDragging || scrollView.isDecelerating else { return }
            tv.syncBufferToViewport()
        }

        func scrollViewDidEndDragging(_ scrollView: UIScrollView, willDecelerate decelerate: Bool) {
            guard let tv = scrollView as? OrionTerminalView else { return }
            if !decelerate {
                tv.syncBufferToViewport()
            }
        }

        func scrollViewDidEndDecelerating(_ scrollView: UIScrollView) {
            guard let tv = scrollView as? OrionTerminalView else { return }
            tv.syncBufferToViewport()
        }
    }
}

private func stColor(_ r: UInt16, _ g: UInt16, _ b: UInt16) -> SwiftTerm.Color {
    SwiftTerm.Color(red: r * 257, green: g * 257, blue: b * 257)
}
