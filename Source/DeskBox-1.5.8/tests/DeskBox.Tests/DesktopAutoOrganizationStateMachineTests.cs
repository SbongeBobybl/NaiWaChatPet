using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class DesktopAutoOrganizationStateMachineTests
{
    [Theory]
    [InlineData(DeskBox.Models.DesktopOrganizationExclusionReason.Folder)]
    [InlineData(DeskBox.Models.DesktopOrganizationExclusionReason.HiddenOrSystem)]
    [InlineData(DeskBox.Models.DesktopOrganizationExclusionReason.ReparsePoint)]
    [InlineData(DeskBox.Models.DesktopOrganizationExclusionReason.TemporaryOrDownloading)]
    [InlineData(DeskBox.Models.DesktopOrganizationExclusionReason.SlowItem)]
    public void ScannerExclusions_AreTerminalIgnoredNotDeferred(
        DeskBox.Models.DesktopOrganizationExclusionReason reason)
    {
        Assert.Equal(
            DesktopAutoOrganizationItemState.Ignored,
            DesktopAutoOrganizationStatePolicy.ForSnapshotExclusion(reason, pathExists: true));
    }

    [Fact]
    public void UnavailableMetadata_RetriesWhenPresentAndIsMissingWhenGone()
    {
        Assert.Equal(
            DesktopAutoOrganizationItemState.Deferred,
            DesktopAutoOrganizationStatePolicy.ForSnapshotExclusion(
                DeskBox.Models.DesktopOrganizationExclusionReason.Unavailable,
                pathExists: true));
        Assert.Equal(
            DesktopAutoOrganizationItemState.Missing,
            DesktopAutoOrganizationStatePolicy.ForSnapshotExclusion(
                DeskBox.Models.DesktopOrganizationExclusionReason.Unavailable,
                pathExists: false));
    }

    [Fact]
    public void FiniteUnavailableRetry_StopsAtLimitWhileLockedRetryPersists()
    {
        DesktopAutoOrganizationRetryDecision transientAtLimit =
            DesktopAutoOrganizationRetrySchedule.Evaluate(
                DesktopAutoOrganizationRetryKind.Finite,
                nextAttempt: 8,
                fastRetryAttemptLimit: 8,
                persistentRetryDelay: TimeSpan.FromMinutes(2));
        DesktopAutoOrganizationRetryDecision transientOverLimit =
            DesktopAutoOrganizationRetrySchedule.Evaluate(
                DesktopAutoOrganizationRetryKind.Finite,
                nextAttempt: 9,
                fastRetryAttemptLimit: 8,
                persistentRetryDelay: TimeSpan.FromMinutes(2));
        DesktopAutoOrganizationRetryDecision lockedLongTerm =
            DesktopAutoOrganizationRetrySchedule.Evaluate(
                DesktopAutoOrganizationRetryKind.Persistent,
                nextAttempt: 99,
                fastRetryAttemptLimit: 8,
                persistentRetryDelay: TimeSpan.FromMinutes(2));

        Assert.True(transientAtLimit.ShouldRetry);
        Assert.False(transientOverLimit.ShouldRetry);
        Assert.True(lockedLongTerm.ShouldRetry);
        Assert.Equal(TimeSpan.FromMinutes(2), lockedLongTerm.Delay);
    }

    [Fact]
    public void ChangedGeneration_PreservesFiniteRetryBudget()
    {
        var stateMachine = new DesktopAutoOrganizationStateMachine();
        DateTimeOffset retryAt = new(2026, 8, 3, 12, 0, 0, TimeSpan.Zero);
        DesktopAutoOrganizationWorkItem first = stateMachine.BeginPending(@"C:\Desktop\unavailable.txt");
        Assert.True(stateMachine.MarkDeferred(first, retryAt));

        DesktopAutoOrganizationWorkItem changed = stateMachine.BeginPending(
            first.Path,
            preserveRetryAttempts: true);

        Assert.Equal(1, stateMachine.GetSnapshot(changed.Path)?.RetryAttempts);
        Assert.NotEqual(first.Generation, changed.Generation);
    }

    [Fact]
    public void IncompleteBaselineCapture_RetainsLastCompleteSnapshot()
    {
        var baseline = new DesktopAutoOrganizationBaseline();
        Assert.True(baseline.TryReplace(
            captureIsComplete: true,
            [@"C:\Desktop\known.txt"]));

        Assert.False(baseline.TryReplace(
            captureIsComplete: false,
            [@"C:\Desktop\partial.txt"]));

        Assert.Equal([@"C:\Desktop\known.txt"], baseline.Snapshot());
    }

    [Fact]
    public void Baseline_ExcludesPendingAndDeferredPathsFromCommittedSnapshot()
    {
        var baseline = new DesktopAutoOrganizationBaseline();

        Assert.True(baseline.TryReplace(
            captureIsComplete: true,
            [@"C:\Desktop\old.txt", @"C:\Desktop\new.txt", @"C:\Desktop\locked.txt"],
            pendingPaths: [@"C:\Desktop\new.txt"],
            excludedPaths: [@"C:\Desktop\locked.txt"]));

        Assert.Equal([@"C:\Desktop\old.txt"], baseline.Snapshot());
    }

    [Fact]
    public void BaselineEventBuffer_RemainsActiveAcrossDrainBatches()
    {
        var buffer = new DesktopAutoOrganizationBaselineEventBuffer();
        buffer.Begin();
        Assert.True(buffer.TryBufferDeletion(@"C:\Desktop\same.txt"));

        Assert.True(buffer.TryDrain(out DesktopAutoOrganizationBaselineEventBatch first));
        Assert.Equal([@"C:\Desktop\same.txt"], first.Deleted);

        // Recreate arrives while the first delete batch is being applied.
        Assert.True(buffer.TryBufferChange(
            @"C:\Desktop\same.txt",
            bypassBaseline: true));
        Assert.True(buffer.TryDrain(out DesktopAutoOrganizationBaselineEventBatch second));
        Assert.Equal([@"C:\Desktop\same.txt"], second.Forced);

        Assert.False(buffer.TryDrain(out _));
        Assert.False(buffer.TryBufferChange(
            @"C:\Desktop\after-flush.txt",
            bypassBaseline: true));
    }

    [Fact]
    public void NewNotification_InvalidatesStaleGeneration()
    {
        var stateMachine = new DesktopAutoOrganizationStateMachine();
        DesktopAutoOrganizationWorkItem first = stateMachine.BeginPending(@"C:\Desktop\file.txt");
        DesktopAutoOrganizationWorkItem second = stateMachine.BeginPending(@"C:\Desktop\file.txt");

        Assert.False(stateMachine.TryTransition(
            first,
            DesktopAutoOrganizationItemState.Pending,
            DesktopAutoOrganizationItemState.Settling));
        Assert.True(stateMachine.TryTransition(
            second,
            DesktopAutoOrganizationItemState.Pending,
            DesktopAutoOrganizationItemState.Settling));
    }

    [Fact]
    public void SamePathReplacement_InvalidatesOldProcessingIdentity()
    {
        var stateMachine = new DesktopAutoOrganizationStateMachine();
        const string path = @"C:\Desktop\same-name.txt";
        DesktopAutoOrganizationWorkItem oldIdentity = stateMachine.BeginPending(path);
        Assert.True(stateMachine.TryTransition(
            oldIdentity,
            DesktopAutoOrganizationItemState.Pending,
            DesktopAutoOrganizationItemState.Settling));
        Assert.True(stateMachine.TryTransition(
            oldIdentity,
            DesktopAutoOrganizationItemState.Settling,
            DesktopAutoOrganizationItemState.Processing));

        DesktopAutoOrganizationWorkItem replacement = stateMachine.BeginPending(path);

        Assert.False(stateMachine.IsCurrent(
            oldIdentity,
            DesktopAutoOrganizationItemState.Processing));
        Assert.True(stateMachine.IsCurrent(
            replacement,
            DesktopAutoOrganizationItemState.Pending));
    }

    [Fact]
    public void DeferredItems_HaveOneNextRetryAndBecomeOneWorkItem()
    {
        var stateMachine = new DesktopAutoOrganizationStateMachine();
        DateTimeOffset now = new(2026, 8, 3, 12, 0, 0, TimeSpan.Zero);
        DesktopAutoOrganizationWorkItem item = stateMachine.BeginPending(@"C:\Desktop\locked.txt");

        Assert.True(stateMachine.MarkDeferred(item, now.AddMinutes(2)));
        Assert.Empty(stateMachine.TakeDueDeferred(now.AddMinutes(1)));

        DesktopAutoOrganizationWorkItem retry = Assert.Single(
            stateMachine.TakeDueDeferred(now.AddMinutes(2)));
        Assert.Equal(item.Path, retry.Path);
        Assert.Empty(stateMachine.TakeDueDeferred(now.AddMinutes(3)));
        Assert.Equal(
            DesktopAutoOrganizationItemState.Pending,
            stateMachine.GetSnapshot(item.Path)?.State);
    }

    [Fact]
    public void RetrySchedule_UsesEarliestNextRetryAcrossAllPaths()
    {
        var stateMachine = new DesktopAutoOrganizationStateMachine();
        DateTimeOffset now = new(2026, 8, 3, 12, 0, 0, TimeSpan.Zero);
        DesktopAutoOrganizationWorkItem later = stateMachine.BeginPending(@"C:\Desktop\later.txt");
        DesktopAutoOrganizationWorkItem earlier = stateMachine.BeginPending(@"C:\Desktop\earlier.txt");

        Assert.True(stateMachine.MarkDeferred(later, now.AddMinutes(2)));
        Assert.True(stateMachine.MarkDeferred(earlier, now.AddSeconds(10)));

        Assert.Equal(now.AddSeconds(10), stateMachine.GetNextRetryAt());
        Assert.Equal(
            earlier.Path,
            Assert.Single(stateMachine.TakeDueDeferred(now.AddSeconds(10))).Path);
        Assert.Equal(now.AddMinutes(2), stateMachine.GetNextRetryAt());
    }

    [Fact]
    public void DisableAndResume_PreservesDeferredAndInvalidatesSettlingWork()
    {
        var stateMachine = new DesktopAutoOrganizationStateMachine();
        DateTimeOffset now = new(2026, 8, 3, 12, 0, 0, TimeSpan.Zero);
        DesktopAutoOrganizationWorkItem item = stateMachine.BeginPending(@"C:\Desktop\copying.zip");
        Assert.True(stateMachine.TryTransition(
            item,
            DesktopAutoOrganizationItemState.Pending,
            DesktopAutoOrganizationItemState.Settling));

        Assert.Single(stateMachine.SuspendRecoverableItems());
        stateMachine.ResumeDeferred(now);

        DesktopAutoOrganizationWorkItem resumed = Assert.Single(
            stateMachine.TakeDueDeferred(now));
        Assert.NotEqual(item.Generation, resumed.Generation);
        Assert.False(stateMachine.MarkTerminal(
            item,
            DesktopAutoOrganizationItemState.Completed));
    }

    [Fact]
    public void ProcessingCancellation_CanReturnToDeferredWithoutDuplicateSchedule()
    {
        var stateMachine = new DesktopAutoOrganizationStateMachine();
        DateTimeOffset retryAt = new(2026, 8, 3, 12, 2, 0, TimeSpan.Zero);
        DesktopAutoOrganizationWorkItem item = stateMachine.BeginPending(@"C:\Desktop\locked.txt");
        Assert.True(stateMachine.TryTransition(
            item,
            DesktopAutoOrganizationItemState.Pending,
            DesktopAutoOrganizationItemState.Settling));
        Assert.True(stateMachine.TryTransition(
            item,
            DesktopAutoOrganizationItemState.Settling,
            DesktopAutoOrganizationItemState.Processing));

        Assert.True(stateMachine.MarkDeferred(item, retryAt));
        Assert.Equal(retryAt, stateMachine.GetNextRetryAt());
        Assert.Empty(stateMachine.TakeDueDeferred(retryAt.AddTicks(-1)));
        Assert.Single(stateMachine.TakeDueDeferred(retryAt));
    }

    [Fact]
    public void RapidDisableEnable_BaselineExcludesProcessingUntilCancellationDefersIt()
    {
        var stateMachine = new DesktopAutoOrganizationStateMachine();
        var baseline = new DesktopAutoOrganizationBaseline();
        const string path = @"C:\Desktop\in-flight.txt";
        DateTimeOffset now = new(2026, 8, 3, 12, 0, 0, TimeSpan.Zero);
        DesktopAutoOrganizationWorkItem item = stateMachine.BeginPending(path);
        Assert.True(stateMachine.TryTransition(
            item,
            DesktopAutoOrganizationItemState.Pending,
            DesktopAutoOrganizationItemState.Settling));
        Assert.True(stateMachine.TryTransition(
            item,
            DesktopAutoOrganizationItemState.Settling,
            DesktopAutoOrganizationItemState.Processing));

        // Disable does not invalidate Processing because a move may already be
        // underway. The immediately following enable must still exclude it.
        stateMachine.SuspendRecoverableItems();
        Assert.True(baseline.TryReplace(
            captureIsComplete: true,
            [path],
            excludedPaths: stateMachine.GetNonTerminalPaths()));
        Assert.Empty(baseline.Snapshot());

        // Cancellation before the move converts that same generation to
        // Deferred and the single scheduler can resume it.
        Assert.True(stateMachine.MarkDeferred(item, now));
        stateMachine.ResumeDeferred(now);
        Assert.Equal(path, Assert.Single(stateMachine.TakeDueDeferred(now)).Path);
    }

    [Fact]
    public void RapidDisableEnable_DoesNotBlockProcessingMoveFromCompleting()
    {
        var stateMachine = new DesktopAutoOrganizationStateMachine();
        const string path = @"C:\Desktop\already-moving.txt";
        DesktopAutoOrganizationWorkItem item = stateMachine.BeginPending(path);
        Assert.True(stateMachine.TryTransition(
            item,
            DesktopAutoOrganizationItemState.Pending,
            DesktopAutoOrganizationItemState.Settling));
        Assert.True(stateMachine.TryTransition(
            item,
            DesktopAutoOrganizationItemState.Settling,
            DesktopAutoOrganizationItemState.Processing));

        stateMachine.SuspendRecoverableItems();

        Assert.True(stateMachine.MarkTerminal(
            item,
            DesktopAutoOrganizationItemState.Completed));
        Assert.Equal(
            DesktopAutoOrganizationItemState.Completed,
            stateMachine.GetSnapshot(path)?.State);
    }

    [Fact]
    public void Rename_TerminatesOldIdentity_AndSamePathCanBecomeNewIdentity()
    {
        var stateMachine = new DesktopAutoOrganizationStateMachine();
        const string path = @"C:\Desktop\renamed.txt";

        DesktopAutoOrganizationWorkItem oldIdentity = stateMachine.BeginPending(path);
        stateMachine.MarkRenamedOrMissing(path);
        Assert.Equal(
            DesktopAutoOrganizationItemState.Missing,
            stateMachine.GetSnapshot(path)?.State);

        DesktopAutoOrganizationWorkItem newIdentity = stateMachine.BeginPending(path);
        Assert.NotEqual(oldIdentity.Generation, newIdentity.Generation);
        Assert.Equal(
            DesktopAutoOrganizationItemState.Pending,
            stateMachine.GetSnapshot(path)?.State);
    }

    [Fact]
    public void InitialWaitDefer_DoesNotConsumeTheTransientRetryBudget()
    {
        var stateMachine = new DesktopAutoOrganizationStateMachine();
        const string path = @"C:\Desktop\waited-out.txt";
        DateTimeOffset due = new(2020, 1, 1, 0, 0, 10, TimeSpan.Zero);

        DesktopAutoOrganizationWorkItem item = stateMachine.BeginPending(path);
        Assert.True(stateMachine.MarkDeferred(item, due, countAttempt: false));
        Assert.Equal(
            DesktopAutoOrganizationItemState.Deferred,
            stateMachine.GetSnapshot(path)?.State);
        Assert.Equal(0, stateMachine.GetSnapshot(path)?.RetryAttempts);

        // A transient failure after the wait still owns the full budget.
        DesktopAutoOrganizationWorkItem dueItem = Assert.Single(
            stateMachine.TakeDueDeferred(due));
        Assert.True(stateMachine.MarkDeferred(dueItem, due.AddMinutes(2)));
        Assert.Equal(1, stateMachine.GetSnapshot(path)?.RetryAttempts);
    }

    [Fact]
    public void ActivityTracker_RecognizesDownloadOrExtractionBursts()
    {
        var tracker = new DesktopAutoOrganizationActivityTracker(
            TimeSpan.FromSeconds(8));
        DateTimeOffset start = new(2026, 8, 3, 12, 0, 0, TimeSpan.Zero);

        tracker.Observe(@"C:\Desktop\archive\one.tmp", start);
        Assert.Equal(
            1,
            tracker.GetSnapshot(@"C:\Desktop\archive\one.tmp", start).EventCount);

        tracker.Observe(
            @"C:\Desktop\archive\two.tmp",
            start.AddSeconds(1));
        DesktopDirectoryActivitySnapshot burst = tracker.GetSnapshot(
            @"C:\Desktop\archive\one.tmp",
            start.AddSeconds(2));
        Assert.Equal(2, burst.EventCount);

        DesktopDirectoryActivitySnapshot expired = tracker.GetSnapshot(
            @"C:\Desktop\archive\one.tmp",
            start.AddSeconds(10));
        Assert.Equal(0, expired.EventCount);
    }
}
