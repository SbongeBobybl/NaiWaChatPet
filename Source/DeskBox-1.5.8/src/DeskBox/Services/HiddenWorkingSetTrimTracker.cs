namespace DeskBox.Services;

/// <summary>
/// Coalesces native visibility and batch-animation notifications into one
/// request per visible-to-hidden transition. Used only on the UI thread.
/// </summary>
internal sealed class HiddenWorkingSetTrimTracker
{
    private bool _observedVisibleWidgets;
    private bool _requestPending;
    private long _generation;

    internal bool TrimmedCurrentHiddenSession { get; private set; }

    internal long? Observe(WidgetMemoryVisibilitySnapshot visibility,bool enabled)
    {
        if (visibility.HasNativeVisibleWidgets || visibility.LogicalVisibleCount > 0)
        {
            _observedVisibleWidgets = true;
            TrimmedCurrentHiddenSession = false;
            CancelPending();
            return null;
        }

        if (!_observedVisibleWidgets)
        {
            return null;
        }

        _observedVisibleWidgets = false;
        CancelPending();
        if (!enabled || visibility.LoadedWindowCount == 0)
        {
            return null;
        }

        _requestPending = true;
        return _generation;
    }

    internal bool IsPending(long generation) =>
        _requestPending && generation == _generation;

    internal bool TryConsume(long generation)
    {
        if (!IsPending(generation))
        {
            return false;
        }

        _requestPending = false;
        return true;
    }

    internal void Complete(long generation,bool trimmed)
    {
        if (generation == _generation && trimmed)
        {
            TrimmedCurrentHiddenSession = true;
        }
    }

    internal void CancelPending()
    {
        _requestPending = false;
        _generation++;
    }
}
