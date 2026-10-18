using System.Security.Cryptography;
using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.ViewModels;

public partial class WidgetViewModel
{
    // Opt-in metadata preserves the existing settings schema and legacy folder behavior.
    public bool IsAiReferenceView => Config.Metadata.TryGetValue("naiwa.referenceView",out var value) && value == "1";

    // Shortcut views own only their generated links; the original files never
    // live inside the view, so the same read-only rules apply.
    public bool IsAiShortcutView => Config.Metadata.TryGetValue("naiwa.shortcutView",out var value) && value == "1";

    public bool IsAiOwnedView => IsAiReferenceView || IsAiShortcutView;

    private async Task LoadAiReferencesAsync(CancellationToken cancellationToken = default)
    {
        await _folderRefreshGate.WaitAsync(cancellationToken);
        try
        {
            var items = new List<WidgetItem>();
            foreach (var reference in Config.Items.ToArray())
            {
                cancellationToken.ThrowIfCancellationRequested();
                try
                {
                    var item = await _fileService.TryCreateWidgetItemAsync(reference.Path,                        hideShortcutArrowOverlay: _hideShortcutArrowOverlay,                        showImageFilesAsIcons: _showImageFilesAsIcons,                        showFileExtensions: _showFileExtensions,                        hideShortcutExtensionWhenShowingFileExtensions: _hideShortcutExtensionWhenShowingFileExtensions,                        loadIcon: false,loadFolderItemCount: false,loadShortcutTarget: false);
                    if (item is not null) items.Add(item);
                }
                catch (Exception ex) when (ex is IOException or UnauthorizedAccessException or ArgumentException) { }
            }
            cancellationToken.ThrowIfCancellationRequested();
            if (_isDisposed) return;
            SyncFolderItems(items); SortItems(); StartItemHydration(); UpdateDependentProperties();
        }
        finally { _folderRefreshGate.Release(); }
    }

    private async Task<IReadOnlyList<string>> ImportAiReferencesAsync(IEnumerable<string> paths,CancellationToken cancellationToken)
    {
        var previous = Config.Items.ToList();
        var known = Config.Items.Select(i => i.Path).ToHashSet(StringComparer.OrdinalIgnoreCase);
        var added = new List<string>();
        foreach (var path in paths)
        {
            cancellationToken.ThrowIfCancellationRequested();
            string full = Path.GetFullPath(path);
            if ((!File.Exists(full) && !Directory.Exists(full)) || !known.Add(full)) continue;
            Config.Items.Add(new WidgetItemConfig { Path = full, SortOrder = Config.Items.Count }); added.Add(full);
        }
        if (!await _settingsService.SaveCheckedAsync(notifySubscribers: false))
        { Config.Items = previous; throw new IOException("无法保存分组引用，原文件未改动。"); }
        await LoadAiReferencesAsync(cancellationToken);
        return added;
    }

    private async Task<FileDeleteBatchResult> RemoveAiReferencesAsync(IEnumerable<WidgetItem> items)
    {
        var paths = items.Select(i => i.Path).ToHashSet(StringComparer.OrdinalIgnoreCase);
        var previous = Config.Items.ToList();
        int removed = Config.Items.RemoveAll(i => paths.Contains(i.Path));
        if (!await _settingsService.SaveCheckedAsync(notifySubscribers: false))
        { Config.Items = previous; throw new IOException("无法保存分组引用，原文件未改动。"); }
        await LoadAiReferencesAsync();
        return new FileDeleteBatchResult(removed,[]);
    }

    // A drop onto a shortcut view creates an owned link next to the existing
    // ones: the dropped original stays exactly where it was, and the recorded
    // hash keeps the matching undo rule able to remove only this new link.
    private async Task<IReadOnlyList<string>> ImportAiShortcutReferencesAsync(
        IEnumerable<string> paths,CancellationToken cancellationToken)
    {
        string? folder = MappedFolderPath;
        if (string.IsNullOrWhiteSpace(folder)) return [];
        Directory.CreateDirectory(folder);
        var added = new List<string>();
        foreach (string path in paths)
        {
            cancellationToken.ThrowIfCancellationRequested();
            string full = Path.GetFullPath(path);
            if (!File.Exists(full) && !Directory.Exists(full)) continue;
            string link = FileService.GetAvailablePath(Path.Combine(folder,Path.GetFileName(full) + ".lnk"));
            DragDropPermissionService.CreateOrUpdateShortcut(link,full,string.Empty,iconPath: null);
            using (var stream = File.OpenRead(link))
                Config.Metadata["naiwa.linkhash." + Path.GetFileName(link)] = Convert.ToHexString(SHA256.HashData(stream));
            added.Add(full);
        }
        if (added.Count == 0) return [];
        if (!await _settingsService.SaveCheckedAsync(notifySubscribers: false))
            throw new IOException("引用文件已创建，但记录保存失败；原文件未移动。");
        await RefreshFolderContentsAsync();
        return added;
    }
}
