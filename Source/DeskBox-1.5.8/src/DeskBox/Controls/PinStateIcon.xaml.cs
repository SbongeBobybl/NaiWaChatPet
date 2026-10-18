using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace DeskBox.Controls;

public sealed partial class PinStateIcon : UserControl
{
    public static readonly DependencyProperty IsPinnedProperty =
        DependencyProperty.Register(
            nameof(IsPinned),            typeof(bool),            typeof(PinStateIcon),            new PropertyMetadata(false,OnIsPinnedChanged));

    public PinStateIcon()
    {
        InitializeComponent();
        UpdatePinState();
    }

    public bool IsPinned
    {
        get => (bool)GetValue(IsPinnedProperty);
        set => SetValue(IsPinnedProperty,value);
    }

    private static void OnIsPinnedChanged(
        DependencyObject sender,        DependencyPropertyChangedEventArgs args) =>
        ((PinStateIcon)sender).UpdatePinState();

    private void UpdatePinState()
    {
        if (PinnedPath is null || UnpinnedPath is null)
        {
            return;
        }

        PinnedPath.Visibility = IsPinned ? Visibility.Visible : Visibility.Collapsed;
        UnpinnedPath.Visibility = IsPinned ? Visibility.Collapsed : Visibility.Visible;
    }
}
