namespace DeskBox.Services;

/// <summary>
/// Separates a short-lived surface suspension from resource teardown. Folder
/// watchers stay warm during a tray animation cycle, while changes observed in
/// the hidden interval are coalesced into one reconciliation on resume.
/// </summary>
internal sealed class WidgetSurfaceActivityTracker
{
    private readonly object _gate = new();
    private bool _isSuspended;
    private bool _hasDeferredChanges;

    public bool IsSuspended
    {
        get
        {
            lock (_gate)
            {
                return _isSuspended;
            }
        }
    }

    public void Suspend()
    {
        lock (_gate)
        {
            _isSuspended = true;
        }
    }

    public bool TryDeferChange()
    {
        lock (_gate)
        {
            if (!_isSuspended)
            {
                return false;
            }

            _hasDeferredChanges = true;
            return true;
        }
    }

    public bool Resume()
    {
        lock (_gate)
        {
            if (!_isSuspended)
            {
                return false;
            }

            _isSuspended = false;
            bool hasDeferredChanges = _hasDeferredChanges;
            _hasDeferredChanges = false;
            return hasDeferredChanges;
        }
    }
}
