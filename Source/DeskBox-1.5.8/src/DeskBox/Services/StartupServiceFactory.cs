namespace DeskBox.Services;

public static class StartupServiceFactory
{
    public static IStartupService Create(AppDistributionService distribution,SettingsService? settingsService = null)
    {
        return distribution.IsMicrosoftStore
            ? new StoreStartupService()
            : new DirectStartupService(settingsService);
    }
}
