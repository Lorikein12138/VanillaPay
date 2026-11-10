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
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
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
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import com.vanillapay.monitor.parse.Channel

private val sans = FontFamily(Font(R.font.instrument_sans))
internal val serif = FontFamily(Font(R.font.source_serif))
internal val mono = FontFamily(Font(R.font.jetbrains_mono))

internal data class Palette(
    val muted: Color, val secondary: Color, val subtle: Color, val accent: Color,
    val good: Color, val wait: Color, val danger: Color, val dark: Boolean,
)

internal val LocalPalette = staticCompositionLocalOf<Palette> { error("MonitorTheme required") }

@Composable
internal fun MonitorTheme(dark: Boolean = isSystemInDarkTheme(), content: @Composable () -> Unit) {
    val palette = if (dark) Palette(Color(0xff30302e), Color(0xffc2c0b6), Color(0xff9c9a92),
        Color(0xffe39a7e), Color(0xffa6cb97), Color(0xffd9bc52), Color(0xffe0857a), dark = true)
    else Palette(Color(0xfff0eee6), Color(0xff5e5d59), Color(0xff73726c),
        Color(0xffb5532f), Color(0xff3d6b2e), Color(0xff8a6d12), Color(0xffa8413a), dark = false)
    val colors = if (dark) darkColorScheme(
        primary = Color(0xfffaf9f5), onPrimary = Color(0xff141413), background = Color(0xff1a1918),
        surface = Color(0xff262624), onSurface = Color(0xfffaf9f5), onBackground = Color(0xfffaf9f5),
        outline = Color(0x3dfaf9f5), outlineVariant = Color(0x1ffaf9f5), error = palette.danger,
        onSurfaceVariant = palette.secondary,
    ) else lightColorScheme(
        primary = Color(0xff141413), onPrimary = Color(0xfffaf9f5), background = Color(0xfffaf9f5),
        surface = Color.White, onSurface = Color(0xff141413), onBackground = Color(0xff141413),
        outline = Color(0x4d1f1e1d), outlineVariant = Color(0x261f1e1d), error = palette.danger,
        onSurfaceVariant = palette.secondary,
    )
    val type = Typography(
        headlineMedium = TextStyle(fontFamily = serif, fontSize = 29.sp, lineHeight = 36.sp),
        titleLarge = TextStyle(fontFamily = serif, fontSize = 23.sp, lineHeight = 30.sp),
        titleMedium = TextStyle(fontFamily = sans, fontWeight = FontWeight.Medium, fontSize = 16.sp, lineHeight = 24.sp),
        bodyLarge = TextStyle(fontFamily = sans, fontSize = 15.sp, lineHeight = 23.sp),
        bodyMedium = TextStyle(fontFamily = sans, fontSize = 14.sp, lineHeight = 21.sp),
        bodySmall = TextStyle(fontFamily = sans, fontSize = 12.sp, lineHeight = 18.sp),
        labelLarge = TextStyle(fontFamily = sans, fontWeight = FontWeight.Medium, fontSize = 14.sp, lineHeight = 21.sp),
    )
    CompositionLocalProvider(LocalPalette provides palette) {
        MaterialTheme(colorScheme = colors, typography = type) {
            Surface(Modifier.fillMaxSize(), color = colors.background, content = content)
        }
    }
}

internal enum class ChannelTone { Brand, Plain }

internal class ChannelArtwork(@get:DrawableRes val drawable: Int, private val light: Color, private val dark: Color) {
    fun color(darkTheme: Boolean): Color = if (darkTheme) dark else light
}

internal fun channelArtwork(channel: Channel): ChannelArtwork = when (channel) {
    Channel.Alipay -> ChannelArtwork(R.drawable.channel_alipay, Color(0xff1677ff), Color(0xff6ba6ff))
    Channel.WeChat -> ChannelArtwork(R.drawable.channel_wechat, Color(0xff07a855), Color(0xff47d68a))
    Channel.QQ -> ChannelArtwork(R.drawable.channel_qq, Color(0xff008cdb), Color(0xff58bfff))
}

internal fun pendingUploadColor(count: Int, accent: Color, normal: Color): Color =
    if (count > 0) accent else normal

@Composable
internal fun ChannelIcon(channel: Channel, size: Dp, tone: ChannelTone, enabled: Boolean = true) {
    val palette = LocalPalette.current
    val artwork = channelArtwork(channel)
    val painter = painterResource(artwork.drawable)
    if (tone == ChannelTone.Brand) {
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
        Text(title, fontFamily = serif, fontSize = 22.sp)
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
    content: @Composable ColumnScope.() -> Unit,
) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        color = MaterialTheme.colorScheme.surface,
        shape = RoundedCornerShape(16.dp),
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
        Column(Modifier.padding(padding), verticalArrangement = Arrangement.spacedBy(gap.dp), content = content)
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
    Button(onClick, modifier, enabled = enabled, shape = RoundedCornerShape(10.dp)) { ActionLabel(label, icon) }
}

@Composable
internal fun Secondary(label: String, onClick: () -> Unit, modifier: Modifier = Modifier, danger: Boolean = false,
    enabled: Boolean = true, compact: Boolean = false, @DrawableRes icon: Int? = null) {
    val color = if (danger) LocalPalette.current.danger else MaterialTheme.colorScheme.onSurface
    val border = if (danger) color else MaterialTheme.colorScheme.outline
    OutlinedButton(onClick, modifier, enabled = enabled, shape = RoundedCornerShape(10.dp), border = BorderStroke(1.dp, border),
        contentPadding = if (compact) PaddingValues(horizontal = 10.dp) else ButtonDefaults.ContentPadding,
        colors = ButtonDefaults.outlinedButtonColors(contentColor = color)) { ActionLabel(label, icon) }
}

@Composable
internal fun TextAction(label: String, action: () -> Unit, enabled: Boolean = true) {
    TextButton(onClick = action, enabled = enabled, contentPadding = PaddingValues(0.dp),
        colors = ButtonDefaults.textButtonColors(contentColor = LocalPalette.current.accent)) {
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
        Modifier.fillMaxWidth(), shape = RoundedCornerShape(8.dp), color = color.copy(alpha = 0.1f),
        border = BorderStroke(0.5.dp, color.copy(alpha = 0.4f)),
    ) {
        Text(text, Modifier.padding(12.dp), style = MaterialTheme.typography.bodyMedium)
    }
}
