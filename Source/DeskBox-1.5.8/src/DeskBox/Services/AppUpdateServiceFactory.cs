namespace DeskBox.Services;

public static class AppUpdateServiceFactory
{
    public static IAppUpdateService Create(AppDistributionService distribution)
    {
        return distribution.IsMicrosoftStore
            ? new StoreAppUpdateService()
            : new AppUpdateService();
    }
}
