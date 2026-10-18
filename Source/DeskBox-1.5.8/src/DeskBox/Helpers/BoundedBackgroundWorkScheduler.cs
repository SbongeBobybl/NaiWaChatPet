using System.Diagnostics;

namespace DeskBox.Helpers;

internal enum BoundedBackgroundWorkStatus
{
    Completed,
    QueueTimedOut,
    ExecutionTimedOut,
    Faulted
}

internal readonly record struct BoundedBackgroundWorkResult<T>(
    BoundedBackgroundWorkStatus Status,    T? Value = default,    Exception? Exception = null);

/// <summary>
/// Runs potentially blocking native or filesystem work without allowing timed-out
/// calls to create an unbounded backlog. A timed-out operation keeps its slot until
/// the underlying call really returns because Windows Shell COM calls cannot be
/// cancelled safely once they have started.
/// </summary>
internal sealed class BoundedBackgroundWorkScheduler
{
    private readonly SemaphoreSlim _slots;

    internal static BoundedBackgroundWorkScheduler SharedShell { get; } = new(2);

    public BoundedBackgroundWorkScheduler(int maxConcurrency)
    {
        ArgumentOutOfRangeException.ThrowIfLessThan(maxConcurrency,1);
        _slots = new SemaphoreSlim(maxConcurrency,maxConcurrency);
    }

    public async Task<BoundedBackgroundWorkResult<T>> RunAsync<T>(
        Func<T> work,        TimeSpan timeout)
    {
        ArgumentNullException.ThrowIfNull(work);
        if (timeout <= TimeSpan.Zero)
        {
            throw new ArgumentOutOfRangeException(nameof(timeout));
        }

        var stopwatch = Stopwatch.StartNew();
        if (!await _slots.WaitAsync(timeout).ConfigureAwait(false))
        {
            return new(BoundedBackgroundWorkStatus.QueueTimedOut);
        }

        TimeSpan remaining = timeout - stopwatch.Elapsed;
        if (remaining <= TimeSpan.Zero)
        {
            _slots.Release();
            return new(BoundedBackgroundWorkStatus.QueueTimedOut);
        }

        Task<T> workTask;
        try
        {
            workTask = Task.Run(work);
        }
        catch (Exception ex)
        {
            _slots.Release();
            return new(BoundedBackgroundWorkStatus.Faulted,Exception: ex);
        }

        _ = workTask.ContinueWith(
            static (completedTask,state) =>
            {
                // Observing Exception prevents a late failure from becoming an
                // unobserved task exception after the caller has already timed out.
                _ = completedTask.Exception;
                ((SemaphoreSlim)state!).Release();
            },            _slots,            CancellationToken.None,            TaskContinuationOptions.ExecuteSynchronously,            TaskScheduler.Default);

        try
        {
            return new(
                BoundedBackgroundWorkStatus.Completed,                await workTask.WaitAsync(remaining).ConfigureAwait(false));
        }
        catch (TimeoutException ex) when (workTask.IsFaulted)
        {
            return new(
                BoundedBackgroundWorkStatus.Faulted,                Exception: workTask.Exception?.GetBaseException() ?? ex);
        }
        catch (TimeoutException)
        {
            return new(BoundedBackgroundWorkStatus.ExecutionTimedOut);
        }
        catch (Exception ex)
        {
            return new(BoundedBackgroundWorkStatus.Faulted,Exception: ex);
        }
    }
}
