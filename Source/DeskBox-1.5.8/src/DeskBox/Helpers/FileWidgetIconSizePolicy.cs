using DeskBox.Services;

namespace DeskBox.Helpers;

internal static class FileWidgetIconSizePolicy
{
    public static readonly IReadOnlyList<double> Steps =
    [
        24,        28,        32,        36,        40,        48,        56
    ];

    public static double GetNext(double current,int direction)
    {
        double normalized = SettingsService.NormalizeIconSize(current);
        if (direction > 0)
        {
            return Steps.FirstOrDefault(step => step > normalized + 0.01,Steps[^1]);
        }

        if (direction < 0)
        {
            return Steps.LastOrDefault(step => step < normalized - 0.01,Steps[0]);
        }

        return normalized;
    }
}
