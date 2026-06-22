@echo off
setlocal EnableExtensions

set "ROOT=%~dp0"
for %%I in ("%ROOT%.") do set "ROOT=%%~fI"

set "OUTDIR=%ROOT%\deploy"
if not exist "%OUTDIR%" mkdir "%OUTDIR%"

for /f %%I in ('powershell -NoProfile -Command "Get-Date -Format yyyyMMdd-HHmmss"') do set "TS=%%I"
set "ZIP=%OUTDIR%\vanillapay-website-%TS%.zip"
set "STAGE=%TEMP%\vanillapay-website-deploy-%TS%"

if exist "%STAGE%" rd /s /q "%STAGE%"
mkdir "%STAGE%"

powershell -NoProfile -ExecutionPolicy Bypass -Command ^
  "$ErrorActionPreference='Stop';" ^
  "$root=(Resolve-Path -LiteralPath '%ROOT%').Path;" ^
  "$stage='%STAGE%';" ^
  "$zip='%ZIP%';" ^
  "$dirs=@('app','config','database','extend','public','route','view');" ^
  "foreach($d in $dirs){$src=Join-Path $root $d; if(Test-Path -LiteralPath $src){Copy-Item -LiteralPath $src -Destination $stage -Recurse -Force}}" ^
  "$files=@('think','composer.json','composer.lock','README.md','LICENSE.txt','.example.env','deploy-server.sh','deploy-baseline-existing-db.sh');" ^
  "foreach($f in $files){$src=Join-Path $root $f; if(Test-Path -LiteralPath $src){Copy-Item -LiteralPath $src -Destination $stage -Force}}" ^
  "$remove=@('runtime','.phpunit.cache','tests','vendor','node_modules','.env','.git','.travis.yml','package.json','package-lock.json','tailwind.config.js','phpunit.xml','public\static\src');" ^
  "foreach($r in $remove){$p=Join-Path $stage $r; if(Test-Path -LiteralPath $p){Remove-Item -LiteralPath $p -Recurse -Force}}" ^
  "Get-ChildItem -LiteralPath $stage -Filter '.gitignore' -Recurse -Force | Remove-Item -Force;" ^
  "if(Test-Path -LiteralPath $zip){Remove-Item -LiteralPath $zip -Force};" ^
  "Add-Type -AssemblyName System.IO.Compression;" ^
  "Add-Type -AssemblyName System.IO.Compression.FileSystem;" ^
  "$archive=[System.IO.Compression.ZipFile]::Open($zip,[System.IO.Compression.ZipArchiveMode]::Create);" ^
  "try{Get-ChildItem -LiteralPath $stage -Recurse -File -Force | ForEach-Object {$relative=$_.FullName.Substring($stage.Length).TrimStart('\','/'); $entryName=$relative -replace '\\','/'; [System.IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive,$_.FullName,$entryName,[System.IO.Compression.CompressionLevel]::Optimal) | Out-Null}}finally{$archive.Dispose()};"

if errorlevel 1 (
  echo Package failed.
  if exist "%STAGE%" rd /s /q "%STAGE%"
  exit /b 1
)

rd /s /q "%STAGE%"
echo Created: %ZIP%
echo.
echo Upload this zip to the BT panel, unzip it, then run: bash deploy-server.sh
exit /b 0
