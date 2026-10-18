using DeskBox.Models;

namespace DeskBox.Services;

/// <summary>
/// Applies the product default for autostart exactly once. Desktop widgets are
/// only useful once they come back with the session, so an installation whose
/// user never made a choice is switched on a single time; from then on the
/// choice belongs to the user, and any state Windows reports as disabled is
/// left exactly as it is.
/// </summary>
internal static class AutoStartDefaultPolicy
{
    internal static bool ShouldApply(AppSettings settings)
    {
        ArgumentNullException.ThrowIfNull(settings);
        return !settings.AutoStartDefaultApplied;
    }

    /// <summary>
    /// Returns the registration state the settings mirror should record.
    /// Only a never-registered startup is enabled — a task that exists but is
    /// disabled (Task Manager, policy, another installation) is never
    /// overridden, and a Windows-consented enable reports <see
    /// cref="StartupRegistrationState.Pending"/>.
    /// </summary>
    internal static StartupRegistrationState Resolve(IStartupService startupService)
    {
        ArgumentNullException.ThrowIfNull(startupService);
        StartupRegistrationState state = startupService.GetState();
        return state == StartupRegistrationState.NotRegistered
            ? startupService.Enable().State
            : state;
    }

    internal static bool IsEnabledState(StartupRegistrationState state) =>
        state is StartupRegistrationState.Enabled or StartupRegistrationState.Pending;

    /// <summary>
    /// A blocked or failed attempt must not consume the one-time default: the
    /// next launch retries once the machine condition clears. Every other
    /// outcome (including every deliberate user or Windows disable) is final
    /// and still marks the default as applied.
    /// </summary>
    internal static bool ShouldMarkApplied(StartupRegistrationState effectiveState) =>
        effectiveState != StartupRegistrationState.BlockedOrFailed;
}
