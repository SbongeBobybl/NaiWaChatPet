using System.Text.Json;
using System.Text.Json.Serialization;
using DeskBox.Models;

namespace DeskBox.Services;

public sealed class TodoWidgetStore
{
    private readonly string _storePath;

    public TodoWidgetStore(string widgetId)
        : this(
            Path.Combine(
                DeskBoxDataPathService.Current.DataDirectory,                "widgets"),            widgetId)
    {
    }

    internal TodoWidgetStore(string widgetsDataRoot,string widgetId)
    {
        if (string.IsNullOrWhiteSpace(widgetId))
        {
            throw new ArgumentException("Widget id cannot be empty.",nameof(widgetId));
        }

        string safeWidgetId = SanitizeWidgetId(widgetId);
        string dataDir = Path.Combine(widgetsDataRoot,safeWidgetId);
        Directory.CreateDirectory(dataDir);
        _storePath = Path.Combine(dataDir,"todo.json");
    }

    internal string StorePath => _storePath;

    internal string AttachmentDirectory => Path.Combine(Path.GetDirectoryName(_storePath)!,"attachments");

    public async Task<TodoWidgetData> LoadAsync()
    {
        return await ResilientJsonStore.LoadAsync(
            _storePath,            json => Normalize(JsonSerializer.Deserialize(
                json,                TodoJsonContext.Default.StoreData)),            () => new TodoWidgetData(),            nameof(TodoWidgetStore));
    }

    public Task SaveAsync(TodoWidgetData data)
    {
        data = Normalize(data);
        // Stream the UTF-8 directly into the temp file instead of paying the
        // transient whole-document string on every save.
        return ResilientJsonStore.SaveAsync(
            _storePath,            async tempPath =>
            {
                await using var stream = new FileStream(
                    tempPath,                    FileMode.Create,                    FileAccess.Write,                    FileShare.None);
                await JsonSerializer.SerializeAsync(
                    stream,                    data,                    TodoJsonContext.Default.StoreData);
            });
    }

    private static TodoWidgetData Normalize(TodoWidgetData? data)
    {
        data ??= new TodoWidgetData();
        data.Version = Math.Max(3,data.Version);
        data.Items ??= [];

        int fallbackSortOrder = 0;
        foreach (var item in data.Items.Where(item => item is not null))
        {
            if (string.IsNullOrWhiteSpace(item.Id))
            {
                item.Id = Guid.NewGuid().ToString("N");
            }

            // Sync-layer field: local records all originate from this device.
            // Records imported with another device_id keep it (last-writer
            // semantics are resolved by the future sync projection).
            item.DeviceId ??= DeviceIdentity.Id;

            item.Text = item.Text?.Trim() ?? string.Empty;
            item.ColorMarker = TodoItem.NormalizeColorMarker(item.ColorMarker);
            item.Recurrence = TodoRecurrence.Normalize(item.Recurrence,item.DueDate);
            item.RecurrenceSeriesId = TodoRecurrenceService.NormalizeSeriesId(item.RecurrenceSeriesId);
            // Notes may contain Markdown where trailing spaces and indentation
            // are meaningful, so only collapse an entirely blank document.
            item.Notes = string.IsNullOrWhiteSpace(item.Notes) ? null : item.Notes;
            item.Steps ??= [];
            item.Attachments ??= [];
            NormalizeSteps(item.Steps);
            NormalizeAttachments(item.Attachments);
            item.ReminderOffsetMinutes = TodoReminderOptions.NormalizeOffsetMinutes(item.ReminderOffsetMinutes);
            item.GeneratedNextItemId = string.IsNullOrWhiteSpace(item.GeneratedNextItemId)
                ? null
                : item.GeneratedNextItemId.Trim();
            if (item.CreatedAt == default)
            {
                item.CreatedAt = DateTimeOffset.UtcNow;
            }

            if (item.UpdatedAt == default)
            {
                item.UpdatedAt = item.CreatedAt;
            }

            if (item.IsCompleted)
            {
                item.CompletedAt ??= item.UpdatedAt == default ? item.CreatedAt : item.UpdatedAt;
            }
            else
            {
                item.CompletedAt = null;
            }

            if (item.DueDate is null)
            {
                item.Recurrence = null;
                item.RecurrenceSeriesId = null;
                item.ReminderLastNotifiedAt = null;
                item.ReminderDismissedForDueDate = null;
                item.SnoozedUntil = null;
                item.SnoozeLastNotifiedAt = null;
            }
            else if (item.IsCompleted)
            {
                item.SnoozedUntil = null;
                item.SnoozeLastNotifiedAt = null;
            }
            else if (item.ReminderDismissedForDueDate is { } dismissedForDueDate &&
                     !DateTimeOffset.Equals(dismissedForDueDate,item.DueDate.Value))
            {
                item.ReminderLastNotifiedAt = null;
                item.ReminderDismissedForDueDate = null;
                item.SnoozedUntil = null;
                item.SnoozeLastNotifiedAt = null;
            }

            if (!item.IsCompleted || item.Recurrence is null)
            {
                item.GeneratedNextItemId = null;
            }

            if (item.Recurrence is null)
            {
                item.RecurrenceSeriesId = null;
            }

            if (item.SortOrder < 0)
            {
                item.SortOrder = fallbackSortOrder;
            }

            fallbackSortOrder++;
        }

        data.Items = data.Items
            .Where(item => item is not null && !string.IsNullOrWhiteSpace(item.Text))
            .GroupBy(item => item.Id,StringComparer.Ordinal)
            .Select(group => group.First())
            .OrderBy(item => item.SortOrder)
            .ThenByDescending(item => item.UpdatedAt)
            .ToList();

        NormalizeRecurrenceSeriesIds(data.Items);
        NormalizeSortOrders(data.Items);
        return data;
    }

    public Task ClearAsync()
    {
        return SaveAsync(new TodoWidgetData());
    }

    /// <summary>
    /// Deletes every persisted artifact of a widget's todo data: the store
    /// file, its resilient backup, the attachments directory, and the widget
    /// directory itself when it becomes empty. Callers invoke this when the
    /// widget is removed (WidgetManager.RemoveWidgetAsync and the duplicate
    /// cleanup in ResetFeatureWidgetAsync) so deleting a todo widget leaves
    /// no orphaned data behind, mirroring GlanceWidgetStore.
    /// </summary>
    public static Task DeleteForWidgetAsync(string widgetId) =>
        DeleteForWidgetAsync(
            Path.Combine(
                DeskBoxDataPathService.Current.DataDirectory,                "widgets"),            widgetId);

    internal static Task DeleteForWidgetAsync(string widgetsDataRoot,string widgetId)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(widgetId);
        string dataDir = Path.Combine(widgetsDataRoot,SanitizeWidgetId(widgetId));
        string storePath = Path.Combine(dataDir,"todo.json");
        TryDeleteFile(storePath);
        TryDeleteFile(ResilientJsonStore.GetBackupPath(storePath));
        TryDeleteDirectory(Path.Combine(dataDir,"attachments"));
        TryDeleteDirectoryIfEmpty(dataDir);
        return Task.CompletedTask;
    }

    private static void TryDeleteFile(string path)
    {
        try
        {
            if (File.Exists(path))
            {
                File.Delete(path);
            }
        }
        catch (Exception ex)
        {
            App.Log($"[TodoWidgetStore] Failed to delete '{path}': {ex.Message}");
        }
    }

    private static void TryDeleteDirectory(string path)
    {
        try
        {
            if (Directory.Exists(path))
            {
                Directory.Delete(path,recursive: true);
            }
        }
        catch (Exception ex)
        {
            App.Log($"[TodoWidgetStore] Failed to delete directory '{path}': {ex.Message}");
        }
    }

    private static void TryDeleteDirectoryIfEmpty(string path)
    {
        try
        {
            if (Directory.Exists(path) && !Directory.EnumerateFileSystemEntries(path).Any())
            {
                Directory.Delete(path);
            }
        }
        catch (Exception ex)
        {
            App.Log($"[TodoWidgetStore] Failed to remove empty directory '{path}': {ex.Message}");
        }
    }

    private static void NormalizeSteps(List<TodoStep> steps)
    {
        int sortOrder = 0;
        foreach (TodoStep step in steps.Where(step => step is not null))
        {
            step.Id = string.IsNullOrWhiteSpace(step.Id) ? Guid.NewGuid().ToString("N") : step.Id.Trim();
            step.Text = step.Text?.Trim() ?? string.Empty;
            step.SortOrder = sortOrder++;
        }

        steps.RemoveAll(step => step is null || string.IsNullOrWhiteSpace(step.Text));
        for (int index = 0; index < steps.Count; index++)
        {
            steps[index].SortOrder = index;
        }
    }

    private static void NormalizeAttachments(List<TodoAttachment> attachments)
    {
        foreach (TodoAttachment attachment in attachments.Where(attachment => attachment is not null))
        {
            attachment.Id = string.IsNullOrWhiteSpace(attachment.Id)
                ? Guid.NewGuid().ToString("N")
                : attachment.Id.Trim();
            attachment.FilePath = attachment.FilePath?.Trim() ?? string.Empty;
            attachment.DisplayName = string.IsNullOrWhiteSpace(attachment.DisplayName)
                ? Path.GetFileName(attachment.FilePath)
                : attachment.DisplayName.Trim();
            attachment.Type = string.IsNullOrWhiteSpace(attachment.Type) ? "file" : attachment.Type.Trim();
            attachment.StorageMode = TodoAttachment.NormalizeStorageMode(attachment.StorageMode);
            attachment.AddedAt = attachment.AddedAt == default ? DateTimeOffset.UtcNow : attachment.AddedAt;
        }

        attachments.RemoveAll(attachment => attachment is null || string.IsNullOrWhiteSpace(attachment.FilePath));
    }

    private static void NormalizeRecurrenceSeriesIds(List<TodoItem> items)
    {
        var recurringItems = items
            .Where(item => item.Recurrence is not null)
            .ToList();
        if (recurringItems.Count == 0)
        {
            return;
        }

        var itemsById = recurringItems.ToDictionary(item => item.Id,StringComparer.Ordinal);
        var visitedIds = new HashSet<string>(StringComparer.Ordinal);

        foreach (var item in recurringItems)
        {
            if (!visitedIds.Add(item.Id))
            {
                continue;
            }

            var component = new List<TodoItem>();
            var queue = new Queue<TodoItem>();
            queue.Enqueue(item);

            while (queue.Count > 0)
            {
                TodoItem current = queue.Dequeue();
                component.Add(current);

                if (!string.IsNullOrWhiteSpace(current.GeneratedNextItemId) &&
                    itemsById.TryGetValue(current.GeneratedNextItemId,out TodoItem? nextItem) &&
                    visitedIds.Add(nextItem.Id))
                {
                    queue.Enqueue(nextItem);
                }

                foreach (var previousItem in recurringItems.Where(entry =>
                             string.Equals(entry.GeneratedNextItemId,current.Id,StringComparison.Ordinal)))
                {
                    if (visitedIds.Add(previousItem.Id))
                    {
                        queue.Enqueue(previousItem);
                    }
                }
            }

            string seriesId = component
                .Select(entry => TodoRecurrenceService.NormalizeSeriesId(entry.RecurrenceSeriesId))
                .FirstOrDefault(seriesId => !string.IsNullOrWhiteSpace(seriesId))
                ?? Guid.NewGuid().ToString("N");

            foreach (var componentItem in component)
            {
                componentItem.RecurrenceSeriesId = seriesId;
            }
        }
    }

    private static void NormalizeSortOrders(List<TodoItem> items)
    {
        for (int index = 0; index < items.Count; index++)
        {
            items[index].SortOrder = index;
        }
    }

    internal static string SanitizeWidgetId(string widgetId)
    {
        string trimmed = widgetId.Trim();
        char[] invalidChars = Path.GetInvalidFileNameChars();
        var safeChars = trimmed.Select(ch => invalidChars.Contains(ch) ? '_' : ch).ToArray();
        string safe = new(safeChars);
        return string.IsNullOrWhiteSpace(safe) ? Guid.NewGuid().ToString("N") : safe;
    }

    /// <summary>
    /// Adopts an orphaned todo store for a freshly created Todo widget.
    ///
    /// Orphaned widget directories under data/widgets/&lt;id&gt;/ arise when a
    /// cloud restore lands a source widget id that does not exist on this
    /// device (the "unmapped" branch — data is preserved on disk but no
    /// widget reads it). Without adoption that data stays invisible forever:
    /// a Todo widget created later gets a NEW id and reads an empty store.
    ///
    /// Claimed-id exclusion is what makes this safe: a directory whose id is
    /// a live widget or sits in DeletedWidgetIds belongs to something the
    /// user still has — or deliberately deleted (deletion tombstones the id
    /// AND removes the directory; the tombstone also covers leftover
    /// directories that survived with extra files). Only directories whose
    /// id this device never owned are eligible, which is exactly the set a
    /// cross-device restore produces.
    /// </summary>
    /// <returns>The adopted source widget id, or null when nothing was
    /// adoptable. Best-effort — never throws.</returns>
    public static async Task<string?> TryAdoptOrphanedStoreAsync(
        string widgetsDataRoot,        string targetWidgetId,        IReadOnlySet<string> claimedWidgetIds,        CancellationToken cancellationToken = default)
    {
        try
        {
            if (!Directory.Exists(widgetsDataRoot))
            {
                return null;
            }

            string targetId = SanitizeWidgetId(targetWidgetId);
            string? candidateDir = null;
            DateTime candidateWriteUtc = DateTime.MinValue;

            foreach (string dir in Directory.EnumerateDirectories(widgetsDataRoot))
            {
                string dirId = Path.GetFileName(dir);
                if (string.IsNullOrWhiteSpace(dirId) ||
                    string.Equals(dirId,targetId,StringComparison.OrdinalIgnoreCase) ||
                    claimedWidgetIds.Contains(dirId))
                {
                    continue;
                }

                string storePath = Path.Combine(dir,"todo.json");
                if (!File.Exists(storePath))
                {
                    continue;
                }

                // Only stores holding at least one live item are worth
                // resurrecting; an empty or all-deleted orphan stays put.
                if (!await OrphanedStoreHasLiveItemAsync(storePath,cancellationToken))
                {
                    continue;
                }

                DateTime writeUtc = File.GetLastWriteTimeUtc(storePath);
                if (candidateDir is null || writeUtc > candidateWriteUtc)
                {
                    candidateDir = dir;
                    candidateWriteUtc = writeUtc;
                }
            }

            if (candidateDir is null)
            {
                return null;
            }

            string targetDir = Path.Combine(widgetsDataRoot,targetId);
            if (Directory.Exists(targetDir))
            {
                // A fresh widget id should have no directory yet; only an
                // already-empty shell may be replaced — never merge into or
                // overwrite a directory that carries anything.
                if (Directory.EnumerateFileSystemEntries(targetDir).Any())
                {
                    return null;
                }

                Directory.Delete(targetDir);
            }

            string adoptedId = Path.GetFileName(candidateDir);
            Directory.Move(candidateDir,targetDir);
            await RebaseManagedAttachmentPathsAsync(
                widgetsDataRoot,adoptedId,targetDir,cancellationToken);
            App.Log(
                $"[TodoWidgetStore] Adopted orphaned todo store '{adoptedId}' for new widget '{targetId}'.");
            return adoptedId;
        }
        catch (Exception ex)
        {
            App.Log($"[TodoWidgetStore] Orphaned store adoption failed: {ex.Message}");
            return null;
        }
    }

    private static async Task<bool> OrphanedStoreHasLiveItemAsync(
        string storePath,        CancellationToken cancellationToken)
    {
        try
        {
            TodoWidgetData? data = JsonSerializer.Deserialize(
                await File.ReadAllTextAsync(storePath,cancellationToken),                TodoJsonContext.Default.StoreData);
            return data?.Items?.Any(item => item is not null && !item.IsDeleted) == true;
        }
        catch (Exception ex) when (ex is JsonException or IOException or InvalidDataException)
        {
            App.Log($"[TodoWidgetStore] Skipped unreadable orphaned store '{storePath}': {ex.Message}");
            return false;
        }
    }

    /// <summary>
    /// After the directory move, managed attachment FilePaths still carry
    /// the source widget id — rewrite the widgets/&lt;old&gt;/ prefix onto
    /// the new one. Unmanaged/external paths are never touched.
    /// </summary>
    private static async Task RebaseManagedAttachmentPathsAsync(
        string widgetsDataRoot,        string sourceWidgetId,        string targetDir,        CancellationToken cancellationToken)
    {
        string oldPrefix = Path.GetFullPath(
                               Path.Combine(widgetsDataRoot,sourceWidgetId)) +
                           Path.DirectorySeparatorChar;
        string newPrefix = Path.GetFullPath(targetDir) + Path.DirectorySeparatorChar;

        var store = new TodoWidgetStore(widgetsDataRoot,Path.GetFileName(targetDir));
        TodoWidgetData data = await store.LoadAsync();
        bool changed = false;
        foreach (TodoAttachment attachment in (data.Items ?? [])
                     .SelectMany(item => item.Attachments ?? [])
                     .Where(attachment => attachment is not null && attachment.IsManagedCopy))
        {
            if (!string.IsNullOrWhiteSpace(attachment.FilePath) &&
                attachment.FilePath.StartsWith(oldPrefix,StringComparison.OrdinalIgnoreCase))
            {
                attachment.FilePath = newPrefix + attachment.FilePath[oldPrefix.Length..];
                changed = true;
            }
        }

        if (changed)
        {
            await store.SaveAsync(data);
        }
    }

}
