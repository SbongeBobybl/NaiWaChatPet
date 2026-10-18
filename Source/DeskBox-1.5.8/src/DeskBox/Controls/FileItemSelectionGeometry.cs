using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Media;

namespace DeskBox.Controls;

/// <summary>
/// Shared, allocation-free geometry and hit-test helpers for item selection.
/// The two hosts still own selection state and container synchronization.
/// </summary>
public static class FileItemSelectionGeometry
{
    public static Windows.Foundation.Rect GetSelectionRect(
        Windows.Foundation.Point start,        Windows.Foundation.Point end)
    {
        return new Windows.Foundation.Rect(
            Math.Min(start.X,end.X),            Math.Min(start.Y,end.Y),            Math.Abs(end.X - start.X),            Math.Abs(end.Y - start.Y));
    }

    public static double GetDragDistance(
        Windows.Foundation.Point start,        Windows.Foundation.Point end)
    {
        double x = end.X - start.X;
        double y = end.Y - start.Y;
        return Math.Sqrt((x * x) + (y * y));
    }

    public static bool Intersects(
        Windows.Foundation.Rect first,        Windows.Foundation.Rect second)
    {
        return first.X < second.X + second.Width &&
               first.X + first.Width > second.X &&
               first.Y < second.Y + second.Height &&
               first.Y + first.Height > second.Y;
    }

    public static bool HasAncestor<T>(DependencyObject source)
        where T : DependencyObject
    {
        DependencyObject? current = source;
        while (current is not null)
        {
            if (current is T)
            {
                return true;
            }

            current = VisualTreeHelper.GetParent(current);
        }

        return false;
    }

    public static bool IsWithinItemSurface(DependencyObject source)
    {
        DependencyObject? current = source;
        while (current is not null)
        {
            if (current is FrameworkElement element &&
                element.Tag as string is "InteractiveSurface" or "StackSurface")
            {
                return true;
            }

            current = VisualTreeHelper.GetParent(current);
        }

        return false;
    }
}
