namespace DeskBox.Services;

internal readonly record struct DesktopDirectoryActivitySnapshot(
    int EventCount,    DateTimeOffset LastEventAt);

/// <summary>
/// Coalesces bursts of file-system notifications by parent directory. Download
/// and extraction tools often create several files in one directory before any
/// individual file is safe to move.
/// </summary>
internal sealed class DesktopAutoOrganizationActivityTracker
{
    private readonly object _gate = new();
    private readonly Dictionary<string, ActivityState> _directories =
        new(StringComparer.OrdinalIgnoreCase);
    private readonly TimeSpan _burstWindow;

    public DesktopAutoOrganizationActivityTracker(TimeSpan burstWindow)
    {
        _burstWindow = burstWindow;
    }

    public void Observe(string path,DateTimeOffset now)
    {
        string? parent = GetParentPath(path);
        if (parent is null)
        {
            return;
        }

        lock (_gate)
        {
            if (_directories.TryGetValue(parent,out ActivityState? state) &&
                now - state.LastEventAt <= _burstWindow)
            {
                state.EventCount++;
                state.LastEventAt = now;
            }
            else
            {
                _directories[parent] = new ActivityState
                {
                    EventCount = 1,
                    LastEventAt = now
                };
            }

            PruneStaleLocked(now);
        }
    }

    public DesktopDirectoryActivitySnapshot GetSnapshot(
        string path,        DateTimeOffset now)
    {
        string? parent = GetParentPath(path);
        if (parent is null)
        {
            return default;
        }

        lock (_gate)
        {
            if (!_directories.TryGetValue(parent,out ActivityState? state) ||
                now - state.LastEventAt > _burstWindow)
            {
                return default;
            }

            return new DesktopDirectoryActivitySnapshot(
                state.EventCount,                state.LastEventAt);
        }
    }

    private void PruneStaleLocked(DateTimeOffset now)
    {
        if (_directories.Count < 256)
        {
            return;
        }

        foreach (string path in _directories
                     .Where(pair => now - pair.Value.LastEventAt > _burstWindow)
                     .Select(pair => pair.Key)
                     .ToArray())
        {
            _directories.Remove(path);
        }
    }

    private static string? GetParentPath(string path)
    {
        try
        {
            return Path.GetDirectoryName(Path.GetFullPath(path));
        }
        catch
        {
            return null;
        }
    }

    private sealed class ActivityState
    {
        public int EventCount { get; set; }

        public DateTimeOffset LastEventAt { get; set; }
    }
}
