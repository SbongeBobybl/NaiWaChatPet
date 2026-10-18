using DeskBox.Models;
using Windows.Graphics;

namespace DeskBox.Tests;

public sealed class WidgetSurfaceBoundsCalculatorTests
{
    [Theory]
    [InlineData(1.00, 36)]
    [InlineData(1.25, 45)]
    [InlineData(1.50, 54)]
    [InlineData(2.00, 72)]
    public void ExpandAndCollapse_PreserveContentCardAtSupportedDpi(
        double dpiScale,
        int expectedNavigationHeight)
    {
        var content = new RectInt32(200, 180, 360, 420);
        var workArea = new RectInt32(0, 0, 1920, 1040);

        WidgetSurfaceBoundsExpansion expansion =
            WidgetSurfaceBoundsCalculator.Expand(
                content,
                workArea,
                navigationLogicalHeight: 36,
                dpiScale);

        Assert.False(expansion.IsNavigationInset);
        Assert.Equal(expectedNavigationHeight, expansion.NavigationHeight);
        Assert.Equal(content.Y - expectedNavigationHeight, expansion.HostBounds.Y);
        Assert.Equal(
            content.Height + expectedNavigationHeight,
            expansion.HostBounds.Height);
        Assert.Equal(
            content,
            WidgetSurfaceBoundsCalculator.Collapse(
                expansion.HostBounds,
                expansion.IsNavigationInset,
                expansion.NavigationHeight));
    }

    [Fact]
    public void TopEdge_UsesInsetWithoutMovingOrResizingContentCard()
    {
        var content = new RectInt32(80, 8, 320, 400);
        var workArea = new RectInt32(0, 0, 1920, 1040);

        WidgetSurfaceBoundsExpansion expansion =
            WidgetSurfaceBoundsCalculator.Expand(
                content,
                workArea,
                navigationLogicalHeight: 36,
                dpiScale: 1.5);

        Assert.True(expansion.IsNavigationInset);
        Assert.Equal(content, expansion.HostBounds);
        Assert.Equal(
            content,
            WidgetSurfaceBoundsCalculator.Collapse(
                expansion.HostBounds,
                expansion.IsNavigationInset,
                expansion.NavigationHeight));
    }

    [Fact]
    public void NonZeroMonitorOrigin_IsRespected()
    {
        var workArea = new RectInt32(-2560, -200, 2560, 1400);
        var atTop = new RectInt32(-2400, -180, 400, 500);
        var belowTop = new RectInt32(-2400, -100, 400, 500);

        Assert.True(
            WidgetSurfaceBoundsCalculator.Expand(
                atTop,
                workArea,
                36,
                1.25).IsNavigationInset);
        Assert.False(
            WidgetSurfaceBoundsCalculator.Expand(
                belowTop,
                workArea,
                36,
                1.25).IsNavigationInset);
    }
}
