// Generated from src/tokens.json (SHA-256: 9194399b6e19e3485d1803ef1c9100b78be21bd47b62944c07001c787d528e5e). Do not edit.
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
        val chrome: String,
        val surfaceHover: String,
        val selection: String,
        val unread: String,
        val navigationSurface: String,
        val navigationAccent: String,
        val navigationControlSurface: String,
        val controlHoverSurface: String,
        val controlPressedSurface: String,
        val controlSelectedSurface: String,
        val controlSelectedHoverSurface: String,
        val controlSelectedPressedSurface: String,
        val controlSelectedText: String,
        val controlDisabledSurface: String,
        val actionPressed: String,
        val identitySlate: String,
        val identityPlum: String,
        val identityClay: String,
        val identityAmber: String,
        val onIdentity: String,
    )

    object Color {
        val light = ColorMode(canvas = "#FFFFFF", surface = "#F5F6F7", surfaceElevated = "#FFFFFF", textPrimary = "#1F2329", textSecondary = "#5A626E", textMuted = "#626A75", border = "#E4E6E9", borderStrong = "#D5D9DF", accent = "#4467B8", accentHover = "#365DA9", accentSubtle = "#EAF0FF", onAccent = "#FFFFFF", info = "#2D5F8C", infoSubtle = "#E8F0F7", warning = "#744B0B", warningSubtle = "#F8EDD8", danger = "#8B352E", dangerSubtle = "#F8E9E7", rail = "#173D35", onRail = "#FFFFFF", focusRing = "#1456F0", chrome = "#FAFBFC", surfaceHover = "#ECEFF3", selection = "#E5EDFF", unread = "#B82F38", navigationSurface = "#EAF0F8", navigationAccent = "#245BDB", navigationControlSurface = "#DCE6F7", controlHoverSurface = "#D7E4F8", controlPressedSurface = "#C9DAF3", controlSelectedSurface = "#DCE7FF", controlSelectedHoverSurface = "#D0DFF8", controlSelectedPressedSurface = "#C2D5F4", controlSelectedText = "#204FA8", controlDisabledSurface = "#E9EDF3", actionPressed = "#2C4F91", identitySlate = "#315B76", identityPlum = "#7D5A8E", identityClay = "#99654A", identityAmber = "#9A6935", onIdentity = "#FFFFFF")
        val dark = ColorMode(canvas = "#1F2227", surface = "#16181C", surfaceElevated = "#282C33", textPrimary = "#E9ECF0", textSecondary = "#B5BCC6", textMuted = "#9BA3AE", border = "#343840", borderStrong = "#464C56", accent = "#7FA2EE", accentHover = "#9AB7F3", accentSubtle = "#263250", onAccent = "#0C1730", info = "#83B6E0", infoSubtle = "#203545", warning = "#E7B864", warningSubtle = "#3D301A", danger = "#F09A91", dangerSubtle = "#422522", rail = "#111A2B", onRail = "#E9ECF0", focusRing = "#82B8FF", chrome = "#16181C", surfaceHover = "#262A31", selection = "#263250", unread = "#F09A91", navigationSurface = "#1C2029", navigationAccent = "#8FB1FF", navigationControlSurface = "#27303F", controlHoverSurface = "#2C3546", controlPressedSurface = "#35405A", controlSelectedSurface = "#263452", controlSelectedHoverSurface = "#2E3E63", controlSelectedPressedSurface = "#374A74", controlSelectedText = "#A9C3FF", controlDisabledSurface = "#24282F", actionPressed = "#B4C9F7", identitySlate = "#315B76", identityPlum = "#7D5A8E", identityClay = "#99654A", identityAmber = "#9A6935", onIdentity = "#FFFFFF")
        val highContrastLight = ColorMode(canvas = "#FFFFFF", surface = "#FFFFFF", surfaceElevated = "#FFFFFF", textPrimary = "#000000", textSecondary = "#1A1A1A", textMuted = "#333333", border = "#000000", borderStrong = "#000000", accent = "#0B3FA8", accentHover = "#082F80", accentSubtle = "#DCE7FF", onAccent = "#FFFFFF", info = "#004C9E", infoSubtle = "#DCEBFF", warning = "#5C3900", warningSubtle = "#FFF0C2", danger = "#7A1710", dangerSubtle = "#FFE3E0", rail = "#000000", onRail = "#FFFFFF", focusRing = "#005FCC", chrome = "#FFFFFF", surfaceHover = "#FFFFFF", selection = "#DCE7FF", unread = "#7A1710", navigationSurface = "#FFFFFF", navigationAccent = "#0B3FA8", navigationControlSurface = "#FFFFFF", controlHoverSurface = "#E6ECF7", controlPressedSurface = "#D0DCF0", controlSelectedSurface = "#DCE7FF", controlSelectedHoverSurface = "#C9D9FB", controlSelectedPressedSurface = "#B3C9F5", controlSelectedText = "#0B3FA8", controlDisabledSurface = "#FFFFFF", actionPressed = "#082F80", identitySlate = "#1F4560", identityPlum = "#5E3F6E", identityClay = "#6F4530", identityAmber = "#6E4A20", onIdentity = "#FFFFFF")
        val highContrastDark = ColorMode(canvas = "#000000", surface = "#000000", surfaceElevated = "#0D0D0D", textPrimary = "#FFFFFF", textSecondary = "#F2F2F2", textMuted = "#D9D9D9", border = "#FFFFFF", borderStrong = "#FFFFFF", accent = "#8CB4FF", accentHover = "#B3CDFF", accentSubtle = "#12213E", onAccent = "#000000", info = "#8FC7FF", infoSubtle = "#173A5E", warning = "#FFD166", warningSubtle = "#493700", danger = "#FF9E95", dangerSubtle = "#501A16", rail = "#000000", onRail = "#FFFFFF", focusRing = "#A8D1FF", chrome = "#000000", surfaceHover = "#0D0D0D", selection = "#12213E", unread = "#FF9E95", navigationSurface = "#000000", navigationAccent = "#8CB4FF", navigationControlSurface = "#0D0D0D", controlHoverSurface = "#19202C", controlPressedSurface = "#26324A", controlSelectedSurface = "#12213E", controlSelectedHoverSurface = "#1A2D52", controlSelectedPressedSurface = "#233B69", controlSelectedText = "#B3CDFF", controlDisabledSurface = "#0D0D0D", actionPressed = "#B3CDFF", identitySlate = "#1F4560", identityPlum = "#5E3F6E", identityClay = "#6F4530", identityAmber = "#6E4A20", onIdentity = "#FFFFFF")
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
        const val control = 0.625
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
    val radius: Map<String, Double> = mapOf("none" to 0.0, "sm" to 0.25, "md" to 0.375, "lg" to 0.5, "control" to 0.625, "xl" to 0.75, "full" to 9999.0)
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
