namespace DeskBox.Tests;

/// <summary>
/// Source contract for the bulk-sync cut-state storm. The 2026-09-17 hang
/// dump captured the UI thread in SyncFolderItems' per-item RemoveAt loop
/// while a cut was pending: every one of the thousands of collection events
/// ran ApplyCutState - an O(items) sweep plus a realized-container visual
/// pass of native lookups and visual-state applications - producing a
/// single-core spin and runaway native XAML memory. The per-event handler
/// must keep only the cheap departed-path pruning and coalesce the sweep to
/// one dispatcher pass, re-checking the clipboard at execution time.
/// </summary>
public sealed class CutStateBulkSyncContractTests
{
    [Fact]
    public void ReconcileAfterItemsChanged_NeverRunsTheSweepPerEvent()
    {
        string surface = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.xaml.cs"))
            .Replace("\r\n", "\n");

        int reconcile = surface.IndexOf(
            "private void ReconcileCutStateAfterItemsChanged(",
            StringComparison.Ordinal);
        Assert.True(reconcile > 0, "the reconcile handler must exist");

        // The handler body ends before the next member; cheap pruning stays
        // synchronous, but the sweep itself must route through the queue.
        int bodyEnd = surface.IndexOf(
            "private void QueueCutStateReconciliation()",
            reconcile,
            StringComparison.Ordinal);
        Assert.True(bodyEnd > reconcile, "the queued reconciler must follow the handler");
        string body = surface[reconcile..bodyEnd];
        Assert.Contains(
            "FileCutStatePolicy.RemoveDepartedPaths(",
            body,
            StringComparison.Ordinal);
        Assert.DoesNotContain(
            "ApplyCutState();",
            body,
            StringComparison.Ordinal);
        Assert.Contains(
            "QueueCutStateReconciliation();",
            body,
            StringComparison.Ordinal);
    }

    [Fact]
    public void QueuedReconciler_CoalescesAndRechecksTheClipboard()
    {
        string surface = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.xaml.cs"))
            .Replace("\r\n", "\n");

        int queue = surface.IndexOf(
            "private void QueueCutStateReconciliation()",
            StringComparison.Ordinal);
        int queuedFlag = surface.IndexOf(
            "_cutStateReconcileQueued = true;",
            queue,
            StringComparison.Ordinal);
        int callback = surface.IndexOf(
            "_cutStateReconcileQueued = false;",
            queue,
            StringComparison.Ordinal);
        int recheck = surface.IndexOf(
            "_cutClipboardPaths.Length > 0",
            callback,
            StringComparison.Ordinal);
        int sweep = surface.IndexOf(
            "ApplyCutState();",
            recheck,
            StringComparison.Ordinal);

        // Coalesced per dispatcher pass, and the callback re-checks the
        // clipboard before sweeping: the final departures of a bulk sync may
        // have drained it entirely.
        Assert.True(queue > 0 && queuedFlag > queue && callback > queuedFlag &&
            recheck > callback && sweep > recheck,
            "the queued reconciler must coalesce and re-check the clipboard before sweeping");
    }
}
