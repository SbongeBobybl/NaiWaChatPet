namespace DeskBox.Services;

internal static class TrayAnimationInterruptionCoordinator
{
    public static int CancelAndRestore<T>(
        IEnumerable<T> windows,        Action<T> cancelAndRestore,        Action<T, Exception>? onFailure = null)
    {
        ArgumentNullException.ThrowIfNull(windows);
        ArgumentNullException.ThrowIfNull(cancelAndRestore);

        int restoredCount = 0;
        foreach (T window in windows)
        {
            try
            {
                cancelAndRestore(window);
                restoredCount++;
            }
            catch (Exception ex)
            {
                onFailure?.Invoke(window,ex);
            }
        }

        return restoredCount;
    }
}
