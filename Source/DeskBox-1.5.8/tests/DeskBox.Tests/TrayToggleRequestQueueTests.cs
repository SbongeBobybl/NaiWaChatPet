using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class TrayToggleRequestQueueTests
{
    [Theory]
    [InlineData(1)]
    [InlineData(2)]
    [InlineData(3)]
    [InlineData(4)]
    public async Task RapidPresses_preserveToggleParity(int pressCount)
    {
        var firstStarted = new TaskCompletionSource(
            TaskCreationOptions.RunContinuationsAsynchronously);
        var releaseFirst = new TaskCompletionSource(
            TaskCreationOptions.RunContinuationsAsynchronously);
        bool state = false;
        int executions = 0;

        var queue = new DeskBox.Services.TrayToggleRequestQueue(async _ =>
        {
            state = !state;
            if (Interlocked.Increment(ref executions) == 1)
            {
                firstStarted.TrySetResult();
                await releaseFirst.Task;
            }
        });

        var requests = new List<Task> { queue.EnqueueAsync("press-1") };
        await firstStarted.Task;
        for (int press = 2; press <= pressCount; press++)
        {
            requests.Add(queue.EnqueueAsync($"press-{press}"));
        }

        releaseFirst.TrySetResult();
        await Task.WhenAll(requests);

        Assert.Equal((pressCount & 1) != 0, state);
        Assert.Equal(pressCount, queue.GetSnapshot().TotalRequests);
    }

    [Fact]
    public async Task BurstRequests_areFoldedByParity_andNeverRunConcurrently()
    {
        var firstStarted = new TaskCompletionSource(
            TaskCreationOptions.RunContinuationsAsynchronously);
        var releaseFirst = new TaskCompletionSource(
            TaskCreationOptions.RunContinuationsAsynchronously);
        int active = 0;
        int maxActive = 0;
        int executions = 0;
        bool state = false;

        var queue = new DeskBox.Services.TrayToggleRequestQueue(async _ =>
        {
            firstStarted.TrySetResult();
            int nowActive = Interlocked.Increment(ref active);
            InterlockedMax(ref maxActive, nowActive);
            Interlocked.Increment(ref executions);
            state = !state;
            try
            {
                await releaseFirst.Task;
            }
            finally
            {
                Interlocked.Decrement(ref active);
            }
        });

        Task first = queue.EnqueueAsync("first");
        await firstStarted.Task;

        Task[] requests =
        [
            first,
            queue.EnqueueAsync("second"),
            queue.EnqueueAsync("third"),
            queue.EnqueueAsync("fourth")
        ];

        releaseFirst.TrySetResult();
        await Task.WhenAll(requests);

        Assert.Equal(2, executions);
        Assert.Equal(1, maxActive);
        Assert.False(state);
    }

    [Fact]
    public async Task FailedToggle_doesNotWedgeLaterRequests()
    {
        int attempts = 0;
        var queue = new DeskBox.Services.TrayToggleRequestQueue(_ =>
        {
            if (Interlocked.Increment(ref attempts) == 1)
            {
                throw new InvalidOperationException("simulated failure");
            }

            return Task.CompletedTask;
        });

        await Assert.ThrowsAsync<InvalidOperationException>(
            () => queue.EnqueueAsync("first"));
        await queue.EnqueueAsync("second");

        TrayToggleQueueSnapshot snapshot = queue.GetSnapshot();
        Assert.Equal(2, attempts);
        Assert.Equal("simulated failure", snapshot.LastError);
    }

    [Fact]
    public async Task TrayIconDoubleClick_withinWindow_togglesOnlyOnce()
    {
        int executions = 0;
        var queue = new DeskBox.Services.TrayToggleRequestQueue(
            _ => { Interlocked.Increment(ref executions); return Task.CompletedTask; },
            trayIconDoubleClickWindow: TimeSpan.FromSeconds(30));

        await queue.EnqueueAsync(DeskBox.Services.TrayToggleRequestQueue.TrayIconSource);
        await queue.EnqueueAsync(DeskBox.Services.TrayToggleRequestQueue.TrayIconSource);
        await queue.EnqueueAsync(DeskBox.Services.TrayToggleRequestQueue.TrayIconSource);

        // The second and third clicks are within the double-click window and
        // must complete without toggling; the widget stays revealed.
        Assert.Equal(1, executions);
        TrayToggleQueueSnapshot snapshot = queue.GetSnapshot();
        Assert.Equal(2, snapshot.SuppressedTrayIconDoubleClicks);
        Assert.Equal(3, snapshot.TotalRequests);
    }

    [Fact]
    public async Task TrayIconClicks_spacedBeyondWindow_toggleEachTime()
    {
        int executions = 0;
        var queue = new DeskBox.Services.TrayToggleRequestQueue(
            _ => { Interlocked.Increment(ref executions); return Task.CompletedTask; },
            trayIconDoubleClickWindow: TimeSpan.FromMilliseconds(40));

        await queue.EnqueueAsync(DeskBox.Services.TrayToggleRequestQueue.TrayIconSource);
        await Task.Delay(120);
        await queue.EnqueueAsync(DeskBox.Services.TrayToggleRequestQueue.TrayIconSource);

        Assert.Equal(2, executions);
        Assert.Equal(0, queue.GetSnapshot().SuppressedTrayIconDoubleClicks);
    }

    [Fact]
    public async Task Suppression_appliesOnlyToTrayIconSource()
    {
        int executions = 0;
        var queue = new DeskBox.Services.TrayToggleRequestQueue(
            _ => { Interlocked.Increment(ref executions); return Task.CompletedTask; },
            trayIconDoubleClickWindow: TimeSpan.FromSeconds(30));

        await queue.EnqueueAsync(DeskBox.Services.TrayToggleRequestQueue.TrayIconSource);
        await queue.EnqueueAsync("global-hotkey");
        await queue.EnqueueAsync("desktop-double-click");

        // Other sources never double-fire from one physical gesture, so they
        // keep parity semantics without a double-click window.
        Assert.Equal(3, executions);
        Assert.Equal(0, queue.GetSnapshot().SuppressedTrayIconDoubleClicks);
    }

    private static void InterlockedMax(ref int location, int value)
    {
        int current;
        do
        {
            current = Volatile.Read(ref location);
            if (value <= current)
            {
                return;
            }
        }
        while (Interlocked.CompareExchange(ref location, value, current) != current);
    }
}
