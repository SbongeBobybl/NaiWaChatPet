using DeskBox.Models;

namespace DeskBox.Services;

public sealed class DesktopOrganizationRuleResolver
{
    public DesktopOrganizationRule? Resolve(
        DesktopOrganizationFileSnapshot item,        IEnumerable<DesktopOrganizationRule> rules,        IReadOnlyCollection<WidgetConfig> widgets)
    {
        var validWidgetIds = widgets
            .Where(widget =>
                widget.WidgetKind == WidgetKind.File &&
                !widget.IsDisabled &&
                !string.IsNullOrWhiteSpace(widget.MappedFolderPath))
            .Select(widget => widget.Id)
            .ToHashSet(StringComparer.Ordinal);

        var candidates = rules
            .Where(rule => rule.IsEnabled && validWidgetIds.Contains(rule.TargetWidgetId))
            .Where(rule => !NormalizeExtensions(rule.ExcludedExtensions).Contains(item.Extension))
            .Select(rule => new
            {
                Rule = rule,
                Rank = GetMatchRank(item,rule)
            })
            .Where(candidate => candidate.Rank > 0)
            .OrderByDescending(candidate => candidate.Rank)
            .ThenBy(candidate => candidate.Rule.Id,StringComparer.Ordinal)
            .ToList();

        return candidates.FirstOrDefault()?.Rule;
    }

    public IReadOnlyList<DesktopOrganizationRuleConflict> FindConflicts(
        IEnumerable<DesktopOrganizationRule> rules)
    {
        var enabledRules = rules.Where(rule => rule.IsEnabled).ToList();
        var conflicts = new List<DesktopOrganizationRuleConflict>();

        AddConflicts(
            enabledRules,            rule => NormalizeExtensions(rule.Extensions),            DesktopOrganizationRuleConflictKind.Extension,            conflicts);
        AddConflicts(
            enabledRules,            rule => rule.SubtypeIds.Where(value => !string.IsNullOrWhiteSpace(value)),            DesktopOrganizationRuleConflictKind.Subtype,            conflicts);
        AddConflicts(
            enabledRules,            rule => rule.CategoryIds.Where(value => !string.IsNullOrWhiteSpace(value)),            DesktopOrganizationRuleConflictKind.Category,            conflicts);

        return conflicts;
    }

    public void AssignExtensionExclusively(
        IList<DesktopOrganizationRule> rules,        string targetWidgetId,        string extension)
    {
        string normalized = DesktopOrganizationClassifier.NormalizeExtension(extension);
        if (string.IsNullOrEmpty(normalized))
        {
            return;
        }

        foreach (DesktopOrganizationRule rule in rules)
        {
            rule.Extensions = NormalizeExtensions(rule.Extensions)
                .Where(value =>
                    string.Equals(rule.TargetWidgetId,targetWidgetId,StringComparison.Ordinal) ||
                    !string.Equals(value,normalized,StringComparison.OrdinalIgnoreCase))
                .ToList();
        }

        DesktopOrganizationRule target = rules.FirstOrDefault(rule =>
            string.Equals(rule.TargetWidgetId,targetWidgetId,StringComparison.Ordinal)) ??
            AddRule(rules,targetWidgetId);

        if (!target.Extensions.Contains(normalized,StringComparer.OrdinalIgnoreCase))
        {
            target.Extensions.Add(normalized);
        }
    }

    private static int GetMatchRank(
        DesktopOrganizationFileSnapshot item,        DesktopOrganizationRule rule)
    {
        if (NormalizeExtensions(rule.Extensions).Contains(item.Extension))
        {
            return 3;
        }

        if (!string.IsNullOrWhiteSpace(item.SubtypeId) &&
            rule.SubtypeIds.Contains(item.SubtypeId,StringComparer.Ordinal))
        {
            return 2;
        }

        return rule.CategoryIds.Contains(item.CategoryId,StringComparer.Ordinal) ? 1 : 0;
    }

    private static HashSet<string> NormalizeExtensions(IEnumerable<string> extensions) =>
        extensions
            .Select(DesktopOrganizationClassifier.NormalizeExtension)
            .Where(extension => !string.IsNullOrEmpty(extension))
            .ToHashSet(StringComparer.OrdinalIgnoreCase);

    private static DesktopOrganizationRule AddRule(
        IList<DesktopOrganizationRule> rules,        string targetWidgetId)
    {
        var rule = new DesktopOrganizationRule { TargetWidgetId = targetWidgetId };
        rules.Add(rule);
        return rule;
    }

    private static void AddConflicts(
        IReadOnlyCollection<DesktopOrganizationRule> rules,        Func<DesktopOrganizationRule, IEnumerable<string>> valueSelector,        DesktopOrganizationRuleConflictKind kind,        ICollection<DesktopOrganizationRuleConflict> output)
    {
        foreach (var group in rules
                     .SelectMany(rule => valueSelector(rule).Distinct(StringComparer.OrdinalIgnoreCase)
                         .Select(value => new { Value = value, Rule = rule }))
                     .GroupBy(item => item.Value,StringComparer.OrdinalIgnoreCase)
                     .Where(group => group.Select(item => item.Rule.TargetWidgetId)
                         .Distinct(StringComparer.Ordinal).Count() > 1))
        {
            output.Add(new DesktopOrganizationRuleConflict(
                kind,                group.Key,                group.Select(item => item.Rule.TargetWidgetId)
                    .Distinct(StringComparer.Ordinal)
                    .ToList()));
        }
    }
}

public enum DesktopOrganizationRuleConflictKind
{
    Category,
    Subtype,
    Extension
}

public sealed record DesktopOrganizationRuleConflict(
    DesktopOrganizationRuleConflictKind Kind,    string Value,    IReadOnlyList<string> TargetWidgetIds);
