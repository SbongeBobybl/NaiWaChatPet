using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class QuickRevealDesktopDismissTrackerTests
{
    [Fact]
    public void MatchingDismissal_IsConsumedExactlyOnce()
    {
        var tracker = new QuickRevealDesktopDismissTracker(500, 8, 8);
        var sequence = new DesktopDoubleClickSequence(
            100,
            100,
            1000,
            102,
            98,
            1200);
        tracker.Record(101, 99, 1050);

        Assert.True(tracker.ConsumeIfSameSequence(sequence));
        Assert.False(tracker.ConsumeIfSameSequence(sequence));
    }

    [Fact]
    public void DismissalBeforeSequence_DoesNotSuppressLaterDesktopDoubleClick()
    {
        var tracker = new QuickRevealDesktopDismissTracker(500, 8, 8);
        tracker.Record(100, 100, 900);

        Assert.False(tracker.ConsumeIfSameSequence(
            new DesktopDoubleClickSequence(100, 100, 1000, 100, 100, 1200)));
    }

    [Fact]
    public void DismissalAtDifferentPoint_DoesNotMatchSequence()
    {
        var tracker = new QuickRevealDesktopDismissTracker(500, 8, 8);
        tracker.Record(110, 100, 1050);

        Assert.False(tracker.ConsumeIfSameSequence(
            new DesktopDoubleClickSequence(100, 100, 1000, 100, 100, 1200)));
    }

    [Fact]
    public void TickCountWraparound_PreservesSameSequenceMatch()
    {
        var tracker = new QuickRevealDesktopDismissTracker(500, 8, 8);
        tracker.Record(100, 100, 20);

        Assert.True(tracker.ConsumeIfSameSequence(
            new DesktopDoubleClickSequence(
                100,
                100,
                uint.MaxValue - 99,
                100,
                100,
                50)));
    }
}
