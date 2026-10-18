using System.Collections.Concurrent;
using DeskBox.Helpers;
using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

/// <summary>
/// The startup storm guard defers the watcher's Start. Settings saves during
/// that window must not arm the watcher behind the guard's back, and the
/// deferred Start must still organize normally once it runs (feedback #576).
/// </summary>
public sealed class DesktopAutoOrganizationWatcherDeferredStartTests : IDisposable
{
    private readonly string _root =
        Path.Combine(Path.GetTempPath(), "DeskBox.Tests", Guid.NewGuid().ToString("N"));

    [Fact]
    public async Task SettingsToggleBeforeStart_DoesNotOrganize_StartStillWorks()
    {
        string desktopPath = Directory.CreateDirectory(Path.Combine(_root, "desktop")).FullName;
        string targetStorage = Directory.CreateDirectory(Path.Combine(_root, "docs-widget")).FullName;
        var settingsService = new SettingsService(Path.Combine(_root, "settings"));
        settingsService.Settings.DesktopAutoOrganizationEnabled = true;
        settingsService.Settings.DesktopOrganization.DesktopAutoOrganizationDelaySeconds = 10;
        settingsService.Settings.Widgets.Add(new WidgetConfig
        {
            Id = "docs-widget",
            Name = "Docs",
            WidgetKind = WidgetKind.File,
            MappedFolderPath = targetStorage
        });
        settingsService.Settings.DesktopOrganizationRules.Add(new DesktopOrganizationRule
        {
            TargetWidgetId = "docs-widget",
            IsEnabled = true,
            Extensions = [".txt"]
        });
        var clock = new ManualClock(new DateTimeOffset(2020, 1, 1, 0, 0, 0, TimeSpan.Zero));
        var fileService = new FileService();
        var organizer = TestOrganizerServices.Create(settingsService, fileService, () => desktopPath);
        var widgetManager = new WidgetManager(
            settingsService,
            fileService,
            organizer,
            new ThemeService(settingsService),
            new QuickCaptureService(new QuickCaptureStore(Path.Combine(_root, "quick-capture"))),
            () => desktopPath,
            recycleManagedFolderDeletes: false);

        using var watcher = new DesktopAutoOrganizationWatcher(
            settingsService,
            organizer,
            widgetManager,
            () => desktopPath,
            () => clock.Now,
            (delay, cancellationToken) => Task.Delay(TimeSpan.FromMilliseconds(50), cancellationToken));

        // Deferred-start window: a settings save would run the enable branch
        // and arm the FileSystemWatcher without the pre-start guard.
        settingsService.Settings.DesktopAutoOrganizationEnabled = true;
        await settingsService.SaveAsync();
        string file = Path.Combine(desktopPath, "deferred.txt");
        File.WriteAllText(file, "content");
        await Task.Delay(500);
        clock.Advance(TimeSpan.FromSeconds(40));
        await Task.Delay(500);
        Assert.True(File.Exists(file),
            "a settings save before Start must not activate organization");

        // The deferred Start adopts the current setting and organizes.
        watcher.Start();
        await Task.Delay(500);
        clock.Advance(TimeSpan.FromSeconds(30));
        await WaitForAsync(() => !File.Exists(file));
        Assert.True(File.Exists(Path.Combine(targetStorage, "deferred.txt")));
    }

    private static async Task WaitForAsync(Func<bool> condition)
    {
        DateTime deadline = DateTime.UtcNow + TimeSpan.FromSeconds(30);
        while (DateTime.UtcNow < deadline)
        {
            if (condition())
            {
                return;
            }

            await Task.Delay(20);
        }

        Assert.True(condition(), "Condition was not met before the timeout.");
    }

    public void Dispose()
    {
        if (Directory.Exists(_root))
        {
            Directory.Delete(_root, recursive: true);
        }
    }

    private sealed class ManualClock
    {
        private readonly object _gate = new();
        private DateTimeOffset _now;

        public ManualClock(DateTimeOffset start)
        {
            _now = start;
        }

        public DateTimeOffset Now
        {
            get
            {
                lock (_gate)
                {
                    return _now;
                }
            }
        }

        public void Advance(TimeSpan delta)
        {
            lock (_gate)
            {
                _now += delta;
            }
        }
    }
}
