namespace DeskBox.Tests;

/// <summary>
/// Pins the resize-end capsule recapture wiring: the capsule keeps the corner
/// it actually occupied when the resize began (observed geometry first), and
/// Config.PositionAnchor — the monitor-edge home for topology restore — is
/// never used to re-derive the capsule corner. A stale Bottom half in the
/// position anchor once pinned capsules to the panel's bottom edge after
/// height resizes.
/// </summary>
public sealed class WidgetCompactResizePlacementContractTests
{
    [Fact]
    public void RecapturePrefersObservedGeometryOverBookkeepingAnchors()
    {
        string collapse = ReadRepositoryFile(
            "src/DeskBox/Views/WidgetWindowBase.Collapse.cs");

        // Observation first: the interaction-start pair captured by
        // CaptureExpandedPairInteraction (Begin) and still valid at the
        // resize commit (cleared only at EndWidgetBoundsInteraction).
        Assert.Contains(
            "ResolveObservedCompactAnchor() ??",
            collapse,
            StringComparison.Ordinal);
        Assert.Contains(
            "_expandedInteractionStartBounds is not { } expandedStart ||",
            collapse,
            StringComparison.Ordinal);
        Assert.Contains(
            "_compactInteractionStartBounds is not { } compactStart",
            collapse,
            StringComparison.Ordinal);
        // Fallback order: expansion anchor bookkeeping, then the position
        // anchor as a last resort only.
        Assert.Contains(
            "_compactExpansionAnchor ??",
            collapse,
            StringComparison.Ordinal);
        // The recapture must feed the resolved corner through the anchor
        // mapping instead of passing Config.PositionAnchor straight into
        // the calculator.
        Assert.Contains(
            "WidgetCompactExpansionCalculator.ToPositionAnchor(anchor),",
            collapse,
            StringComparison.Ordinal);
        // The expansion anchor stays in sync with the observed corner so
        // the next expand/collapse cycle is consistent.
        Assert.Contains(
            "_compactExpansionAnchor = anchor;",
            collapse,
            StringComparison.Ordinal);
    }

    [Fact]
    public void ObservedCornerClassificationLivesInThePureCalculator()
    {
        string calculator = ReadRepositoryFile(
            "src/DeskBox/Services/WidgetCompactExpansionCalculator.cs");

        Assert.Contains(
            "public static WidgetCompactExpansionAnchor ResolveAnchorFromObservedCompactBounds(",
            calculator,
            StringComparison.Ordinal);
        Assert.Contains(
            "public static string ToPositionAnchor(WidgetCompactExpansionAnchor anchor)",
            calculator,
            StringComparison.Ordinal);
    }

    private static string ReadRepositoryFile(string relativePath) =>
        File.ReadAllText(TestPaths.FromRepository(relativePath));
}
