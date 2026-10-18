using DeskBox.Services;
using Windows.Graphics;

namespace DeskBox.Tests;

public sealed class WidgetGroupDropHitTestPolicyTests
{
    [Fact]
    public void Contains_AcceptsOnlyTheVisibleTitleRectangle()
    {
        var titleBounds = new RectInt32(120, 80, 240, 42);

        Assert.True(WidgetGroupDropHitTestPolicy.Contains(titleBounds, 180, 100));
        Assert.False(WidgetGroupDropHitTestPolicy.Contains(titleBounds, 180, 122));
        Assert.False(WidgetGroupDropHitTestPolicy.Contains(titleBounds, 180, 220));
    }

    [Fact]
    public void Contains_UsesHalfOpenEdges()
    {
        var titleBounds = new RectInt32(10, 20, 30, 40);

        Assert.True(WidgetGroupDropHitTestPolicy.Contains(titleBounds, 10, 20));
        Assert.True(WidgetGroupDropHitTestPolicy.Contains(titleBounds, 39, 59));
        Assert.False(WidgetGroupDropHitTestPolicy.Contains(titleBounds, 40, 59));
        Assert.False(WidgetGroupDropHitTestPolicy.Contains(titleBounds, 39, 60));
    }

    [Fact]
    public void Contains_RejectsMissingOrEmptyTitleBounds()
    {
        Assert.False(WidgetGroupDropHitTestPolicy.Contains(null, 0, 0));
        Assert.False(WidgetGroupDropHitTestPolicy.Contains(
            new RectInt32(0, 0, 0, 12),
            0,
            0));
    }

    [Fact]
    public void Contains_SupportsNegativeMonitorCoordinates()
    {
        var titleBounds = new RectInt32(-1920, -50, 300, 40);

        Assert.True(WidgetGroupDropHitTestPolicy.Contains(
            titleBounds,
            -1800,
            -30));
    }

    [Fact]
    public void Contains_CompactIdentityDoesNotIncludeAdjacentCompactActions()
    {
        var compactIdentityBounds = new RectInt32(500, 300, 96, 34);

        Assert.True(WidgetGroupDropHitTestPolicy.Contains(
            compactIdentityBounds,
            560,
            317));
        Assert.False(WidgetGroupDropHitTestPolicy.Contains(
            compactIdentityBounds,
            610,
            317));
    }
}
