import SwiftUI

enum OrionTheme {
    static let bgPrimary = Color(hex: 0x1A1A1C)
    static let bgSecondary = Color(hex: 0x212124)
    static let bgSurface = Color(hex: 0x28282C)
    static let bgHover = Color(hex: 0x2F2F34)
    static let bgActive = Color(hex: 0x35353B)
    static let bgTerminal = Color(hex: 0x131316)
    static let border = Color.white.opacity(0.12)
    static let borderDim = Color.white.opacity(0.06)
    static let textPrimary = Color(hex: 0xEAEAEC)
    static let textSecondary = Color(hex: 0xC8C8CF)
    static let textDim = Color(hex: 0x6E6E78)
    static let accentBlue = Color(hex: 0x7CA9F7)
    static let accentGreen = Color(hex: 0x8ACFA3)
    static let accentRed = Color(hex: 0xE89180)
    static let accentYellow = Color(hex: 0xE6B86B)
    static let accentPurple = Color(hex: 0xB9A3EC)
    static let accentRose = Color(hex: 0xE89BB4)
    static let accentSlate = Color(hex: 0x8E93A6)

    enum Terminal {
        static let background = UIColor(hex: 0x131316)
        static let foreground = UIColor(hex: 0xEAEAEC)
        static let cursor = UIColor(hex: 0xEAEAEC)
        static let selectionBackground = UIColor(red: 0.486, green: 0.663, blue: 0.969, alpha: 0.3)
    }
}

enum AgentSigilKind {
    case claude
    case codex
    case reviewer
    case scribe
    case shell
    case server
    case editor
    case diagnostics

    init(_ value: String) {
        let lowered = value.lowercased()
        if lowered.contains("claude") { self = .claude }
        else if lowered.contains("codex") { self = .codex }
        else if lowered.contains("reviewer") { self = .reviewer }
        else if lowered.contains("scribe") { self = .scribe }
        else if lowered.contains("server") { self = .server }
        else if lowered.contains("editor") { self = .editor }
        else if lowered.contains("diagnostics") { self = .diagnostics }
        else { self = .shell }
    }

    var color: Color {
        switch self {
        case .claude: return OrionTheme.accentPurple
        case .codex: return OrionTheme.accentGreen
        case .reviewer: return Color(hex: 0xF4B46A)
        case .scribe: return OrionTheme.accentRose
        case .shell: return OrionTheme.accentSlate
        case .server: return OrionTheme.accentBlue
        case .editor: return OrionTheme.accentYellow
        case .diagnostics: return Color(hex: 0x9BC5FF)
        }
    }

    var deepColor: Color {
        switch self {
        case .claude: return Color(hex: 0x7A5FD4)
        case .codex: return Color(hex: 0x4FA872)
        case .reviewer: return Color(hex: 0xC58330)
        case .scribe: return Color(hex: 0xB66585)
        case .shell: return Color(hex: 0x5E6376)
        case .server, .diagnostics: return Color(hex: 0x5A8BE8)
        case .editor: return Color(hex: 0xB98433)
        }
    }
}

struct AgentSigilView: View {
    let kind: AgentSigilKind
    let size: CGFloat
    let strong: Bool

    init(_ value: String, size: CGFloat = 24, strong: Bool = false) {
        self.kind = AgentSigilKind(value)
        self.size = size
        self.strong = strong
    }

    var body: some View {
        ZStack {
            RoundedRectangle(cornerRadius: max(5, size * 0.28), style: .continuous)
                .fill(background)
                .overlay {
                    if !strong {
                        RoundedRectangle(cornerRadius: max(5, size * 0.28), style: .continuous)
                            .stroke(kind.color.opacity(0.24), lineWidth: 0.7)
                    }
                }
            SigilCanvas(kind: kind, color: strong ? .white : kind.color)
                .padding(size * 0.18)
        }
        .frame(width: size, height: size)
    }

    private var background: some ShapeStyle {
        if strong {
            return AnyShapeStyle(LinearGradient(colors: [kind.color, kind.deepColor], startPoint: .topLeading, endPoint: .bottomTrailing))
        }
        return AnyShapeStyle(kind.color.opacity(0.13))
    }
}

private struct SigilCanvas: View {
    let kind: AgentSigilKind
    let color: Color

    var body: some View {
        Canvas { context, size in
            let sx = size.width / 24
            let sy = size.height / 24
            func p(_ x: CGFloat, _ y: CGFloat) -> CGPoint { CGPoint(x: x * sx, y: y * sy) }
            func rect(_ x: CGFloat, _ y: CGFloat, _ w: CGFloat, _ h: CGFloat) -> CGRect {
                CGRect(x: x * sx, y: y * sy, width: w * sx, height: h * sy)
            }

            var path = Path()
            switch kind {
            case .claude:
                path.addEllipse(in: rect(4, 4, 16, 16))
                path.addArc(center: p(16, 12), radius: 7 * sx, startAngle: .degrees(100), endAngle: .degrees(260), clockwise: false)
                context.stroke(path, with: .color(color), style: StrokeStyle(lineWidth: 1.9 * sx, lineCap: .round, lineJoin: .round))
                context.fill(Path(ellipseIn: rect(7.8, 11.3, 2.1, 2.1)), with: .color(color))
                return
            case .codex:
                path.move(to: p(6, 6)); path.addLine(to: p(12, 12)); path.addLine(to: p(6, 18))
                path.move(to: p(13, 6)); path.addLine(to: p(19, 12)); path.addLine(to: p(13, 18))
            case .reviewer:
                path.addEllipse(in: rect(4, 4, 12, 12))
                path.move(to: p(14.5, 14.5)); path.addLine(to: p(19, 19))
                context.stroke(path, with: .color(color), style: StrokeStyle(lineWidth: 1.9 * sx, lineCap: .round, lineJoin: .round))
                context.fill(Path(ellipseIn: rect(8.4, 8.4, 3.2, 3.2)), with: .color(color))
                return
            case .scribe:
                path.move(to: p(6, 5)); path.addLine(to: p(12, 5))
                path.move(to: p(9, 5)); path.addLine(to: p(9, 19))
                path.move(to: p(6, 19)); path.addLine(to: p(14, 19))
                context.stroke(path, with: .color(color), style: StrokeStyle(lineWidth: 1.9 * sx, lineCap: .round, lineJoin: .round))
                context.fill(Path(ellipseIn: rect(15.7, 6.7, 2.6, 2.6)), with: .color(color))
                return
            case .server:
                path.move(to: p(7, 5)); path.addLine(to: p(17, 5)); path.addLine(to: p(19, 12)); path.addLine(to: p(17, 19)); path.addLine(to: p(7, 19)); path.addLine(to: p(5, 12)); path.closeSubpath()
                path.move(to: p(8.5, 12)); path.addLine(to: p(15.5, 12))
            case .editor:
                path.move(to: p(7, 5)); path.addLine(to: p(17, 5))
                path.move(to: p(7, 10)); path.addLine(to: p(15, 10))
                path.move(to: p(7, 15)); path.addLine(to: p(13, 15))
                path.move(to: p(17, 14)); path.addLine(to: p(19, 16)); path.addLine(to: p(15, 20)); path.addLine(to: p(13, 20)); path.addLine(to: p(13, 18)); path.closeSubpath()
            case .diagnostics:
                path.addEllipse(in: rect(5, 5, 14, 14))
                path.move(to: p(12, 8)); path.addLine(to: p(12, 12))
                path.move(to: p(12, 16)); path.addLine(to: p(12.01, 16))
            case .shell:
                path.move(to: p(7, 7)); path.addLine(to: p(13, 12)); path.addLine(to: p(7, 17))
                path.move(to: p(14, 17)); path.addLine(to: p(19, 17))
            }
            context.stroke(path, with: .color(color), style: StrokeStyle(lineWidth: 1.9 * sx, lineCap: .round, lineJoin: .round))
        }
    }
}

struct OrionMarkView: View {
    let size: CGFloat

    init(size: CGFloat = 32) {
        self.size = size
    }

    var body: some View {
        ZStack {
            RoundedRectangle(cornerRadius: size * 0.3, style: .continuous)
                .fill(LinearGradient(colors: [Color(hex: 0x2B2D32), OrionTheme.bgPrimary], startPoint: .topLeading, endPoint: .bottomTrailing))
                .overlay(RoundedRectangle(cornerRadius: size * 0.3, style: .continuous).stroke(OrionTheme.borderDim, lineWidth: 0.7))
            GeometryReader { proxy in
                let w = proxy.size.width
                Path { path in
                    path.move(to: CGPoint(x: w * 0.23, y: w * 0.68))
                    path.addQuadCurve(to: CGPoint(x: w * 0.77, y: w * 0.41), control: CGPoint(x: w * 0.5, y: w * 0.32))
                }
                .stroke(OrionTheme.accentBlue.opacity(0.32), style: StrokeStyle(lineWidth: max(0.7, w * 0.02), lineCap: .round))
                Circle().fill(OrionTheme.accentBlue).frame(width: w * 0.11, height: w * 0.11).position(x: w * 0.32, y: w * 0.59)
                Circle().fill(OrionTheme.textPrimary).frame(width: w * 0.14, height: w * 0.14).position(x: w * 0.5, y: w * 0.5)
                Circle().fill(OrionTheme.accentBlue).frame(width: w * 0.11, height: w * 0.11).position(x: w * 0.68, y: w * 0.45)
            }
            .padding(size * 0.14)
        }
        .frame(width: size, height: size)
    }
}

extension Color {
    init(hex: UInt32) {
        let r = Double((hex >> 16) & 0xFF) / 255.0
        let g = Double((hex >> 8) & 0xFF) / 255.0
        let b = Double(hex & 0xFF) / 255.0
        self.init(red: r, green: g, blue: b)
    }
}

extension UIColor {
    convenience init(hex: UInt32) {
        let r = CGFloat((hex >> 16) & 0xFF) / 255.0
        let g = CGFloat((hex >> 8) & 0xFF) / 255.0
        let b = CGFloat(hex & 0xFF) / 255.0
        self.init(red: r, green: g, blue: b, alpha: 1.0)
    }
}
