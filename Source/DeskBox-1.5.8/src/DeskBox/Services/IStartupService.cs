using DeskBox.Models;

namespace DeskBox.Services;

public enum StartupRegistrationState
{
    Enabled,
    DisabledByUser,
    NotRegistered,
    PathMismatch,
    Pending,
    BlockedOrFailed,
    DisabledByTaskScheduler
}

public sealed record StartupOperationResult(
    StartupRegistrationState State,    string ErrorMessage = "",    StartupMode? EffectiveMode = null,    bool UsedFallback = false)
{
    public bool IsEnabled => State == StartupRegistrationState.Enabled;

    public bool RequiresSystemSettings =>
        State == StartupRegistrationState.DisabledByUser;
}

public interface IStartupService
{
    StartupRegistrationState GetState();
    bool IsEnabled();
    string? GetRunValue();
    StartupOperationResult Enable();
    StartupOperationResult Disable();
    StartupOperationResult SetEnabled(bool enabled);
}
