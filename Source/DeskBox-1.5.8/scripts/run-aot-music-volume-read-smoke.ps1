[CmdletBinding()]
param(
    [string]$SummaryPath,

    [string]$DataRoot,

    [ValidateRange(10, 600)]
    [int]$TimeoutSeconds = 120,

    [switch]$KeepRunning
)

$ErrorActionPreference = "Stop"
$scenario = "SystemAndSnapshotReadOnly"
$scenarioDirectoryName = "system-and-snapshot-read-only"
$musicReadSmokeEnvironmentVariable = "DESKBOX_AOT_MUSIC_VOLUME_READ_SMOKE"
$musicMutationSmokeEnvironmentVariable = "DESKBOX_AOT_MUSIC_VOLUME_MUTATION_SMOKE"
$musicSessionMutationSmokeEnvironmentVariable =
    "DESKBOX_AOT_MUSIC_VOLUME_SESSION_MUTATION_SMOKE"
$shortcutSmokeEnvironmentVariable = "DESKBOX_AOT_SHORTCUT_SMOKE"
$shellSmokeEnvironmentVariable = "DESKBOX_AOT_SHELL_SMOKE"
$mutationSmokeEnvironmentVariable = "DESKBOX_AOT_QUICK_ACCESS_MUTATION_SMOKE"
$managedUiSmokeEnvironmentVariable = "DESKBOX_AOT_MANAGED_UI_SMOKE"
$musicBackendEnvironmentVariable = "DESKBOX_MUSIC_VOLUME_BACKEND"
$systemVolumeTolerance = 0.005
$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot ".."))
$launcher = Join-Path $PSScriptRoot "start-aot-preview.ps1"
$previewSessionPath = Join-Path $repoRoot ".artifacts\aot-preview\win-x64\session.json"
$evidenceRoot = Join-Path $repoRoot ".artifacts\aot-music-volume-read-smoke\win-x64"

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

    if (-not [bool]$Evidence.success -or
        [uint32]$Evidence.status -ne 0 -or
        [int]$Evidence.operationHResult -lt 0 -or
        [int]$Evidence.deviceHResult -lt 0 -or
        [int]$Evidence.systemHResult -lt 0 -or
        (([uint32]$Evidence.attemptedPhases -band 0x0F) -ne 0x0F) -or
        -not (Test-NormalizedVolume -Value ([double]$Evidence.systemVolume))) {
        throw "$Name does not prove a successful real Rust endpoint/system read."
    }
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
        "DeskBox-AotPreview\{0}-{1}-stage5b3a-{2}" -f @(
            $worktreeName,
            $pathHash,
            $scenarioDirectoryName))
}
$DataRoot = [System.IO.Path]::GetFullPath($DataRoot)
$resultPath = Join-Path $DataRoot (
    "aot-music-volume-read-smoke\$scenarioDirectoryName\result.json")

$previewSession = $null
try {
    $previousMusicReadSmoke = [Environment]::GetEnvironmentVariable(
        $musicReadSmokeEnvironmentVariable,
        "Process")
    $previousMusicMutationSmoke = [Environment]::GetEnvironmentVariable(
        $musicMutationSmokeEnvironmentVariable,
        "Process")
    $previousMusicSessionMutationSmoke = [Environment]::GetEnvironmentVariable(
        $musicSessionMutationSmokeEnvironmentVariable,
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
            $musicReadSmokeEnvironmentVariable,
            $scenario,
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
    $previewSession = Get-Content -LiteralPath $previewSessionPath -Raw | ConvertFrom-Json
    if (-not (Test-PathEqual -Left $previewSession.previewDataRoot -Right $DataRoot)) {
        throw "Preview session data root does not match the music-volume read smoke root."
    }

    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    $smokeResult = $null
    while ([DateTime]::UtcNow -lt $deadline) {
        if (Test-Path -LiteralPath $resultPath -PathType Leaf) {
            try {
                $candidate = Get-Content -LiteralPath $resultPath -Raw | ConvertFrom-Json
                $smokeResult = $candidate
                if ($candidate.state -eq "Completed" -or $candidate.state -eq "Failed") {
                    break
                }
            }
            catch {
                # Retry a bounded transient sharing race during the atomic JSON replace.
            }
        }

        Start-Sleep -Milliseconds 250
    }

    if ($null -eq $smokeResult -or
        ($smokeResult.state -ne "Completed" -and $smokeResult.state -ne "Failed")) {
        throw "AOT music-volume read smoke timed out after $TimeoutSeconds seconds. Last state='$($smokeResult.state)'; result='$resultPath'."
    }
    if ($smokeResult.state -ne "Completed" -or -not [bool]$smokeResult.success) {
        throw "AOT music-volume read smoke failed: $($smokeResult.error)"
    }
    if ([int]$smokeResult.processId -ne [int]$previewSession.primaryProcessId) {
        throw "AOT music-volume result PID does not match the current preview primary PID."
    }
    if (-not (Test-PathEqual -Left $smokeResult.executablePath -Right $previewSession.executablePath)) {
        throw "AOT music-volume executable path does not match the audited preview session."
    }
    if (-not [string]::Equals(
            [string]$smokeResult.executableSha256,
            [string]$previewSession.executableSha256,
            [StringComparison]::OrdinalIgnoreCase)) {
        throw "AOT music-volume executableSha256 does not match the audited preview session."
    }
    if (-not (Test-PathEqual -Left $smokeResult.modulePath -Right $previewSession.rustNativePath)) {
        throw "AOT music-volume ModulePath does not match the audited Rust module path."
    }
    if (-not [string]::Equals(
            [string]$smokeResult.moduleSha256,
            [string]$previewSession.rustNativeSha256,
            [StringComparison]::OrdinalIgnoreCase)) {
        throw "AOT music-volume ModuleSha256 does not match rustNativeSha256 from the audited preview session."
    }
    if ([bool]$smokeResult.isDynamicCodeSupported -or
        $smokeResult.selectedBackend -ne "Rust" -or
        $smokeResult.loadState -ne "Loaded" -or
        [uint32]$smokeResult.abiVersion -ne 2 -or
        (([uint64]$smokeResult.capabilities -band 0x20) -ne 0x20) -or
        [string]::IsNullOrWhiteSpace([string]$smokeResult.moduleHandle) -or
        $smokeResult.moduleHandle -eq "0x0") {
        throw "AOT music-volume smoke did not prove the loaded Rust read boundary."
    }
    if ($smokeResult.scenario -ne $scenario -or
        -not (Test-PathEqual -Left $smokeResult.previewDataRoot -Right $DataRoot)) {
        throw "AOT music-volume structured evidence does not match the requested scenario/root."
    }

    Assert-NativeSystemRead -Evidence $smokeResult.nativeSystemBefore -Name "nativeSystemBefore"
    Assert-NativeSystemRead -Evidence $smokeResult.nativeSystemAfter -Name "nativeSystemAfter"
    $nativeSnapshot = $smokeResult.nativeSnapshot
    if (-not [bool]$nativeSnapshot.success -or
        [uint32]$nativeSnapshot.status -ne 0 -or
        [int]$nativeSnapshot.operationHResult -lt 0 -or
        [int]$nativeSnapshot.deviceHResult -lt 0 -or
        [int]$nativeSnapshot.systemHResult -lt 0 -or
        [int]$nativeSnapshot.sessionHResult -lt 0 -or
        (([uint32]$nativeSnapshot.attemptedPhases -band 0x1F) -ne 0x1F) -or
        -not (Test-NormalizedVolume -Value ([double]$nativeSnapshot.systemVolume)) -or
        -not (Test-NormalizedVolume -Value ([double]$nativeSnapshot.sessionVolume))) {
        throw "nativeSnapshot does not prove a successful real Rust system/session snapshot read."
    }
    if ([bool]$nativeSnapshot.hasSessionVolume) {
        if ([uint32]$nativeSnapshot.matchKind -lt 1 -or
            [uint32]$nativeSnapshot.matchKind -gt 7 -or
            (([uint32]$nativeSnapshot.attemptedPhases -band 0x20) -eq 0)) {
            throw "Matched session evidence has an invalid match kind or missing session-volume phase."
        }
    }
    elseif ([uint32]$nativeSnapshot.matchKind -ne 0 -or
        (([uint32]$nativeSnapshot.attemptedPhases -band 0x20) -ne 0)) {
        throw "No-session evidence unexpectedly reports a match kind or session-volume phase."
    }

    $systemReadings = @(
        [double]$smokeResult.productSystemVolume,
        [double]$smokeResult.productSnapshotSystemVolume,
        [double]$nativeSnapshot.systemVolume,
        [double]$smokeResult.nativeSystemVolumeAfter)
    if (-not (Test-NormalizedVolume -Value ([double]$smokeResult.nativeSystemVolumeBefore)) -or
        @($systemReadings | Where-Object {
            [Math]::Abs($_ - [double]$smokeResult.nativeSystemVolumeBefore) -gt $systemVolumeTolerance
        }).Count -gt 0) {
        throw "System volume changed or the product/native read values diverged beyond tolerance."
    }
    if ([bool]$smokeResult.productSnapshotHasSessionVolume -ne
        [bool]$nativeSnapshot.hasSessionVolume) {
        throw "Product and native snapshot evidence disagree about session presence."
    }
    if (-not (Test-NormalizedVolume -Value ([double]$smokeResult.productSnapshotSessionVolume))) {
        throw "Product session volume is not finite and normalized."
    }
    if ([bool]$nativeSnapshot.hasSessionVolume -and
        [Math]::Abs(
            [double]$smokeResult.productSnapshotSessionVolume -
            [double]$nativeSnapshot.sessionVolume) -gt $systemVolumeTolerance) {
        throw "Product and native session volume differ beyond tolerance."
    }

    $requiredSteps = @(
        "default-audio-endpoint",
        "native-system-before-hresults",
        "native-system-before-phases",
        "native-system-before-volume",
        "product-system-volume",
        "product-snapshot-system-volume",
        "product-snapshot-session-volume",
        "native-snapshot-success",
        "native-snapshot-hresults",
        "native-snapshot-phases",
        "native-snapshot-system-volume",
        "native-snapshot-session-volume",
        "native-snapshot-session-shape",
        "product-native-session-presence",
        "native-system-after-success",
        "native-system-after-hresults",
        "native-system-after-phases",
        "native-system-after-volume",
        "system-volume-unchanged",
        "runtime-native-aot",
        "music-volume-backend-rust",
        "module-loaded",
        "module-handle",
        "module-abi",
        "module-music-volume-capability")
    $missingSteps = @($requiredSteps | Where-Object { $_ -notin @($smokeResult.steps) })
    if ($missingSteps.Count -gt 0) {
        throw "AOT music-volume read result is missing required steps: $($missingSteps -join ', ')."
    }

    $productionStateAfter = Get-DirectoryStateFingerprint -Path $previewSession.productionDataRoot
    if (-not [string]::Equals(
            [string]$productionStateAfter.fingerprint,
            [string]$previewSession.productionDataFingerprintBefore,
            [StringComparison]::OrdinalIgnoreCase) -or
        [int]$productionStateAfter.fileCount -ne [int]$previewSession.productionDataFileCountBefore -or
        [long]$productionStateAfter.bytes -ne [long]$previewSession.productionDataBytesBefore) {
        throw "Production data changed during the AOT music-volume read smoke."
    }

    New-Item -ItemType Directory -Path $evidenceRoot -Force | Out-Null
    $sessionPath = Join-Path $evidenceRoot "session.json"
    $session = [ordered]@{
        schemaVersion = 1
        generatedAtUtc = [DateTime]::UtcNow.ToString("O")
        scenario = $scenario
        resultPath = $resultPath
        previewSessionPath = $previewSessionPath
        executablePath = $smokeResult.executablePath
        executableSha256 = $smokeResult.executableSha256
        rustNativePath = $smokeResult.modulePath
        rustNativeSha256 = $smokeResult.moduleSha256
        nativeModuleHandle = $smokeResult.moduleHandle
        abiVersion = [uint32]$smokeResult.abiVersion
        capabilities = [uint64]$smokeResult.capabilities
        selectedBackend = $smokeResult.selectedBackend
        productSystemVolume = [double]$smokeResult.productSystemVolume
        productSnapshotSystemVolume = [double]$smokeResult.productSnapshotSystemVolume
        productSnapshotSessionVolume = [double]$smokeResult.productSnapshotSessionVolume
        productSnapshotHasSessionVolume = [bool]$smokeResult.productSnapshotHasSessionVolume
        sessionMatchObserved = [bool]$smokeResult.sessionMatchObserved
        nativeSystemVolumeBefore = [double]$smokeResult.nativeSystemVolumeBefore
        nativeSystemVolumeAfter = [double]$smokeResult.nativeSystemVolumeAfter
        nativeSnapshotStatus = [uint32]$nativeSnapshot.status
        nativeSnapshotOperationHResult = [int]$nativeSnapshot.operationHResult
        nativeSnapshotAttemptedPhases = [uint32]$nativeSnapshot.attemptedPhases
        nativeSnapshotMatchKind = [uint32]$nativeSnapshot.matchKind
        nativeSnapshotSessionHResult = [int]$nativeSnapshot.sessionHResult
        steps = @($smokeResult.steps)
        previewDataRoot = $DataRoot
        productionDataRoot = $previewSession.productionDataRoot
        productionDataFingerprintBefore = $previewSession.productionDataFingerprintBefore
        productionDataFingerprintAfter = $productionStateAfter.fingerprint
        productionDataFileCountAfter = $productionStateAfter.fileCount
        productionDataBytesAfter = $productionStateAfter.bytes
        processId = [int]$smokeResult.processId
    }
    $session | ConvertTo-Json -Depth 6 | Set-Content -LiteralPath $sessionPath -Encoding UTF8

    [PSCustomObject]@{
        Scenario = $scenario
        Success = $true
        ProcessId = [int]$smokeResult.processId
        Exe = $smokeResult.executablePath
        ExeSha256 = $smokeResult.executableSha256
        RustNativeDll = $smokeResult.modulePath
        RustNativeSha256 = $smokeResult.moduleSha256
        AbiVersion = [uint32]$smokeResult.abiVersion
        Capabilities = [uint64]$smokeResult.capabilities
        ProductSystemVolume = [double]$smokeResult.productSystemVolume
        NativeSystemVolumeBefore = [double]$smokeResult.nativeSystemVolumeBefore
        NativeSystemVolumeAfter = [double]$smokeResult.nativeSystemVolumeAfter
        SessionMatchObserved = [bool]$smokeResult.sessionMatchObserved
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
