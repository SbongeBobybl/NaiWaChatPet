using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WidgetSurfaceSnapshotCacheTests
{
    [Fact]
    public void AddOrUpdate_EvictsLeastRecentlyUsedToPixelBudget()
    {
        var cache = new WidgetSurfaceSnapshotCache<object>(pixelBudget: 200);
        var first = new object();
        var second = new object();
        var third = new object();
        cache.AddOrUpdate("a", first, 10, 10);
        cache.AddOrUpdate("b", second, 10, 10);
        Assert.True(cache.TryGet("a", out _));

        cache.AddOrUpdate("c", third, 10, 10);

        Assert.True(cache.TryGet("a", out var retained));
        Assert.Same(first, retained);
        Assert.False(cache.TryGet("b", out _));
        Assert.True(cache.TryGet("c", out _));
        Assert.Equal(200, cache.TotalPixels);
    }

    [Fact]
    public void OversizedSnapshot_IsRejectedWithoutEvictingExisting()
    {
        var cache = new WidgetSurfaceSnapshotCache<object>(pixelBudget: 100);
        var retained = new object();
        cache.AddOrUpdate("a", retained, 10, 10);

        cache.AddOrUpdate("large", new object(), 11, 10);

        Assert.Equal(1, cache.Count);
        Assert.True(cache.TryGet("a", out var result));
        Assert.Same(retained, result);
    }

    [Fact]
    public void Clear_ReleasesAllBudget()
    {
        var cache = new WidgetSurfaceSnapshotCache<object>(pixelBudget: 100);
        cache.AddOrUpdate("a", new object(), 5, 5);

        cache.Clear();

        Assert.Equal(0, cache.Count);
        Assert.Equal(0, cache.TotalPixels);
    }

    [Fact]
    public void EntryLimit_KeepsOnlyPreviousCurrentAndNextVisuals()
    {
        var cache = new WidgetSurfaceSnapshotCache<object>(
            pixelBudget: 1000,
            entryLimit: 3);
        cache.AddOrUpdate("a", new object(), 5, 5);
        cache.AddOrUpdate("b", new object(), 5, 5);
        cache.AddOrUpdate("c", new object(), 5, 5);
        cache.AddOrUpdate("d", new object(), 5, 5);

        Assert.Equal(3, cache.Count);
        Assert.False(cache.TryGet("a", out _));
        Assert.True(cache.TryGet("d", out _));
    }
}
