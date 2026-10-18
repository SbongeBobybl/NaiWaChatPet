using Windows.Graphics;

namespace DeskBox.Models;

public readonly record struct WidgetSurfaceBoundsExpansion(
    RectInt32 HostBounds,    bool IsNavigationInset,    int NavigationHeight);

/// <summary>
/// Converts between the persisted content-card rectangle and the physical
/// Surface-host rectangle that also contains the external navigation bar.
/// </summary>
public static class WidgetSurfaceBoundsCalculator
{
    public static WidgetSurfaceBoundsExpansion Expand(
        RectInt32 contentBounds,        RectInt32 workArea,        double navigationLogicalHeight,        double dpiScale)
    {
        int navigationHeight = Math.Max(
            1,            (int)Math.Round(navigationLogicalHeight * Math.Max(0.01,dpiScale)));
        bool inset = contentBounds.Y - navigationHeight < workArea.Y;
        return new WidgetSurfaceBoundsExpansion(
            inset
                ? contentBounds
                : new RectInt32(
                    contentBounds.X,                    contentBounds.Y - navigationHeight,                    contentBounds.Width,                    contentBounds.Height + navigationHeight),            inset,            navigationHeight);
    }

    public static RectInt32 Collapse(
        RectInt32 hostBounds,        bool isNavigationInset,        int navigationHeight)
    {
        return isNavigationInset
            ? hostBounds
            : new RectInt32(
                hostBounds.X,                hostBounds.Y + Math.Max(1,navigationHeight),                hostBounds.Width,                Math.Max(1,hostBounds.Height - Math.Max(1,navigationHeight)));
    }
}
