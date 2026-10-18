using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class StartupWidgetRestoreRunnerTests
{
    [Fact]
    public async Task SlowContentDoesNotBlockEveryFollowingWidget_AndConcurrencyIsBounded()
    {
        var started = new List<int>();
        var gates = Enumerable.Range(0, 4).Select(_ =>
            new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously)).ToArray();
        var secondStarted = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        var thirdStarted = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        int active = 0, maximumActive = 0;
        Task run = StartupWidgetRestoreRunner.RestoreAsync(
            new[] { 0, 1, 2, 3 },
            async index =>
            {
                lock (started) started.Add(index);
                int current = Interlocked.Increment(ref active);
                maximumActive = Math.Max(maximumActive, current);
                if (index == 1) secondStarted.SetResult();
                if (index == 2) thirdStarted.SetResult();
                await gates[index].Task;
                Interlocked.Decrement(ref active);
            },
            (_, ex) => throw new InvalidOperationException("Unexpected restore failure", ex));

        await secondStarted.Task.WaitAsync(TimeSpan.FromSeconds(5));
        lock (started) Assert.Equal(new[] { 0, 1 }, started);
        gates[1].SetResult();
        await thirdStarted.Task.WaitAsync(TimeSpan.FromSeconds(5));
        Assert.False(gates[0].Task.IsCompleted);
        Assert.False(run.IsCompleted);
        foreach (var gate in gates) gate.TrySetResult();
        await run.WaitAsync(TimeSpan.FromSeconds(5));
        Assert.Equal(2, maximumActive);
        Assert.Equal(0, active);
        lock (started) Assert.Equal(new[] { 0, 1, 2, 3 }, started);
    }

    [Fact]
    public async Task FailedWidgetDoesNotPreventOtherWidgetsFromRestoring()
    {
        var restored = new List<int>();
        var failures = new List<int>();
        await StartupWidgetRestoreRunner.RestoreAsync(
            new[] { 0, 1, 2 },
            index =>
            {
                if (index == 1) throw new IOException("Unavailable content");
                restored.Add(index);
                return Task.CompletedTask;
            },
            (index, _) => failures.Add(index));
        Assert.Equal(new[] { 0, 2 }, restored);
        Assert.Equal(new[] { 1 }, failures);
    }
}
