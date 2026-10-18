using DeskBox.Models;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace DeskBox.Services;

[JsonSourceGenerationOptions(GenerationMode = JsonSourceGenerationMode.Metadata)]
[JsonSerializable(
    typeof(Dictionary<string, string>),
    TypeInfoPropertyName = "StringMap")]
[JsonSerializable(
    typeof(Dictionary<string, List<string>>),
    TypeInfoPropertyName = "StringListMap")]
[JsonSerializable(typeof(List<string>), TypeInfoPropertyName = "StringList")]
internal sealed partial class WidgetMetadataJsonContext : JsonSerializerContext
{
}

public static class WidgetFileStackSettings
{
    private const string LooseItemOrderKeyPrefix = "Item:";
    private const string ManualStackKeyPrefix = "Manual:";

    public const string EnabledOverrideMetadataKey = "FileStacksEnabled";
    public const string GroupByOverrideMetadataKey = "FileStackGroupBy";
    public const string ThresholdOverrideMetadataKey = "FileStackThreshold";
    public const string OrderByOverrideMetadataKey = "FileStackOrderBy";
    public const string OpenModeOverrideMetadataKey = "FileStackOpenMode";
    public const string DisabledStacksMetadataKey = "FileStackDisabledGroups";
    public const string StackNameOverridesMetadataKey = "FileStackNameOverrides";
    public const string StackOrderMetadataKey = "FileStackGroupOrder";
    public const string StackMemberOverridesMetadataKey = "FileStackMemberOverrides";

    public static bool? GetEnabledOverride(WidgetConfig config)
    {
        if (config.Metadata is null ||
            !config.Metadata.TryGetValue(EnabledOverrideMetadataKey,out string? value) ||
            !bool.TryParse(value,out bool enabled))
        {
            return null;
        }

        return enabled;
    }

    public static string? GetGroupByOverride(WidgetConfig config)
    {
        if (config.Metadata is null ||
            !config.Metadata.TryGetValue(GroupByOverrideMetadataKey,out string? value))
        {
            return null;
        }

        if (!string.Equals(value,SettingsService.FileStackGroupByKind,StringComparison.OrdinalIgnoreCase) &&
            !string.Equals(value,SettingsService.FileStackGroupByDateAdded,StringComparison.OrdinalIgnoreCase) &&
            !string.Equals(value,SettingsService.FileStackGroupByDateCreated,StringComparison.OrdinalIgnoreCase) &&
            !string.Equals(value,SettingsService.FileStackGroupByDateModified,StringComparison.OrdinalIgnoreCase) &&
            !string.Equals(value,SettingsService.FileStackGroupByCustom,StringComparison.OrdinalIgnoreCase))
        {
            return null;
        }

        string normalized = SettingsService.NormalizeFileStackGroupBy(value);
        return normalized == SettingsService.FileStackGroupByDateAdded
            ? SettingsService.FileStackGroupByKind
            : normalized;
    }

    public static int? GetThresholdOverride(WidgetConfig config)
    {
        if (config.Metadata is null ||
            !config.Metadata.TryGetValue(ThresholdOverrideMetadataKey,out string? value) ||
            !int.TryParse(value,out int threshold) ||
            SettingsService.NormalizeFileStackThreshold(threshold) != threshold)
        {
            return null;
        }

        return threshold;
    }

    public static string? GetOrderByOverride(WidgetConfig config)
    {
        if (config.Metadata is null ||
            !config.Metadata.TryGetValue(OrderByOverrideMetadataKey,out string? value))
        {
            return null;
        }

        if (!string.Equals(value,SettingsService.FileStackOrderByWidget,StringComparison.OrdinalIgnoreCase) &&
            !string.Equals(value,SettingsService.FileStackOrderByName,StringComparison.OrdinalIgnoreCase) &&
            !string.Equals(value,SettingsService.FileStackOrderByDateAdded,StringComparison.OrdinalIgnoreCase) &&
            !string.Equals(value,SettingsService.FileStackOrderByDateModified,StringComparison.OrdinalIgnoreCase))
        {
            return null;
        }

        return SettingsService.NormalizeFileStackOrderBy(value);
    }

    public static string? GetOpenModeOverride(WidgetConfig config)
    {
        if (config.Metadata is null ||
            !config.Metadata.TryGetValue(OpenModeOverrideMetadataKey,out string? value) ||
            (!string.Equals(
                 value,                 SettingsService.FileStackOpenModeInline,                 StringComparison.OrdinalIgnoreCase) &&
             !string.Equals(
                 value,                 SettingsService.FileStackOpenModePopover,                 StringComparison.OrdinalIgnoreCase)))
        {
            return null;
        }

        return SettingsService.NormalizeFileStackOpenMode(value);
    }

    public static bool ResolveEnabled(WidgetConfig config,bool globalDefault) =>
        GetEnabledOverride(config) ?? globalDefault;

    public static string ResolveGroupBy(WidgetConfig config,string? globalDefault) =>
        GetGroupByOverride(config) ?? SettingsService.NormalizeFileStackGroupBy(globalDefault);

    public static int ResolveThreshold(WidgetConfig config,int globalDefault) =>
        GetThresholdOverride(config) ?? SettingsService.NormalizeFileStackThreshold(globalDefault);

    public static string ResolveOrderBy(WidgetConfig config,string? globalDefault) =>
        GetOrderByOverride(config) ?? SettingsService.NormalizeFileStackOrderBy(globalDefault);

    public static string ResolveOpenMode(WidgetConfig config,string? globalDefault) =>
        GetOpenModeOverride(config) ?? SettingsService.NormalizeFileStackOpenMode(globalDefault);

    public static bool FollowsGlobalDefaults(WidgetConfig config) =>
        GetEnabledOverride(config) is null &&
        GetGroupByOverride(config) is null &&
        GetThresholdOverride(config) is null &&
        GetOrderByOverride(config) is null &&
        GetOpenModeOverride(config) is null;

    public static void SetEnabledOverride(WidgetConfig config,bool? enabled)
    {
        config.Metadata ??= [];
        if (enabled is null)
        {
            config.Metadata.Remove(EnabledOverrideMetadataKey);
            return;
        }

        config.Metadata[EnabledOverrideMetadataKey] = enabled.Value.ToString();
    }

    public static void SetGroupByOverride(WidgetConfig config,string? groupBy)
    {
        config.Metadata ??= [];
        if (groupBy is null)
        {
            config.Metadata.Remove(GroupByOverrideMetadataKey);
            return;
        }

        string normalized = SettingsService.NormalizeFileStackGroupBy(groupBy);
        config.Metadata[GroupByOverrideMetadataKey] =
            normalized == SettingsService.FileStackGroupByDateAdded
                ? SettingsService.FileStackGroupByKind
                : normalized;
    }

    public static void SetThresholdOverride(WidgetConfig config,int? threshold)
    {
        config.Metadata ??= [];
        if (threshold is null)
        {
            config.Metadata.Remove(ThresholdOverrideMetadataKey);
            return;
        }

        config.Metadata[ThresholdOverrideMetadataKey] =
            SettingsService.NormalizeFileStackThreshold(threshold.Value).ToString();
    }

    public static void SetOrderByOverride(WidgetConfig config,string? orderBy)
    {
        config.Metadata ??= [];
        if (orderBy is null)
        {
            config.Metadata.Remove(OrderByOverrideMetadataKey);
            return;
        }

        config.Metadata[OrderByOverrideMetadataKey] =
            SettingsService.NormalizeFileStackOrderBy(orderBy);
    }

    public static void SetOpenModeOverride(WidgetConfig config,string? openMode)
    {
        config.Metadata ??= [];
        if (openMode is null)
        {
            config.Metadata.Remove(OpenModeOverrideMetadataKey);
            return;
        }

        config.Metadata[OpenModeOverrideMetadataKey] =
            SettingsService.NormalizeFileStackOpenMode(openMode);
    }

    public static void ClearOverrides(WidgetConfig config)
    {
        config.Metadata?.Remove(EnabledOverrideMetadataKey);
        config.Metadata?.Remove(GroupByOverrideMetadataKey);
        config.Metadata?.Remove(ThresholdOverrideMetadataKey);
        config.Metadata?.Remove(OrderByOverrideMetadataKey);
        config.Metadata?.Remove(OpenModeOverrideMetadataKey);
    }

    // ── Stack customizations (rename / unstack / manual group order) ──

    /// <summary>
    /// Group keys whose stacks the user explicitly dissolved ("don't stack this
    /// group"). Members of these groups are always projected as loose items.
    /// </summary>
    public static HashSet<string> GetDisabledStacks(WidgetConfig config) =>
        ReadStringCollection(config,DisabledStacksMetadataKey);

    public static void SetDisabledStacks(WidgetConfig config,IEnumerable<string> stackKeys) =>
        WriteStringCollection(config,DisabledStacksMetadataKey,stackKeys);

    /// <summary>User-assigned display names per stack group key.</summary>
    public static Dictionary<string, string> GetStackNameOverrides(WidgetConfig config)
    {
        if (config.Metadata is null ||
            !config.Metadata.TryGetValue(StackNameOverridesMetadataKey,out string? json) ||
            string.IsNullOrWhiteSpace(json))
        {
            return new Dictionary<string, string>(StringComparer.Ordinal);
        }

        try
        {
            return JsonSerializer.Deserialize(
                       json,                       WidgetMetadataJsonContext.Default.StringMap) ??
                new Dictionary<string, string>(StringComparer.Ordinal);
        }
        catch (JsonException)
        {
            return new Dictionary<string, string>(StringComparer.Ordinal);
        }
    }

    public static void SetStackNameOverrides(WidgetConfig config,IReadOnlyDictionary<string, string> overrides)
    {
        config.Metadata ??= [];
        if (overrides.Count == 0)
        {
            config.Metadata.Remove(StackNameOverridesMetadataKey);
            return;
        }

        Dictionary<string, string> values = overrides.ToDictionary(
            entry => entry.Key,            entry => entry.Value,            StringComparer.Ordinal);
        config.Metadata[StackNameOverridesMetadataKey] = JsonSerializer.Serialize(
            values,            WidgetMetadataJsonContext.Default.StringMap);
    }

    /// <summary>
    /// Manual group order (group keys in display order). Keys that no longer exist
    /// are ignored at merge time; new groups append in default order.
    /// </summary>
    public static List<string> GetStackOrder(WidgetConfig config) =>
        ReadStringList(config,StackOrderMetadataKey);

    public static void SetStackOrder(WidgetConfig config,IEnumerable<string>? stackKeys)
    {
        if (stackKeys is null)
        {
            config.Metadata?.Remove(StackOrderMetadataKey);
            return;
        }

        WriteStringCollection(config,StackOrderMetadataKey,stackKeys);
    }

    public static Dictionary<string, List<string>> GetStackMemberOverrides(
        WidgetConfig config)
    {
        if (config.Metadata is null ||
            !config.Metadata.TryGetValue(
                StackMemberOverridesMetadataKey,                out string? json) ||
            string.IsNullOrWhiteSpace(json))
        {
            return new Dictionary<string, List<string>>(
                StringComparer.Ordinal);
        }

        try
        {
            Dictionary<string, List<string>>? values =
                JsonSerializer.Deserialize(
                    json,                    WidgetMetadataJsonContext.Default.StringListMap);
            if (values is null)
            {
                return new Dictionary<string, List<string>>(
                    StringComparer.Ordinal);
            }

            return values
                .Where(entry =>
                    !string.IsNullOrWhiteSpace(entry.Key))
                .ToDictionary(
                    entry => entry.Key,                    entry => (entry.Value ?? [])
                        .Where(path =>
                            !string.IsNullOrWhiteSpace(path))
                        .Distinct(StringComparer.OrdinalIgnoreCase)
                        .ToList(),                    StringComparer.Ordinal);
        }
        catch (JsonException)
        {
            return new Dictionary<string, List<string>>(
                StringComparer.Ordinal);
        }
    }

    public static void SetStackMemberOverrides(
        WidgetConfig config,        IReadOnlyDictionary<string, List<string>> overrides)
    {
        config.Metadata ??= [];
        Dictionary<string, List<string>> normalized = overrides
            .Where(entry =>
                !string.IsNullOrWhiteSpace(entry.Key))
            .Select(entry => new
            {
                entry.Key,
                Paths = (entry.Value ?? [])
                    .Where(path =>
                        !string.IsNullOrWhiteSpace(path))
                    .Distinct(StringComparer.OrdinalIgnoreCase)
                    .ToList()
            })
            .Where(entry => entry.Paths.Count > 0)
            .ToDictionary(
                entry => entry.Key,                entry => entry.Paths,                StringComparer.Ordinal);
        if (normalized.Count == 0)
        {
            config.Metadata.Remove(
                StackMemberOverridesMetadataKey);
            return;
        }

        config.Metadata[StackMemberOverridesMetadataKey] =
            JsonSerializer.Serialize(
                normalized,                WidgetMetadataJsonContext.Default.StringListMap);
    }

    /// <summary>
    /// Rebases every widget-owned absolute path after its managed root folder is
    /// renamed. The calculation is completed before the config is mutated so a
    /// failed directory move never leaves partially migrated stack metadata.
    /// </summary>
    public static bool RebaseManagedFolderPaths(
        WidgetConfig config,        string oldRootPath,        string newRootPath)
    {
        string oldRoot = NormalizeRootPath(oldRootPath);
        string newRoot = NormalizeRootPath(newRootPath);
        if (string.Equals(oldRoot,newRoot,StringComparison.OrdinalIgnoreCase))
        {
            return false;
        }

        Dictionary<string, List<string>> currentMembers =
            GetStackMemberOverrides(config);
        Dictionary<string, List<string>> rebasedMembers = currentMembers
            .ToDictionary(
                entry => entry.Key,                entry => entry.Value
                    .Select(path => RebasePath(path,oldRoot,newRoot))
                    .Distinct(StringComparer.OrdinalIgnoreCase)
                    .ToList(),                StringComparer.Ordinal);

        List<string> currentOrder = GetStackOrder(config);
        List<string> rebasedOrder = currentOrder
            .Select(key => RebaseLooseItemOrderKey(key,oldRoot,newRoot))
            .Distinct(StringComparer.Ordinal)
            .ToList();

        Dictionary<string, DateTimeOffset> currentAddedAt =
            config.FileAddedAtByPath ?? [];
        var rebasedAddedAt = new Dictionary<string, DateTimeOffset>(
            StringComparer.OrdinalIgnoreCase);
        foreach ((string path, DateTimeOffset addedAt) in currentAddedAt)
        {
            rebasedAddedAt[RebasePath(path,oldRoot,newRoot)] = addedAt;
        }

        bool membersChanged = !StringListMapsEqual(
            currentMembers,            rebasedMembers);
        bool orderChanged = !currentOrder.SequenceEqual(
            rebasedOrder,            StringComparer.Ordinal);
        bool addedAtChanged = !StringDateMapsEqual(
            currentAddedAt,            rebasedAddedAt);
        if (!membersChanged && !orderChanged && !addedAtChanged)
        {
            return false;
        }

        if (membersChanged)
        {
            SetStackMemberOverrides(config,rebasedMembers);
        }

        if (orderChanged)
        {
            SetStackOrder(config,rebasedOrder);
        }

        if (addedAtChanged)
        {
            config.FileAddedAtByPath = rebasedAddedAt;
        }

        return true;
    }

    /// <summary>
    /// Removes display metadata for manual stacks that no longer have enough
    /// persisted members to exist. Automatic group customizations are retained
    /// because those groups can legitimately disappear and return later.
    /// </summary>
    public static bool PruneOrphanedManualStackMetadata(WidgetConfig config)
    {
        Dictionary<string, List<string>> members =
            GetStackMemberOverrides(config);
        HashSet<string> activeManualKeys = members
            .Where(entry =>
                entry.Key.StartsWith(
                    ManualStackKeyPrefix,                    StringComparison.Ordinal) &&
                entry.Value.Count >= 2)
            .Select(entry => entry.Key)
            .ToHashSet(StringComparer.Ordinal);

        bool changed = false;
        string[] incompleteManualKeys = members
            .Where(entry =>
                entry.Key.StartsWith(
                    ManualStackKeyPrefix,                    StringComparison.Ordinal) &&
                !activeManualKeys.Contains(entry.Key))
            .Select(entry => entry.Key)
            .ToArray();
        foreach (string key in incompleteManualKeys)
        {
            changed |= members.Remove(key);
        }

        Dictionary<string, string> names = GetStackNameOverrides(config);
        foreach (string key in names.Keys
                     .Where(key => IsOrphanedManualKey(key,activeManualKeys))
                     .ToArray())
        {
            changed |= names.Remove(key);
        }

        HashSet<string> disabled = GetDisabledStacks(config);
        changed |= disabled.RemoveWhere(key =>
            IsOrphanedManualKey(key,activeManualKeys)) > 0;

        List<string> order = GetStackOrder(config);
        List<string> prunedOrder = order
            .Where(key => !IsOrphanedManualKey(key,activeManualKeys))
            .ToList();
        changed |= prunedOrder.Count != order.Count;
        if (!changed)
        {
            return false;
        }

        SetStackMemberOverrides(config,members);
        SetStackNameOverrides(config,names);
        SetDisabledStacks(config,disabled);
        SetStackOrder(config,prunedOrder);
        return true;
    }

    private static bool IsOrphanedManualKey(
        string key,        IReadOnlySet<string> activeManualKeys) =>
        key.StartsWith(ManualStackKeyPrefix,StringComparison.Ordinal) &&
        !activeManualKeys.Contains(key);

    private static string RebaseLooseItemOrderKey(
        string key,        string oldRoot,        string newRoot)
    {
        if (!key.StartsWith(LooseItemOrderKeyPrefix,StringComparison.Ordinal))
        {
            return key;
        }

        string path = key[LooseItemOrderKeyPrefix.Length..];
        string rebasedPath = RebasePath(path,oldRoot,newRoot);
        return string.Equals(path,rebasedPath,StringComparison.OrdinalIgnoreCase)
            ? key
            : LooseItemOrderKeyPrefix + rebasedPath.ToUpperInvariant();
    }

    private static string RebasePath(
        string path,        string oldRoot,        string newRoot)
    {
        if (string.IsNullOrWhiteSpace(path))
        {
            return path;
        }

        string fullPath;
        try
        {
            fullPath = Path.GetFullPath(path);
        }
        catch
        {
            return path;
        }

        if (string.Equals(fullPath,oldRoot,StringComparison.OrdinalIgnoreCase))
        {
            return newRoot;
        }

        string oldPrefix = oldRoot + Path.DirectorySeparatorChar;
        if (!fullPath.StartsWith(oldPrefix,StringComparison.OrdinalIgnoreCase))
        {
            return path;
        }

        return Path.Combine(newRoot,fullPath[oldPrefix.Length..]);
    }

    private static string NormalizeRootPath(string path) =>
        Path.GetFullPath(path)
            .TrimEnd(Path.DirectorySeparatorChar,Path.AltDirectorySeparatorChar);

    private static bool StringListMapsEqual(
        IReadOnlyDictionary<string, List<string>> left,        IReadOnlyDictionary<string, List<string>> right)
    {
        if (left.Count != right.Count)
        {
            return false;
        }

        return left.All(entry =>
            right.TryGetValue(entry.Key,out List<string>? values) &&
            entry.Value.SequenceEqual(values,StringComparer.OrdinalIgnoreCase));
    }

    private static bool StringDateMapsEqual(
        IReadOnlyDictionary<string, DateTimeOffset> left,        IReadOnlyDictionary<string, DateTimeOffset> right)
    {
        if (left.Count != right.Count)
        {
            return false;
        }

        return left.All(entry =>
            right.TryGetValue(entry.Key,out DateTimeOffset value) &&
            value == entry.Value);
    }

    private static List<string> ReadStringList(
        WidgetConfig config,        string key)
    {
        if (config.Metadata is null ||
            !config.Metadata.TryGetValue(key,out string? json) ||
            string.IsNullOrWhiteSpace(json))
        {
            return [];
        }

        try
        {
            return JsonSerializer.Deserialize(
                       json,                       WidgetMetadataJsonContext.Default.StringList) is
                { } list
                ? list
                    .Where(value =>
                        !string.IsNullOrWhiteSpace(value))
                    .Distinct(StringComparer.Ordinal)
                    .ToList()
                : [];
        }
        catch (JsonException)
        {
            return [];
        }
    }

    private static HashSet<string> ReadStringCollection(WidgetConfig config,string key)
    {
        if (config.Metadata is null ||
            !config.Metadata.TryGetValue(key,out string? json) ||
            string.IsNullOrWhiteSpace(json))
        {
            return new HashSet<string>(StringComparer.Ordinal);
        }

        try
        {
            return JsonSerializer.Deserialize(
                       json,                       WidgetMetadataJsonContext.Default.StringList) is { } list
                ? new HashSet<string>(list,StringComparer.Ordinal)
                : new HashSet<string>(StringComparer.Ordinal);
        }
        catch (JsonException)
        {
            return new HashSet<string>(StringComparer.Ordinal);
        }
    }

    private static void WriteStringCollection(WidgetConfig config,string key,IEnumerable<string> values)
    {
        config.Metadata ??= [];
        var list = values
            .Where(value => !string.IsNullOrWhiteSpace(value))
            .Distinct(StringComparer.Ordinal)
            .ToList();
        if (list.Count == 0)
        {
            config.Metadata.Remove(key);
            return;
        }

        config.Metadata[key] = JsonSerializer.Serialize(
            list,            WidgetMetadataJsonContext.Default.StringList);
    }

    public static bool NormalizeOverrides(WidgetConfig config)
    {
        if (config.Metadata is null)
        {
            return false;
        }

        bool changed = NormalizeOverride(
            config,            EnabledOverrideMetadataKey,            GetEnabledOverride(config)?.ToString());
        changed |= NormalizeOverride(
            config,            GroupByOverrideMetadataKey,            GetGroupByOverride(config));
        changed |= NormalizeOverride(
            config,            ThresholdOverrideMetadataKey,            GetThresholdOverride(config)?.ToString());
        changed |= NormalizeOverride(
            config,            OrderByOverrideMetadataKey,            GetOrderByOverride(config));
        changed |= NormalizeOverride(
            config,            OpenModeOverrideMetadataKey,            GetOpenModeOverride(config));
        changed |= PruneOrphanedManualStackMetadata(config);
        return changed;
    }

    private static bool NormalizeOverride(
        WidgetConfig config,        string key,        string? normalizedValue)
    {
        if (!config.Metadata.TryGetValue(key,out string? currentValue))
        {
            return false;
        }

        if (normalizedValue is null)
        {
            config.Metadata.Remove(key);
            return true;
        }

        if (string.Equals(currentValue,normalizedValue,StringComparison.Ordinal))
        {
            return false;
        }

        config.Metadata[key] = normalizedValue;
        return true;
    }
}
