<#
.SYNOPSIS
Creates a read-only integrity inventory for a source tree and a DeskBox
destination tree after an interrupted copy or move.

.DESCRIPTION
The script never changes either input root. It compares entries by relative
path, type, file length and last-write time, writes a CSV report plus a JSON
summary, and can hash same-size mismatch candidates below a configurable size.
#>
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [string]$SourceRoot,

    [Parameter(Mandatory = $true)]
    [string]$DestinationRoot,

    [Parameter(Mandatory = $true)]
    [string]$OutputDirectory,

    [switch]$HashMismatches,

    [long]$MaxHashBytes = 536870912,

    [double]$TimestampToleranceSeconds = 2
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Resolve-InventoryRoot {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,

        [Parameter(Mandatory = $true)]
        [string]$Label
    )

    if (-not (Test-Path -LiteralPath $Path -PathType Container)) {
        throw "$Label does not exist or is not a directory: $Path"
    }

    $resolvedPath = [System.IO.Path]::GetFullPath(
        (Resolve-Path -LiteralPath $Path).Path)
    return $resolvedPath.TrimEnd([char[]]@(
        [System.IO.Path]::DirectorySeparatorChar,
        [System.IO.Path]::AltDirectorySeparatorChar))
}

function Test-PathInsideRoot {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Candidate,

        [Parameter(Mandatory = $true)]
        [string]$Root
    )

    $normalizedCandidate = [System.IO.Path]::GetFullPath($Candidate).TrimEnd(
        [char[]]@(
            [System.IO.Path]::DirectorySeparatorChar,
            [System.IO.Path]::AltDirectorySeparatorChar))
    if ($normalizedCandidate.Equals(
            $Root,
            [System.StringComparison]::OrdinalIgnoreCase)) {
        return $true
    }

    $rootPrefix = $Root + [System.IO.Path]::DirectorySeparatorChar
    return $normalizedCandidate.StartsWith(
        $rootPrefix,
        [System.StringComparison]::OrdinalIgnoreCase)
}

function Get-InventoryRelativePath {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root,

        [Parameter(Mandatory = $true)]
        [string]$FullPath
    )

    $rootUri = [System.Uri]::new(
        $Root.TrimEnd([char[]]@('\', '/')) +
        [System.IO.Path]::DirectorySeparatorChar)
    $pathUri = [System.Uri]::new($FullPath)
    return [System.Uri]::UnescapeDataString(
        $rootUri.MakeRelativeUri($pathUri).ToString()).Replace(
            '/',
            [System.IO.Path]::DirectorySeparatorChar)
}

function Add-InventoryError {
    param(
        [Parameter(Mandatory = $true)]
        [AllowEmptyCollection()]
        [System.Collections.Generic.List[object]]$Errors,

        [Parameter(Mandatory = $true)]
        [string]$RootLabel,

        [Parameter(Mandatory = $true)]
        [string]$Path,

        [Parameter(Mandatory = $true)]
        [string]$Message
    )

    $Errors.Add([pscustomobject]@{
        Root = $RootLabel
        Path = $Path
        Error = $Message
    })
}

function Get-TreeInventory {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Root,

        [Parameter(Mandatory = $true)]
        [string]$RootLabel,

        [Parameter(Mandatory = $true)]
        [AllowEmptyCollection()]
        [System.Collections.Generic.List[object]]$Errors
    )

    $inventory = [System.Collections.Generic.Dictionary[string, object]]::new(
        [System.StringComparer]::OrdinalIgnoreCase)
    $walkErrors = @()
    $items = Get-ChildItem -LiteralPath $Root -Force -Recurse `
        -ErrorAction SilentlyContinue -ErrorVariable +walkErrors
    foreach ($walkError in $walkErrors) {
        Add-InventoryError `
            -Errors $Errors `
            -RootLabel $RootLabel `
            -Path $Root `
            -Message $walkError.Exception.Message
    }

    foreach ($item in $items) {
        try {
            $fullPath = [System.IO.Path]::GetFullPath($item.FullName)
            $relativePath = Get-InventoryRelativePath `
                -Root $Root `
                -FullPath $fullPath
            $isDirectory = [bool]$item.PSIsContainer
            $inventory[$relativePath] = [pscustomobject]@{
                RelativePath = $relativePath
                FullPath = $fullPath
                EntryType = if ($isDirectory) { "Directory" } else { "File" }
                Length = if ($isDirectory) { $null } else { [long]$item.Length }
                LastWriteTimeUtc = $item.LastWriteTimeUtc
            }
        }
        catch {
            Add-InventoryError `
                -Errors $Errors `
                -RootLabel $RootLabel `
                -Path $item.FullName `
                -Message $_.Exception.Message
        }
    }

    return ,$inventory
}

$resolvedSourceRoot = Resolve-InventoryRoot `
    -Path $SourceRoot `
    -Label "SourceRoot"
$resolvedDestinationRoot = Resolve-InventoryRoot `
    -Path $DestinationRoot `
    -Label "DestinationRoot"
if ($resolvedSourceRoot.Equals(
        $resolvedDestinationRoot,
        [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "SourceRoot and DestinationRoot must be different directories."
}

$resolvedOutputDirectory = [System.IO.Path]::GetFullPath($OutputDirectory)
if ((Test-PathInsideRoot `
        -Candidate $resolvedOutputDirectory `
        -Root $resolvedSourceRoot) -or
    (Test-PathInsideRoot `
        -Candidate $resolvedOutputDirectory `
        -Root $resolvedDestinationRoot)) {
    throw "OutputDirectory must be outside both trees so the report cannot change the inventory."
}

$null = New-Item -ItemType Directory -Path $resolvedOutputDirectory -Force
$inventoryErrors = [System.Collections.Generic.List[object]]::new()
$sourceInventory = Get-TreeInventory `
    -Root $resolvedSourceRoot `
    -RootLabel "Source" `
    -Errors $inventoryErrors
$destinationInventory = Get-TreeInventory `
    -Root $resolvedDestinationRoot `
    -RootLabel "Destination" `
    -Errors $inventoryErrors

$relativePaths = @(
    $sourceInventory.Keys
    $destinationInventory.Keys
) | Sort-Object -Unique
$rows = [System.Collections.Generic.List[object]]::new()
$timestampTolerance = [TimeSpan]::FromSeconds(
    [Math]::Max(0, $TimestampToleranceSeconds))

foreach ($relativePath in $relativePaths) {
    $sourceEntry = if ($sourceInventory.ContainsKey($relativePath)) {
        $sourceInventory[$relativePath]
    }
    else {
        $null
    }
    $destinationEntry = if ($destinationInventory.ContainsKey($relativePath)) {
        $destinationInventory[$relativePath]
    }
    else {
        $null
    }

    $status = "Match"
    if ($null -eq $sourceEntry) {
        $status = "DestinationOnly"
    }
    elseif ($null -eq $destinationEntry) {
        $status = "SourceOnly"
    }
    elseif ($sourceEntry.EntryType -ne $destinationEntry.EntryType) {
        $status = "TypeMismatch"
    }
    elseif ($sourceEntry.EntryType -eq "File") {
        if ($sourceEntry.Length -ne $destinationEntry.Length) {
            $status = "SizeMismatch"
        }
        elseif ([Math]::Abs(
                ($sourceEntry.LastWriteTimeUtc -
                    $destinationEntry.LastWriteTimeUtc).TotalSeconds) -gt
            $timestampTolerance.TotalSeconds) {
            $status = "TimestampMismatch"
        }
    }

    $sourceHash = $null
    $destinationHash = $null
    if ($HashMismatches -and
        $status -eq "TimestampMismatch" -and
        $sourceEntry.Length -le $MaxHashBytes) {
        try {
            $sourceHash = (Get-FileHash `
                -LiteralPath $sourceEntry.FullPath `
                -Algorithm SHA256).Hash
            $destinationHash = (Get-FileHash `
                -LiteralPath $destinationEntry.FullPath `
                -Algorithm SHA256).Hash
            $status = if ($sourceHash -eq $destinationHash) {
                "MetadataOnlyMismatch"
            }
            else {
                "ContentMismatch"
            }
        }
        catch {
            Add-InventoryError `
                -Errors $inventoryErrors `
                -RootLabel "Hash" `
                -Path $relativePath `
                -Message $_.Exception.Message
            $status = "HashFailed"
        }
    }

    $rows.Add([pscustomobject]@{
        RelativePath = $relativePath
        Status = $status
        SourceType = if ($null -eq $sourceEntry) { $null } else { $sourceEntry.EntryType }
        DestinationType = if ($null -eq $destinationEntry) { $null } else { $destinationEntry.EntryType }
        SourceLength = if ($null -eq $sourceEntry) { $null } else { $sourceEntry.Length }
        DestinationLength = if ($null -eq $destinationEntry) { $null } else { $destinationEntry.Length }
        SourceLastWriteTimeUtc = if ($null -eq $sourceEntry) { $null } else { $sourceEntry.LastWriteTimeUtc.ToString("O") }
        DestinationLastWriteTimeUtc = if ($null -eq $destinationEntry) { $null } else { $destinationEntry.LastWriteTimeUtc.ToString("O") }
        SourceSha256 = $sourceHash
        DestinationSha256 = $destinationHash
    })
}

$runStamp = Get-Date -Format "yyyyMMdd-HHmmss"
$reportPath = Join-Path `
    $resolvedOutputDirectory `
    "DeskBox-Transfer-Integrity-$runStamp.csv"
$summaryPath = Join-Path `
    $resolvedOutputDirectory `
    "DeskBox-Transfer-Integrity-$runStamp.json"
$errorsPath = Join-Path `
    $resolvedOutputDirectory `
    "DeskBox-Transfer-Integrity-Errors-$runStamp.csv"
$rows | Export-Csv -LiteralPath $reportPath -NoTypeInformation -Encoding utf8
if ($inventoryErrors.Count -gt 0) {
    $inventoryErrors | Export-Csv `
        -LiteralPath $errorsPath `
        -NoTypeInformation `
        -Encoding utf8
}

$statusCounts = @{}
foreach ($group in ($rows | Group-Object -Property Status)) {
    $statusCounts[$group.Name] = $group.Count
}
$summary = [ordered]@{
    SchemaVersion = 1
    GeneratedAtUtc = [DateTime]::UtcNow.ToString("O")
    SourceRoot = $resolvedSourceRoot
    DestinationRoot = $resolvedDestinationRoot
    EntryCount = $rows.Count
    StatusCounts = $statusCounts
    EnumerationErrorCount = $inventoryErrors.Count
    HashMismatches = [bool]$HashMismatches
    MaxHashBytes = $MaxHashBytes
    TimestampToleranceSeconds = $TimestampToleranceSeconds
    ReportPath = $reportPath
    ErrorsPath = if ($inventoryErrors.Count -gt 0) { $errorsPath } else { $null }
}
$summary | ConvertTo-Json -Depth 5 | Set-Content `
    -LiteralPath $summaryPath `
    -Encoding utf8

Write-Output ([pscustomobject]@{
    ReportPath = $reportPath
    SummaryPath = $summaryPath
    ErrorsPath = if ($inventoryErrors.Count -gt 0) { $errorsPath } else { $null }
    EntryCount = $rows.Count
    ErrorCount = $inventoryErrors.Count
})
