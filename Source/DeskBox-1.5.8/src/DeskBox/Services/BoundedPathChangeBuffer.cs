namespace DeskBox.Services;

internal enum BoundedPathChangeWriteResult
{
    AddedOrUpdated,
    Overflowed,
    IgnoredAfterOverflow
}

/// <summary>
/// Keeps a coalesced path-to-change map within a fixed entry budget. Once the
/// budget is exceeded, individual changes are discarded and the caller must
/// perform a full reconciliation before treating the persisted state as clean.
/// Callers are responsible for synchronization.
/// </summary>
internal sealed class BoundedPathChangeBuffer<TValue>
{
    private readonly int _capacity;
    private readonly Dictionary<string, TValue> _entries;

    public BoundedPathChangeBuffer(
        int capacity,        IEqualityComparer<string>? comparer = null)
    {
        ArgumentOutOfRangeException.ThrowIfNegativeOrZero(capacity);
        _capacity = capacity;
        _entries = new Dictionary<string, TValue>(
            capacity,            comparer ?? StringComparer.Ordinal);
    }

    public int Count => _entries.Count;
    public bool IsOverflowed { get; private set; }
    public IEnumerable<KeyValuePair<string, TValue>> Entries => _entries;
    public IEnumerable<string> Keys => _entries.Keys;

    public BoundedPathChangeWriteResult Set(string path,TValue value)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(path);
        if (IsOverflowed)
        {
            return BoundedPathChangeWriteResult.IgnoredAfterOverflow;
        }

        if (_entries.Count >= _capacity && !_entries.ContainsKey(path))
        {
            _entries.Clear();
            IsOverflowed = true;
            return BoundedPathChangeWriteResult.Overflowed;
        }

        _entries[path] = value;
        return BoundedPathChangeWriteResult.AddedOrUpdated;
    }

    public void Reset()
    {
        _entries.Clear();
        IsOverflowed = false;
    }
}
