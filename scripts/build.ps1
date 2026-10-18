param([Parameter(Mandatory=$true)][string]$IsccPath)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
if (-not (Test-Path -LiteralPath $IsccPath -PathType Leaf)) { throw 'ISCC.exe not found' }
New-Item -ItemType Directory -Force -Path (Join-Path $root 'build') | Out-Null
Push-Location (Join-Path $root 'Source/NaiWaChatPet/source')
try {
    & go build -trimpath -ldflags '-H=windowsgui' -o (Join-Path $root 'build/NaiWaChatPet.exe') ./cmd/codexpet
    if ($LASTEXITCODE -ne 0) { throw 'Pet build failed' }
} finally { Pop-Location }
& powershell -NoProfile -ExecutionPolicy Bypass -File (Join-Path $root 'Source/DeskBox-1.5.8/scripts/publish-aot-retail.ps1') -Platform x64
if ($LASTEXITCODE -ne 0) { throw 'Organizer AOT publish failed' }
& python (Join-Path $PSScriptRoot 'release.py') --payload (Join-Path $root 'Source/DeskBox-1.5.8/.artifacts/aot-retail/win-x64/publish') --pet (Join-Path $root 'build/NaiWaChatPet.exe') --iscc $IsccPath
if ($LASTEXITCODE -ne 0) { throw 'Release failed' }
