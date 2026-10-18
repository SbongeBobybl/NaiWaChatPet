namespace DeskBox.Services;

internal static class GlanceCalendarNavigationResolver
{
    public static DateOnly ResolveWheelTarget(
        DateOnly currentMonth,        int wheelDelta,        DateOnly minimumMonth,        DateOnly maximumMonth)
    {
        DateOnly normalizedCurrent = new(currentMonth.Year,currentMonth.Month,1);
        DateOnly normalizedMinimum = new(minimumMonth.Year,minimumMonth.Month,1);
        DateOnly normalizedMaximum = new(maximumMonth.Year,maximumMonth.Month,1);
        if (wheelDelta == 0)
        {
            return normalizedCurrent;
        }

        DateOnly target = normalizedCurrent.AddMonths(wheelDelta > 0 ? -1 : 1);
        if (target < normalizedMinimum)
        {
            return normalizedMinimum;
        }

        return target > normalizedMaximum ? normalizedMaximum : target;
    }

    public static DateOnly ResolveDisplayedMonth(
        IEnumerable<DateOnly> visibleDates,        DateOnly fallbackMonth)
    {
        ArgumentNullException.ThrowIfNull(visibleDates);

        DateOnly normalizedFallback = new(
            fallbackMonth.Year,            fallbackMonth.Month,            1);
        return visibleDates
            .GroupBy(date => new { date.Year, date.Month })
            .Select(group => new
            {
                Month = new DateOnly(group.Key.Year,group.Key.Month,1),
                Count = group.Count()
            })
            .OrderByDescending(candidate => candidate.Count)
            .ThenBy(candidate => MonthDistance(candidate.Month,normalizedFallback))
            .Select(candidate => candidate.Month)
            .FirstOrDefault(normalizedFallback);
    }

    private static int MonthDistance(DateOnly left,DateOnly right) =>
        Math.Abs(((left.Year - right.Year) * 12) + left.Month - right.Month);
}
