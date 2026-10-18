using DeskBox.Controls;

namespace DeskBox.Views;

public sealed partial class ContentWidgetWindow
{
    protected override void OnCompactVisualStateChanged(bool collapsed)
    {
        if (collapsed)
        {
            App.Current?.WidgetManager?.CancelWidgetSurfaceSwitch(_config.Id);
        }
    }

    internal void SetGroupMemberLoading(string? widgetId,bool isLoading)
    {
        ContentWidgetShell.SetGroupMemberLoading(widgetId,isLoading);
    }
}
