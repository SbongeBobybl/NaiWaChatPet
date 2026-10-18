[CmdletBinding()]
param(
    [string]$SummaryPath,

    [string]$DataRoot,

    [ValidateRange(10, 600)]
    [int]$TimeoutSeconds = 120
)

$ErrorActionPreference = "Stop"
$mutationSmokeEnvironmentVariable = "DESKBOX_AOT_QUICK_ACCESS_MUTATION_SMOKE"
$shortcutSmokeEnvironmentVariable = "DESKBOX_AOT_SHORTCUT_SMOKE"
$shellSmokeEnvironmentVariable = "DESKBOX_AOT_SHELL_SMOKE"
$musicReadSmokeEnvironmentVariable = "DESKBOX_AOT_MUSIC_VOLUME_READ_SMOKE"
$musicMutationSmokeEnvironmentVariable = "DESKBOX_AOT_MUSIC_VOLUME_MUTATION_SMOKE"
$musicSessionMutationSmokeEnvironmentVariable =
    "DESKBOX_AOT_MUSIC_VOLUME_SESSION_MUTATION_SMOKE"
$managedUiSmokeEnvironmentVariable = "DESKBOX_AOT_MANAGED_UI_SMOKE"
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$launcher = Join-Path $PSScriptRoot "start-aot-preview.ps1"
$previewSessionPath = Join-Path $repoRoot ".artifacts\aot-preview\win-x64\session.json"
$evidenceRoot = Join-Path $repoRoot ".artifacts\aot-quick-access-mutation-smoke\win-x64"

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

    $processIds = [System.Collections.Generic.HashSet[int]]::new()
    foreach ($process in @(Get-Process DeskBox -ErrorAction SilentlyContinue)) {
        try {
            if (Test-PathEqual -Left $process.MainModule.FileName -Right $ExecutablePath) {
                $null = $processIds.Add($process.Id)
            }
        }
        catch {
            # Access to a process module can race with process exit.
        }
    }

    foreach ($process in @(Get-CimInstance Win32_Process -Filter "Name='DeskBox.exe'")) {
        if (-not [string]::IsNullOrWhiteSpace($process.ExecutablePath) -and
            (Test-PathEqual -Left $process.ExecutablePath -Right $ExecutablePath)) {
            $null = $processIds.Add([int]$process.ProcessId)
        }
    }

    foreach ($processId in $processIds) {
        Stop-Process -Id $processId -Force -ErrorAction SilentlyContinue
    }
    foreach ($processId in $processIds) {
        Wait-Process -Id $processId -Timeout 5 -ErrorAction SilentlyContinue
    }
}

function Get-ScenarioDirectoryName {
    param(
        [Parameter(Mandatory)]
        [ValidateSet(
            "PinUnpin",
            "PinThenFail",
            "PinThenAwaitExternalCompensation",
            "CompensateUnpin")]
        [string]$Scenario
    )

    if ($Scenario -eq "PinUnpin") {
        return "pin-unpin"
    }
    if ($Scenario -eq "PinThenFail") {
        return "pin-then-fail"
    }
    if ($Scenario -eq "PinThenAwaitExternalCompensation") {
        return "pin-then-await-external-compensation"
    }

    return "compensate-unpin"
}

function Invoke-MutationScenario {
    param(
        [Parameter(Mandatory)]
        [ValidateSet(
            "PinUnpin",
            "PinThenFail",
            "PinThenAwaitExternalCompensation",
            "CompensateUnpin")]
        [string]$Scenario,

        [Parameter(Mandatory)]
        [ValidateSet(
            "preflight",
            "in-process-failure",
            "forced-termination",
            "recovery",
            "main",
            "postflight")]
        [string]$Phase
    )

    $scenarioDirectoryName = Get-ScenarioDirectoryName -Scenario $Scenario
    $resultPath = Join-Path $DataRoot (
        "aot-quick-access-mutation-smoke\$scenarioDirectoryName\result.json")
    $temporaryResultPath = $resultPath + ".tmp"
    if (Test-Path -LiteralPath $resultPath -PathType Leaf) {
        Remove-Item -LiteralPath $resultPath -Force
    }
    if (Test-Path -LiteralPath $temporaryResultPath -PathType Leaf) {
        Remove-Item -LiteralPath $temporaryResultPath -Force
    }

    $previewSession = $null
    try {
        $previousMutationSmoke = [Environment]::GetEnvironmentVariable(
            $mutationSmokeEnvironmentVariable,
            "Process")
        $previousShortcutSmoke = [Environment]::GetEnvironmentVariable(
            $shortcutSmokeEnvironmentVariable,
            "Process")
        $previousShellSmoke = [Environment]::GetEnvironmentVariable(
            $shellSmokeEnvironmentVariable,
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
                $mutationSmokeEnvironmentVariable,
                $Scenario,
                "Process")
            [Environment]::SetEnvironmentVariable(
                $shortcutSmokeEnvironmentVariable,
                $null,
                "Process")
            [Environment]::SetEnvironmentVariable(
                $shellSmokeEnvironmentVariable,
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
                $mutationSmokeEnvironmentVariable,
                $previousMutationSmoke,
                "Process")
            [Environment]::SetEnvironmentVariable(
                $shortcutSmokeEnvironmentVariable,
                $previousShortcutSmoke,
                "Process")
            [Environment]::SetEnvironmentVariable(
                $shellSmokeEnvironmentVariable,
                $previousShellSmoke,
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
            throw "The $Phase preview session data root does not match the mutation root."
        }
        if (-not (Test-PathEqual -Left $previewSession.executablePath -Right $expectedExecutablePath)) {
            throw "The $Phase preview session executable does not match the audited executable."
        }

        if ($null -eq $script:productionBaseline) {
            $script:productionBaseline = [PSCustomObject]@{
                root = [string]$previewSession.productionDataRoot
                fingerprint = [string]$previewSession.productionDataFingerprintBefore
                fileCount = [int]$previewSession.productionDataFileCountBefore
                bytes = [long]$previewSession.productionDataBytesBefore
            }
        }
        elseif (-not (Test-PathEqual `
                -Left $script:productionBaseline.root `
                -Right $previewSession.productionDataRoot)) {
            throw "The $Phase preview session changed the production data root."
        }

        $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
        $smokeResult = $null
        while ([DateTime]::UtcNow -lt $deadline) {
            if (Test-Path -LiteralPath $resultPath -PathType Leaf) {
                try {
                    $candidate = Get-Content -LiteralPath $resultPath -Raw | ConvertFrom-Json
                    if ([int]$candidate.ProcessId -eq [int]$previewSession.primaryProcessId) {
                        $smokeResult = $candidate
                        if ($candidate.State -eq "Completed" -or
                            $candidate.State -eq "Failed" -or
                            ($Scenario -eq "PinThenAwaitExternalCompensation" -and
                                $candidate.State -eq "AwaitingExternalCompensation")) {
                            break
                        }
                    }
                }
                catch {
                    # Retry a bounded transient sharing race during the atomic JSON replacement.
                }
            }

            Start-Sleep -Milliseconds 250
        }

        $expectedExternalWait =
            $Scenario -eq "PinThenAwaitExternalCompensation" -and
            $smokeResult.State -eq "AwaitingExternalCompensation"
        if ($null -eq $smokeResult -or
            ($smokeResult.State -ne "Completed" -and
                $smokeResult.State -ne "Failed" -and
                -not $expectedExternalWait)) {
            throw "AOT Quick Access $Phase/$Scenario timed out after $TimeoutSeconds seconds. Last state='$($smokeResult.State)'; result='$resultPath'."
        }

        $phaseEvidencePath = Join-Path $evidenceRoot "$Phase-result.json"
        Copy-Item -LiteralPath $resultPath -Destination $phaseEvidencePath -Force

        return [PSCustomObject]@{
            Phase = $Phase
            Scenario = $Scenario
            Result = $smokeResult
            ResultPath = $resultPath
            EvidencePath = $phaseEvidencePath
            PreviewSession = $previewSession
            PreviewOutput = $previewOutput
        }
    }
    finally {
        $pathToStop = if ($null -ne $previewSession -and
            -not [string]::IsNullOrWhiteSpace([string]$previewSession.executablePath)) {
            [string]$previewSession.executablePath
        }
        else {
            $expectedExecutablePath
        }
        if (-not [string]::IsNullOrWhiteSpace($pathToStop)) {
            Stop-ExactPreviewProcess -ExecutablePath $pathToStop
        }
    }
}

function Assert-MutationResult {
    param(
        [Parameter(Mandatory)]
        [PSCustomObject]$Run,

        [switch]$AllowExpectedFailure,

        [switch]$AllowAwaitingExternalCompensation,

        [switch]$RequireInitiallyPinned
    )

    $smokeResult = $Run.Result
    $previewSession = $Run.PreviewSession
    if ($AllowExpectedFailure.IsPresent) {
        if ($smokeResult.State -ne "Failed" -or
            [bool]$smokeResult.Success -or
            -not ([string]$smokeResult.Error).Contains(
                "intentional-after-pin",
                [StringComparison]::Ordinal)) {
            throw "AOT Quick Access $($Run.Phase) did not produce the expected in-process failure."
        }
    }
    elseif ($AllowAwaitingExternalCompensation.IsPresent) {
        if ($smokeResult.State -ne "AwaitingExternalCompensation" -or
            [bool]$smokeResult.Success) {
            throw "AOT Quick Access $($Run.Phase) did not reach the forced-termination checkpoint."
        }
    }
    elseif ($smokeResult.State -ne "Completed" -or -not [bool]$smokeResult.Success) {
        throw "AOT Quick Access $($Run.Phase)/$($Run.Scenario) failed: $($smokeResult.Error)"
    }
    if ($smokeResult.Scenario -ne $Run.Scenario -or
        -not (Test-PathEqual -Left $smokeResult.PreviewDataRoot -Right $DataRoot) -or
        -not (Test-PathEqual -Left $smokeResult.TargetFolder -Right $targetFolder)) {
        throw "AOT Quick Access $($Run.Phase) evidence does not match the requested scenario/root/target."
    }
    if (-not (Test-PathEqual `
            -Left $smokeResult.ExecutablePath `
            -Right $previewSession.executablePath) -or
        -not [string]::Equals(
            [string]$smokeResult.ExecutableSha256,
            [string]$previewSession.executableSha256,
            [StringComparison]::OrdinalIgnoreCase)) {
        throw "AOT Quick Access $($Run.Phase) executable identity does not match the audited preview session."
    }
    if (-not (Test-PathEqual -Left $smokeResult.ModulePath -Right $previewSession.rustNativePath) -or
        -not [string]::Equals(
            [string]$smokeResult.ModuleSha256,
            [string]$previewSession.rustNativeSha256,
            [StringComparison]::OrdinalIgnoreCase)) {
        throw "AOT Quick Access $($Run.Phase) Rust module identity does not match the audited preview session."
    }
    if ([bool]$smokeResult.IsDynamicCodeSupported -or
        $smokeResult.QuickAccessBackend -ne "Rust" -or
        $smokeResult.LoadState -ne "Loaded" -or
        [uint32]$smokeResult.AbiVersion -ne 2 -or
        (([uint64]$smokeResult.Capabilities -band 0x80) -ne 0x80) -or
        [string]::IsNullOrWhiteSpace([string]$smokeResult.ModuleHandle) -or
        $smokeResult.ModuleHandle -eq "0x0") {
        throw "AOT Quick Access $($Run.Phase) did not prove the loaded Rust boundary."
    }
    if (-not $AllowAwaitingExternalCompensation.IsPresent) {
        if (-not [bool]$smokeResult.CleanupSucceeded -or
            $smokeResult.FinalPublicState -ne "NotPinned" -or
            $smokeResult.FinalNativeState -ne "NotPinned" -or
            -not [bool]$smokeResult.FinalNativeSuccess -or
            -not [string]::IsNullOrWhiteSpace([string]$smokeResult.FinalPublicError)) {
            throw "AOT Quick Access $($Run.Phase) did not prove final public and native NotPinned compensation. Target='$targetFolder'."
        }
    }

    $requiredSteps = @(
        "runtime-native-aot",
        "quick-access-backend-rust",
        "module-loaded",
        "module-handle",
        "module-abi",
        "module-capabilities")
    if (-not $AllowAwaitingExternalCompensation.IsPresent) {
        $requiredSteps += @(
            "cleanup-unpin-request",
            "cleanup-final-not-pinned",
            "cleanup-native-not-pinned")
    }
    if ($Run.Scenario -in @(
            "PinUnpin",
            "PinThenFail",
            "PinThenAwaitExternalCompensation")) {
        $requiredSteps += @(
            "mutation-initial-not-pinned",
            "mutation-pin-request",
            "mutation-pinned-public",
            "mutation-pinned-native")
        if ($smokeResult.InitialPublicState -ne "NotPinned" -or
            $smokeResult.PinnedPublicState -ne "Pinned" -or
            $smokeResult.PinnedNativeState -ne "Pinned" -or
            -not [bool]$smokeResult.PinnedNativeSuccess) {
            throw "AOT Quick Access $($Run.Phase) did not prove NotPinned -> Pinned."
        }
    }
    if ($Run.Scenario -eq "PinUnpin") {
        $requiredSteps += @(
            "mutation-unpin-request",
            "mutation-unpinned-public",
            "mutation-unpinned-native")
        if ($smokeResult.UnpinnedPublicState -ne "NotPinned" -or
            $smokeResult.UnpinnedNativeState -ne "NotPinned" -or
            -not [bool]$smokeResult.UnpinnedNativeSuccess) {
            throw "AOT Quick Access main mutation did not prove NotPinned -> Pinned -> NotPinned."
        }
    }
    elseif ($Run.Scenario -eq "CompensateUnpin") {
        $requiredSteps += "compensation-initial-state-readable"
        if ($smokeResult.InitialPublicState -notin @("Pinned", "NotPinned")) {
            throw "AOT Quick Access compensation could not establish a readable initial state."
        }
        if ($RequireInitiallyPinned.IsPresent -and
            $smokeResult.InitialPublicState -ne "Pinned") {
            throw "AOT Quick Access recovery did not observe the pinned state left by forced termination."
        }
    }

    $missingSteps = @($requiredSteps | Where-Object { $_ -notin @($smokeResult.Steps) })
    if ($missingSteps.Count -gt 0) {
        throw "AOT Quick Access $($Run.Phase) result is missing steps: $($missingSteps -join ', ')."
    }
}

function Assert-InProcessFailureResult {
    param(
        [Parameter(Mandatory)]
        [PSCustomObject]$Run
    )

    Assert-MutationResult -Run $Run -AllowExpectedFailure
}

function Assert-ForcedTerminationResult {
    param(
        [Parameter(Mandatory)]
        [PSCustomObject]$Run
    )

    Assert-MutationResult -Run $Run -AllowAwaitingExternalCompensation
}

if (-not (Test-Path -LiteralPath $launcher -PathType Leaf)) {
    throw "Native AOT preview launcher was not found at '$launcher'."
}
if ([string]::IsNullOrWhiteSpace($SummaryPath)) {
    $SummaryPath = Join-Path $repoRoot ".artifacts\aot-audit\win-x64\summary.json"
}
$SummaryPath = [System.IO.Path]::GetFullPath($SummaryPath)
if (-not (Test-Path -LiteralPath $SummaryPath -PathType Leaf)) {
    throw "Native AOT audit summary was not found at '$SummaryPath'."
}
$auditSummary = Get-Content -LiteralPath $SummaryPath -Raw | ConvertFrom-Json
$expectedExecutablePath = [System.IO.Path]::GetFullPath(
    (Join-Path $auditSummary.publishDirectory "DeskBox.exe"))
if ([string]::IsNullOrWhiteSpace($DataRoot)) {
    $worktreeName = (Split-Path $repoRoot -Leaf) -replace '[^A-Za-z0-9._-]', '-'
    $pathHash = (Get-TextSha256 -Value $repoRoot.ToUpperInvariant()).Substring(0, 8)
    $DataRoot = Join-Path $env:LOCALAPPDATA (
        "DeskBox-AotPreview\{0}-{1}-stage5b2b-quick-access-mutation" -f @(
            $worktreeName,
            $pathHash))
}
$DataRoot = [System.IO.Path]::GetFullPath($DataRoot)
$targetFolder = [System.IO.Path]::GetFullPath((Join-Path $DataRoot (
    "aot-quick-access-mutation-smoke\mutation-target")))
New-Item -ItemType Directory -Path $evidenceRoot -Force | Out-Null

$script:productionBaseline = $null
$preflightRun = $null
$inProcessFailureRun = $null
$forcedTerminationRun = $null
$recoveryRun = $null
$mainRun = $null
$postflightRun = $null
$primaryError = $null
$postflightError = $null
$productionError = $null

try {
    try {
        $preflightRun = Invoke-MutationScenario `
            -Scenario "CompensateUnpin" `
            -Phase "preflight"
        Assert-MutationResult -Run $preflightRun

        $inProcessFailureRun = Invoke-MutationScenario `
            -Scenario "PinThenFail" `
            -Phase "in-process-failure"
        Assert-InProcessFailureResult -Run $inProcessFailureRun

        $forcedTerminationRun = Invoke-MutationScenario `
            -Scenario "PinThenAwaitExternalCompensation" `
            -Phase "forced-termination"
        Assert-ForcedTerminationResult -Run $forcedTerminationRun

        $recoveryRun = Invoke-MutationScenario `
            -Scenario "CompensateUnpin" `
            -Phase "recovery"
        Assert-MutationResult -Run $recoveryRun -RequireInitiallyPinned

        $mainRun = Invoke-MutationScenario `
            -Scenario "PinUnpin" `
            -Phase "main"
        Assert-MutationResult -Run $mainRun
    }
    catch {
        $primaryError = $_
    }
}
finally {
    try {
        $postflightRun = Invoke-MutationScenario `
            -Scenario "CompensateUnpin" `
            -Phase "postflight"
        Assert-MutationResult -Run $postflightRun
    }
    catch {
        $postflightError = $_
    }
}

if ($null -ne $script:productionBaseline) {
    try {
        $productionStateAfter = Get-DirectoryStateFingerprint `
            -Path $script:productionBaseline.root
        if (-not [string]::Equals(
                [string]$productionStateAfter.fingerprint,
                [string]$script:productionBaseline.fingerprint,
                [StringComparison]::OrdinalIgnoreCase) -or
            [int]$productionStateAfter.fileCount -ne [int]$script:productionBaseline.fileCount -or
            [long]$productionStateAfter.bytes -ne [long]$script:productionBaseline.bytes) {
            throw "Production data changed during the complete AOT Quick Access mutation sequence."
        }
    }
    catch {
        $productionError = $_
    }
}

if ($null -ne $postflightError) {
    throw (
        "AOT Quick Access postflight compensation failed. The target may still be pinned; " +
        "inspect or manually unpin '$targetFolder'. Error: $($postflightError.Exception.Message)")
}
if ($null -ne $productionError) {
    throw $productionError
}
if ($null -ne $primaryError) {
    throw (
        "AOT Quick Access mutation sequence failed, but postflight compensation proved " +
        "the target is NotPinned. Error: $($primaryError.Exception.Message)")
}

$sessionPath = Join-Path $evidenceRoot "session.json"
$session = [ordered]@{
    schemaVersion = 1
    generatedAtUtc = [DateTime]::UtcNow.ToString("O")
    summaryPath = $SummaryPath
    dataRoot = $DataRoot
    targetFolder = $targetFolder
    executablePath = $mainRun.Result.ExecutablePath
    executableSha256 = $mainRun.Result.ExecutableSha256
    rustNativePath = $mainRun.Result.ModulePath
    rustNativeSha256 = $mainRun.Result.ModuleSha256
    nativeModuleHandle = $mainRun.Result.ModuleHandle
    abiVersion = [uint32]$mainRun.Result.AbiVersion
    capabilities = [uint64]$mainRun.Result.Capabilities
    preflightResultPath = $preflightRun.EvidencePath
    inProcessFailureResultPath = $inProcessFailureRun.EvidencePath
    forcedTerminationResultPath = $forcedTerminationRun.EvidencePath
    recoveryResultPath = $recoveryRun.EvidencePath
    mainResultPath = $mainRun.EvidencePath
    postflightResultPath = $postflightRun.EvidencePath
    initialPublicState = $mainRun.Result.InitialPublicState
    pinnedPublicState = $mainRun.Result.PinnedPublicState
    pinnedNativeState = $mainRun.Result.PinnedNativeState
    inProcessFailureCleanupSucceeded = [bool]$inProcessFailureRun.Result.CleanupSucceeded
    recoveryInitialPublicState = $recoveryRun.Result.InitialPublicState
    recoveryFinalPublicState = $recoveryRun.Result.FinalPublicState
    recoveryFinalNativeState = $recoveryRun.Result.FinalNativeState
    finalPublicState = $postflightRun.Result.FinalPublicState
    finalNativeState = $postflightRun.Result.FinalNativeState
    cleanupSucceeded = [bool]$postflightRun.Result.CleanupSucceeded
    productionDataRoot = $script:productionBaseline.root
    productionDataFingerprintBefore = $script:productionBaseline.fingerprint
    productionDataFingerprintAfter = $productionStateAfter.fingerprint
    productionDataFileCountAfter = $productionStateAfter.fileCount
    productionDataBytesAfter = $productionStateAfter.bytes
}
$session | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $sessionPath -Encoding UTF8

[PSCustomObject]@{
    Scenario = "PinUnpin"
    Success = $true
    InitialPublicState = $mainRun.Result.InitialPublicState
    PinnedPublicState = $mainRun.Result.PinnedPublicState
    PinnedNativeState = $mainRun.Result.PinnedNativeState
    InProcessFailureCleanupSucceeded = [bool]$inProcessFailureRun.Result.CleanupSucceeded
    ForcedTerminationLeftPinned = $recoveryRun.Result.InitialPublicState -eq "Pinned"
    RecoveryFinalPublicState = $recoveryRun.Result.FinalPublicState
    RecoveryFinalNativeState = $recoveryRun.Result.FinalNativeState
    FinalPublicState = $postflightRun.Result.FinalPublicState
    FinalNativeState = $postflightRun.Result.FinalNativeState
    CleanupSucceeded = [bool]$postflightRun.Result.CleanupSucceeded
    Exe = $mainRun.Result.ExecutablePath
    ExeSha256 = $mainRun.Result.ExecutableSha256
    RustNativeDll = $mainRun.Result.ModulePath
    RustNativeSha256 = $mainRun.Result.ModuleSha256
    AbiVersion = [uint32]$mainRun.Result.AbiVersion
    Capabilities = [uint64]$mainRun.Result.Capabilities
    DataRoot = $DataRoot
    TargetFolder = $targetFolder
    SessionPath = $sessionPath
    ProductionDataFingerprint = $productionStateAfter.fingerprint
}
