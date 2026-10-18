using DeskBox.Models;
using DeskBox.Services;
using Windows.Graphics;

namespace DeskBox.Tests;

public sealed class WidgetCompactExpansionCalculatorTests
{
    [Theory]
    [InlineData(WidgetCompactExpansionAnchor.LeftTop, 500, 300)]
    [InlineData(WidgetCompactExpansionAnchor.RightTop, 180, 300)]
    [InlineData(WidgetCompactExpansionAnchor.LeftBottom, 500, 42)]
    [InlineData(WidgetCompactExpansionAnchor.RightBottom, 180, 42)]
    public void Resolve_PreservesTheSelectedCompactCorner(
        WidgetCompactExpansionAnchor anchor,
        int expectedX,
        int expectedY)
    {
        RectInt32 compact = new(500, 300, 80, 42);
        WidgetCompactExpansionLayout result = WidgetCompactExpansionCalculator.Resolve(
            compact,
            new SizeInt32(400, 300),
            new RectInt32(0, 0, 1200, 900),
            [anchor]);

        Assert.Equal(new RectInt32(expectedX, expectedY, 400, 300), result.ExpandedBounds);
        Assert.Equal(
            WidgetCompactExpansionCalculator.GetPivot(compact, anchor),
            WidgetCompactExpansionCalculator.GetPivot(result.ExpandedBounds, anchor));
    }

    [Fact]
    public void Resolve_ChoosesAFittingDirectionBeforePreferredOverflow()
    {
        WidgetCompactExpansionLayout result = WidgetCompactExpansionCalculator.Resolve(
            new RectInt32(850, 40, 120, 42),
            new SizeInt32(500, 400),
            new RectInt32(0, 0, 1000, 800),
            [
                WidgetCompactExpansionAnchor.LeftTop,
                WidgetCompactExpansionAnchor.RightTop
            ]);

        Assert.Equal(WidgetCompactExpansionAnchor.RightTop, result.Anchor);
        Assert.False(result.IsSizeConstrained);
        Assert.Equal(new RectInt32(470, 40, 500, 400), result.ExpandedBounds);
    }

    [Fact]
    public void Resolve_ConstrainsSizeWithoutBreakingTheSharedCorner()
    {
        RectInt32 compact = new(480, 360, 80, 42);
        WidgetCompactExpansionLayout result = WidgetCompactExpansionCalculator.Resolve(
            compact,
            new SizeInt32(900, 700),
            new RectInt32(0, 0, 1000, 800),
            [WidgetCompactExpansionAnchor.LeftTop]);

        Assert.True(result.IsSizeConstrained);
        Assert.Equal(new RectInt32(480, 360, 520, 440), result.ExpandedBounds);
        Assert.Equal(
            WidgetCompactExpansionCalculator.GetPivot(compact, result.Anchor),
            WidgetCompactExpansionCalculator.GetPivot(result.ExpandedBounds, result.Anchor));
    }

    [Fact]
    public void Resolve_StrictFixedDirectionRefusesAConstrainedTarget()
    {
        RectInt32 compact = new(480, 738, 80, 42);
        WidgetCompactExpansionLayout result = WidgetCompactExpansionCalculator.Resolve(
            compact,
            new SizeInt32(900, 700),
            new RectInt32(0, 0, 1000, 800),
            [WidgetCompactExpansionAnchor.LeftTop],
            requireFullSize: true);

        Assert.False(result.CanExpand);
        Assert.Equal(compact, result.ExpandedBounds);
        Assert.True(result.IsSizeConstrained);
        Assert.Equal(new SizeInt32(900, 700), result.RequestedSize);
    }

    [Fact]
    public void Resolve_StrictFixedDirectionAllowsAFullTarget()
    {
        WidgetCompactExpansionLayout result = WidgetCompactExpansionCalculator.Resolve(
            new RectInt32(480, 100, 80, 42),
            new SizeInt32(400, 300),
            new RectInt32(0, 0, 1000, 800),
            [WidgetCompactExpansionAnchor.LeftTop],
            requireFullSize: true);

        Assert.True(result.CanExpand);
        Assert.False(result.IsSizeConstrained);
        Assert.Equal(new RectInt32(480, 100, 400, 300), result.ExpandedBounds);
    }

    [Theory]
    [InlineData(WidgetCompactExpansionAnchor.LeftTop)]
    [InlineData(WidgetCompactExpansionAnchor.RightTop)]
    [InlineData(WidgetCompactExpansionAnchor.LeftBottom)]
    [InlineData(WidgetCompactExpansionAnchor.RightBottom)]
    public void InterpolateAnchoredBounds_KeepsPivotFixedForEveryFrame(
        WidgetCompactExpansionAnchor anchor)
    {
        RectInt32 compact = new(500, 300, 160, 42);
        PointInt32 pivot = WidgetCompactExpansionCalculator.GetPivot(compact, anchor);
        RectInt32 expanded = WidgetCompactExpansionCalculator.CreateBoundsFromPivot(
            pivot,
            new SizeInt32(420, 320),
            anchor);

        for (int step = 0; step <= 20; step++)
        {
            RectInt32 frame = WidgetCompactExpansionCalculator.InterpolateAnchoredBounds(
                compact,
                expanded,
                pivot,
                anchor,
                step / 20d);
            Assert.Equal(pivot, WidgetCompactExpansionCalculator.GetPivot(frame, anchor));
        }
    }

    [Fact]
    public void InterpolateAnchoredBounds_IsSymmetric()
    {
        const WidgetCompactExpansionAnchor anchor = WidgetCompactExpansionAnchor.RightBottom;
        RectInt32 compact = new(500, 300, 160, 42);
        PointInt32 pivot = WidgetCompactExpansionCalculator.GetPivot(compact, anchor);
        RectInt32 expanded = WidgetCompactExpansionCalculator.CreateBoundsFromPivot(
            pivot,
            new SizeInt32(420, 320),
            anchor);

        RectInt32 opening = WidgetCompactExpansionCalculator.InterpolateAnchoredBounds(
            compact,
            expanded,
            pivot,
            anchor,
            0.35);
        RectInt32 closing = WidgetCompactExpansionCalculator.InterpolateAnchoredBounds(
            expanded,
            compact,
            pivot,
            anchor,
            0.65);

        Assert.Equal(opening, closing);
    }

    [Theory]
    [InlineData(WidgetCompactExpansionAnchor.RightTop, "Right", WidgetCompactExpansionAnchor.LeftTop)]
    [InlineData(WidgetCompactExpansionAnchor.RightBottom, "Right", WidgetCompactExpansionAnchor.LeftBottom)]
    [InlineData(WidgetCompactExpansionAnchor.LeftTop, "Left", WidgetCompactExpansionAnchor.RightTop)]
    [InlineData(WidgetCompactExpansionAnchor.LeftBottom, "Left", WidgetCompactExpansionAnchor.RightBottom)]
    public void ResolveHorizontalResizeAnchor_PreservesTheOppositeEdge(
        WidgetCompactExpansionAnchor current,
        string direction,
        WidgetCompactExpansionAnchor expected)
    {
        Assert.Equal(
            expected,
            WidgetCompactExpansionCalculator.ResolveHorizontalResizeAnchor(
                current,
                direction));
    }

    [Theory]
    [InlineData(100, 50, WidgetCompactExpansionAnchor.LeftTop)]
    [InlineData(420, 50, WidgetCompactExpansionAnchor.RightTop)]
    [InlineData(100, 308, WidgetCompactExpansionAnchor.LeftBottom)]
    [InlineData(420, 308, WidgetCompactExpansionAnchor.RightBottom)]
    public void ResolveAnchorFromObservedCompactBounds_ClassifiesTheOccupiedCorner(
        int compactX,
        int compactY,
        WidgetCompactExpansionAnchor expected)
    {
        Assert.Equal(
            expected,
            WidgetCompactExpansionCalculator.ResolveAnchorFromObservedCompactBounds(
                new RectInt32(compactX, compactY, 80, 42),
                new RectInt32(100, 50, 400, 300)));
    }

    [Fact]
    public void ResolveAnchorFromObservedCompactBounds_ResolvesTiesTowardLeftTop()
    {
        // A capsule centered on the panel must classify deterministically.
        Assert.Equal(
            WidgetCompactExpansionAnchor.LeftTop,
            WidgetCompactExpansionCalculator.ResolveAnchorFromObservedCompactBounds(
                new RectInt32(160, 129, 80, 42),
                new RectInt32(0, 0, 400, 300)));
    }

    [Fact]
    public void ResolveAnchorFromObservedCompactBounds_SurvivesAnchorPlacementDivergence()
    {
        // The observed corner is computed from geometry only: a capsule that
        // sits at the panel's top while the recorded anchor says bottom still
        // classifies as top, so a height resize keeps it at the top edge.
        RectInt32 expanded = new(1332, 28, 577, 690);
        RectInt32 compactAtTop = new(1332, 28, 577, 52);

        WidgetCompactExpansionAnchor observed =
            WidgetCompactExpansionCalculator.ResolveAnchorFromObservedCompactBounds(
                compactAtTop,
                expanded);

        Assert.Equal(WidgetCompactExpansionAnchor.LeftTop, observed);
        Assert.NotEqual(
            observed,
            WidgetCompactExpansionCalculator.FromPositionAnchor("RightBottom"));
    }

    [Theory]
    [InlineData(WidgetCompactExpansionAnchor.LeftTop, 1332, 28)]
    [InlineData(WidgetCompactExpansionAnchor.RightTop, 1909 - 200, 28)]
    [InlineData(WidgetCompactExpansionAnchor.LeftBottom, 1332, 728 - 42)]
    [InlineData(WidgetCompactExpansionAnchor.RightBottom, 1909 - 200, 728 - 42)]
    public void Calculate_WithObservedCornerKeepsTheCapsuleOnItsOwnEdgeAfterHeightResize(
        WidgetCompactExpansionAnchor anchor,
        int expectedX,
        int expectedY)
    {
        // Grow the panel from 690 to 700 tall; the capsule must stay pinned
        // to the corner it occupied, not jump to the opposite edge. (Custom
        // widths are still floored at MinWidth=144 by the calculator.)
        RectInt32 resized = new(1332, 28, 577, 700);

        RectInt32 compact = WidgetCompactBoundsCalculator.Calculate(
            resized,
            WidgetCompactExpansionCalculator.ToPositionAnchor(anchor),
            dpiScale: 1.0,
            contentMode: null,
            widgetKind: WidgetKind.File,
            compactWidth: 200.0,
            clampCustomWidth: false,
            heightContentMode: null);

        Assert.Equal(200, compact.Width);
        Assert.Equal(42, compact.Height);
        Assert.Equal(expectedX, compact.X);
        Assert.Equal(expectedY, compact.Y);
    }

    [Fact]
    public void ToPositionAnchor_RoundTripsThroughFromPositionAnchor()
    {
        foreach (WidgetCompactExpansionAnchor anchor in Enum.GetValues(
                     typeof(WidgetCompactExpansionAnchor)))
        {
            Assert.Equal(
                anchor,
                WidgetCompactExpansionCalculator.FromPositionAnchor(
                    WidgetCompactExpansionCalculator.ToPositionAnchor(anchor)));
        }
    }
}
