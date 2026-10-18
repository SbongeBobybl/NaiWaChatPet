namespace DeskBox.Controls;

/// <summary>
/// A released mouse button does not mean the native drag loop has finished.
/// Keep recovery out of that loop until the source completion callback arrives.
/// </summary>
internal sealed class FileDragSessionState
{
    private string? _sessionId;

    internal bool IsSystemDragInProgress => _sessionId is not null;

    internal bool ReleaseRecoveryPending { get; private set; }

    internal void Begin(string sessionId)
    {
        ArgumentException.ThrowIfNullOrWhiteSpace(sessionId);
        _sessionId = sessionId;
        ReleaseRecoveryPending = false;
    }

    internal bool DeferReleaseRecovery()
    {
        if (!IsSystemDragInProgress)
        {
            return false;
        }

        ReleaseRecoveryPending = true;
        return true;
    }

    internal void Complete(string? sessionId)
    {
        if (string.Equals(_sessionId,sessionId,StringComparison.Ordinal))
        {
            _sessionId = null;
            ReleaseRecoveryPending = false;
        }
    }
}
