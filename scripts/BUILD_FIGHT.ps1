param([string]$Go = 'go', [switch]$LinuxServer)
$ErrorActionPreference = 'Stop'
$fightRoot = Split-Path $PSScriptRoot -Parent
$fightSource = Join-Path $fightRoot 'Source/NaiWaChatPet/source'
$fightOutput = Join-Path $fightRoot 'Release/NaiWa-Fight-20261021-r9'
New-Item -ItemType Directory -Force -Path $fightOutput | Out-Null
Push-Location $fightSource
try {
    & $Go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Tests failed' }
    & $Go build -buildvcs=false -trimpath -ldflags '-s -w -H windowsgui' -o (Join-Path $fightOutput 'NaiWaChatPet.exe') ./cmd/codexpet
    if ($LASTEXITCODE -ne 0) { throw 'Client build failed' }
    & $Go build -buildvcs=false -trimpath -o (Join-Path $fightOutput 'NaiWaFightServer.exe') ./cmd/fightserver
    if ($LASTEXITCODE -ne 0) { throw 'Server build failed' }
    if ($LinuxServer) {
        $fightPreviousGOOS = $env:GOOS
        $fightPreviousCGO = $env:CGO_ENABLED
        try {
            $env:GOOS = 'linux'; $env:CGO_ENABLED = '0'
            & $Go build -buildvcs=false -trimpath -o (Join-Path $fightOutput 'naiwa-fight-server-linux-amd64') ./cmd/fightserver
            if ($LASTEXITCODE -ne 0) { throw 'Linux server build failed' }
        } finally { $env:GOOS = $fightPreviousGOOS; $env:CGO_ENABLED = $fightPreviousCGO }
    }
} finally { Pop-Location }
Copy-Item -LiteralPath (Join-Path $fightRoot 'Source/NaiWaChatPet/animations') -Destination $fightOutput -Recurse -Force
Copy-Item -LiteralPath (Join-Path $fightSource 'play/combat.json') -Destination $fightOutput -Force
Write-Output $fightOutput
