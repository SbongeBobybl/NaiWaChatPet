using DeskBox.Models;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Media;

namespace DeskBox.Views;

public sealed partial class ContentWidgetWindow
{
    private bool NaiWaDarkTheme => SettingsService.Settings.NaiWaWidgetTheme == "dark";

    private Windows.UI.Color NaiWaSurfaceColor => NaiWaDarkTheme
        ? Windows.UI.Color.FromArgb(255,58,52,40)
        : Windows.UI.Color.FromArgb(255,255,243,199);

    private void ApplyNaiWaTheme()
    {
        if (_config.WidgetKind != WidgetKind.File) return;
        RootGrid.RequestedTheme = NaiWaDarkTheme ? ElementTheme.Dark : ElementTheme.Light;
        ContentWidgetShell.RequestedTheme = NaiWaDarkTheme ? ElementTheme.Dark : ElementTheme.Light;
        // The shell's local text brushes were authored for a dark plate; they
        // must be regenerated for the warm light surface or labels disappear.
        ApplyWidgetForegroundAppearance();
        ContentWidgetShell.TitleBar.Background = new SolidColorBrush(NaiWaSurfaceColor);
        ContentWidgetShell.TitleBar.CornerRadius = new CornerRadius(12);
    }
}
