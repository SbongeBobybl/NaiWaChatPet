using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WidgetSurfacePromotionTransactionTests
{
    [Fact]
    public async Task PrepareFailure_DoesNotPresentRetireOrRollback()
    {
        var events = new List<string>();

        await Assert.ThrowsAsync<InvalidOperationException>(() =>
            WidgetSurfacePromotionTransaction.ExecuteAsync(
                prepareCandidateAsync: () =>
                {
                    events.Add("prepare");
                    return Task.FromException<string>(
                        new InvalidOperationException("prepare failed"));
                },
                presentCandidate: _ => events.Add("present"),
                commitAndRetireLegacy: _ => events.Add("retire"),
                rollbackCandidate: _ => events.Add("rollback")));

        Assert.Equal(["prepare"], events);
    }

    [Fact]
    public async Task PresentFailure_RollsBackCandidateWithoutRetiringLegacy()
    {
        var events = new List<string>();

        await Assert.ThrowsAsync<InvalidOperationException>(() =>
            WidgetSurfacePromotionTransaction.ExecuteAsync(
                prepareCandidateAsync: () =>
                {
                    events.Add("prepare");
                    return Task.FromResult("candidate");
                },
                presentCandidate: _ =>
                {
                    events.Add("present");
                    throw new InvalidOperationException("present failed");
                },
                commitAndRetireLegacy: _ => events.Add("retire"),
                rollbackCandidate: _ => events.Add("rollback")));

        Assert.Equal(["prepare", "present", "rollback"], events);
    }

    [Fact]
    public async Task Success_PresentsBeforeCommittingAndRetiringLegacy()
    {
        var events = new List<string>();

        string candidate =
            await WidgetSurfacePromotionTransaction.ExecuteAsync(
                prepareCandidateAsync: () =>
                {
                    events.Add("prepare");
                    return Task.FromResult("candidate");
                },
                presentCandidate: _ => events.Add("present"),
                commitAndRetireLegacy: _ => events.Add("retire"),
                rollbackCandidate: _ => events.Add("rollback"));

        Assert.Equal("candidate", candidate);
        Assert.Equal(["prepare", "present", "retire"], events);
    }

    [Fact]
    public async Task CommitFailure_RollsBackCandidate()
    {
        var events = new List<string>();

        await Assert.ThrowsAsync<InvalidOperationException>(() =>
            WidgetSurfacePromotionTransaction.ExecuteAsync(
                prepareCandidateAsync: () =>
                {
                    events.Add("prepare");
                    return Task.FromResult("candidate");
                },
                presentCandidate: _ => events.Add("present"),
                commitAndRetireLegacy: _ =>
                {
                    events.Add("commit");
                    throw new InvalidOperationException("commit failed");
                },
                rollbackCandidate: _ => events.Add("rollback")));

        Assert.Equal(["prepare", "present", "commit", "rollback"], events);
    }

    [Fact]
    public async Task AsyncFirstFrameFailure_RollsBackBeforeCommit()
    {
        var events = new List<string>();

        await Assert.ThrowsAsync<TimeoutException>(() =>
            WidgetSurfacePromotionTransaction.ExecuteAsync(
                prepareCandidateAsync: () =>
                {
                    events.Add("prepare");
                    return Task.FromResult("candidate");
                },
                presentCandidateAsync: _ =>
                {
                    events.Add("first-frame");
                    return Task.FromException(
                        new TimeoutException("frame timeout"));
                },
                commitAndRetireLegacyAsync: _ =>
                {
                    events.Add("commit");
                    return Task.CompletedTask;
                },
                rollbackCandidate: _ => events.Add("rollback")));

        Assert.Equal(["prepare", "first-frame", "rollback"], events);
    }

    [Fact]
    public async Task AsyncCommitFailure_RollsBackCandidate()
    {
        var events = new List<string>();

        await Assert.ThrowsAsync<InvalidOperationException>(() =>
            WidgetSurfacePromotionTransaction.ExecuteAsync(
                prepareCandidateAsync: () => Task.FromResult("candidate"),
                presentCandidateAsync: _ => Task.CompletedTask,
                commitAndRetireLegacyAsync: _ =>
                    Task.FromException(
                        new InvalidOperationException("save failed")),
                rollbackCandidate: _ => events.Add("rollback")));

        Assert.Equal(["rollback"], events);
    }
}
