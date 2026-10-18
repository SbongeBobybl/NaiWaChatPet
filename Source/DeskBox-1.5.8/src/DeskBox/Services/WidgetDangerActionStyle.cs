using Microsoft.UI;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Media;

namespace DeskBox.Services;

/// <summary>
/// Applies the native Fluent critical color to widget-closing actions. The
/// theme resource is softer than pure red and automatically follows light,
/// dark, and high-contrast modes.
/// </summary>
internal static class WidgetDangerActionStyle
{
    internal const string CriticalBrushResourceKey =
        "SystemFillColorCriticalBrush";

    internal static void Apply(MenuFlyoutItem item)
    {
        ArgumentNullException.ThrowIfNull(item);
        Brush brush = ResolveBrush();
        item.Foreground = brush;
        if (item.Icon is FontIcon icon)
        {
            icon.Foreground = brush;
        }
    }

    private static Brush ResolveBrush()
    {
        if (Application.Current?.Resources.TryGetValue(
                CriticalBrushResourceKey,                out object? resource) == true &&
            resource is Brush brush)
        {
            return brush;
        }

        // Fluent light-theme critical foreground fallback (#C42B1C).
        return new SolidColorBrush(
            ColorHelper.FromArgb(0xFF,0xC4,0x2B,0x1C));
    }
}
