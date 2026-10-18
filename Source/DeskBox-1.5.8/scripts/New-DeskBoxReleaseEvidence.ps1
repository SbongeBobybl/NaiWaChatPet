<#
.SYNOPSIS
Creates deterministic integrity evidence for a DeskBox release directory.

.DESCRIPTION
Hashes files below an explicitly supplied release root and writes
release-manifest.json plus SHA256SUMS into that root. The script never invokes
Git, uploads files, or deletes files. Existing evidence files are replaced and
excluded from the artifact set so repeated runs with identical inputs are
byte-for-byte stable.

ArtifactPath values are optional paths relative to ArtifactRoot. When omitted,
all regular files below ArtifactRoot are included. Reparse points are rejected
to prevent hashing a target outside the supplied root.

ProvenancePath optionally records explicitly named lock/toolchain files relative
to ProvenanceRoot. For example:

  -ProvenanceRoot D:\project\wingezi `
  -ProvenancePath global.json,rust-toolchain.toml,native\Cargo.lock,src\DeskBox\packages.lock.json

.EXAMPLE
./scripts/New-DeskBoxReleaseEvidence.ps1 `
  -ArtifactRoot .\artifacts\release\win-x64 `
  -ProductVersion 1.4.5 `
  -Commit 0123456789abcdef `
  -RuntimeIdentifier win-x64 `
  -Channel direct
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$ArtifactRoot,

    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string]$ProductVersion,

    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string]$Commit,

    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string]$RuntimeIdentifier,

    [Parameter(Mandatory = $true)]
    [ValidateNotNullOrEmpty()]
    [string]$Channel,

    [switch]$Dirty,

    [string[]]$ArtifactPath,

    [string]$ProvenanceRoot,

    [string[]]$ProvenancePath,

    [ValidateNotNullOrEmpty()]
    [string]$ManifestName = 'release-manifest.json',

    [ValidateNotNullOrEmpty()]
    [string]$ChecksumsName = 'SHA256SUMS'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version 3.0

function Assert-LeafOutputName {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,
        [Parameter(Mandatory = $true)]
        [string]$ParameterName
    )

    if ([System.IO.Path]::IsPathRooted($Name) -or
        $Name -ne [System.IO.Path]::GetFileName($Name) -or
        $Name -in '.', '..') {
        throw "$ParameterName must be a file name without a directory: '$Name'."
    }
}

function Get-ValidatedRoot {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [string]$Name
    )

    $item = Get-Item -LiteralPath $Path -Force
    if (-not $item.PSIsContainer) {
        throw "$Name must identify a directory: '$Path'."
    }
    if (($item.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
        throw "$Name cannot be a reparse point: '$Path'."
    }
    return [System.IO.Path]::GetFullPath($item.FullName).TrimEnd('\', '/')
}

function Resolve-ContainedFile {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root,
        [Parameter(Mandatory = $true)]
        [string]$RelativePath,
        [Parameter(Mandatory = $true)]
        [string]$ParameterName
    )

    if ([string]::IsNullOrWhiteSpace($RelativePath) -or
        [System.IO.Path]::IsPathRooted($RelativePath)) {
        throw "$ParameterName entries must be non-empty paths relative to their root: '$RelativePath'."
    }

    $candidate = [System.IO.Path]::GetFullPath((Join-Path $Root $RelativePath))
    $prefix = $Root + [System.IO.Path]::DirectorySeparatorChar
    if (-not $candidate.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "$ParameterName entry escapes its supplied root: '$RelativePath'."
    }

    $current = $Root
    foreach ($segment in ($candidate.Substring($prefix.Length) -split '[\\/]')) {
        $current = Join-Path $current $segment
        $component = Get-Item -LiteralPath $current -Force
        if (($component.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0) {
            throw "$ParameterName entry traverses a reparse point: '$RelativePath'."
        }
    }

    $item = Get-Item -LiteralPath $candidate -Force
    if ($item.PSIsContainer) {
        throw "$ParameterName entry must identify a file: '$RelativePath'."
    }
    return [System.IO.Path]::GetFullPath($item.FullName)
}

function Get-NormalizedRelativePath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root,
        [Parameter(Mandatory = $true)]
        [string]$Path
    )

    $prefix = $Root + [System.IO.Path]::DirectorySeparatorChar
    if (-not $Path.StartsWith($prefix, [System.StringComparison]::OrdinalIgnoreCase)) {
        throw "Path is outside its supplied root: '$Path'."
    }
    return $Path.Substring($prefix.Length).Replace('\', '/')
}

function Get-Sha256 {
    param([Parameter(Mandatory = $true)][string]$Path)

    $stream = [System.IO.File]::Open(
        $Path,
        [System.IO.FileMode]::Open,
        [System.IO.FileAccess]::Read,
        [System.IO.FileShare]::Read)
    try {
        $sha256 = [System.Security.Cryptography.SHA256]::Create()
        try {
            $bytes = $sha256.ComputeHash($stream)
            return ([System.BitConverter]::ToString($bytes)).Replace('-', '').ToLowerInvariant()
        }
        finally {
            $sha256.Dispose()
        }
    }
    finally {
        $stream.Dispose()
    }
}

function Get-ProvenanceKind {
    param([Parameter(Mandatory = $true)][string]$RelativePath)

    $name = [System.IO.Path]::GetFileName($RelativePath)
    if ($name -ieq 'packages.lock.json') { return 'nuget-lock' }
    if ($name -ieq 'Cargo.lock') { return 'cargo-lock' }
    if ($name -like 'rust-toolchain*') { return 'rust-toolchain' }
    if ($name -ieq 'global.json') { return 'dotnet-toolchain' }
    return 'other'
}

function Write-Utf8FileDurably {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][AllowEmptyString()][string]$Content
    )

    $encoding = New-Object System.Text.UTF8Encoding($false)
    $stream = New-Object System.IO.FileStream(
        $Path,
        [System.IO.FileMode]::CreateNew,
        [System.IO.FileAccess]::Write,
        [System.IO.FileShare]::None)
    try {
        $writer = New-Object System.IO.StreamWriter($stream, $encoding)
        try {
            $writer.Write($Content)
            $writer.Flush()
            $stream.Flush($true)
        }
        finally {
            $writer.Dispose()
        }
    }
    finally {
        $stream.Dispose()
    }
}

function Install-EvidenceFile {
    param(
        [Parameter(Mandatory = $true)][string]$TemporaryPath,
        [Parameter(Mandatory = $true)][string]$DestinationPath,
        [Parameter(Mandatory = $true)][string]$BackupPath
    )

    if ([System.IO.File]::Exists($DestinationPath)) {
        [System.IO.File]::Replace($TemporaryPath, $DestinationPath, $BackupPath, $true)
        return 'replaced'
    }

    [System.IO.File]::Move($TemporaryPath, $DestinationPath)
    return 'created'
}

function Restore-EvidenceFile {
    param(
        [Parameter(Mandatory = $true)][string]$State,
        [Parameter(Mandatory = $true)][string]$DestinationPath,
        [Parameter(Mandatory = $true)][string]$BackupPath
    )

    if ($State -eq 'replaced' -and [System.IO.File]::Exists($BackupPath)) {
        if ([System.IO.File]::Exists($DestinationPath)) {
            [System.IO.File]::Replace($BackupPath, $DestinationPath, $null, $true)
        }
        else {
            [System.IO.File]::Move($BackupPath, $DestinationPath)
        }
    }
    elseif ($State -eq 'created' -and [System.IO.File]::Exists($DestinationPath)) {
        [System.IO.File]::Delete($DestinationPath)
    }
}

function Sort-RecordsByPath {
    param([Parameter(Mandatory = $true)][AllowEmptyCollection()][object[]]$Records)

    $recordsByPath = New-Object 'System.Collections.Generic.Dictionary[string,object]' `
        ([System.StringComparer]::Ordinal)
    foreach ($record in $Records) {
        if ($recordsByPath.ContainsKey([string]$record.path)) {
            throw "Duplicate evidence path: '$($record.path)'."
        }
        $recordsByPath.Add([string]$record.path, $record)
    }

    $paths = [string[]]@($recordsByPath.Keys)
    [System.Array]::Sort($paths, [System.StringComparer]::Ordinal)
    return @($paths | ForEach-Object { $recordsByPath[$_] })
}

Assert-LeafOutputName -Name $ManifestName -ParameterName 'ManifestName'
Assert-LeafOutputName -Name $ChecksumsName -ParameterName 'ChecksumsName'
if ($ManifestName -ieq $ChecksumsName) {
    throw 'ManifestName and ChecksumsName must be different.'
}

$artifactRootFull = Get-ValidatedRoot -Path $ArtifactRoot -Name 'ArtifactRoot'
$manifestPath = Join-Path $artifactRootFull $ManifestName
$checksumsPath = Join-Path $artifactRootFull $ChecksumsName
$manifestInternalPrefix = ".$ManifestName."
$checksumsInternalPrefix = ".$ChecksumsName."

$artifactFiles = @()
if ($null -ne $ArtifactPath -and $ArtifactPath.Count -gt 0) {
    foreach ($relativePath in $ArtifactPath) {
        $artifactFiles += Resolve-ContainedFile `
            -Root $artifactRootFull `
            -RelativePath $relativePath `
            -ParameterName 'ArtifactPath'
    }
}
else {
    $items = @(Get-ChildItem -LiteralPath $artifactRootFull -Force -Recurse)
    $reparsePoint = $items | Where-Object {
        ($_.Attributes -band [System.IO.FileAttributes]::ReparsePoint) -ne 0
    } | Select-Object -First 1
    if ($null -ne $reparsePoint) {
        throw "ArtifactRoot contains a reparse point, which is not allowed: '$($reparsePoint.FullName)'."
    }

    $artifactFiles = @($items | Where-Object {
        -not $_.PSIsContainer -and
        $_.FullName -ine $manifestPath -and
        $_.FullName -ine $checksumsPath -and
        -not $_.Name.StartsWith($manifestInternalPrefix, [System.StringComparison]::OrdinalIgnoreCase) -and
        -not $_.Name.StartsWith($checksumsInternalPrefix, [System.StringComparison]::OrdinalIgnoreCase)
    } | ForEach-Object { [System.IO.Path]::GetFullPath($_.FullName) })
}

$artifactRecords = foreach ($file in $artifactFiles) {
    if ($file -ieq $manifestPath -or $file -ieq $checksumsPath) {
        throw "Evidence output files cannot also be artifacts: '$file'."
    }
    $relativePath = Get-NormalizedRelativePath -Root $artifactRootFull -Path $file
    [ordered]@{
        path = $relativePath
        size = [long](Get-Item -LiteralPath $file -Force).Length
        sha256 = Get-Sha256 -Path $file
    }
}
$artifactRecords = @(Sort-RecordsByPath -Records @($artifactRecords))

$duplicateArtifact = $artifactRecords | Group-Object { $_.path.ToLowerInvariant() } |
    Where-Object Count -gt 1 | Select-Object -First 1
if ($null -ne $duplicateArtifact) {
    throw "ArtifactPath contains a duplicate entry: '$($duplicateArtifact.Group[0].path)'."
}

$provenanceRecords = @()
if ($null -ne $ProvenancePath -and $ProvenancePath.Count -gt 0) {
    if ([string]::IsNullOrWhiteSpace($ProvenanceRoot)) {
        throw 'ProvenanceRoot is required when ProvenancePath is supplied.'
    }
    $provenanceRootFull = Get-ValidatedRoot -Path $ProvenanceRoot -Name 'ProvenanceRoot'
    $provenanceRecords = foreach ($relativePath in $ProvenancePath) {
        $file = Resolve-ContainedFile `
            -Root $provenanceRootFull `
            -RelativePath $relativePath `
            -ParameterName 'ProvenancePath'
        $normalizedPath = Get-NormalizedRelativePath -Root $provenanceRootFull -Path $file
        [ordered]@{
            path = $normalizedPath
            kind = Get-ProvenanceKind -RelativePath $normalizedPath
            size = [long](Get-Item -LiteralPath $file -Force).Length
            sha256 = Get-Sha256 -Path $file
        }
    }
    $provenanceRecords = @(Sort-RecordsByPath -Records @($provenanceRecords))
}

$manifest = [ordered]@{
    schemaVersion = 1
    productVersion = $ProductVersion
    commit = $Commit
    dirty = [bool]$Dirty
    runtimeIdentifier = $RuntimeIdentifier
    channel = $Channel
    artifacts = @($artifactRecords)
    provenance = @($provenanceRecords)
}

$json = ($manifest | ConvertTo-Json -Depth 6).Replace("`r`n", "`n") + "`n"
$checksumLines = @($artifactRecords | ForEach-Object { "$($_.sha256) *$($_.path)" })
$checksums = if ($checksumLines.Count -eq 0) { '' } else { ($checksumLines -join "`n") + "`n" }

$operationId = [guid]::NewGuid().ToString('N')
$manifestTempPath = Join-Path $artifactRootFull ".$ManifestName.$operationId.tmp"
$checksumsTempPath = Join-Path $artifactRootFull ".$ChecksumsName.$operationId.tmp"
$manifestBackupPath = Join-Path $artifactRootFull ".$ManifestName.$operationId.bak"
$checksumsBackupPath = Join-Path $artifactRootFull ".$ChecksumsName.$operationId.bak"
$manifestState = 'unchanged'
$checksumsState = 'unchanged'
$installationComplete = $false
try {
    Write-Utf8FileDurably -Path $manifestTempPath -Content $json
    Write-Utf8FileDurably -Path $checksumsTempPath -Content $checksums

    $manifestState = Install-EvidenceFile `
        -TemporaryPath $manifestTempPath `
        -DestinationPath $manifestPath `
        -BackupPath $manifestBackupPath
    $checksumsState = Install-EvidenceFile `
        -TemporaryPath $checksumsTempPath `
        -DestinationPath $checksumsPath `
        -BackupPath $checksumsBackupPath
    $installationComplete = $true
}
catch {
    $installationError = $_
    if (-not $installationComplete) {
        try {
            Restore-EvidenceFile `
                -State $checksumsState `
                -DestinationPath $checksumsPath `
                -BackupPath $checksumsBackupPath
        }
        finally {
            Restore-EvidenceFile `
                -State $manifestState `
                -DestinationPath $manifestPath `
                -BackupPath $manifestBackupPath
        }
    }
    throw $installationError
}
finally {
    foreach ($ownedTemporaryPath in @(
        $manifestTempPath,
        $checksumsTempPath,
        $manifestBackupPath,
        $checksumsBackupPath)) {
        if ([System.IO.File]::Exists($ownedTemporaryPath)) {
            [System.IO.File]::Delete($ownedTemporaryPath)
        }
    }
}

Write-Output "Release evidence written for $($artifactRecords.Count) artifact(s):"
Write-Output $manifestPath
Write-Output $checksumsPath
