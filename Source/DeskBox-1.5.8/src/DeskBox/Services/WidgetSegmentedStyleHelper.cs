using CommunityToolkit.WinUI.Controls;
using DeskBox.Helpers;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Media;

namespace DeskBox.Services;

public static class WidgetSegmentedStyleHelper
{
    /// <summary>
    /// Applies the widget tab style and the neutral pointer states in one step.
    /// </summary>
    public static void Apply(Segmented segmented,string? style)
    {
        ArgumentNullException.ThrowIfNull(segmented);

        ApplyNeutralPointerStates(segmented);

        string normalizedStyle = SettingsService.NormalizeWidgetTabStyle(style);
        if (normalizedStyle == SettingsService.WidgetTabStyleButton)
        {
            segmented.ClearValue(FrameworkElement.StyleProperty);
            return;
        }

        if (Application.Current.Resources.TryGetValue("WidgetPivotSegmentedStyle",out object resource) &&
            resource is Style segmentedStyle)
        {
            segmented.Style = segmentedStyle;
        }
    }

    /// <summary>
    /// Keeps a Segmented strip's hover and pressed states neutral.
    ///
    /// The CommunityToolkit template keeps its own copy of the Pivot item
    /// brushes and resolves them from the control, and the app widens the
    /// accent scope to include <c>SystemControlHighlightAccentBrush</c>, so a
    /// segment would otherwise wash with the accent color under the pointer.
    /// Only the pointer states are overridden: hovering or pressing a segment
    /// describes what the pointer is doing, while the selected item is a
    /// deliberate choice and keeps the DeskBox accent.
    /// </summary>
    public static void ApplyNeutralPointerStates(Segmented segmented)
    {
        ArgumentNullException.ThrowIfNull(segmented);

        ResourceDictionary resources = segmented.Resources;
        // Resolved by the segment's own theme, not the application scope: a
        // bare application lookup follows the system theme and inverts the
        // colors when the app's theme override disagrees with it.
        Brush? hover = NeutralInteractionBrush.ResolveThemedBrush(
            NeutralInteractionBrush.FillSecondaryKey,            segmented);
        Brush? pressed = NeutralInteractionBrush.ResolveThemedBrush(
            NeutralInteractionBrush.FillTertiaryKey,            segmented);
        Brush? foreground = NeutralInteractionBrush.ResolveThemedBrush(
            NeutralInteractionBrush.TextPrimaryKey,            segmented);

        SetBrush(resources,"PivotItemBackgroundPointerOver",hover);
        SetBrush(resources,"PivotItemBackgroundPressed",pressed ?? hover);
        SetBrush(resources,"PivotItemForegroundPointerOver",foreground);
    }

    private static void SetBrush(
        ResourceDictionary resources,        string key,        Brush? brush)
    {
        if (brush is null)
        {
            return;
        }

        // A per-instance copy keeps the shared theme dictionary untouched.
        // The copy is a snapshot: a theme flip re-resolves the template's
        // themed references to that same snapshot, so the pointer states must
        // be re-applied when the theme changes; the widget surfaces and
        // windows hook their theme-change handlers for this.
        resources[key] = brush is SolidColorBrush solid
            ? new SolidColorBrush(solid.Color)
            : brush;
    }
}
