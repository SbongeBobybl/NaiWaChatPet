namespace DeskBox.Services;

internal readonly record struct WidgetExpandedLayerLease(
    IntPtr WindowHandle,    long Generation)
{
    public bool IsActive => WindowHandle != IntPtr.Zero && Generation > 0;
}

/// <summary>
/// Coordinates the single widget that is allowed to own the expanded peer
/// layer. Generations make delayed collapse callbacks harmless after the
/// pointer has already moved to another capsule.
/// </summary>
internal static class WidgetExpandedLayerLeasePolicy
{
    public static WidgetExpandedLayerLease Acquire(
        WidgetExpandedLayerLease current,        IntPtr windowHandle)
    {
        if (windowHandle == IntPtr.Zero)
        {
            return current;
        }

        long generation = current.Generation == long.MaxValue
            ? 1
            : current.Generation + 1;
        return new WidgetExpandedLayerLease(windowHandle,generation);
    }

    public static bool Owns(
        WidgetExpandedLayerLease current,        IntPtr windowHandle,        long generation)
    {
        return current.IsActive &&
            current.WindowHandle == windowHandle &&
            current.Generation == generation;
    }

    public static WidgetExpandedLayerLease Release(
        WidgetExpandedLayerLease current,        IntPtr windowHandle,        long generation)
    {
        return Owns(current,windowHandle,generation)
            ? new WidgetExpandedLayerLease(IntPtr.Zero,current.Generation)
            : current;
    }
}
