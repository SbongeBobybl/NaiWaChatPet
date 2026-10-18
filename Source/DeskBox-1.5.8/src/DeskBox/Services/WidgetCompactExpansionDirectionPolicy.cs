using DeskBox.Models;
using Windows.Graphics;

namespace DeskBox.Services;

internal static class WidgetCompactExpansionDirectionPolicy
{
    public const string OverrideMetadataKey = "CompactExpansionDirection";

    private static readonly string[] OverrideValues =
    [
        SettingsService.WidgetCompactExpansionDirectionAuto,        SettingsService.WidgetCompactExpansionDirectionDown,        SettingsService.WidgetCompactExpansionDirectionUp
    ];

    public static bool RequiresFullSize(string? configuredDirection) =>
        SettingsService.NormalizeWidgetCompactExpansionDirection(configuredDirection) !=
        SettingsService.WidgetCompactExpansionDirectionAuto;

    /// <summary>
    /// Per-widget direction override stored in <see cref="WidgetConfig.Metadata"/>.
    /// Null means the capsule follows the global setting.
    /// </summary>
    public static string? GetOverride(WidgetConfig config)
    {
        if (config.Metadata is null ||
            !config.Metadata.TryGetValue(OverrideMetadataKey,out string? value))
        {
            return null;
        }

        foreach (string known in OverrideValues)
        {
            if (string.Equals(value,known,StringComparison.OrdinalIgnoreCase))
            {
                return known;
            }
        }

        return null;
    }

    public static void SetOverride(WidgetConfig config,string? direction)
    {
        if (direction is null)
        {
            config.Metadata?.Remove(OverrideMetadataKey);
            return;
        }

        config.Metadata ??= [];
        config.Metadata[OverrideMetadataKey] =
            SettingsService.NormalizeWidgetCompactExpansionDirection(direction);
    }

    public static string ResolveEffective(WidgetConfig config,string? globalDirection) =>
        GetOverride(config) ??
        SettingsService.NormalizeWidgetCompactExpansionDirection(globalDirection);

    public static IReadOnlyList<WidgetCompactExpansionAnchor> Apply(
        string? configuredDirection,        IReadOnlyList<WidgetCompactExpansionAnchor> anchors)
    {
        string direction = SettingsService.NormalizeWidgetCompactExpansionDirection(
            configuredDirection);
        if (direction == SettingsService.WidgetCompactExpansionDirectionAuto)
        {
            return anchors;
        }

        bool expandsDown = direction == SettingsService.WidgetCompactExpansionDirectionDown;
        var constrained = new List<WidgetCompactExpansionAnchor>(Math.Max(2,anchors.Count));
        foreach (WidgetCompactExpansionAnchor anchor in anchors)
        {
            WidgetCompactExpansionAnchor mapped = (anchor, expandsDown) switch
            {
                (WidgetCompactExpansionAnchor.RightTop or WidgetCompactExpansionAnchor.RightBottom, true) =>
                    WidgetCompactExpansionAnchor.RightTop,                (WidgetCompactExpansionAnchor.RightTop or WidgetCompactExpansionAnchor.RightBottom, false) =>
                    WidgetCompactExpansionAnchor.RightBottom,                (_, true) => WidgetCompactExpansionAnchor.LeftTop,                _ => WidgetCompactExpansionAnchor.LeftBottom
            };
            if (!constrained.Contains(mapped))
            {
                constrained.Add(mapped);
            }
        }

        if (constrained.Count == 0)
        {
            constrained.Add(expandsDown
                ? WidgetCompactExpansionAnchor.LeftTop
                : WidgetCompactExpansionAnchor.LeftBottom);
            constrained.Add(expandsDown
                ? WidgetCompactExpansionAnchor.RightTop
                : WidgetCompactExpansionAnchor.RightBottom);
        }

        return constrained;
    }

    /// <summary>
    /// Resolves the layout an expansion request should use. A fixed direction
    /// keeps its direction and only adapts the expanded size to the space that
    /// is actually available; automatic direction may still pick another
    /// anchor. Expansion therefore never hard-fails for lack of room.
    /// </summary>
    public static WidgetCompactExpansionLayout ResolveAdaptive(
        RectInt32 compactBounds,        SizeInt32 requestedSize,        RectInt32 workArea,        IReadOnlyList<WidgetCompactExpansionAnchor> anchors,        string? configuredDirection)
    {
        IReadOnlyList<WidgetCompactExpansionAnchor> constrained =
            Apply(configuredDirection,anchors);
        WidgetCompactExpansionLayout strict = WidgetCompactExpansionCalculator.Resolve(
            compactBounds,            requestedSize,            workArea,            constrained,            RequiresFullSize(configuredDirection));
        return strict.CanExpand
            ? strict
            : WidgetCompactExpansionCalculator.Resolve(
                compactBounds,                requestedSize,                workArea,                constrained);
    }
}
