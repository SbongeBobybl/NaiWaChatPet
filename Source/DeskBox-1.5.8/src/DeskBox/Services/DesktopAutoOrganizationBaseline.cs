namespace DeskBox.Services;

/// <summary>
/// Holds the last complete desktop enumeration. Incomplete captures are
/// deliberately rejected so a transient IO/access failure cannot erase the
/// known baseline and make old files look newly created.
/// </summary>
internal sealed class DesktopAutoOrganizationBaseline
{
    private readonly object _gate = new();
    private readonly HashSet<string> _paths = new(StringComparer.OrdinalIgnoreCase);

    public bool Contains(string path)
    {
        lock (_gate)
        {
            return _paths.Contains(path);
        }
    }

    public HashSet<string> Snapshot()
    {
        lock (_gate)
        {
            return new HashSet<string>(_paths,StringComparer.OrdinalIgnoreCase);
        }
    }

    public bool TryReplace(
        bool captureIsComplete,        IEnumerable<string> currentPaths,        IEnumerable<string>? pendingPaths = null,        IEnumerable<string>? excludedPaths = null)
    {
        if (!captureIsComplete)
        {
            return false;
        }

        var replacement = new HashSet<string>(
            currentPaths,            StringComparer.OrdinalIgnoreCase);
        if (pendingPaths is not null)
        {
            replacement.ExceptWith(pendingPaths);
        }

        if (excludedPaths is not null)
        {
            replacement.ExceptWith(excludedPaths);
        }

        lock (_gate)
        {
            _paths.Clear();
            _paths.UnionWith(replacement);
        }

        return true;
    }

    public void Add(string path)
    {
        lock (_gate)
        {
            _paths.Add(path);
        }
    }

    public void Remove(string path)
    {
        lock (_gate)
        {
            _paths.Remove(path);
        }
    }
}

internal readonly record struct DesktopAutoOrganizationBaselineEventBatch(
    string[] Changed,    string[] Forced,    string[] Deleted);

internal sealed class DesktopAutoOrganizationBaselineEventBuffer
{
    private readonly object _gate = new();
    private readonly HashSet<string> _changed = new(StringComparer.OrdinalIgnoreCase);
    private readonly HashSet<string> _forced = new(StringComparer.OrdinalIgnoreCase);
    private readonly HashSet<string> _deleted = new(StringComparer.OrdinalIgnoreCase);
    private bool _active;

    public void Begin()
    {
        lock (_gate)
        {
            _changed.Clear();
            _forced.Clear();
            _deleted.Clear();
            _active = true;
        }
    }

    public bool TryBufferChange(string path,bool bypassBaseline)
    {
        lock (_gate)
        {
            if (!_active)
            {
                return false;
            }

            (bypassBaseline ? _forced : _changed).Add(path);
            return true;
        }
    }

    public bool TryBufferDeletion(string path)
    {
        lock (_gate)
        {
            if (!_active)
            {
                return false;
            }

            _deleted.Add(path);
            return true;
        }
    }

    public bool TryDrain(out DesktopAutoOrganizationBaselineEventBatch batch)
    {
        lock (_gate)
        {
            if (_changed.Count == 0 && _forced.Count == 0 && _deleted.Count == 0)
            {
                _active = false;
                batch = default;
                return false;
            }

            batch = new DesktopAutoOrganizationBaselineEventBatch(
                _changed.ToArray(),                _forced.ToArray(),                _deleted.ToArray());
            _changed.Clear();
            _forced.Clear();
            _deleted.Clear();
            return true;
        }
    }
}
