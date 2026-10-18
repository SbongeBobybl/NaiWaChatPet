namespace DeskBox.Services;

/// <summary>
/// Weak-reference bookkeeping for the set of items (windows) a service
/// tracks. Pure logic keyed on reference identity so the tracking contract
/// stays unit-testable without a WinUI runtime, and so a bookkeeping bug
/// can never pin a window in memory.
/// </summary>
internal sealed class WindowTrackingRegistry<T> where T : class
{
    private readonly List<WeakReference<T>> _tracked = [];

    /// <summary>
    /// Registers <paramref name="item"/>. Returns false when it is already
    /// tracked, so tracking is idempotent and event subscriptions stay
    /// single.
    /// </summary>
    public bool Track(T item)
    {
        ArgumentNullException.ThrowIfNull(item);
        SweepDead();
        if (IndexOf(item) >= 0)
        {
            return false;
        }

        _tracked.Add(new WeakReference<T>(item));
        return true;
    }

    /// <summary>
    /// Removes <paramref name="item"/>. Returns false when it was not
    /// tracked, so untracking is idempotent.
    /// </summary>
    public bool Untrack(T item)
    {
        ArgumentNullException.ThrowIfNull(item);
        int index = IndexOf(item);
        if (index < 0)
        {
            return false;
        }

        _tracked.RemoveAt(index);
        return true;
    }

    /// <summary>
    /// A real close (never a cancelled one) removes the window from
    /// tracking. Semantically identical to <see cref="Untrack"/>; the
    /// separate name keeps close-path call sites self-describing.
    /// </summary>
    public bool NotifyClosed(T item) => Untrack(item);

    /// <summary>
    /// Returns the live tracked items and sweeps dead references as a side
    /// effect, so enumeration can never observe or retain a dead window.
    /// </summary>
    public List<T> EnumerateAlive()
    {
        SweepDead();
        var alive = new List<T>(_tracked.Count);
        foreach (WeakReference<T> entry in _tracked)
        {
            if (entry.TryGetTarget(out T? target))
            {
                alive.Add(target);
            }
        }

        return alive;
    }

    /// <summary>Live entry count after sweeping dead references.</summary>
    public int TrackedCount
    {
        get
        {
            SweepDead();
            return _tracked.Count;
        }
    }

    private int IndexOf(T item)
    {
        for (int i = 0; i < _tracked.Count; i++)
        {
            if (_tracked[i].TryGetTarget(out T? target) &&
                ReferenceEquals(target,item))
            {
                return i;
            }
        }

        return -1;
    }

    private void SweepDead() =>
        _tracked.RemoveAll(entry => !entry.TryGetTarget(out _));
}
