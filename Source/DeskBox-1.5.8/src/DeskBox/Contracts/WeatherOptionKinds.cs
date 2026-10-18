namespace DeskBox.Contracts;

/// <summary>
/// Canonical option values, bounds and read normalizers for the Weather
/// settings section (the single source the section editor binds against;
/// <see cref="Services.SettingsService"/> keeps its historical constants as
/// aliases of these). The write-side policy stays in
/// <c>Services.WeatherSettingsPolicy</c>; the normalizers here mirror the
/// old shell-constructor read semantics the editor's snapshot projection
/// used before the migration.
/// </summary>
public static class WeatherOptionKinds
{
    public const string LocationModeAuto = "Auto";
    public const string LocationModeManual = "Manual";

    public const string TemperatureUnitCelsius = "Celsius";
    public const string TemperatureUnitFahrenheit = "Fahrenheit";

    public const string WindSpeedUnitKmh = "kmh";
    public const string WindSpeedUnitMs = "ms";
    public const string WindSpeedUnitMph = "mph";

    public const string DefaultViewToday = "Today";
    public const string DefaultViewWeek = "Week";

    public const string SkinStandard = "Standard";
    public const string SkinRich = "Rich";

    public const string IconStyleDeskBox = "DeskBox";
    public const string IconStyleFlat = "Flat";
    public const string IconStyleLine = "Line";
    public const string IconStyleFluent = "Fluent";
    public const string DefaultIconStyle = IconStyleFluent;

    /// <summary>The icon styles offered by the icon-style combo. The Fill
    /// (gradient) Meteocons set was dropped: SvgImageSource renders it
    /// identically to Flat on WinUI, so the extra option claimed a visual
    /// difference that never materialized. The system-emoji option was
    /// removed because Segoe UI Emoji renders differently (and worse) on
    /// Windows 10 than on Windows 11; Fluent is the default and leads the
    /// combo, and persisted "Emoji" values normalize to it on load.</summary>
    public static readonly string[] IconStyleValues =
    [
        IconStyleFluent,        IconStyleDeskBox,        IconStyleFlat,        IconStyleLine
    ];

    public const string DataSourceMsn = "MSN";
    public const string DataSourceOpenMeteo = "OpenMeteo";

    public const int RefreshMinMinutes = 15;
    public const int RefreshMaxMinutes = 180;
    public const int DefaultRefreshIntervalMinutes = 60;

    /// <summary>The refresh-interval choices offered by the refresh combo.</summary>
    public static readonly int[] RefreshIntervalSteps = [15,30,60,180];

    /// <summary>The display toggles offered by the display-options flyout.</summary>
    public static readonly string[] DisplayOptionKeys =
    [
        "Forecast",        "Sunrise",        "UvIndex",        "Precipitation",        "Humidity",        "Wind",        "Pressure"
    ];

    public static string NormalizeTemperatureUnit(string? unit) =>
        unit == TemperatureUnitFahrenheit
            ? TemperatureUnitFahrenheit
            : TemperatureUnitCelsius;

    public static string NormalizeWindSpeedUnit(string? unit) => unit is
        WindSpeedUnitMs or
        WindSpeedUnitMph
        ? unit
        : WindSpeedUnitKmh;

    public static string NormalizeDefaultView(string? view) =>
        view == DefaultViewWeek ? DefaultViewWeek : DefaultViewToday;

    public static string NormalizeSkin(string? skin) =>
        skin == SkinRich ? SkinRich : SkinStandard;

    public static string NormalizeIconStyle(string? style) => style is
        IconStyleDeskBox or IconStyleFlat or IconStyleLine or IconStyleFluent
        ? style
        : DefaultIconStyle;

    public static string NormalizeDataSource(string? source) =>
        source == DataSourceOpenMeteo ? DataSourceOpenMeteo : DataSourceMsn;

    public static int NormalizeRefreshInterval(int minutes) =>
        Math.Clamp(minutes,RefreshMinMinutes,RefreshMaxMinutes);
}
