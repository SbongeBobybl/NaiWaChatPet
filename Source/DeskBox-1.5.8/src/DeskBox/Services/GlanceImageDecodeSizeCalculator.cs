namespace DeskBox.Services;

internal static class GlanceImageDecodeSizeCalculator
{
    // The floor only has to cover the smallest meaningful glance surface with
    // its shrink-refresh headroom (the default 360x240 presentation needs
    // ~546 physical pixels). NeedsRefresh's 0.8 hysteresis already prevents
    // decode churn near the floor, so the historical 720 floor paid ~2x on
    // every small widget without buying stability.
    internal const int MinimumDecodePixelWidth = 480;
    internal const int MaximumDecodePixelWidth = 2560;

    private const double DefaultLogicalWidth = 360;
    private const double DefaultLogicalHeight = 240;
    private const double ShrinkRefreshRatio = 0.8;

    // The shrink-refresh cycle displays at up to 1 / ShrinkRefreshRatio of
    // the settled size, so decoding at exactly that headroom covers it. The
    // historical blanket 2x supersample paid 4x the memory for headroom
    // nothing else consumed (issue tracked in the 2026-10-03 memory audit).
    private const double SupersamplingFactor = 1.0 / ShrinkRefreshRatio;

    // The compact capsule background re-decodes only when the capsule width
    // actually changes, so it needs the same modest headroom but no
    // shrink-refresh hysteresis of its own.
    internal const double CompactBackgroundSupersamplingFactor = 1.0 / ShrinkRefreshRatio;

    internal static int Calculate(
        double logicalWidth,        double logicalHeight,        double rasterizationScale)
    {
        double width = NormalizeDimension(logicalWidth,DefaultLogicalWidth);
        double height = NormalizeDimension(logicalHeight,DefaultLogicalHeight);
        double scale = double.IsFinite(rasterizationScale) && rasterizationScale > 0
            ? rasterizationScale
            : 1;
        scale = Math.Clamp(scale,1,4);

        double physicalDiagonal = Math.Sqrt((width * width) + (height * height)) * scale;
        double requestedWidth = Math.Ceiling(physicalDiagonal * SupersamplingFactor);
        if (requestedWidth <= MinimumDecodePixelWidth)
        {
            return MinimumDecodePixelWidth;
        }

        if (requestedWidth >= MaximumDecodePixelWidth)
        {
            return MaximumDecodePixelWidth;
        }

        return (int)requestedWidth;
    }

    internal static bool NeedsRefresh(int currentDecodePixelWidth,int requiredDecodePixelWidth)
    {
        if (requiredDecodePixelWidth <= 0)
        {
            return false;
        }

        if (currentDecodePixelWidth <= 0 || requiredDecodePixelWidth > currentDecodePixelWidth)
        {
            return true;
        }

        return requiredDecodePixelWidth <= Math.Floor(currentDecodePixelWidth * ShrinkRefreshRatio);
    }

    private static double NormalizeDimension(double value,double fallback)
    {
        return double.IsFinite(value) && value > 0 ? value : fallback;
    }
}
