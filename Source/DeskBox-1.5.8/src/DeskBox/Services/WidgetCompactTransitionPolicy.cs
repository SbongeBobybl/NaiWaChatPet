namespace DeskBox.Services;

internal enum WidgetCompactTransitionReason
{
    Interaction,
    CollapseBehaviorDisabled
}

internal static class WidgetCompactTransitionPolicy
{
    public static WidgetCompactTransitionReason ResolveReason(
        WidgetCollapseBehavior previousBehavior,        WidgetCollapseBehavior currentBehavior)
    {
        return previousBehavior != WidgetCollapseBehavior.Expanded &&
            currentBehavior == WidgetCollapseBehavior.Expanded
                ? WidgetCompactTransitionReason.CollapseBehaviorDisabled
                : WidgetCompactTransitionReason.Interaction;
    }

    public static bool ShouldCaptureCurrentCompactPlacement(
        WidgetCompactTransitionReason reason,        bool wasTargetCollapsed,        bool compactBoundsWereActive,        bool transitionWasActive)
    {
        return reason == WidgetCompactTransitionReason.Interaction &&
            wasTargetCollapsed &&
            compactBoundsWereActive &&
            !transitionWasActive;
    }
}
