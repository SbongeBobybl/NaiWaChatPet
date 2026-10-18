using System.Security.Cryptography;
using DeskBox.Models;
using Windows.Graphics;

namespace DeskBox.Services;

public sealed partial class DesktopOrganizationCoordinator
{
    public async Task<(int Created, int Retained)> CreateShortcutViewsAsync(DesktopOrganizationPlan plan,        CancellationToken cancellationToken,bool rememberRules = false)
    {
        if (!plan.IsAiPlan || plan.OrganizationMode != AiOrganizationMode.Shortcuts)
            throw new InvalidOperationException("需要快捷方式计划。");
        LastCarryAnimationSkipped = false;
        LastCarrySkipReason = null;
        await DesktopOrganizationTransaction.OperationGate.WaitAsync(cancellationToken);
        var created = new List<WidgetConfig>();
        var createdByTarget = new List<(DesktopOrganizationTargetPlan Target,WidgetConfig Config,string Folder)>();
        var ruleTargets = new List<DesktopOrganizationTargetPlan>();
        int links = 0, retained = 0;
        var carry = new NaiWaCarryCoordinator(_settingsService);
        carry.BeginRun();
        string? wholeRunReceipt = null;
        bool wholeRunCompleted = false;
        try
        {
            if (_settingsService.Settings.Widgets.Any(w => w.Metadata.GetValueOrDefault("naiwa.viewPlan") == plan.Id))
                throw new InvalidOperationException("此引用计划已经执行，请刷新后检查结果。");
            await RemovePreviousNaiWaViewsAsync(cancellationToken);
            AssignStrictAiBounds(plan);
            // Phase 1: publish every empty shortcut grid before any link is built.
            int categoryIndex = 0;
            foreach (var target in plan.Targets)
            {
                cancellationToken.ThrowIfCancellationRequested();
                var bounds = target.PlannedBounds ?? throw new InvalidOperationException("缺少布局。");
                string id = Guid.NewGuid().ToString("N");
                string folder = Path.Combine(plan.StorageRootPath,"NaiWa-References",id);
                if (Directory.Exists(folder) || File.Exists(folder)) throw new IOException("引用目录已存在。");
                var config = new WidgetConfig
                {
                    Id = id, Name = target.SuggestedDisplayName, IsDefaultTitle = false,
                    WidgetKind = WidgetKind.File, Width = 240, Height = 180, IsVisible = true,
                    IsCollapsed = target.Collapsed, IsPositionLocked = target.Locked,
                    BoundsCoordinateVersion = WidgetConfig.CurrentBoundsCoordinateVersion,
                    MappedFolderPath = folder, FollowsDefaultStoragePath = false,
                    Metadata = new() { ["naiwa.shortcutView"] = "1", ["naiwa.viewPlan"] = plan.Id,
                        ["naiwa.createdUtc"] = DateTimeOffset.UtcNow.ToString("O") }
                };
                _widgetManager.ApplyNewWidgetPlacement(config,new NewWidgetPlacementContext(new RectInt32(
                    (int)bounds.X,(int)bounds.Y,(int)bounds.Width,(int)bounds.Height)));
                _settingsService.Settings.Widgets.Add(config);
                // Persist ownership before creating links. Incomplete creations stay visible and undoable.
                if (!await _settingsService.SaveCheckedAsync(notifySubscribers: false))
                { _settingsService.Settings.Widgets.Remove(config); throw new IOException("无法保存引用创建记录。"); }
                created.Add(config); Directory.CreateDirectory(folder);
                await _widgetManager.ShowWidgetAsync(config.Id,reveal: true,autoRestoreOnReveal: false);
                StampCategoryIcon(config,categoryIndex++);
                createdByTarget.Add((target.CloneWith(config.Id,target.SuggestedDisplayName,
                    target.TargetDirectoryPath,target.CreatesWidget,target.Items),config,folder));
            }

            if (createdByTarget.Count > 0)
                await Task.Delay(450,cancellationToken);

            bool simpleAnimation =
                NaiWaAnimationPolicy.Read(_settingsService.Settings) == NaiWaAnimationPolicy.Simple;
            wholeRunReceipt = simpleAnimation
                ? await PlayWholeRunTripAsync(
                    createdByTarget.Select(pair => (pair.Target,pair.Config)).ToArray(),
                    carry,
                    cancellationToken)
                : null;

            // A single failed trip must not mute the remaining categories:
            // the reported defect was that only the first category animated.
            // Two consecutive failures mean the pet is really unavailable,
            // and only then is the rest of the run filled silently.
            int animationFailures = 0;
            foreach (var (target,config,folder) in createdByTarget)
            {
                ruleTargets.Add(target.CloneWith(config.Id,target.SuggestedDisplayName,                    target.TargetDirectoryPath,target.CreatesWidget,target.Items));
                foreach (var batch in target.Items.Chunk(40))
                {
                    cancellationToken.ThrowIfCancellationRequested();
                    var ready = batch.Where(ShortcutSourceUnchanged).ToArray();
                    retained += batch.Length - ready.Length;
                    if (ready.Length == 0) continue;
                    string? receipt = null;
                    if (!simpleAnimation)
                    {
                        try
                        {
                            // Carry IPC and the pet stage watchdog own the timeout.
                            // A second ten-second deadline used to cancel the delivery leg.
                            receipt = await carry.BeforeBatchAsync(ready,target,cancellationToken);
                            animationFailures = 0;
                        }
                        catch (Exception ex) when (!cancellationToken.IsCancellationRequested &&
                            ex is (IOException or OperationCanceledException or UnauthorizedAccessException or TimeoutException))
                        {
                            // Shortcut creation never moves the source; keep it usable
                            // even when the pet is stuck in its decorative animation.
                            animationFailures++;
                            LastCarryAnimationSkipped = true;
                            LastCarrySkipReason = ex.Message;
                            App.Log($"[NaiWa] Shortcut animation skipped for {target.SuggestedDisplayName} (failure {animationFailures}): {ex.GetType().Name}: {ex.Message}");
                        }
                    }
                    bool committed = false;
                    try
                    {
                        foreach (var item in ready)
                        {
                            cancellationToken.ThrowIfCancellationRequested();
                            if (!ShortcutSourceUnchanged(item)) throw new IOException("搬运期间项目已变化，请重新扫描。");
                            bool copyLink = item.Extension.Equals(".lnk",StringComparison.OrdinalIgnoreCase)
                                || item.Extension.Equals(".url",StringComparison.OrdinalIgnoreCase);
                            string link = FileService.GetAvailablePath(Path.Combine(folder,copyLink ? item.Name : item.Name + ".lnk"));
                            if (copyLink) File.Copy(item.SourcePath,link,overwrite: false);
                            else DragDropPermissionService.CreateOrUpdateShortcut(link,item.SourcePath,string.Empty,iconPath: null);
                            using (var stream = File.OpenRead(link))
                                config.Metadata["naiwa.linkhash." + Path.GetFileName(link)] = Convert.ToHexString(SHA256.HashData(stream));
                            if (!await _settingsService.SaveCheckedAsync(notifySubscribers: false))
                                throw new IOException("引用已创建但记录保存失败，原文件未移动。");
                            links++;
                        }
                        committed = true;
                        await _widgetManager.RefreshFileWidgetAsync(config.Id);
                    }
                    finally
                    {
                        if (receipt is not null)
                        {
                            try { await carry.AfterMoveAsync(receipt,committed); }
                            catch (Exception ex) { App.Log($"[NaiWa] Batch shortcut receipt failed: {ex.GetType().Name}"); }
                        }
                    }
                }
            }
            wholeRunCompleted = true;
        }
        finally
        {
            try
            {
                if (wholeRunReceipt is not null)
                    await carry.AfterMoveAsync(wholeRunReceipt,wholeRunCompleted);
            }
            catch (Exception ex) { App.Log($"[NaiWa] Whole-run receipt failed: {ex.GetType().Name}"); }
            finally { DesktopOrganizationTransaction.OperationGate.Release(); }
        }
        int remembered = 0;
        if (rememberRules && ruleTargets.Count > 0)
        {
            await DesktopOrganizationTransaction.OperationGate.WaitAsync();
            try { remembered = await RememberCreatedRulesAsync(ruleTargets,true); }
            finally { DesktopOrganizationTransaction.OperationGate.Release(); }
        }
        App.Log($"[NaiWa] Shortcut references links={links} retained={retained} rememberedRules={remembered}");
        return (links, retained);
    }

    private static bool ShortcutSourceUnchanged(DesktopOrganizationFileSnapshot item)
    {
        FileSystemInfo info = item.IsDirectory ? new DirectoryInfo(item.SourcePath) : new FileInfo(item.SourcePath);
        return info.Exists && (info is not FileInfo file || file.Length == item.Size)
            && info.LastWriteTimeUtc == item.LastWriteTimeUtc && (info.Attributes & FileAttributes.ReparsePoint) == 0;
    }

    private static void RemoveUnchangedOwnedLinks(WidgetConfig widget)
    {
        string? folder = widget.MappedFolderPath;
        if (folder is null || !Directory.Exists(folder) || (File.GetAttributes(folder) & FileAttributes.ReparsePoint) != 0) return;
        foreach (var pair in widget.Metadata.Where(p => p.Key.StartsWith("naiwa.linkhash.",StringComparison.Ordinal)))
        {
            string name = pair.Key["naiwa.linkhash.".Length..];
            if (Path.GetFileName(name) != name) continue;
            string path = Path.Combine(folder,name);
            if (!File.Exists(path) || (File.GetAttributes(path) & FileAttributes.ReparsePoint) != 0) continue;
            string hash; using (var stream = File.OpenRead(path)) hash = Convert.ToHexString(SHA256.HashData(stream));
            if (hash == pair.Value) File.Delete(path);
        }
        // Never recursively remove a folder: user additions and changed links survive undo.
        if (!Directory.EnumerateFileSystemEntries(folder).Any()) Directory.Delete(folder);
    }
}
