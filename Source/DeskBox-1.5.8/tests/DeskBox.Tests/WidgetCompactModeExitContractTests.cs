namespace DeskBox.Tests;

public sealed class WidgetCompactModeExitContractTests
{
    [Fact]
    public void CollapseBehaviorDisabled_RestoresPersistedExpandedBounds()
    {
        string source = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Views/WidgetWindowBase.Collapse.cs"));
        string applyBehavior = Slice(
            source,
            "private void ApplyEffectiveCollapseBehavior",
            "private void SynchronizeCompactPointerStateForSmartEntry");
        string transition = Slice(
            source,
            "private void SetCollapsedState(",
            "private void DeferCompactExpansionUntilReady");

        Assert.Contains("WidgetCompactTransitionPolicy.ResolveReason", applyBehavior, StringComparison.Ordinal);
        Assert.Contains("transitionReason: transitionReason", applyBehavior, StringComparison.Ordinal);
        Assert.Contains("CollapseBehaviorDisabled", transition, StringComparison.Ordinal);
        Assert.Contains("ResolvePersistedExpandedHostBounds", transition, StringComparison.Ordinal);
        Assert.Contains("ResetCompactGeometryForExpandedMode", transition, StringComparison.Ordinal);
        Assert.Contains("bypassExpansionReadiness = true", transition, StringComparison.Ordinal);
    }

    [Fact]
    public void CapsuleBarCandidateRemoval_ClearsWindowConstraint()
    {
        string source = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetManager.CapsuleArrangement.cs"));
        string apply = Slice(
            source,
            "private void ApplyCapsuleArrangementIfChanged",
            "private void ClearRetiredCapsuleArrangementConstraints");
        string clear = Slice(
            source,
            "private void ClearRetiredCapsuleArrangementConstraints",
            "internal bool BeginCapsuleBarDrag");

        Assert.Contains("previouslyConstrainedIds", apply, StringComparison.Ordinal);
        Assert.Contains("ClearRetiredCapsuleArrangementConstraints", apply, StringComparison.Ordinal);
        Assert.Contains("ClearCompactArrangementConstraint", clear, StringComparison.Ordinal);
    }

    [Fact]
    public void CollapseTransition_SuppressesHoverExpansionUntilPointerExit()
    {
        string source = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Views/WidgetWindowBase.Collapse.cs"));
        string transition = Slice(
            source,
            "private void SetCollapsedState(",
            "private RectInt32 ResolvePersistedExpandedHostBounds()");

        int guard = transition.IndexOf(
            "if (collapsed && !_targetCollapsed)",
            StringComparison.Ordinal);
        int suppress = transition.IndexOf(
            "_suppressSmartExpansionUntilPointerExit = true;",
            guard,
            StringComparison.Ordinal);
        int cancelHover = transition.IndexOf(
            "CancelTimer(ref _collapseHoverTimer);",
            guard,
            StringComparison.Ordinal);
        int targetChange = transition.IndexOf(
            "_targetCollapsed = collapsed;",
            StringComparison.Ordinal);

        Assert.True(guard >= 0);
        Assert.InRange(suppress, guard + 1, targetChange - 1);
        Assert.InRange(cancelHover, guard + 1, targetChange - 1);
    }

    private static string Slice(string source, string startMarker, string endMarker)
    {
        int start = source.IndexOf(startMarker, StringComparison.Ordinal);
        int end = source.IndexOf(endMarker, start, StringComparison.Ordinal);
        Assert.True(start >= 0, $"Missing start marker: {startMarker}");
        Assert.True(end > start, $"Missing end marker: {endMarker}");
        return source[start..end];
    }
}
