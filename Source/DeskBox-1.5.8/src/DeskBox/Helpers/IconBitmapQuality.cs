namespace DeskBox.Helpers;

/// <summary>
/// Shared heuristics for identifying a Shell icon that is technically valid but
/// is only a small glyph centered inside a much larger transparent canvas.
/// This is intentionally conservative: it is used for Shell-item icons, never
/// media thumbnails, and must not reject ordinary application artwork.
/// </summary>
internal static class IconBitmapQuality
{
    private const int MinimumCanvasDimension = 96;
    private const double MaximumVisibleDimensionRatio = 0.35;
    private const int MinimumSignificantAlpha = 32;

    /// <summary>
    /// Alpha at which a pixel counts as artwork rather than as paint the Shell
    /// adds when it fills a canvas it has no artwork for. Measured Shell
    /// payloads place that near-invisible border between alpha 32 and 95 while
    /// the real glyph sits above 240, so half of the peak alpha separates them
    /// without trimming the glyph's antialiased edge.
    /// </summary>
    internal static byte SignificantAlphaThreshold(byte peakAlpha) =>
        (byte)Math.Max(MinimumSignificantAlpha,peakAlpha / 2);

    internal static bool IsLikelyPadded(
        int width,        int height,        int visibleWidth,        int visibleHeight)
    {
        if (width < MinimumCanvasDimension ||
            height < MinimumCanvasDimension ||
            visibleWidth <= 0 ||
            visibleHeight <= 0)
        {
            return false;
        }

        // A genuinely tiny visible rectangle in both dimensions is the common
        // signature of a 16/32/48 px Shell icon scaled into a Jumbo canvas.
        return visibleWidth <= width * MaximumVisibleDimensionRatio &&
               visibleHeight <= height * MaximumVisibleDimensionRatio;
    }
}
