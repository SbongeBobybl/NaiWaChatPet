using DeskBox.Models;

namespace DeskBox.Services;

internal enum InitialFileWidgetSetupDecision
{
    None,
    DeferUntilInteractiveLaunch,
    ResolveExistingConfiguration,
    CreateDefaultWidget
}

internal readonly record struct InitialFileWidgetSetupSnapshot(
    bool IsInteractiveLaunch,    SettingsLoadRecoveryState SettingsLoadState,    bool HasResolvedSetup,    bool HasConfiguredFileWidget);

internal static class InitialFileWidgetSetupPolicy
{
    public static InitialFileWidgetSetupDecision Evaluate(
        InitialFileWidgetSetupSnapshot snapshot)
    {
        if (snapshot.HasResolvedSetup ||
            snapshot.SettingsLoadState == SettingsLoadRecoveryState.DefaultsAfterFailure)
        {
            return InitialFileWidgetSetupDecision.None;
        }

        if (!snapshot.IsInteractiveLaunch)
        {
            return InitialFileWidgetSetupDecision.DeferUntilInteractiveLaunch;
        }

        return snapshot.HasConfiguredFileWidget
            ? InitialFileWidgetSetupDecision.ResolveExistingConfiguration
            : InitialFileWidgetSetupDecision.CreateDefaultWidget;
    }

    public static bool HasConfiguredFileWidget(AppSettings settings)
    {
        return settings.Widgets.Any(widget =>
            widget.WidgetKind == WidgetKind.File &&
            !settings.DeletedWidgetIds.Contains(widget.Id));
    }
}
