import { colorModes, nativeIdentifier, sourceHeader } from "./shared.mjs";

/** Android adapter: a Kotlin object tree rendered from the validated source. */

function kotlinString(value) {
  return JSON.stringify(value);
}

function kotlinMap(value, valueRenderer = kotlinString) {
  return `mapOf(${Object.entries(value)
    .map(([key, item]) => `${kotlinString(key)} to ${valueRenderer(item)}`)
    .join(", ")})`;
}

export function kotlinDouble(value) {
  return Number.isInteger(value) ? `${value}.0` : String(value);
}

function kotlinTypographyToken(value) {
  return `TypographyToken(${kotlinDouble(value.webRem)}, ${kotlinDouble(value.lineHeight)}, ${value.weight}, ${kotlinString(value.swiftTextStyle)}, ${kotlinDouble(value.androidSp)})`;
}

function kotlinNamespace(name, values, renderer = String) {
  return `    object ${name} {\n${Object.entries(values)
    .map(([key, value]) => `        const val ${nativeIdentifier(key)} = ${renderer(value)}`)
    .join("\n")}\n    }`;
}

function kotlinTypes(tokens) {
  const semanticNames = Object.keys(tokens.color.light);
  const colorModeFields = semanticNames
    .map((name) => `        val ${nativeIdentifier(name)}: String`)
    .join(",\n");
  const modes = colorModes
    .map(
      (mode) =>
        `        val ${mode} = ColorMode(${semanticNames
          .map((name) => `${nativeIdentifier(name)} = ${kotlinString(tokens.color[mode][name])}`)
          .join(", ")})`,
    )
    .join("\n");
  return `    data class TypographyToken(
        val webRem: Double,
        val lineHeight: Double,
        val weight: Int,
        val swiftTextStyle: String,
        val androidSp: Double,
    )

    data class ReducedMotionPolicy(
        val durationMultiplier: Double,
        val distanceMultiplier: Double,
        val allowOpacity: Boolean,
        val essentialOnly: Boolean,
    )

    data class ColorMode(
${colorModeFields},
    )

    object Color {
${modes}
    }`;
}

function kotlinValues(tokens) {
  const typographyObject = Object.entries(tokens.typography)
    .map(([key, value]) => `        val ${nativeIdentifier(key)} = ${kotlinTypographyToken(value)}`)
    .join("\n");
  return `    val typography: Map<String, TypographyToken> = ${kotlinMap(tokens.typography, kotlinTypographyToken)}
    object Typography {
${typographyObject}
    }
${kotlinNamespace("Space", tokens.space, kotlinDouble)}
${kotlinNamespace("Radius", tokens.radius, kotlinDouble)}
${kotlinNamespace("Layer", tokens.layer)}
${kotlinNamespace("MotionDurationMs", tokens.motion.durationMs)}
    val space: Map<String, Double> = ${kotlinMap(tokens.space, kotlinDouble)}
    val radius: Map<String, Double> = ${kotlinMap(tokens.radius, kotlinDouble)}
    val layer: Map<String, Int> = ${kotlinMap(tokens.layer, String)}
    val motionDurationMs: Map<String, Int> = ${kotlinMap(tokens.motion.durationMs, String)}
    val motionEasing: Map<String, String> = ${kotlinMap(tokens.motion.easing)}
    val reducedMotion = ReducedMotionPolicy(
        durationMultiplier = ${tokens.motion.reducedMotion.durationMultiplier}.0,
        distanceMultiplier = ${tokens.motion.reducedMotion.distanceMultiplier}.0,
        allowOpacity = ${tokens.motion.reducedMotion.allowOpacity},
        essentialOnly = ${tokens.motion.reducedMotion.essentialOnly},
    )
    const val minimumContrast: Double = ${tokens.accessibility.minimumContrast}
    const val maximumTextScale: Double = ${tokens.accessibility.maximumTextScale}.0`;
}

export function renderKotlin(tokens, digest) {
  return `${sourceHeader("//", digest)}object ThreadlineTokens {\n${kotlinTypes(tokens)}\n\n${kotlinValues(tokens)}\n}\n`;
}
