namespace DeskBox.Tests;

/// <summary>
/// Pins the storm-guard wiring: the ledger's clean-exit call sites, the two
/// startup gates, and the watcher's pre-start settings guard. The unit tests
/// cover the policy; these pins keep the App-side integration from silently
/// losing a call site to a refactor (feedback #576 batch).
/// </summary>
public sealed class StartupStormGuardContractTests
{
    private const string AppSource = "src/DeskBox/App.xaml.cs";
    private const string AppStartupSource = "src/DeskBox/App.Startup.cs";
    private const string WatcherSource = "src/DeskBox/Services/DesktopAutoOrganizationWatcher.cs";
    private const string SnapshotSource = "src/DeskBox/App.DiagnosticsBundle.cs";

    [Fact]
    public void CleanExitIsRecordedOnEveryIntentionalExitPath()
    {
        string app = File.ReadAllText(TestPaths.FromRepository(AppSource));

        // Shutdown sequence (tray exit / restart / update), end-session flush,
        // and the elevated-relaunch exit all carry the breadcrumb close.
        Assert.Equal(3, Count(app, "_startupSessionLedger?.MarkCleanExit();"));
    }

    [Fact]
    public void FatalStartupClosesTheBreadcrumbThroughTheStaticPath()
    {
        string startup = File.ReadAllText(TestPaths.FromRepository(AppStartupSource));

        Assert.Contains(
            "Current?._startupSessionLedger?.MarkCleanExit();",
            startup,
            StringComparison.Ordinal);
    }

    [Fact]
    public void StormGatesCoverOrganizationRecoveryAndWatcherStart()
    {
        string app = File.ReadAllText(TestPaths.FromRepository(AppSource));

        // The recovery skip and the deferred watcher start are the two
        // file-mutating startup surfaces the guard suppresses.
        Assert.Contains(
            "Storm guard: deferring desktop-organization recovery",
            app,
            StringComparison.Ordinal);
        Assert.Contains(
            "Storm guard: desktop auto-organization watcher start deferred",
            app,
            StringComparison.Ordinal);
        Assert.Contains(
            "SuppressFileMutatingStartup",
            app,
            StringComparison.Ordinal);
    }

    [Fact]
    public void WatcherIgnoresSettingsTogglesBeforeStart()
    {
        string watcher = File.ReadAllText(TestPaths.FromRepository(WatcherSource));

        // Without this guard a settings save during the deferred window
        // re-enables the FileSystemWatcher and bypasses the storm guard.
        Assert.Contains(
            "Settings toggles must not bypass the deferred start",
            watcher,
            StringComparison.Ordinal);
    }

    [Fact]
    public void DiagnosticSnapshotCarriesTheStartupLedgerSummary()
    {
        string snapshot = File.ReadAllText(TestPaths.FromRepository(SnapshotSource));

        Assert.Contains("StartupLedger: startupLedger", snapshot, StringComparison.Ordinal);
    }

    private static int Count(string source, string value) =>
        source.Split(value, StringSplitOptions.None).Length - 1;
}
