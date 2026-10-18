param(
    [Parameter(Mandatory = $true)][string]$ExePath,
    [int]$TimeoutSeconds = 40
)
$ErrorActionPreference = 'Stop'

# Ensure no DeskBox from this repo is already running (shared single-instance mutex).
Get-Process DeskBox -ErrorAction SilentlyContinue |
    Where-Object { $_.Path -like 'D:\project\wingezi*' } |
    ForEach-Object { Stop-Process -Id $_.Id -Force }
Start-Sleep -Milliseconds 800

$log = "$env:LOCALAPPDATA\DeskBox\DeskBox.log"
$beforeLength = if (Test-Path $log) { (Get-Item $log).Length } else { 0 }

$process = Start-Process -FilePath $ExePath -PassThru -WorkingDirectory (Split-Path $ExePath -Parent)
$startTime = $process.StartTime
$deadline = (Get-Date).AddSeconds($TimeoutSeconds)
$completedLine = $null

while ((Get-Date) -lt $deadline) {
    Start-Sleep -Milliseconds 120
    if ($process.HasExited) { break }
    try {
        $stream = [System.IO.File]::Open($log, 'Open', 'Read', 'ReadWrite')
        try {
            if ($stream.Length -gt $beforeLength) {
                $stream.Position = $beforeLength
                $reader = New-Object System.IO.StreamReader($stream)
                $newText = $reader.ReadToEnd()
                if ($newText -match 'OnLaunched completed successfully') {
                    $completedLine = ($newText -split "`n" | Where-Object { $_ -match 'OnLaunched completed successfully' } | Select-Object -Last 1)
                    break
                }
            }
        } finally { $stream.Dispose() }
    } catch { }
}

$elapsedMs = -1
if ($completedLine -and $completedLine -match '^\[(\d{2}):(\d{2}):(\d{2}\.\d{3})\]') {
    $now = Get-Date
    $lineTime = Get-Date -Hour ([int]$Matches[1]) -Minute ([int]$Matches[2]) -Second ([int]([double]$Matches[3])) -Millisecond ([int](([double]$Matches[3] % 1) * 1000))
    $elapsedMs = [int]($lineTime - $startTime).TotalMilliseconds
}

Write-Output ("elapsedMs={0} line={1}" -f $elapsedMs, $completedLine)

if (-not $process.HasExited) {
    Stop-Process -Id $process.Id -Force
    Start-Sleep -Milliseconds 900
}
