namespace DeskBox.Models;

/// <summary>
/// Retired Todo reminder offset table, kept as a data contract only.
///
/// The reminder service, notification router and recurrence scheduler are
/// deleted; this table survives because the Todo store normalizes
/// <c>ReminderOffsetMinutes</c> while reading a historical document.
/// </summary>
public static class TodoReminderOptions
{
    public const int ReminderOff = -1;

    public static readonly int[] SupportedOffsetMinutes =
    [
        0,        5,        10,        15,        30,        60,        1440
    ];

    public static int? NormalizeOffsetMinutes(int? minutes)
    {
        if (minutes is null)
        {
            return null;
        }

        return minutes.Value == ReminderOff ||
               SupportedOffsetMinutes.Contains(minutes.Value)
            ? minutes.Value
            : null;
    }

    public static bool IsReminderOff(int? minutes)
    {
        return minutes == ReminderOff;
    }
}
