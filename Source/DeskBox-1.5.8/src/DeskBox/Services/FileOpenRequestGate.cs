namespace DeskBox.Services;

/// <summary>
/// UI-thread-confined duplicate suppression. Only path strings and timestamps
/// are retained, never WidgetItem instances, decoded icons, or XAML controls.
/// </summary>
internal sealed class FileOpenRequestGate
{
    internal const int HistoryLimit = 128;
    private readonly HashSet<string> _active = new(StringComparer.OrdinalIgnoreCase);
    private readonly Dictionary<string, long> _recent = new(StringComparer.OrdinalIgnoreCase);

    internal int HistoryCount => _recent.Count;

    internal bool TryBegin(string path,long now,uint doubleClickTimeMs)
    {
        string key = Normalize(path);
        if (_active.Contains(key))
        {
            return false;
        }

        foreach (string expired in _recent
                     .Where(entry => now - entry.Value >= doubleClickTimeMs)
                     .Select(entry => entry.Key)
                     .ToArray())
        {
            _recent.Remove(expired);
        }

        if (_recent.ContainsKey(key))
        {
            return false;
        }

        if (_recent.Count >= HistoryLimit)
        {
            _recent.Remove(_recent.MinBy(entry => entry.Value).Key);
        }

        _active.Add(key);
        _recent[key] = now;
        return true;
    }

    internal bool IsActive(string path) => _active.Contains(Normalize(path));

    internal void Complete(string path,bool dispatched)
    {
        string key = Normalize(path);
        _active.Remove(key);
        if (!dispatched)
        {
            // A failure or queue rejection must not delay an intentional retry.
            _recent.Remove(key);
        }
    }

    internal void Clear()
    {
        _active.Clear();
        _recent.Clear();
    }

    internal void ClearRecent()
    {
        _recent.Clear();
    }

    private static string Normalize(string path)
    {
        try
        {
            // Lexical only: no disk/network access on the input thread.
            return Path.IsPathFullyQualified(path)
                ? Path.GetFullPath(path)
                : path;
        }
        catch (Exception ex) when (ex is ArgumentException or NotSupportedException)
        {
            return path;
        }
    }
}
