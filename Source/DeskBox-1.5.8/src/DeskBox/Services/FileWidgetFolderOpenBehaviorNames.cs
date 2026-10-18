using DeskBox.Models;

namespace DeskBox.Services;

public static class FileWidgetFolderOpenBehaviorNames
{
    // Canonical global values live in Contracts
    // (FileWidgetFolderOpenBehaviors, batch 45); these historical constants
    // are aliases so existing consumers keep compiling unchanged.
    public const string Explorer = DeskBox.Contracts.FileWidgetFolderOpenBehaviors.Explorer;
    public const string Embedded = DeskBox.Contracts.FileWidgetFolderOpenBehaviors.Embedded;
    public const string FollowGlobal = DeskBox.Contracts.FileWidgetFolderOpenBehaviors.FollowGlobal;
    public const string MetadataKey = "FolderOpenBehavior";

    public static string NormalizeGlobal(string? value) =>
        DeskBox.Contracts.FileWidgetFolderOpenBehaviors.NormalizeGlobal(value);

    public static string? GetOverride(WidgetConfig config)
    {
        ArgumentNullException.ThrowIfNull(config);
        if (!config.Metadata.TryGetValue(MetadataKey,out string? value))
        {
            return null;
        }

        return value switch
        {
            Explorer => Explorer,            Embedded => Embedded,            _ => null
        };
    }

    public static string Resolve(AppSettings settings,WidgetConfig config) =>
        GetOverride(config) ?? NormalizeGlobal(settings.FileWidgetFolderOpenBehavior);

    public static void SetOverride(WidgetConfig config,string? value)
    {
        ArgumentNullException.ThrowIfNull(config);
        config.Metadata ??= [];
        if (value is Explorer or Embedded)
        {
            config.Metadata[MetadataKey] = value;
            return;
        }

        config.Metadata.Remove(MetadataKey);
    }

    public static bool NormalizeOverride(WidgetConfig config)
    {
        ArgumentNullException.ThrowIfNull(config);
        config.Metadata ??= [];
        if (!config.Metadata.TryGetValue(MetadataKey,out string? value) ||
            value is Explorer or Embedded)
        {
            return false;
        }

        config.Metadata.Remove(MetadataKey);
        return true;
    }
}
