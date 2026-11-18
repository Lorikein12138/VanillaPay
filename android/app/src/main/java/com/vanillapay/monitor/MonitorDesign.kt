package com.vanillapay.monitor

import androidx.annotation.DrawableRes
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Shapes
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.vanillapay.monitor.parse.Channel

private val sans = FontFamily(Font(R.font.instrument_sans))
internal val serif = FontFamily(Font(R.font.source_serif))
internal val mono = FontFamily(Font(R.font.jetbrains_mono))
// The bundled Noto Serif SC subset covers the static screen and dialog headings.
private val headingSerif = FontFamily(Font(R.font.noto_serif_sc_headings))

internal data class Palette(
    val muted: Color, val secondary: Color, val subtle: Color, val accent: Color,
    val good: Color, val wait: Color, val danger: Color, val dark: Boolean,
)

internal val LocalPalette = staticCompositionLocalOf<Palette> { error("MonitorTheme required") }

private val monitorShapes = Shapes(
    extraSmall = RoundedCornerShape(4.dp),
    small = RoundedCornerShape(8.dp),
    medium = RoundedCornerShape(12.dp),
    large = RoundedCornerShape(16.dp),
    extraLarge = RoundedCornerShape(20.dp),
)

private val baseType = Typography()
private val monitorType = baseType.copy(
    displayLarge = baseType.displayLarge.copy(fontFamily = headingSerif),
    displayMedium = baseType.displayMedium.copy(fontFamily = headingSerif),
    displaySmall = baseType.displaySmall.copy(fontFamily = headingSerif),
    headlineLarge = baseType.headlineLarge.copy(fontFamily = headingSerif),
    headlineMedium = baseType.headlineMedium.copy(fontFamily = headingSerif, lineHeight = 36.sp),
    headlineSmall = baseType.headlineSmall.copy(fontFamily = headingSerif),
    titleLarge = baseType.titleLarge.copy(fontFamily = headingSerif, lineHeight = 30.sp),
    titleMedium = baseType.titleMedium.copy(fontFamily = sans),
    titleSmall = baseType.titleSmall.copy(fontFamily = sans),
    bodyLarge = baseType.bodyLarge.copy(fontFamily = sans, fontSize = 15.sp, lineHeight = 23.sp),
    bodyMedium = baseType.bodyMedium.copy(fontFamily = sans, lineHeight = 21.sp),
    bodySmall = baseType.bodySmall.copy(fontFamily = sans, lineHeight = 18.sp),
    labelLarge = baseType.labelLarge.copy(fontFamily = sans),
    labelMedium = baseType.labelMedium.copy(fontFamily = sans),
    labelSmall = baseType.labelSmall.copy(fontFamily = sans),
)

@Composable
internal fun MonitorTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    val palette = if (dark) Palette(Color(0xff4a3730), Color(0xffc9baaa), Color(0xffbdb3a3),
        Color(0xffe69a7b), Color(0xffb4d0b5), Color(0xffedc18a), Color(0xffffb4a8), dark = true)
    else Palette(Color(0xffead9ca), Color(0xff685b50), Color(0xff726b60),
        Color(0xffa94e36), Color(0xff436751), Color(0xff8c5621), Color(0xffa53c35), dark = false)
    val paper = if (dark) Color(0xff242321) else Color(0xfff5f1e8)
    val surface = if (dark) Color(0xff302e2a) else Color(0xfffffcf5)
    val ink = if (dark) Color(0xfff4eee3) else Color(0xff302821)
    val colors = (if (dark) darkColorScheme() else lightColorScheme()).copy(
        primary = palette.accent,
        onPrimary = if (dark) Color(0xff2e211b) else Color(0xfffffaf4),
        primaryContainer = palette.muted,
        onPrimaryContainer = ink,
        background = paper,
        onBackground = ink,
        surface = surface,
        onSurface = ink,
        surfaceTint = palette.accent,
        surfaceVariant = palette.muted,
        onSurfaceVariant = palette.secondary,
        surfaceContainerHigh = surface,
        surfaceContainerHighest = palette.muted,
        outline = if (dark) Color(0xffaaa194) else Color(0xff80796e),
        outlineVariant = if (dark) Color(0xff575149) else Color(0xffd8d0c2),
        error = palette.danger,
    )
    CompositionLocalProvider(LocalPalette provides palette) {
        MaterialTheme(colorScheme = colors, typography = monitorType, shapes = monitorShapes) {
            Surface(Modifier.fillMaxSize(), color = colors.background, content = content)
        }
    }
}

@Composable
internal fun MonitorDialog(
    title: String,
    onDismissRequest: () -> Unit,
    confirmButton: @Composable () -> Unit,
    dismissButton: @Composable () -> Unit,
    content: @Composable () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismissRequest,
        modifier = Modifier.padding(horizontal = 12.dp),
        title = { Text(title) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                content()
            }
        },
        confirmButton = confirmButton,
        dismissButton = dismissButton,
        shape = MaterialTheme.shapes.large,
        containerColor = MaterialTheme.colorScheme.surface,
        titleContentColor = MaterialTheme.colorScheme.onSurface,
        textContentColor = LocalPalette.current.secondary,
    )
}

internal enum class ChannelTone { Brand, Plain }

internal class ChannelArtwork(@get:DrawableRes val drawable: Int, private val light: Color, private val dark: Color) {
    fun color(darkTheme: Boolean): Color = if (darkTheme) dark else light
}

internal fun channelArtwork(channel: Channel): ChannelArtwork = when (channel) {
    Channel.Alipay -> ChannelArtwork(R.drawable.channel_alipay, Color(0xff1677ff), Color(0xff6ba6ff))
    Channel.WeChat -> ChannelArtwork(R.drawable.channel_wechat, Color(0xff07a855), Color(0xff47d68a))
}

internal fun pendingUploadColor(count: Int, accent: Color, normal: Color): Color =
    if (count > 0) accent else normal

@Composable
internal fun ChannelIcon(channel: Channel?, size: Dp, tone: ChannelTone, enabled: Boolean = true) {
    val palette = LocalPalette.current
    val artwork = channel?.let(::channelArtwork)
    val painter = painterResource(artwork?.drawable ?: R.drawable.channel_other)
    if (tone == ChannelTone.Brand && artwork != null) {
        val background = if (enabled) artwork.color(palette.dark) else palette.muted
        Box(
            Modifier.size(size).background(background, RoundedCornerShape(size / 4)),
            contentAlignment = Alignment.Center,
        ) {
            Icon(
                painter, contentDescription = null, modifier = Modifier.size(size * 0.6f),
                tint = if (enabled) Color.White else palette.subtle,
            )
        }
    } else {
        Icon(painter, contentDescription = null, modifier = Modifier.size(size), tint = palette.subtle)
    }
}

@Composable
internal fun Screen(
    topBar: (@Composable () -> Unit)? = null,
    footer: (@Composable () -> Unit)? = null,
    content: @Composable ColumnScope.() -> Unit,
) {
    Column(Modifier.fillMaxSize().safeDrawingPadding()) {
        topBar?.invoke()
        Column(
            Modifier.fillMaxWidth().weight(1f).verticalScroll(rememberScrollState()).padding(horizontal = 20.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp), content = content,
        )
        footer?.invoke()
    }
}

@Composable
internal fun BackHeader(title: String, back: () -> Unit) {
    Row(Modifier.fillMaxWidth().padding(start = 8.dp, end = 20.dp, top = 12.dp),
        verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
        HeaderIcon(R.drawable.monitor_back, "返回", back)
        Text(title, style = MaterialTheme.typography.titleLarge)
    }
}

@Composable
internal fun HeaderIcon(@DrawableRes icon: Int, label: String, click: () -> Unit) {
    IconButton(click, Modifier.size(44.dp)) {
        Icon(painterResource(icon), contentDescription = label, modifier = Modifier.size(22.dp), tint = LocalPalette.current.secondary)
    }
}

@Composable
internal fun ActionLabel(label: String, @DrawableRes icon: Int?) {
    Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
        icon?.let { Icon(painterResource(it), null, Modifier.size(16.dp)) }
        Text(label)
    }
}

@Composable
internal fun Panel(
    padding: PaddingValues = PaddingValues(15.dp), gap: Int = 8,
    receipt: Boolean = false,
    content: @Composable ColumnScope.() -> Unit,
) {
    val colors = MaterialTheme.colorScheme
    val palette = LocalPalette.current
    val paper = if (palette.dark) Color(0xff41342b) else Color(0xfff9eddb)
    Surface(
        modifier = Modifier.fillMaxWidth(),
        color = if (receipt) paper else colors.surface,
        contentColor = colors.onSurface,
        shape = MaterialTheme.shapes.medium,
        border = BorderStroke(1.dp, colors.outlineVariant),
    ) {
        Column {
            if (receipt) HorizontalDivider(thickness = 3.dp, color = palette.accent)
            Column(Modifier.padding(padding), verticalArrangement = Arrangement.spacedBy(gap.dp), content = content)
        }
    }
}

@Composable
internal fun Rule() = HorizontalDivider(thickness = 0.5.dp, color = MaterialTheme.colorScheme.outlineVariant)

@Composable
internal fun Headline(text: String) = Text(text, style = MaterialTheme.typography.headlineMedium)

@Composable
internal fun Caption(text: String) = Text(text, color = LocalPalette.current.subtle, style = MaterialTheme.typography.bodySmall)

@Composable
internal fun Paragraph(text: String) = Text(text, color = LocalPalette.current.secondary, style = MaterialTheme.typography.bodyMedium)

@Composable
internal fun Action(label: String, onClick: () -> Unit, enabled: Boolean = true, modifier: Modifier = Modifier,
    @DrawableRes icon: Int? = null) {
    Button(onClick, modifier, enabled = enabled, shape = MaterialTheme.shapes.small) { ActionLabel(label, icon) }
}

@Composable
internal fun Secondary(label: String, onClick: () -> Unit, modifier: Modifier = Modifier, danger: Boolean = false,
    enabled: Boolean = true, compact: Boolean = false, @DrawableRes icon: Int? = null) {
    val colors = MaterialTheme.colorScheme
    val color = if (danger) LocalPalette.current.danger else colors.onSurface
    val border = if (!enabled) colors.outlineVariant else if (danger) color else colors.outline
    OutlinedButton(onClick, modifier, enabled = enabled, shape = MaterialTheme.shapes.small, border = BorderStroke(1.dp, border),
        contentPadding = if (compact) PaddingValues(horizontal = 10.dp) else ButtonDefaults.ContentPadding,
        colors = ButtonDefaults.outlinedButtonColors(contentColor = color)) { ActionLabel(label, icon) }
}

@Composable
internal fun TextAction(label: String, action: () -> Unit, enabled: Boolean = true, danger: Boolean = false) {
    TextButton(onClick = action, enabled = enabled, contentPadding = PaddingValues(0.dp),
        shape = MaterialTheme.shapes.small,
        colors = ButtonDefaults.textButtonColors(contentColor = if (danger) LocalPalette.current.danger else LocalPalette.current.accent)) {
        Text(label)
    }
}

@Composable
internal fun Pill(text: String, color: Color) {
    Text(text, Modifier.background(color.copy(alpha = 0.13f), RoundedCornerShape(50)).padding(horizontal = 8.dp, vertical = 3.dp),
        color = color, style = MaterialTheme.typography.bodySmall)
}

@Composable
internal fun Callout(text: String, warning: Boolean = true) {
    val color = if (warning) LocalPalette.current.wait else LocalPalette.current.danger
    Surface(
        Modifier.fillMaxWidth(), shape = MaterialTheme.shapes.small, color = color.copy(alpha = 0.1f),
        border = BorderStroke(0.5.dp, color.copy(alpha = 0.4f)),
    ) {
        Text(text, Modifier.padding(12.dp), color = MaterialTheme.colorScheme.onSurface, style = MaterialTheme.typography.bodyMedium)
    }
}
