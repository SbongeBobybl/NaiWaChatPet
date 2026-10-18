[CmdletBinding()]
param(
    [string]$SummaryPath,

    [string]$DataRoot,

    [ValidateRange(10, 600)]
    [int]$TimeoutSeconds = 120,

    [ValidateRange(3, 60)]
    [int]$FixtureReadySeconds = 15
)

$ErrorActionPreference = "Stop"
$musicSessionMutationSmokeEnvironmentVariable =
    "DESKBOX_AOT_MUSIC_VOLUME_SESSION_MUTATION_SMOKE"
$musicSessionFixturePidEnvironmentVariable =
    "DESKBOX_AOT_MUSIC_VOLUME_SESSION_FIXTURE_PID"
$musicMutationSmokeEnvironmentVariable = "DESKBOX_AOT_MUSIC_VOLUME_MUTATION_SMOKE"
$musicReadSmokeEnvironmentVariable = "DESKBOX_AOT_MUSIC_VOLUME_READ_SMOKE"
$shortcutSmokeEnvironmentVariable = "DESKBOX_AOT_SHORTCUT_SMOKE"
$shellSmokeEnvironmentVariable = "DESKBOX_AOT_SHELL_SMOKE"
$mutationSmokeEnvironmentVariable = "DESKBOX_AOT_QUICK_ACCESS_MUTATION_SMOKE"
$managedUiSmokeEnvironmentVariable = "DESKBOX_AOT_MANAGED_UI_SMOKE"
$musicBackendEnvironmentVariable = "DESKBOX_MUSIC_VOLUME_BACKEND"
$sessionVolumeTolerance = 0.005
$systemVolumeTolerance = 0.005
$expectedMatchKind = 4
$controlledFixtureProcessName = "deskbox-audio-session-fixture"
$controlledSourceAppUserModelId = "DeskBox.Aot.Controlled.Session.Identity"
$controlledSourceDisplayName = $controlledFixtureProcessName
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$launcher = Join-Path $PSScriptRoot "start-aot-preview.ps1"
$previewSessionPath = Join-Path $repoRoot ".artifacts\aot-preview\win-x64\session.json"
$evidenceRoot = Join-Path $repoRoot (
    ".artifacts\aot-music-volume-session-mutation-smoke\win-x64")
$fixtureManifest = Join-Path $repoRoot (
    "native\deskbox-audio-session-fixture\Cargo.toml")
$fixtureTargetTriple = "x86_64-pc-windows-msvc"
$fixtureExecutablePath = Join-Path $repoRoot (
    "native\target\$fixtureTargetTriple\release\deskbox-audio-session-fixture.exe")

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

function Stop-ExactFixtureProcess {
    param(
        [Parameter(Mandatory)]
        [int]$ProcessId,

        [Parameter(Mandatory)]
        [string]$ExecutablePath
    )

    $candidate = Get-CimInstance Win32_Process -Filter "ProcessId=$ProcessId" -ErrorAction SilentlyContinue
    if ($null -ne $candidate -and
        -not [string]::IsNullOrWhiteSpace([string]$candidate.ExecutablePath) -and
        (Test-PathEqual -Left $candidate.ExecutablePath -Right $ExecutablePath)) {
        Stop-Process -Id $ProcessId -Force -ErrorAction SilentlyContinue
        Wait-Process -Id $ProcessId -Timeout 5 -ErrorAction SilentlyContinue
    }
}

function ConvertTo-QuotedProcessArgument {
    param(
        [Parameter(Mandatory)]
        [string]$Value
    )

    return '"' + $Value.Replace('"', '\"') + '"'
}

function Test-NormalizedVolume {
    param([double]$Value)

    return -not [double]::IsNaN($Value) -and
        -not [double]::IsInfinity($Value) -and
        $Value -ge 0.0 -and
        $Value -le 1.0
}

function Assert-NativeMatchedSessionRead {
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
        [int]$Evidence.sessionHResult -lt 0 -or
        (([uint32]$Evidence.attemptedPhases -band 0x3F) -ne 0x3F) -or
        -not [bool]$Evidence.hasSessionVolume -or
        [uint32]$Evidence.matchKind -ne $expectedMatchKind -or
        -not (Test-NormalizedVolume -Value ([double]$Evidence.systemVolume)) -or
        -not (Test-NormalizedVolume -Value ([double]$Evidence.sessionVolume))) {
        throw "$Name does not prove a successful kind-$expectedMatchKind Rust session match."
    }
}

function Get-ScenarioDirectoryName {
    param(
        [Parameter(Mandatory)]
        [ValidateSet(
            "ReadMatchedSession",
            "ChangeRestore",
            "ChangeThenFail",
            "ChangeThenAwaitExternalRecovery",
            "RecoverOriginal")]
        [string]$Scenario
    )

    switch ($Scenario) {
        "ReadMatchedSession" { return "read-matched-session" }
        "ChangeRestore" { return "change-restore" }
        "ChangeThenFail" { return "change-then-fail" }
        "ChangeThenAwaitExternalRecovery" {
            return "change-then-await-external-recovery"
        }
        default { return "recover-original" }
    }
}

function Invoke-MusicVolumeSessionMutationScenario {
    param(
        [Parameter(Mandatory)]
        [ValidateSet(
            "ReadMatchedSession",
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

    if ($null -eq $script:fixtureProcess -or $script:fixtureProcess.HasExited) {
        throw "The controlled silent audio fixture is not running before $Phase."
    }

    $scenarioDirectoryName = Get-ScenarioDirectoryName -Scenario $Scenario
    $resultPath = Join-Path $DataRoot (
        "aot-music-volume-session-mutation-smoke\$scenarioDirectoryName\result.json")
    $temporaryResultPath = $resultPath + ".tmp"
    if (Test-Path -LiteralPath $resultPath -PathType Leaf) {
        Remove-Item -LiteralPath $resultPath -Force
    }
    if (Test-Path -LiteralPath $temporaryResultPath -PathType Leaf) {
        Remove-Item -LiteralPath $temporaryResultPath -Force
    }

    $previewSession = $null
    try {
        $previousMusicSessionMutationSmoke = [Environment]::GetEnvironmentVariable(
            $musicSessionMutationSmokeEnvironmentVariable,
            "Process")
        $previousMusicMutationSmoke = [Environment]::GetEnvironmentVariable(
            $musicMutationSmokeEnvironmentVariable,
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
        $previousMusicSessionFixturePid = [Environment]::GetEnvironmentVariable(
            $musicSessionFixturePidEnvironmentVariable,
            "Process")
        try {
            [Environment]::SetEnvironmentVariable(
                $musicSessionMutationSmokeEnvironmentVariable,
                $Scenario,
                "Process")
            [Environment]::SetEnvironmentVariable(
                $musicSessionFixturePidEnvironmentVariable,
                $script:fixtureProcess.Id.ToString(),
                "Process")
            [Environment]::SetEnvironmentVariable(
                $musicMutationSmokeEnvironmentVariable,
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
                $musicSessionMutationSmokeEnvironmentVariable,
                $previousMusicSessionMutationSmoke,
                "Process")
            [Environment]::SetEnvironmentVariable(
                $musicSessionFixturePidEnvironmentVariable,
                $previousMusicSessionFixturePid,
                "Process")
            [Environment]::SetEnvironmentVariable(
                $musicMutationSmokeEnvironmentVariable,
                $previousMusicMutationSmoke,
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
            throw "The $Phase preview data root does not match the session mutation root."
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
            throw "AOT session-volume $Phase/$Scenario timed out after $TimeoutSeconds seconds. Last state='$($smokeResult.state)'; result='$resultPath'."
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

function Assert-MusicVolumeSessionMutationResult {
    param(
        [Parameter(Mandatory)]
        [PSCustomObject]$Run,

        [switch]$AllowExpectedFailure,

        [switch]$AllowAwaitingExternalRecovery,

        [switch]$RequireRecoveryIntent,

        [switch]$RequireNoRecoveryIntent,

        [Nullable[double]]$ExpectedOriginalSessionVolume
    )

    $result = $Run.Result
    $preview = $Run.PreviewSession
    if ($AllowExpectedFailure.IsPresent) {
        if ($result.state -ne "Failed" -or
            [bool]$result.success -or
            -not ([string]$result.error).Contains(
                "intentional-after-session-volume-change",
                [StringComparison]::Ordinal)) {
            throw "AOT session-volume $($Run.Phase) did not produce the expected failure."
        }
    }
    elseif ($AllowAwaitingExternalRecovery.IsPresent) {
        if ($result.state -ne "AwaitingExternalRecovery" -or
            [bool]$result.success -or
            -not [bool]$result.recoveryIntentPreserved) {
            throw "AOT session-volume $($Run.Phase) did not reach the recovery checkpoint."
        }
    }
    elseif ($result.state -ne "Completed" -or -not [bool]$result.success) {
        throw "AOT session-volume $($Run.Phase)/$($Run.Scenario) failed: $($result.error)"
    }

    if ($result.scenario -ne $Run.Scenario -or
        [int]$result.processId -ne [int]$preview.primaryProcessId -or
        [int]$result.fixtureProcessId -ne [int]$script:fixtureProcess.Id -or
        $result.fixtureProcessName -ne $controlledFixtureProcessName -or
        $result.sourceAppUserModelId -ne $controlledSourceAppUserModelId -or
        $result.sourceDisplayName -ne $controlledSourceDisplayName -or
        -not (Test-PathEqual -Left $result.previewDataRoot -Right $DataRoot)) {
        throw "AOT session-volume $($Run.Phase) evidence does not match root/PIDs/identity."
    }
    if (-not (Test-PathEqual -Left $result.executablePath -Right $preview.executablePath) -or
        -not [string]::Equals(
            [string]$result.executableSha256,
            [string]$preview.executableSha256,
            [StringComparison]::OrdinalIgnoreCase)) {
        throw "AOT session-volume $($Run.Phase) executable identity is untrusted."
    }
    if (-not (Test-PathEqual -Left $result.modulePath -Right $preview.rustNativePath) -or
        -not [string]::Equals(
            [string]$result.moduleSha256,
            [string]$preview.rustNativeSha256,
            [StringComparison]::OrdinalIgnoreCase)) {
        throw "AOT session-volume $($Run.Phase) Rust module identity is untrusted."
    }
    if ([bool]$result.isDynamicCodeSupported -or
        $result.selectedBackend -ne "Rust" -or
        $result.loadState -ne "Loaded" -or
        [uint32]$result.abiVersion -ne 2 -or
        (([uint64]$result.capabilities -band 0x20) -ne 0x20) -or
        [string]::IsNullOrWhiteSpace([string]$result.moduleHandle) -or
        $result.moduleHandle -eq "0x0") {
        throw "AOT session-volume $($Run.Phase) did not prove the loaded Rust boundary."
    }

    $requiredSteps = @(
        "runtime-native-aot",
        "music-volume-backend-rust",
        "module-loaded",
        "module-handle",
        "module-abi",
        "module-music-volume-capability")
    if ($Run.Scenario -eq "ReadMatchedSession") {
        Assert-NativeMatchedSessionRead -Evidence $result.nativeInitial -Name "nativeInitial"
        Assert-NativeMatchedSessionRead -Evidence $result.nativeFinal -Name "nativeFinal"
        if ([Math]::Abs(
                [double]$result.initialSystemVolume -
                [double]$result.finalSystemVolume) -gt $systemVolumeTolerance -or
            [Math]::Abs(
                [double]$result.originalSessionVolume -
                [double]$result.finalSessionVolume) -gt $sessionVolumeTolerance) {
            throw "AOT session-volume $($Run.Phase) read-only values changed."
        }
        $requiredSteps += "read-matched-session-completed"
    }
    elseif ($Run.Scenario -in @(
            "ChangeRestore",
            "ChangeThenFail",
            "ChangeThenAwaitExternalRecovery")) {
        Assert-NativeMatchedSessionRead -Evidence $result.nativeInitial -Name "nativeInitial"
        Assert-NativeMatchedSessionRead -Evidence $result.nativeProbe -Name "nativeProbe"
        if (-not [bool]$result.recoveryIntentPersisted -or
            -not [bool]$result.probeRequestSucceeded -or
            [Math]::Abs(
                [double]$result.observedProbeSessionVolume -
                [double]$result.probeSessionVolume) -gt $sessionVolumeTolerance -or
            [Math]::Abs(
                [double]$result.probeSessionVolume -
                [double]$result.originalSessionVolume) -lt 0.06 -or
            [Math]::Abs(
                [double]$result.nativeProbe.systemVolume -
                [double]$result.initialSystemVolume) -gt $systemVolumeTolerance) {
            throw "AOT session-volume $($Run.Phase) did not prove a durable session-only change."
        }
        $requiredSteps += @(
            "session-recovery-intent-read-back",
            "product-session-probe-setter-succeeded",
            "probe-session-volume-verified",
            "probe-system-volume-unchanged")
    }

    if (-not $AllowAwaitingExternalRecovery.IsPresent) {
        if (-not [bool]$result.cleanupSucceeded -or
            [bool]$result.recoveryIntentPreserved) {
            throw "AOT session-volume $($Run.Phase) did not finish with verified recovery."
        }
        Assert-NativeMatchedSessionRead -Evidence $result.nativeFinal -Name "nativeFinal"
        if ($Run.Scenario -ne "ReadMatchedSession") {
            if ([Math]::Abs(
                    [double]$result.finalSystemVolume -
                    [double]$result.initialSystemVolume) -gt $systemVolumeTolerance) {
                throw "AOT session-volume $($Run.Phase) changed system master volume."
            }
            if ($Run.Scenario -eq "RecoverOriginal" -and
                -not [bool]$result.recoveryIntentFound) {
                $requiredSteps += "recovery-no-intent"
            }
            else {
                $requiredSteps += @(
                    "product-session-restore-setter-succeeded",
                    "recovery-original-session-verified",
                    "system-volume-unchanged",
                    "session-recovery-intent-cleared-after-verification")
            }
        }
    }

    if ($RequireRecoveryIntent.IsPresent -and
        (-not [bool]$result.recoveryIntentFound -or
            -not [bool]$result.recoveryIntentLoaded)) {
        throw "AOT session recovery did not load the forced-termination intent."
    }
    if ($RequireNoRecoveryIntent.IsPresent -and [bool]$result.recoveryIntentFound) {
        throw "AOT session-volume $($Run.Phase) unexpectedly found a stale recovery intent."
    }
    if ($null -ne $ExpectedOriginalSessionVolume -and
        [Math]::Abs(
            [double]$result.finalSessionVolume -
            [double]$ExpectedOriginalSessionVolume) -gt $sessionVolumeTolerance) {
        throw "AOT session-volume $($Run.Phase) did not restore the original session volume."
    }

    $missingSteps = @($requiredSteps | Where-Object { $_ -notin @($result.steps) })
    if ($missingSteps.Count -gt 0) {
        throw "AOT session-volume $($Run.Phase) is missing steps: $($missingSteps -join ', ')."
    }
}

function Get-RecoveryHint {
    $hint = "Original controlled session volume: unknown."
    if (Test-Path -LiteralPath $recoveryIntentPath -PathType Leaf) {
        try {
            $intent = Get-Content -LiteralPath $recoveryIntentPath -Raw |
                ConvertFrom-Json
            $hint = "Original controlled session volume: $([double]$intent.originalSessionVolume)."
        }
        catch {
            $hint = "Original controlled session volume: unreadable from preserved intent."
        }
    }

    return "$hint Recovery intent: '$recoveryIntentPath'."
}

if (-not (Test-Path -LiteralPath $launcher -PathType Leaf)) {
    throw "Native AOT preview launcher was not found at '$launcher'."
}
if (-not (Test-Path -LiteralPath $fixtureManifest -PathType Leaf)) {
    throw "Controlled Rust audio fixture manifest was not found at '$fixtureManifest'."
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
        "DeskBox-AotPreview\{0}-{1}-stage5b3c-music-volume-session-mutation" -f @(
            $worktreeName,
            $pathHash))
}
$DataRoot = [System.IO.Path]::GetFullPath($DataRoot)
$recoveryIntentPath = Join-Path $DataRoot (
    "aot-music-volume-session-mutation-smoke\session-recovery-intent.json")
$fixtureRoot = Join-Path $DataRoot (
    "aot-music-volume-session-mutation-smoke\fixture")
$fixtureWavePath = Join-Path $fixtureRoot "silent-loop.wav"
$fixtureReadyPath = Join-Path $fixtureRoot "ready.txt"
$fixtureStopPath = Join-Path $fixtureRoot "stop.txt"
$fixtureStdoutPath = Join-Path $evidenceRoot "fixture.stdout.log"
$fixtureStderrPath = Join-Path $evidenceRoot "fixture.stderr.log"
New-Item -ItemType Directory -Path $evidenceRoot -Force | Out-Null
New-Item -ItemType Directory -Path $fixtureRoot -Force | Out-Null

if (Test-Path -LiteralPath $recoveryIntentPath -PathType Leaf) {
    throw "A controlled-session recovery intent from an earlier interrupted run is still present. It is not treated as recovered because its audio session no longer has a proven live owner. $(Get-RecoveryHint)"
}

$existingFixtureProcesses = @(
    Get-CimInstance Win32_Process -Filter (
        "Name='deskbox-audio-session-fixture.exe'") -ErrorAction SilentlyContinue)
if ($existingFixtureProcesses.Count -ne 0) {
    throw "A controlled audio fixture is already running; refusing to touch it. PIDs=$($existingFixtureProcesses.ProcessId -join ',')."
}

$cargo = Get-Command cargo -ErrorAction Stop
& $cargo.Source `
    build `
    --manifest-path $fixtureManifest `
    --package deskbox-audio-session-fixture `
    --target $fixtureTargetTriple `
    --release `
    --locked
if ($LASTEXITCODE -ne 0) {
    throw "Controlled Rust audio fixture build failed with exit code $LASTEXITCODE."
}
if (-not (Test-Path -LiteralPath $fixtureExecutablePath -PathType Leaf)) {
    throw "Controlled Rust audio fixture was not produced at '$fixtureExecutablePath'."
}
$fixtureExecutableSha256 = (Get-FileHash -LiteralPath $fixtureExecutablePath -Algorithm SHA256).Hash

foreach ($marker in @($fixtureReadyPath, $fixtureStopPath, $fixtureStdoutPath, $fixtureStderrPath)) {
    if (Test-Path -LiteralPath $marker -PathType Leaf) {
        Remove-Item -LiteralPath $marker -Force
    }
}

$fixtureArguments = @(
    "--parent-pid",
    $PID.ToString(),
    "--wave",
    (ConvertTo-QuotedProcessArgument -Value $fixtureWavePath),
    "--ready",
    (ConvertTo-QuotedProcessArgument -Value $fixtureReadyPath),
    "--stop",
    (ConvertTo-QuotedProcessArgument -Value $fixtureStopPath))
$script:fixtureProcess = Start-Process `
    -FilePath $fixtureExecutablePath `
    -ArgumentList $fixtureArguments `
    -WindowStyle Hidden `
    -RedirectStandardOutput $fixtureStdoutPath `
    -RedirectStandardError $fixtureStderrPath `
    -PassThru

try {
    $fixtureReadyDeadline = [DateTime]::UtcNow.AddSeconds($FixtureReadySeconds)
    while ([DateTime]::UtcNow -lt $fixtureReadyDeadline -and
        -not (Test-Path -LiteralPath $fixtureReadyPath -PathType Leaf) -and
        -not $script:fixtureProcess.HasExited) {
        Start-Sleep -Milliseconds 100
    }
    if ($script:fixtureProcess.HasExited -or
        -not (Test-Path -LiteralPath $fixtureReadyPath -PathType Leaf)) {
        $fixtureError = if (Test-Path -LiteralPath $fixtureStderrPath -PathType Leaf) {
            Get-Content -LiteralPath $fixtureStderrPath -Raw
        }
        else {
            "No fixture error log was produced."
        }
        throw "Controlled silent audio fixture did not become ready. $fixtureError"
    }

    $exactFixture = Get-CimInstance Win32_Process -Filter (
        "ProcessId=$($script:fixtureProcess.Id)") -ErrorAction Stop
    if ($null -eq $exactFixture -or
        -not (Test-PathEqual `
            -Left $exactFixture.ExecutablePath `
            -Right $fixtureExecutablePath)) {
        throw "The ready audio fixture process does not match the built executable."
    }
    Start-Sleep -Milliseconds 500
}
catch {
    if ($null -ne $script:fixtureProcess -and -not $script:fixtureProcess.HasExited) {
        Set-Content -LiteralPath $fixtureStopPath -Value "stop" -Encoding Ascii
        Wait-Process -Id $script:fixtureProcess.Id -Timeout 2 -ErrorAction SilentlyContinue
        if (-not $script:fixtureProcess.HasExited) {
            Stop-ExactFixtureProcess `
                -ProcessId $script:fixtureProcess.Id `
                -ExecutablePath $fixtureExecutablePath
        }
    }
    throw
}

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
$fixtureCleanupError = $null
$originalSessionVolume = $null
$productionStateAfter = $null

try {
    try {
        $preflightRun = Invoke-MusicVolumeSessionMutationScenario `
            -Scenario "ReadMatchedSession" `
            -Phase "preflight"
        Assert-MusicVolumeSessionMutationResult `
            -Run $preflightRun `
            -RequireNoRecoveryIntent
        $originalSessionVolume = [double]$preflightRun.Result.finalSessionVolume

        $inProcessFailureRun = Invoke-MusicVolumeSessionMutationScenario `
            -Scenario "ChangeThenFail" `
            -Phase "in-process-failure"
        Assert-MusicVolumeSessionMutationResult `
            -Run $inProcessFailureRun `
            -AllowExpectedFailure `
            -ExpectedOriginalSessionVolume $originalSessionVolume

        $forcedTerminationRun = Invoke-MusicVolumeSessionMutationScenario `
            -Scenario "ChangeThenAwaitExternalRecovery" `
            -Phase "forced-termination"
        Assert-MusicVolumeSessionMutationResult `
            -Run $forcedTerminationRun `
            -AllowAwaitingExternalRecovery

        $recoveryRun = Invoke-MusicVolumeSessionMutationScenario `
            -Scenario "RecoverOriginal" `
            -Phase "recovery"
        Assert-MusicVolumeSessionMutationResult `
            -Run $recoveryRun `
            -RequireRecoveryIntent `
            -ExpectedOriginalSessionVolume $originalSessionVolume
        if ([Math]::Abs(
                [double]$recoveryRun.Result.recoveryObservedSessionVolume -
                [double]$forcedTerminationRun.Result.probeSessionVolume) -gt
            $sessionVolumeTolerance) {
            throw "Independent recovery did not observe the session probe left by forced termination."
        }

        $mainRun = Invoke-MusicVolumeSessionMutationScenario `
            -Scenario "ChangeRestore" `
            -Phase "main"
        Assert-MusicVolumeSessionMutationResult `
            -Run $mainRun `
            -ExpectedOriginalSessionVolume $originalSessionVolume
    }
    catch {
        $primaryError = $_
    }
}
finally {
    try {
        $postflightRun = Invoke-MusicVolumeSessionMutationScenario `
            -Scenario "RecoverOriginal" `
            -Phase "postflight"
        Assert-MusicVolumeSessionMutationResult -Run $postflightRun
        if ($null -ne $originalSessionVolume -and
            [Math]::Abs(
                [double]$postflightRun.Result.finalSessionVolume -
                [double]$originalSessionVolume) -gt $sessionVolumeTolerance) {
            throw "Postflight read differs from the saved original controlled session volume."
        }
        if ($null -ne $preflightRun -and
            [Math]::Abs(
                [double]$postflightRun.Result.finalSystemVolume -
                [double]$preflightRun.Result.initialSystemVolume) -gt
            $systemVolumeTolerance) {
            throw "System master volume changed across the session-only recovery matrix."
        }
    }
    catch {
        $postflightError = $_
    }

    if ($null -ne $script:productionBaseline) {
        try {
            $productionStateAfter = Get-DirectoryStateFingerprint `
                -Path $script:productionBaseline.root
            if (-not [string]::Equals(
                    [string]$productionStateAfter.fingerprint,
                    [string]$script:productionBaseline.fingerprint,
                    [StringComparison]::OrdinalIgnoreCase) -or
                [int]$productionStateAfter.fileCount -ne
                    [int]$script:productionBaseline.fileCount -or
                [long]$productionStateAfter.bytes -ne
                    [long]$script:productionBaseline.bytes) {
                throw "Production data changed during the AOT session-volume sequence."
            }
        }
        catch {
            $productionError = $_
        }
    }

    try {
        Set-Content -LiteralPath $fixtureStopPath -Value "stop" -Encoding Ascii
        Wait-Process -Id $script:fixtureProcess.Id -Timeout 5 -ErrorAction SilentlyContinue
        if (-not $script:fixtureProcess.HasExited) {
            Stop-ExactFixtureProcess `
                -ProcessId $script:fixtureProcess.Id `
                -ExecutablePath $fixtureExecutablePath
        }
        $remainingFixture = Get-CimInstance Win32_Process -Filter (
            "ProcessId=$($script:fixtureProcess.Id)") -ErrorAction SilentlyContinue
        if ($null -ne $remainingFixture) {
            throw "Controlled fixture PID $($script:fixtureProcess.Id) remained after cleanup."
        }
    }
    catch {
        $fixtureCleanupError = $_
    }
}

$remainingProcesses = @(Get-ExactPreviewProcesses `
    -ExecutablePath $script:expectedExecutablePath)
if ($remainingProcesses.Count -ne 0) {
    throw "Audited AOT process cleanup failed; PIDs=$($remainingProcesses.ProcessId -join ','). $(Get-RecoveryHint)"
}
if ($null -ne $fixtureCleanupError) {
    throw "Controlled fixture cleanup failed. $($fixtureCleanupError.Exception.Message)"
}
if ($null -ne $postflightError) {
    throw "AOT session-volume postflight recovery failed. $(Get-RecoveryHint) Error: $($postflightError.Exception.Message)"
}
if ($null -ne $productionError) {
    throw $productionError
}
if ($null -ne $primaryError) {
    throw "AOT session-volume sequence failed, but postflight recovery succeeded and the original controlled session volume was verified. Error: $($primaryError.Exception.Message)"
}

$sessionPath = Join-Path $evidenceRoot "session.json"
$session = [ordered]@{
    schemaVersion = 1
    generatedAtUtc = [DateTime]::UtcNow.ToString("O")
    summaryPath = $SummaryPath
    dataRoot = $DataRoot
    recoveryIntentPath = $recoveryIntentPath
    fixtureProcessId = [int]$script:fixtureProcess.Id
    fixtureProcessName = $controlledFixtureProcessName
    fixtureExecutablePath = $fixtureExecutablePath
    fixtureExecutableSha256 = $fixtureExecutableSha256
    fixtureWavePath = $fixtureWavePath
    fixtureWasSilent = $true
    sourceAppUserModelId = $controlledSourceAppUserModelId
    sourceDisplayName = $controlledSourceDisplayName
    expectedMatchKind = $expectedMatchKind
    executablePath = $mainRun.Result.executablePath
    executableSha256 = $mainRun.Result.executableSha256
    rustNativePath = $mainRun.Result.modulePath
    rustNativeSha256 = $mainRun.Result.moduleSha256
    nativeModuleHandle = $mainRun.Result.moduleHandle
    abiVersion = [uint32]$mainRun.Result.abiVersion
    capabilities = [uint64]$mainRun.Result.capabilities
    originalSessionVolume = [double]$originalSessionVolume
    systemVolume = [double]$preflightRun.Result.initialSystemVolume
    inProcessFailureProbeSessionVolume =
        [double]$inProcessFailureRun.Result.probeSessionVolume
    inProcessFailureFinalSessionVolume =
        [double]$inProcessFailureRun.Result.finalSessionVolume
    forcedTerminationProbeSessionVolume =
        [double]$forcedTerminationRun.Result.probeSessionVolume
    recoveryObservedSessionVolume =
        [double]$recoveryRun.Result.recoveryObservedSessionVolume
    recoveryFinalSessionVolume = [double]$recoveryRun.Result.finalSessionVolume
    mainProbeSessionVolume = [double]$mainRun.Result.probeSessionVolume
    mainFinalSessionVolume = [double]$mainRun.Result.finalSessionVolume
    postflightFinalSessionVolume = [double]$postflightRun.Result.finalSessionVolume
    postflightSystemVolume = [double]$postflightRun.Result.finalSystemVolume
    preflightResultPath = $preflightRun.EvidencePath
    inProcessFailureResultPath = $inProcessFailureRun.EvidencePath
    forcedTerminationResultPath = $forcedTerminationRun.EvidencePath
    recoveryResultPath = $recoveryRun.EvidencePath
    mainResultPath = $mainRun.EvidencePath
    postflightResultPath = $postflightRun.EvidencePath
    cleanupSucceeded = [bool]$postflightRun.Result.cleanupSucceeded
    recoveryIntentPreserved = Test-Path -LiteralPath $recoveryIntentPath -PathType Leaf
    fixtureCleanupSucceeded = $true
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
    FixtureProcessId = [int]$script:fixtureProcess.Id
    FixtureExecutable = $fixtureExecutablePath
    FixtureSha256 = $fixtureExecutableSha256
    MatchKind = $expectedMatchKind
    OriginalSessionVolume = [double]$originalSessionVolume
    InProcessFailureProbeSessionVolume =
        [double]$inProcessFailureRun.Result.probeSessionVolume
    InProcessFailureFinalSessionVolume =
        [double]$inProcessFailureRun.Result.finalSessionVolume
    ForcedTerminationProbeSessionVolume =
        [double]$forcedTerminationRun.Result.probeSessionVolume
    RecoveryObservedSessionVolume =
        [double]$recoveryRun.Result.recoveryObservedSessionVolume
    RecoveryFinalSessionVolume = [double]$recoveryRun.Result.finalSessionVolume
    MainProbeSessionVolume = [double]$mainRun.Result.probeSessionVolume
    FinalSessionVolume = [double]$postflightRun.Result.finalSessionVolume
    SystemVolume = [double]$postflightRun.Result.finalSystemVolume
    RecoveryIntentPreserved =
        Test-Path -LiteralPath $recoveryIntentPath -PathType Leaf
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
