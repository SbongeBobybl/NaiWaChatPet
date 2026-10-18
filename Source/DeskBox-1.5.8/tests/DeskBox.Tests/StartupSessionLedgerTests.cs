using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class StartupSessionLedgerTests : IDisposable
{
    private readonly string _temporaryRoot = Path.Combine(
        Path.GetTempPath(),
        "DeskBoxStartupLedgerTests",
        Guid.NewGuid().ToString("N"));

    private DateTimeOffset _utcNow = new(2026, 10, 13, 14, 0, 0, TimeSpan.Zero);

    private StartupSessionLedger CreateLedger() => new(
        Path.Combine(_temporaryRoot, "startup-ledger.json"),
        () => _utcNow);

    [Fact]
    public void StormPolicy_TripsOnlyForRecentAbnormalDeath()
    {
        DateTimeOffset now = new(2026, 10, 13, 14, 0, 0, TimeSpan.Zero);

        // No previous entry, clean exit, and unknown phases never trip.
        Assert.False(StartupStormPolicy.Assess(null, now).SuppressFileMutatingStartup);
        Assert.False(StartupStormPolicy.Assess(
            new StartupSessionLedgerEntry(1, StartupStormPolicy.PhaseCleanExit, now - TimeSpan.FromMinutes(1)),
            now).SuppressFileMutatingStartup);
        Assert.False(StartupStormPolicy.Assess(
            new StartupSessionLedgerEntry(1, "unknown", now - TimeSpan.FromMinutes(1)),
            now).SuppressFileMutatingStartup);

        // Abnormal death inside the window trips.
        Assert.True(StartupStormPolicy.Assess(
            new StartupSessionLedgerEntry(1, StartupStormPolicy.PhaseStarted, now - TimeSpan.FromMinutes(4)),
            now).SuppressFileMutatingStartup);

        // The same death one minute past the window does not.
        Assert.False(StartupStormPolicy.Assess(
            new StartupSessionLedgerEntry(
                1,
                StartupStormPolicy.PhaseStarted,
                now - StartupStormPolicy.SuppressionWindow - TimeSpan.FromMinutes(1)),
            now).SuppressFileMutatingStartup);

        // Clock skew reads as noise, not as a storm.
        Assert.False(StartupStormPolicy.Assess(
            new StartupSessionLedgerEntry(1, StartupStormPolicy.PhaseStarted, now + TimeSpan.FromMinutes(5)),
            now).SuppressFileMutatingStartup);
    }

    [Fact]
    public void BeginPrimaryLaunch_ReportsPreviousAbnormalLaunch()
    {
        Directory.CreateDirectory(_temporaryRoot);
        CreateLedger().BeginPrimaryLaunch();
        _utcNow += TimeSpan.FromMinutes(2);

        StartupStormAssessment assessment = CreateLedger().BeginPrimaryLaunch();

        Assert.True(assessment.SuppressFileMutatingStartup);
        Assert.NotNull(assessment.PreviousLaunchStartedAtUtc);
    }

    [Fact]
    public void MarkCleanExit_PreventsSuppressionOnNextLaunch()
    {
        Directory.CreateDirectory(_temporaryRoot);
        StartupSessionLedger first = CreateLedger();
        first.BeginPrimaryLaunch();
        first.MarkCleanExit();
        _utcNow += TimeSpan.FromMinutes(1);

        StartupStormAssessment assessment = CreateLedger().BeginPrimaryLaunch();

        Assert.False(assessment.SuppressFileMutatingStartup);
    }

    [Fact]
    public void TornOrForeignLedger_FailsOpen()
    {
        Directory.CreateDirectory(_temporaryRoot);
        string ledgerPath = Path.Combine(_temporaryRoot, "startup-ledger.json");
        File.WriteAllText(ledgerPath, "{\"version\": 1, \"phase\": \"started\""); // torn JSON

        StartupStormAssessment assessment = CreateLedger().BeginPrimaryLaunch();

        Assert.False(assessment.SuppressFileMutatingStartup);

        // A future version's entry must not be interpreted either.
        _utcNow += TimeSpan.FromSeconds(30);
        File.WriteAllText(
            ledgerPath,
            "{\"version\":99,\"phase\":\"started\",\"timestampUtc\":\"2026-10-13T13:59:30Z\"}");
        Assert.False(CreateLedger().BeginPrimaryLaunch().SuppressFileMutatingStartup);
    }

    [Fact]
    public void FirstEverLaunch_NeverSuppresses()
    {
        Directory.CreateDirectory(_temporaryRoot);

        StartupStormAssessment assessment = CreateLedger().BeginPrimaryLaunch();

        Assert.False(assessment.SuppressFileMutatingStartup);
        Assert.Null(assessment.PreviousLaunchStartedAtUtc);
    }

    public void Dispose()
    {
        if (Directory.Exists(_temporaryRoot))
        {
            Directory.Delete(_temporaryRoot, recursive: true);
        }
    }
}
