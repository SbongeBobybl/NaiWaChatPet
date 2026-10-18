using DeskBox.Models;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace DeskBox.Services;

internal static class WidgetChromeMenuBuilder
{
    public static MenuFlyoutSubItem Create(
        WidgetConfig config,        WidgetContentDescriptor descriptor,        LocalizationService localizationService,        WidgetManager? widgetManager,        Action<WidgetChromeMode> applyMode)
    {
        bool grouped = widgetManager?.IsWidgetGrouped(config.Id) == true;
        WidgetChromeMode selectedMode = grouped
            ? widgetManager!.GetWidgetGroupChromeMode(config.Id) ??
              WidgetChromeMode.Standard
            : WidgetChromeModeNames.GetOverrideMode(config);
        var subItem = new MenuFlyoutSubItem
        {
            Text = localizationService.T("Widget.ChromeMode.Title"),
            Icon = new FontIcon { Glyph = "\uE771" }
        };

        foreach (var mode in new[]
                 {
                     WidgetChromeMode.System,                     WidgetChromeMode.Standard,                     WidgetChromeMode.Compact,                     WidgetChromeMode.Overlay,                     WidgetChromeMode.Hidden
                 })
        {
            if (!grouped &&
                mode == WidgetChromeMode.Hidden &&
                !descriptor.CanHideChrome)
            {
                continue;
            }

            if (!grouped &&
                mode == WidgetChromeMode.Overlay &&
                !descriptor.CanUseOverlayChrome)
            {
                continue;
            }

            bool isEnabled =
                !grouped ||
                WidgetGroupChromePolicy.IsSupportedGroupMode(mode);
            string text = localizationService.T(GetTextKey(mode));
            if (!isEnabled)
            {
                text += $" · {localizationService.T("Widget.Group.ChromeLocked")}";
            }

            var item = new ToggleMenuFlyoutItem
            {
                Text = text,
                IsChecked = selectedMode == mode,
                IsEnabled = isEnabled
            };
            if (!isEnabled)
            {
                ToolTipService.SetToolTip(
                    item,                    localizationService.T("Widget.Group.ChromeLocked"));
            }
            item.Click += (_,_) => applyMode(mode);
            subItem.Items.Add(item);
        }

        subItem.Items.Add(new MenuFlyoutSeparator());
        subItem.Items.Add(CreateTitleButtonsSubItem(
            localizationService,            App.Current.SettingsService));

        return subItem;
    }

    private static MenuFlyoutSubItem CreateTitleButtonsSubItem(
        LocalizationService localizationService,        SettingsService settingsService)
    {
        var subItem = new MenuFlyoutSubItem
        {
            Text = localizationService.T("Widget.TitleButtons.Title"),
            Icon = new FontIcon { Glyph = "\uE7C9" }
        };
        var items = new List<(string Action, ToggleMenuFlyoutItem Item)>();

        foreach (string action in SettingsService.SupportedWidgetHoverButtonActions)
        {
            var item = new ToggleMenuFlyoutItem
            {
                Text = localizationService.T(GetTitleButtonTextKey(action))
            };
            items.Add((action, item));
            item.Click += (_,_) =>
            {
                if (!SettingsService.TryUpdateWidgetHoverButtonAction(
                        settingsService.Settings.WidgetHoverButtonActions,                        action,                        item.IsChecked,                        out string updatedValue))
                {
                    RefreshTitleButtonItems(items,settingsService.Settings.WidgetHoverButtonActions);
                    return;
                }

                settingsService.Settings.ShowHoverButtons = true;
                settingsService.Settings.WidgetHoverButtonActions = updatedValue;
                RefreshTitleButtonItems(items,updatedValue);
                settingsService.SaveDebounced();
            };
            subItem.Items.Add(item);
        }

        RefreshTitleButtonItems(items,settingsService.Settings.WidgetHoverButtonActions);
        return subItem;
    }

    private static void RefreshTitleButtonItems(
        IReadOnlyList<(string Action, ToggleMenuFlyoutItem Item)> items,        string? value)
    {
        var selected = SettingsService.ParseWidgetHoverButtonActions(value);
        foreach (var (action, item) in items)
        {
            item.IsChecked = selected.Contains(action,StringComparer.Ordinal);
            item.IsEnabled = SettingsService.CanToggleWidgetHoverButtonAction(value,action);
        }
    }

    private static string GetTitleButtonTextKey(string action)
    {
        return action switch
        {
            SettingsService.WidgetHoverActionLockPosition => "Settings.HoverButtonActions.LockPosition",            SettingsService.WidgetHoverActionLockSize => "Settings.HoverButtonActions.LockSize",            SettingsService.WidgetHoverActionAdd => "Settings.HoverButtonActions.Add",            SettingsService.WidgetHoverActionDelete => "Settings.HoverButtonActions.Delete",            _ => "Settings.HoverButtonActions.More"
        };
    }

    private static string GetTextKey(WidgetChromeMode mode)
    {
        return mode switch
        {
            WidgetChromeMode.System => "Widget.ChromeMode.System",            WidgetChromeMode.Compact => "Widget.ChromeMode.Compact",            WidgetChromeMode.Overlay => "Widget.ChromeMode.Overlay",            WidgetChromeMode.Hidden => "Widget.ChromeMode.Hidden",            _ => "Widget.ChromeMode.Standard"
        };
    }
}
