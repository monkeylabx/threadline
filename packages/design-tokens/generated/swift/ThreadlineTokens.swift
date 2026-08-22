// Generated from src/tokens.json (SHA-256: 03dc33ef3d4dfa9b654d6c21b3a8fa14be5577fcaf702230a4654c4f9c1ffc81). Do not edit.
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
  }

  public enum Color {
    public static let light = ColorMode(canvas: "#FFFFFF", surface: "#F5F7F5", surfaceElevated: "#FFFFFF", textPrimary: "#17211D", textSecondary: "#53605B", textMuted: "#68736E", border: "#DCE2DF", borderStrong: "#C8D0CC", accent: "#1D705E", accentHover: "#125B49", accentSubtle: "#E2F1EB", onAccent: "#FFFFFF", info: "#2D5F8C", infoSubtle: "#E8F0F7", warning: "#744B0B", warningSubtle: "#F8EDD8", danger: "#8B352E", dangerSubtle: "#F8E9E7", rail: "#173D35", onRail: "#FFFFFF", focusRing: "#005FCC")
    public static let dark = ColorMode(canvas: "#101713", surface: "#17211D", surfaceElevated: "#1E2A25", textPrimary: "#F4F7F5", textSecondary: "#CDD7D2", textMuted: "#A7B3AD", border: "#35443D", borderStrong: "#4A5B53", accent: "#6FC6AB", accentHover: "#8AD7BE", accentSubtle: "#213E34", onAccent: "#071C15", info: "#83B6E0", infoSubtle: "#203545", warning: "#E7B864", warningSubtle: "#3D301A", danger: "#F09A91", dangerSubtle: "#422522", rail: "#0B2721", onRail: "#F4F7F5", focusRing: "#82B8FF")
    public static let highContrastLight = ColorMode(canvas: "#FFFFFF", surface: "#FFFFFF", surfaceElevated: "#FFFFFF", textPrimary: "#000000", textSecondary: "#1A1A1A", textMuted: "#333333", border: "#000000", borderStrong: "#000000", accent: "#004C3E", accentHover: "#00382E", accentSubtle: "#D6F5EA", onAccent: "#FFFFFF", info: "#004C9E", infoSubtle: "#DCEBFF", warning: "#5C3900", warningSubtle: "#FFF0C2", danger: "#7A1710", dangerSubtle: "#FFE3E0", rail: "#000000", onRail: "#FFFFFF", focusRing: "#005FCC")
    public static let highContrastDark = ColorMode(canvas: "#000000", surface: "#000000", surfaceElevated: "#0D0D0D", textPrimary: "#FFFFFF", textSecondary: "#F2F2F2", textMuted: "#D9D9D9", border: "#FFFFFF", borderStrong: "#FFFFFF", accent: "#73F2C6", accentHover: "#A4FFDF", accentSubtle: "#123E31", onAccent: "#000000", info: "#8FC7FF", infoSubtle: "#173A5E", warning: "#FFD166", warningSubtle: "#493700", danger: "#FF9E95", dangerSubtle: "#501A16", rail: "#000000", onRail: "#FFFFFF", focusRing: "#A8D1FF")
  }

  public static let typography: [String: TypographyToken] = ["caption": TypographyToken(webRem: 0.75, lineHeight: 1.4, weight: 500, swiftTextStyle: "caption1", androidSp: 12), "label": TypographyToken(webRem: 0.875, lineHeight: 1.4, weight: 600, swiftTextStyle: "subheadline", androidSp: 14), "body": TypographyToken(webRem: 1, lineHeight: 1.5, weight: 400, swiftTextStyle: "body", androidSp: 16), "bodyEmphasis": TypographyToken(webRem: 1, lineHeight: 1.5, weight: 600, swiftTextStyle: "headline", androidSp: 16), "title": TypographyToken(webRem: 1.25, lineHeight: 1.3, weight: 650, swiftTextStyle: "title3", androidSp: 20), "display": TypographyToken(webRem: 1.75, lineHeight: 1.2, weight: 700, swiftTextStyle: "title1", androidSp: 28), "mono": TypographyToken(webRem: 0.875, lineHeight: 1.6, weight: 400, swiftTextStyle: "body", androidSp: 14)]
  public enum Typography {
    public static let caption = TypographyToken(webRem: 0.75, lineHeight: 1.4, weight: 500, swiftTextStyle: "caption1", androidSp: 12)
    public static let label = TypographyToken(webRem: 0.875, lineHeight: 1.4, weight: 600, swiftTextStyle: "subheadline", androidSp: 14)
    public static let body = TypographyToken(webRem: 1, lineHeight: 1.5, weight: 400, swiftTextStyle: "body", androidSp: 16)
    public static let bodyEmphasis = TypographyToken(webRem: 1, lineHeight: 1.5, weight: 600, swiftTextStyle: "headline", androidSp: 16)
    public static let title = TypographyToken(webRem: 1.25, lineHeight: 1.3, weight: 650, swiftTextStyle: "title3", androidSp: 20)
    public static let display = TypographyToken(webRem: 1.75, lineHeight: 1.2, weight: 700, swiftTextStyle: "title1", androidSp: 28)
    public static let mono = TypographyToken(webRem: 0.875, lineHeight: 1.6, weight: 400, swiftTextStyle: "body", androidSp: 14)
  }
  public enum Space {
    public static let none = 0
    public static let xxs = 0.125
    public static let xs = 0.25
    public static let sm = 0.375
    public static let md = 0.5
    public static let lg = 0.75
    public static let xl = 1
    public static let _2Xl = 1.5
    public static let _3Xl = 2
    public static let _4Xl = 3
  }
  public enum Radius {
    public static let none = 0
    public static let sm = 0.25
    public static let md = 0.375
    public static let lg = 0.5
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
  public static let space: [String: Double] = ["none": 0, "xxs": 0.125, "xs": 0.25, "sm": 0.375, "md": 0.5, "lg": 0.75, "xl": 1, "2xl": 1.5, "3xl": 2, "4xl": 3]
  public static let radius: [String: Double] = ["none": 0, "sm": 0.25, "md": 0.375, "lg": 0.5, "xl": 0.75, "full": 9999]
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
