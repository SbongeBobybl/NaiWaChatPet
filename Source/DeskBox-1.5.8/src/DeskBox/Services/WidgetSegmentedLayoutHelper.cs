using CommunityToolkit.WinUI.Controls;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Xaml;

namespace DeskBox.Services;

public static class WidgetSegmentedLayoutHelper
{
    // EqualPanel subtracts its inter-item spacing before dividing the final
    // arrange width. A transient near-zero host width can therefore produce a
    // negative child width inside the Toolkit control. Keep the Segmented
    // surface wider than its own chrome even when its parent is momentarily
    // compressed during a content or display-topology transition.
    public const double MinimumSafeWidth = 96;

    // Retained so callers can initialize all widget presentation helpers in
    // one place. Segmented width changes are now fully owned by Toolkit's
    // EqualPanel, so no dispatcher follow-up is necessary.
    public static void Initialize(DispatcherQueue dispatcher) => _ = dispatcher;

    public static void ApplyNaturalItemWidths(Segmented segmented)
    {
        segmented.MinWidth = MinimumSafeWidth;
        foreach (SegmentedItem item in segmented.Items.OfType<SegmentedItem>())
        {
            ResetToolkitManagedWidth(item);
            item.ClearValue(Microsoft.UI.Xaml.Controls.Control.PaddingProperty);
            item.ClearValue(FrameworkElement.MinHeightProperty);
        }
    }

    public static void ApplyEqualItemWidths(Segmented segmented)
    {
        segmented.MinWidth = MinimumSafeWidth;
        SegmentedItem[] items = segmented.Items
            .OfType<SegmentedItem>()
            .ToArray();
        double minHeight = Math.Max(24,segmented.MinHeight - 3);
        var padding = new Thickness(4,1,4,2);

        // Segmented is built on CommunityToolkit's EqualPanel. Assigning a
        // fixed width here is unsafe: a master/detail switch can shrink the
        // parent before this helper receives its next SizeChanged event, and
        // EqualPanel then attempts to arrange those stale widths in a smaller
        // rectangle. Let the panel calculate equal widths from its current
        // final size on every arrange pass instead.
        foreach (SegmentedItem item in items)
        {
            ResetToolkitManagedWidth(item);
            if (item.Visibility == Visibility.Visible)
            {
                item.Padding = padding;
                item.MinHeight = minHeight;
            }
        }
    }

    private static void ResetToolkitManagedWidth(SegmentedItem item)
    {
        item.Width = double.NaN;
        item.MaxWidth = double.PositiveInfinity;
        item.MinWidth = 0;
    }
}
