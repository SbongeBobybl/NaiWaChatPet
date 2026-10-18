using DeskBox.Services;
using Windows.Graphics;

namespace DeskBox.Tests;

public sealed class WidgetSnapCalculatorTests
{
    private static readonly WidgetSnapTarget s_target = new(
        new RectInt32(202, 202, 100, 100),
        new IntPtr(42));

    [Fact]
    public void Move_RightEdgeKeepsConfiguredGapBeforeTarget()
    {
        WidgetMoveSnapResult result = ResolveMove(new RectInt32(96, 220, 100, 60));

        Assert.Equal(97, result.Bounds.X);
        WidgetSnapMatch match = AssertSnap(result.HorizontalMatch);
        Assert.Equal(WidgetSnapEdge.Right, match.SourceEdge);
        Assert.Equal(WidgetSnapEdge.Left, match.TargetEdge);
        Assert.True(match.UsesSpacing);
        Assert.Equal(197, match.Coordinate);
    }

    [Fact]
    public void Move_LeftEdgeKeepsConfiguredGapAfterTarget()
    {
        var target = new WidgetSnapTarget(
            new RectInt32(0, 20, 100, 100),
            new IntPtr(43));

        WidgetMoveSnapResult result = WidgetSnapCalculator.ResolveMove(
            new RectInt32(104, 40, 100, 60),
            [target],
            workArea: null,
            spacing: 5,
            engageThreshold: 8,
            releaseThreshold: 12);

        Assert.Equal(105, result.Bounds.X);
        WidgetSnapMatch match = AssertSnap(result.HorizontalMatch);
        Assert.Equal(WidgetSnapEdge.Left, match.SourceEdge);
        Assert.Equal(WidgetSnapEdge.Right, match.TargetEdge);
        Assert.True(match.UsesSpacing);
    }

    [Fact]
    public void Move_BottomEdgeKeepsConfiguredGapAboveTarget()
    {
        WidgetMoveSnapResult result = ResolveMove(new RectInt32(220, 96, 60, 100));

        Assert.Equal(97, result.Bounds.Y);
        WidgetSnapMatch match = AssertSnap(result.VerticalMatch);
        Assert.Equal(WidgetSnapEdge.Bottom, match.SourceEdge);
        Assert.Equal(WidgetSnapEdge.Top, match.TargetEdge);
        Assert.True(match.UsesSpacing);
    }

    [Fact]
    public void Move_TopEdgeKeepsConfiguredGapBelowTarget()
    {
        var target = new WidgetSnapTarget(
            new RectInt32(20, 0, 100, 100),
            new IntPtr(44));

        WidgetMoveSnapResult result = WidgetSnapCalculator.ResolveMove(
            new RectInt32(40, 104, 60, 100),
            [target],
            workArea: null,
            spacing: 5,
            engageThreshold: 8,
            releaseThreshold: 12);

        Assert.Equal(105, result.Bounds.Y);
        WidgetSnapMatch match = AssertSnap(result.VerticalMatch);
        Assert.Equal(WidgetSnapEdge.Top, match.SourceEdge);
        Assert.Equal(WidgetSnapEdge.Bottom, match.TargetEdge);
        Assert.True(match.UsesSpacing);
    }

    [Fact]
    public void Move_SameEdgeAlignmentDoesNotAddSpacing()
    {
        WidgetMoveSnapResult result = ResolveMove(new RectInt32(205, 220, 80, 60));

        Assert.Equal(202, result.Bounds.X);
        WidgetSnapMatch match = AssertSnap(result.HorizontalMatch);
        Assert.Equal(WidgetSnapEdge.Left, match.SourceEdge);
        Assert.Equal(WidgetSnapEdge.Left, match.TargetEdge);
        Assert.False(match.UsesSpacing);
    }

    [Fact]
    public void Move_WorkAreaRightAndBottomEdgesUseZeroGap()
    {
        WidgetMoveSnapResult result = WidgetSnapCalculator.ResolveMove(
            new RectInt32(901, 701, 100, 100),
            [],
            new RectInt32(0, 0, 1000, 800),
            spacing: 5,
            engageThreshold: 8,
            releaseThreshold: 12);

        Assert.Equal(900, result.Bounds.X);
        Assert.Equal(700, result.Bounds.Y);
        Assert.Equal(IntPtr.Zero, AssertSnap(result.HorizontalMatch).TargetWindowHandle);
        Assert.Equal(IntPtr.Zero, AssertSnap(result.VerticalMatch).TargetWindowHandle);
    }

    [Fact]
    public void Move_StickyMatchHoldsUntilReleaseThreshold()
    {
        WidgetMoveSnapResult engaged = ResolveMove(new RectInt32(96, 220, 100, 60));
        WidgetSnapMatch sticky = AssertSnap(engaged.HorizontalMatch);

        WidgetMoveSnapResult held = WidgetSnapCalculator.ResolveMove(
            new RectInt32(108, 220, 100, 60),
            [s_target],
            workArea: null,
            spacing: 5,
            engageThreshold: 8,
            releaseThreshold: 12,
            stickyHorizontal: sticky);
        WidgetMoveSnapResult released = WidgetSnapCalculator.ResolveMove(
            new RectInt32(110, 220, 100, 60),
            [s_target],
            workArea: null,
            spacing: 5,
            engageThreshold: 8,
            releaseThreshold: 12,
            stickyHorizontal: sticky);

        Assert.Equal(97, held.Bounds.X);
        Assert.NotNull(held.HorizontalMatch);
        Assert.Equal(110, released.Bounds.X);
        Assert.Null(released.HorizontalMatch);
    }

    [Fact]
    public void Resize_RightEdgeUsesConfiguredGap()
    {
        WidgetSnapMatch? match = WidgetSnapCalculator.ResolveResizeEdge(
            new RectInt32(96, 220, 100, 60),
            WidgetSnapEdge.Right,
            [s_target],
            workArea: null,
            spacing: 5,
            threshold: 8);

        WidgetSnapMatch snap = AssertSnap(match);
        Assert.Equal(197, snap.Coordinate);
        Assert.True(snap.UsesSpacing);
    }

    private static WidgetMoveSnapResult ResolveMove(RectInt32 proposedBounds) =>
        WidgetSnapCalculator.ResolveMove(
            proposedBounds,
            [s_target],
            workArea: null,
            spacing: 5,
            engageThreshold: 8,
            releaseThreshold: 12);

    private static WidgetSnapMatch AssertSnap(WidgetSnapMatch? match)
    {
        Assert.True(match.HasValue);
        return match.GetValueOrDefault();
    }
}
