using DeskBox.Models;
using DeskBox.ViewModels;

namespace DeskBox.Services;

internal enum ItemDropBehavior
{
    None,
    FolderImport,
    StackImport,
    Launch
}

/// <summary>
/// Single classification of what a grid item does when files are dropped on
/// it. The XAML DragOver/Drop predicates, the native OLE hit-test
/// normalization and the shell drop description all resolve drops through
/// this policy, so the three paths cannot disagree. Adding a new item-level
/// drop behavior means changing this type and its tests, never a call site.
/// </summary>
internal static class ItemDropBehaviorPolicy
{
    internal static ItemDropBehavior Resolve(WidgetItem item)
    {
        if (item is WidgetStackItem)
        {
            return ItemDropBehavior.StackImport;
        }

        if (item.IsFolder && item.Path.Length > 0)
        {
            return ItemDropBehavior.FolderImport;
        }

        return ShortcutLaunchPolicy.EvaluateItem(item) == ShortcutLaunchDecision.Launch
            ? ItemDropBehavior.Launch
            : ItemDropBehavior.None;
    }
}
