namespace DeskBox.Services;

/// <summary>
/// Pixel-budgeted LRU used by a Surface for low-cost transition visuals.
/// It never owns live member view models or XAML trees.
/// </summary>
internal sealed class WidgetSurfaceSnapshotCache<TSnapshot>
    where TSnapshot : class
{
    private readonly long _pixelBudget;
    private readonly int _entryLimit;
    private readonly Dictionary<string, Entry> _entries =
        new(StringComparer.Ordinal);
    private readonly LinkedList<string> _lru = new();
    private readonly object _gate = new();
    private long _totalPixels;

    public WidgetSurfaceSnapshotCache(long pixelBudget,int entryLimit = 3)
    {
        if (pixelBudget <= 0)
        {
            throw new ArgumentOutOfRangeException(nameof(pixelBudget));
        }
        if (entryLimit <= 0)
        {
            throw new ArgumentOutOfRangeException(nameof(entryLimit));
        }

        _pixelBudget = pixelBudget;
        _entryLimit = entryLimit;
    }

    public long PixelBudget => _pixelBudget;

    public long TotalPixels
    {
        get
        {
            lock (_gate)
            {
                return _totalPixels;
            }
        }
    }

    public int Count
    {
        get
        {
            lock (_gate)
            {
                return _entries.Count;
            }
        }
    }

    public void AddOrUpdate(
        string memberId,        TSnapshot snapshot,        int pixelWidth,        int pixelHeight)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(memberId);
        ArgumentNullException.ThrowIfNull(snapshot);
        long pixels = checked((long)Math.Max(1,pixelWidth) *
                              Math.Max(1,pixelHeight));

        lock (_gate)
        {
            RemoveCore(memberId);
            if (pixels > _pixelBudget)
            {
                return;
            }

            var node = _lru.AddFirst(memberId);
            _entries[memberId] = new Entry(snapshot,pixels,node);
            _totalPixels += pixels;
            while ((_totalPixels > _pixelBudget ||
                    _entries.Count > _entryLimit) &&
                   _lru.Last is { } last)
            {
                RemoveCore(last.Value);
            }
        }
    }

    public bool TryGet(string memberId,out TSnapshot? snapshot)
    {
        lock (_gate)
        {
            if (!_entries.TryGetValue(memberId,out Entry? entry))
            {
                snapshot = null;
                return false;
            }

            _lru.Remove(entry.Node);
            _lru.AddFirst(entry.Node);
            snapshot = entry.Snapshot;
            return true;
        }
    }

    public bool Remove(string memberId)
    {
        lock (_gate)
        {
            return RemoveCore(memberId);
        }
    }

    public void Clear()
    {
        lock (_gate)
        {
            _entries.Clear();
            _lru.Clear();
            _totalPixels = 0;
        }
    }

    private bool RemoveCore(string memberId)
    {
        if (!_entries.Remove(memberId,out Entry? entry))
        {
            return false;
        }

        _lru.Remove(entry.Node);
        _totalPixels -= entry.Pixels;
        return true;
    }

    private sealed record Entry(
        TSnapshot Snapshot,        long Pixels,        LinkedListNode<string> Node);
}
