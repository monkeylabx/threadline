// Generated from src/tokens.json (SHA-256: 03dc33ef3d4dfa9b654d6c21b3a8fa14be5577fcaf702230a4654c4f9c1ffc81). Do not edit.
object ThreadlineTokens {
    data class TypographyToken(
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
        val canvas: String,
        val surface: String,
        val surfaceElevated: String,
        val textPrimary: String,
        val textSecondary: String,
        val textMuted: String,
        val border: String,
        val borderStrong: String,
        val accent: String,
        val accentHover: String,
        val accentSubtle: String,
        val onAccent: String,
        val info: String,
        val infoSubtle: String,
        val warning: String,
        val warningSubtle: String,
        val danger: String,
        val dangerSubtle: String,
        val rail: String,
        val onRail: String,
        val focusRing: String,
    )

    object Color {
        val light = ColorMode(canvas = "#FFFFFF", surface = "#F5F7F5", surfaceElevated = "#FFFFFF", textPrimary = "#17211D", textSecondary = "#53605B", textMuted = "#68736E", border = "#DCE2DF", borderStrong = "#C8D0CC", accent = "#1D705E", accentHover = "#125B49", accentSubtle = "#E2F1EB", onAccent = "#FFFFFF", info = "#2D5F8C", infoSubtle = "#E8F0F7", warning = "#744B0B", warningSubtle = "#F8EDD8", danger = "#8B352E", dangerSubtle = "#F8E9E7", rail = "#173D35", onRail = "#FFFFFF", focusRing = "#005FCC")
        val dark = ColorMode(canvas = "#101713", surface = "#17211D", surfaceElevated = "#1E2A25", textPrimary = "#F4F7F5", textSecondary = "#CDD7D2", textMuted = "#A7B3AD", border = "#35443D", borderStrong = "#4A5B53", accent = "#6FC6AB", accentHover = "#8AD7BE", accentSubtle = "#213E34", onAccent = "#071C15", info = "#83B6E0", infoSubtle = "#203545", warning = "#E7B864", warningSubtle = "#3D301A", danger = "#F09A91", dangerSubtle = "#422522", rail = "#0B2721", onRail = "#F4F7F5", focusRing = "#82B8FF")
        val highContrastLight = ColorMode(canvas = "#FFFFFF", surface = "#FFFFFF", surfaceElevated = "#FFFFFF", textPrimary = "#000000", textSecondary = "#1A1A1A", textMuted = "#333333", border = "#000000", borderStrong = "#000000", accent = "#004C3E", accentHover = "#00382E", accentSubtle = "#D6F5EA", onAccent = "#FFFFFF", info = "#004C9E", infoSubtle = "#DCEBFF", warning = "#5C3900", warningSubtle = "#FFF0C2", danger = "#7A1710", dangerSubtle = "#FFE3E0", rail = "#000000", onRail = "#FFFFFF", focusRing = "#005FCC")
        val highContrastDark = ColorMode(canvas = "#000000", surface = "#000000", surfaceElevated = "#0D0D0D", textPrimary = "#FFFFFF", textSecondary = "#F2F2F2", textMuted = "#D9D9D9", border = "#FFFFFF", borderStrong = "#FFFFFF", accent = "#73F2C6", accentHover = "#A4FFDF", accentSubtle = "#123E31", onAccent = "#000000", info = "#8FC7FF", infoSubtle = "#173A5E", warning = "#FFD166", warningSubtle = "#493700", danger = "#FF9E95", dangerSubtle = "#501A16", rail = "#000000", onRail = "#FFFFFF", focusRing = "#A8D1FF")
    }

    val typography: Map<String, TypographyToken> = mapOf("caption" to TypographyToken(0.75, 1.4, 500, "caption1", 12.0), "label" to TypographyToken(0.875, 1.4, 600, "subheadline", 14.0), "body" to TypographyToken(1.0, 1.5, 400, "body", 16.0), "bodyEmphasis" to TypographyToken(1.0, 1.5, 600, "headline", 16.0), "title" to TypographyToken(1.25, 1.3, 650, "title3", 20.0), "display" to TypographyToken(1.75, 1.2, 700, "title1", 28.0), "mono" to TypographyToken(0.875, 1.6, 400, "body", 14.0))
    object Typography {
        val caption = TypographyToken(0.75, 1.4, 500, "caption1", 12.0)
        val label = TypographyToken(0.875, 1.4, 600, "subheadline", 14.0)
        val body = TypographyToken(1.0, 1.5, 400, "body", 16.0)
        val bodyEmphasis = TypographyToken(1.0, 1.5, 600, "headline", 16.0)
        val title = TypographyToken(1.25, 1.3, 650, "title3", 20.0)
        val display = TypographyToken(1.75, 1.2, 700, "title1", 28.0)
        val mono = TypographyToken(0.875, 1.6, 400, "body", 14.0)
    }
    object Space {
        const val none = 0.0
        const val xxs = 0.125
        const val xs = 0.25
        const val sm = 0.375
        const val md = 0.5
        const val lg = 0.75
        const val xl = 1.0
        const val _2Xl = 1.5
        const val _3Xl = 2.0
        const val _4Xl = 3.0
    }
    object Radius {
        const val none = 0.0
        const val sm = 0.25
        const val md = 0.375
        const val lg = 0.5
        const val xl = 0.75
        const val full = 9999.0
    }
    object Layer {
        const val base = 0
        const val sticky = 100
        const val overlay = 400
        const val modal = 600
        const val toast = 800
    }
    object MotionDurationMs {
        const val instant = 0
        const val fast = 120
        const val standard = 180
        const val slow = 260
    }
    val space: Map<String, Double> = mapOf("none" to 0.0, "xxs" to 0.125, "xs" to 0.25, "sm" to 0.375, "md" to 0.5, "lg" to 0.75, "xl" to 1.0, "2xl" to 1.5, "3xl" to 2.0, "4xl" to 3.0)
    val radius: Map<String, Double> = mapOf("none" to 0.0, "sm" to 0.25, "md" to 0.375, "lg" to 0.5, "xl" to 0.75, "full" to 9999.0)
    val layer: Map<String, Int> = mapOf("base" to 0, "sticky" to 100, "overlay" to 400, "modal" to 600, "toast" to 800)
    val motionDurationMs: Map<String, Int> = mapOf("instant" to 0, "fast" to 120, "standard" to 180, "slow" to 260)
    val motionEasing: Map<String, String> = mapOf("standard" to "cubic-bezier(0.22, 0.72, 0.25, 1)", "enter" to "cubic-bezier(0.16, 1, 0.3, 1)", "exit" to "cubic-bezier(0.4, 0, 1, 1)")
    val reducedMotion = ReducedMotionPolicy(
        durationMultiplier = 0.0,
        distanceMultiplier = 0.0,
        allowOpacity = true,
        essentialOnly = true,
    )
    const val minimumContrast: Double = 4.5
    const val maximumTextScale: Double = 2.0
}
