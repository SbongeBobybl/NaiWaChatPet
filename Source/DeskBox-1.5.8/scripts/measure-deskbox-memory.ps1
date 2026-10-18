[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateRange(1, 2147483647)]
    [int]$ProcessId,

    [Parameter(Mandatory = $true)]
    [string]$ScenarioName,

    [string]$ExpectedExecutablePath,

    [string]$OutputDirectory,

    [ValidateRange(3, 3600)]
    [int]$SampleCount = 30,

    [ValidateRange(100, 60000)]
    [int]$IntervalMilliseconds = 1000,

    [ValidateRange(0, 60)]
    [int]$WarmupSeconds = 0,

    [switch]$IncludeGpuSnapshot
)

$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
if ([string]::IsNullOrWhiteSpace($ScenarioName)) {
    throw "A non-empty scenario name is required."
}
if ([string]::IsNullOrWhiteSpace($OutputDirectory)) {
    $OutputDirectory = Join-Path $repoRoot ".artifacts\memory-baseline"
}

$OutputDirectory = [System.IO.Path]::GetFullPath($OutputDirectory)
if (-not [string]::IsNullOrWhiteSpace($ExpectedExecutablePath)) {
    $ExpectedExecutablePath = [System.IO.Path]::GetFullPath($ExpectedExecutablePath)
}

if (-not ("DeskBox.MemoryMeasurement.NativeMethods" -as [type])) {
    Add-Type -TypeDefinition @"
using System;
using System.Collections.Generic;
using System.Runtime.InteropServices;
using System.Text;

namespace DeskBox.MemoryMeasurement
{
    public static class NativeMethods
    {
        public sealed class WindowInfo
        {
            public string Handle { get; set; } = "";
            public string OwnerHandle { get; set; } = "";
            public uint ThreadId { get; set; }
            public bool IsVisible { get; set; }
            public string ClassName { get; set; } = "";
            public string Title { get; set; } = "";
            public int Width { get; set; }
            public int Height { get; set; }
        }

        [StructLayout(LayoutKind.Sequential)]
        private struct Rect
        {
            public int Left;
            public int Top;
            public int Right;
            public int Bottom;
        }

        private delegate bool EnumWindowsProc(IntPtr hwnd, IntPtr lParam);

        [DllImport("user32.dll")]
        private static extern bool EnumWindows(EnumWindowsProc callback, IntPtr lParam);

        [DllImport("user32.dll")]
        private static extern uint GetWindowThreadProcessId(IntPtr hwnd, out uint processId);

        [DllImport("user32.dll")]
        private static extern bool IsWindowVisible(IntPtr hwnd);

        [DllImport("user32.dll", CharSet = CharSet.Unicode)]
        private static extern int GetWindowText(IntPtr hwnd, StringBuilder text, int maxCount);

        [DllImport("user32.dll", CharSet = CharSet.Unicode)]
        private static extern int GetClassName(IntPtr hwnd, StringBuilder className, int maxCount);

        [DllImport("user32.dll")]
        private static extern IntPtr GetWindow(IntPtr hwnd, uint command);

        [DllImport("user32.dll")]
        private static extern bool GetWindowRect(IntPtr hwnd, out Rect rect);

        [DllImport("user32.dll")]
        public static extern uint GetGuiResources(IntPtr process, uint flags);

        public static int CountTopLevelWindows(uint targetProcessId, bool visibleOnly)
        {
            int count = 0;
            EnumWindows((hwnd, _) =>
            {
                GetWindowThreadProcessId(hwnd, out uint ownerProcessId);
                if (ownerProcessId == targetProcessId &&
                    (!visibleOnly || IsWindowVisible(hwnd)))
                {
                    count++;
                }
                return true;
            }, IntPtr.Zero);
            return count;
        }

        public static WindowInfo[] CaptureTopLevelWindows(uint targetProcessId)
        {
            const uint GetOwner = 4;
            var windows = new List<WindowInfo>();
            EnumWindows((hwnd, _) =>
            {
                uint threadId = GetWindowThreadProcessId(hwnd, out uint ownerProcessId);
                if (ownerProcessId != targetProcessId)
                {
                    return true;
                }

                var className = new StringBuilder(256);
                var title = new StringBuilder(512);
                GetClassName(hwnd, className, className.Capacity);
                GetWindowText(hwnd, title, title.Capacity);
                GetWindowRect(hwnd, out Rect rect);
                windows.Add(new WindowInfo
                {
                    Handle = $"0x{hwnd.ToInt64():X}",
                    OwnerHandle = $"0x{GetWindow(hwnd, GetOwner).ToInt64():X}",
                    ThreadId = threadId,
                    IsVisible = IsWindowVisible(hwnd),
                    ClassName = className.ToString(),
                    Title = title.ToString(),
                    Width = Math.Max(0, rect.Right - rect.Left),
                    Height = Math.Max(0, rect.Bottom - rect.Top)
                });
                return true;
            }, IntPtr.Zero);
            return windows.ToArray();
        }
    }
}
"@
}

function Get-Percentile([double[]]$Values, [double]$Percentile) {
    if ($Values.Count -eq 0) {
        return $null
    }

    $ordered = @($Values | Sort-Object)
    $index = [Math]::Ceiling($Percentile * $ordered.Count) - 1
    $index = [Math]::Max(0, [Math]::Min($ordered.Count - 1, $index))
    return [double]$ordered[$index]
}

function Get-Distribution([double[]]$Values) {
    if ($Values.Count -eq 0) {
        return $null
    }

    [PSCustomObject]@{
        Minimum = [double](($Values | Measure-Object -Minimum).Minimum)
        Median = Get-Percentile $Values 0.50
        P95 = Get-Percentile $Values 0.95
        Maximum = [double](($Values | Measure-Object -Maximum).Maximum)
    }
}

function Get-GpuSnapshot([int]$TargetProcessId) {
    try {
        $counterSet = Get-Counter -ListSet "GPU Process Memory" -ErrorAction Stop
        $paths = @($counterSet.PathsWithInstances | Where-Object {
            $_ -like "*pid_$TargetProcessId`_*"
        })
        if ($paths.Count -eq 0) {
            return [PSCustomObject]@{
                Available = $false
                Reason = "No GPU Process Memory instances were found for PID $TargetProcessId."
            }
        }

        $samples = @((Get-Counter -Counter $paths -ErrorAction Stop).CounterSamples)
        function Sum-Counter([string]$Name) {
            [double](($samples |
                Where-Object { $_.Path -match ("\\" + [regex]::Escape($Name) + "$") } |
                Measure-Object -Property CookedValue -Sum).Sum)
        }

        return [PSCustomObject]@{
            Available = $true
            DedicatedUsageBytes = Sum-Counter "dedicated usage"
            SharedUsageBytes = Sum-Counter "shared usage"
            LocalUsageBytes = Sum-Counter "local usage"
            NonLocalUsageBytes = Sum-Counter "non local usage"
            TotalCommittedBytes = Sum-Counter "total committed"
            InstanceCount = @($samples | Select-Object -ExpandProperty InstanceName -Unique).Count
        }
    }
    catch {
        return [PSCustomObject]@{
            Available = $false
            Reason = $_.Exception.Message
        }
    }
}

$process = Get-Process -Id $ProcessId -ErrorAction Stop
$process.Refresh()
$executablePath = $process.MainModule.FileName
if ([string]::IsNullOrWhiteSpace($executablePath)) {
    throw "The executable path for PID $ProcessId could not be resolved."
}
$executablePath = [System.IO.Path]::GetFullPath($executablePath)

if (-not [string]::IsNullOrWhiteSpace($ExpectedExecutablePath) -and
    -not [string]::Equals(
        $executablePath,
        $ExpectedExecutablePath,
        [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "PID $ProcessId is running '$executablePath', not the expected '$ExpectedExecutablePath'."
}

$executable = Get-Item -LiteralPath $executablePath
$version = $executable.VersionInfo
$gitCommit = (& git -C $repoRoot rev-parse HEAD).Trim()
$gitBranch = (& git -C $repoRoot branch --show-current).Trim()
$gitStatus = @(& git -C $repoRoot status --porcelain=v1)

if ($WarmupSeconds -gt 0) {
    Start-Sleep -Seconds $WarmupSeconds
}

$gpuBefore = if ($IncludeGpuSnapshot) {
    Get-GpuSnapshot $ProcessId
}
else {
    $null
}

$measurements = [System.Collections.Generic.List[object]]::new()
$previousCpu = $process.TotalProcessorTime
$previousTimestamp = [DateTimeOffset]::UtcNow

for ($sampleIndex = 0; $sampleIndex -lt $SampleCount; $sampleIndex++) {
    $process.Refresh()
    if ($process.HasExited) {
        throw "DeskBox PID $ProcessId exited during sample $sampleIndex."
    }

    $timestamp = [DateTimeOffset]::UtcNow
    $cpu = $process.TotalProcessorTime
    $elapsedMilliseconds = ($timestamp - $previousTimestamp).TotalMilliseconds
    $cpuDeltaMilliseconds = ($cpu - $previousCpu).TotalMilliseconds
    $normalizedCpuPercent = if ($elapsedMilliseconds -gt 0) {
        ($cpuDeltaMilliseconds / $elapsedMilliseconds / [Environment]::ProcessorCount) * 100.0
    }
    else {
        0.0
    }

    $moduleNames = @($process.Modules | ForEach-Object ModuleName)
    $measurements.Add([PSCustomObject]@{
        Index = $sampleIndex
        TimestampUtc = $timestamp.ToString("O")
        WorkingSetBytes = $process.WorkingSet64
        PrivateBytes = $process.PrivateMemorySize64
        HandleCount = $process.HandleCount
        ThreadCount = $process.Threads.Count
        GdiObjectCount = [DeskBox.MemoryMeasurement.NativeMethods]::GetGuiResources($process.Handle, 0)
        UserObjectCount = [DeskBox.MemoryMeasurement.NativeMethods]::GetGuiResources($process.Handle, 1)
        TopLevelWindowCount = [DeskBox.MemoryMeasurement.NativeMethods]::CountTopLevelWindows([uint32]$ProcessId, $false)
        VisibleTopLevelWindowCount = [DeskBox.MemoryMeasurement.NativeMethods]::CountTopLevelWindows([uint32]$ProcessId, $true)
        NormalizedCpuPercent = $normalizedCpuPercent
        DeskBoxNativeModuleLoaded = $moduleNames -contains "deskbox_native.dll"
    })

    $previousCpu = $cpu
    $previousTimestamp = $timestamp
    if ($sampleIndex + 1 -lt $SampleCount) {
        Start-Sleep -Milliseconds $IntervalMilliseconds
    }
}

$gpuAfter = if ($IncludeGpuSnapshot) {
    Get-GpuSnapshot $ProcessId
}
else {
    $null
}

$workingSetDistribution = Get-Distribution @($measurements | ForEach-Object { [double]$_.WorkingSetBytes })
$privateDistribution = Get-Distribution @($measurements | ForEach-Object { [double]$_.PrivateBytes })
$handleDistribution = Get-Distribution @($measurements | ForEach-Object { [double]$_.HandleCount })
$threadDistribution = Get-Distribution @($measurements | ForEach-Object { [double]$_.ThreadCount })
$cpuDistribution = Get-Distribution @($measurements | ForEach-Object { [double]$_.NormalizedCpuPercent })

$summary = [PSCustomObject]@{
    SchemaVersion = 2
    GeneratedAtUtc = [DateTimeOffset]::UtcNow.ToString("O")
    ScenarioName = $ScenarioName
    Process = [PSCustomObject]@{
        Id = $ProcessId
        ExecutablePath = $executablePath
        ExecutableBytes = $executable.Length
        ExecutableSha256 = (Get-FileHash -LiteralPath $executablePath -Algorithm SHA256).Hash
        FileVersion = $version.FileVersion
        ProductVersion = $version.ProductVersion
        StartTime = $process.StartTime.ToUniversalTime().ToString("O")
    }
    Repository = [PSCustomObject]@{
        Root = $repoRoot
        Branch = $gitBranch
        Commit = $gitCommit
        StatusEntries = $gitStatus
    }
    Sampling = [PSCustomObject]@{
        SampleCount = $SampleCount
        IntervalMilliseconds = $IntervalMilliseconds
        WarmupSeconds = $WarmupSeconds
        LogicalProcessorCount = [Environment]::ProcessorCount
    }
    Distributions = [PSCustomObject]@{
        WorkingSetBytes = $workingSetDistribution
        PrivateBytes = $privateDistribution
        HandleCount = $handleDistribution
        ThreadCount = $threadDistribution
        NormalizedCpuPercent = $cpuDistribution
    }
    GpuBefore = $gpuBefore
    GpuAfter = $gpuAfter
    WindowInventory = @([DeskBox.MemoryMeasurement.NativeMethods]::CaptureTopLevelWindows([uint32]$ProcessId))
    Measurements = @($measurements)
}

New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
$safeScenario = [regex]::Replace($ScenarioName.Trim(), "[^A-Za-z0-9._-]+", "-").Trim("-")
if ([string]::IsNullOrWhiteSpace($safeScenario)) {
    $safeScenario = "scenario"
}
$outputPath = Join-Path $OutputDirectory (
    "{0}-{1}.json" -f [DateTime]::UtcNow.ToString("yyyyMMddTHHmmssZ"), $safeScenario)
$summary | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $outputPath -Encoding utf8

[PSCustomObject]@{
    OutputPath = $outputPath
    ScenarioName = $ScenarioName
    ProcessId = $ProcessId
    WorkingSetMedianMiB = [Math]::Round($workingSetDistribution.Median / 1MB, 2)
    PrivateBytesMedianMiB = [Math]::Round($privateDistribution.Median / 1MB, 2)
    HandlesMedian = [Math]::Round($handleDistribution.Median, 0)
    ThreadsMedian = [Math]::Round($threadDistribution.Median, 0)
    CpuMedianPercent = [Math]::Round($cpuDistribution.Median, 4)
    GpuTotalCommittedAfterMiB = if ($gpuAfter.Available) {
        [Math]::Round($gpuAfter.TotalCommittedBytes / 1MB, 2)
    }
    else {
        $null
    }
} | Format-List
