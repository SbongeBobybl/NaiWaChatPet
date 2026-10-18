namespace DeskBox.Models;

/// <summary>
/// Reconciles a complete folder snapshot with the user's manual item order.
/// The live order wins during a running session, but only for paths that
/// survive in this snapshot: after embedded folder navigation the live
/// collection still holds the folder just left, so its foreign paths must
/// not outrank the persisted order for the folder being loaded. The
/// persisted order is the fallback when no live path applies — cold start
/// or a folder switch.
/// </summary>
internal static class WidgetManualOrderPolicy
{
    public static IReadOnlyList<T> Reconcile<T>(
        IReadOnlyList<T> refreshedItems,        IReadOnlyList<string> liveOrderPaths,        IReadOnlyList<WidgetItemConfig> persistedItems,        Func<T, string> pathSelector)
    {
        ArgumentNullException.ThrowIfNull(refreshedItems);
        ArgumentNullException.ThrowIfNull(liveOrderPaths);
        ArgumentNullException.ThrowIfNull(persistedItems);
        ArgumentNullException.ThrowIfNull(pathSelector);

        var refreshedPaths = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        foreach (T refreshedItem in refreshedItems)
        {
            string? path = pathSelector(refreshedItem);
            if (!string.IsNullOrWhiteSpace(path))
            {
                refreshedPaths.Add(path);
            }
        }

        List<string> liveBaseline = liveOrderPaths
            .Where(refreshedPaths.Contains)
            .ToList();

        IReadOnlyList<string> baseline = liveBaseline.Count > 0
            ? liveBaseline
            : persistedItems
                .Where(item => !string.IsNullOrWhiteSpace(item.Path))
                .OrderBy(item => item.SortOrder)
                .Select(item => item.Path)
                .ToList();

        var rankByPath = new Dictionary<string, int>(StringComparer.OrdinalIgnoreCase);
        for (int index = 0; index < baseline.Count; index++)
        {
            string path = baseline[index];
            if (!string.IsNullOrWhiteSpace(path))
            {
                rankByPath.TryAdd(path,index);
            }
        }

        var uniqueItems = new List<(T Item, int SnapshotIndex, int? Rank)>();
        var seenPaths = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
        for (int index = 0; index < refreshedItems.Count; index++)
        {
            T item = refreshedItems[index];
            string path = pathSelector(item);
            if (string.IsNullOrWhiteSpace(path) || !seenPaths.Add(path))
            {
                continue;
            }

            uniqueItems.Add((
                item,
                index,
                rankByPath.TryGetValue(path,out int rank) ? rank : null));
        }

        return uniqueItems
            .OrderBy(entry => entry.Rank.HasValue ? 0 : 1)
            .ThenBy(entry => entry.Rank ?? int.MaxValue)
            .ThenBy(entry => entry.SnapshotIndex)
            .Select(entry => entry.Item)
            .ToList();
    }
}
