using DeskBox.Models;
using DeskBox.Services;
using DeskBox.ViewModels;

namespace DeskBox.Tests;

public sealed class WeatherResilienceTests : IDisposable
{
    private readonly string _tempRoot = Path.Combine(
        Path.GetTempPath(),
        "DeskBox.Tests",
        Guid.NewGuid().ToString("N"));

    [Fact]
    public async Task CacheStore_RoundTripsForecastAndPreservesLocationResolutionAge()
    {
        var store = CreateStore();
        DateTimeOffset resolvedAt = DateTimeOffset.UtcNow.AddHours(-3);
        Assert.True(await store.SaveLocationAsync(new WeatherCachedLocation
        {
            Latitude = 33.7931,
            Longitude = 113.1446,
            Name = "平顶山",
            ResolvedAtUtc = resolvedAt
        }));

        Assert.True(await store.SaveForecastAsync(CreateForecast(
            fetchedAtUtc: DateTimeOffset.UtcNow.AddMinutes(-2))));

        WeatherCacheState restored = await new WeatherCacheStore(store.StorePath).LoadAsync();

        WeatherCachedLocation location = Assert.IsType<WeatherCachedLocation>(
            restored.LastLocation);
        Assert.Equal(resolvedAt, location.ResolvedAtUtc);
        Assert.Equal("平顶山", location.Name);
        WeatherCachedForecast forecast = Assert.IsType<WeatherCachedForecast>(
            restored.LastForecast);
        Assert.Equal(21, Assert.IsType<WeatherCurrent>(forecast.Data!.Current).Temperature);
        Assert.Equal(SettingsService.WeatherDataSourceMsn, forecast.RequestedSource);
    }

    [Fact]
    public async Task CacheStore_DoesNotPromoteForecastCoordinatesToResolvedLocation()
    {
        var store = CreateStore();

        Assert.True(await store.SaveForecastAsync(CreateForecast(
            fetchedAtUtc: DateTimeOffset.UtcNow.AddMinutes(-2))));

        WeatherCacheState restored = await store.LoadAsync();

        Assert.Null(restored.LastLocation);
        Assert.NotNull(restored.LastForecast);
    }

    [Fact]
    public async Task WeatherService_ReturnsFreshDiskSnapshotWithoutNetworkRequest()
    {
        var store = CreateStore();
        Assert.True(await store.SaveForecastAsync(CreateForecast(
            fetchedAtUtc: DateTimeOffset.UtcNow.AddMinutes(-2))));
        using var service = new WeatherService(store);

        WeatherData? data = await service.GetWeatherAsync(
            33.7931,
            113.1446,
            "平顶山",
            cacheDuration: TimeSpan.FromMinutes(30),
            dataSource: SettingsService.WeatherDataSourceMsn);

        Assert.NotNull(data);
        Assert.Equal(21, data.Current!.Temperature);
        Assert.Equal("平顶山", data.LocationName);
        Assert.False(data.IsStale);
    }

    [Fact]
    public async Task InitializeAsync_RestoresManualCitySnapshotWithoutStartingNetworkRefresh()
    {
        var store = CreateStore();
        Assert.True(await store.SaveForecastAsync(CreateForecast(
            fetchedAtUtc: DateTimeOffset.UtcNow.AddMinutes(-2))));

        var settings = new SettingsService(Path.Combine(_tempRoot, "settings"));
        settings.Settings.WeatherAutoLocation = false;
        settings.Settings.WeatherLatitude = 33.7931;
        settings.Settings.WeatherLongitude = 113.1446;
        settings.Settings.WeatherCityName = "平顶山";
        using var viewModel = new WeatherWidgetViewModel(
            new WidgetConfig
            {
                Id = Guid.NewGuid().ToString("N"),
                Name = "Weather",
                WidgetKind = WidgetKind.Weather
            },
            new WeatherService(store),
            TestServices.CreateLocalizationService(),
            settings,
            dispatcherQueue: null);

        await viewModel.InitializeAsync();

        Assert.True(viewModel.HasData);
        Assert.Equal("平顶山", viewModel.LocationDisplay);
        Assert.NotEqual("--\u00B0", viewModel.CurrentTemperatureText);
        Assert.False(viewModel.IsRefreshing);
    }

    [Theory]
    [InlineData(1, 2)]
    [InlineData(2, 5)]
    [InlineData(3, 15)]
    [InlineData(20, 30)]
    public void BackoffPolicy_BoundsAutomaticRetryDelay(
        int consecutiveFailures,
        int expectedMinutes)
    {
        Assert.Equal(
            TimeSpan.FromMinutes(expectedMinutes),
            WeatherRefreshBackoffPolicy.GetFailureDelay(consecutiveFailures));
    }

    [Fact]
    public void WeatherUiAndTrayAnimation_KeepRefreshNonBlockingAndGuardDisplayArea()
    {
        string weatherXaml = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetContents/WeatherWidgetContent.xaml"));
        string refresh = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WeatherWidgetViewModel.RefreshAndLayout.cs"));
        string trayAnimation = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetTrayAnimationController.cs"));

        Assert.Contains("x:Name=\"LoadingOverlay\"", weatherXaml, StringComparison.Ordinal);
        Assert.Contains("IsHitTestVisible=\"False\"", weatherXaml, StringComparison.Ordinal);
        Assert.Contains("await LoadCachedWeatherAsync();", refresh, StringComparison.Ordinal);
        Assert.Contains("if (displayArea is null)", trayAnimation, StringComparison.Ordinal);
        Assert.Contains("using minimum slide offsets", trayAnimation, StringComparison.Ordinal);
    }

    private WeatherCacheStore CreateStore() => new(Path.Combine(
        _tempRoot,
        WeatherCacheStore.FileName));

    private static WeatherCachedForecast CreateForecast(DateTimeOffset fetchedAtUtc) => new()
    {
        Latitude = 33.7931,
        Longitude = 113.1446,
        LocationName = "平顶山",
        RequestedSource = SettingsService.WeatherDataSourceMsn,
        ActualSource = SettingsService.WeatherDataSourceMsn,
        FetchedAtUtc = fetchedAtUtc,
        Data = new WeatherData
        {
            Latitude = 33.7931,
            Longitude = 113.1446,
            Current = new WeatherCurrent
            {
                Time = "2026-08-25T09:00",
                Temperature = 21,
                ApparentTemperature = 20,
                Humidity = 60,
                WindSpeed = 8,
                WindDirection = 90,
                Pressure = 1008,
                WeatherCode = 1,
                IsDay = 1
            }
        }
    };

    public void Dispose()
    {
        if (Directory.Exists(_tempRoot))
        {
            Directory.Delete(_tempRoot, recursive: true);
        }
    }
}
