using DeskBox.Services;
using DeskBox.ViewModels;

namespace DeskBox.Tests;

public sealed class WidgetVisibilityLifecyclePolicyTests
{
    [Fact]
    public void SurfaceActivityTracker_CoalescesHiddenChangesIntoOneResume()
    {
        var tracker = new WidgetSurfaceActivityTracker();

        Assert.False(tracker.TryDeferChange());
        tracker.Suspend();
        Assert.True(tracker.IsSuspended);
        Assert.True(tracker.TryDeferChange());
        Assert.True(tracker.TryDeferChange());

        Assert.True(tracker.Resume());
        Assert.False(tracker.IsSuspended);
        Assert.False(tracker.Resume());
    }

    [Fact]
    public void FileSurfaceRefreshPolicy_UsesDirtySignalOrThirtySecondFreshness()
    {
        DateTime now = new(2026, 8, 26, 12, 0, 0, DateTimeKind.Utc);

        Assert.True(FileSurfaceRefreshPolicy.ShouldReconcile(
            now,
            now.AddSeconds(-1),
            hasDeferredChanges: true));
        Assert.False(FileSurfaceRefreshPolicy.ShouldReconcile(
            now,
            now.AddSeconds(-29),
            hasDeferredChanges: false));
        Assert.True(FileSurfaceRefreshPolicy.ShouldReconcile(
            now,
            now.AddSeconds(-30),
            hasDeferredChanges: false));
    }

    [Fact]
    public void MusicRevealRefreshPolicy_SkipsRapidCleanRevealButHonorsChanges()
    {
        DateTime now = new(2026, 8, 26, 12, 0, 0, DateTimeKind.Utc);

        Assert.False(MusicWidgetViewModel.ShouldRefreshAfterReveal(
            now,
            now.AddSeconds(-29),
            hasHiddenChanges: false));
        Assert.True(MusicWidgetViewModel.ShouldRefreshAfterReveal(
            now,
            now.AddSeconds(-1),
            hasHiddenChanges: true));
        Assert.True(MusicWidgetViewModel.ShouldRefreshAfterReveal(
            now,
            now.AddSeconds(-30),
            hasHiddenChanges: false));
    }
}
