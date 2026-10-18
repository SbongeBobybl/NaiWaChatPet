using DeskBox;

namespace DeskBox.Tests;

/// <summary>
/// Pins the tray context menu resilience contract for the SecondWindow
/// prewarm failure: one failed open rebuilds the hidden host once, two
/// consecutive failures degrade the session permanently to the native
/// PopupMenu mode, and every menu item carries a Command so the native menu
/// stays clickable (it only executes Command, never Click).
/// </summary>
public sealed class TrayContextMenuResilienceTests
{
    [Fact]
    public void ModePolicy_FirstFailure_RequestsHostRebuildInsteadOfDegradation()
    {
        var policy = new TrayContextMenuModePolicy();

        bool degraded = policy.RecordFailure();

        Assert.False(degraded);
        Assert.False(policy.IsDegradedToPopupMenu);
        Assert.Equal(1, policy.ConsecutiveSecondWindowFailures);
    }

    [Fact]
    public void ModePolicy_TwoConsecutiveFailures_DegradeTheSession()
    {
        var policy = new TrayContextMenuModePolicy();
        policy.RecordFailure();

        bool degraded = policy.RecordFailure();

        Assert.True(degraded);
        Assert.True(policy.IsDegradedToPopupMenu);
    }

    [Fact]
    public void ModePolicy_SuccessBetweenFailures_ResetsTheCount()
    {
        var policy = new TrayContextMenuModePolicy();
        policy.RecordFailure();

        policy.RecordSuccess();
        bool degraded = policy.RecordFailure();

        Assert.False(degraded);
        Assert.Equal(1, policy.ConsecutiveSecondWindowFailures);
    }

    [Fact]
    public void ModePolicy_Degradation_IsIrreversibleForTheSession()
    {
        var policy = new TrayContextMenuModePolicy();
        policy.RecordFailure();
        policy.RecordFailure();

        // A later success may reset the count, but the session must stay on
        // the native menu: the second-window host already proved broken here.
        policy.RecordSuccess();
        bool degradedAgain = policy.RecordFailure();

        Assert.True(policy.IsDegradedToPopupMenu);
        Assert.False(degradedAgain);
    }

    [Fact]
    public void InteractionGate_AllowsOnlyTheFirstCallerUntilTheScheduledReset()
    {
        var resets = new List<Action>();
        var gate = new TrayMenuInteractionGate(reset => resets.Add(reset));

        // The XAML Click path and the command path fire back-to-back; only
        // the first may run the interaction.
        Assert.True(gate.TryEnter());
        Assert.False(gate.TryEnter());
        Assert.Single(resets);

        resets[0].Invoke();

        // After the reset lands, the next interaction is runnable again.
        Assert.True(gate.TryEnter());
    }

    [Fact]
    public void ShowTrayContextMenuFromTray_UsesThreeTierRecovery()
    {
        string tray = Read("src/DeskBox/App.Tray.cs");

        // Only the XamlRoot ArgumentException participates in the fallback;
        // other failures keep the historical log-only behavior.
        string filter = Slice(
            tray,
            "private static bool IsSecondWindowHostFailure",
            "private void ShowTrayContextMenuInNativeMode");
        Assert.Contains("ArgumentException", filter, StringComparison.Ordinal);
        Assert.Contains("\"XamlRoot\"", filter, StringComparison.Ordinal);

        string show = Slice(
            tray,
            "private void ShowTrayContextMenuFromTray()",
            "private static bool IsSecondWindowHostFailure");
        // Tier 3 entry: once degraded, opens never touch the second-window host.
        Assert.Contains("IsDegradedToPopupMenu", show, StringComparison.Ordinal);
        Assert.Contains("ShowTrayContextMenuInNativeMode(point);", show, StringComparison.Ordinal);
        // A successful open clears the failure count.
        Assert.Contains("_trayContextMenuModePolicy.RecordSuccess();", show, StringComparison.Ordinal);
        Assert.Contains(
            "[Tray] Failed to show tray context menu: {ex}",
            show,
            StringComparison.Ordinal);

        string recovery = Slice(
            tray,
            "private void RecoverTrayContextMenuAfterSecondWindowFailure",
            "private void DegradeTrayContextMenuToNativeMode");
        // Tier 2: re-assigning the flyout makes the library recreate the host.
        Assert.Contains("ContextFlyout = null;", recovery, StringComparison.Ordinal);
        Assert.Contains("ContextFlyout = _trayContextMenu;", recovery, StringComparison.Ordinal);

        string degrade = Slice(
            tray,
            "private void DegradeTrayContextMenuToNativeMode",
            "internal void ShowTrayContextMenuForOnboarding");
        Assert.Contains("ContextMenuMode = ContextMenuMode.PopupMenu;", degrade, StringComparison.Ordinal);
        Assert.Contains(
            "[Tray] Tray context menu degraded to native PopupMenu mode (SecondWindow host failed to initialize)",
            degrade,
            StringComparison.Ordinal);
        // Degrading immediately shows the native menu for the same right-click.
        Assert.Contains("ShowTrayContextMenuInNativeMode(point);", degrade, StringComparison.Ordinal);

        // The degrade path must switch the mode property so the library's own
        // DynamicDependency-annotated native path stays trim-safe; DeskBox
        // never constructs the native popup itself.
        Assert.DoesNotContain("new PopupMenu", tray, StringComparison.Ordinal);
    }

    [Fact]
    public void TrayMenuItems_AreAllCommandBackedForTheNativeFallback()
    {
        string tray = Read("src/DeskBox/App.Tray.cs");

        string create = Slice(
            tray,
            "private void CreateTrayIcon()",
            "/// <summary>Attempts per tray creation pass");
        Assert.DoesNotContain(".Click +=", create, StringComparison.Ordinal);
        // Seven fixed items: organize, map folder, feature widgets, settings,
        // managed storage, update, exit.
        Assert.Equal(7, CountOccurrences(create, "AttachTrayMenuItemInteraction("));

        string createWidget = Slice(
            tray,
            "private MenuFlyoutItem CreateTrayCreateWidgetItem",
            "private static void AttachTrayMenuItemInteraction");
        Assert.DoesNotContain(".Click +=", createWidget, StringComparison.Ordinal);
        Assert.Equal(1, CountOccurrences(createWidget, "AttachTrayMenuItemInteraction("));

        // The attach helper is the only place items get wired: Click for the
        // XAML second-window flyout, Command for the native PopupMenu
        // fallback, and the gate keeping one interaction single-shot.
        string attach = Slice(
            tray,
            "private static void AttachTrayMenuItemInteraction",
            "private void PrepareTrayContextMenu(");
        Assert.Contains("item.Click +=", attach, StringComparison.Ordinal);
        Assert.Contains("item.Command = new RelayCommand(", attach, StringComparison.Ordinal);
        Assert.Contains("gate.TryEnter()", attach, StringComparison.Ordinal);
    }

    [Fact]
    public void RunTraySettingsActionAsync_SkipsTheFlyoutSettleWaitWhenDegraded()
    {
        string tray = Read("src/DeskBox/App.Tray.cs");
        string settings = Slice(
            tray,
            "private async Task RunTraySettingsActionAsync",
            "internal async Task CreateFolderWidgetFromPickerAsync");

        int guard = settings.IndexOf("IsDegradedToPopupMenu", StringComparison.Ordinal);
        int delay = settings.IndexOf("await Task.Delay(300);", StringComparison.Ordinal);
        Assert.True(guard >= 0, "The settings action must branch on the degraded mode.");
        Assert.True(
            delay > guard,
            "The 300ms helper-window settle delay must sit behind the degraded guard.");
    }

    private static int CountOccurrences(string source, string value)
    {
        int count = 0;
        int index = 0;
        while ((index = source.IndexOf(value, index, StringComparison.Ordinal)) >= 0)
        {
            count++;
            index += value.Length;
        }

        return count;
    }

    private static string Slice(string source, string startMarker, string endMarker)
    {
        int start = source.IndexOf(startMarker, StringComparison.Ordinal);
        Assert.True(start >= 0, $"Start marker not found: {startMarker}");
        int end = source.IndexOf(endMarker, start + startMarker.Length, StringComparison.Ordinal);
        Assert.True(end > start, $"End marker not found: {endMarker}");
        return source[start..end];
    }

    private static string Read(string relativePath) =>
        File.ReadAllText(TestPaths.FromRepository(relativePath));
}
