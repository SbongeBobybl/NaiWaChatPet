# Scenario memory measurement harness for DeskBox.
#
# Drives a repeated UI scenario while sampling process memory, then records a
# baseline summary. See docs/memory-optimization-plan.md (Phase 0 / P0.5).
#
# Usage examples:
#   .\measure-scenario-memory.ps1 -Scenario stack-toggle -Cycles 20
#   .\measure-scenario-memory.ps1 -Scenario appearance-switch -Manual
#   .\measure-scenario-memory.ps1 -Scenario group-scroll -Cycles 20 -WithDotnetCounters
#
# Degradation rule (user requirement): if real-UI driving fails more than
# -FailuresBeforeDegrade consecutive rounds, stop driving entirely and fall
# back to sampling only (operator may keep acting manually); the summary is
# marked degraded=true.

[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('stack-toggle', 'capsule-toggle', 'group-scroll', 'appearance-switch')]
    [string]$Scenario,

    [int]$Cycles = 20,
    [int]$IdleSeconds = 60,
    [int]$SampleIntervalSeconds = 2,
    [int]$FailuresBeforeDegrade = 5,
    [int]$ActionDelayMs = 1200,
    [int]$WarmupSeconds = 20,
    [int]$ManualSeconds = 240,
    [switch]$Manual,
    [switch]$WithDotnetCounters,
    [string]$ConfigFile = "$PSScriptRoot\scenario-coords.json",
    [string]$OutputRoot = "$PSScriptRoot\..\artifacts\memory-scenarios"
)

$ErrorActionPreference = 'Stop'
# Param-default evaluation happens before $PSScriptRoot is populated on some
# PowerShell 5.1 hosts, so re-resolve script-relative defaults in the body.
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
if (-not (Test-Path $ConfigFile)) { $ConfigFile = Join-Path $scriptDir 'scenario-coords.json' }
if (-not (Test-Path $OutputRoot)) { $OutputRoot = Join-Path (Split-Path $scriptDir -Parent) 'artifacts\memory-scenarios' }
$repoRoot = Split-Path $scriptDir -Parent

Add-Type -TypeDefinition @"
using System;
using System.Runtime.InteropServices;
public static class NativeInput {
    [DllImport("user32.dll")] public static extern void mouse_event(uint flags, uint dx, uint dy, uint data, UIntPtr extra);
    [DllImport("user32.dll")] public static extern void keybd_event(byte vk, byte scan, uint flags, UIntPtr extra);
}
"@
Add-Type -AssemblyName System.Windows.Forms
$ScreenW = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds.Width
$ScreenH = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds.Height

function Move-Abs([int]$x, [int]$y) {
    $dx = [int](($x * 65535) / ($ScreenW - 1))
    $dy = [int](($y * 65535) / ($ScreenH - 1))
    [NativeInput]::mouse_event(0x8001, [uint32]$dx, [uint32]$dy, 0, [UIntPtr]::Zero)
}

function Invoke-Click([int]$x, [int]$y) {
    Move-Abs $x $y
    Start-Sleep -Milliseconds 60
    [NativeInput]::mouse_event(2, 0, 0, 0, [UIntPtr]::Zero)
    Start-Sleep -Milliseconds 40
    [NativeInput]::mouse_event(4, 0, 0, 0, [UIntPtr]::Zero)
}

function Invoke-Wheel([int]$x, [int]$y, [int]$delta) {
    Move-Abs $x $y
    Start-Sleep -Milliseconds 60
    [NativeInput]::mouse_event(0x800, 0, 0, [uint32]$delta, [UIntPtr]::Zero)
}

function Get-DeskBox {
    $candidates = Get-Process -Name DeskBox -ErrorAction SilentlyContinue
    if (-not $candidates) { return $null }
    $mine = $candidates | Where-Object { $_.Path -like "*\project\wingezi*" } | Select-Object -First 1
    if ($mine) { return $mine }
    return ($candidates | Select-Object -First 1)
}

if (-not (Test-Path $ConfigFile)) {
    throw "Coordinate config not found: $ConfigFile — fill scenario-coords.json first."
}
$coords = Get-Content $ConfigFile -Raw | ConvertFrom-Json

$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$outDir = Join-Path $OutputRoot "$Scenario-$stamp"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$csvPath = Join-Path $outDir 'samples.csv'
"timestamp,phase,ws_mb,private_mb,handles,threads" | Out-File $csvPath -Encoding utf8

$proc = Get-DeskBox
if (-not $proc) { throw "DeskBox is not running. Start it from the canonical Debug output first." }
$procId = $proc.Id
Write-Host "Target: DeskBox PID $procId ($($proc.Path))"

$dotnetCounters = $null
if ($WithDotnetCounters) {
    if (Get-Command dotnet-counters -ErrorAction SilentlyContinue) {
        $ccPath = Join-Path $outDir 'dotnet-counters.csv'
        $dotnetCounters = Start-Process dotnet-counters -ArgumentList @(
            'collect', '-p', "$procId", '--refresh-interval', '2',
            '--counters', 'System.Runtime', '--format', 'csv', '-o', $ccPath
        ) -PassThru -WindowStyle Hidden
        Write-Host "dotnet-counters -> $ccPath"
    } else {
        Write-Warning "dotnet-counters not installed; skipping managed/native split (P0.5 degraded)."
    }
}

$script:failures = 0
$script:degraded = $false

function Sample([string]$phase) {
    $p = Get-Process -Id $procId -ErrorAction SilentlyContinue
    if (-not $p) { return $false }
    "{0},{1},{2:N1},{3:N1},{4},{5}" -f (Get-Date -Format 'HH:mm:ss'), $phase,
        ($p.WorkingSet64 / 1MB), ($p.PrivateMemorySize64 / 1MB),
        $p.HandleCount, $p.Threads.Count | Out-File $csvPath -Append -Encoding utf8
    return $true
}

function Invoke-DriverOnce([int]$cycle) {
    # Throws on any driver problem; caller counts consecutive failures.
    switch ($Scenario) {
        'stack-toggle' {
            $c = $coords.'stack-toggle'
            Invoke-Click $c.open[0] $c.open[1]
            Start-Sleep -Milliseconds $ActionDelayMs
            if (-not (Sample "cycle-$cycle-open")) { throw "process exited" }
            Invoke-Click $c.dismiss[0] $c.dismiss[1]
        }
        'capsule-toggle' {
            # Compact bars on this desktop are click-expand (widgetCollapseBehavior=Expanded):
            # click the bar to expand, click the expanded widget's collapse chevron to fold back.
            $c = $coords.'capsule-toggle'
            Invoke-Click $c.expand[0] $c.expand[1]
            Start-Sleep -Milliseconds ([Math]::Max($ActionDelayMs, 1500))
            if (-not (Sample "cycle-$cycle-open")) { throw "process exited" }
            Invoke-Click $c.collapse[0] $c.collapse[1]
        }
        'group-scroll' {
            $c = $coords.'group-scroll'
            for ($t = 0; $t -lt [Math]::Max(1, $c.ticks); $t++) {
                Invoke-Wheel $c.wheel[0] $c.wheel[1] $c.delta
                Start-Sleep -Milliseconds 250
            }
        }
        'appearance-switch' {
            if (-not $Manual) {
                throw "appearance-switch requires -Manual (settings navigation is not automated)."
            }
        }
    }
}

Write-Host "Phase: warmup ${WarmupSeconds}s (settle after startup)..."
$warmupEnd = (Get-Date).AddSeconds($WarmupSeconds)
while ((Get-Date) -lt $warmupEnd) {
    if (-not (Sample 'warmup')) { throw "DeskBox exited during warmup." }
    Start-Sleep -Seconds $SampleIntervalSeconds
}

if ($Manual -or $Scenario -eq 'appearance-switch') {
    Write-Host ""
    Write-Host "=== MANUAL MODE ===" -ForegroundColor Yellow
    Write-Host "Sampling for $ManualSeconds seconds. Perform the '$Scenario' action $Cycles times now."
    Write-Host "(Settings > 外观: toggle 材质/主题色/语言 back and forth.)"
    $manualEnd = (Get-Date).AddSeconds($ManualSeconds)
    while ((Get-Date) -lt $manualEnd) {
        if (-not (Sample 'manual-cycle')) { break }
        Start-Sleep -Seconds $SampleIntervalSeconds
    }
} else {
    Write-Host "Phase: driving $Scenario x $Cycles (interval ${ActionDelayMs}ms)..."
    for ($cycle = 1; $cycle -le $Cycles; $cycle++) {
        try {
            Invoke-DriverOnce $cycle
            $script:failures = 0
        } catch {
            $script:failures++
            Write-Warning "cycle ${cycle}: driver failure #$($script:failures): $($_.Exception.Message)"
            if ($script:failures -ge $FailuresBeforeDegrade) {
                Write-Warning "DEGRADED: $FailuresBeforeDegrade consecutive failures — stopping UI driving per policy."
                $script:degraded = $true
                break
            }
        }
        Start-Sleep -Milliseconds $ActionDelayMs
        if (-not (Sample "cycle-$cycle")) { Write-Warning 'process exited'; break }
    }
}

Write-Host "Phase: idle ${IdleSeconds}s..."
$idleEnd = (Get-Date).AddSeconds($IdleSeconds)
while ((Get-Date) -lt $idleEnd) {
    if (-not (Sample 'idle')) { break }
    Start-Sleep -Seconds $SampleIntervalSeconds
}

if ($dotnetCounters) {
    Stop-Process -Id $dotnetCounters.Id -Force -ErrorAction SilentlyContinue
}

# ---- Summary + baseline append ----
$rows = Import-Csv $csvPath
$driving = @($rows | Where-Object { $_.phase -like 'cycle-*' -or $_.phase -eq 'manual-cycle' })
$idleRows = @($rows | Where-Object { $_.phase -eq 'idle' })
function Mb([string]$v) { [double]::Parse($v, [Globalization.CultureInfo]::InvariantCulture) }

$summary = [ordered]@{
    scenario       = $Scenario
    timestamp      = (Get-Date -Format 'yyyy-MM-dd HH:mm:ss')
    commit         = (git -C $repoRoot rev-parse --short HEAD)
    cycles         = if ($driving.Count) { ($driving | Where-Object phase -like 'cycle-*-open').Count } else { $null }
    degraded       = $script:degraded
    wsStartMb      = [Math]::Round((Mb $rows[0].ws_mb), 1)
    wsPeakMb       = [Math]::Round(($rows | ForEach-Object { Mb $_.ws_mb } | Measure-Object -Maximum).Maximum, 1)
    wsEndMb        = [Math]::Round((Mb $rows[-1].ws_mb), 1)
    wsAfterIdleMb  = if ($idleRows.Count) { [Math]::Round((Mb $idleRows[-1].ws_mb), 1) } else { $null }
    privStartMb    = [Math]::Round((Mb $rows[0].private_mb), 1)
    privPeakMb     = [Math]::Round(($rows | ForEach-Object { Mb $_.private_mb } | Measure-Object -Maximum).Maximum, 1)
    privEndMb      = [Math]::Round((Mb $rows[-1].private_mb), 1)
    privAfterIdleMb = if ($idleRows.Count) { [Math]::Round((Mb $idleRows[-1].private_mb), 1) } else { $null }
    csv            = $csvPath
}

$summary | ConvertTo-Json | Out-File (Join-Path $outDir 'summary.json') -Encoding utf8
Write-Host ""
Write-Host ("Summary: WS {0} -> peak {1} -> end {2} -> idle {3} MB | Priv {4} -> peak {5} -> end {6} -> idle {7} MB | degraded={8}" -f `
    $summary.wsStartMb, $summary.wsPeakMb, $summary.wsEndMb, $summary.wsAfterIdleMb, `
    $summary.privStartMb, $summary.privPeakMb, $summary.privEndMb, $summary.privAfterIdleMb, $summary.degraded)

$baselineFile = Join-Path $repoRoot "docs\baselines\memory-scenarios.json"
$baselineDir = Split-Path $baselineFile -Parent
New-Item -ItemType Directory -Force -Path $baselineDir | Out-Null
$all = @()
if (Test-Path $baselineFile) {
    try {
        $loaded = Get-Content $baselineFile -Raw | ConvertFrom-Json
        # PS 5.1 round-tripping can produce {"value":[...],"Count":n} envelopes
        # or nested arrays — flatten defensively so the file stays a plain list.
        foreach ($e in @($loaded)) {
            if ($e -is [System.Array]) { $all += @($e) }
            elseif ($e.PSObject.Properties.Name -contains 'value' -and
                    $e.PSObject.Properties.Name -notcontains 'scenario') {
                $all += @($e.value)
            } else { $all += $e }
        }
    } catch { $all = @() }
}
$all += [pscustomobject]$summary
$all | ConvertTo-Json -Depth 4 | Out-File $baselineFile -Encoding utf8
Write-Host "Baseline appended: $baselineFile"
