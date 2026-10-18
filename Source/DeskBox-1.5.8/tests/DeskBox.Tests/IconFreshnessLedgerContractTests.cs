using DeskBox.ViewModels;

namespace DeskBox.Tests;

/// <summary>
/// Pins the icon freshness ledger (2026-10-13): refresh merges own icon
/// invalidation by comparing each item's extraction-time size/timestamp
/// against the incoming snapshot. This replaces lifecycle-triggered
/// blanket icon clears (wake/unlock/second-instance/import/organize), which
/// made mapped-folder widgets re-extract every icon after every trigger —
/// the "icons load slowly, repeatedly" report. Unchanged items keep their
/// icons; changed items drop the reference and re-fetch under the
/// content-versioned cache key; recovery refreshes reset retry-state
/// markers so timed-out sources retry.
/// </summary>
public sealed class IconFreshnessLedgerContractTests
{
    [Fact]
    public void UnchangedItem_KeepsItsIcon_ChangedItem_Drops()
    {
        var stamp = new DateTime(2026, 10, 13, 12, 0, 0);

        // Fresh: same size and timestamp as recorded at extraction.
        Assert.False(WidgetViewModel.ShouldDropStaleHydratedIcon(
            hasIcon: true,
            hydratedFileSize: 100,
            hydratedStamp: stamp,
            sourceFileSize: 100,
            sourceStamp: stamp));
        // Stale: size moved (out-of-band replacement).
        Assert.True(WidgetViewModel.ShouldDropStaleHydratedIcon(
            hasIcon: true,
            hydratedFileSize: 100,
            hydratedStamp: stamp,
            sourceFileSize: 250,
            sourceStamp: stamp));
        // Stale: timestamp moved (edit / folder content change).
        Assert.True(WidgetViewModel.ShouldDropStaleHydratedIcon(
            hasIcon: true,
            hydratedFileSize: 100,
            hydratedStamp: stamp,
            sourceFileSize: 100,
            sourceStamp: stamp.AddMinutes(5)));
        // No reference to protect.
        Assert.False(WidgetViewModel.ShouldDropStaleHydratedIcon(
            hasIcon: false,
            hydratedFileSize: -1,
            hydratedStamp: default,
            sourceFileSize: 100,
            sourceStamp: stamp));
    }

    [Fact]
    public void Ledger_IsStampedAtEveryExtractionPoint()
    {
        string itemModel = File.ReadAllText(
            TestPaths.FromRepository("src/DeskBox/Models/WidgetItem.cs"));
        Assert.Contains("internal long HydratedIconFileSize", itemModel, StringComparison.Ordinal);
        Assert.Contains("internal DateTime HydratedIconStamp", itemModel, StringComparison.Ordinal);

        string hydration = File.ReadAllText(
            TestPaths.FromRepository("src/DeskBox/ViewModels/WidgetViewModel.ItemHydration.cs"));
        // Hydration application and the shortcut-icon refresh both stamp
        // (existence only — legal stamp sites may grow).
        Assert.Contains(
            "HydratedIconFileSize = item.FileSize;",
            hydration,
            StringComparison.Ordinal);

        string fileService = File.ReadAllText(
            TestPaths.FromRepository("src/DeskBox/Services/FileService.cs"));
        // Items created with a loaded icon stamp too.
        Assert.Contains(
            "HydratedIconFileSize = item.FileSize;",
            fileService,
            StringComparison.Ordinal);
    }

    [Fact]
    public void RefreshMerge_OwnsInvalidationThroughTheLedger()
    {
        string watchers = File.ReadAllText(
            TestPaths.FromRepository("src/DeskBox/ViewModels/WidgetViewModel.SortingAndWatchers.cs"));
        Assert.Contains(
            "ShouldDropStaleHydratedIcon(",
            watchers,
            StringComparison.Ordinal);
    }

    [Fact]
    public void RecoveryRefreshes_SkipTheBlanketClear_ButResetRetryState()
    {
        string operations = File.ReadAllText(
            TestPaths.FromRepository("src/DeskBox/ViewModels/WidgetViewModel.Operations.cs"));
        Assert.Contains(
            "public async Task RefreshFromConfigAsync(",
            operations,
            StringComparison.Ordinal);
        Assert.Contains(
            "bool clearIconCacheBeforeHydration = true",
            operations,
            StringComparison.Ordinal);
        // The wake-recovery property the blanket clear used to carry.
        Assert.Contains(
            "_fileService.ResetIconTransientFailures();",
            operations,
            StringComparison.Ordinal);

        string fileService = File.ReadAllText(
            TestPaths.FromRepository("src/DeskBox/Services/FileService.cs"));
        Assert.Contains(
            "IconHelper.ResetTransientFailures();",
            fileService,
            StringComparison.Ordinal);

        string iconHelper = File.ReadAllText(
            TestPaths.FromRepository("src/DeskBox/Helpers/IconHelper.cs"));
        Assert.Contains(
            "internal static void ResetTransientFailures()",
            iconHelper,
            StringComparison.Ordinal);
    }

    [Fact]
    public void UnchangedShortcut_KeepsItsTarget_ChangedShortcut_Rereads()
    {
        var stamp = new DateTime(2026, 10, 13, 12, 0, 0);

        // Fresh: same .lnk size/timestamp as recorded at extraction.
        Assert.False(WidgetViewModel.ShouldDropStaleHydratedShortcutTarget(
            targetIsShortcut: true,
            targetHydratedIconFileSize: 100,
            targetHydratedIconStamp: stamp,
            sourceIsShortcut: true,
            sourceFileSize: 100,
            sourceLastModified: stamp));
        // Stale: the .lnk was rewritten (target re-pointed).
        Assert.True(WidgetViewModel.ShouldDropStaleHydratedShortcutTarget(
            targetIsShortcut: true,
            targetHydratedIconFileSize: 100,
            targetHydratedIconStamp: stamp,
            sourceIsShortcut: true,
            sourceFileSize: 250,
            sourceLastModified: stamp));
        // Unstamped ledger (icon extraction never succeeded): no
        // information — preserve instead of wiping every merge.
        Assert.False(WidgetViewModel.ShouldDropStaleHydratedShortcutTarget(
            targetIsShortcut: true,
            targetHydratedIconFileSize: -1,
            targetHydratedIconStamp: default,
            sourceIsShortcut: true,
            sourceFileSize: 100,
            sourceLastModified: stamp));
        // Non-shortcuts never take this path.
        Assert.False(WidgetViewModel.ShouldDropStaleHydratedShortcutTarget(
            targetIsShortcut: false,
            targetHydratedIconFileSize: 100,
            targetHydratedIconStamp: stamp,
            sourceIsShortcut: false,
            sourceFileSize: 100,
            sourceLastModified: stamp));
    }

    [Fact]
    public void ShortcutTargetHydration_LoadsOnceAndSurvivesMerges()
    {
        string hydration = File.ReadAllText(
            TestPaths.FromRepository("src/DeskBox/ViewModels/WidgetViewModel.ItemHydration.cs"));
        // The loaded flag bounds the per-generation shell-link reads...
        Assert.Contains(
            "!item.IsShortcutTargetLoaded",
            hydration,
            StringComparison.Ordinal);
        // ...and only a successful read arms it.
        Assert.Contains(
            "item.IsShortcutTargetLoaded = true;",
            hydration,
            StringComparison.Ordinal);

        // Merge snapshots never read targets, so the merge must not wipe a
        // hydrated value (the two-defects-masked-each-other trap).
        string watchers = File.ReadAllText(
            TestPaths.FromRepository("src/DeskBox/ViewModels/WidgetViewModel.SortingAndWatchers.cs"));
        Assert.Contains(
            "string.IsNullOrWhiteSpace(target.TargetPath)",
            watchers,
            StringComparison.Ordinal);
        Assert.Contains(
            "ShouldDropStaleHydratedShortcutTarget(",
            watchers,
            StringComparison.Ordinal);
    }

    [Fact]
    public void LifecycleImportAndOrganizeTriggers_NoLongerBlanketClearIcons()
    {
        // Import into mapped folders, shortcut drops, quick capture, and
        // RefreshFileWidgetAsync (wake/unlock recovery, desktop organize,
        // storage migration) all opt out; the structure-change default
        // stays true for API compatibility.
        string[] sites =
        [
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.ItemVisuals.cs",
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.ShortcutDrop.cs",
            "src/DeskBox/Services/WidgetManager.FeatureWidgets.cs",
            "src/DeskBox/Services/WidgetManager.cs"
        ];

        // Existence per trigger family, not an exact census: adding a
        // legal opt-out site must not break this pin, while reintroducing
        // a blanket icon clear at any of these triggers would.
        foreach (string site in sites)
        {
            Assert.Contains(
                "RefreshFromConfigAsync(clearIconCacheBeforeHydration: false)",
                File.ReadAllText(TestPaths.FromRepository(site)),
                StringComparison.Ordinal);
        }
    }

}
