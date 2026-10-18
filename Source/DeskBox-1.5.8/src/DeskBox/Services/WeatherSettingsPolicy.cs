using DeskBox.Contracts;
using DeskBox.Models;

namespace DeskBox.Services;

internal enum WeatherDisplayOption
{
    Forecast,
    Sunrise,
    UvIndex,
    Precipitation,
    Humidity,
    Wind,
    Pressure
}

/// <summary>
/// Applies the persisted, local-only weather preferences shared by the settings
/// surface and persistence validation. Network and location resolution do not
/// belong to this policy.
/// </summary>
internal static class WeatherSettingsPolicy
{
    internal static void SetAutoLocation(AppSettings settings,bool enabled)
    {
        ArgumentNullException.ThrowIfNull(settings);
        settings.WeatherAutoLocation = enabled;
    }

    internal static bool TrySetManualLocation(
        AppSettings settings,        string cityName,        double latitude,        double longitude)
    {
        ArgumentNullException.ThrowIfNull(settings);
        ArgumentNullException.ThrowIfNull(cityName);
        if (!double.IsFinite(latitude) ||
            !double.IsFinite(longitude) ||
            latitude is < -90 or > 90 ||
            longitude is < -180 or > 180)
        {
            return false;
        }

        settings.WeatherAutoLocation = false;
        settings.WeatherCityName = cityName;
        settings.WeatherLatitude = latitude;
        settings.WeatherLongitude = longitude;
        return true;
    }

    internal static void SetTemperatureUnit(AppSettings settings,string value)
    {
        ArgumentNullException.ThrowIfNull(settings);
        settings.WeatherTemperatureUnit =
            value == SettingsService.WeatherTemperatureUnitFahrenheit
                ? SettingsService.WeatherTemperatureUnitFahrenheit
                : SettingsService.WeatherTemperatureUnitCelsius;
    }

    internal static void SetWindSpeedUnit(AppSettings settings,string value)
    {
        ArgumentNullException.ThrowIfNull(settings);
        settings.WeatherWindSpeedUnit = value is
            SettingsService.WeatherWindSpeedUnitMs or
            SettingsService.WeatherWindSpeedUnitMph
                ? value
                : SettingsService.WeatherWindSpeedUnitKmh;
    }

    internal static void SetDefaultView(AppSettings settings,string value)
    {
        ArgumentNullException.ThrowIfNull(settings);
        settings.WeatherDefaultView = value == SettingsService.WeatherDefaultViewWeek
            ? SettingsService.WeatherDefaultViewWeek
            : SettingsService.WeatherDefaultViewToday;
    }

    internal static void SetSkin(AppSettings settings,string value)
    {
        ArgumentNullException.ThrowIfNull(settings);
        settings.WeatherSkin = value == SettingsService.WeatherSkinRich
            ? SettingsService.WeatherSkinRich
            : SettingsService.WeatherSkinStandard;
    }

    internal static void SetIconStyle(AppSettings settings,string value)
    {
        ArgumentNullException.ThrowIfNull(settings);
        settings.WeatherIconStyle = WeatherOptionKinds.NormalizeIconStyle(value);
    }

    internal static void SetRefreshInterval(AppSettings settings,int minutes)
    {
        ArgumentNullException.ThrowIfNull(settings);
        settings.WeatherRefreshIntervalMinutes = Math.Clamp(
            minutes,            SettingsService.WeatherRefreshMinMinutes,            SettingsService.WeatherRefreshMaxMinutes);
    }

    internal static void SetDisplayOption(
        AppSettings settings,        WeatherDisplayOption option,        bool enabled)
    {
        ArgumentNullException.ThrowIfNull(settings);
        switch (option)
        {
            case WeatherDisplayOption.Forecast:
                settings.WeatherShowForecast = enabled;
                break;
            case WeatherDisplayOption.Sunrise:
                settings.WeatherShowSunrise = enabled;
                break;
            case WeatherDisplayOption.UvIndex:
                settings.WeatherShowUvIndex = enabled;
                break;
            case WeatherDisplayOption.Precipitation:
                settings.WeatherShowPrecipitation = enabled;
                break;
            case WeatherDisplayOption.Humidity:
                settings.WeatherShowHumidity = enabled;
                break;
            case WeatherDisplayOption.Wind:
                settings.WeatherShowWind = enabled;
                break;
            case WeatherDisplayOption.Pressure:
                settings.WeatherShowPressure = enabled;
                break;
            default:
                throw new ArgumentOutOfRangeException(nameof(option),option,null);
        }
    }
}
