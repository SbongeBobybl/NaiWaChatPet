param(
    [ValidateSet("Debug", "Release")]
    [string]$Configuration = "Debug",

    [ValidateSet("x64", "ARM64")]
    [string]$Platform = "x64",

    [ValidateSet("Direct", "Store")]
    [string]$Distribution = "Direct",

    [ValidateSet("win-x64", "win-arm64")]
    [string]$RuntimeIdentifier,

    [switch]$Build,

    [switch]$NoStop,

    [string]$DataRoot,

    [switch]$UseProductionData
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$repoRootPath = $repoRoot.Path
$project = Join-Path $repoRoot "src\DeskBox\DeskBox.csproj"
$dotnetFromRepo = Join-Path $repoRoot ".codex-temp\dotnet10\dotnet.exe"
$dotnet = if (Test-Path -LiteralPath $dotnetFromRepo) { $dotnetFromRepo } else { "dotnet" }

[xml]$projectXml = Get-Content -LiteralPath $project -Raw
$targetFramework = $projectXml.Project.PropertyGroup |
    Where-Object { $_.TargetFramework } |
    Select-Object -First 1 -ExpandProperty TargetFramework

if ([string]::IsNullOrWhiteSpace($targetFramework)) {
    throw "Unable to read TargetFramework from $project"
}

if ([string]::IsNullOrWhiteSpace($RuntimeIdentifier)) {
    $RuntimeIdentifier = if ($Platform -eq "ARM64") { "win-arm64" } else { "win-x64" }
}

if ($UseProductionData.IsPresent -and -not [string]::IsNullOrWhiteSpace($DataRoot)) {
    throw "-DataRoot and -UseProductionData cannot be used together."
}

if ($Configuration -eq "Debug" -and
    -not $UseProductionData.IsPresent -and
    [string]::IsNullOrWhiteSpace($DataRoot)) {
    $worktreeName = Split-Path $repoRootPath -Leaf
    $safeWorktreeName = $worktreeName -replace '[^A-Za-z0-9._-]', '-'
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $pathBytes = [System.Text.Encoding]::UTF8.GetBytes($repoRootPath.ToUpperInvariant())
        $pathHash = [System.BitConverter]::ToString($sha.ComputeHash($pathBytes), 0, 4).Replace("-", "")
    }
    finally {
        $sha.Dispose()
    }

    $DataRoot = Join-Path $env:LOCALAPPDATA "DeskBox-Dev\$safeWorktreeName-$pathHash"
}

if (-not $NoStop.IsPresent) {
    Get-CimInstance Win32_Process -Filter "Name='DeskBox.exe'" |
        Where-Object {
            $_.ExecutablePath -and
            $_.ExecutablePath.StartsWith($repoRootPath, [System.StringComparison]::OrdinalIgnoreCase)
        } |
        ForEach-Object { Stop-Process -Id $_.ProcessId -Force }
    Start-Sleep -Milliseconds 800
}

if ($Build.IsPresent) {
    & $dotnet build $project `
        -c $Configuration `
        -p:Platform=$Platform `
        -p:RuntimeIdentifier=$RuntimeIdentifier `
        -p:DeskBoxDistribution=$Distribution `
        -v:minimal

    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
}

$outputDir = Join-Path $repoRoot "src\DeskBox\bin\$Platform\$Configuration\$targetFramework\$RuntimeIdentifier"
$exe = Join-Path $outputDir "DeskBox.exe"

if (-not (Test-Path -LiteralPath $exe)) {
    throw "DeskBox.exe was not found at $exe. Run this script with -Build first."
}

$previousDataRoot = [Environment]::GetEnvironmentVariable("DESKBOX_DEV_DATA_ROOT", "Process")
try {
    if ([string]::IsNullOrWhiteSpace($DataRoot)) {
        [Environment]::SetEnvironmentVariable("DESKBOX_DEV_DATA_ROOT", $null, "Process")
    }
    else {
        $DataRoot = [System.IO.Path]::GetFullPath($DataRoot)
        [Environment]::SetEnvironmentVariable("DESKBOX_DEV_DATA_ROOT", $DataRoot, "Process")
    }

    $process = Start-Process `
        -FilePath $exe `
        -WorkingDirectory $outputDir `
        -WindowStyle Hidden `
        -PassThru
}
finally {
    [Environment]::SetEnvironmentVariable("DESKBOX_DEV_DATA_ROOT", $previousDataRoot, "Process")
}

Start-Sleep -Seconds 3
$runningProcess = Get-Process -Id $process.Id -ErrorAction SilentlyContinue

[PSCustomObject]@{
    Exe = $exe
    ProcessId = $process.Id
    Running = [bool]$runningProcess
    StartTime = if ($runningProcess) { $runningProcess.StartTime } else { $null }
    DataRoot = if ([string]::IsNullOrWhiteSpace($DataRoot)) { "Production" } else { $DataRoot }
}
