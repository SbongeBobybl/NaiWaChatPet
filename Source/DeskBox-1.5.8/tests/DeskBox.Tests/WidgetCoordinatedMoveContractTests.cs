namespace DeskBox.Tests;

public sealed class WidgetCoordinatedMoveContractTests
{
    [Fact]
    public void CtrlTitleDrag_UsesAtomicSameMonitorBatchWithoutZOrderMutation()
    {
        string interaction = Read("src/DeskBox/Views/WidgetWindowBase.Interaction.cs");
        string manager = Read("src/DeskBox/Services/WidgetManager.CoordinatedMove.cs");
        string contentInteraction = Read(
            "src/DeskBox/Views/ContentWidgetWindow.WindowInteraction.cs");

        Assert.Contains("VirtualKey.Control", interaction, StringComparison.Ordinal);
        Assert.Contains("TryBeginCoordinatedMove(HWnd)", interaction, StringComparison.Ordinal);
        Assert.Contains("UpdateCoordinatedMove(", interaction, StringComparison.Ordinal);
        Assert.Contains("frame.DeltaX", interaction, StringComparison.Ordinal);
        Assert.Contains("frame.DeltaY", interaction, StringComparison.Ordinal);
        Assert.Contains("CompleteCoordinatedMove(HWnd, hasMoved)", interaction, StringComparison.Ordinal);
        Assert.Contains("if (!_isCoordinatedMoveDrag)", interaction, StringComparison.Ordinal);

        Assert.Contains("window.Visible", manager, StringComparison.Ordinal);
        Assert.Contains("window.CanParticipateInCoordinatedMove", manager, StringComparison.Ordinal);
        Assert.Contains("MonitorFromWindow", manager, StringComparison.Ordinal);
        Assert.Contains("BeginDeferWindowPos", manager, StringComparison.Ordinal);
        Assert.Contains("DeferWindowPos", manager, StringComparison.Ordinal);
        Assert.Contains("EndDeferWindowPos", manager, StringComparison.Ordinal);
        Assert.Contains("SWP_NOZORDER", manager, StringComparison.Ordinal);
        Assert.Contains("SWP_NOACTIVATE", manager, StringComparison.Ordinal);
        Assert.Contains("UpdateWidgetsBatch", manager, StringComparison.Ordinal);
        Assert.Contains(
            "!Win32Helper.IsKeyPressed(Windows.System.VirtualKey.Control)",
            contentInteraction,
            StringComparison.Ordinal);
    }

    [Fact]
    public void WidgetSpacingSetting_UsesSliderInsteadOfNumberBox()
    {
        string settingsXaml = Read("src/DeskBox/Views/SettingsWindow.xaml");
        int start = settingsXaml.IndexOf(
            "Settings.WidgetSnap.Spacing.Title",
            StringComparison.Ordinal);
        Assert.True(start >= 0);
        int end = settingsXaml.IndexOf(
            "</toolkit:SettingsCard>",
            start,
            StringComparison.Ordinal);
        Assert.True(end > start);
        string card = settingsXaml[start..end];

        Assert.Contains("<Slider", card, StringComparison.Ordinal);
        Assert.Contains("StepFrequency=\"1\"", card, StringComparison.Ordinal);
        Assert.Contains("SnapSpacingText", card, StringComparison.Ordinal);
        Assert.DoesNotContain("<NumberBox", card, StringComparison.Ordinal);
    }

    private static string Read(string relativePath) =>
        File.ReadAllText(TestPaths.FromRepository(relativePath));
}
