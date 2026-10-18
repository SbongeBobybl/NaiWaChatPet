using DeskBox.Models;
using Windows.Graphics;

namespace DeskBox.Services;

internal static class InitialFileWidgetPlacementPolicy
{
    internal const double RightMargin = 24;
    internal const double TopMargin = 72;

    public static RectInt32 CalculateRightAlignedBounds(
        RectInt32 workArea,        double logicalWidth,        double logicalHeight,        double dpiScale)
    {
        double scale = double.IsFinite(dpiScale) && dpiScale > 0
            ? dpiScale
            : 1.0;
        int width = WidgetPositioningService.ToPhysicalPixels(
            Math.Max(SettingsService.MinWidgetWidth,logicalWidth),            scale);
        int height = WidgetPositioningService.ToPhysicalPixels(
            Math.Max(SettingsService.MinWidgetHeight,logicalHeight),            scale);
        int rightMargin = WidgetPositioningService.ToPhysicalPixels(RightMargin,scale);
        int topMargin = WidgetPositioningService.ToPhysicalPixels(TopMargin,scale);

        var requestedBounds = new RectInt32(
            workArea.X + workArea.Width - width - rightMargin,            workArea.Y + topMargin,            width,            height);
        return WidgetPositioningService.EnsureVisible(requestedBounds,workArea);
    }

    public static void Apply(
        WidgetConfig config,        RectInt32 workArea,        double dpiScale)
    {
        RectInt32 bounds = CalculateRightAlignedBounds(
            workArea,            config.Width,            config.Height,            dpiScale);
        WidgetPositioningService.UpdateConfigFromPhysicalBounds(config,bounds,workArea);
        WidgetPositioningService.CaptureAnchor(config,bounds,workArea);
    }
}
