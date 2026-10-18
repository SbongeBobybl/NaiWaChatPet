using DeskBox.Models;
using Windows.Graphics;

namespace DeskBox.Services;

public sealed partial class DesktopOrganizationCoordinator
{
    public bool LastCarryAnimationSkipped { get; private set; }

    /// <summary>
    /// Why the last run had to skip an animation, or null when every trip
    /// played. Surfaced in the workbench message so a skipped animation can be
    /// diagnosed from the UI instead of guessed at.
    /// </summary>
    public string? LastCarrySkipReason { get; private set; }

    public async Task<int> CreateReferenceViewsAsync(DesktopOrganizationPlan plan,CancellationToken cancellationToken,        bool rememberRules = false)
    {
        if (!plan.IsAiPlan || plan.OrganizationMode != AiOrganizationMode.ViewOnly)
            throw new InvalidOperationException("仅视图计划才能创建分组引用。");
        LastCarryAnimationSkipped = false;
        LastCarrySkipReason = null;
        await DesktopOrganizationTransaction.OperationGate.WaitAsync(cancellationToken);
        var created = new List<WidgetConfig>();
        var createdByTarget = new List<(DesktopOrganizationTargetPlan Target,WidgetConfig Config)>();
        var ruleTargets = new List<DesktopOrganizationTargetPlan>();
        var carry = new NaiWaCarryCoordinator(_settingsService);
        carry.BeginRun();
        string? wholeRunReceipt = null;
        bool wholeRunCompleted = false;
        try
        {
            if (_settingsService.Settings.Widgets.Any(w => w.Metadata.GetValueOrDefault("naiwa.viewPlan") == plan.Id)) return 0;
            // A new classification replaces the previous one: the old boxes are
            // closed and removed first, so re-running the organizer never leaves
            // two generations of identical categories on the desktop.
            await RemovePreviousNaiWaViewsAsync(cancellationToken);
            AssignStrictAiBounds(plan);
            // Phase 1: publish every empty category grid first. The user sees
            // the complete set of destinations before the pet starts carrying.
            int categoryIndex = 0;
            foreach (var target in plan.Targets)
            {
                cancellationToken.ThrowIfCancellationRequested();
                var bounds = target.PlannedBounds ?? throw new InvalidOperationException("缺少已验证布局。");
                var config = new WidgetConfig
                {
                    Id = Guid.NewGuid().ToString("N"), Name = target.SuggestedDisplayName, IsDefaultTitle = false,
                    WidgetKind = WidgetKind.File, Width = 240, Height = 180, IsVisible = true,
                    IsCollapsed = target.Collapsed, IsPositionLocked = target.Locked,
                    BoundsCoordinateVersion = WidgetConfig.CurrentBoundsCoordinateVersion,
                    FollowsDefaultStoragePath = false, MappedFolderPath = null,
                    Metadata = new() { ["naiwa.referenceView"] = "1", ["naiwa.viewPlan"] = plan.Id,
                        ["naiwa.createdUtc"] = DateTimeOffset.UtcNow.ToString("O") }
                };
                _widgetManager.ApplyNewWidgetPlacement(config,new NewWidgetPlacementContext(new RectInt32(
                    (int)Math.Round(bounds.X),(int)Math.Round(bounds.Y),(int)Math.Round(bounds.Width),(int)Math.Round(bounds.Height))));
                StampCategoryIcon(config,categoryIndex++);
                _settingsService.Settings.Widgets.Add(config);
                if (!await _settingsService.SaveCheckedAsync(notifySubscribers: false))
                { _settingsService.Settings.Widgets.Remove(config); throw new IOException("无法保存分类框。"); }
                created.Add(config);
                await _widgetManager.ShowWidgetAsync(config.Id,reveal: true,autoRestoreOnReveal: false);
                createdByTarget.Add((target.CloneWith(config.Id,target.SuggestedDisplayName,
                    target.TargetDirectoryPath,target.CreatesWidget,target.Items),config));
            }

            if (createdByTarget.Count > 0)
                await Task.Delay(450,cancellationToken);

            // Simple animation: one trip for the whole classification. Full
            // animation instead plays one trip per category inside the loop.
            bool simpleAnimation =
                NaiWaAnimationPolicy.Read(_settingsService.Settings) == NaiWaAnimationPolicy.Simple;
            wholeRunReceipt = simpleAnimation
                ? await PlayWholeRunTripAsync(createdByTarget,carry,cancellationToken)
                : null;

            // Phase 2: fill the already-visible grids. A single failed trip
            // must not mute the remaining categories (that produced the
            // "only the first organize animates" defect); two consecutive
            // failures mean the pet is really unavailable.
            int animationFailures = 0;
            foreach (var (target,config) in createdByTarget)
            {
                foreach (var batch in target.Items.Chunk(40))
                {
                    cancellationToken.ThrowIfCancellationRequested();
                    foreach (var item in batch)
                        if (!File.Exists(item.SourcePath) && !Directory.Exists(item.SourcePath))
                            throw new IOException("桌面项目已变化，请重新扫描。已完成的引用仍可撤销。");
                    string? receipt = null;
                    if (!simpleAnimation)
                    {
                        try
                        {
                            // Carry IPC and the pet stage watchdog own the timeout.
                            // A second ten-second deadline used to cancel the delivery leg.
                            receipt = await carry.BeforeBatchAsync(batch,target,cancellationToken);
                            animationFailures = 0;
                        }
                        catch (Exception ex) when (!cancellationToken.IsCancellationRequested &&
                            ex is (IOException or OperationCanceledException or UnauthorizedAccessException or TimeoutException))
                        {
                            // The pet animation is decorative for a reference view.
                            // A busy/missing pet must not discard the classification.
                            animationFailures++;
                            LastCarryAnimationSkipped = true;
                            LastCarrySkipReason = ex.Message;
                            App.Log($"[NaiWa] Reference animation skipped for {target.SuggestedDisplayName} (failure {animationFailures}): {ex.GetType().Name}: {ex.Message}");
                        }
                    }
                    bool committed = false;
                    int previousCount = config.Items.Count;
                    try
                    {
                        cancellationToken.ThrowIfCancellationRequested();
                        foreach (var item in batch)
                        {
                            if (!File.Exists(item.SourcePath) && !Directory.Exists(item.SourcePath))
                                throw new IOException("搬运期间桌面项目已变化，请重新扫描。");
                            config.Items.Add(new WidgetItemConfig { Path = item.SourcePath, SortOrder = config.Items.Count });
                        }
                        if (!await _settingsService.SaveCheckedAsync(notifySubscribers: false))
                            throw new IOException("引用保存失败，原文件未移动。");
                        committed = true;
                        await _widgetManager.RefreshFileWidgetAsync(config.Id);
                    }
                    finally
                    {
                        if (!committed) config.Items.RemoveRange(previousCount,config.Items.Count - previousCount);
                        if (receipt is not null)
                        {
                            try { await carry.AfterMoveAsync(receipt,committed); }
                            catch (Exception ex) { App.Log($"[NaiWa] Batch reference receipt failed: {ex.GetType().Name}"); }
                        }
                    }
                }
                ruleTargets.Add(target.CloneWith(config.Id,target.SuggestedDisplayName,                    target.TargetDirectoryPath,target.CreatesWidget,target.Items));
            }


            wholeRunCompleted = true;
            await RememberCreatedRulesAsync(ruleTargets,rememberRules);
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
        return created.Count;
    }

    /// <summary>
    /// Optional "remember this classification" step. Rules route by widget
    /// identity and are evaluated after every protection, so remembering never
    /// widens what may be moved; the settings page can disable or delete them.
    /// </summary>
    private async Task<int> RememberCreatedRulesAsync(
        IReadOnlyList<DesktopOrganizationTargetPlan> targets,bool rememberRules)
    {
        if (!rememberRules || targets.Count == 0) return 0;
        var rules = _settingsService.Settings.DesktopOrganizationRules;
        int before = rules.Count;
        DesktopOrganizationRuleFactory.AddForNewTargets(targets,rules);
        if (await _settingsService.SaveCheckedAsync(notifySubscribers: false)) return rules.Count - before;
        rules.RemoveRange(before,rules.Count - before);
        App.Log("[NaiWa] Remembering the category rules failed; the saved views stay, no rule was written.");
        return 0;
    }

    /// <summary>
    /// Clears the boxes of a previous view-only run before a writing run
    /// (move / shortcuts). Keeping the old reference boxes would leave two
    /// boxes per category and make each move target ambiguous.
    /// </summary>
    internal Task PrepareNonViewExecutionAsync(CancellationToken cancellationToken) =>
        RemovePreviousNaiWaViewsAsync(cancellationToken);

    /// <summary>
    /// Closes and deletes every category box created by an earlier organizer
    /// run, keeping the link folders that still matter. Sources are never
    /// touched: only the views are replaced.
    /// </summary>
    internal async Task RemovePreviousNaiWaViewsAsync(CancellationToken cancellationToken)
    {
        var stale = _settingsService.Settings.Widgets
            .Where(widget =>
                widget.Metadata.GetValueOrDefault("naiwa.referenceView") == "1" ||
                widget.Metadata.GetValueOrDefault("naiwa.shortcutView") == "1")
            .ToArray();
        if (stale.Length == 0) return;

        foreach (WidgetConfig widget in stale)
        {
            cancellationToken.ThrowIfCancellationRequested();
            if (widget.Metadata.GetValueOrDefault("naiwa.shortcutView") == "1")
            {
                RemoveUnchangedOwnedLinks(widget);
            }

            await _widgetManager.RemoveWidgetAsync(widget.Id, WidgetRemovalAction.RemoveWidgetOnly);
        }

        await _settingsService.SaveCheckedAsync(notifySubscribers: false);
        App.Log($"[NaiWa] Replaced {stale.Length} category boxes from the previous run.");
    }

    /// <summary>Stamps a category icon variant onto a freshly created box.</summary>
    private static void StampCategoryIcon(WidgetConfig config, int index)
    {
        config.Metadata["naiwa.icon"] = WidgetTitleIconKindNames.ToSettingValue(
            WidgetTitleIconKindNames.NaiWaCategoryKindFor(index));
    }

    /// <summary>
    /// Simple animation = one trip for the whole classification: the pet
    /// collects everything once, then every category is filled. Returns the
    /// receipt so the caller can close the animation after filling.
    /// </summary>
    private async Task<string?> PlayWholeRunTripAsync(
        IReadOnlyList<(DesktopOrganizationTargetPlan Target, WidgetConfig Config)> targets,
        NaiWaCarryCoordinator carry,
        CancellationToken cancellationToken)
    {
        if (targets.Count == 0 || NaiWaAnimationPolicy.Read(_settingsService.Settings) != NaiWaAnimationPolicy.Simple)
            return null;

        DesktopOrganizationTargetPlan first = targets[0].Target;
        DesktopOrganizationFileSnapshot[] items = targets
            .SelectMany(pair => pair.Target.Items)
            .Where(item => item.IsEligible)
            .Take(40)
            .ToArray();
        if (items.Length == 0) return null;

        try
        {
            App.Log($"[NaiWa] Whole-run animation with {items.Length} items.");
            return await carry.BeforeBatchAsync(items, first, cancellationToken);
        }
        catch (Exception ex) when (!cancellationToken.IsCancellationRequested)
        {
            LastCarryAnimationSkipped = true;
            LastCarrySkipReason = ex.Message;
            App.Log($"[NaiWa] Whole-run animation skipped: {ex.GetType().Name}: {ex.Message}");
            return null;
        }
    }

    // View history lives in the persisted widget metadata. A crash during undo
    // leaves the remaining members discoverable, without any file rollback.
    public async Task<int> UndoLatestReferenceViewsAsync()
    {
        await DesktopOrganizationTransaction.OperationGate.WaitAsync();
        try
        {
            var views = _settingsService.Settings.Widgets.Where(w => w.Metadata.GetValueOrDefault("naiwa.referenceView") == "1"
                || w.Metadata.GetValueOrDefault("naiwa.shortcutView") == "1")
                .OrderByDescending(w => w.Metadata.GetValueOrDefault("naiwa.createdUtc"),StringComparer.Ordinal).ToArray();
            string? planId = views.FirstOrDefault()?.Metadata.GetValueOrDefault("naiwa.viewPlan");
            if (planId is null) return 0;
            var remove = views.Where(w => w.Metadata.GetValueOrDefault("naiwa.viewPlan") == planId).ToArray();
            foreach (var widget in remove)
            {
                if (widget.Metadata.GetValueOrDefault("naiwa.shortcutView") == "1") RemoveUnchangedOwnedLinks(widget);
                await _widgetManager.RemoveWidgetAsync(widget.Id,WidgetRemovalAction.RemoveWidgetOnly);
            }
            if (!await _settingsService.SaveCheckedAsync(notifySubscribers: false)) throw new IOException("视图撤销保存失败，请重试。原文件未改动。");
            return remove.Length;
        }
        finally { DesktopOrganizationTransaction.OperationGate.Release(); }
    }
}
