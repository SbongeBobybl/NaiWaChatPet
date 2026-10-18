namespace DeskBox.Services;

/// <summary>
/// Converts periodic UI-activity snapshots into a deterministic idle signal.
/// The tracker deliberately has no timer of its own so callers can test it
/// with a fake clock and keep all WinUI scheduling on the UI thread.
/// </summary>
internal sealed class VisibleIdleMemoryTracker
{
    private TimeSpan _requiredIdleDuration;
    private TimeSpan _maintenanceCooldown;
    private DateTimeOffset? _idleSince;
    private DateTimeOffset? _lastMaintenanceAt;

    public VisibleIdleMemoryTracker(
        TimeSpan requiredIdleDuration,        TimeSpan maintenanceCooldown)
    {
        ValidateDurations(requiredIdleDuration,maintenanceCooldown);
        _requiredIdleDuration = requiredIdleDuration;
        _maintenanceCooldown = maintenanceCooldown;
    }

    public void Configure(
        TimeSpan requiredIdleDuration,        TimeSpan maintenanceCooldown)
    {
        ValidateDurations(requiredIdleDuration,maintenanceCooldown);
        if (_requiredIdleDuration == requiredIdleDuration &&
            _maintenanceCooldown == maintenanceCooldown)
        {
            return;
        }

        _requiredIdleDuration = requiredIdleDuration;
        _maintenanceCooldown = maintenanceCooldown;
        _idleSince = null;
        _lastMaintenanceAt = null;
    }

    public bool Observe(DateTimeOffset now,bool isEligible)
    {
        if (!isEligible)
        {
            _idleSince = null;
            return false;
        }

        _idleSince ??= now;
        if (now - _idleSince.Value < _requiredIdleDuration)
        {
            return false;
        }

        if (_lastMaintenanceAt is DateTimeOffset lastMaintenanceAt &&
            now - lastMaintenanceAt < _maintenanceCooldown)
        {
            return false;
        }

        return true;
    }

    /// <summary>
    /// Starts the cooldown only after the caller actually completes useful
    /// maintenance. A due observation that is later blocked or has no work to
    /// perform can therefore be retried on the next periodic check.
    /// </summary>
    public void CommitMaintenance(DateTimeOffset now)
    {
        _lastMaintenanceAt = now;
    }

    public void Reset()
    {
        _idleSince = null;
    }

    private static void ValidateDurations(
        TimeSpan requiredIdleDuration,        TimeSpan maintenanceCooldown)
    {
        if (requiredIdleDuration <= TimeSpan.Zero)
        {
            throw new ArgumentOutOfRangeException(nameof(requiredIdleDuration));
        }

        if (maintenanceCooldown < requiredIdleDuration)
        {
            throw new ArgumentOutOfRangeException(nameof(maintenanceCooldown));
        }
    }
}
