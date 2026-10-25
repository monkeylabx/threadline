import { colorModes, nativeIdentifier, sourceHeader } from "./shared.mjs";

/** iOS adapter: typed Swift namespaces rendered from the validated source. */

function swiftString(value) {
  return JSON.stringify(value);
}

function swiftDictionary(value, valueRenderer = swiftString) {
  return `[${Object.entries(value)
    .map(([key, item]) => `${swiftString(key)}: ${valueRenderer(item)}`)
    .join(", ")}]`;
}

function swiftTypographyToken(value) {
  return `TypographyToken(webRem: ${value.webRem}, lineHeight: ${value.lineHeight}, weight: ${value.weight}, swiftTextStyle: ${swiftString(value.swiftTextStyle)}, androidSp: ${value.androidSp})`;
}

function swiftNamespace(name, values, renderer = String) {
  return `  public enum ${name} {\n${Object.entries(values)
    .map(([key, value]) => `    public static let ${nativeIdentifier(key)} = ${renderer(value)}`)
    .join("\n")}\n  }`;
}

function swiftTypes(tokens) {
  const semanticNames = Object.keys(tokens.color.light);
  const colorModeFields = semanticNames.map((name) => `    public let ${nativeIdentifier(name)}: String`).join("\n");
  const modes = colorModes
    .map(
      (mode) =>
        `    public static let ${mode} = ColorMode(${semanticNames
          .map((name) => `${nativeIdentifier(name)}: ${swiftString(tokens.color[mode][name])}`)
          .join(", ")})`,
    )
    .join("\n");
  return `  public struct TypographyToken: Sendable {
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
${colorModeFields}
  }

  public enum Color {
${modes}
  }`;
}

function swiftValues(tokens) {
  return `  public static let typography: [String: TypographyToken] = ${swiftDictionary(tokens.typography, swiftTypographyToken)}
${swiftNamespace("Typography", tokens.typography, swiftTypographyToken)}
${swiftNamespace("Space", tokens.space)}
${swiftNamespace("Radius", tokens.radius)}
${swiftNamespace("Layer", tokens.layer)}
${swiftNamespace("MotionDurationMs", tokens.motion.durationMs)}
  public static let space: [String: Double] = ${swiftDictionary(tokens.space, String)}
  public static let radius: [String: Double] = ${swiftDictionary(tokens.radius, String)}
  public static let layer: [String: Int] = ${swiftDictionary(tokens.layer, String)}
  public static let motionDurationMs: [String: Int] = ${swiftDictionary(tokens.motion.durationMs, String)}
  public static let motionEasing: [String: String] = ${swiftDictionary(tokens.motion.easing)}
  public static let reducedMotion = ReducedMotionPolicy(
    durationMultiplier: ${tokens.motion.reducedMotion.durationMultiplier},
    distanceMultiplier: ${tokens.motion.reducedMotion.distanceMultiplier},
    allowOpacity: ${tokens.motion.reducedMotion.allowOpacity},
    essentialOnly: ${tokens.motion.reducedMotion.essentialOnly}
  )
  public static let minimumContrast = ${tokens.accessibility.minimumContrast}
  public static let maximumTextScale = ${tokens.accessibility.maximumTextScale}`;
}

export function renderSwift(tokens, digest) {
  return `${sourceHeader("//", digest)}public enum ThreadlineTokens {\n${swiftTypes(tokens)}\n\n${swiftValues(tokens)}\n}\n`;
}
