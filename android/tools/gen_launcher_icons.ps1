# Generates launcher PNGs from img/VanillaClub.png using System.Drawing.
# Run from anywhere: powershell -ExecutionPolicy Bypass -File android/tools/gen_launcher_icons.ps1
Add-Type -AssemblyName System.Drawing
$ErrorActionPreference = 'Stop'

$androidRoot = Split-Path -Parent $PSScriptRoot          # android/
$src = Join-Path $androidRoot 'img\VanillaClub.png'
$resDir = Join-Path $androidRoot 'app\src\main\res'
$logo = New-Object System.Drawing.Bitmap($src)

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

# Adaptive foreground: whole logo scaled to 64% centered on a transparent 108-canvas.
$fg = @{ 'mdpi' = 108; 'hdpi' = 162; 'xhdpi' = 216; 'xxhdpi' = 324; 'xxxhdpi' = 432 }
foreach ($d in $fg.Keys) {
    $size = $fg[$d]
    $s = New-Surface $size
    $target = [int]($size * 0.64)
    $offset = [int](($size - $target) / 2)
    $s.G.DrawImage($logo, $offset, $offset, $target, $target)
    $s.G.Dispose()
    Save-Png $s.Bmp "drawable-$d\ic_launcher_foreground.png"
    $s.Bmp.Dispose()
}

# Legacy launcher (pre-API26): the finished circular logo (transparent corners) at icon size.
$lg = @{ 'mdpi' = 48; 'hdpi' = 72; 'xhdpi' = 96; 'xxhdpi' = 144; 'xxxhdpi' = 192 }
foreach ($d in $lg.Keys) {
    $size = $lg[$d]
    $s = New-Surface $size
    $s.G.DrawImage($logo, 0, 0, $size, $size)
    $s.G.Dispose()
    Save-Png $s.Bmp "drawable-$d\ic_launcher.png"
    $s.Bmp.Dispose()
}

$logo.Dispose()
Write-Host 'Launcher icons generated.'
