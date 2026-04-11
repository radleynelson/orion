import SwiftUI

enum OrionTheme {
    static let bgPrimary = Color(hex: 0x2D2D2D)
    static let bgSecondary = Color(hex: 0x252525)
    static let bgSurface = Color(hex: 0x333333)
    static let bgHover = Color(hex: 0x3A3A3A)
    static let bgActive = Color(hex: 0x404040)
    static let bgTerminal = Color(hex: 0x1E1E1E)
    static let border = Color(hex: 0x3D3D3D)
    static let borderDim = Color(hex: 0x333333)
    static let textPrimary = Color(hex: 0xD4D4D4)
    static let textSecondary = Color(hex: 0xB0B0B0)
    static let textDim = Color(hex: 0x808080)
    static let accentBlue = Color(hex: 0x6CB6FF)
    static let accentGreen = Color(hex: 0x7EE787)
    static let accentRed = Color(hex: 0xFF7B72)
    static let accentYellow = Color(hex: 0xD29922)
    static let accentPurple = Color(hex: 0xD2A8FF)

    enum Terminal {
        static let background = UIColor(hex: 0x1E1E1E)
        static let foreground = UIColor(hex: 0xD4D4D4)
        static let cursor = UIColor(hex: 0xD4D4D4)
        static let selectionBackground = UIColor(red: 0.424, green: 0.714, blue: 1.0, alpha: 0.3)
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
