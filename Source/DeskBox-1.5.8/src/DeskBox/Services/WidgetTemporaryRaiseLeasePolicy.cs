namespace DeskBox.Services;

internal readonly record struct WidgetTemporaryRaiseLease(
    IReadOnlyList<IntPtr>? WindowHandles,    long Generation)
{
    public IReadOnlyList<IntPtr> ActiveWindowHandles =>
        WindowHandles ?? Array.Empty<IntPtr>();

    public bool IsActive => ActiveWindowHandles.Count > 0 && Generation > 0;
}

/// <summary>
/// Tracks windows detached from the Explorer desktop owner for a temporary
/// manager-initiated raise. Generations keep delayed callbacks from releasing
/// a newer interaction.
/// </summary>
internal static class WidgetTemporaryRaiseLeasePolicy
{
    public static bool CanRestoreDesktopLayer(
        bool isVisible,        bool isHideAnimationRunning,        bool isClosing)
    {
        return isVisible && !isHideAnimationRunning && !isClosing;
    }

    public static bool ShouldArmSafetyRestore(bool isAtDesktopLayer)
    {
        return !isAtDesktopLayer;
    }

    public static bool ShouldDeferSafetyRestore(
        bool isDragging,        bool isResizing,        bool hasBlockingFlyout,        bool isManagerInteractionActive)
    {
        return isDragging ||
            isResizing ||
            hasBlockingFlyout ||
            isManagerInteractionActive;
    }

    public static WidgetTemporaryRaiseLease Acquire(
        WidgetTemporaryRaiseLease current,        IEnumerable<IntPtr> windowHandles)
    {
        List<IntPtr> handles = current.ActiveWindowHandles
            .Concat(windowHandles)
            .Where(handle => handle != IntPtr.Zero)
            .Distinct()
            .ToList();
        if (handles.Count == 0)
        {
            return current;
        }

        long generation = current.Generation == long.MaxValue
            ? 1
            : current.Generation + 1;
        return new WidgetTemporaryRaiseLease(handles,generation);
    }

    public static bool OwnsGeneration(
        WidgetTemporaryRaiseLease current,        long generation)
    {
        return current.IsActive && current.Generation == generation;
    }

    public static WidgetTemporaryRaiseLease Release(
        WidgetTemporaryRaiseLease current,        long generation)
    {
        return OwnsGeneration(current,generation)
            ? new WidgetTemporaryRaiseLease([],current.Generation)
            : current;
    }

    public static WidgetTemporaryRaiseLease Forget(
        WidgetTemporaryRaiseLease current,        IntPtr windowHandle)
    {
        if (!current.IsActive || windowHandle == IntPtr.Zero)
        {
            return current;
        }

        List<IntPtr> remaining = current.ActiveWindowHandles
            .Where(handle => handle != windowHandle)
            .ToList();
        return new WidgetTemporaryRaiseLease(remaining,current.Generation);
    }
}
