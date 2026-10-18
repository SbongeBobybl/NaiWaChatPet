namespace DeskBox.Services;

public readonly record struct AiLayoutBox(double X,double Y,double Width,double Height)
{
    public bool Overlaps(AiLayoutBox other) => X < other.X + other.Width && X + Width > other.X
        && Y < other.Y + other.Height && Y + Height > other.Y;
}
public readonly record struct AiLayoutScreen(AiLayoutBox WorkArea,double Scale);

/// <summary>
/// Requested corner for one widget. <see cref="Auto"/> keeps the canonical
/// reading order; an explicit corner is honoured only when a free slot exists
/// in that direction, because overlapping is never an accepted fallback.
/// </summary>
public enum AiLayoutAnchor
{
    Auto,
    TopLeft,
    TopRight,
    BottomLeft,
    BottomRight
}

public readonly record struct AiLayoutRequest(AiLayoutAnchor Anchor);

public static class AiLayoutGeometry
{
    // Deterministic physical-pixel placement; no overlap fallback exists.
    // The entire result is staged so failure cannot leave a partially applied layout.
    public static IReadOnlyList<AiLayoutBox> Place(int count,IReadOnlyList<AiLayoutScreen> screens,        IReadOnlyList<AiLayoutBox> occupied)
    {
        if (count < 0 || count > 32) throw new ArgumentOutOfRangeException(nameof(count));
        var requests = new AiLayoutRequest[count];
        for (int i = 0; i < count; i++) requests[i] = new AiLayoutRequest(AiLayoutAnchor.Auto);
        return Place(requests,screens,occupied);
    }

    // Anchored requests are served first from their preferred corner; the
    // remaining requests keep the canonical top-to-bottom, left-to-right order.
    public static IReadOnlyList<AiLayoutBox> Place(IReadOnlyList<AiLayoutRequest> requests,        IReadOnlyList<AiLayoutScreen> screens,IReadOnlyList<AiLayoutBox> occupied)
    {
        if (requests.Count > 32) throw new ArgumentOutOfRangeException(nameof(requests));
        var result = new AiLayoutBox?[requests.Count];
        if (requests.Count == 0) return [];
        var free = CandidateSlots(screens,occupied);
        foreach (int index in Enumerable.Range(0,requests.Count)
                     .OrderBy(index => requests[index].Anchor == AiLayoutAnchor.Auto ? 1 : 0)
                     .ThenBy(index => index))
        {
            int pick = requests[index].Anchor == AiLayoutAnchor.Auto ? 0 : CornerIndex(free,requests[index].Anchor);
            if (pick < 0 || free.Count == 0) continue;
            result[index] = free[pick];
            free.RemoveAt(pick);
        }
        if (result.Any(box => box is null))
            throw new InvalidOperationException("屏幕空间不足，请减少分类或先收起现有格子；本次不会创建重叠格子。");
        return result.Select(box => box!.Value).ToArray();
    }

    private static List<AiLayoutBox> CandidateSlots(IReadOnlyList<AiLayoutScreen> screens,        IReadOnlyList<AiLayoutBox> occupied)
    {
        const int maxCandidates = 512;
        var candidates = new List<AiLayoutBox>();
        foreach (var screen in screens)
        {
            var area = screen.WorkArea;
            if (!double.IsFinite(screen.Scale) || screen.Scale <= 0 || area.Width <= 0 || area.Height <= 0) continue;
            double scale = screen.Scale, gap = 14 * scale, width = 240 * scale, height = 180 * scale;
            for (double y = area.Y + gap; y + height <= area.Y + area.Height - gap; y += height + gap)
            for (double x = area.X + gap; x + width <= area.X + area.Width - gap; x += width + gap)
            {
                var slot = new AiLayoutBox(x,y,width,height);
                var spacing = new AiLayoutBox(x - gap / 2,y - gap / 2,width + gap,height + gap);
                if (occupied.Any(spacing.Overlaps) || candidates.Any(spacing.Overlaps)) continue;
                candidates.Add(slot);
                if (candidates.Count == maxCandidates) return candidates;
            }
        }
        return candidates;
    }

    private static int CornerIndex(List<AiLayoutBox> free,AiLayoutAnchor anchor)
    {
        if (free.Count == 0) return -1;
        int best = 0;
        for (int i = 1; i < free.Count; i++)
            if (IsBetter(free[i],free[best],anchor)) best = i;
        return best;
    }

    private static bool IsBetter(AiLayoutBox candidate,AiLayoutBox current,AiLayoutAnchor anchor)
    {
        // Reading order puts smaller coordinates first; a corner only changes
        // which axis is reversed. Ties fall back to the canonical order.
        double candidatePrimary = anchor is AiLayoutAnchor.BottomLeft or AiLayoutAnchor.BottomRight ? -candidate.Y : candidate.Y;
        double currentPrimary = anchor is AiLayoutAnchor.BottomLeft or AiLayoutAnchor.BottomRight ? -current.Y : current.Y;
        if (candidatePrimary != currentPrimary) return candidatePrimary < currentPrimary;
        double candidateSecondary = anchor is AiLayoutAnchor.TopRight or AiLayoutAnchor.BottomRight ? -candidate.X : candidate.X;
        double currentSecondary = anchor is AiLayoutAnchor.TopRight or AiLayoutAnchor.BottomRight ? -current.X : current.X;
        return candidateSecondary < currentSecondary;
    }
}
