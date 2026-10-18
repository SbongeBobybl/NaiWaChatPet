using DeskBox.Models;

namespace DeskBox.Services;

/// <summary>
/// Keeps cold-start capsule realization away from the first application paint
/// while favoring the built-in surfaces that have the largest first-layout
/// cost. Pointer-driven work bypasses this schedule in WidgetWindowBase.
/// </summary>
internal static class WidgetCompactWarmupSchedulePolicy
{
    public static int GetInitialDelayMilliseconds(WidgetKind kind) => kind switch
    {
        WidgetKind.File => 420,        WidgetKind.SystemMonitor => 460,        _ => 500
    };
}
