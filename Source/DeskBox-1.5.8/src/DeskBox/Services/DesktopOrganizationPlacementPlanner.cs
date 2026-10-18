using DeskBox.Models;

namespace DeskBox.Services;

public sealed class DesktopOrganizationPlacementPlanner
{
    public const double DefaultEdgeMargin = 16;
    public const double DefaultGap = 12;

    public bool TryAssignBounds(
        DesktopOrganizationPlan plan,        DesktopOrganizationRect workArea,        IReadOnlyCollection<DesktopOrganizationRect> occupiedBounds,        double widgetWidth,        double widgetHeight,        double edgeMargin = DefaultEdgeMargin,        double gap = DefaultGap)
    {
        var occupied = occupiedBounds.ToList();
        foreach (DesktopOrganizationTargetPlan target in plan.Targets.Where(target => target.CreatesWidget))
        {
            DesktopOrganizationRect? available = FindNextAvailable(
                workArea,                occupied,                widgetWidth,                widgetHeight,                edgeMargin,                gap);
            // Prefer a clear position, but do not block organization merely
            // because the desktop is already full. The fallback chooses the
            // candidate with the smallest overlap and keeps it on-screen.
            available ??= FindBestEffortPosition(
                workArea,                occupied,                widgetWidth,                widgetHeight,                edgeMargin,                gap);
            if (available is null)
            {
                foreach (DesktopOrganizationTargetPlan planned in plan.Targets)
                {
                    planned.PlannedBounds = null;
                }

                return false;
            }

            target.PlannedBounds = available;
            occupied.Add(available.Value);
        }

        return true;
    }

    private static DesktopOrganizationRect? FindNextAvailable(
        DesktopOrganizationRect workArea,        IReadOnlyCollection<DesktopOrganizationRect> occupied,        double width,        double height,        double edgeMargin,        double gap)
    {
        double top = workArea.Y + edgeMargin;
        double bottom = workArea.Bottom - edgeMargin;
        double right = workArea.Right - edgeMargin;
        for (double x = right - width;
             x >= workArea.X + edgeMargin;
             x -= width + gap)
        {
            for (double y = top;
                 y + height <= bottom;
                 y += height + gap)
            {
                var candidate = new DesktopOrganizationRect(x,y,width,height);
                if (!occupied.Any(bounds => candidate.Intersects(bounds)))
                {
                    return candidate;
                }
            }
        }

        return null;
    }

    private static DesktopOrganizationRect? FindBestEffortPosition(
        DesktopOrganizationRect workArea,        IReadOnlyCollection<DesktopOrganizationRect> occupied,        double width,        double height,        double edgeMargin,        double gap)
    {
        if (!double.IsFinite(width) || !double.IsFinite(height) || width <= 0 || height <= 0)
        {
            return null;
        }

        double minimumX = workArea.X + edgeMargin;
        double maximumX = Math.Max(minimumX,workArea.Right - edgeMargin - width);
        double minimumY = workArea.Y + edgeMargin;
        double maximumY = Math.Max(minimumY,workArea.Bottom - edgeMargin - height);
        DesktopOrganizationRect? best = null;
        double bestOverlap = double.PositiveInfinity;
        double bestDistanceFromPreferred = double.PositiveInfinity;
        double preferredX = maximumX;
        double preferredY = minimumY;

        for (double x = maximumX; x >= minimumX; x -= Math.Max(1,width + gap))
        {
            for (double y = minimumY; y <= maximumY; y += Math.Max(1,height + gap))
            {
                var candidate = new DesktopOrganizationRect(x,y,width,height);
                double overlap = occupied.Sum(bounds => GetIntersectionArea(candidate,bounds));
                double distanceFromPreferred =
                    Math.Abs(candidate.X - preferredX) +
                    Math.Abs(candidate.Y - preferredY);

                if (overlap < bestOverlap ||
                    overlap.Equals(bestOverlap) && distanceFromPreferred < bestDistanceFromPreferred)
                {
                    best = candidate;
                    bestOverlap = overlap;
                    bestDistanceFromPreferred = distanceFromPreferred;
                }
            }
        }

        return best ?? new DesktopOrganizationRect(preferredX,preferredY,width,height);
    }

    private static double GetIntersectionArea(
        DesktopOrganizationRect first,        DesktopOrganizationRect second)
    {
        double width = Math.Max(0,Math.Min(first.Right,second.Right) - Math.Max(first.X,second.X));
        double height = Math.Max(0,Math.Min(first.Bottom,second.Bottom) - Math.Max(first.Y,second.Y));
        return width * height;
    }
}
