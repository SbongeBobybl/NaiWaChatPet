namespace DeskBox.Tests;

public sealed class ShellUiForegroundContractTests
{
    [Fact]
    public void BrokenShortcut_OnlyPromotesItsOwnedNativeDialog()
    {
        string shortcut = ReadSource("src/DeskBox/Helpers/ShortcutHelper.cs");
        string monitor = ReadSource("src/DeskBox/Helpers/ShellUiForegroundMonitor.cs");

        Assert.Contains(
            "using IDisposable foregroundMonitor = ShellUiForegroundMonitor.Start(ownerHwnd);",
            shortcut,
            StringComparison.Ordinal);
        Assert.Contains(
            "requiredOwnerHwnd: ownerHwnd",
            monitor,
            StringComparison.Ordinal);
        Assert.Contains("Win32Helper.SetWindowTopMost(dialogHwnd);", monitor, StringComparison.Ordinal);
        Assert.Contains("Win32Helper.SetForegroundWindow(dialogHwnd)", monitor, StringComparison.Ordinal);
        Assert.DoesNotContain("SetWindowTopMost(ownerHwnd)", monitor, StringComparison.Ordinal);
    }

    [Fact]
    public void DialogMonitor_IsBoundedAndDoesNotBlockTheUiThread()
    {
        string monitor = ReadSource("src/DeskBox/Helpers/ShellUiForegroundMonitor.cs");

        Assert.Contains("Task.Run(() => MonitorOwnedDialogAsync(", monitor, StringComparison.Ordinal);
        Assert.Contains("DiscoveryWindow = TimeSpan.FromSeconds(10)", monitor, StringComparison.Ordinal);
        Assert.Contains("Task.Delay(PollInterval, cancellationToken)", monitor, StringComparison.Ordinal);
        Assert.DoesNotContain(".Wait()", monitor, StringComparison.Ordinal);
        Assert.DoesNotContain(".Result", monitor, StringComparison.Ordinal);
    }

    private static string ReadSource(string path) =>
        File.ReadAllText(TestPaths.FromRepository(path));
}
