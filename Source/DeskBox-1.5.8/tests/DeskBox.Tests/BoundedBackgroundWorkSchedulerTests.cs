using System.Diagnostics;
using DeskBox.Helpers;

namespace DeskBox.Tests;

public sealed class BoundedBackgroundWorkSchedulerTests
{
    [Fact]
    public async Task RunAsync_CompletesOrdinaryWork()
    {
        var scheduler = new BoundedBackgroundWorkScheduler(maxConcurrency: 1);

        BoundedBackgroundWorkResult<int> result = await scheduler.RunAsync(
            () => 42,
            TimeSpan.FromSeconds(1));

        Assert.Equal(BoundedBackgroundWorkStatus.Completed, result.Status);
        Assert.Equal(42, result.Value);
    }

    [Fact]
    public async Task RunAsync_DistinguishesWorkerTimeoutExceptionFromDeadline()
    {
        var scheduler = new BoundedBackgroundWorkScheduler(maxConcurrency: 1);

        BoundedBackgroundWorkResult<int> result = await scheduler.RunAsync<int>(
            () => throw new TimeoutException("worker failure"),
            TimeSpan.FromSeconds(1));

        Assert.Equal(BoundedBackgroundWorkStatus.Faulted, result.Status);
        TimeoutException exception = Assert.IsType<TimeoutException>(result.Exception);
        Assert.Equal("worker failure", exception.Message);
    }

    [Fact]
    public async Task RunAsync_TimedOutWorkKeepsItsSlotUntilItReallyReturns()
    {
        var scheduler = new BoundedBackgroundWorkScheduler(maxConcurrency: 1);
        using var releaseWorker = new ManualResetEventSlim();
        var workerStarted = new TaskCompletionSource(
            TaskCreationOptions.RunContinuationsAsynchronously);
        var workerExited = new TaskCompletionSource(
            TaskCreationOptions.RunContinuationsAsynchronously);
        int queuedWorkStarted = 0;

        try
        {
            Task<BoundedBackgroundWorkResult<int>> blocked = scheduler.RunAsync(
                () =>
                {
                    workerStarted.TrySetResult();
                    releaseWorker.Wait();
                    workerExited.TrySetResult();
                    return 1;
                },
                TimeSpan.FromMilliseconds(250));
            await workerStarted.Task.WaitAsync(TimeSpan.FromSeconds(2));

            var stopwatch = Stopwatch.StartNew();
            BoundedBackgroundWorkResult<int> timedOut = await blocked;
            Assert.Equal(
                BoundedBackgroundWorkStatus.ExecutionTimedOut,
                timedOut.Status);
            Assert.True(stopwatch.Elapsed < TimeSpan.FromSeconds(2));

            BoundedBackgroundWorkResult<int> queued = await scheduler.RunAsync(
                () =>
                {
                    Interlocked.Increment(ref queuedWorkStarted);
                    return 2;
                },
                TimeSpan.FromMilliseconds(150));
            Assert.Equal(
                BoundedBackgroundWorkStatus.QueueTimedOut,
                queued.Status);
            Assert.Equal(0, Volatile.Read(ref queuedWorkStarted));
        }
        finally
        {
            releaseWorker.Set();
        }

        await workerExited.Task.WaitAsync(TimeSpan.FromSeconds(2));
        await Task.Delay(25);
        BoundedBackgroundWorkResult<int> recovered = await scheduler.RunAsync(
            () => 3,
            TimeSpan.FromSeconds(1));
        Assert.Equal(BoundedBackgroundWorkStatus.Completed, recovered.Status);
        Assert.Equal(3, recovered.Value);
    }
}
