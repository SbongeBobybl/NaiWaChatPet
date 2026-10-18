using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class DisplayTopologyTransitionCoordinatorTests
{
    [Fact]
    public void StabilityTracker_RequiresConsecutiveMatchingSignatures()
    {
        var tracker = new DisplayTopologyStabilityTracker(requiredObservations: 2);

        Assert.False(tracker.Observe("display-a"));
        Assert.False(tracker.Observe("display-b"));
        Assert.True(tracker.Observe("display-b"));
    }

    [Fact]
    public void StabilityTracker_ResetRequiresFreshStablePair()
    {
        var tracker = new DisplayTopologyStabilityTracker(requiredObservations: 2);
        Assert.False(tracker.Observe("display-a"));
        Assert.True(tracker.Observe("display-a"));

        tracker.Reset();

        Assert.Null(tracker.LastSignature);
        Assert.False(tracker.Observe("display-a"));
        Assert.True(tracker.Observe("display-a"));
    }

    [Fact]
    public void CombineReasons_DeduplicatesSignalsWithinOneGeneration()
    {
        string reasons = DisplayTopologyTransitionCoordinator.CombineReasons(
            string.Empty,
            "widget-window-message");
        reasons = DisplayTopologyTransitionCoordinator.CombineReasons(
            reasons,
            "display-area-watcher");
        reasons = DisplayTopologyTransitionCoordinator.CombineReasons(
            reasons,
            "widget-window-message");

        Assert.Equal("widget-window-message,display-area-watcher", reasons);
    }
}
