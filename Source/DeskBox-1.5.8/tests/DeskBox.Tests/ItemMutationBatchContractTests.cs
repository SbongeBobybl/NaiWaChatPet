namespace DeskBox.Tests;

/// <summary>
/// Source contract for the batch import mutation scope. WidgetViewModel
/// needs a live dispatcher, so the lifecycle is pinned at the source. The
/// pins deliberately encode asynchronous reality, not source adjacency: the
/// queue calls only enqueue, hydration snapshots the rendered prefix
/// synchronously at startup, and the deferred hydration start therefore
/// lives inside the reconcile callback — the bug this guards is asserting
/// the right calls in the wrong completion order.
/// </summary>
public sealed class ItemMutationBatchContractTests
{
    [Fact]
    public void Scope_Finalization_DefersEachReactionExactlyOnce()
    {
        string batch = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.ItemMutationBatch.cs"))
            .Replace("\r\n", "\n");

        Assert.Contains("internal IDisposable EnterItemMutationScope()", batch, StringComparison.Ordinal);
        Assert.Contains("MarkItemMutationBatchDirty() => _itemMutationBatchDirty = true;", batch, StringComparison.Ordinal);
        // Nested scopes finalize only when the last one closes.
        Assert.Contains(
            "owner._itemMutationBatchDepth > 0 || !owner._itemMutationBatchDirty",
            batch,
            StringComparison.Ordinal);

        // Finalization runs each deferred reaction exactly once...
        foreach (string reaction in new[]
                 {
                     "owner.NormalizeSortOrder();",
                     "owner.PersistManualOrderSnapshotIfChanged();",
                     "owner._addedAtPersistPending = false;",
                     "owner.PersistAddedAtTracking();",
                     "owner.QueueStackDisplayRebuild();",
                     "owner.QueuePostBatchHydration();",
                     "owner.RunDeferredFolderRefreshAsync();"
                 })
        {
            Assert.Contains(reaction, batch, StringComparison.Ordinal);
        }

        // ...in an order where the persisted state settles before the
        // projections rebuild, and hydration is NOT started at scope exit:
        // the queues only enqueue and hydration snapshots the rendered
        // prefix synchronously, so a direct start here would read the stale
        // prefix. The start belongs to the reconcile callback.
        Assert.Contains("owner.QueuePostBatchHydration();", batch, StringComparison.Ordinal);
        Assert.DoesNotContain("owner.StartItemHydration();", batch, StringComparison.Ordinal);
        Assert.True(
            batch.IndexOf("owner.PersistAddedAtTracking();", StringComparison.Ordinal) <
            batch.IndexOf("owner.QueueStackDisplayRebuild();", StringComparison.Ordinal),
            "AddedAt persistence must settle before the projections rebuild");
        Assert.True(
            batch.IndexOf("owner.NormalizeSortOrder();", StringComparison.Ordinal) <
            batch.IndexOf("owner.QueueStackDisplayRebuild();", StringComparison.Ordinal),
            "normalize must precede the stack rebuild");
    }

    [Fact]
    public void DeferredHydration_StartsInsideTheReconcileCallback()
    {
        string windowing = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.Windowing.cs"))
            .Replace("\r\n", "\n");

        int callback = windowing.IndexOf(
            "_renderWindowReconcileQueued = false;",
            StringComparison.Ordinal);
        int reconciled = windowing.IndexOf(
            "ReconcileRenderWindow();",
            callback,
            StringComparison.Ordinal);
        int sourceChanged = windowing.IndexOf(
            "RenderWindowSourceChanged?.Invoke();",
            callback,
            StringComparison.Ordinal);
        int pendingCheck = windowing.IndexOf(
            "if (_pendingPostBatchHydration)",
            callback,
            StringComparison.Ordinal);
        int hydration = windowing.IndexOf(
            "StartItemHydration();",
            pendingCheck,
            StringComparison.Ordinal);

        // The pending flag must be consumed strictly after the callback has
        // applied the settled rendered prefix — this ordering is the actual
        // fix; asserting it on adjacent source lines would prove nothing.
        Assert.True(callback > 0 && reconciled > callback && sourceChanged > reconciled &&
            pendingCheck > sourceChanged && hydration > pendingCheck,
            "deferred hydration must start after the reconcile applied the settled prefix");
        Assert.Contains("owner.QueuePostBatchHydration()", File.ReadAllText(
            TestPaths.FromRepository(
                "src/DeskBox/ViewModels/WidgetViewModel.ItemMutationBatch.cs")), StringComparison.Ordinal);
    }

    [Fact]
    public void UpsertAndRemoval_DeferDerivedWorkInsideABatch()
    {
        string watchers = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.SortingAndWatchers.cs"))
            .Replace("\r\n", "\n");

        // Both UpsertFolderItemAsync branches route through the gated helper.
        Assert.Equal(2, CountOccurrences(watchers, "FinishItemUpsert();"));
        // The old per-upsert tails ended with hydration followed by the
        // branch return; only the helper may carry that sequence now, and it
        // never returns true.
        Assert.DoesNotContain(
            "StartItemHydration();\n            return true;",
            watchers,
            StringComparison.Ordinal);

        // The helper defers while a batch is active...
        int helper = watchers.IndexOf(
            "private void FinishItemUpsert()",
            StringComparison.Ordinal);
        int gate = watchers.IndexOf(
            "if (_itemMutationBatchDepth > 0)",
            helper,
            StringComparison.Ordinal);
        int deferredReturn = watchers.IndexOf(
            "MarkItemMutationBatchDirty();",
            gate,
            StringComparison.Ordinal);
        int normalize = watchers.IndexOf(
            "NormalizeSortOrder();",
            gate,
            StringComparison.Ordinal);
        Assert.True(gate > helper && deferredReturn > gate && normalize > deferredReturn,
            "FinishItemUpsert must gate on the batch depth before its fallback tail");

        // ...and RemoveItemByPath defers the same way before its own tail.
        int removal = watchers.IndexOf(
            "private void RemoveItemByPath",
            StringComparison.Ordinal);
        int removalGate = watchers.IndexOf(
            "if (_itemMutationBatchDepth > 0)",
            removal,
            StringComparison.Ordinal);
        int removalDirty = watchers.IndexOf(
            "MarkItemMutationBatchDirty();",
            removalGate,
            StringComparison.Ordinal);
        int removalNormalize = watchers.IndexOf(
            "NormalizeSortOrder();",
            removalGate,
            StringComparison.Ordinal);
        Assert.True(removalGate > removal && removalDirty > removalGate &&
            removalNormalize > removalDirty,
            "RemoveItemByPath must gate on the batch depth before its tail");
    }

    [Fact]
    public void AddedAtTracking_PersistsOncePerBatchNotOncePerFile()
    {
        string addedAt = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.AddedAt.cs"))
            .Replace("\r\n", "\n");

        // All four mutation sites (record, assign, transfer, remove) defer.
        Assert.Equal(4, CountOccurrences(addedAt, "PersistAddedAtTrackingIfNeeded();"));
        int helper = addedAt.IndexOf(
            "private void PersistAddedAtTrackingIfNeeded()",
            StringComparison.Ordinal);
        int gate = addedAt.IndexOf(
            "if (_itemMutationBatchDepth > 0)",
            helper,
            StringComparison.Ordinal);
        int pending = addedAt.IndexOf(
            "_addedAtPersistPending = true;",
            gate,
            StringComparison.Ordinal);
        int fallback = addedAt.IndexOf(
            "PersistAddedAtTracking();",
            gate,
            StringComparison.Ordinal);
        Assert.True(helper > 0 && gate > helper && pending > gate && fallback > pending,
            "the AddedAt helper must gate on the batch depth before its direct persist");

        // The scope finalization consumes the pending persist exactly once.
        string batch = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.ItemMutationBatch.cs"));
        Assert.Equal(1, CountOccurrences(batch, "owner._addedAtPersistPending = false;"));
        Assert.Equal(1, CountOccurrences(batch, "owner.PersistAddedAtTracking();"));
    }

    [Fact]
    public void QueuedReactions_GateOnTheBatchAndMarkDirty()
    {
        string windowing = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.Windowing.cs"))
            .Replace("\r\n", "\n");
        string stacks = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.Stacks.cs"))
            .Replace("\r\n", "\n");

        // The render-window queue defers inside a batch, before its own
        // coalescing flag: the finalization must be able to enqueue for real.
        int queue = windowing.IndexOf(
            "private void QueueRenderWindowReconcile()",
            StringComparison.Ordinal);
        int disposedCheck = windowing.IndexOf(
            "if (_isDisposed)",
            queue,
            StringComparison.Ordinal);
        int gate = windowing.IndexOf(
            "if (_itemMutationBatchDepth > 0)",
            queue,
            StringComparison.Ordinal);
        int queuedFlag = windowing.IndexOf(
            "if (_renderWindowReconcileQueued)",
            queue,
            StringComparison.Ordinal);
        Assert.True(
            disposedCheck > queue && gate > disposedCheck && queuedFlag > gate,
            "the batch gate must sit between the disposed check and the coalescing flag");
        Assert.Contains(
            "reconciles would re-mirror the prefix once per dispatcher pass",
            windowing,
            StringComparison.Ordinal);

        // The stack queue keeps its staleness flag set during the batch so
        // mid-batch readers know the projection is behind, then defers the
        // enqueue itself.
        int stackQueue = stacks.IndexOf(
            "private void QueueStackDisplayRebuild()",
            StringComparison.Ordinal);
        int staleness = stacks.IndexOf(
            "_hasBuiltStackDisplay = false;",
            stackQueue,
            StringComparison.Ordinal);
        int stackQueuedFlag = stacks.IndexOf(
            "if (_stackRebuildQueued)",
            stackQueue,
            StringComparison.Ordinal);
        int stackGate = stacks.IndexOf(
            "if (_itemMutationBatchDepth > 0)",
            stackQueue,
            StringComparison.Ordinal);
        int stackEnqueue = stacks.IndexOf(
            "_stackRebuildQueued = true;",
            stackQueue,
            StringComparison.Ordinal);
        Assert.True(
            staleness > stackQueue && stackQueuedFlag > staleness &&
            stackGate > stackQueuedFlag && stackEnqueue > stackGate,
            "the stack staleness flag and queued-flag check must precede the batch gate and the enqueue");
    }

    [Fact]
    public void WatcherFullReload_DefersToOneAuthoritativeRefreshPerBatch()
    {
        string watchers = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.SortingAndWatchers.cs"))
            .Replace("\r\n", "\n");

        // A full reload mid-import bypasses every per-item gate, so the
        // ShouldUseFullReload branch defers while a batch is open.
        int shouldUse = watchers.IndexOf(
            "if (ShouldUseFullReload(changeBatch, CurrentFolderPath))",
            StringComparison.Ordinal);
        int gate = watchers.IndexOf(
            "if (_itemMutationBatchDepth > 0)",
            shouldUse,
            StringComparison.Ordinal);
        int defer = watchers.IndexOf(
            "_pendingFolderRefreshAfterBatch = true;",
            gate,
            StringComparison.Ordinal);
        int reload = watchers.IndexOf(
            "await LoadFolderContentsAsync(CurrentFolderPath);",
            shouldUse,
            StringComparison.Ordinal);
        Assert.True(shouldUse > 0 && gate > shouldUse && defer > gate && reload > defer,
            "the full-reload branch must defer inside a batch before its direct reload");

        // The deferred refresh re-enters through the folder refresh gate and
        // the batch finalization triggers it at most once.
        int deferredRunner = watchers.IndexOf(
            "private async Task RunDeferredFolderRefreshAsync()",
            StringComparison.Ordinal);
        int gateWait = watchers.IndexOf(
            "await _folderRefreshGate.WaitAsync();",
            deferredRunner,
            StringComparison.Ordinal);
        Assert.True(deferredRunner > 0 && gateWait > deferredRunner,
            "the deferred refresh must serialize through the folder refresh gate");
        string batch = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.ItemMutationBatch.cs"));
        Assert.Equal(1, CountOccurrences(batch, "owner._pendingFolderRefreshAfterBatch = false;"));
        Assert.Equal(1, CountOccurrences(batch, "owner.RunDeferredFolderRefreshAsync();"));
    }

    [Fact]
    public void FullReload_CommitsOnlyAfterACommitPointBatchCheck()
    {
        // The entry-time deferral alone cannot close the race: a reload that
        // started before the batch opened awaits its directory enumeration
        // for seconds, and the batch can open during that await. The guard
        // must sit at the commit point - after the last enumeration await,
        // before any Items mutation - with no await in between, so both the
        // primary watcher path and the exception fallback path are covered
        // by one check.
        string hydration = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.ItemHydration.cs"))
            .Replace("\r\n", "\n");

        int guard = hydration.IndexOf(
            "if (_itemMutationBatchDepth > 0)\n        {\n            // Commit-point race guard.",
            StringComparison.Ordinal);
        Assert.True(guard > 0, "the commit-point guard must exist in LoadFolderContentsAsync");
        Assert.Contains("_pendingFolderRefreshAfterBatch = true;", hydration, StringComparison.Ordinal);
        Assert.Contains(
            "No await may\n            // appear between this check and the Items mutation below.",
            hydration,
            StringComparison.Ordinal);

        int addedTimes = hydration.IndexOf(
            "ApplyPersistedAddedTimes(items);",
            guard,
            StringComparison.Ordinal);
        int sync = hydration.IndexOf(
            "SyncFolderItems(items);",
            guard,
            StringComparison.Ordinal);
        int sort = hydration.IndexOf(
            "SortItems();",
            sync,
            StringComparison.Ordinal);
        int hydrate = hydration.IndexOf(
            "StartItemHydration();",
            sort,
            StringComparison.Ordinal);
        Assert.True(addedTimes > guard && sync > addedTimes && sort > sync && hydrate > sort,
            "the Items mutation sequence must run only after the commit guard");

        string betweenGuardAndSync = hydration[guard..sync];
        // Strip comment lines first - the guard's own comment explains the
        // no-await rule and would otherwise trip the check itself.
        string code = string.Join("\n", betweenGuardAndSync.Split('\n')
            .Where(line => !line.TrimStart().StartsWith("//", StringComparison.Ordinal)));
        Assert.DoesNotContain(
            "await ",
            code,
            StringComparison.Ordinal);
        // The guard must be reachable from every reload path: both the
        // primary watcher branch and the exception fallback funnel through
        // LoadFolderContentsAsync, which returns false - the existing
        // "snapshot not applied" convention - when it defers.
        Assert.Contains("return false;", hydration[guard..(guard + betweenGuardAndSync.Length)], StringComparison.Ordinal);
    }

    [Fact]
    public void RenderWindowQueue_RollsBackFlagsWhenEnqueueFails()
    {
        string windowing = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.Windowing.cs"))
            .Replace("\r\n", "\n");

        // A failed enqueue must not leave the coalescing flag or a deferred
        // hydration start set, or the widget never reconciles or hydrates
        // again.
        int enqueue = windowing.IndexOf(
            "if (!_dispatcherQueue.TryEnqueue(() =>",
            StringComparison.Ordinal);
        Assert.True(enqueue > 0, "the reconcile enqueue must be failure-checked");
        int failureBranch = windowing.IndexOf(
            "_renderWindowReconcileQueued = false;\n            _pendingPostBatchHydration = false;",
            enqueue,
            StringComparison.Ordinal);
        Assert.True(failureBranch > enqueue,
            "both flags must roll back when the enqueue fails");
    }

    [Fact]
    public void BulkImportLoops_OpenExactlyOneScopeEach()
    {
        string operations = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.Operations.cs"));

        // Three scope consumers: the two bulk import loops and the move-out
        // handler (drag-out is the mirror amplifier of the import; both pay
        // the same per-item derived work without the scope).
        Assert.Equal(3, CountOccurrences(operations, "EnterItemMutationScope()"));
        // The transfer-results batch must finalize even when an upsert
        // throws: scope disposal sits in a finally alongside the perf log.
        Assert.Contains(
            "IDisposable batchScope = EnterItemMutationScope();",
            operations,
            StringComparison.Ordinal);
        Assert.Contains("batchScope.Dispose();", operations, StringComparison.Ordinal);
        // The perf log must not stop the stopwatch before finalization:
        // upsertLoopMs and finalizeSyncMs together bound the real cost.
        Assert.Contains("upsertLoopMs=", operations, StringComparison.Ordinal);
        Assert.Contains("finalizeSyncMs=", operations, StringComparison.Ordinal);
        Assert.True(
            operations.IndexOf("batchScope.Dispose();", StringComparison.Ordinal) <
            operations.IndexOf("finalizeSyncMs=", StringComparison.Ordinal),
            "the finalize measurement must include the scope disposal");
    }

    [Fact]
    public void BatchPathIndex_IsScopedStoresReferencesAndSelfHeals()
    {
        // The scoped path dictionary exists only between scope open and
        // close, maps paths to live references (never indexes - index
        // shifts from concurrent inserts must not stale it), and falls back
        // to the linear scan when a reference is gone.
        string batch = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.ItemMutationBatch.cs"))
            .Replace("\r\n", "\n");

        Assert.Contains(
            "private Dictionary<string, WidgetItem>? _batchItemsByPath;",
            batch,
            StringComparison.Ordinal);
        // Built once when the first scope opens, not per file...
        int open = batch.IndexOf(
            "internal IDisposable EnterItemMutationScope()",
            StringComparison.Ordinal);
        int build = batch.IndexOf(
            "_batchItemsByPath = index;",
            open,
            StringComparison.Ordinal);
        Assert.True(open > 0 && build > open, "the index must be built inside the scope open");

        // ...dropped when the last scope closes...
        int close = batch.IndexOf(
            "owner._itemMutationBatchDepth == 0",
            StringComparison.Ordinal);
        int dropped = batch.IndexOf(
            "owner._batchItemsByPath = null;",
            close,
            StringComparison.Ordinal);
        Assert.True(close > 0 && dropped > close, "the index must be dropped at scope close");

        // ...reference-resolved, with the linear fallback ONLY for a stale
        // reference. A dictionary miss must return -1 directly - the miss is
        // the fresh-import fast path, and falling back to the full-list
        // scan there was the bug that kept the O(n^2) alive.
        int lookup = batch.IndexOf(
            "private int FindItemIndexForManagedMutation(string path)",
            StringComparison.Ordinal);
        int missReturn = batch.IndexOf(
            "if (!_batchItemsByPath.TryGetValue(path, out WidgetItem? existing))\n        {\n            return -1;\n        }",
            lookup,
            StringComparison.Ordinal);
        int referenceResolve = batch.IndexOf(
            "IndexOfReference(Items, existing, 0)",
            lookup,
            StringComparison.Ordinal);
        Assert.True(lookup > 0 && missReturn > lookup && referenceResolve > missReturn,
            "a dictionary miss must return -1 immediately, before any index resolution");
        // The method contains two linear-scan returns (the no-batch early
        // exit and the stale-reference fallback); the fallback one is the
        // second, after the reference resolution.
        int fallback = batch.IndexOf(
            "return FindItemIndexByPath(path);",
            referenceResolve,
            StringComparison.Ordinal);
        Assert.True(fallback > referenceResolve,
            "the linear scan may only serve the stale-reference fallback");

        // The two managed mutation paths route through the scoped lookup and
        // keep the dictionary in sync with their Items mutations.
        string watchers = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.SortingAndWatchers.cs"))
            .Replace("\r\n", "\n");
        int upsert = watchers.IndexOf(
            "int existingIndex = FindItemIndexForManagedMutation(path);",
            StringComparison.Ordinal);
        int removal = watchers.IndexOf(
            "int index = FindItemIndexForManagedMutation(path);",
            StringComparison.Ordinal);
        Assert.True(upsert > 0 && removal > upsert, "both mutation paths must use the routed lookup");
        Assert.Equal(2, CountOccurrences(watchers, "TrackManagedItemByPath(path, item);"));
        Assert.Equal(1, CountOccurrences(watchers, "UntrackManagedItemByPath(path);"));
    }

    [Fact]
    public void SortedInsertIndex_IsABinaryLowerBoundNotALinearScan()
    {
        string watchers = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.SortingAndWatchers.cs"))
            .Replace("\r\n", "\n");

        int method = watchers.IndexOf(
            "private int GetSortedInsertIndex(WidgetItem candidate)",
            StringComparison.Ordinal);
        int call = watchers.IndexOf(
            "return BinarySearchSortedInsertIndex(",
            method,
            StringComparison.Ordinal);
        int helper = watchers.IndexOf(
            "internal static int BinarySearchSortedInsertIndex(",
            StringComparison.Ordinal);
        Assert.True(method > 0 && call > method && helper > call,
            "GetSortedInsertIndex must delegate to the binary lower-bound search");

        // The old per-candidate linear scan is gone - that scan is the O(n)
        // per file this phase removes.
        int methodEnd = watchers.IndexOf("\n    }\n", call, StringComparison.Ordinal);
        string body = watchers[method..methodEnd];
        Assert.DoesNotContain(
            "for (int index = 0; index < Items.Count; index++)",
            body,
            StringComparison.Ordinal);
    }

    private static int CountOccurrences(string text, string needle)
    {
        int count = 0;
        int index = 0;
        while ((index = text.IndexOf(needle, index, StringComparison.Ordinal)) >= 0)
        {
            count++;
            index += needle.Length;
        }

        return count;
    }
}
