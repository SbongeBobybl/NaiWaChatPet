using Windows.Graphics;

namespace DeskBox.Services;

internal static class WidgetCoordinatedMoveCalculator
{
    public static RectInt32 GetUnion(IEnumerable<RectInt32> bounds)
    {
        using IEnumerator<RectInt32> enumerator = bounds.GetEnumerator();
        if (!enumerator.MoveNext())
        {
            return default;
        }

        RectInt32 first = enumerator.Current;
        int left = first.X;
        int top = first.Y;
        int right = first.X + first.Width;
        int bottom = first.Y + first.Height;
        while (enumerator.MoveNext())
        {
            RectInt32 current = enumerator.Current;
            left = Math.Min(left,current.X);
            top = Math.Min(top,current.Y);
            right = Math.Max(right,current.X + current.Width);
            bottom = Math.Max(bottom,current.Y + current.Height);
        }

        return new RectInt32(
            left,            top,            Math.Max(1,right - left),            Math.Max(1,bottom - top));
    }

    public static PointInt32 ClampDelta(
        RectInt32 groupBounds,        PointInt32 requestedDelta,        RectInt32 workArea)
    {
        return new PointInt32(
            ClampAxis(
                groupBounds.X,                groupBounds.Width,                requestedDelta.X,                workArea.X,                workArea.Width),            ClampAxis(
                groupBounds.Y,                groupBounds.Height,                requestedDelta.Y,                workArea.Y,                workArea.Height));
    }

    private static int ClampAxis(
        int groupStart,        int groupSize,        int requestedDelta,        int workAreaStart,        int workAreaSize)
    {
        if (groupSize > workAreaSize || workAreaSize <= 0)
        {
            return requestedDelta;
        }

        int minimum = workAreaStart - groupStart;
        int maximum = workAreaStart + workAreaSize - (groupStart + groupSize);

        // Existing off-screen placement is tolerated, but a coordinated move
        // may not push the group farther out. This also avoids a jump at the
        // first pointer frame when the saved layout already straddles an edge.
        minimum = Math.Min(0,minimum);
        maximum = Math.Max(0,maximum);
        return Math.Clamp(requestedDelta,minimum,maximum);
    }
}
