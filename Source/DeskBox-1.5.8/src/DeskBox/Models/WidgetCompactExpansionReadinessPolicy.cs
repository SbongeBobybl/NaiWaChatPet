namespace DeskBox.Models;

public enum WidgetCompactExpansionReadinessDecision
{
    ExpandNow,
    WaitForWarmup,
    ExpandWithLiveLayoutFallback
}

/// <summary>
/// Readiness may improve first-frame smoothness, but can never become a hard
/// functional gate. Every deferred request therefore has a fixed deadline.
/// </summary>
public static class WidgetCompactExpansionReadinessPolicy
{
    public const int DefaultDeadlineMilliseconds = 96;

    /// <summary>
    /// Hard ceiling for the total hold when the warm-up is still actively
    /// running at the default deadline. The capsule has not moved yet, so a
    /// bounded extra wait is invisible next to a live-layout fallback that
    /// stutters the expansion's opening frames.
    /// </summary>
    public const int ExtendedDeadlineMilliseconds = 320;

    public static WidgetCompactExpansionReadinessDecision Decide(
        bool isReady,        bool deadlineElapsed)
    {
        if (isReady)
        {
            return WidgetCompactExpansionReadinessDecision.ExpandNow;
        }

        return deadlineElapsed
            ? WidgetCompactExpansionReadinessDecision.ExpandWithLiveLayoutFallback
            : WidgetCompactExpansionReadinessDecision.WaitForWarmup;
    }

    /// <summary>
    /// Extra milliseconds the deferral may wait past the default deadline.
    /// Only a warm-up that is both alive and currently allowed to run can
    /// finish; a blocked one must fall back immediately instead of stalling
    /// the capsule for the full extension.
    /// </summary>
    public static int ResolveDeadlineExtensionMilliseconds(
        bool warmupActive,        bool warmupCanRunNow)
    {
        return warmupActive && warmupCanRunNow
            ? ExtendedDeadlineMilliseconds - DefaultDeadlineMilliseconds
            : 0;
    }
}
