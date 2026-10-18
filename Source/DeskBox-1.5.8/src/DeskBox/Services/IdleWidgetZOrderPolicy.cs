namespace DeskBox.Services;

internal readonly record struct IdleWidgetZOrderCandidate(
    IntPtr WindowHandle,    string DisplayKey,    double Top,    double Left,    string StableKey);

/// <summary>
/// Defines the canonical peer order used while widgets are visible but idle.
/// Lower rows are kept above upper rows so an upper widget's downward shadow
/// cannot darken the top edge of the widget below it.
/// </summary>
internal static class IdleWidgetZOrderPolicy
{
    public static IReadOnlyList<IdleWidgetZOrderCandidate> OrderHighestToLowest(
        IEnumerable<IdleWidgetZOrderCandidate> candidates)
    {
        return candidates
            .Where(candidate => candidate.WindowHandle != IntPtr.Zero)
            .GroupBy(candidate => candidate.WindowHandle)
            .Select(group => group.First())
            .OrderBy(candidate => candidate.DisplayKey,StringComparer.Ordinal)
            .ThenByDescending(candidate => candidate.Top)
            .ThenByDescending(candidate => candidate.Left)
            .ThenBy(candidate => candidate.StableKey,StringComparer.Ordinal)
            .ToList();
    }

    /// <summary>
    /// The idle peer order only exists to keep an upper widget's drop shadow
    /// off the widget below it. When Windows drop shadows are disabled the
    /// normalization has no visual purpose and every applied reorder is pure
    /// repaint churn.
    /// </summary>
    public static bool ShouldNormalizeIdlePeerOrder(bool windowDropShadowEnabled) =>
        windowDropShadowEnabled;

    /// <summary>
    /// Reports whether the HWND z-order already matches the requested peer
    /// order: no requested peer may sit above the requested highest handle,
    /// and walking down from it must encounter the requested peers in order.
    /// </summary>
    public static bool MatchesRequestedOrder(
        IReadOnlyList<IntPtr> requestedHighestToLowest,        IReadOnlyList<IntPtr> peersObservedAboveHighest,        IReadOnlyList<IntPtr> peersObservedFromHighestDown)
    {
        return peersObservedAboveHighest.Count == 0 &&
               peersObservedFromHighestDown.SequenceEqual(requestedHighestToLowest);
    }
}
