namespace DeskBox.Controls;

/// <summary>
/// Pointer feedback for one realized file surface. A successful Shell open
/// ends the current highlight even if focus changes before PointerReleased.
/// This only controls painting; it never consumes input or changes selection.
/// </summary>
internal struct FileItemPointerFeedback
{
    private bool _suppressReleaseHover;
    private (double X, double Y)? _lastPointerPosition;

    internal FileItemSurfaceVisualState OnOpenDispatched()
    {
        _suppressReleaseHover = true;
        return FileItemSurfaceVisualState.Normal;
    }

    internal FileItemSurfaceVisualState OnPointerEntered()
    {
        _suppressReleaseHover = false;
        return FileItemSurfaceVisualState.Hover;
    }

    internal FileItemSurfaceVisualState OnPointerPressed()
    {
        _suppressReleaseHover = false;
        return FileItemSurfaceVisualState.Pressed;
    }

    internal FileItemSurfaceVisualState OnPointerMoved(
        FileItemSurfaceVisualState currentState,        bool isInContact,        double x,        double y)
    {
        bool positionChanged = _lastPointerPosition != (x, y);
        RecordPointerPosition(x,y);
        // Layout/focus changes can repeat the same pointer sample. They must
        // not undo open-success cleanup while the mouse remains stationary.
        return isInContact || (_suppressReleaseHover && !positionChanged)
            ? currentState
            : OnPointerEntered();
    }

    internal void RecordPointerPosition(double x,double y) =>
        _lastPointerPosition = (x, y);

    internal FileItemSurfaceVisualState OnPointerReleased(bool inside) =>
        inside && !_suppressReleaseHover
            ? FileItemSurfaceVisualState.Hover
            : FileItemSurfaceVisualState.Normal;

    internal void ResetForReuse()
    {
        _suppressReleaseHover = false;
        _lastPointerPosition = null;
    }
}
