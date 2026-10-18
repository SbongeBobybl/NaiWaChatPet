[CmdletBinding()]
param(
    [string]$SummaryPath,

    [string]$DataRoot,

    [ValidateRange(10, 600)]
    [int]$TimeoutSeconds = 120,

    [switch]$KeepRunning
)

$ErrorActionPreference = "Stop"
$scenario = "ExplorerQuickAccessReadOnly"
$scenarioDirectoryName = "explorer-quick-access-read-only"
$shellSmokeEnvironmentVariable = "DESKBOX_AOT_SHELL_SMOKE"
$shortcutSmokeEnvironmentVariable = "DESKBOX_AOT_SHORTCUT_SMOKE"
$mutationSmokeEnvironmentVariable = "DESKBOX_AOT_QUICK_ACCESS_MUTATION_SMOKE"
$musicReadSmokeEnvironmentVariable = "DESKBOX_AOT_MUSIC_VOLUME_READ_SMOKE"
$musicMutationSmokeEnvironmentVariable = "DESKBOX_AOT_MUSIC_VOLUME_MUTATION_SMOKE"
$musicSessionMutationSmokeEnvironmentVariable =
    "DESKBOX_AOT_MUSIC_VOLUME_SESSION_MUTATION_SMOKE"
$managedUiSmokeEnvironmentVariable = "DESKBOX_AOT_MANAGED_UI_SMOKE"
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$launcher = Join-Path $PSScriptRoot "start-aot-preview.ps1"
$previewSessionPath = Join-Path $repoRoot ".artifacts\aot-preview\win-x64\session.json"
$evidenceRoot = Join-Path $repoRoot ".artifacts\aot-shell-smoke\win-x64"

function Get-TextSha256 {
    param(
        [AllowEmptyString()]
        [string]$Value
    )

    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        return [System.BitConverter]::ToString(
            $sha.ComputeHash([System.Text.Encoding]::UTF8.GetBytes($Value))).Replace("-", "")
    }
    finally {
        $sha.Dispose()
    }
}

function Get-DirectoryStateFingerprint {
    param(
        [Parameter(Mandatory)]
        [string]$Path
    )

    $normalizedPath = [System.IO.Path]::GetFullPath($Path).TrimEnd(
        [System.IO.Path]::DirectorySeparatorChar,
        [System.IO.Path]::AltDirectorySeparatorChar)
    if (-not (Test-Path -LiteralPath $normalizedPath -PathType Container)) {
        return [PSCustomObject]@{
            exists = $false
            fileCount = 0
            bytes = 0L
            fingerprint = Get-TextSha256 -Value "<missing>"
        }
    }

    $files = @(Get-ChildItem -LiteralPath $normalizedPath -File -Recurse -Force)
    $records = [System.Collections.Generic.List[string]]::new()
    foreach ($file in $files) {
        $relativePath = $file.FullName.Substring($normalizedPath.Length).TrimStart('\', '/')
        $records.Add(
            ("{0}|{1}|{2}" -f @(
                $relativePath.Replace('\', '/').ToUpperInvariant(),
                $file.Length,
                $file.LastWriteTimeUtc.Ticks)))
    }
    $records.Sort([System.StringComparer]::Ordinal)

    return [PSCustomObject]@{
        exists = $true
        fileCount = $files.Count
        bytes = [long](($files | Measure-Object -Property Length -Sum).Sum)
        fingerprint = Get-TextSha256 -Value ([string]::Join("`n", $records))
    }
}

function Test-PathEqual {
    param(
        [Parameter(Mandatory)]
        [string]$Left,

        [Parameter(Mandatory)]
        [string]$Right
    )

    return [System.IO.Path]::GetFullPath($Left).TrimEnd('\', '/').Equals(
        [System.IO.Path]::GetFullPath($Right).TrimEnd('\', '/'),
        [System.StringComparison]::OrdinalIgnoreCase)
}

function Stop-ExactPreviewProcess {
    param(
        [Parameter(Mandatory)]
        [string]$ExecutablePath
    )

    @(Get-CimInstance Win32_Process -Filter "Name='DeskBox.exe'") |
        Where-Object {
            -not [string]::IsNullOrWhiteSpace($_.ExecutablePath) -and
            (Test-PathEqual -Left $_.ExecutablePath -Right $ExecutablePath)
        } |
        ForEach-Object { Stop-Process -Id $_.ProcessId -Force }
}

if (-not (Test-Path -LiteralPath $launcher -PathType Leaf)) {
    throw "Native AOT preview launcher was not found at '$launcher'."
}

if ([string]::IsNullOrWhiteSpace($SummaryPath)) {
    $SummaryPath = Join-Path $repoRoot ".artifacts\aot-audit\win-x64\summary.json"
}
$SummaryPath = [System.IO.Path]::GetFullPath($SummaryPath)

if ([string]::IsNullOrWhiteSpace($DataRoot)) {
    $worktreeName = (Split-Path $repoRoot -Leaf) -replace '[^A-Za-z0-9._-]', '-'
    $pathHash = (Get-TextSha256 -Value $repoRoot.ToUpperInvariant()).Substring(0, 8)
    $DataRoot = Join-Path $env:LOCALAPPDATA (
        "DeskBox-AotPreview\{0}-{1}-stage5b2a-{2}" -f @(
            $worktreeName,
            $pathHash,
            $scenarioDirectoryName))
}
$DataRoot = [System.IO.Path]::GetFullPath($DataRoot)
$resultPath = Join-Path $DataRoot (
    "aot-shell-smoke\$scenarioDirectoryName\result.json")

$previewSession = $null
try {
    $previousShellSmoke = [Environment]::GetEnvironmentVariable(
        $shellSmokeEnvironmentVariable,
        "Process")
    $previousShortcutSmoke = [Environment]::GetEnvironmentVariable(
        $shortcutSmokeEnvironmentVariable,
        "Process")
    $previousMutationSmoke = [Environment]::GetEnvironmentVariable(
        $mutationSmokeEnvironmentVariable,
        "Process")
    $previousMusicReadSmoke = [Environment]::GetEnvironmentVariable(
        $musicReadSmokeEnvironmentVariable,
        "Process")
    $previousMusicMutationSmoke = [Environment]::GetEnvironmentVariable(
        $musicMutationSmokeEnvironmentVariable,
        "Process")
    $previousMusicSessionMutationSmoke = [Environment]::GetEnvironmentVariable(
        $musicSessionMutationSmokeEnvironmentVariable,
        "Process")
    $previousManagedUiSmoke = [Environment]::GetEnvironmentVariable(
        $managedUiSmokeEnvironmentVariable,
        "Process")
    try {
        [Environment]::SetEnvironmentVariable(
            $shellSmokeEnvironmentVariable,
            $scenario,
            "Process")
        [Environment]::SetEnvironmentVariable(
            $shortcutSmokeEnvironmentVariable,
            $null,
            "Process")
        [Environment]::SetEnvironmentVariable(
            $mutationSmokeEnvironmentVariable,
            $null,
            "Process")
        [Environment]::SetEnvironmentVariable(
            $musicReadSmokeEnvironmentVariable,
            $null,
            "Process")
        [Environment]::SetEnvironmentVariable(
            $musicMutationSmokeEnvironmentVariable,
            $null,
            "Process")
        [Environment]::SetEnvironmentVariable(
            $musicSessionMutationSmokeEnvironmentVariable,
            $null,
            "Process")
        [Environment]::SetEnvironmentVariable(
            $managedUiSmokeEnvironmentVariable,
            $null,
            "Process")
        $previewOutput = @(
            & $launcher `
                -SummaryPath $SummaryPath `
                -DataRoot $DataRoot `
                -StartupWaitSeconds 5)
    }
    finally {
        [Environment]::SetEnvironmentVariable(
            $shellSmokeEnvironmentVariable,
            $previousShellSmoke,
            "Process")
        [Environment]::SetEnvironmentVariable(
            $shortcutSmokeEnvironmentVariable,
            $previousShortcutSmoke,
            "Process")
        [Environment]::SetEnvironmentVariable(
            $mutationSmokeEnvironmentVariable,
            $previousMutationSmoke,
            "Process")
        [Environment]::SetEnvironmentVariable(
            $musicReadSmokeEnvironmentVariable,
            $previousMusicReadSmoke,
            "Process")
        [Environment]::SetEnvironmentVariable(
            $musicMutationSmokeEnvironmentVariable,
            $previousMusicMutationSmoke,
            "Process")
        [Environment]::SetEnvironmentVariable(
            $musicSessionMutationSmokeEnvironmentVariable,
            $previousMusicSessionMutationSmoke,
            "Process")
        [Environment]::SetEnvironmentVariable(
            $managedUiSmokeEnvironmentVariable,
            $previousManagedUiSmoke,
            "Process")
    }

    if (-not (Test-Path -LiteralPath $previewSessionPath -PathType Leaf)) {
        throw "Native AOT preview session.json was not created at '$previewSessionPath'."
    }
    $previewSession = Get-Content -LiteralPath $previewSessionPath -Raw | ConvertFrom-Json
    if (-not (Test-PathEqual -Left $previewSession.previewDataRoot -Right $DataRoot)) {
        throw "Preview session data root does not match the AOT shell smoke root."
    }

    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    $smokeResult = $null
    while ([DateTime]::UtcNow -lt $deadline) {
        if (Test-Path -LiteralPath $resultPath -PathType Leaf) {
            try {
                $candidate = Get-Content -LiteralPath $resultPath -Raw | ConvertFrom-Json
                $smokeResult = $candidate
                if ($candidate.State -eq "Completed" -or $candidate.State -eq "Failed") {
                    break
                }
            }
            catch {
                # Retry a bounded transient sharing race while the app atomically replaces JSON.
            }
        }

        Start-Sleep -Milliseconds 250
    }

    if ($null -eq $smokeResult -or
        ($smokeResult.State -ne "Completed" -and $smokeResult.State -ne "Failed")) {
        throw "AOT shell smoke timed out after $TimeoutSeconds seconds. Last state='$($smokeResult.State)'; result='$resultPath'."
    }
    if ($smokeResult.State -ne "Completed" -or -not [bool]$smokeResult.Success) {
        throw "AOT shell smoke failed: $($smokeResult.Error)"
    }

    if (-not (Test-PathEqual -Left $smokeResult.ExecutablePath -Right $previewSession.executablePath)) {
        throw "AOT shell smoke executable path does not match the audited preview session."
    }
    if (-not [string]::Equals(
            [string]$smokeResult.ExecutableSha256,
            [string]$previewSession.executableSha256,
            [StringComparison]::OrdinalIgnoreCase)) {
        throw "AOT shell smoke executableSha256 does not match the audited preview session."
    }
    if (-not (Test-PathEqual -Left $smokeResult.ModulePath -Right $previewSession.rustNativePath)) {
        throw "AOT shell smoke ModulePath does not match the audited Rust module path."
    }
    if (-not [string]::Equals(
            [string]$smokeResult.ModuleSha256,
            [string]$previewSession.rustNativeSha256,
            [StringComparison]::OrdinalIgnoreCase)) {
        throw "AOT shell smoke ModuleSha256 does not match rustNativeSha256 from the audited preview session."
    }
    if ([bool]$smokeResult.IsDynamicCodeSupported -or
        $smokeResult.ExplorerBackend -ne "Rust" -or
        $smokeResult.QuickAccessBackend -ne "Rust" -or
        $smokeResult.LoadState -ne "Loaded" -or
        [uint32]$smokeResult.AbiVersion -ne 2 -or
        (([uint64]$smokeResult.Capabilities -band 0xC0) -ne 0xC0) -or
        [string]::IsNullOrWhiteSpace([string]$smokeResult.ModuleHandle) -or
        $smokeResult.ModuleHandle -eq "0x0") {
        throw "AOT shell smoke did not prove loaded Explorer and Quick Access Rust boundaries."
    }
    if ($smokeResult.Scenario -ne $scenario -or
        -not (Test-PathEqual -Left $smokeResult.PreviewDataRoot -Right $DataRoot)) {
        throw "AOT shell smoke structured evidence does not match the requested scenario/root."
    }

    if (-not [bool]$smokeResult.ExplorerServiceSucceeded -or
        -not [bool]$smokeResult.ExplorerNativeSuccess -or
        -not [bool]$smokeResult.ExplorerMarkerExists -or
        $smokeResult.ExplorerMarkerText -ne "explorer-shell-launch" -or
        [uint32]$smokeResult.ExplorerNativeAttemptedPhases -eq 0) {
        throw "ExplorerMarkerExists and the detailed Rust launch evidence were not proven."
    }
    if ($smokeResult.QuickAccessStateBefore -ne "NotPinned" -or
        $smokeResult.QuickAccessStateAfter -ne "NotPinned" -or
        $smokeResult.QuickAccessNativeState -ne "NotPinned" -or
        -not [bool]$smokeResult.QuickAccessNativeSuccess -or
        -not [string]::IsNullOrWhiteSpace([string]$smokeResult.QuickAccessErrorBefore) -or
        -not [string]::IsNullOrWhiteSpace([string]$smokeResult.QuickAccessErrorAfter)) {
        throw "Quick Access read-only states were not consistently NotPinned."
    }

    $requiredSteps = @(
        "quick-access-query-before",
        "quick-access-native-query",
        "explorer-product-service",
        "explorer-native-phases",
        "explorer-launch-marker",
        "quick-access-query-after",
        "runtime-native-aot",
        "explorer-backend-rust",
        "quick-access-backend-rust",
        "module-loaded",
        "module-handle",
        "module-abi",
        "module-capabilities")
    $missingSteps = @($requiredSteps | Where-Object { $_ -notin @($smokeResult.Steps) })
    if ($missingSteps.Count -gt 0) {
        throw "AOT shell smoke result is missing required steps: $($missingSteps -join ', ')."
    }

    $productionStateAfter = Get-DirectoryStateFingerprint -Path $previewSession.productionDataRoot
    if (-not [string]::Equals(
            [string]$productionStateAfter.fingerprint,
            [string]$previewSession.productionDataFingerprintBefore,
            [StringComparison]::OrdinalIgnoreCase) -or
        [int]$productionStateAfter.fileCount -ne [int]$previewSession.productionDataFileCountBefore -or
        [long]$productionStateAfter.bytes -ne [long]$previewSession.productionDataBytesBefore) {
        throw "Production data changed during the AOT shell smoke."
    }

    $scenarioEvidenceDirectory = Join-Path $evidenceRoot $scenarioDirectoryName
    New-Item -ItemType Directory -Path $scenarioEvidenceDirectory -Force | Out-Null
    $sessionPath = Join-Path $scenarioEvidenceDirectory "session.json"
    $session = [ordered]@{
        schemaVersion = 1
        generatedAtUtc = [DateTime]::UtcNow.ToString("O")
        scenario = $scenario
        resultPath = $resultPath
        previewSessionPath = $previewSessionPath
        executablePath = $smokeResult.ExecutablePath
        executableSha256 = $smokeResult.ExecutableSha256
        rustNativePath = $smokeResult.ModulePath
        rustNativeSha256 = $smokeResult.ModuleSha256
        nativeModuleHandle = $smokeResult.ModuleHandle
        abiVersion = [uint32]$smokeResult.AbiVersion
        capabilities = [uint64]$smokeResult.Capabilities
        explorerBackend = $smokeResult.ExplorerBackend
        explorerMarkerExists = [bool]$smokeResult.ExplorerMarkerExists
        explorerAttemptedPhases = [uint32]$smokeResult.ExplorerNativeAttemptedPhases
        quickAccessBackend = $smokeResult.QuickAccessBackend
        quickAccessStateBefore = $smokeResult.QuickAccessStateBefore
        quickAccessStateAfter = $smokeResult.QuickAccessStateAfter
        quickAccessNativeState = $smokeResult.QuickAccessNativeState
        quickAccessNativeAttemptedPhases = [uint32]$smokeResult.QuickAccessNativeAttemptedPhases
        steps = @($smokeResult.Steps)
        previewDataRoot = $DataRoot
        productionDataRoot = $previewSession.productionDataRoot
        productionDataFingerprintBefore = $previewSession.productionDataFingerprintBefore
        productionDataFingerprintAfter = $productionStateAfter.fingerprint
        productionDataFileCountAfter = $productionStateAfter.fileCount
        productionDataBytesAfter = $productionStateAfter.bytes
        processId = [int]$smokeResult.ProcessId
    }
    $session | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $sessionPath -Encoding UTF8

    [PSCustomObject]@{
        Scenario = $scenario
        Success = $true
        ProcessId = [int]$smokeResult.ProcessId
        Exe = $smokeResult.ExecutablePath
        ExeSha256 = $smokeResult.ExecutableSha256
        RustNativeDll = $smokeResult.ModulePath
        RustNativeSha256 = $smokeResult.ModuleSha256
        AbiVersion = [uint32]$smokeResult.AbiVersion
        Capabilities = [uint64]$smokeResult.Capabilities
        ExplorerMarkerExists = [bool]$smokeResult.ExplorerMarkerExists
        QuickAccessStateBefore = $smokeResult.QuickAccessStateBefore
        QuickAccessStateAfter = $smokeResult.QuickAccessStateAfter
        QuickAccessNativeState = $smokeResult.QuickAccessNativeState
        DataRoot = $DataRoot
        ResultPath = $resultPath
        SessionPath = $sessionPath
        ProductionDataFingerprint = $productionStateAfter.fingerprint
        Running = $KeepRunning.IsPresent
    }
}
finally {
    if (-not $KeepRunning.IsPresent -and
        $null -ne $previewSession -and
        -not [string]::IsNullOrWhiteSpace([string]$previewSession.executablePath)) {
        Stop-ExactPreviewProcess -ExecutablePath $previewSession.executablePath
    }
}
