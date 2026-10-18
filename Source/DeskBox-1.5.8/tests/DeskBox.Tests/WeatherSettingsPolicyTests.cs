using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WeatherSettingsPolicyTests
{
    [Fact]
    public void LocalPreferences_ApplyValidatedManualValuesAndAllDisplayFlags()
    {
        var settings = new AppSettings();

        Assert.True(WeatherSettingsPolicy.TrySetManualLocation(
            settings,
            "Chengdu",
            30.5728,
            104.0668));
        WeatherSettingsPolicy.SetTemperatureUnit(
            settings,
            SettingsService.WeatherTemperatureUnitFahrenheit);
        WeatherSettingsPolicy.SetWindSpeedUnit(
            settings,
            SettingsService.WeatherWindSpeedUnitMph);
        WeatherSettingsPolicy.SetDefaultView(
            settings,
            SettingsService.WeatherDefaultViewWeek);
        WeatherSettingsPolicy.SetSkin(
            settings,
            SettingsService.WeatherSkinStandard);
        WeatherSettingsPolicy.SetRefreshInterval(settings, 15);
        foreach (WeatherDisplayOption option in Enum.GetValues<WeatherDisplayOption>())
        {
            WeatherSettingsPolicy.SetDisplayOption(settings, option, enabled: false);
        }

        Assert.False(settings.WeatherAutoLocation);
        Assert.Equal("Chengdu", settings.WeatherCityName);
        Assert.Equal(30.5728, settings.WeatherLatitude, 4);
        Assert.Equal(104.0668, settings.WeatherLongitude, 4);
        Assert.Equal(SettingsService.WeatherTemperatureUnitFahrenheit, settings.WeatherTemperatureUnit);
        Assert.Equal(SettingsService.WeatherWindSpeedUnitMph, settings.WeatherWindSpeedUnit);
        Assert.Equal(SettingsService.WeatherDefaultViewWeek, settings.WeatherDefaultView);
        Assert.Equal(SettingsService.WeatherSkinStandard, settings.WeatherSkin);
        Assert.Equal(15, settings.WeatherRefreshIntervalMinutes);
        Assert.False(settings.WeatherShowForecast);
        Assert.False(settings.WeatherShowSunrise);
        Assert.False(settings.WeatherShowUvIndex);
        Assert.False(settings.WeatherShowPrecipitation);
        Assert.False(settings.WeatherShowHumidity);
        Assert.False(settings.WeatherShowWind);
        Assert.False(settings.WeatherShowPressure);
    }

    [Theory]
    [InlineData(double.NaN, 0)]
    [InlineData(0, double.PositiveInfinity)]
    [InlineData(-90.001, 0)]
    [InlineData(0, 180.001)]
    public void ManualLocation_RejectsInvalidCoordinatesWithoutChangingSettings(
        double latitude,
        double longitude)
    {
        var settings = new AppSettings
        {
            WeatherAutoLocation = true,
            WeatherCityName = "Original",
            WeatherLatitude = 1,
            WeatherLongitude = 2
        };

        Assert.False(WeatherSettingsPolicy.TrySetManualLocation(
            settings,
            "Rejected",
            latitude,
            longitude));

        Assert.True(settings.WeatherAutoLocation);
        Assert.Equal("Original", settings.WeatherCityName);
        Assert.Equal(1, settings.WeatherLatitude);
        Assert.Equal(2, settings.WeatherLongitude);
    }

    [Fact]
    public void InvalidSelections_NormalizeToExistingProductFallbacks()
    {
        var settings = new AppSettings();

        WeatherSettingsPolicy.SetTemperatureUnit(settings, "Kelvin");
        WeatherSettingsPolicy.SetWindSpeedUnit(settings, "knots");
        WeatherSettingsPolicy.SetDefaultView(settings, "Month");
        WeatherSettingsPolicy.SetSkin(settings, "Unknown");
        WeatherSettingsPolicy.SetRefreshInterval(settings, 999);

        Assert.Equal(SettingsService.WeatherTemperatureUnitCelsius, settings.WeatherTemperatureUnit);
        Assert.Equal(SettingsService.WeatherWindSpeedUnitKmh, settings.WeatherWindSpeedUnit);
        Assert.Equal(SettingsService.WeatherDefaultViewToday, settings.WeatherDefaultView);
        Assert.Equal(SettingsService.WeatherSkinStandard, settings.WeatherSkin);
        Assert.Equal(SettingsService.WeatherRefreshMaxMinutes, settings.WeatherRefreshIntervalMinutes);
    }
}
