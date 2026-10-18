using DeskBox.Contracts;
using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

/// <summary>
/// Retirement migration contract for the six removed DeskBox features
/// (待办 Todo, 随记 QuickCapture, 搜索 Search, 天气 Weather, 音乐 Music, 时光 Glance).
///
/// Three separate invariants are pinned here, because the retirement is only
/// safe when all three hold at once:
///   1. a retired feature can never activate again,
///   2. a retired widget record is never rewritten into a File widget (the old
///      Weather record that surfaced as an empty file box was exactly this
///      bug), and
///   3. the user's stored preference values survive untouched, so a later
///      build or a manual revert still finds the original data.
/// </summary>
public sealed class NaiWaRetiredFeatureMigrationTests
{
    private static readonly WidgetKind[] RetiredKinds =
    [
        WidgetKind.Todo,
        WidgetKind.QuickCapture,
        WidgetKind.Search,
        WidgetKind.Weather,
        WidgetKind.Music,
        WidgetKind.Glance,
    ];

    [Fact]
    public void RetiredWidgetsStayInactiveWithoutErasingStoredPreferences()
    {
        var settings = new AppSettings();
        foreach (WidgetKind kind in RetiredKinds)
        {
            settings.FeatureWidgetEnabledStates[kind.ToString()] = true;
        }

        FeatureWidgetSettings.Normalize(settings);

        foreach (WidgetKind kind in RetiredKinds)
        {
            // The stored preference is preserved verbatim ...
            Assert.True(
                settings.FeatureWidgetEnabledStates[kind.ToString()],
                $"{kind} enabled-state was erased");
            // ... but the feature never reports itself as on, ...
            Assert.False(
                FeatureWidgetSettings.IsEnabled(settings, kind),
                $"{kind} reported itself enabled after retirement");
            // ... it is still a *known* kind (so normalizers leave it alone), ...
            Assert.True(
                WidgetRegistry.Default.IsKnown(kind),
                $"{kind} stopped being a known kind");
            // ... and it can never mint a window again.
            Assert.False(
                WidgetRegistry.Default.CanCreateWindow(kind),
                $"{kind} is still creatable");
            Assert.False(
                WidgetRegistry.Default.IsImplemented(kind),
                $"{kind} is still implemented");
        }

        // The organizer backbone is untouched.
        Assert.True(WidgetRegistry.Default.CanCreateWindow(WidgetKind.File));
        Assert.True(WidgetRegistry.Default.IsImplemented(WidgetKind.File));
    }

    [Fact]
    public void RetiredWidgetRecordsAreNotRewrittenIntoFileWidgets()
    {
        var settings = new AppSettings();
        string[] retiredIds = new string[RetiredKinds.Length];
        for (int index = 0; index < RetiredKinds.Length; index++)
        {
            var config = new WidgetConfig
            {
                Name = RetiredKinds[index].ToString() + " (historical)",
                WidgetKind = RetiredKinds[index],
                IsDisabled = true,
                IsVisible = false,
            };
            retiredIds[index] = config.Id;
            settings.Widgets.Add(config);
        }

        // A genuinely unknown kind is the one that may be downgraded: it can
        // never have had a window, and File is the only safe shell for it.
        var unknown = new WidgetConfig { Name = "future-kind", WidgetKind = (WidgetKind)4096 };
        settings.Widgets.Add(unknown);

        // Exercise the same normalizer pass the settings loader runs on load.
        SettingsService.NormalizeFeatureWidgetSettings(settings);
        SettingsService.NormalizeTodoSettings(settings);
        SettingsService.NormalizeWeatherSettings(settings);
        SettingsService.NormalizeWidgetContentSettings(settings);

        for (int index = 0; index < RetiredKinds.Length; index++)
        {
            WidgetConfig config = settings.Widgets.Single(w => w.Id == retiredIds[index]);
            Assert.Equal(RetiredKinds[index], config.WidgetKind);
            Assert.NotEqual(WidgetKind.File, config.WidgetKind);
        }

        Assert.Equal(WidgetKind.File, settings.Widgets.Single(w => w.Id == unknown.Id).WidgetKind);
    }

    [Fact]
    public async Task LoadingAHistoricalLayoutLeavesRetiredKindsAsTheirOwnKind()
    {
        // Regression for the "empty 天气 file box" defect: widget-layout.json
        // carried a Weather record, SettingsService's normalizer did not know
        // the kind, and the record was rewritten to WidgetKind.File — which
        // then opened as an empty organizer file box on every start.
        string root = Path.Combine(
            Path.GetTempPath(), "DeskBox.NaiWa.Retired", Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(root);

        var layoutWidgets = new List<string>();
        var expectations = new List<(string Id, WidgetKind Kind)>();
        foreach (WidgetKind kind in RetiredKinds)
        {
            string id = Guid.NewGuid().ToString("N");
            expectations.Add((id, kind));
            layoutWidgets.Add(
                "{\"id\":\"" + id + "\",\"name\":\"" + kind + "-historical\"," +
                "\"widgetKind\":\"" + kind + "\",\"isDisabled\":true,\"isVisible\":false}");
        }

        await File.WriteAllTextAsync(
            Path.Combine(root, "widget-layout.json"),
            "{\"schemaVersion\":1,\"layout\":{\"widgets\":[" +
            string.Join(",", layoutWidgets) + "]}}");
        await File.WriteAllTextAsync(
            Path.Combine(root, "settings.json"),
            """{"schemaVersion":1}""");

        var service = new SettingsService(root);
        await service.LoadAsync();

        foreach ((string id, WidgetKind kind) in expectations)
        {
            WidgetConfig? config = service.Settings.Widgets.FirstOrDefault(w => w.Id == id);
            Assert.NotNull(config);
            Assert.Equal(kind, config!.WidgetKind);
            Assert.NotEqual(WidgetKind.File, config.WidgetKind);
            // A retired record is never handed to a window on the next start.
            Assert.False(WidgetRegistry.Default.IsAvailableForSession(config, service.Settings));
        }
    }

    [Fact]
    public void RetiredWidgetsAreNeverAvailableForSessionRestore()
    {
        var settings = new AppSettings();
        foreach (WidgetKind kind in RetiredKinds)
        {
            // Even a record left marked visible+enabled by an older release
            // must not come back as a window on the next start.
            var config = new WidgetConfig
            {
                WidgetKind = kind,
                IsDisabled = false,
                IsVisible = true,
            };
            Assert.False(
                WidgetRegistry.Default.IsAvailableForSession(config, settings),
                $"{kind} was offered for session restore");
        }

        var fileWidget = new WidgetConfig
        {
            WidgetKind = WidgetKind.File,
            IsDisabled = false,
            IsVisible = true,
        };
        Assert.True(WidgetRegistry.Default.IsAvailableForSession(fileWidget, settings));
    }

    [Fact]
    public void RetiredFeatureSettingValuesStillRoundTripThroughJson()
    {
        // The retired features' settings members are the settings.json wire
        // contract: an older build (or a user reverting) must still read back
        // exactly what it wrote. Retirement must not silently drop the keys.
        var settings = new AppSettings();
        settings.QuickCaptureShowTabBar = false;
        settings.QuickCaptureRecentLimit = 77;
        settings.TodoShowCompletedTab = true;
        settings.TodoDefaultFilter = TodoOptionKinds.DefaultFilterCompleted;
        settings.TodoDefaultReminderOffsetMinutes = 30;
        settings.WeatherCityName = "Fixture City";
        settings.EnableGlanceImageAutoRotation = false;

        string json = System.Text.Json.JsonSerializer.Serialize(settings);
        AppSettings reloaded = System.Text.Json.JsonSerializer.Deserialize<AppSettings>(json)!;

        Assert.False(reloaded.QuickCaptureShowTabBar);
        Assert.Equal(77, reloaded.QuickCaptureRecentLimit);
        Assert.True(reloaded.TodoShowCompletedTab);
        Assert.Equal(TodoOptionKinds.DefaultFilterCompleted, reloaded.TodoDefaultFilter);
        Assert.Equal(30, reloaded.TodoDefaultReminderOffsetMinutes);
        Assert.Equal("Fixture City", reloaded.WeatherCityName);
        Assert.False(reloaded.EnableGlanceImageAutoRotation);
    }
}
