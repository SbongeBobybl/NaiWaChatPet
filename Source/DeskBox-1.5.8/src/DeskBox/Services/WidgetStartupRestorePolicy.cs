using DeskBox.Models;

namespace DeskBox.Services;

/// <summary>
/// Defines which persisted widgets are restored when DeskBox starts. Startup
/// visibility is intentionally session-based: a previous process shutdown must
/// not leave otherwise enabled widgets hidden for the next launch.
/// </summary>
internal static class WidgetStartupRestorePolicy
{
    /// <summary>
    /// Decides whether the restored widgets stay hidden once startup has
    /// created them. The quick-reveal layer already keeps the desktop clean
    /// until the first reveal; the silent-startup preference opts every
    /// layer mode into that same behavior. Returns the diagnostics reason
    /// for the hide, or null when the widgets should show immediately.
    /// </summary>
    public static string? GetStartupHideReason(bool usesQuickRevealLayer,AppSettings settings)
    {
        ArgumentNullException.ThrowIfNull(settings);

        if (usesQuickRevealLayer)
        {
            return "quick-reveal-layer";
        }

        return settings.Core.SilentStartup ? "silent-startup" : null;
    }

    public static IReadOnlyList<WidgetConfig> SelectEnabledWidgets(
        AppSettings settings,        Func<string, bool> isDeleted)
    {
        ArgumentNullException.ThrowIfNull(settings);
        ArgumentNullException.ThrowIfNull(isDeleted);

        return settings.Widgets
            .Where(widget =>
                !widget.IsDisabled &&
                !isDeleted(widget.Id) &&
                WidgetGroupSettings.IsActiveMember(settings,widget.Id))
            .ToList();
    }

    public static bool MarkVisible(
        AppSettings settings,        IEnumerable<WidgetConfig> widgets)
    {
        ArgumentNullException.ThrowIfNull(settings);
        ArgumentNullException.ThrowIfNull(widgets);

        bool changed = false;
        foreach (WidgetConfig widget in widgets)
        {
            WidgetGroupConfig? group = WidgetGroupSettings.FindByMember(
                settings,                widget.Id);
            if (group is not null &&
                string.Equals(group.ActiveMemberId,widget.Id,StringComparison.Ordinal))
            {
                if (!group.IsVisible)
                {
                    group.IsVisible = true;
                    changed = true;
                }

                foreach (string memberId in group.MemberIds)
                {
                    WidgetConfig? member = settings.Widgets.FirstOrDefault(candidate =>
                        string.Equals(candidate.Id,memberId,StringComparison.Ordinal));
                    if (member is not null && !member.IsVisible)
                    {
                        member.IsVisible = true;
                        changed = true;
                    }
                }

                continue;
            }

            if (!widget.IsVisible)
            {
                widget.IsVisible = true;
                changed = true;
            }
        }

        return changed;
    }
}
