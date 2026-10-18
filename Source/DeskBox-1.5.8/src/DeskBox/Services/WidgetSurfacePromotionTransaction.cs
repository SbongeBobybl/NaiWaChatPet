namespace DeskBox.Services;

/// <summary>
/// Orders the one-time promotion from a legacy member window to a unified
/// Surface host. The legacy host is retired only by the commit callback, after
/// the candidate has been prepared and presented successfully.
/// </summary>
internal static class WidgetSurfacePromotionTransaction
{
    internal static async Task<TCandidate> ExecuteAsync<TCandidate>(
        Func<Task<TCandidate>> prepareCandidateAsync,        Func<TCandidate, Task> presentCandidateAsync,        Func<TCandidate, Task> commitAndRetireLegacyAsync,        Action<TCandidate> rollbackCandidate)
    {
        ArgumentNullException.ThrowIfNull(prepareCandidateAsync);
        ArgumentNullException.ThrowIfNull(presentCandidateAsync);
        ArgumentNullException.ThrowIfNull(commitAndRetireLegacyAsync);
        ArgumentNullException.ThrowIfNull(rollbackCandidate);

        TCandidate candidate = await prepareCandidateAsync();
        try
        {
            await presentCandidateAsync(candidate);
            await commitAndRetireLegacyAsync(candidate);
            return candidate;
        }
        catch
        {
            try
            {
                rollbackCandidate(candidate);
            }
            catch (Exception rollbackException)
            {
                App.Log(
                    $"[WidgetSurface] Promotion candidate rollback failed: " +
                    $"{rollbackException}");
            }

            throw;
        }
    }

    internal static async Task<TCandidate> ExecuteAsync<TCandidate>(
        Func<Task<TCandidate>> prepareCandidateAsync,        Action<TCandidate> presentCandidate,        Action<TCandidate> commitAndRetireLegacy,        Action<TCandidate> rollbackCandidate)
    {
        ArgumentNullException.ThrowIfNull(prepareCandidateAsync);
        ArgumentNullException.ThrowIfNull(presentCandidate);
        ArgumentNullException.ThrowIfNull(commitAndRetireLegacy);
        ArgumentNullException.ThrowIfNull(rollbackCandidate);

        TCandidate candidate = await prepareCandidateAsync();
        try
        {
            presentCandidate(candidate);
            commitAndRetireLegacy(candidate);
            return candidate;
        }
        catch
        {
            try
            {
                rollbackCandidate(candidate);
            }
            catch (Exception rollbackException)
            {
                App.Log(
                    $"[WidgetSurface] Promotion candidate rollback failed: " +
                    $"{rollbackException}");
            }

            throw;
        }
    }
}
