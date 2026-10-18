using DeskBox.Models;

namespace DeskBox.Services;

/// <summary>
/// Routing rules are the only persistent "remember this category" state. They
/// always route by widget identity and are evaluated after the system, user and
/// dependency protections, so a remembered rule can never override protection.
/// </summary>
internal static class DesktopOrganizationRuleFactory
{
    public static void AddForNewTargets(
        IEnumerable<DesktopOrganizationTargetPlan> targets,        ICollection<DesktopOrganizationRule> rules)
    {
        foreach (DesktopOrganizationTargetPlan target in targets.Where(target => target.CreatesWidget))
        {
            string[] sourceCategories = target.Items
                .Select(item => item.CategoryId)
                .Where(categoryId => !string.IsNullOrWhiteSpace(categoryId))
                .Distinct(StringComparer.Ordinal)
                .ToArray();
            rules.Add(new DesktopOrganizationRule
            {
                TargetWidgetId = target.TargetWidgetId,
                CategoryIds = sourceCategories.Length > 0
                    ? sourceCategories.ToList()
                    : [target.CategoryId]
            });
        }
    }
}
