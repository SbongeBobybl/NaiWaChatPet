namespace DeskBox.Models;

internal enum QuickLookNavigationDirection
{
    Left,
    Up,
    Right,
    Down
}

internal sealed record QuickLookSurfaceNavigationSnapshot(
    string SurfaceId,    double X,    double Y,    double Width,    double Height,    IReadOnlyList<string> Paths);

internal readonly record struct QuickLookNavigationTarget(
    string SurfaceId,    string Path);

/// <summary>
/// Resolves the next file surface when native selector navigation reaches a
/// surface boundary. Navigation within one GridView/ListView remains native.
/// </summary>
internal static class QuickLookNavigationPolicy
{
    private const double DirectionEpsilon = 0.5;

    internal static QuickLookNavigationTarget? ResolveAdjacentSurface(
        IReadOnlyList<QuickLookSurfaceNavigationSnapshot> surfaces,        string currentSurfaceId,        QuickLookNavigationDirection direction)
    {
        QuickLookSurfaceNavigationSnapshot? current = surfaces
            .FirstOrDefault(surface => string.Equals(
                surface.SurfaceId,                currentSurfaceId,                StringComparison.Ordinal));
        if (current is null)
        {
            return null;
        }

        double currentCenterX = current.X + (current.Width / 2);
        double currentCenterY = current.Y + (current.Height / 2);
        QuickLookSurfaceNavigationSnapshot? adjacent = surfaces
            .Where(surface =>
                !string.Equals(
                    surface.SurfaceId,                    currentSurfaceId,                    StringComparison.Ordinal) &&
                surface.Paths.Count > 0)
            .Select(surface => CreateCandidate(
                current,                currentCenterX,                currentCenterY,                surface,                direction))
            .Where(candidate => candidate is not null)
            .Select(candidate => candidate!.Value)
            .OrderBy(candidate => candidate.PerpendicularGap > 0 ? 1 : 0)
            .ThenBy(candidate => candidate.DirectionRatio)
            .ThenBy(candidate => candidate.PrimaryDistance)
            .ThenBy(candidate => candidate.PerpendicularDistance)
            .ThenBy(candidate => candidate.Surface.SurfaceId,StringComparer.Ordinal)
            .Select(candidate => candidate.Surface)
            .FirstOrDefault();
        if (adjacent is null)
        {
            return null;
        }

        string path = direction is QuickLookNavigationDirection.Left or
            QuickLookNavigationDirection.Up
                ? adjacent.Paths[^1]
                : adjacent.Paths[0];
        return new QuickLookNavigationTarget(adjacent.SurfaceId,path);
    }

    private static Candidate? CreateCandidate(
        QuickLookSurfaceNavigationSnapshot current,        double currentCenterX,        double currentCenterY,        QuickLookSurfaceNavigationSnapshot candidate,        QuickLookNavigationDirection direction)
    {
        double candidateCenterX = candidate.X + (candidate.Width / 2);
        double candidateCenterY = candidate.Y + (candidate.Height / 2);
        double deltaX = candidateCenterX - currentCenterX;
        double deltaY = candidateCenterY - currentCenterY;
        bool isInDirection = direction switch
        {
            QuickLookNavigationDirection.Left => deltaX < -DirectionEpsilon,            QuickLookNavigationDirection.Up => deltaY < -DirectionEpsilon,            QuickLookNavigationDirection.Right => deltaX > DirectionEpsilon,            QuickLookNavigationDirection.Down => deltaY > DirectionEpsilon,            _ => false
        };
        if (!isInDirection)
        {
            return null;
        }

        bool horizontal = direction is QuickLookNavigationDirection.Left or
            QuickLookNavigationDirection.Right;
        double primaryDistance = horizontal
            ? Math.Abs(deltaX)
            : Math.Abs(deltaY);
        double perpendicularDistance = horizontal
            ? Math.Abs(deltaY)
            : Math.Abs(deltaX);
        double perpendicularGap = horizontal
            ? IntervalGap(
                current.Y,                current.Y + current.Height,                candidate.Y,                candidate.Y + candidate.Height)
            : IntervalGap(
                current.X,                current.X + current.Width,                candidate.X,                candidate.X + candidate.Width);

        return new Candidate(
            candidate,            primaryDistance,            perpendicularDistance,            perpendicularGap,            perpendicularDistance / Math.Max(DirectionEpsilon,primaryDistance));
    }

    private static double IntervalGap(
        double firstStart,        double firstEnd,        double secondStart,        double secondEnd)
    {
        if (firstEnd < secondStart)
        {
            return secondStart - firstEnd;
        }

        return secondEnd < firstStart
            ? firstStart - secondEnd
            : 0;
    }

    private readonly record struct Candidate(
        QuickLookSurfaceNavigationSnapshot Surface,        double PrimaryDistance,        double PerpendicularDistance,        double PerpendicularGap,        double DirectionRatio);
}
