using DeskBox.Services;
using Windows.Graphics;

namespace DeskBox.Tests;

public sealed class WidgetCoordinatedMoveCalculatorTests
{
    [Fact]
    public void GetUnion_ContainsEveryWidget()
    {
        RectInt32 result = WidgetCoordinatedMoveCalculator.GetUnion(
            [
                new RectInt32(100, 80, 200, 120),
                new RectInt32(350, 40, 90, 260),
                new RectInt32(20, 170, 60, 40)
            ]);

        Assert.Equal(20, result.X);
        Assert.Equal(40, result.Y);
        Assert.Equal(420, result.Width);
        Assert.Equal(260, result.Height);
    }

    [Fact]
    public void ClampDelta_ConstrainsCompleteGroupToWorkArea()
    {
        var group = new RectInt32(100, 100, 300, 200);
        var workArea = new RectInt32(0, 0, 500, 400);

        PointInt32 positive = WidgetCoordinatedMoveCalculator.ClampDelta(
            group,
            new PointInt32(200, 200),
            workArea);
        PointInt32 negative = WidgetCoordinatedMoveCalculator.ClampDelta(
            group,
            new PointInt32(-200, -200),
            workArea);

        Assert.Equal(100, positive.X);
        Assert.Equal(100, positive.Y);
        Assert.Equal(-100, negative.X);
        Assert.Equal(-100, negative.Y);
    }

    [Fact]
    public void ClampDelta_DoesNotWorsenExistingOffscreenPlacement()
    {
        var group = new RectInt32(-20, 50, 300, 200);
        var workArea = new RectInt32(0, 0, 500, 400);

        PointInt32 fartherOut = WidgetCoordinatedMoveCalculator.ClampDelta(
            group,
            new PointInt32(-10, 0),
            workArea);
        PointInt32 towardWorkArea = WidgetCoordinatedMoveCalculator.ClampDelta(
            group,
            new PointInt32(10, 0),
            workArea);

        Assert.Equal(0, fartherOut.X);
        Assert.Equal(10, towardWorkArea.X);
    }

    [Fact]
    public void ClampDelta_OversizedGroupPreservesRequestedMovement()
    {
        PointInt32 result = WidgetCoordinatedMoveCalculator.ClampDelta(
            new RectInt32(-50, 10, 700, 300),
            new PointInt32(17, -9),
            new RectInt32(0, 0, 500, 400));

        Assert.Equal(17, result.X);
        Assert.Equal(-9, result.Y);
    }
}
