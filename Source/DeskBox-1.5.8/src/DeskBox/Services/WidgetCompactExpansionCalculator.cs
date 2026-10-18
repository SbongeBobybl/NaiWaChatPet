using Windows.Graphics;

namespace DeskBox.Services;

public enum WidgetCompactExpansionAnchor
{
    LeftTop,
    RightTop,
    LeftBottom,
    RightBottom
}

public readonly record struct WidgetCompactExpansionLayout(
    WidgetCompactExpansionAnchor Anchor,    PointInt32 Pivot,    RectInt32 ExpandedBounds,    SizeInt32 RequestedSize,    bool IsSizeConstrained,    bool CanExpand = true);

public static class WidgetCompactExpansionCalculator
{
    private static readonly WidgetCompactExpansionAnchor[] DefaultAnchorOrder =
    [
        WidgetCompactExpansionAnchor.LeftTop,        WidgetCompactExpansionAnchor.RightTop,        WidgetCompactExpansionAnchor.LeftBottom,        WidgetCompactExpansionAnchor.RightBottom
    ];

    public static WidgetCompactExpansionLayout Resolve(
        RectInt32 compactBounds,        SizeInt32 requestedSize,        RectInt32 workArea,        IReadOnlyList<WidgetCompactExpansionAnchor>? anchorOrder = null,        bool requireFullSize = false)
    {
        int originalRequestedWidth = Math.Max(1,requestedSize.Width);
        int originalRequestedHeight = Math.Max(1,requestedSize.Height);
        int requestedWidth = Math.Min(
            originalRequestedWidth,            Math.Max(1,workArea.Width));
        int requestedHeight = Math.Min(
            originalRequestedHeight,            Math.Max(1,workArea.Height));
        var normalizedRequestedSize = new SizeInt32(requestedWidth,requestedHeight);
        IReadOnlyList<WidgetCompactExpansionAnchor> candidates =
            anchorOrder is { Count: > 0 }
                ? anchorOrder.Distinct().ToArray()
                : DefaultAnchorOrder;

        Candidate? best = null;
        for (int index = 0; index < candidates.Count; index++)
        {
            Candidate candidate = CreateCandidate(
                compactBounds,                normalizedRequestedSize,                workArea,                candidates[index],                index);
            if (best is null || candidate.IsBetterThan(best.Value))
            {
                best = candidate;
            }
        }

        Candidate resolved = best ?? CreateCandidate(
            compactBounds,            normalizedRequestedSize,            workArea,            WidgetCompactExpansionAnchor.LeftTop,            0);
        bool fitsRequestedSize =
            resolved.Bounds.Width == originalRequestedWidth &&
            resolved.Bounds.Height == originalRequestedHeight &&
            IsInsideWorkArea(resolved.Bounds,workArea);
        bool canExpand = !requireFullSize || fitsRequestedSize;
        return new WidgetCompactExpansionLayout(
            resolved.Anchor,            resolved.Pivot,            canExpand ? resolved.Bounds : compactBounds,            new SizeInt32(originalRequestedWidth,originalRequestedHeight),            !fitsRequestedSize,            canExpand);
    }

    public static RectInt32 CreateBoundsFromPivot(
        PointInt32 pivot,        SizeInt32 size,        WidgetCompactExpansionAnchor anchor)
    {
        int width = Math.Max(1,size.Width);
        int height = Math.Max(1,size.Height);
        bool anchorsRight = anchor is
            WidgetCompactExpansionAnchor.RightTop or
            WidgetCompactExpansionAnchor.RightBottom;
        bool anchorsBottom = anchor is
            WidgetCompactExpansionAnchor.LeftBottom or
            WidgetCompactExpansionAnchor.RightBottom;
        return new RectInt32(
            anchorsRight ? pivot.X - width : pivot.X,            anchorsBottom ? pivot.Y - height : pivot.Y,            width,            height);
    }

    public static PointInt32 GetPivot(
        RectInt32 bounds,        WidgetCompactExpansionAnchor anchor)
    {
        bool anchorsRight = anchor is
            WidgetCompactExpansionAnchor.RightTop or
            WidgetCompactExpansionAnchor.RightBottom;
        bool anchorsBottom = anchor is
            WidgetCompactExpansionAnchor.LeftBottom or
            WidgetCompactExpansionAnchor.RightBottom;
        return new PointInt32(
            anchorsRight ? bounds.X + bounds.Width : bounds.X,            anchorsBottom ? bounds.Y + bounds.Height : bounds.Y);
    }

    public static RectInt32 InterpolateAnchoredBounds(
        RectInt32 from,        RectInt32 to,        PointInt32 pivot,        WidgetCompactExpansionAnchor anchor,        double progress)
    {
        double value = Math.Clamp(progress,0,1);
        int width = Math.Max(1,Lerp(from.Width,to.Width,value));
        int height = Math.Max(1,Lerp(from.Height,to.Height,value));
        return CreateBoundsFromPivot(pivot,new SizeInt32(width,height),anchor);
    }

    public static WidgetCompactExpansionAnchor? FromPositionAnchor(string? anchor)
    {
        return anchor switch
        {
            WidgetPositionAnchors.LeftTop => WidgetCompactExpansionAnchor.LeftTop,            WidgetPositionAnchors.RightTop => WidgetCompactExpansionAnchor.RightTop,            WidgetPositionAnchors.LeftBottom => WidgetCompactExpansionAnchor.LeftBottom,            WidgetPositionAnchors.RightBottom => WidgetCompactExpansionAnchor.RightBottom,            _ => null
        };
    }

    public static string ToPositionAnchor(WidgetCompactExpansionAnchor anchor)
    {
        return anchor switch
        {
            WidgetCompactExpansionAnchor.RightTop => WidgetPositionAnchors.RightTop,            WidgetCompactExpansionAnchor.LeftBottom => WidgetPositionAnchors.LeftBottom,            WidgetCompactExpansionAnchor.RightBottom => WidgetPositionAnchors.RightBottom,            _ => WidgetPositionAnchors.LeftTop
        };
    }

    /// <summary>
    /// Classifies which corner of the expanded bounds the capsule actually
    /// occupied, by comparing centers. Ties resolve toward LeftTop so a
    /// centered capsule classifies deterministically. This observes reality
    /// instead of trusting bookkeeping anchors, which may have diverged from
    /// the capsule's true corner.
    /// </summary>
    public static WidgetCompactExpansionAnchor ResolveAnchorFromObservedCompactBounds(
        RectInt32 compactBounds,        RectInt32 expandedBounds)
    {
        bool anchorRight =
            compactBounds.X + Math.Max(1,compactBounds.Width) / 2 >
            expandedBounds.X + Math.Max(1,expandedBounds.Width) / 2;
        bool anchorBottom =
            compactBounds.Y + Math.Max(1,compactBounds.Height) / 2 >
            expandedBounds.Y + Math.Max(1,expandedBounds.Height) / 2;
        return (anchorRight, anchorBottom) switch
        {
            (true, true) => WidgetCompactExpansionAnchor.RightBottom,            (true, false) => WidgetCompactExpansionAnchor.RightTop,            (false, true) => WidgetCompactExpansionAnchor.LeftBottom,            _ => WidgetCompactExpansionAnchor.LeftTop
        };
    }

    public static WidgetCompactExpansionAnchor ResolveHorizontalResizeAnchor(
        WidgetCompactExpansionAnchor currentAnchor,        string? resizeDirection)
    {
        bool anchorsBottom = currentAnchor is
            WidgetCompactExpansionAnchor.LeftBottom or
            WidgetCompactExpansionAnchor.RightBottom;
        return resizeDirection switch
        {
            "Right" => anchorsBottom
                ? WidgetCompactExpansionAnchor.LeftBottom
                : WidgetCompactExpansionAnchor.LeftTop,            "Left" => anchorsBottom
                ? WidgetCompactExpansionAnchor.RightBottom
                : WidgetCompactExpansionAnchor.RightTop,            _ => currentAnchor
        };
    }

    private static Candidate CreateCandidate(
        RectInt32 compactBounds,        SizeInt32 requestedSize,        RectInt32 workArea,        WidgetCompactExpansionAnchor anchor,        int preferenceIndex)
    {
        PointInt32 pivot = GetPivot(compactBounds,anchor);
        int workRight = workArea.X + Math.Max(1,workArea.Width);
        int workBottom = workArea.Y + Math.Max(1,workArea.Height);
        bool anchorsRight = anchor is
            WidgetCompactExpansionAnchor.RightTop or
            WidgetCompactExpansionAnchor.RightBottom;
        bool anchorsBottom = anchor is
            WidgetCompactExpansionAnchor.LeftBottom or
            WidgetCompactExpansionAnchor.RightBottom;
        int availableWidth = anchorsRight
            ? pivot.X - workArea.X
            : workRight - pivot.X;
        int availableHeight = anchorsBottom
            ? pivot.Y - workArea.Y
            : workBottom - pivot.Y;
        int width = Math.Max(1,Math.Min(requestedSize.Width,availableWidth));
        int height = Math.Max(1,Math.Min(requestedSize.Height,availableHeight));
        RectInt32 bounds = CreateBoundsFromPivot(
            pivot,            new SizeInt32(width,height),            anchor);
        bool fitsRequestedSize = width == requestedSize.Width && height == requestedSize.Height;
        long retainedArea = (long)width * height;
        return new Candidate(
            anchor,            pivot,            bounds,            fitsRequestedSize,            retainedArea,            preferenceIndex);
    }

    private static int Lerp(int from,int to,double progress) =>
        (int)Math.Round(from + ((to - from) * progress));

    private static bool IsInsideWorkArea(RectInt32 bounds,RectInt32 workArea)
    {
        long right = (long)bounds.X + bounds.Width;
        long bottom = (long)bounds.Y + bounds.Height;
        long workRight = (long)workArea.X + Math.Max(1,workArea.Width);
        long workBottom = (long)workArea.Y + Math.Max(1,workArea.Height);
        return bounds.X >= workArea.X &&
            bounds.Y >= workArea.Y &&
            right <= workRight &&
            bottom <= workBottom;
    }

    private readonly record struct Candidate(
        WidgetCompactExpansionAnchor Anchor,        PointInt32 Pivot,        RectInt32 Bounds,        bool FitsRequestedSize,        long RetainedArea,        int PreferenceIndex)
    {
        public bool IsBetterThan(Candidate other)
        {
            if (FitsRequestedSize != other.FitsRequestedSize)
            {
                return FitsRequestedSize;
            }

            if (!FitsRequestedSize && RetainedArea != other.RetainedArea)
            {
                return RetainedArea > other.RetainedArea;
            }

            return PreferenceIndex < other.PreferenceIndex;
        }
    }
}
