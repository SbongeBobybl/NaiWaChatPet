using System.Text.Json;
using System.Text.Json.Serialization;

namespace DeskBox.Services;

internal sealed record StartupSessionLedgerEntry(int Version,string Phase,DateTimeOffset TimestampUtc);

internal sealed record StartupStormAssessment(
    bool SuppressFileMutatingStartup,    DateTimeOffset? PreviousLaunchStartedAtUtc,    string Reason);

/// <summary>
/// Decides whether the current launch should keep desktop file mutations
/// deferred. A previous primary launch that wrote "started" and never wrote
/// "clean-exit" died abnormally (crash, power loss, task-manager kill); when
/// that happened inside the suppression window — the cadence of a logon-time
/// Explorer crash loop re-firing the Run entry — every relaunch would
/// otherwise replay startup file operations into an environment that is
/// killing processes mid-flight (feedback #576).
/// </summary>
internal static class StartupStormPolicy
{
    internal const string PhaseStarted = "started";
    internal const string PhaseCleanExit = "clean-exit";
    internal const int LedgerVersion = 1;

    /// <summary>
    /// Only launches this recent after an abnormal death trip the guard. Age
    /// is measured from the previous launch's START, not its death: storm
    /// relaunches live one to four minutes, so start time is a faithful
    /// proxy for the death moment in exactly the cadence this guard targets.
    /// A long-running session that is task-killed and relaunched minutes
    /// later therefore does NOT trip (start is hours old) and its organizer
    /// runs immediately; that is accepted, not an oversight.
    /// </summary>
    internal static readonly TimeSpan SuppressionWindow = TimeSpan.FromMinutes(10);

    internal static StartupStormAssessment Assess(
        StartupSessionLedgerEntry? previous,        DateTimeOffset nowUtc)
    {
        if (previous is null)
        {
            return new StartupStormAssessment(false,null,"no-previous-entry");
        }

        if (!string.Equals(previous.Phase,PhaseStarted,StringComparison.Ordinal))
        {
            return new StartupStormAssessment(
                false,                null,                $"previous-phase={previous.Phase}");
        }

        TimeSpan age = nowUtc - previous.TimestampUtc;
        if (age < TimeSpan.Zero)
        {
            // A future timestamp is clock skew, not evidence of a storm.
            return new StartupStormAssessment(
                false,                previous.TimestampUtc,                $"clock-skew={-age.TotalSeconds:F0}s");
        }

        if (age > SuppressionWindow)
        {
            return new StartupStormAssessment(
                false,                previous.TimestampUtc,                $"abnormal-exit-age={age.TotalSeconds:F0}s");
        }

        return new StartupStormAssessment(
            true,            previous.TimestampUtc,            $"abnormal-exit-age={age.TotalSeconds:F0}s");
    }
}

/// <summary>
/// Persists the launch breadcrumb beside the application log. The write is a
/// plain overwrite on purpose: a torn file only ever fails the next read
/// open, and a false-open guard is the safe direction (no suppression) —
/// neither justifies routing this through an atomic tmp+move pipeline.
/// </summary>
internal sealed class StartupSessionLedger
{
    private readonly string _filePath;
    private readonly Func<DateTimeOffset> _utcNow;

    public StartupSessionLedger(string filePath,Func<DateTimeOffset>? utcNow = null)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(filePath);
        _filePath = filePath;
        // lambda 不能直接作 ?? 右操作数，经 method group 转换。
        _utcNow = utcNow ?? DefaultUtcNow;
    }

    private static DateTimeOffset DefaultUtcNow() => DateTimeOffset.UtcNow;

    /// <summary>
    /// Records that a primary launch began and reports whether the previous
    /// launch's death warrants deferring file-mutating startup work.
    /// </summary>
    public StartupStormAssessment BeginPrimaryLaunch()
    {
        StartupSessionLedgerEntry? previous = TryRead();
        StartupStormAssessment assessment = StartupStormPolicy.Assess(previous,_utcNow());
        TryWrite(new StartupSessionLedgerEntry(
            StartupStormPolicy.LedgerVersion,            StartupStormPolicy.PhaseStarted,            _utcNow()));
        return assessment;
    }

    /// <summary>
    /// Records an intentional exit (tray exit, update restart, OS session
    /// end, fatal-startup exit). Called at the start of the shutdown
    /// sequence so a death during teardown still reads as clean.
    /// </summary>
    public void MarkCleanExit()
    {
        TryWrite(new StartupSessionLedgerEntry(
            StartupStormPolicy.LedgerVersion,            StartupStormPolicy.PhaseCleanExit,            _utcNow()));
    }

    private StartupSessionLedgerEntry? TryRead()
    {
        try
        {
            if (!File.Exists(_filePath))
            {
                return null;
            }

            var entry = JsonSerializer.Deserialize(
                File.ReadAllText(_filePath),                StartupLedgerJsonContext.Default.StartupSessionLedgerEntry);
            return entry is { } value && value.Version == StartupStormPolicy.LedgerVersion
                ? value
                : null;
        }
        catch (Exception ex)
        {
            // Unreadable or torn ledger: fail open (no suppression). The
            // overwrite below repairs the file for the next launch.
            App.LogVerbose($"[Startup] Session ledger read failed: {ex.Message}");
            return null;
        }
    }

    private void TryWrite(StartupSessionLedgerEntry entry)
    {
        try
        {
            File.WriteAllText(
                _filePath,                JsonSerializer.Serialize(entry,StartupLedgerJsonContext.Default.StartupSessionLedgerEntry));
        }
        catch (Exception ex)
        {
            App.LogVerbose($"[Startup] Session ledger write failed: {ex.Message}");
        }
    }
}

[JsonSourceGenerationOptions(
    PropertyNamingPolicy = JsonKnownNamingPolicy.CamelCase,
    WriteIndented = false)]
[JsonSerializable(
    typeof(StartupSessionLedgerEntry),
    TypeInfoPropertyName = "StartupSessionLedgerEntry")]
internal sealed partial class StartupLedgerJsonContext : JsonSerializerContext
{
}
