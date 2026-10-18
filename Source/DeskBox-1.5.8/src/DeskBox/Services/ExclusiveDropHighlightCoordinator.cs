namespace DeskBox.Services;

/// <summary>
/// Tracks the single widget that may present an external-file drop highlight.
/// A weak reference avoids extending the lifetime of a closed widget window.
/// </summary>
internal sealed class ExclusiveDropHighlightCoordinator<T>
    where T : class
{
    private readonly object _gate = new();
    private WeakReference<T>? _activeOwner;

    public T? Activate(T owner)
    {
        ArgumentNullException.ThrowIfNull(owner);

        lock (_gate)
        {
            T? previousOwner = null;
            if (_activeOwner is not null &&
                _activeOwner.TryGetTarget(out T? activeOwner) &&
                !ReferenceEquals(activeOwner,owner))
            {
                previousOwner = activeOwner;
            }

            _activeOwner = new WeakReference<T>(owner);
            return previousOwner;
        }
    }

    public void Deactivate(T owner)
    {
        ArgumentNullException.ThrowIfNull(owner);

        lock (_gate)
        {
            if (_activeOwner is not null &&
                _activeOwner.TryGetTarget(out T? activeOwner) &&
                ReferenceEquals(activeOwner,owner))
            {
                _activeOwner = null;
            }
        }
    }

    public T? DeactivateActive()
    {
        lock (_gate)
        {
            T? activeOwner = null;
            _activeOwner?.TryGetTarget(out activeOwner);
            _activeOwner = null;
            return activeOwner;
        }
    }
}
