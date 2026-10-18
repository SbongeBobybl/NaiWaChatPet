namespace DeskBox.Models;

public enum WidgetCompactLayerRestoreDecision
{
    WaitForFrameCommit,
    Restore,
    Cancel
}

/// <summary>
/// Keeps a hover-expanded widget above its peers until the compositor has
/// presented the final collapsed surface. Lowering the native window before
/// that commit lets sibling widgets cut through the last animation frames.
/// </summary>
public static class WidgetCompactLayerRestorePolicy
{
    public const int RequiredCommittedFrames = 2;

    public static WidgetCompactLayerRestoreDecision Decide(
        bool isClosing,        bool collapseInitialized,        bool targetCollapsed,        bool transitionActive,        long activeGeneration,        long restoreGeneration,        int committedFrames,        bool deadlineElapsed)
    {
        if (isClosing ||
            !collapseInitialized ||
            !targetCollapsed ||
            activeGeneration != restoreGeneration)
        {
            return WidgetCompactLayerRestoreDecision.Cancel;
        }

        if (transitionActive)
        {
            return WidgetCompactLayerRestoreDecision.WaitForFrameCommit;
        }

        return committedFrames >= RequiredCommittedFrames || deadlineElapsed
            ? WidgetCompactLayerRestoreDecision.Restore
            : WidgetCompactLayerRestoreDecision.WaitForFrameCommit;
    }
}
