[CmdletBinding(SupportsShouldProcess = $true)]
param(
    [Parameter()]
    [ValidateSet('DeskBox.exe', 'DeskBox.Updater.exe')]
    [string[]]$ExecutableName = @('DeskBox.exe')
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$localDumpsRoot = 'HKCU:\Software\Microsoft\Windows\Windows Error Reporting\LocalDumps'
$managementRoot = 'HKCU:\Software\DeskBox\Support\LocalDumps'
$managedMarkerName = 'DeskBoxManagedBy'
$managedMarkerValue = 'DeskBox.LocalDumps.v1'

function Get-RegistryValueOrNull {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,

        [Parameter(Mandatory = $true)]
        [string]$Name
    )

    if (-not (Test-Path -LiteralPath $Path)) {
        return $null
    }

    try {
        return Get-ItemPropertyValue -LiteralPath $Path -Name $Name -ErrorAction Stop
    }
    catch [System.Management.Automation.PSArgumentException] {
        return $null
    }
}

function Test-ManagedValueMatch {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Name,

        [AllowNull()]
        [object]$CurrentValue,

        [AllowNull()]
        [object]$ManagedValue
    )

    if ($Name -eq 'DumpFolder') {
        return [string]::Equals(
            [string]$CurrentValue,
            [string]$ManagedValue,
            [StringComparison]::OrdinalIgnoreCase)
    }

    return $CurrentValue -eq $ManagedValue
}

$requestedExecutables = @($ExecutableName | Sort-Object -Unique)

foreach ($exeName in $requestedExecutables) {
    $targetKey = Join-Path $localDumpsRoot $exeName
    $managementKey = Join-Path $managementRoot $exeName
    $targetExists = Test-Path -LiteralPath $targetKey
    $managementExists = Test-Path -LiteralPath $managementKey

    if (-not $targetExists -and -not $managementExists) {
        Write-Host "$exeName has no LocalDumps settings managed by DeskBox."
        continue
    }

    $targetMarker = Get-RegistryValueOrNull -Path $targetKey -Name $managedMarkerName
    $managementMarker = Get-RegistryValueOrNull -Path $managementKey -Name 'ManagedMarker'

    if ($managementMarker -ne $managedMarkerValue) {
        Write-Warning "$exeName has no valid DeskBox management record. Existing settings were left unchanged."
        continue
    }

    if (-not $targetExists) {
        if ($PSCmdlet.ShouldProcess($managementKey, "Remove stale DeskBox management record for $exeName")) {
            Remove-Item -LiteralPath $managementKey
        }

        continue
    }

    if ($targetMarker -ne $managedMarkerValue) {
        Write-Warning "$exeName is no longer marked as DeskBox-managed. Existing settings were left unchanged."
        continue
    }

    if (-not $PSCmdlet.ShouldProcess($targetKey, "Disable DeskBox-managed crash dumps for $exeName")) {
        continue
    }

    $managedValues = @(
        @{ Name = 'DumpFolder'; StateName = 'ManagedDumpFolder' },
        @{ Name = 'DumpCount'; StateName = 'ManagedDumpCount' },
        @{ Name = 'DumpType'; StateName = 'ManagedDumpType' }
    )

    foreach ($entry in $managedValues) {
        $currentValue = Get-RegistryValueOrNull -Path $targetKey -Name $entry.Name
        $managedValue = Get-RegistryValueOrNull -Path $managementKey -Name $entry.StateName

        if ($null -eq $currentValue) {
            continue
        }

        if (Test-ManagedValueMatch -Name $entry.Name -CurrentValue $currentValue -ManagedValue $managedValue) {
            Remove-ItemProperty -LiteralPath $targetKey -Name $entry.Name
        }
        else {
            Write-Warning "${exeName}: $($entry.Name) was changed after DeskBox configured it and was left unchanged."
        }
    }

    Remove-ItemProperty -LiteralPath $targetKey -Name $managedMarkerName

    $targetItem = Get-Item -LiteralPath $targetKey
    $remainingValueNames = @($targetItem.GetValueNames())
    $remainingSubkeys = @(Get-ChildItem -LiteralPath $targetKey)
    if ($remainingValueNames.Count -eq 0 -and $remainingSubkeys.Count -eq 0) {
        Remove-Item -LiteralPath $targetKey
    }

    Remove-Item -LiteralPath $managementKey
    Write-Host "$exeName crash dumps disabled. Existing dump files were not deleted."
}
