namespace DeskBox.Services;

/// <summary>
/// Coalesces the two WinUI activation callbacks produced by one stack click.
/// Depending on template and input routing, ItemClick can run either before or
/// after the stack surface's PointerReleased handler.
/// </summary>
internal sealed class StackInputActivationArbiter
{
    private string? _pressedStackKey;
    private string? _releasedActivationKey;
    private bool _activatedDuringPointer;

    public void BeginPointer(string stackKey)
    {
        _pressedStackKey = stackKey;
        _releasedActivationKey = null;
        _activatedDuringPointer = false;
    }

    public bool ShouldActivateFromItemClick(string stackKey)
    {
        if (string.Equals(
                _releasedActivationKey,                stackKey,                StringComparison.Ordinal))
        {
            _releasedActivationKey = null;
            return false;
        }

        if (!string.Equals(
                _pressedStackKey,                stackKey,                StringComparison.Ordinal))
        {
            return true;
        }

        if (_activatedDuringPointer)
        {
            return false;
        }

        _activatedDuringPointer = true;
        return true;
    }

    public bool ShouldActivateFromPointerRelease(
        string stackKey,        bool isValidRelease)
    {
        if (!isValidRelease ||
            _activatedDuringPointer ||
            !string.Equals(
                _pressedStackKey,                stackKey,                StringComparison.Ordinal))
        {
            return false;
        }

        _activatedDuringPointer = true;
        _releasedActivationKey = stackKey;
        return true;
    }

    public void EndPointer()
    {
        _pressedStackKey = null;
        _activatedDuringPointer = false;
    }

    public void CancelPointer()
    {
        _pressedStackKey = null;
        _releasedActivationKey = null;
        _activatedDuringPointer = false;
    }
}
