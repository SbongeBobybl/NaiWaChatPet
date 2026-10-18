namespace DeskBox.Services;

internal static class FileSurfaceRefreshPolicy
{
    internal static readonly TimeSpan ReconciliationFreshnessWindow =
        TimeSpan.FromSeconds(30);

    public static bool ShouldReconcile(
        DateTime utcNow,        DateTime lastReconciliationUtc,        bool hasDeferredChanges)
    {
        return hasDeferredChanges ||
            lastReconciliationUtc == DateTime.MinValue ||
            utcNow - lastReconciliationUtc >= ReconciliationFreshnessWindow;
    }
}
