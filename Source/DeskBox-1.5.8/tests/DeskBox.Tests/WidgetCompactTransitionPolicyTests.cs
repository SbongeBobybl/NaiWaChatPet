using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WidgetCompactTransitionPolicyTests
{
    private const int InteractionReason = (int)WidgetCompactTransitionReason.Interaction;
    private const int BehaviorDisabledReason =
        (int)WidgetCompactTransitionReason.CollapseBehaviorDisabled;

    [Theory]
    [InlineData(WidgetCollapseBehavior.Smart, WidgetCollapseBehavior.Expanded, BehaviorDisabledReason)]
    [InlineData(WidgetCollapseBehavior.Click, WidgetCollapseBehavior.Expanded, BehaviorDisabledReason)]
    [InlineData(WidgetCollapseBehavior.Expanded, WidgetCollapseBehavior.Smart, InteractionReason)]
    [InlineData(WidgetCollapseBehavior.Smart, WidgetCollapseBehavior.Smart, InteractionReason)]
    public void ResolveReason_DistinguishesLeavingCompactBehavior(
        WidgetCollapseBehavior previousBehavior,
        WidgetCollapseBehavior currentBehavior,
        int expected)
    {
        Assert.Equal(
            (WidgetCompactTransitionReason)expected,
            WidgetCompactTransitionPolicy.ResolveReason(previousBehavior, currentBehavior));
    }

    [Theory]
    [InlineData(InteractionReason, true, true, false, true)]
    [InlineData(InteractionReason, true, true, true, false)]
    [InlineData(InteractionReason, false, true, false, false)]
    [InlineData(BehaviorDisabledReason, true, true, false, false)]
    public void CaptureCurrentCompactPlacement_RequiresStableInteractiveExpansion(
        int reason,
        bool wasTargetCollapsed,
        bool compactBoundsWereActive,
        bool transitionWasActive,
        bool expected)
    {
        Assert.Equal(
            expected,
            WidgetCompactTransitionPolicy.ShouldCaptureCurrentCompactPlacement(
                (WidgetCompactTransitionReason)reason,
                wasTargetCollapsed,
                compactBoundsWereActive,
                transitionWasActive));
    }
}
