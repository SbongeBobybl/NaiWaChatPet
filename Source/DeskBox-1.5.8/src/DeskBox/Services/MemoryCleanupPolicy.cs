namespace DeskBox.Services;

internal readonly record struct MemoryCleanupActivitySnapshot(
    bool HasVisibleWidgets,    bool IsWidgetInteractionActive,    bool IsSettingsOpen,    bool IsOnboardingOpen,    bool IsSearchPopupVisible,    bool IsDeskBoxForeground,    bool IsPointerOverDeskBox,    bool IsDesktopOrganizationOpen = false);

[Flags]
internal enum BackgroundMemoryCleanupStage
{
    None = 0,
    SoftCache = 1,
    DeepFinalizerCollection = 2,
    LongHiddenMaintenance = 4
}

internal static class MemoryCleanupPolicy
{
    /// <summary>
    /// Visible-idle trims only fire once the process is genuinely bloated, so
    /// an idle desktop never pays trim/fault churn just to keep a number low.
    /// </summary>
    internal const long VisibleIdleWorkingSetThresholdBytes = 240L * 1024 * 1024;
    internal const long VisibleIdlePrivateBytesThreshold = 260L * 1024 * 1024;

    public static bool IsVisibleIdleCandidate(MemoryCleanupActivitySnapshot snapshot)
    {
        return snapshot.HasVisibleWidgets &&
            !snapshot.IsWidgetInteractionActive &&
            !snapshot.IsSettingsOpen &&
            !snapshot.IsOnboardingOpen &&
            !snapshot.IsSearchPopupVisible &&
            !snapshot.IsDesktopOrganizationOpen &&
            !snapshot.IsDeskBoxForeground &&
            !snapshot.IsPointerOverDeskBox;
    }

    /// <summary>
    /// Restores the pre-1.4.5 visible-idle working-set trim gate: the user is
    /// fully away (idle contract in <see cref="IsVisibleIdleCandidate"/>) and
    /// both footprints exceed the bloat thresholds. Back-faults while anything
    /// is visible would jitter frame pacing, hence the strict presence gates.
    /// While widgets are visible a separate quiescence path
    /// (QuiescenceWorkingSetTrimTracker) may trim behind its own 240 MB
    /// absolute working-set floor plus quiet-tier and blocker gates; this
    /// policy stays the conservative channel for a user who has fully left.
    /// </summary>
    public static bool ShouldTrimVisibleIdleWorkingSet(
        MemoryCleanupActivitySnapshot snapshot,        long workingSetBytes,        long privateBytes)
    {
        return IsVisibleIdleCandidate(snapshot) &&
            workingSetBytes >= VisibleIdleWorkingSetThresholdBytes &&
            privateBytes >= VisibleIdlePrivateBytesThreshold;
    }

    /// <summary>
    /// Deep collection is allowed only after transient UI has gone away and
    /// the user is not currently interacting with DeskBox. Unlike visible-idle
    /// cache trimming, this may run while widgets remain visible, because it
    /// only targets unreachable managed/WinRT graphs and never rebuilds a live
    /// widget view.
    /// </summary>
    public static bool IsDeepCleanupCandidate(
        MemoryCleanupActivitySnapshot snapshot)
    {
        return !snapshot.IsWidgetInteractionActive &&
            !snapshot.IsSettingsOpen &&
            !snapshot.IsOnboardingOpen &&
            !snapshot.IsSearchPopupVisible &&
            !snapshot.IsDesktopOrganizationOpen &&
            !snapshot.IsDeskBoxForeground &&
            !snapshot.IsPointerOverDeskBox;
    }

    public static BackgroundMemoryCleanupStage GetDueBackgroundStages(
        TimeSpan hiddenDuration,        int softDelaySeconds,        int deepDelaySeconds,        BackgroundMemoryCleanupStage completedStages)
    {
        BackgroundMemoryCleanupStage due = BackgroundMemoryCleanupStage.None;
        AddIfDue(
            BackgroundMemoryCleanupStage.SoftCache,            softDelaySeconds,            hiddenDuration,            completedStages,            ref due);
        AddIfDue(
            BackgroundMemoryCleanupStage.DeepFinalizerCollection,            softDelaySeconds,            hiddenDuration,            completedStages,            ref due);
        AddIfDue(
            BackgroundMemoryCleanupStage.LongHiddenMaintenance,            deepDelaySeconds,            hiddenDuration,            completedStages,            ref due);
        return due;
    }

    public static TimeSpan? GetDelayUntilNextBackgroundStage(
        TimeSpan hiddenDuration,        int softDelaySeconds,        int deepDelaySeconds,        BackgroundMemoryCleanupStage completedStages)
    {
        TimeSpan? nextDelay = null;
        ConsiderPendingStage(
            BackgroundMemoryCleanupStage.SoftCache,            softDelaySeconds,            hiddenDuration,            completedStages,            ref nextDelay);
        ConsiderPendingStage(
            BackgroundMemoryCleanupStage.DeepFinalizerCollection,            softDelaySeconds,            hiddenDuration,            completedStages,            ref nextDelay);
        ConsiderPendingStage(
            BackgroundMemoryCleanupStage.LongHiddenMaintenance,            deepDelaySeconds,            hiddenDuration,            completedStages,            ref nextDelay);
        return nextDelay;
    }

    public static TimeSpan GetCappedBackgroundRetryDelay(
        int attemptNumber,        int initialDelaySeconds,        int maximumDelaySeconds)
    {
        ArgumentOutOfRangeException.ThrowIfLessThan(
            initialDelaySeconds,            1);
        ArgumentOutOfRangeException.ThrowIfLessThan(
            maximumDelaySeconds,            initialDelaySeconds);

        int exponent = Math.Clamp(attemptNumber - 1,0,30);
        long scaledDelay = (long)initialDelaySeconds << exponent;
        return TimeSpan.FromSeconds(
            Math.Min(maximumDelaySeconds,scaledDelay));
    }

    private static void AddIfDue(
        BackgroundMemoryCleanupStage stage,        int delaySeconds,        TimeSpan hiddenDuration,        BackgroundMemoryCleanupStage completedStages,        ref BackgroundMemoryCleanupStage due)
    {
        if (delaySeconds == PerformanceSettingsPolicy.CleanupNever ||
            completedStages.HasFlag(stage) ||
            hiddenDuration < TimeSpan.FromSeconds(delaySeconds))
        {
            return;
        }

        due |= stage;
    }

    private static void ConsiderPendingStage(
        BackgroundMemoryCleanupStage stage,        int delaySeconds,        TimeSpan hiddenDuration,        BackgroundMemoryCleanupStage completedStages,        ref TimeSpan? nextDelay)
    {
        if (delaySeconds == PerformanceSettingsPolicy.CleanupNever ||
            completedStages.HasFlag(stage))
        {
            return;
        }

        TimeSpan remaining = TimeSpan.FromSeconds(delaySeconds) - hiddenDuration;
        if (remaining < TimeSpan.Zero)
        {
            remaining = TimeSpan.Zero;
        }

        if (nextDelay is null || remaining < nextDelay.Value)
        {
            nextDelay = remaining;
        }
    }

}
