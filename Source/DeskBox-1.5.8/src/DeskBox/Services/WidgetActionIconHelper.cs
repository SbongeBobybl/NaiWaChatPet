using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace DeskBox.Services;

public static class WidgetActionIconHelper
{
    public static void ApplyLockState(
        FrameworkElement positionRegularIcon,        FrameworkElement positionFilledIcon,        bool isPositionLocked,        FrameworkElement sizeRegularIcon,        FrameworkElement sizeFilledIcon,        bool isSizeLocked)
    {
        ApplyPairState(positionRegularIcon,positionFilledIcon,isPositionLocked);
        ApplyPairState(sizeRegularIcon,sizeFilledIcon,isSizeLocked);
    }

    public static void ApplyPairSize(FrameworkElement regularIcon,FrameworkElement filledIcon,WidgetTitleBarMetrics metrics)
    {
        WidgetTitleBarMetricsCalculator.ApplyActionIcon(regularIcon,metrics);
        WidgetTitleBarMetricsCalculator.ApplyActionIcon(filledIcon,metrics);
    }

    private static void ApplyPairState(FrameworkElement regularIcon,FrameworkElement filledIcon,bool isFilled)
    {
        regularIcon.Visibility = isFilled ? Visibility.Collapsed : Visibility.Visible;
        filledIcon.Visibility = isFilled ? Visibility.Visible : Visibility.Collapsed;
    }
}
