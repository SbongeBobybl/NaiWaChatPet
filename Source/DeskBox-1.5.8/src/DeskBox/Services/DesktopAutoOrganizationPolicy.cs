using DeskBox.Models;

namespace DeskBox.Services;

/// <summary>
/// Presets for how long a newly observed desktop file waits before desktop
/// auto-organization processes it. Values outside the presets are normalized
/// so hand-edited settings files can never reach the watcher.
/// </summary>
public static class DesktopAutoOrganizationPolicy
{
    /// <summary>The realtime preset: 10 seconds after the file last changed.</summary>
    public const int DefaultDelaySeconds = 10;

    /// <summary>Delay presets offered in settings, in ascending order.</summary>
    public static readonly int[] SupportedDelaySeconds =
        [10,60,5 * 60,30 * 60,60 * 60,12 * 60 * 60];

    public static int NormalizeDelaySeconds(int value) =>
        SupportedDelaySeconds.Contains(value) ? value : DefaultDelaySeconds;

    public static TimeSpan GetDelay(AppSettings settings) => TimeSpan.FromSeconds(
        NormalizeDelaySeconds(settings.DesktopOrganization.DesktopAutoOrganizationDelaySeconds));
}
