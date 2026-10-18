using DeskBox.Helpers;
using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class ManagedStorageDesktopShortcutServiceTests : IDisposable
{
    private readonly string _root = Path.Combine(
        Path.GetTempPath(),
        "DeskBoxShortcutTests",
        Guid.NewGuid().ToString("N"));

    [Fact]
    public void AppSettings_DesktopStorageShortcutDefaultsOff()
    {
        var settings = new AppSettings();

        Assert.False(settings.ManagedStorageDesktopShortcutEnabled);
        Assert.Equal(string.Empty, settings.ManagedStorageDesktopShortcutPath);
    }

    [Fact]
    public void GetAvailableShortcutPath_DoesNotOverwriteAnExistingName()
    {
        Directory.CreateDirectory(_root);
        string existing = Path.Combine(
            _root,
            ManagedStorageDesktopShortcutService.ShortcutFileName);
        File.WriteAllText(existing, "belongs to the user");

        string candidate =
            ManagedStorageDesktopShortcutService.GetAvailableShortcutPath(_root);

        Assert.Equal(Path.Combine(_root, "DeskBox Files (2).lnk"), candidate);
        Assert.Equal("belongs to the user", File.ReadAllText(existing));
    }

    [Fact]
    public async Task SyncAsync_DoesNotCreateWithoutAnExplicitUserAction()
    {
        string dataDirectory = Path.Combine(_root, "data");
        string desktopDirectory = Path.Combine(_root, "desktop");
        string storageRoot = Path.Combine(_root, "storage");
        Directory.CreateDirectory(desktopDirectory);
        Directory.CreateDirectory(storageRoot);
        File.WriteAllText(Path.Combine(storageRoot, "kept.txt"), "one");

        var settingsService = new SettingsService(dataDirectory);
        settingsService.Settings.DefaultManagedStorageRootPath = storageRoot;
        settingsService.Settings.ManagedStorageDesktopShortcutEnabled = true;
        var service = new ManagedStorageDesktopShortcutService(
            settingsService,
            desktopDirectory);

        await service.SyncAsync();

        Assert.Empty(Directory.EnumerateFileSystemEntries(desktopDirectory));
        Assert.False(settingsService.Settings.ManagedStorageDesktopShortcutEnabled);
        Assert.Equal(string.Empty, settingsService.Settings.ManagedStorageDesktopShortcutPath);
    }

    [Fact]
    public async Task ExplicitShortcut_CreatesRetargetsHonorsManualDeletionAndRemoves()
    {
        string dataDirectory = Path.Combine(_root, "data");
        string desktopDirectory = Path.Combine(_root, "desktop");
        string firstRoot = Path.Combine(_root, "storage-one");
        string secondRoot = Path.Combine(_root, "storage-two");
        Directory.CreateDirectory(desktopDirectory);
        Directory.CreateDirectory(firstRoot);
        Directory.CreateDirectory(secondRoot);
        File.WriteAllText(Path.Combine(firstRoot, "kept.txt"), "one");
        File.WriteAllText(Path.Combine(secondRoot, "kept.txt"), "two");

        var settingsService = new SettingsService(dataDirectory);
        settingsService.Settings.DefaultManagedStorageRootPath = firstRoot;
        var service = new ManagedStorageDesktopShortcutService(
            settingsService,
            desktopDirectory);

        Assert.True(await service.CreateAsync());

        string shortcutPath = settingsService.Settings.ManagedStorageDesktopShortcutPath;
        Assert.True(settingsService.Settings.ManagedStorageDesktopShortcutEnabled);
        Assert.True(service.HasShortcut());
        Assert.True(File.Exists(shortcutPath));
        Assert.Equal(
            Path.GetFullPath(firstRoot),
            Path.GetFullPath(ShortcutHelper.ReadStoredMetadata(shortcutPath)!.TargetPath));

        settingsService.Settings.DefaultManagedStorageRootPath = secondRoot;
        await service.SyncAsync(firstRoot);

        Assert.Equal(shortcutPath, settingsService.Settings.ManagedStorageDesktopShortcutPath);
        Assert.Equal(
            Path.GetFullPath(secondRoot),
            Path.GetFullPath(ShortcutHelper.ReadStoredMetadata(shortcutPath)!.TargetPath));

        File.Delete(shortcutPath);
        await service.SyncAsync();

        Assert.False(File.Exists(shortcutPath));
        Assert.False(service.HasShortcut());
        Assert.False(settingsService.Settings.ManagedStorageDesktopShortcutEnabled);
        Assert.Equal(string.Empty, settingsService.Settings.ManagedStorageDesktopShortcutPath);

        Assert.True(await service.CreateAsync());
        string recreatedShortcutPath =
            settingsService.Settings.ManagedStorageDesktopShortcutPath;
        Assert.True(File.Exists(recreatedShortcutPath));
        Assert.True(await service.RemoveAsync());
        Assert.False(File.Exists(recreatedShortcutPath));
        Assert.False(settingsService.Settings.ManagedStorageDesktopShortcutEnabled);
        Assert.Equal(string.Empty, settingsService.Settings.ManagedStorageDesktopShortcutPath);
    }

    public void Dispose()
    {
        if (Directory.Exists(_root))
        {
            Directory.Delete(_root, recursive: true);
        }
    }
}
