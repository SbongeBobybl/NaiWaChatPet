$ErrorActionPreference = 'Stop'
$sourcePath = Join-Path $PSScriptRoot 'source'
$outputDirectory = Join-Path $PSScriptRoot 'ReadyToRun'
New-Item -ItemType Directory -Path $outputDirectory -Force | Out-Null
Push-Location $sourcePath
try {
    & go build -trimpath -ldflags '-s -w -H windowsgui' -o (Join-Path $outputDirectory 'NaiWaChatPet.exe') ./cmd/codexpet
    if ($LASTEXITCODE -ne 0) { throw 'Go build failed' }
} finally { Pop-Location }
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'animations') -Destination $outputDirectory -Recurse -Force
Write-Output (Join-Path $outputDirectory 'NaiWaChatPet.exe')
