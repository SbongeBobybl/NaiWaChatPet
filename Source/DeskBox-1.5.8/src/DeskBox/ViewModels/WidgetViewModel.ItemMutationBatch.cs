using DeskBox.Models;

namespace DeskBox.ViewModels;

/// <summary>
/// Batch mutation scope for bulk item imports. A 2000-file import used to
/// pay the per-item derived work 2000 times: a full NormalizeSortOrder pass
/// over the list, a manual-order persistence check, an AddedAt settings
/// persist, a hydration restart (generation bump plus cancellation of the
/// still-running pass), and — wherever an await let the dispatcher run a
/// queued callback — a render window reconcile and a full stack-display
/// rebuild. Inside the scope those reactions are deferred; disposing the
/// scope runs each of them exactly once against the settled item list.
/// </summary>
public partial class WidgetViewModel
{
    private int _itemMutationBatchDepth;
    private bool _itemMutationBatchDirty;
    private bool _addedAtPersistPending;
    private bool _pendingFolderRefreshAfterBatch;

    /// <summary>
    /// Batch-scoped path index: path to the live item reference, built once
    /// when the first scope opens and dropped when it closes. Values are
    /// references rather than indexes on purpose - index shifts from the
    /// inserts running mid-batch cannot stale it, and it never needs the
    /// Move/Sort/rename bookkeeping that makes a permanent path-index map a
    /// bug nursery.
    /// </summary>
    private Dictionary<string, WidgetItem>? _batchItemsByPath;

    /// <summary>
    /// Opens a batch mutation scope. Disposing it finalizes: one sort-order
    /// normalization pass, one manual-order persistence check, one AddedAt
    /// persistence, one stack-display rebuild, one render-window reconcile
    /// (which in turn re-checks viewport coverage and then starts hydration
    /// against the settled prefix), and at most one deferred watcher
    /// refresh. An exception inside the scope still finalizes — callers
    /// hold it across try/finally or using blocks.
    /// </summary>
    internal IDisposable EnterItemMutationScope()
    {
        if (_itemMutationBatchDepth == 0)
        {
            // One existence index per batch, not per file: the upsert path
            // consults it instead of scanning the whole list per file.
            var index = new Dictionary<string, WidgetItem>(
                StringComparer.OrdinalIgnoreCase);
            foreach (WidgetItem item in Items)
            {
                if (!string.IsNullOrEmpty(item.Path))
                {
                    index[item.Path] = item;
                }
            }

            _batchItemsByPath = index;
        }

        _itemMutationBatchDepth++;
        return new ItemMutationScope(this);
    }

    /// <summary>
    /// Path lookup for the managed mutation paths (upsert and removal).
    /// Outside a batch this is the plain linear scan. Inside a batch the
    /// scope dictionary is the membership authority: a miss is "not
    /// present", O(1) - the fresh-import fast path, and the whole point of
    /// the index. Every mid-batch Items mutation flows through the tracked
    /// upsert/removal paths (full reloads are deferred by the commit-point
    /// guard, the sort-mode rebuild re-adds the same references), so a miss
    /// cannot be false. Only a hit whose reference has gone stale (an
    /// out-of-band object swap) pays one linear scan as a defensive
    /// fallback - the index can only over-perform, never misreport.
    /// </summary>
    private int FindItemIndexForManagedMutation(string path)
    {
        if (_batchItemsByPath is null)
        {
            return FindItemIndexByPath(path);
        }

        if (!_batchItemsByPath.TryGetValue(path,out WidgetItem? existing))
        {
            return -1;
        }

        int referenceIndex = IndexOfReference(Items,existing,0);
        if (referenceIndex >= 0)
        {
            return referenceIndex;
        }

        return FindItemIndexByPath(path);
    }

    private void TrackManagedItemByPath(string path,WidgetItem item)
    {
        if (_batchItemsByPath is not null && !string.IsNullOrEmpty(path))
        {
            _batchItemsByPath[path] = item;
        }
    }

    private void UntrackManagedItemByPath(string path) =>
        _batchItemsByPath?.Remove(path);

    private void MarkItemMutationBatchDirty() => _itemMutationBatchDirty = true;

    private sealed class ItemMutationScope : IDisposable
    {
        private WidgetViewModel? _owner;

        public ItemMutationScope(WidgetViewModel owner) => _owner = owner;

        public void Dispose()
        {
            WidgetViewModel? owner = _owner;
            _owner = null;
            if (owner is null || owner._itemMutationBatchDepth <= 0)
            {
                return;
            }

            owner._itemMutationBatchDepth--;
            if (owner._itemMutationBatchDepth == 0)
            {
                owner._batchItemsByPath = null;
            }

            if (owner._itemMutationBatchDepth > 0 || !owner._itemMutationBatchDirty)
            {
                return;
            }

            owner._itemMutationBatchDirty = false;
            owner.NormalizeSortOrder();
            owner.PersistManualOrderSnapshotIfChanged();
            if (owner._addedAtPersistPending)
            {
                owner._addedAtPersistPending = false;
                owner.PersistAddedAtTracking();
            }

            owner.QueueStackDisplayRebuild();
            // Hydration must not start here: the render reconcile below only
            // enqueues, and every hydration pipeline snapshots the rendered
            // prefix synchronously at startup. QueuePostBatchHydration hands
            // the start to that callback, so hydration reads the settled
            // prefix, not the stale one.
            owner.QueuePostBatchHydration();
            if (owner._pendingFolderRefreshAfterBatch)
            {
                owner._pendingFolderRefreshAfterBatch = false;
                _ = owner.RunDeferredFolderRefreshAsync();
            }
        }
    }
}
