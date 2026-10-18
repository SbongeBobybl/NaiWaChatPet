param(
    [string]$ExpectedVersion
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

function Read-RequiredMatch {
    param(
        [string]$Path,
        [string]$Pattern,
        [string]$Name
    )

    $content = Get-Content -LiteralPath $Path -Raw
    $match = [regex]::Match($content, $Pattern, [System.Text.RegularExpressions.RegexOptions]::Multiline)
    if (-not $match.Success) {
        throw "Could not read $Name from $Path"
    }

    return $match.Groups[1].Value
}

$csproj = Join-Path $root 'src\DeskBox\DeskBox.csproj'
$packageManifest = Join-Path $root 'src\DeskBox\Package.appxmanifest'
$x64Installer = Join-Path $root 'installer\DeskBox.iss'
$arm64Installer = Join-Path $root 'installer\DeskBox.arm64.iss'

$projectVersion = Read-RequiredMatch $csproj '<Version>([^<]+)</Version>' 'project version'
$projectVersionInfo = Read-RequiredMatch $csproj '<FileVersion>([^<]+)</FileVersion>' 'project file version'
$packageVersionInfo = Read-RequiredMatch $packageManifest 'Version="([^"]+)"' 'package version'
$x64Version = Read-RequiredMatch $x64Installer '#define MyAppVersion "([^"]+)"' 'x64 installer version'
$x64VersionInfo = Read-RequiredMatch $x64Installer '#define MyAppVersionInfo "([^"]+)"' 'x64 installer file version'
$arm64Version = Read-RequiredMatch $arm64Installer '#define MyAppVersion "([^"]+)"' 'ARM64 installer version'
$arm64VersionInfo = Read-RequiredMatch $arm64Installer '#define MyAppVersionInfo "([^"]+)"' 'ARM64 installer file version'

$checks = @(
    @{ Name = 'x64 installer'; Actual = $x64Version; Expected = $projectVersion }
    @{ Name = 'ARM64 installer'; Actual = $arm64Version; Expected = $projectVersion }
    @{ Name = 'project file version'; Actual = $projectVersionInfo; Expected = "$projectVersion.0" }
    @{ Name = 'package version'; Actual = $packageVersionInfo; Expected = "$projectVersion.0" }
    @{ Name = 'x64 installer file version'; Actual = $x64VersionInfo; Expected = "$projectVersion.0" }
    @{ Name = 'ARM64 installer file version'; Actual = $arm64VersionInfo; Expected = "$projectVersion.0" }
)

if (-not [string]::IsNullOrWhiteSpace($ExpectedVersion)) {
    $checks += @{ Name = 'requested release version'; Actual = $projectVersion; Expected = $ExpectedVersion }
}

$failures = @($checks | Where-Object { $_.Actual -ne $_.Expected })
if ($failures.Count -gt 0) {
    $details = $failures | ForEach-Object { "$($_.Name): actual '$($_.Actual)', expected '$($_.Expected)'" }
    throw "DeskBox direct release version mismatch:`n$($details -join [Environment]::NewLine)"
}

Write-Output "DeskBox direct release versions are consistent: $projectVersion"
