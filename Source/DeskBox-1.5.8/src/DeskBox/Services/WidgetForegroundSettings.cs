using DeskBox.Helpers;
using DeskBox.Models;
using Windows.UI;

namespace DeskBox.Services;

/// <summary>
/// Normalizes and resolves global and per-widget foreground preferences.
/// Per-widget values live in <see cref="WidgetConfig.Metadata"/> so older
/// settings files can read and ignore them without a schema migration.
/// </summary>
public static class WidgetForegroundSettings
{
    public const string ModeFollowTheme = Contracts.WidgetForegroundKinds.FollowTheme;
    public const string ModeLight = Contracts.WidgetForegroundKinds.Light;
    public const string ModeDark = Contracts.WidgetForegroundKinds.Dark;
    public const string ModeCustom = Contracts.WidgetForegroundKinds.Custom;

    public const string DefaultCustomColorHex = Contracts.WidgetForegroundKinds.DefaultCustomColorHex;

    public const string ModeOverrideMetadataKey = "WidgetForegroundMode";
    public const string ColorOverrideMetadataKey = "WidgetForegroundColor";

    public static string NormalizeMode(string? value)
    {
        if (string.Equals(value,ModeLight,StringComparison.OrdinalIgnoreCase))
        {
            return ModeLight;
        }

        if (string.Equals(value,ModeDark,StringComparison.OrdinalIgnoreCase))
        {
            return ModeDark;
        }

        if (string.Equals(value,ModeCustom,StringComparison.OrdinalIgnoreCase))
        {
            return ModeCustom;
        }

        return ModeFollowTheme;
    }

    public static string? GetModeOverride(WidgetConfig config)
    {
        ArgumentNullException.ThrowIfNull(config);
        if (config.Metadata is null ||
            !config.Metadata.TryGetValue(ModeOverrideMetadataKey,out string? value))
        {
            return null;
        }

        return IsSupportedMode(value) ? NormalizeMode(value) : null;
    }

    public static string ResolveMode(WidgetConfig config,AppSettings settings) =>
        GetModeOverride(config) ?? NormalizeMode(settings.WidgetForegroundMode);

    public static Color ResolveCustomColor(WidgetConfig config,AppSettings settings)
    {
        if (config.Metadata is not null &&
            config.Metadata.TryGetValue(ColorOverrideMetadataKey,out string? overrideValue) &&
            TryNormalizeColor(overrideValue,out _,out Color overrideColor))
        {
            return overrideColor;
        }

        return TryNormalizeColor(settings.WidgetForegroundColor,out _,out Color globalColor)
            ? globalColor
            : AccentColorHelper.FromHex(DefaultCustomColorHex);
    }

    public static void SetModeOverride(WidgetConfig config,string? value)
    {
        ArgumentNullException.ThrowIfNull(config);
        config.Metadata ??= [];
        if (string.IsNullOrWhiteSpace(value))
        {
            config.Metadata.Remove(ModeOverrideMetadataKey);
            return;
        }

        config.Metadata[ModeOverrideMetadataKey] = NormalizeMode(value);
    }

    public static void SetCustomColorOverride(WidgetConfig config,Color color)
    {
        ArgumentNullException.ThrowIfNull(config);
        config.Metadata ??= [];
        config.Metadata[ColorOverrideMetadataKey] = AccentColorHelper.ToHex(
            Color.FromArgb(0xFF,color.R,color.G,color.B));
    }

    public static bool NormalizeGlobal(AppSettings settings)
    {
        ArgumentNullException.ThrowIfNull(settings);
        bool changed = false;

        string mode = NormalizeMode(settings.WidgetForegroundMode);
        if (!string.Equals(settings.WidgetForegroundMode,mode,StringComparison.Ordinal))
        {
            settings.WidgetForegroundMode = mode;
            changed = true;
        }

        if (!TryNormalizeColor(settings.WidgetForegroundColor,out string colorHex,out _))
        {
            colorHex = DefaultCustomColorHex;
        }

        if (!string.Equals(settings.WidgetForegroundColor,colorHex,StringComparison.Ordinal))
        {
            settings.WidgetForegroundColor = colorHex;
            changed = true;
        }

        return changed;
    }

    public static bool NormalizeOverrides(WidgetConfig config)
    {
        ArgumentNullException.ThrowIfNull(config);
        config.Metadata ??= [];
        bool changed = NormalizeOptionalValue(
            config,            ModeOverrideMetadataKey,            IsSupportedMode,            NormalizeMode);

        if (config.Metadata.TryGetValue(ColorOverrideMetadataKey,out string? colorValue))
        {
            if (!TryNormalizeColor(colorValue,out string colorHex,out _))
            {
                config.Metadata.Remove(ColorOverrideMetadataKey);
                changed = true;
            }
            else if (!string.Equals(colorValue,colorHex,StringComparison.Ordinal))
            {
                config.Metadata[ColorOverrideMetadataKey] = colorHex;
                changed = true;
            }
        }

        return changed;
    }

    private static bool NormalizeOptionalValue(
        WidgetConfig config,        string key,        Func<string?, bool> isSupported,        Func<string?, string> normalize)
    {
        if (!config.Metadata.TryGetValue(key,out string? value))
        {
            return false;
        }

        if (!isSupported(value))
        {
            config.Metadata.Remove(key);
            return true;
        }

        string normalized = normalize(value);
        if (string.Equals(value,normalized,StringComparison.Ordinal))
        {
            return false;
        }

        config.Metadata[key] = normalized;
        return true;
    }

    private static bool TryNormalizeColor(
        string? value,        out string colorHex,        out Color color)
    {
        if (!AccentColorHelper.TryParseHex(value,out color))
        {
            colorHex = DefaultCustomColorHex;
            return false;
        }

        color = Color.FromArgb(0xFF,color.R,color.G,color.B);
        colorHex = AccentColorHelper.ToHex(color);
        return true;
    }

    private static bool IsSupportedMode(string? value) =>
        string.Equals(value,ModeFollowTheme,StringComparison.OrdinalIgnoreCase) ||
        string.Equals(value,ModeLight,StringComparison.OrdinalIgnoreCase) ||
        string.Equals(value,ModeDark,StringComparison.OrdinalIgnoreCase) ||
        string.Equals(value,ModeCustom,StringComparison.OrdinalIgnoreCase);
}
