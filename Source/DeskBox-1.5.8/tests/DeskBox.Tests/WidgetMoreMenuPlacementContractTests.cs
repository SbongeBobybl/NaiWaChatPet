namespace DeskBox.Tests;

public sealed class WidgetMoreMenuPlacementContractTests
{
    [Fact]
    public void WidgetShell_ForwardsTheCurrentMouseInvocationPosition()
    {
        string source = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetShell.xaml.cs"));
        string click = SliceMethod(
            source,
            "private void MoreButton_Click",
            "private void MoreButton_PointerPressed");
        string pointerPressed = SliceMethod(
            source,
            "private void MoreButton_PointerPressed",
            "private void MoreButton_PointerReleased");

        Assert.Contains(
            "EventHandler<WidgetMenuRequestedEventArgs>? MoreRequested",
            source,
            StringComparison.Ordinal);
        Assert.Contains("new WidgetMenuRequestedEventArgs", click, StringComparison.Ordinal);
        Assert.Contains("MoreButton,", click, StringComparison.Ordinal);
        Assert.Contains("ConsumePendingMoreMenuPointerPosition()", click, StringComparison.Ordinal);

        Assert.Contains("PointerDeviceType.Mouse", pointerPressed, StringComparison.Ordinal);
        Assert.Contains("PointerDeviceType.Touchpad", pointerPressed, StringComparison.Ordinal);
        Assert.Contains("IsLeftButtonPressed", pointerPressed, StringComparison.Ordinal);
        Assert.Contains("MoreMenuPointerOffsetDips", pointerPressed, StringComparison.Ordinal);
    }

    [Fact]
    public void NonMouseAndStaleInvocations_FallBackToTheMoreButtonAnchor()
    {
        string source = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetShell.xaml.cs"));
        string constructor = SliceMethod(
            source,
            "public WidgetShell()",
            "internal void ResumeVisualActivity()");
        string release = SliceMethod(
            source,
            "private void MoreButton_PointerReleased",
            "private void MoreButton_PointerCanceled");
        string consume = SliceMethod(
            source,
            "private Windows.Foundation.Point? ConsumePendingMoreMenuPointerPosition",
            "private void ClearPendingMoreMenuPointerPosition");

        Assert.Contains("UIElement.PointerCanceledEvent", constructor, StringComparison.Ordinal);
        Assert.Contains("UIElement.KeyDownEvent", constructor, StringComparison.Ordinal);
        Assert.Contains("DispatcherQueue.TryEnqueue", release, StringComparison.Ordinal);
        Assert.Contains("ClearPendingMoreMenuPointerPosition();", consume, StringComparison.Ordinal);
        Assert.Contains("MoreMenuPointerMaximumAgeMilliseconds", consume, StringComparison.Ordinal);
        Assert.Contains("return null;", consume, StringComparison.Ordinal);
    }

    [Fact]
    public void MenuConsumers_UseTheExplicitAnchorAndOptionalPointerPosition()
    {
        AssertMenuConsumer(
            "src/DeskBox/Views/ContentWidgetWindow.Commands.cs",
            "ShowFlyoutWithInteraction");
    }

    private static void AssertMenuConsumer(string relativePath, string showMethod)
    {
        string source = File.ReadAllText(TestPaths.FromRepository(relativePath));
        string handler = SliceMethod(
            source,
            "private void MoreButton_Click",
            relativePath.Contains("ContentWidgetWindow", StringComparison.Ordinal)
                ? "private void PositionLockButton_Click"
                : "private void TitleBarGrid_RightTapped");

        Assert.Contains("WidgetMenuRequestedEventArgs e", handler, StringComparison.Ordinal);
        Assert.Contains(showMethod, handler, StringComparison.Ordinal);
        Assert.Contains("e.Anchor", handler, StringComparison.Ordinal);
        Assert.Contains("e.PointerPosition", handler, StringComparison.Ordinal);
        Assert.DoesNotContain("sender as FrameworkElement", handler, StringComparison.Ordinal);
    }

    private static string SliceMethod(string source, string startMarker, string endMarker)
    {
        int start = source.IndexOf(startMarker, StringComparison.Ordinal);
        Assert.True(start >= 0, $"Missing source marker: {startMarker}");
        int end = source.IndexOf(endMarker, start, StringComparison.Ordinal);
        Assert.True(end > start, $"Missing source marker: {endMarker}");
        return source[start..end];
    }
}
