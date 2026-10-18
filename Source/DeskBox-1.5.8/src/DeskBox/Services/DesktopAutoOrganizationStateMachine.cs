namespace DeskBox.Services;

internal enum DesktopAutoOrganizationItemState
{
    Pending,
    Settling,
    Processing,
    Deferred,
    Completed,
    Ignored,
    Missing
}

internal readonly record struct DesktopAutoOrganizationWorkItem(
    string Path,    long Generation);

internal readonly record struct DesktopAutoOrganizationStateSnapshot(
    string Path,    DesktopAutoOrganizationItemState State,    long Generation,    int RetryAttempts,    DateTimeOffset? NextRetryAt);

internal static class DesktopAutoOrganizationStatePolicy
{
    public static DesktopAutoOrganizationItemState ForSnapshotExclusion(
        Models.DesktopOrganizationExclusionReason reason,        bool pathExists)
    {
        if (reason == Models.DesktopOrganizationExclusionReason.None)
        {
            return DesktopAutoOrganizationItemState.Processing;
        }

        if (reason == Models.DesktopOrganizationExclusionReason.Unavailable)
        {
            return pathExists
                ? DesktopAutoOrganizationItemState.Deferred
                : DesktopAutoOrganizationItemState.Missing;
        }

        // Scanner exclusions are deliberate terminal decisions: folders,
        // hidden/system and reparse items, placeholders, temporary downloads,
        // public-desktop entries and slow/oversized files must not retry forever.
        return DesktopAutoOrganizationItemState.Ignored;
    }
}

internal enum DesktopAutoOrganizationRetryKind
{
    Finite,
    Persistent
}

internal readonly record struct DesktopAutoOrganizationRetryDecision(
    bool ShouldRetry,    TimeSpan Delay);

internal static class DesktopAutoOrganizationRetrySchedule
{
    public static DesktopAutoOrganizationRetryDecision Evaluate(
        DesktopAutoOrganizationRetryKind kind,        int nextAttempt,        int fastRetryAttemptLimit,        TimeSpan persistentRetryDelay)
    {
        if (kind == DesktopAutoOrganizationRetryKind.Finite &&
            nextAttempt > fastRetryAttemptLimit)
        {
            return new DesktopAutoOrganizationRetryDecision(false,TimeSpan.Zero);
        }

        TimeSpan delay = nextAttempt > fastRetryAttemptLimit
            ? persistentRetryDelay
            : TimeSpan.FromSeconds(Math.Min(30,2 * nextAttempt));
        return new DesktopAutoOrganizationRetryDecision(true,delay);
    }
}

/// <summary>
/// Owns the complete state of every path observed by desktop auto-organization.
/// All transitions are generation checked so stale async work cannot overwrite a
/// rename, a later change notification, or a disable/enable cycle.
/// </summary>
internal sealed class DesktopAutoOrganizationStateMachine
{
    private readonly object _gate = new();
    private readonly Dictionary<string, Entry> _entries =
        new(StringComparer.OrdinalIgnoreCase);
    private long _nextGeneration;

    public DesktopAutoOrganizationWorkItem BeginPending(
        string path,        bool preserveRetryAttempts = false)
    {
        lock (_gate)
        {
            long generation = ++_nextGeneration;
            int retryAttempts = preserveRetryAttempts &&
                _entries.TryGetValue(path,out Entry? previous)
                    ? previous.RetryAttempts
                    : 0;
            _entries[path] = new Entry
            {
                Path = path,
                State = DesktopAutoOrganizationItemState.Pending,
                Generation = generation,
                RetryAttempts = retryAttempts
            };
            return new DesktopAutoOrganizationWorkItem(path,generation);
        }
    }

    public bool TryTransition(
        DesktopAutoOrganizationWorkItem workItem,        DesktopAutoOrganizationItemState expected,        DesktopAutoOrganizationItemState next)
    {
        lock (_gate)
        {
            if (!TryGetCurrent(workItem,out Entry entry) || entry.State != expected)
            {
                return false;
            }

            entry.State = next;
            return true;
        }
    }

    public bool IsCurrent(
        DesktopAutoOrganizationWorkItem workItem,        DesktopAutoOrganizationItemState expectedState)
    {
        lock (_gate)
        {
            return TryGetCurrent(workItem,out Entry entry) &&
                   entry.State == expectedState;
        }
    }

    public bool MarkTerminal(
        DesktopAutoOrganizationWorkItem workItem,        DesktopAutoOrganizationItemState terminalState)
    {
        if (terminalState is not (
            DesktopAutoOrganizationItemState.Completed or
            DesktopAutoOrganizationItemState.Ignored or
            DesktopAutoOrganizationItemState.Missing))
        {
            throw new ArgumentOutOfRangeException(nameof(terminalState));
        }

        lock (_gate)
        {
            if (!TryGetCurrent(workItem,out Entry entry))
            {
                return false;
            }

            entry.State = terminalState;
            entry.RetryAttempts = 0;
            entry.NextRetryAt = null;
            PruneTerminalEntries();
            return true;
        }
    }

    public bool MarkDeferred(
        DesktopAutoOrganizationWorkItem workItem,        DateTimeOffset nextRetryAt,        bool countAttempt = true)
    {
        lock (_gate)
        {
            if (!TryGetCurrent(workItem,out Entry entry))
            {
                return false;
            }

            entry.State = DesktopAutoOrganizationItemState.Deferred;
            if (countAttempt)
            {
                entry.RetryAttempts++;
            }
            entry.NextRetryAt = nextRetryAt;
            return true;
        }
    }

    public void MarkRenamedOrMissing(string path)
    {
        lock (_gate)
        {
            _entries[path] = new Entry
            {
                Path = path,
                State = DesktopAutoOrganizationItemState.Missing,
                Generation = ++_nextGeneration
            };
            PruneTerminalEntries();
        }
    }

    public IReadOnlyList<string> SuspendRecoverableItems()
    {
        lock (_gate)
        {
            var deferred = new List<string>();
            foreach (Entry entry in _entries.Values)
            {
                if (entry.State is DesktopAutoOrganizationItemState.Pending or
                    DesktopAutoOrganizationItemState.Settling)
                {
                    // Invalidate work that is still safe to suspend. Processing
                    // is allowed to finish because a move may already be in flight.
                    entry.Generation = ++_nextGeneration;
                    entry.State = DesktopAutoOrganizationItemState.Deferred;
                    entry.NextRetryAt = null;
                }

                if (entry.State == DesktopAutoOrganizationItemState.Deferred)
                {
                    entry.NextRetryAt = null;
                    deferred.Add(entry.Path);
                }
            }

            return deferred;
        }
    }

    public void ResumeDeferred(DateTimeOffset now)
    {
        lock (_gate)
        {
            foreach (Entry entry in _entries.Values)
            {
                if (entry.State == DesktopAutoOrganizationItemState.Deferred)
                {
                    entry.NextRetryAt = now;
                }
            }
        }
    }

    public IReadOnlyList<DesktopAutoOrganizationWorkItem> TakeDueDeferred(
        DateTimeOffset now)
    {
        lock (_gate)
        {
            var due = new List<DesktopAutoOrganizationWorkItem>();
            foreach (Entry entry in _entries.Values)
            {
                if (entry.State != DesktopAutoOrganizationItemState.Deferred ||
                    entry.NextRetryAt is null ||
                    entry.NextRetryAt > now)
                {
                    continue;
                }

                entry.Generation = ++_nextGeneration;
                entry.State = DesktopAutoOrganizationItemState.Pending;
                entry.NextRetryAt = null;
                due.Add(new DesktopAutoOrganizationWorkItem(entry.Path,entry.Generation));
            }

            return due;
        }
    }

    public DateTimeOffset? GetNextRetryAt()
    {
        lock (_gate)
        {
            return _entries.Values
                .Where(entry =>
                    entry.State == DesktopAutoOrganizationItemState.Deferred &&
                    entry.NextRetryAt is not null)
                .Select(entry => entry.NextRetryAt)
                .Min();
        }
    }

    public IReadOnlyList<string> GetNonTerminalPaths()
    {
        lock (_gate)
        {
            return _entries.Values
                .Where(entry => entry.State is
                    DesktopAutoOrganizationItemState.Pending or
                    DesktopAutoOrganizationItemState.Settling or
                    DesktopAutoOrganizationItemState.Processing or
                    DesktopAutoOrganizationItemState.Deferred)
                .Select(entry => entry.Path)
                .ToArray();
        }
    }

    public DesktopAutoOrganizationStateSnapshot? GetSnapshot(string path)
    {
        lock (_gate)
        {
            return _entries.TryGetValue(path,out Entry? entry)
                ? new DesktopAutoOrganizationStateSnapshot(
                    entry.Path,                    entry.State,                    entry.Generation,                    entry.RetryAttempts,                    entry.NextRetryAt)
                : null;
        }
    }

    private bool TryGetCurrent(
        DesktopAutoOrganizationWorkItem workItem,        out Entry entry)
    {
        if (_entries.TryGetValue(workItem.Path,out Entry? found) &&
            found.Generation == workItem.Generation)
        {
            entry = found;
            return true;
        }

        entry = null!;
        return false;
    }

    private void PruneTerminalEntries()
    {
        const int retainedTerminalLimit = 512;
        Entry[] terminalEntries = _entries.Values
            .Where(entry => entry.State is
                DesktopAutoOrganizationItemState.Completed or
                DesktopAutoOrganizationItemState.Ignored or
                DesktopAutoOrganizationItemState.Missing)
            .OrderByDescending(entry => entry.Generation)
            .ToArray();
        foreach (Entry entry in terminalEntries.Skip(retainedTerminalLimit))
        {
            _entries.Remove(entry.Path);
        }
    }

    private sealed class Entry
    {
        public required string Path { get; init; }

        public DesktopAutoOrganizationItemState State { get; set; }

        public long Generation { get; set; }

        public int RetryAttempts { get; set; }

        public DateTimeOffset? NextRetryAt { get; set; }
    }
}
