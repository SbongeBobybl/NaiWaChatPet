namespace DeskBox.Models;

/// <summary>
/// Desktop organization rules, auto-organization baseline, and the recent-organization history (local-domain data, still stored in settings.json).
/// </summary>
public sealed class DesktopOrganizationSettingsSlice
{
    /// <summary>
    /// Recent organization history used for undo and quick review.
    /// </summary>
    public List<OrganizationHistoryEntry> RecentOrganizationHistory { get; set; } = [];

    /// <summary>
    /// Stable per-widget routing rules used by desktop one-click and automatic
    /// organization. Rules target widget IDs and therefore survive renames and
    /// group membership changes.
    /// </summary>
    public List<DesktopOrganizationRule> DesktopOrganizationRules { get; set; } = [];

    /// <summary>
    /// Whether newly created, stable desktop files may be routed automatically.
    /// This is deliberately opt-in.
    /// </summary>
    public bool DesktopAutoOrganizationEnabled { get; set; }

    /// <summary>
    /// How long a newly observed desktop file waits before automatic
    /// organization processes it; always one of the preset values in
    /// <see cref="Services.DesktopAutoOrganizationPolicy.SupportedDelaySeconds"/>.
    /// The default is the realtime preset (10 seconds).
    /// </summary>
    public int DesktopAutoOrganizationDelaySeconds { get; set; } = 10;

    /// <summary>
    /// Baseline created when automatic organization is enabled. Existing
    /// desktop content is never processed merely by enabling the feature.
    /// </summary>
    public DateTimeOffset? DesktopAutoOrganizationBaselineUtc { get; set; }

    /// <summary>
    /// User-edited category template for the NaiWa-assisted preview, stored as
    /// "categoryId=displayName" entries. Empty means the built-in template.
    /// Ids stay stable so remembered rules keep routing after a rename.
    /// </summary>
    public List<string> NaiWaCategoryTemplate { get; set; } = [];
    public string NaiWaWidgetTheme { get; set; } = "light";
    public bool NaiWaTransparentBody { get; set; } = true;

    /// <summary>
    /// How much of the pet's carry animation to play before the files are
    /// handled. <c>full</c> walks to every category (one trip per category),
    /// <c>simple</c> plays a single trip for the whole plan, <c>none</c> skips
    /// the animation entirely. Values are validated by
    /// <see cref="Services.NaiWaAnimationPolicy"/>.
    /// </summary>
    public string NaiWaCarryAnimation { get; set; } = Services.NaiWaAnimationPolicy.Full;
}
