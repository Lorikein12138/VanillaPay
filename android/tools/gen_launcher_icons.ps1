# Generates launcher + in-app logo PNGs using System.Drawing.
# Run: powershell -ExecutionPolicy Bypass -File android/tools/gen_launcher_icons.ps1
#
# Launcher art source: img/VanillaClubIcon.png  -> full-bleed square art (gradient to every edge
#   + flower/leaf motif inset in the centre). It is used as the adaptive icon's BACKGROUND layer
#   (foreground is transparent) so launchers that shrink the adaptive *foreground* (MIUI/HyperOS)
#   cannot create an inner-square seam. Drawn with a tiny overscan so edges stay fully opaque.
# In-app logo:        img/VanillaClub.png        -> original circular logo with transparent corners,
#   used only inside the app (header), never as the launcher icon.
Add-Type -AssemblyName System.Drawing
$ErrorActionPreference = 'Stop'

$androidRoot = Split-Path -Parent $PSScriptRoot          # android/
$resDir = Join-Path $androidRoot 'app\src\main\res'
$icon = New-Object System.Drawing.Bitmap((Join-Path $androidRoot 'img\VanillaClubIcon.png'))
$logo = New-Object System.Drawing.Bitmap((Join-Path $androidRoot 'img\VanillaClub.png'))

function New-Surface([int]$size) {
    $bmp = New-Object System.Drawing.Bitmap($size, $size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
    $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::HighQuality
    $g.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
    $g.CompositingQuality = [System.Drawing.Drawing2D.CompositingQuality]::HighQuality
    $g.Clear([System.Drawing.Color]::Transparent)
    return @{ Bmp = $bmp; G = $g }
}

function Save-Png($bmp, [string]$relPath) {
    $path = Join-Path $resDir $relPath
    $dir = Split-Path -Parent $path
    if (-not (Test-Path $dir)) { New-Item -ItemType Directory -Force -Path $dir | Out-Null }
    $bmp.Save($path, [System.Drawing.Imaging.ImageFormat]::Png)
    Write-Host "wrote $relPath"
}

function Emit($src, [hashtable]$buckets, [string]$name, [bool]$fullBleed) {
    foreach ($d in $buckets.Keys) {
        $size = $buckets[$d]
        $s = New-Surface $size
        if ($fullBleed) {
            $pad = [math]::Max(1, [int]($size * 0.012))
            $s.G.DrawImage($src, -$pad, -$pad, $size + 2 * $pad, $size + 2 * $pad)
        } else {
            $s.G.DrawImage($src, 0, 0, $size, $size)
        }
        $s.G.Dispose()
        Save-Png $s.Bmp "drawable-$d\$name"
        $s.Bmp.Dispose()
    }
}

$adaptive = @{ 'mdpi' = 108; 'hdpi' = 162; 'xhdpi' = 216; 'xxhdpi' = 324; 'xxxhdpi' = 432 }
$legacy = @{ 'mdpi' = 48; 'hdpi' = 72; 'xhdpi' = 96; 'xxhdpi' = 144; 'xxxhdpi' = 192 }

# Adaptive BACKGROUND layer: full-bleed launcher art (foreground stays transparent in the XML).
Emit $icon $adaptive 'ic_launcher_bg.png' $true

# Legacy launcher (pre-API26): the same full-bleed art at icon sizes.
Emit $icon $legacy 'ic_launcher.png' $true

# In-app logo: original circular logo with transparent corners (header use only).
Emit $logo $legacy 'ic_logo.png' $false

$icon.Dispose()
$logo.Dispose()
Write-Host 'Launcher background + legacy + logo icons generated.'
