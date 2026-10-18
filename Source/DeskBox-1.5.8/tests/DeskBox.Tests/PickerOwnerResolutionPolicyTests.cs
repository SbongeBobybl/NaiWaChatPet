using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class PickerOwnerResolutionPolicyTests
{
    [Fact]
    public void Resolve_prefersWindowsThatAreProvablyOnTheActiveDesktop()
    {
        Assert.Equal(
            PickerOwnerSource.TrayWindow,
            PickerOwnerResolutionPolicy.Resolve(
                trayWindowUsable: true,
                settingsWindowUsable: false,
                cachedHelperWindowUsable: false));
        Assert.Equal(
            PickerOwnerSource.TrayWindow,
            PickerOwnerResolutionPolicy.Resolve(
                trayWindowUsable: true,
                settingsWindowUsable: true,
                cachedHelperWindowUsable: true));

        Assert.Equal(
            PickerOwnerSource.SettingsWindow,
            PickerOwnerResolutionPolicy.Resolve(
                trayWindowUsable: false,
                settingsWindowUsable: true,
                cachedHelperWindowUsable: true));

        Assert.Equal(
            PickerOwnerSource.CachedHelperWindow,
            PickerOwnerResolutionPolicy.Resolve(
                trayWindowUsable: false,
                settingsWindowUsable: false,
                cachedHelperWindowUsable: true));

        Assert.Equal(
            PickerOwnerSource.FreshHelperWindow,
            PickerOwnerResolutionPolicy.Resolve(
                trayWindowUsable: false,
                settingsWindowUsable: false,
                cachedHelperWindowUsable: false));
    }
}
