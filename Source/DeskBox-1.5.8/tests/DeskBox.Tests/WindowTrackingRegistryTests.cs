using DeskBox.Services;
using System.Runtime.CompilerServices;

namespace DeskBox.Tests;

public sealed class WindowTrackingRegistryTests
{
    [Fact]
    public void Track_IsIdempotent()
    {
        var registry = new WindowTrackingRegistry<object>();
        var item = new object();

        Assert.True(registry.Track(item));
        Assert.False(registry.Track(item));
        Assert.Equal(1, registry.TrackedCount);
    }

    [Fact]
    public void ItemStaysTrackedUntilRealClose()
    {
        // Regression contract for the settings-window theme freeze: a
        // cancelled close (hide-and-reuse) never raises Closed, so the
        // entry must survive until NotifyClosed actually runs.
        var registry = new WindowTrackingRegistry<object>();
        var item = new object();
        registry.Track(item);

        Assert.Contains(item, registry.EnumerateAlive());
        Assert.Equal(1, registry.TrackedCount);
    }

    [Fact]
    public void NotifyClosed_RemovesItem()
    {
        var registry = new WindowTrackingRegistry<object>();
        var item = new object();
        registry.Track(item);

        Assert.True(registry.NotifyClosed(item));
        Assert.Empty(registry.EnumerateAlive());
        Assert.Equal(0, registry.TrackedCount);
    }

    [Fact]
    public void Untrack_IsIdempotent()
    {
        var registry = new WindowTrackingRegistry<object>();
        var item = new object();
        registry.Track(item);

        Assert.True(registry.Untrack(item));
        Assert.False(registry.Untrack(item));
        Assert.Equal(0, registry.TrackedCount);
    }

    [Fact]
    public void EnumerateAlive_SweepsCollectedTargets()
    {
        var registry = new WindowTrackingRegistry<object>();
        var keep = new object();
        TrackTransient(registry);
        registry.Track(keep);

        GC.Collect();
        GC.WaitForPendingFinalizers();
        GC.Collect();

        var alive = registry.EnumerateAlive();
        Assert.Single(alive);
        Assert.Same(keep, alive[0]);
    }

    [MethodImpl(MethodImplOptions.NoInlining)]
    private static void TrackTransient(WindowTrackingRegistry<object> registry) =>
        registry.Track(new object());
}
