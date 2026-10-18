[CmdletBinding()]
param(
    [string]$SummaryPath,

    [string]$DataRoot,

    [ValidateRange(10, 600)]
    [int]$TimeoutSeconds = 120
)

$ErrorActionPreference = "Stop"
$musicMutationSmokeEnvironmentVariable = "DESKBOX_AOT_MUSIC_VOLUME_MUTATION_SMOKE"
$musicSessionMutationSmokeEnvironmentVariable =
    "DESKBOX_AOT_MUSIC_VOLUME_SESSION_MUTATION_SMOKE"
$musicReadSmokeEnvironmentVariable = "DESKBOX_AOT_MUSIC_VOLUME_READ_SMOKE"
$shortcutSmokeEnvironmentVariable = "DESKBOX_AOT_SHORTCUT_SMOKE"
$shellSmokeEnvironmentVariable = "DESKBOX_AOT_SHELL_SMOKE"
$mutationSmokeEnvironmentVariable = "DESKBOX_AOT_QUICK_ACCESS_MUTATION_SMOKE"
$managedUiSmokeEnvironmentVariable = "DESKBOX_AOT_MANAGED_UI_SMOKE"
$musicBackendEnvironmentVariable = "DESKBOX_MUSIC_VOLUME_BACKEND"
$systemVolumeTolerance = 0.005
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$launcher = Join-Path $PSScriptRoot "start-aot-preview.ps1"
$previewSessionPath = Join-Path $repoRoot ".artifacts\aot-preview\win-x64\session.json"
$evidenceRoot = Join-Path $repoRoot ".artifacts\aot-music-volume-mutation-smoke\win-x64"

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

function Get-ExactPreviewProcesses {
    param(
        [Parameter(Mandatory)]
        [string]$ExecutablePath
    )

    return @(
        Get-CimInstance Win32_Process -Filter "Name='DeskBox.exe'" |
            Where-Object {
                -not [string]::IsNullOrWhiteSpace($_.ExecutablePath) -and
                (Test-PathEqual -Left $_.ExecutablePath -Right $ExecutablePath)
            }
    )
}

function Stop-ExactPreviewProcess {
    param(
        [Parameter(Mandatory)]
        [string]$ExecutablePath
    )

    $processes = @(Get-ExactPreviewProcesses -ExecutablePath $ExecutablePath)
    foreach ($process in $processes) {
        Stop-Process -Id $process.ProcessId -Force -ErrorAction SilentlyContinue
    }
    foreach ($process in $processes) {
        Wait-Process -Id $process.ProcessId -Timeout 5 -ErrorAction SilentlyContinue
    }
}

function Test-NormalizedVolume {
    param([double]$Value)

    return -not [double]::IsNaN($Value) -and
        -not [double]::IsInfinity($Value) -and
        $Value -ge 0.0 -and
        $Value -le 1.0
}

function Assert-NativeSystemRead {
    param(
        [Parameter(Mandatory)]
        $Evidence,

        [Parameter(Mandatory)]
        [string]$Name
    )

    if ($null -eq $Evidence -or
        -not [bool]$Evidence.success -or
        [uint32]$Evidence.status -ne 0 -or
        [int]$Evidence.operationHResult -lt 0 -or
        [int]$Evidence.deviceHResult -lt 0 -or
        [int]$Evidence.systemHResult -lt 0 -or
        (([uint32]$Evidence.attemptedPhases -band 0x0F) -ne 0x0F) -or
        -not (Test-NormalizedVolume -Value ([double]$Evidence.systemVolume))) {
        throw "$Name does not prove a successful real Rust endpoint/system read."
    }
}

function Get-ScenarioDirectoryName {
    param(
        [Parameter(Mandatory)]
        [ValidateSet(
            "ChangeRestore",
            "ChangeThenFail",
            "ChangeThenAwaitExternalRecovery",
            "RecoverOriginal")]
        [string]$Scenario
    )

    switch ($Scenario) {
        "ChangeRestore" { return "change-restore" }
        "ChangeThenFail" { return "change-then-fail" }
        "ChangeThenAwaitExternalRecovery" {
            return "change-then-await-external-recovery"
        }
        default { return "recover-original" }
    }
}

function Invoke-MusicVolumeMutationScenario {
    param(
        [Parameter(Mandatory)]
        [ValidateSet(
            "ChangeRestore",
            "ChangeThenFail",
            "ChangeThenAwaitExternalRecovery",
            "RecoverOriginal")]
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
        "aot-music-volume-mutation-smoke\$scenarioDirectoryName\result.json")
    $temporaryResultPath = $resultPath + ".tmp"
    if (Test-Path -LiteralPath $resultPath -PathType Leaf) {
        Remove-Item -LiteralPath $resultPath -Force
    }
    if (Test-Path -LiteralPath $temporaryResultPath -PathType Leaf) {
        Remove-Item -LiteralPath $temporaryResultPath -Force
    }

    $previewSession = $null
    try {
        $previousMusicMutationSmoke = [Environment]::GetEnvironmentVariable(
            $musicMutationSmokeEnvironmentVariable,
            "Process")
        $previousMusicSessionMutationSmoke = [Environment]::GetEnvironmentVariable(
            $musicSessionMutationSmokeEnvironmentVariable,
            "Process")
        $previousMusicReadSmoke = [Environment]::GetEnvironmentVariable(
            $musicReadSmokeEnvironmentVariable,
            "Process")
        $previousShortcutSmoke = [Environment]::GetEnvironmentVariable(
            $shortcutSmokeEnvironmentVariable,
            "Process")
        $previousShellSmoke = [Environment]::GetEnvironmentVariable(
            $shellSmokeEnvironmentVariable,
            "Process")
        $previousMutationSmoke = [Environment]::GetEnvironmentVariable(
            $mutationSmokeEnvironmentVariable,
            "Process")
        $previousManagedUiSmoke = [Environment]::GetEnvironmentVariable(
            $managedUiSmokeEnvironmentVariable,
            "Process")
        $previousMusicBackend = [Environment]::GetEnvironmentVariable(
            $musicBackendEnvironmentVariable,
            "Process")
        try {
            [Environment]::SetEnvironmentVariable(
                $musicMutationSmokeEnvironmentVariable,
                $Scenario,
                "Process")
            [Environment]::SetEnvironmentVariable(
                $musicSessionMutationSmokeEnvironmentVariable,
                $null,
                "Process")
            [Environment]::SetEnvironmentVariable(
                $musicReadSmokeEnvironmentVariable,
                $null,
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
                $mutationSmokeEnvironmentVariable,
                $null,
                "Process")
            [Environment]::SetEnvironmentVariable(
                $managedUiSmokeEnvironmentVariable,
                $null,
                "Process")
            [Environment]::SetEnvironmentVariable(
                $musicBackendEnvironmentVariable,
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
                $musicMutationSmokeEnvironmentVariable,
                $previousMusicMutationSmoke,
                "Process")
            [Environment]::SetEnvironmentVariable(
                $musicSessionMutationSmokeEnvironmentVariable,
                $previousMusicSessionMutationSmoke,
                "Process")
            [Environment]::SetEnvironmentVariable(
                $musicReadSmokeEnvironmentVariable,
                $previousMusicReadSmoke,
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
                $mutationSmokeEnvironmentVariable,
                $previousMutationSmoke,
                "Process")
            [Environment]::SetEnvironmentVariable(
                $managedUiSmokeEnvironmentVariable,
                $previousManagedUiSmoke,
                "Process")
            [Environment]::SetEnvironmentVariable(
                $musicBackendEnvironmentVariable,
                $previousMusicBackend,
                "Process")
        }

        if (-not (Test-Path -LiteralPath $previewSessionPath -PathType Leaf)) {
            throw "Native AOT preview session.json was not created at '$previewSessionPath'."
        }
        $previewSession = Get-Content -LiteralPath $previewSessionPath -Raw |
            ConvertFrom-Json
        if (-not (Test-PathEqual -Left $previewSession.previewDataRoot -Right $DataRoot)) {
            throw "The $Phase preview data root does not match the mutation root."
        }
        if (-not (Test-PathEqual `
                -Left $previewSession.executablePath `
                -Right $script:expectedExecutablePath)) {
            throw "The $Phase preview executable does not match the audited executable."
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
            throw "The $Phase preview changed the production data root."
        }

        $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
        $smokeResult = $null
        while ([DateTime]::UtcNow -lt $deadline) {
            if (Test-Path -LiteralPath $resultPath -PathType Leaf) {
                try {
                    $candidate = Get-Content -LiteralPath $resultPath -Raw |
                        ConvertFrom-Json
                    if ([int]$candidate.processId -eq
                        [int]$previewSession.primaryProcessId) {
                        $smokeResult = $candidate
                        $awaitingExternal =
                            $Scenario -eq "ChangeThenAwaitExternalRecovery" -and
                            $candidate.state -eq "AwaitingExternalRecovery"
                        if ($candidate.state -eq "Completed" -or
                            $candidate.state -eq "Failed" -or
                            $awaitingExternal) {
                            break
                        }
                    }
                }
                catch {
                    # Retry a bounded transient sharing race during atomic JSON replacement.
                }
            }

            Start-Sleep -Milliseconds 250
        }

        $expectedExternalWait =
            $null -ne $smokeResult -and
            $Scenario -eq "ChangeThenAwaitExternalRecovery" -and
            $smokeResult.state -eq "AwaitingExternalRecovery"
        if ($null -eq $smokeResult -or
            ($smokeResult.state -ne "Completed" -and
                $smokeResult.state -ne "Failed" -and
                -not $expectedExternalWait)) {
            throw "AOT music-volume $Phase/$Scenario timed out after $TimeoutSeconds seconds. Last state='$($smokeResult.state)'; result='$resultPath'."
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
            $script:expectedExecutablePath
        }
        if (-not [string]::IsNullOrWhiteSpace($pathToStop)) {
            Stop-ExactPreviewProcess -ExecutablePath $pathToStop
        }
    }
}

function Assert-MusicVolumeMutationResult {
    param(
        [Parameter(Mandatory)]
        [PSCustomObject]$Run,

        [switch]$AllowExpectedFailure,

        [switch]$AllowAwaitingExternalRecovery,

        [switch]$RequireRecoveryIntent,

        [switch]$RequireNoRecoveryIntent,

        [Nullable[double]]$ExpectedOriginalVolume
    )

    $result = $Run.Result
    $preview = $Run.PreviewSession
    if ($AllowExpectedFailure.IsPresent) {
        if ($result.state -ne "Failed" -or
            [bool]$result.success -or
            -not ([string]$result.error).Contains(
                "intentional-after-system-volume-change",
                [StringComparison]::Ordinal)) {
            throw "AOT music-volume $($Run.Phase) did not produce the expected failure."
        }
    }
    elseif ($AllowAwaitingExternalRecovery.IsPresent) {
        if ($result.state -ne "AwaitingExternalRecovery" -or
            [bool]$result.success -or
            -not [bool]$result.recoveryIntentPreserved) {
            throw "AOT music-volume $($Run.Phase) did not reach the recovery checkpoint."
        }
    }
    elseif ($result.state -ne "Completed" -or -not [bool]$result.success) {
        throw "AOT music-volume $($Run.Phase)/$($Run.Scenario) failed: $($result.error)"
    }

    if ($result.scenario -ne $Run.Scenario -or
        [int]$result.processId -ne [int]$preview.primaryProcessId -or
        -not (Test-PathEqual -Left $result.previewDataRoot -Right $DataRoot)) {
        throw "AOT music-volume $($Run.Phase) evidence does not match scenario/root/PID."
    }
    if (-not (Test-PathEqual -Left $result.executablePath -Right $preview.executablePath) -or
        -not [string]::Equals(
            [string]$result.executableSha256,
            [string]$preview.executableSha256,
            [StringComparison]::OrdinalIgnoreCase)) {
        throw "AOT music-volume $($Run.Phase) executable identity is untrusted."
    }
    if (-not (Test-PathEqual -Left $result.modulePath -Right $preview.rustNativePath) -or
        -not [string]::Equals(
            [string]$result.moduleSha256,
            [string]$preview.rustNativeSha256,
            [StringComparison]::OrdinalIgnoreCase)) {
        throw "AOT music-volume $($Run.Phase) Rust module identity is untrusted."
    }
    if ([bool]$result.isDynamicCodeSupported -or
        $result.selectedBackend -ne "Rust" -or
        $result.loadState -ne "Loaded" -or
        [uint32]$result.abiVersion -ne 2 -or
        (([uint64]$result.capabilities -band 0x20) -ne 0x20) -or
        [string]::IsNullOrWhiteSpace([string]$result.moduleHandle) -or
        $result.moduleHandle -eq "0x0") {
        throw "AOT music-volume $($Run.Phase) did not prove the loaded Rust boundary."
    }

    $requiredSteps = @(
        "runtime-native-aot",
        "music-volume-backend-rust",
        "module-loaded",
        "module-handle",
        "module-abi",
        "module-music-volume-capability")
    if ($Run.Scenario -in @(
            "ChangeRestore",
            "ChangeThenFail",
            "ChangeThenAwaitExternalRecovery")) {
        Assert-NativeSystemRead -Evidence $result.nativeInitial -Name "nativeInitial"
        Assert-NativeSystemRead -Evidence $result.nativeProbe -Name "nativeProbe"
        if (-not [bool]$result.recoveryIntentPersisted -or
            -not [bool]$result.probeRequestSucceeded -or
            [Math]::Abs(
                [double]$result.observedProbeVolume -
                [double]$result.probeVolume) -gt $systemVolumeTolerance -or
            [Math]::Abs(
                [double]$result.probeVolume -
                [double]$result.originalVolume) -lt 0.04) {
            throw "AOT music-volume $($Run.Phase) did not prove a durable real change."
        }
        $requiredSteps += @(
            "recovery-intent-read-back",
            "product-probe-setter-succeeded",
            "probe-volume-verified")
    }

    if (-not $AllowAwaitingExternalRecovery.IsPresent) {
        if (-not [bool]$result.cleanupSucceeded -or
            [bool]$result.recoveryIntentPreserved) {
            throw "AOT music-volume $($Run.Phase) did not finish with verified recovery."
        }
        Assert-NativeSystemRead -Evidence $result.nativeFinal -Name "nativeFinal"
        if ($Run.Scenario -eq "RecoverOriginal" -and
            -not [bool]$result.recoveryIntentFound) {
            $requiredSteps += @(
                "recovery-no-intent-system-volume-hresults",
                "recovery-no-intent-system-volume-phases",
                "recovery-no-intent-system-volume-normalized",
                "recovery-no-intent")
        }
        else {
            $requiredSteps += @(
                "recovery-final-system-volume-hresults",
                "recovery-final-system-volume-phases",
                "recovery-final-system-volume-normalized")
            $requiredSteps += @(
                "product-restore-setter-succeeded",
                "recovery-original-verified",
                "recovery-intent-cleared-after-verification")
        }
    }

    if ($RequireRecoveryIntent.IsPresent -and
        (-not [bool]$result.recoveryIntentFound -or
            -not [bool]$result.recoveryIntentLoaded)) {
        throw "AOT music-volume recovery did not load the forced-termination intent."
    }
    if ($RequireNoRecoveryIntent.IsPresent -and [bool]$result.recoveryIntentFound) {
        throw "AOT music-volume $($Run.Phase) unexpectedly found a stale recovery intent."
    }
    if ($null -ne $ExpectedOriginalVolume -and
        [Math]::Abs(
            [double]$result.finalVolume -
            [double]$ExpectedOriginalVolume) -gt $systemVolumeTolerance) {
        throw "AOT music-volume $($Run.Phase) did not restore the expected original volume."
    }

    $missingSteps = @($requiredSteps | Where-Object { $_ -notin @($result.steps) })
    if ($missingSteps.Count -gt 0) {
        throw "AOT music-volume $($Run.Phase) is missing steps: $($missingSteps -join ', ')."
    }
}

function Get-RecoveryHint {
    $hint = "Original system volume: unknown."
    if (Test-Path -LiteralPath $recoveryIntentPath -PathType Leaf) {
        try {
            $intent = Get-Content -LiteralPath $recoveryIntentPath -Raw |
                ConvertFrom-Json
            $hint = "Original system volume: $([double]$intent.originalVolume)."
        }
        catch {
            $hint = "Original system volume: unreadable from preserved intent."
        }
    }

    return "$hint Recovery intent: '$recoveryIntentPath'."
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
$script:expectedExecutablePath = [System.IO.Path]::GetFullPath(
    (Join-Path $auditSummary.publishDirectory "DeskBox.exe"))
if ([string]::IsNullOrWhiteSpace($DataRoot)) {
    $worktreeName = (Split-Path $repoRoot -Leaf) -replace '[^A-Za-z0-9._-]', '-'
    $pathHash = (Get-TextSha256 -Value $repoRoot.ToUpperInvariant()).Substring(0, 8)
    $DataRoot = Join-Path $env:LOCALAPPDATA (
        "DeskBox-AotPreview\{0}-{1}-stage5b3b-music-volume-mutation" -f @(
            $worktreeName,
            $pathHash))
}
$DataRoot = [System.IO.Path]::GetFullPath($DataRoot)
$recoveryIntentPath = Join-Path $DataRoot (
    "aot-music-volume-mutation-smoke\recovery-intent.json")
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
$originalVolume = $null

try {
    try {
        $preflightRun = Invoke-MusicVolumeMutationScenario `
            -Scenario "RecoverOriginal" `
            -Phase "preflight"
        Assert-MusicVolumeMutationResult `
            -Run $preflightRun `
            -RequireNoRecoveryIntent
        $originalVolume = [double]$preflightRun.Result.finalVolume

        $inProcessFailureRun = Invoke-MusicVolumeMutationScenario `
            -Scenario "ChangeThenFail" `
            -Phase "in-process-failure"
        Assert-MusicVolumeMutationResult `
            -Run $inProcessFailureRun `
            -AllowExpectedFailure `
            -ExpectedOriginalVolume $originalVolume

        $forcedTerminationRun = Invoke-MusicVolumeMutationScenario `
            -Scenario "ChangeThenAwaitExternalRecovery" `
            -Phase "forced-termination"
        Assert-MusicVolumeMutationResult `
            -Run $forcedTerminationRun `
            -AllowAwaitingExternalRecovery

        $recoveryRun = Invoke-MusicVolumeMutationScenario `
            -Scenario "RecoverOriginal" `
            -Phase "recovery"
        Assert-MusicVolumeMutationResult `
            -Run $recoveryRun `
            -RequireRecoveryIntent `
            -ExpectedOriginalVolume $originalVolume
        if ([Math]::Abs(
                [double]$recoveryRun.Result.recoveryObservedVolume -
                [double]$forcedTerminationRun.Result.probeVolume) -gt
            $systemVolumeTolerance) {
            throw "Independent recovery did not observe the probe left by forced termination."
        }

        $mainRun = Invoke-MusicVolumeMutationScenario `
            -Scenario "ChangeRestore" `
            -Phase "main"
        Assert-MusicVolumeMutationResult `
            -Run $mainRun `
            -ExpectedOriginalVolume $originalVolume
    }
    catch {
        $primaryError = $_
    }
}
finally {
    try {
        $postflightRun = Invoke-MusicVolumeMutationScenario `
            -Scenario "RecoverOriginal" `
            -Phase "postflight"
        Assert-MusicVolumeMutationResult `
            -Run $postflightRun `
            -RequireNoRecoveryIntent
        if ($null -ne $originalVolume -and
            [Math]::Abs(
                [double]$postflightRun.Result.finalVolume -
                [double]$originalVolume) -gt $systemVolumeTolerance) {
            throw "Postflight read differs from the saved original system volume."
        }
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
            throw "Production data changed during the AOT music-volume mutation sequence."
        }
    }
    catch {
        $productionError = $_
    }
}

$remainingProcesses = @(Get-ExactPreviewProcesses `
    -ExecutablePath $script:expectedExecutablePath)
if ($remainingProcesses.Count -ne 0) {
    throw "Audited AOT process cleanup failed; PIDs=$($remainingProcesses.ProcessId -join ','). $(Get-RecoveryHint)"
}
if ($null -ne $postflightError) {
    throw "AOT music-volume postflight recovery failed. $(Get-RecoveryHint) Error: $($postflightError.Exception.Message)"
}
if ($null -ne $productionError) {
    throw $productionError
}
if ($null -ne $primaryError) {
    throw "AOT music-volume sequence failed, but postflight recovery succeeded and the original volume was verified. Error: $($primaryError.Exception.Message)"
}

$sessionPath = Join-Path $evidenceRoot "session.json"
$session = [ordered]@{
    schemaVersion = 1
    generatedAtUtc = [DateTime]::UtcNow.ToString("O")
    summaryPath = $SummaryPath
    dataRoot = $DataRoot
    recoveryIntentPath = $recoveryIntentPath
    executablePath = $mainRun.Result.executablePath
    executableSha256 = $mainRun.Result.executableSha256
    rustNativePath = $mainRun.Result.modulePath
    rustNativeSha256 = $mainRun.Result.moduleSha256
    nativeModuleHandle = $mainRun.Result.moduleHandle
    abiVersion = [uint32]$mainRun.Result.abiVersion
    capabilities = [uint64]$mainRun.Result.capabilities
    originalVolume = [double]$originalVolume
    inProcessFailureProbeVolume = [double]$inProcessFailureRun.Result.probeVolume
    inProcessFailureFinalVolume = [double]$inProcessFailureRun.Result.finalVolume
    forcedTerminationProbeVolume = [double]$forcedTerminationRun.Result.probeVolume
    recoveryObservedVolume = [double]$recoveryRun.Result.recoveryObservedVolume
    recoveryFinalVolume = [double]$recoveryRun.Result.finalVolume
    mainProbeVolume = [double]$mainRun.Result.probeVolume
    mainFinalVolume = [double]$mainRun.Result.finalVolume
    postflightFinalVolume = [double]$postflightRun.Result.finalVolume
    preflightResultPath = $preflightRun.EvidencePath
    inProcessFailureResultPath = $inProcessFailureRun.EvidencePath
    forcedTerminationResultPath = $forcedTerminationRun.EvidencePath
    recoveryResultPath = $recoveryRun.EvidencePath
    mainResultPath = $mainRun.EvidencePath
    postflightResultPath = $postflightRun.EvidencePath
    cleanupSucceeded = [bool]$postflightRun.Result.cleanupSucceeded
    recoveryIntentPreserved = Test-Path -LiteralPath $recoveryIntentPath -PathType Leaf
    productionDataRoot = $script:productionBaseline.root
    productionDataFingerprintBefore = $script:productionBaseline.fingerprint
    productionDataFingerprintAfter = $productionStateAfter.fingerprint
    productionDataFileCountAfter = $productionStateAfter.fileCount
    productionDataBytesAfter = $productionStateAfter.bytes
}
$session | ConvertTo-Json -Depth 6 |
    Set-Content -LiteralPath $sessionPath -Encoding UTF8

[PSCustomObject]@{
    Scenario = "ChangeRestore"
    Success = $true
    OriginalVolume = [double]$originalVolume
    InProcessFailureProbeVolume = [double]$inProcessFailureRun.Result.probeVolume
    InProcessFailureFinalVolume = [double]$inProcessFailureRun.Result.finalVolume
    ForcedTerminationProbeVolume = [double]$forcedTerminationRun.Result.probeVolume
    RecoveryObservedVolume = [double]$recoveryRun.Result.recoveryObservedVolume
    RecoveryFinalVolume = [double]$recoveryRun.Result.finalVolume
    MainProbeVolume = [double]$mainRun.Result.probeVolume
    FinalVolume = [double]$postflightRun.Result.finalVolume
    RecoveryIntentPreserved = Test-Path -LiteralPath $recoveryIntentPath -PathType Leaf
    CleanupSucceeded = [bool]$postflightRun.Result.cleanupSucceeded
    Exe = $mainRun.Result.executablePath
    ExeSha256 = $mainRun.Result.executableSha256
    RustNativeDll = $mainRun.Result.modulePath
    RustNativeSha256 = $mainRun.Result.moduleSha256
    AbiVersion = [uint32]$mainRun.Result.abiVersion
    Capabilities = [uint64]$mainRun.Result.capabilities
    DataRoot = $DataRoot
    SessionPath = $sessionPath
    ProductionDataFingerprint = $productionStateAfter.fingerprint
}
