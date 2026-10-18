namespace DeskBox.Services;

/// <summary>
/// Overlaps asynchronous content readiness without dispatching WinUI creation
/// to worker threads. The caller's synchronization context is retained.
/// </summary>
internal static class StartupWidgetRestoreRunner
{
    internal const int MaximumPendingRestores = 2;

    internal static async Task RestoreAsync<T>(
        IReadOnlyList<T> widgets,        Func<T, Task> restore,        Action<T, Exception> reportFailure)
    {
        var pending = new List<Task>(MaximumPendingRestores);
        foreach (T widget in widgets)
        {
            if (pending.Count == MaximumPendingRestores)
            {
                Task completed = await Task.WhenAny(pending);
                await completed;
                pending.Remove(completed);
            }

            pending.Add(RestoreOneAsync(widget));
            // Let content Loaded events and the first rendered frame progress.
            await Task.Yield();
        }

        await Task.WhenAll(pending);

        async Task RestoreOneAsync(T widget)
        {
            try
            {
                await restore(widget);
            }
            catch (Exception ex)
            {
                reportFailure(widget,ex);
            }
        }
    }
}
