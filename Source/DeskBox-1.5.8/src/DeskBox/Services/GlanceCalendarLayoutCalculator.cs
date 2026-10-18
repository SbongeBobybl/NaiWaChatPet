namespace DeskBox.Services;

internal static class GlanceCalendarLayoutCalculator
{
    private const double CompactCalendarThreshold = 320;

    public static bool IsCompact(double availableHeight) =>
        availableHeight < CompactCalendarThreshold;

    public static double CalculatePanelHeight(
        double availableHeight,        bool isCompact,        bool hasTraditionalCalendar)
    {
        _ = hasTraditionalCalendar;

        if (isCompact)
        {
            // Compact calendars keep the clock inside the material surface.
            // Let the surface use almost all available height so the six native
            // week rows are not squeezed after reserving that compact header.
            return Math.Clamp(availableHeight - 40,238,268);
        }

        // Above the compact breakpoint the clock sits outside the material
        // surface. Grow continuously with the widget instead of pinning the
        // calendar to a short fixed card, while retaining room for that clock.
        return Math.Clamp(availableHeight - 102,244,310);
    }

    public static double CalculateDayHeight(
        double panelHeight,        bool isCompact,        bool hasTraditionalCalendar)
    {
        _ = hasTraditionalCalendar;

        // CalendarView owns the header and weekday metrics. Reserve those fixed
        // rows (plus the compact time strip) and divide the rest into six weeks.
        double fixedContentHeight = isCompact ? 104 : 58;
        return Math.Clamp(
            (panelHeight - fixedContentHeight) / 6,            24,            42);
    }

    public static bool ShouldShowTraditionalDetails(
        double panelWidth,        double dayHeight,        bool isCompact,        bool hasTraditionalCalendar) =>
        hasTraditionalCalendar &&
        !isCompact &&
        panelWidth >= 280 &&
        dayHeight >= 28;
}
