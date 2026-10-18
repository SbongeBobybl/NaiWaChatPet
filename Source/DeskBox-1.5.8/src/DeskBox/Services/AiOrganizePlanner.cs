using DeskBox.Models;

namespace DeskBox.Services;

// Builds a new preview from scanned snapshots. AI never supplies a disk path.
public static class AiOrganizePlanner
{
    // Reference collection does not read file contents or move directories, so
    // the quick physical-move byte/item limits must not hide these candidates.
    public static DesktopOrganizationPlan IncludeReferenceCandidates(DesktopOrganizationPlan source)
    {
        var existing = source.Targets.SelectMany(t => t.Items).Select(i => i.SourcePath)
            .ToHashSet(StringComparer.OrdinalIgnoreCase);
        var additional = source.SourceItems.Concat(source.ExcludedItems)
            .DistinctBy(i => i.SourcePath,StringComparer.OrdinalIgnoreCase)
            .Where(i => !existing.Contains(i.SourcePath) && i.ExclusionReason is
                DesktopOrganizationExclusionReason.None or DesktopOrganizationExclusionReason.Folder or
                DesktopOrganizationExclusionReason.SlowItem or DesktopOrganizationExclusionReason.BatchLimit or
                DesktopOrganizationExclusionReason.PublicDesktopItem or DesktopOrganizationExclusionReason.SourceNotSelected)
            .Select(i => i with { ExclusionReason = DesktopOrganizationExclusionReason.None }).ToList();
        var included = additional.Select(i => i.SourcePath).ToHashSet(StringComparer.OrdinalIgnoreCase);
        var targets = source.Targets.ToList();
        if (additional.Count > 0) targets.Add(new DesktopOrganizationTargetPlan
        {
            SourceBucketId = "naiwa:additional", TargetWidgetId = "naiwa:additional",
            SuggestedDisplayName = "其他桌面项目", CreatesWidget = true, Items = additional
        });
        return new DesktopOrganizationPlan
        {
            IsAiPlan = source.IsAiPlan, OrganizationMode = source.OrganizationMode,
            DesktopPath = source.DesktopPath, PublicDesktopPath = source.PublicDesktopPath,
            PublicDesktopUnavailable = source.PublicDesktopUnavailable,
            IncludePersonalDesktop = true, IncludePublicDesktop = !source.PublicDesktopUnavailable,
            SourceItems = source.SourceItems, StorageRootPath = source.StorageRootPath, Targets = targets,
            ExcludedItems = source.ExcludedItems.Where(i => !included.Contains(i.SourcePath)).ToList()
        };
    }
    public static AiOrganizeCategory[] DefaultCategories =>
    [new("system","系统应用"),new("development","开发工具"),new("study","学习"),     new("work","工作"),new("references","资料"),new("entertainment","娱乐"),     new("temporary","临时"),new("other","其他")];

    public static bool IsDependencyProtected(DesktopOrganizationFileSnapshot item) =>
        item.IsDirectory || item.Extension.ToLowerInvariant() is
        ".exe" or ".dll" or ".json" or ".ini" or ".config" or ".sys" or ".bat" or ".cmd" or ".ps1";

    public static string OfflineCategory(DesktopOrganizationFileSnapshot item) => item.CategoryId switch
    {
        DesktopOrganizationCategoryIds.Shortcuts => "system",        DesktopOrganizationCategoryIds.Documents or DesktopOrganizationCategoryIds.Images => "references",        DesktopOrganizationCategoryIds.Media => "entertainment",        DesktopOrganizationCategoryIds.Packages => "temporary",        _ => "other"
    };

    /// <summary>
    /// Maps an accepted intent position onto a placement preference. "keep"
    /// means "do not move an existing widget", which for a newly created
    /// reference view is the canonical automatic slot.
    /// </summary>
    public static string NormalizePosition(string? position) => position switch
    {
        "top_left" or "top_right" or "bottom_left" or "bottom_right" => position,        _ => "auto"
    };

    /// <summary>
    /// Parses the persisted "categoryId=displayName" template. Returns an empty
    /// list when any entry is invalid, so a corrupt template falls back to the
    /// built-in categories instead of producing a half-valid one.
    /// </summary>
    public static List<AiOrganizeCategory> ParseTemplate(IEnumerable<string> entries)
    {
        var parsed = new List<AiOrganizeCategory>();
        foreach (string line in entries)
        {
            int split = line.IndexOf('=');
            if (split <= 0) return [];
            string id = line[..split].Trim(), label = line[(split + 1)..].Trim();
            if (id.Length > 64 || id.Any(c => !char.IsAsciiLetterOrDigit(c) && c is not ('_' or '-'))
                || string.IsNullOrWhiteSpace(label) || label.Length > 40 || label.IndexOfAny(Path.GetInvalidFileNameChars()) >= 0
                || label is "." or ".." || parsed.Any(c => c.Id == id)) return [];
            parsed.Add(new(id,label));
        }
        return parsed.Count is >= 1 and <= 32 ? parsed : [];
    }

    public static List<string> FormatTemplate(IEnumerable<AiOrganizeCategory> categories) =>
        categories.Select(c => c.Id + "=" + c.DisplayName).ToList();

    public static DesktopOrganizationPlan Build(DesktopOrganizationPlan source,        IReadOnlyDictionary<string, string> categoryByPath,IReadOnlyList<AiOrganizeCategory> categories,        AiOrganizationMode mode = AiOrganizationMode.ViewOnly,        IReadOnlyDictionary<string, AiLayoutIntent>? intents = null)
    {
        if (categories.Count is < 1 or > 32 || categories.Select(x => x.Id).Distinct(StringComparer.Ordinal).Count() != categories.Count
            || categories.Any(c => string.IsNullOrWhiteSpace(c.Id) || string.IsNullOrWhiteSpace(c.DisplayName)
                || c.DisplayName.Length > 80 || c.DisplayName.IndexOfAny(Path.GetInvalidFileNameChars()) >= 0
                || c.DisplayName is "." or "..")) throw new ArgumentException("分类模板无效。");
        var names = categories.ToDictionary(c => c.Id,c => c.DisplayName,StringComparer.Ordinal);
        var targets = new List<DesktopOrganizationTargetPlan>();
        var excluded = source.ExcludedItems.ToList();
        var pending = new Dictionary<string, List<DesktopOrganizationFileSnapshot>>(StringComparer.Ordinal);
        foreach (var target in source.Targets)
        {
            var eligible = target.Items.Where(i => i.IsEligible && (mode != AiOrganizationMode.Move || (!IsDependencyProtected(i) && (i.SourceScope != DesktopOrganizationSourceScope.Public || source.IncludePublicDesktop)))).ToList();
            excluded.AddRange(target.Items.Except(eligible).Select(i => i with { ExclusionReason = DesktopOrganizationExclusionReason.UserChoice }));
            // Existing manual routing has authority over all model/user AI suggestions.
            if (!target.CreatesWidget && !eligible.Any(i => categoryByPath.ContainsKey(i.SourcePath)))
            {
                if (eligible.Count > 0) targets.Add(target.CloneWith(mode == AiOrganizationMode.Move ? target.TargetWidgetId : Guid.NewGuid().ToString("N"),target.SuggestedDisplayName,                    target.TargetDirectoryPath,mode != AiOrganizationMode.Move,eligible));
                continue;
            }
            foreach (var item in eligible)
            {
                string category = categoryByPath.TryGetValue(item.SourcePath,out var requested) && names.ContainsKey(requested)
                    ? requested : OfflineCategory(item);
                if (!names.ContainsKey(category)) category = categories[^1].Id;
                if (!pending.TryGetValue(category,out var list)) pending[category] = list = [];
                list.Add(item);
            }
        }
        var reserved = source.Targets.Select(t => t.TargetDirectoryPath).ToHashSet(StringComparer.OrdinalIgnoreCase);
        foreach (var category in categories)
        {
            if (!pending.TryGetValue(category.Id,out var items) || items.Count == 0) continue;
            var directory = FileService.GetAvailablePath(Path.Combine(source.StorageRootPath,category.DisplayName),reserved);
            reserved.Add(directory);
            AiLayoutIntent? intent = null;
            if (intents is not null && intents.TryGetValue(category.Id,out var found)) intent = found;
            targets.Add(new DesktopOrganizationTargetPlan
            {
                SourceBucketId = "ai:" + category.Id, CategoryId = category.Id,
                TargetWidgetId = Guid.NewGuid().ToString("N"), SuggestedDisplayName = category.DisplayName,
                TargetDirectoryPath = directory, CreatesWidget = true, Items = items,
                PositionPreference = NormalizePosition(intent?.Position ?? "top_right"),
                Collapsed = intent?.Collapsed == true,
                Locked = intent?.Locked == true
            });
        }
        return new DesktopOrganizationPlan
        {
            IsAiPlan = true, OrganizationMode = mode, DesktopPath = source.DesktopPath, PublicDesktopPath = source.PublicDesktopPath,
            PublicDesktopUnavailable = source.PublicDesktopUnavailable, IncludePersonalDesktop = source.IncludePersonalDesktop,
            IncludePublicDesktop = source.IncludePublicDesktop, SourceItems = source.SourceItems,
            StorageRootPath = source.StorageRootPath, Targets = targets, ExcludedItems = excluded
        };
    }
}
