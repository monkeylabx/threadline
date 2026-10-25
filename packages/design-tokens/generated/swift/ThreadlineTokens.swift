// Generated from src/tokens.json (SHA-256: 20394858975d49f5413eb3e72a0086e2442c4da55c41b004897e2f8f2a067242). Do not edit.
public enum ThreadlineTokens {
  public struct TypographyToken: Sendable {
    public let webRem: Double
    public let lineHeight: Double
    public let weight: Int
    public let swiftTextStyle: String
    public let androidSp: Double
  }

  public struct ReducedMotionPolicy: Sendable {
    public let durationMultiplier: Double
    public let distanceMultiplier: Double
    public let allowOpacity: Bool
    public let essentialOnly: Bool
  }

  public struct ColorMode: Sendable {
    public let canvas: String
    public let surface: String
    public let surfaceElevated: String
    public let textPrimary: String
    public let textSecondary: String
    public let textMuted: String
    public let border: String
    public let borderStrong: String
    public let accent: String
    public let accentHover: String
    public let accentSubtle: String
    public let onAccent: String
    public let info: String
    public let infoSubtle: String
    public let warning: String
    public let warningSubtle: String
    public let danger: String
    public let dangerSubtle: String
    public let rail: String
    public let onRail: String
    public let focusRing: String
    public let chrome: String
    public let surfaceHover: String
    public let selection: String
    public let unread: String
    public let navigationSurface: String
    public let navigationAccent: String
    public let navigationControlSurface: String
    public let controlHoverSurface: String
    public let controlPressedSurface: String
    public let controlSelectedSurface: String
    public let controlSelectedHoverSurface: String
    public let controlSelectedPressedSurface: String
    public let controlSelectedText: String
    public let controlDisabledSurface: String
    public let actionPressed: String
    public let identitySlate: String
    public let identityPlum: String
    public let identityClay: String
    public let identityAmber: String
    public let onIdentity: String
  }

  public enum Color {
    public static let light = ColorMode(canvas: "#FFFFFF", surface: "#F5F6F7", surfaceElevated: "#FFFFFF", textPrimary: "#1F2329", textSecondary: "#5A626E", textMuted: "#626A75", border: "#E4E6E9", borderStrong: "#D5D9DF", accent: "#4467B8", accentHover: "#365DA9", accentSubtle: "#EAF0FF", onAccent: "#FFFFFF", info: "#2D5F8C", infoSubtle: "#E8F0F7", warning: "#744B0B", warningSubtle: "#F8EDD8", danger: "#8B352E", dangerSubtle: "#F8E9E7", rail: "#173D35", onRail: "#FFFFFF", focusRing: "#1456F0", chrome: "#FAFBFC", surfaceHover: "#ECEFF3", selection: "#E5EDFF", unread: "#B82F38", navigationSurface: "#EAF0F8", navigationAccent: "#245BDB", navigationControlSurface: "#DCE6F7", controlHoverSurface: "#D7E4F8", controlPressedSurface: "#C9DAF3", controlSelectedSurface: "#DCE7FF", controlSelectedHoverSurface: "#D0DFF8", controlSelectedPressedSurface: "#C2D5F4", controlSelectedText: "#204FA8", controlDisabledSurface: "#E9EDF3", actionPressed: "#2C4F91", identitySlate: "#315B76", identityPlum: "#7D5A8E", identityClay: "#99654A", identityAmber: "#9A6935", onIdentity: "#FFFFFF")
    public static let dark = ColorMode(canvas: "#1F2227", surface: "#16181C", surfaceElevated: "#282C33", textPrimary: "#E9ECF0", textSecondary: "#B5BCC6", textMuted: "#9BA3AE", border: "#343840", borderStrong: "#464C56", accent: "#7FA2EE", accentHover: "#9AB7F3", accentSubtle: "#263250", onAccent: "#0C1730", info: "#83B6E0", infoSubtle: "#203545", warning: "#E7B864", warningSubtle: "#3D301A", danger: "#F09A91", dangerSubtle: "#422522", rail: "#111A2B", onRail: "#E9ECF0", focusRing: "#82B8FF", chrome: "#16181C", surfaceHover: "#262A31", selection: "#263250", unread: "#F09A91", navigationSurface: "#1C2029", navigationAccent: "#8FB1FF", navigationControlSurface: "#27303F", controlHoverSurface: "#2C3546", controlPressedSurface: "#35405A", controlSelectedSurface: "#263452", controlSelectedHoverSurface: "#2E3E63", controlSelectedPressedSurface: "#374A74", controlSelectedText: "#A9C3FF", controlDisabledSurface: "#24282F", actionPressed: "#B4C9F7", identitySlate: "#315B76", identityPlum: "#7D5A8E", identityClay: "#99654A", identityAmber: "#9A6935", onIdentity: "#FFFFFF")
    public static let highContrastLight = ColorMode(canvas: "#FFFFFF", surface: "#FFFFFF", surfaceElevated: "#FFFFFF", textPrimary: "#000000", textSecondary: "#1A1A1A", textMuted: "#333333", border: "#000000", borderStrong: "#000000", accent: "#0B3FA8", accentHover: "#082F80", accentSubtle: "#DCE7FF", onAccent: "#FFFFFF", info: "#004C9E", infoSubtle: "#DCEBFF", warning: "#5C3900", warningSubtle: "#FFF0C2", danger: "#7A1710", dangerSubtle: "#FFE3E0", rail: "#000000", onRail: "#FFFFFF", focusRing: "#005FCC", chrome: "#FFFFFF", surfaceHover: "#FFFFFF", selection: "#DCE7FF", unread: "#7A1710", navigationSurface: "#FFFFFF", navigationAccent: "#0B3FA8", navigationControlSurface: "#FFFFFF", controlHoverSurface: "#E6ECF7", controlPressedSurface: "#D0DCF0", controlSelectedSurface: "#DCE7FF", controlSelectedHoverSurface: "#C9D9FB", controlSelectedPressedSurface: "#B3C9F5", controlSelectedText: "#0B3FA8", controlDisabledSurface: "#FFFFFF", actionPressed: "#082F80", identitySlate: "#1F4560", identityPlum: "#5E3F6E", identityClay: "#6F4530", identityAmber: "#6E4A20", onIdentity: "#FFFFFF")
    public static let highContrastDark = ColorMode(canvas: "#000000", surface: "#000000", surfaceElevated: "#0D0D0D", textPrimary: "#FFFFFF", textSecondary: "#F2F2F2", textMuted: "#D9D9D9", border: "#FFFFFF", borderStrong: "#FFFFFF", accent: "#8CB4FF", accentHover: "#B3CDFF", accentSubtle: "#12213E", onAccent: "#000000", info: "#8FC7FF", infoSubtle: "#173A5E", warning: "#FFD166", warningSubtle: "#493700", danger: "#FF9E95", dangerSubtle: "#501A16", rail: "#000000", onRail: "#FFFFFF", focusRing: "#A8D1FF", chrome: "#000000", surfaceHover: "#0D0D0D", selection: "#12213E", unread: "#FF9E95", navigationSurface: "#000000", navigationAccent: "#8CB4FF", navigationControlSurface: "#0D0D0D", controlHoverSurface: "#19202C", controlPressedSurface: "#26324A", controlSelectedSurface: "#12213E", controlSelectedHoverSurface: "#1A2D52", controlSelectedPressedSurface: "#233B69", controlSelectedText: "#B3CDFF", controlDisabledSurface: "#0D0D0D", actionPressed: "#B3CDFF", identitySlate: "#1F4560", identityPlum: "#5E3F6E", identityClay: "#6F4530", identityAmber: "#6E4A20", onIdentity: "#FFFFFF")
  }

  public static let typography: [String: TypographyToken] = ["caption": TypographyToken(webRem: 0.75, lineHeight: 1.4, weight: 500, swiftTextStyle: "caption1", androidSp: 12), "label": TypographyToken(webRem: 0.875, lineHeight: 1.4, weight: 600, swiftTextStyle: "subheadline", androidSp: 14), "bodyCompact": TypographyToken(webRem: 0.8125, lineHeight: 1.7, weight: 400, swiftTextStyle: "footnote", androidSp: 13), "body": TypographyToken(webRem: 1, lineHeight: 1.5, weight: 400, swiftTextStyle: "body", androidSp: 16), "bodyEmphasis": TypographyToken(webRem: 1, lineHeight: 1.5, weight: 600, swiftTextStyle: "headline", androidSp: 16), "title": TypographyToken(webRem: 1.25, lineHeight: 1.3, weight: 650, swiftTextStyle: "title3", androidSp: 20), "display": TypographyToken(webRem: 1.75, lineHeight: 1.2, weight: 700, swiftTextStyle: "title1", androidSp: 28), "mono": TypographyToken(webRem: 0.875, lineHeight: 1.6, weight: 400, swiftTextStyle: "body", androidSp: 14)]
  public enum Typography {
    public static let caption = TypographyToken(webRem: 0.75, lineHeight: 1.4, weight: 500, swiftTextStyle: "caption1", androidSp: 12)
    public static let label = TypographyToken(webRem: 0.875, lineHeight: 1.4, weight: 600, swiftTextStyle: "subheadline", androidSp: 14)
    public static let bodyCompact = TypographyToken(webRem: 0.8125, lineHeight: 1.7, weight: 400, swiftTextStyle: "footnote", androidSp: 13)
    public static let body = TypographyToken(webRem: 1, lineHeight: 1.5, weight: 400, swiftTextStyle: "body", androidSp: 16)
    public static let bodyEmphasis = TypographyToken(webRem: 1, lineHeight: 1.5, weight: 600, swiftTextStyle: "headline", androidSp: 16)
    public static let title = TypographyToken(webRem: 1.25, lineHeight: 1.3, weight: 650, swiftTextStyle: "title3", androidSp: 20)
    public static let display = TypographyToken(webRem: 1.75, lineHeight: 1.2, weight: 700, swiftTextStyle: "title1", androidSp: 28)
    public static let mono = TypographyToken(webRem: 0.875, lineHeight: 1.6, weight: 400, swiftTextStyle: "body", androidSp: 14)
  }
  public enum Space {
    public static let none = 0
    public static let xxs = 0.125
    public static let composerFieldGap = 0.1875
    public static let xs = 0.25
    public static let sm = 0.375
    public static let md = 0.5
    public static let composerToolbarInset = 0.625
    public static let lg = 0.75
    public static let composerInputInset = 0.8125
    public static let composerIconWidth = 0.8125
    public static let xl = 1
    public static let composerIconHeight = 1.125
    public static let composerOuterInset = 1.25
    public static let _2Xl = 1.5
    public static let composerControl = 1.625
    public static let _3Xl = 2
    public static let composerFieldMin = 2.6875
    public static let _4Xl = 3
    public static let composerFieldMax = 10.125
  }
  public enum Radius {
    public static let none = 0
    public static let sm = 0.25
    public static let composerTool = 0.3125
    public static let md = 0.375
    public static let lg = 0.5
    public static let composerSend = 0.5625
    public static let control = 0.625
    public static let xl = 0.75
    public static let full = 9999
  }
  public enum Layer {
    public static let base = 0
    public static let sticky = 100
    public static let overlay = 400
    public static let modal = 600
    public static let toast = 800
  }
  public enum MotionDurationMs {
    public static let instant = 0
    public static let fast = 120
    public static let standard = 180
    public static let slow = 260
  }
  public static let space: [String: Double] = ["none": 0, "xxs": 0.125, "composerFieldGap": 0.1875, "xs": 0.25, "sm": 0.375, "md": 0.5, "composerToolbarInset": 0.625, "lg": 0.75, "composerInputInset": 0.8125, "composerIconWidth": 0.8125, "xl": 1, "composerIconHeight": 1.125, "composerOuterInset": 1.25, "2xl": 1.5, "composerControl": 1.625, "3xl": 2, "composerFieldMin": 2.6875, "4xl": 3, "composerFieldMax": 10.125]
  public static let radius: [String: Double] = ["none": 0, "sm": 0.25, "composerTool": 0.3125, "md": 0.375, "lg": 0.5, "composerSend": 0.5625, "control": 0.625, "xl": 0.75, "full": 9999]
  public static let layer: [String: Int] = ["base": 0, "sticky": 100, "overlay": 400, "modal": 600, "toast": 800]
  public static let motionDurationMs: [String: Int] = ["instant": 0, "fast": 120, "standard": 180, "slow": 260]
  public static let motionEasing: [String: String] = ["standard": "cubic-bezier(0.22, 0.72, 0.25, 1)", "enter": "cubic-bezier(0.16, 1, 0.3, 1)", "exit": "cubic-bezier(0.4, 0, 1, 1)"]
  public static let reducedMotion = ReducedMotionPolicy(
    durationMultiplier: 0,
    distanceMultiplier: 0,
    allowOpacity: true,
    essentialOnly: true
  )
  public static let minimumContrast = 4.5
  public static let maximumTextScale = 2
}
