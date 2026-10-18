[CmdletBinding(SupportsShouldProcess = $true)]
param(
    [Parameter()]
    [ValidateSet('DeskBox.exe', 'DeskBox.Updater.exe')]
    [string[]]$ExecutableName = @('DeskBox.exe'),

    [Parameter()]
    [ValidateNotNullOrEmpty()]
    [string]$DumpFolder = (Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'DeskBox\CrashDumps'),

    [Parameter()]
    [ValidateRange(1, 50)]
    [int]$DumpCount = 5,

    [Parameter()]
    [ValidateSet('Mini', 'Full')]
    [string]$DumpType = 'Mini'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$localDumpsRoot = 'HKCU:\Software\Microsoft\Windows\Windows Error Reporting\LocalDumps'
$managementRoot = 'HKCU:\Software\DeskBox\Support\LocalDumps'
$managedMarkerName = 'DeskBoxManagedBy'
$managedMarkerValue = 'DeskBox.LocalDumps.v1'
$dumpTypeValue = if ($DumpType -eq 'Full') { 2 } else { 1 }

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

function Test-RegistryValueEqual {
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

$expandedDumpFolder = [Environment]::ExpandEnvironmentVariables($DumpFolder)
if (-not [IO.Path]::IsPathRooted($expandedDumpFolder)) {
    throw 'DumpFolder must be an absolute path.'
}

$resolvedDumpFolder = [IO.Path]::GetFullPath($expandedDumpFolder)
$requestedExecutables = @($ExecutableName | Sort-Object -Unique)

# Preflight every requested executable before making any changes. Existing per-exe
# WER settings are never taken over unless this script already owns them.
foreach ($exeName in $requestedExecutables) {
    $targetKey = Join-Path $localDumpsRoot $exeName
    $managementKey = Join-Path $managementRoot $exeName
    $targetExists = Test-Path -LiteralPath $targetKey
    $managementExists = Test-Path -LiteralPath $managementKey

    if ($targetExists) {
        $targetMarker = Get-RegistryValueOrNull -Path $targetKey -Name $managedMarkerName
        $managementMarker = Get-RegistryValueOrNull -Path $managementKey -Name 'ManagedMarker'
        if ($targetMarker -ne $managedMarkerValue -or $managementMarker -ne $managedMarkerValue) {
            throw "LocalDumps settings already exist for $exeName and are not managed by this script. No settings were changed."
        }

        $managedValues = @(
            @{ Name = 'DumpFolder'; StateName = 'ManagedDumpFolder' },
            @{ Name = 'DumpCount'; StateName = 'ManagedDumpCount' },
            @{ Name = 'DumpType'; StateName = 'ManagedDumpType' }
        )

        foreach ($entry in $managedValues) {
            $currentValue = Get-RegistryValueOrNull -Path $targetKey -Name $entry.Name
            $managedValue = Get-RegistryValueOrNull -Path $managementKey -Name $entry.StateName
            if (-not (Test-RegistryValueEqual -Name $entry.Name -CurrentValue $currentValue -ManagedValue $managedValue)) {
                throw "$exeName $($entry.Name) changed since DeskBox configured it. No settings were changed. Disable first or preserve the external configuration manually."
            }
        }
    }
    elseif ($managementExists) {
        throw "Managed state exists for $exeName but its LocalDumps key is missing. Run the disable script before enabling it again."
    }
}

if ($PSCmdlet.ShouldProcess($resolvedDumpFolder, 'Create crash dump directory')) {
    $null = New-Item -ItemType Directory -Path $resolvedDumpFolder -Force
}

foreach ($exeName in $requestedExecutables) {
    $targetKey = Join-Path $localDumpsRoot $exeName
    $managementKey = Join-Path $managementRoot $exeName
    $targetExistedBefore = Test-Path -LiteralPath $targetKey
    $managementExistedBefore = Test-Path -LiteralPath $managementKey

    if (-not $PSCmdlet.ShouldProcess($targetKey, "Enable $DumpType crash dumps for $exeName")) {
        continue
    }

    try {
        $null = New-Item -Path $managementKey -Force
        $null = New-ItemProperty -LiteralPath $managementKey -Name 'ManagedMarker' -Value $managedMarkerValue -PropertyType String -Force
        $null = New-ItemProperty -LiteralPath $managementKey -Name 'ManagedDumpFolder' -Value $resolvedDumpFolder -PropertyType String -Force
        $null = New-ItemProperty -LiteralPath $managementKey -Name 'ManagedDumpCount' -Value $DumpCount -PropertyType DWord -Force
        $null = New-ItemProperty -LiteralPath $managementKey -Name 'ManagedDumpType' -Value $dumpTypeValue -PropertyType DWord -Force

        $null = New-Item -Path $targetKey -Force
        $null = New-ItemProperty -LiteralPath $targetKey -Name $managedMarkerName -Value $managedMarkerValue -PropertyType String -Force
        $null = New-ItemProperty -LiteralPath $targetKey -Name 'DumpFolder' -Value $resolvedDumpFolder -PropertyType ExpandString -Force
        $null = New-ItemProperty -LiteralPath $targetKey -Name 'DumpCount' -Value $DumpCount -PropertyType DWord -Force
        $null = New-ItemProperty -LiteralPath $targetKey -Name 'DumpType' -Value $dumpTypeValue -PropertyType DWord -Force
    }
    catch {
        # Only roll back the exact per-exe keys involved in this failed first-time
        # setup. Never remove the LocalDumps or DeskBox support parent trees.
        if (-not $targetExistedBefore -and
            (Test-Path -LiteralPath $targetKey) -and
            (Get-RegistryValueOrNull -Path $targetKey -Name $managedMarkerName) -eq $managedMarkerValue) {
            Remove-Item -LiteralPath $targetKey -ErrorAction SilentlyContinue
        }

        if (-not $managementExistedBefore -and
            (Test-Path -LiteralPath $managementKey) -and
            (Get-RegistryValueOrNull -Path $managementKey -Name 'ManagedMarker') -eq $managedMarkerValue) {
            Remove-Item -LiteralPath $managementKey -ErrorAction SilentlyContinue
        }

        throw
    }

    Write-Host "$exeName crash dumps enabled: $resolvedDumpFolder ($DumpType, keep $DumpCount)."
}
