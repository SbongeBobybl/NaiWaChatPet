using DeskBox.Contracts;
using DeskBox.Models;

namespace DeskBox.Services;

/// <summary>
/// Normalizes and resolves the per-widget border override. FollowGlobal is
/// the default: the widget uses the global border style and color — except
/// that a widget with a custom background image drops its border so the
/// photo reads edge to edge. An explicit Accent/Neutral/None choice always
/// wins over that heuristic.
/// </summary>
public static class WidgetBorderCustomization
{
    public const string ModeMetadataKey = "WidgetBorderMode";

    public const string ModeFollowGlobal = "FollowGlobal";
    public const string ModeAccent = "Accent";
    public const string ModeNeutral = "Neutral";
    public const string ModeNone = "None";

    public static string NormalizeMode(string? value)
    {
        if (string.Equals(value,ModeAccent,StringComparison.OrdinalIgnoreCase))
        {
            return ModeAccent;
        }

        if (string.Equals(value,ModeNeutral,StringComparison.OrdinalIgnoreCase))
        {
            return ModeNeutral;
        }

        if (string.Equals(value,ModeNone,StringComparison.OrdinalIgnoreCase))
        {
            return ModeNone;
        }

        return ModeFollowGlobal;
    }

    public static string? GetModeOverride(WidgetConfig config)
    {
        ArgumentNullException.ThrowIfNull(config);
        if (config.Metadata is not null &&
            config.Metadata.TryGetValue(ModeMetadataKey,out string? value))
        {
            string normalized = NormalizeMode(value);
            return normalized == ModeFollowGlobal ? null : normalized;
        }

        return null;
    }

    public static void SetModeOverride(WidgetConfig config,string? value)
    {
        ArgumentNullException.ThrowIfNull(config);
        string normalized = NormalizeMode(value);
        if (normalized == ModeFollowGlobal)
        {
            config.Metadata?.Remove(ModeMetadataKey);
            return;
        }

        config.Metadata ??= [];
        config.Metadata[ModeMetadataKey] = normalized;
    }

    public static bool NormalizeOverrides(WidgetConfig config)
    {
        ArgumentNullException.ThrowIfNull(config);
        if (config.Metadata is not null &&
            config.Metadata.TryGetValue(ModeMetadataKey,out string? value) &&
            NormalizeMode(value) == ModeFollowGlobal)
        {
            config.Metadata.Remove(ModeMetadataKey);
            return true;
        }

        return false;
    }

    /// <summary>
    /// Maps the per-widget override onto the global style/color pair. Pure
    /// so the exact matrix can be contract-tested.
    /// </summary>
    public static (string Style, string ColorMode) ApplyOverride(
        string? mode,        string globalStyle,        string globalColorMode,        bool hasCustomBackground)
    {
        globalStyle = WidgetBorderKinds.NormalizeStyle(globalStyle);
        globalColorMode = WidgetBorderKinds.NormalizeColorMode(globalColorMode);

        switch (NormalizeMode(mode))
        {
            case ModeNone:
                return (WidgetBorderKinds.StyleNone, WidgetBorderKinds.ColorNone);

            case ModeAccent:
            case ModeNeutral:
            {
                // An explicit color choice on a globally borderless look must
                // still draw something; fall back to the thin default.
                string style = globalStyle == WidgetBorderKinds.StyleNone
                    ? WidgetBorderKinds.StyleThin
                    : globalStyle;
                string colorMode = NormalizeMode(mode) == ModeAccent
                    ? WidgetBorderKinds.ColorAccent
                    : WidgetBorderKinds.ColorNeutral;
                return (style, colorMode);
            }

            default:
                // FollowGlobal: a custom background drops the border so the
                // image reads edge to edge; plain widgets follow the globals.
                return hasCustomBackground
                    ? (WidgetBorderKinds.StyleNone, WidgetBorderKinds.ColorNone)
                    : (globalStyle, globalColorMode);
        }
    }
}
