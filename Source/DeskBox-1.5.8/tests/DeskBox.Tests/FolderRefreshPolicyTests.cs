using DeskBox.Services;
using DeskBox.ViewModels;

namespace DeskBox.Tests;

public sealed class FolderRefreshPolicyTests
{
    /// <summary>
    /// Pins the folder-count freshness rule for refresh merges: snapshots
    /// are enumerated with folder item counts unloaded, so an unchanged
    /// folder (same LastWriteTime) keeps its loaded count instead of being
    /// re-enumerated on every activation reconciliation.
    /// </summary>
    [Fact]
    public void UnchangedFolder_KeepsItsLoadedCount()
    {
        var stamp = new DateTime(2026, 10, 13, 12, 0, 0);

        Assert.True(WidgetViewModel.DeriveFolderItemCountLoaded(
            targetIsFolder: true,
            targetIsLoaded: true,
            targetLastModified: stamp,
            sourceIsFolder: true,
            sourceIsLoaded: false,
            sourceLastModified: stamp));
    }

    [Fact]
    public void ChangedFolder_LosesItsLoadedCount()
    {
        Assert.False(WidgetViewModel.DeriveFolderItemCountLoaded(
            targetIsFolder: true,
            targetIsLoaded: true,
            targetLastModified: new DateTime(2026, 10, 13, 12, 0, 0),
            sourceIsFolder: true,
            sourceIsLoaded: false,
            sourceLastModified: new DateTime(2026, 10, 13, 12, 30, 0)));
    }

    [Fact]
    public void UnloadedFolder_AndNonFolderItems_FollowTheSnapshot()
    {
        var stamp = new DateTime(2026, 10, 13, 12, 0, 0);

        // Never loaded on the target: nothing to keep.
        Assert.False(WidgetViewModel.DeriveFolderItemCountLoaded(
            targetIsFolder: true,
            targetIsLoaded: false,
            targetLastModified: stamp,
            sourceIsFolder: true,
            sourceIsLoaded: false,
            sourceLastModified: stamp));
        // Files are born loaded in snapshots; the source-loaded clause wins
        // regardless of item kind.
        Assert.True(WidgetViewModel.DeriveFolderItemCountLoaded(
            targetIsFolder: false,
            targetIsLoaded: true,
            targetLastModified: stamp,
            sourceIsFolder: false,
            sourceIsLoaded: true,
            sourceLastModified: stamp));
        Assert.True(WidgetViewModel.DeriveFolderItemCountLoaded(
            targetIsFolder: true,
            targetIsLoaded: false,
            targetLastModified: stamp,
            sourceIsFolder: true,
            sourceIsLoaded: true,
            sourceLastModified: stamp.AddDays(1)));
    }

    /// <summary>
    /// The freshness decision must be derived before the merge overwrites
    /// <c>target.LastModified</c> with the incoming snapshot value — after
    /// the overwrite the timestamps always compare equal and the rule
    /// silently degenerates to "always keep".
    /// </summary>
    [Fact]
    public void FolderCountFreshness_IsDerivedBeforeTheTimestampOverwrite()
    {
        string watchers = File.ReadAllText(
            TestPaths.FromRepository(
                "src/DeskBox/ViewModels/WidgetViewModel.SortingAndWatchers.cs"));
        int derive = watchers.IndexOf(
            "DeriveFolderItemCountLoaded(",
            StringComparison.Ordinal);
        int overwrite = watchers.IndexOf(
            "target.LastModified = source.LastModified;",
            StringComparison.Ordinal);
        Assert.True(
            derive >= 0 && overwrite > derive,
            "The folder-count freshness decision must be derived before " +
            "LastModified is overwritten in the merge.");
    }

    [Fact]
    public async Task CompleteParentSnapshot_DistinguishesPresentFromNotFound()
    {
        string root = Path.Combine(Path.GetTempPath(), $"DeskBox-refresh-{Guid.NewGuid():N}");
        Directory.CreateDirectory(root);
        try
        {
            string present = Path.Combine(root, "present.txt");
            string missing = Path.Combine(root, "missing.txt");
            await File.WriteAllTextAsync(present, "ready");

            FolderPathSnapshot snapshot =
                await FileService.CaptureDirectChildSnapshotAsync(root);

            Assert.Equal(FolderSnapshotStatus.Complete, snapshot.Status);
            Assert.Equal(
                FolderEntryRefreshStatus.Available,
                FileService.ClassifyDirectChild(snapshot, present));
            Assert.Equal(
                FolderEntryRefreshStatus.NotFound,
                FileService.ClassifyDirectChild(snapshot, missing));
        }
        finally
        {
            Directory.Delete(root, recursive: true);
        }
    }

    [Fact]
    public async Task EmptyParentSnapshot_IsSuccessfulEmptyNotUnavailable()
    {
        string root = Path.Combine(Path.GetTempPath(), $"DeskBox-empty-{Guid.NewGuid():N}");
        Directory.CreateDirectory(root);
        try
        {
            FolderPathSnapshot snapshot =
                await FileService.CaptureDirectChildSnapshotAsync(root);

            Assert.Equal(FolderSnapshotStatus.SuccessEmpty, snapshot.Status);
            Assert.True(FolderSnapshotStatusPolicy.IsSuccessful(snapshot.Status));
            Assert.Empty(snapshot.Paths);
        }
        finally
        {
            Directory.Delete(root, recursive: true);
        }
    }

    [Fact]
    public void FolderSnapshotPolicy_RejectsIncompleteStates()
    {
        Assert.True(FolderSnapshotStatusPolicy.IsSuccessful(
            FolderSnapshotStatus.SuccessWithItems));
        Assert.True(FolderSnapshotStatusPolicy.IsSuccessful(
            FolderSnapshotStatus.SuccessEmpty));
        Assert.False(FolderSnapshotStatusPolicy.IsSuccessful(
            FolderSnapshotStatus.Partial));
        Assert.False(FolderSnapshotStatusPolicy.IsSuccessful(
            FolderSnapshotStatus.Unavailable));
        Assert.False(FolderSnapshotStatusPolicy.IsSuccessful(
            FolderSnapshotStatus.AccessDenied));
    }

    [Fact]
    public async Task UnavailableParentSnapshot_NeverClaimsThatChildWasDeleted()
    {
        string root = Path.Combine(Path.GetTempPath(), $"DeskBox-offline-{Guid.NewGuid():N}");
        string child = Path.Combine(root, "retained.txt");

        FolderPathSnapshot snapshot =
            await FileService.CaptureDirectChildSnapshotAsync(root);

        Assert.Equal(FolderSnapshotStatus.Unavailable, snapshot.Status);
        Assert.Equal(
            FolderEntryRefreshStatus.Unavailable,
            FileService.ClassifyDirectChild(snapshot, child));
        Assert.False(WidgetViewModel.ShouldRemoveExistingItem(
            WatcherChangeTypes.Deleted,
            FolderEntryRefreshStatus.Unavailable));
    }

    [Fact]
    public void ShellMoveCompletion_RequiresDestinationAndMissingSource()
    {
        string root = Path.Combine(Path.GetTempPath(), $"DeskBox-shell-{Guid.NewGuid():N}");
        Directory.CreateDirectory(root);
        try
        {
            string source = Path.Combine(root, "source.txt");
            string destination = Path.Combine(root, "destination.txt");
            File.WriteAllText(source, "source");
            File.WriteAllText(destination, "destination");

            Assert.False(FileService.IsCompletedShellMove(source, destination));

            File.Delete(source);
            Assert.True(FileService.IsCompletedShellMove(source, destination));
        }
        finally
        {
            Directory.Delete(root, recursive: true);
        }
    }

    [Fact]
    public void ShellMoveBatchCompletion_RequiresEveryMoveToBeComplete()
    {
        string root = Path.Combine(
            Path.GetTempPath(),
            $"DeskBox-shell-batch-{Guid.NewGuid():N}");
        Directory.CreateDirectory(root);
        try
        {
            string firstSource = Path.Combine(root, "first-source.txt");
            string firstDestination = Path.Combine(root, "first-destination.txt");
            string secondSource = Path.Combine(root, "second-source.txt");
            string secondDestination = Path.Combine(root, "second-destination.txt");
            File.WriteAllText(firstDestination, "first");
            File.WriteAllText(secondSource, "second");
            File.WriteAllText(secondDestination, "second");

            var plans = new[]
            {
                new FileService.FileTransferPlan(firstSource, firstDestination),
                new FileService.FileTransferPlan(secondSource, secondDestination)
            };

            // The retired batch helper was exactly this fold over the
            // shared per-move completion predicate (the first source never
            // exists, so only the second plan starts incomplete).
            Assert.False(plans.All(plan => FileService.IsCompletedShellMove(
                plan.SourcePath,
                plan.DestinationPath)));

            File.Delete(secondSource);
            Assert.True(plans.All(plan => FileService.IsCompletedShellMove(
                plan.SourcePath,
                plan.DestinationPath)));
        }
        finally
        {
            Directory.Delete(root, recursive: true);
        }
    }

    [Theory]
    [InlineData(WatcherChangeTypes.Deleted, (int)FolderEntryRefreshStatus.NotFound, true)]
    [InlineData(WatcherChangeTypes.Changed, (int)FolderEntryRefreshStatus.NotFound, false)]
    [InlineData(WatcherChangeTypes.Deleted, (int)FolderEntryRefreshStatus.Unavailable, false)]
    [InlineData(WatcherChangeTypes.Changed, (int)FolderEntryRefreshStatus.Filtered, true)]
    public void IncrementalRemoval_RequiresExplicitTerminalState(
        WatcherChangeTypes changeType,
        int state,
        bool expected)
    {
        Assert.Equal(
            expected,
            WidgetViewModel.ShouldRemoveExistingItem(
                changeType,
                (FolderEntryRefreshStatus)state));
    }

    [Fact]
    public void NativeWatcher_CoversFileAndDirectoryMetadataChanges()
    {
        NotifyFilters filters = FolderWatcherService.NativeNotifyFilter;

        Assert.True(filters.HasFlag(NotifyFilters.FileName));
        Assert.True(filters.HasFlag(NotifyFilters.DirectoryName));
        Assert.True(filters.HasFlag(NotifyFilters.LastWrite));
        Assert.True(filters.HasFlag(NotifyFilters.Size));
        Assert.True(filters.HasFlag(NotifyFilters.CreationTime));
        Assert.True(filters.HasFlag(NotifyFilters.Attributes));
        Assert.True(FolderWatcherService.NativeBufferSizeBytes >= 32 * 1024);
    }

    [Fact]
    public void FolderWatcherHealthSnapshot_PreservesReconnectDiagnostics()
    {
        var snapshot = new FolderWatcherHealthSnapshot(
            "Z:\\Mapped",
            FolderWatcherHealth.Unavailable,
            NativeWatcherActive: false,
            QueryWatcherActive: false,
            ReconnectPending: true,
            ReconnectCount: 2,
            LastEventAt: null,
            LastError: "offline");

        Assert.Equal(FolderWatcherHealth.Unavailable, snapshot.Status);
        Assert.True(snapshot.ReconnectPending);
        Assert.Equal(2, snapshot.ReconnectCount);
    }

    [Fact]
    public void IsCurrentWatcherBatch_AcceptsMappedFolderAndRejectsStaleFolder()
    {
        string mapped = Path.Combine(Path.GetTempPath(), "DeskBox", "mapped");
        string stale = Path.Combine(Path.GetTempPath(), "DeskBox", "stale");

        Assert.True(WidgetViewModel.IsCurrentWatcherBatch(
            new FolderChangeBatch(mapped, [], false),
            mapped));
        Assert.False(WidgetViewModel.IsCurrentWatcherBatch(
            new FolderChangeBatch(stale, [], true),
            mapped));

        Assert.Equal(
            7,
            new FolderChangeBatch(mapped, [], false, Generation: 7).Generation);
    }

    [Fact]
    public void IsCurrentWatcherBatch_AcceptsPublicDesktopForCombinedDesktopWidget()
    {
        var (userDesktop, publicDesktop) = FileService.GetDesktopPaths();

        Assert.True(WidgetViewModel.IsCurrentWatcherBatch(
            new FolderChangeBatch(publicDesktop, [], true),
            userDesktop));
    }
}
