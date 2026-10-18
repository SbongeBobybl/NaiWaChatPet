using DeskBox.Models;
using DeskBox.Services;
using DeskBox.ViewModels;
using Microsoft.UI.Xaml;

namespace DeskBox.Tests;

/// <summary>
/// Pins the release/acquire pairing for item icon references (audit
/// 2026-10-03 §7.3, regression 2026-10-13): item models kept every
/// LRU-evicted icon bitmap pinned for their lifetime, so a large grid held
/// tens of MB the icon cache believed it released. The release side nulls
/// only icons nothing displays — the rendered prefix plus the collapsed
/// stack preview members — and every display transition (render-window
/// reconcile, window growth, popover open, parked-member reuse, window
/// reveal) owns an acquisition half through HealReleasedItemIconsIfIdle or
/// StartItemHydration. A release without a paired acquisition is how folded
/// stack members permanently lost their icons after a group switch: the
/// growth-trigger hydration used to be skipped entirely under the stack
/// projection.
/// </summary>
public sealed class FileItemIconLongHiddenReleaseContractTests
{
    [Fact]
    public void ViewModel_ReleasesOnlyIconsNothingDisplays()
    {
        string hydration = ReadRepositoryFile(
            "src/DeskBox/ViewModels/WidgetViewModel.ItemHydration.cs");

        Assert.Contains(
            "internal int ReleaseUnrenderedItemIcons()",
            hydration,
            StringComparison.Ordinal);
        // The displayed set is the keep-list: the rendered prefix plus what
        // realized stack tiles preview (Members[0..3]) must keep their icons.
        Assert.Contains(
            "var displayed = ComputeDisplayedIconKeepSet(RenderedItems);",
            hydration,
            StringComparison.Ordinal);
        Assert.Contains(
            "item.Icon is not null && !displayed.Contains(item)",
            hydration,
            StringComparison.Ordinal);
    }

    [Fact]
    public void KeepSet_RetainsCollapsedStackPreviewMembersButNotTheTail()
    {
        WidgetItem[] members = CreateMembers(6);
        var stack = new WidgetStackItem
        {
            Category = WidgetStackCategory.Documents,
            StackKey = "Kind:Documents"
        };
        UpdateMembers(stack, members);
        var loose = new WidgetItem { Name = "Loose", Path = "C:\\Loose.txt" };

        HashSet<WidgetItem> keepSet = WidgetViewModel.ComputeDisplayedIconKeepSet(
            [stack, loose]);

        Assert.Contains(stack, keepSet);
        Assert.Contains(loose, keepSet);
        for (int previewIndex = 0; previewIndex < 4; previewIndex++)
        {
            Assert.Contains(members[previewIndex], keepSet);
        }

        // Members past the preview quartet are not displayed by a collapsed
        // tile; they re-acquire when the stack expands or the popover opens.
        Assert.DoesNotContain(members[4], keepSet);
        Assert.DoesNotContain(members[5], keepSet);
    }

    [Fact]
    public void FileAdapter_ReleasesUnrenderedIconsAtTheModelLevelOnly()
    {
        string adapter = ReadRepositoryFile(
            "src/DeskBox/Controls/WidgetContents/FileWidgetContentAdapter.cs");

        // The call goes straight to the view model: the file surface must not
        // grow a long-hidden teardown path (pinned by
        // WidgetVisualActivityContractTests).
        Assert.Contains(
            "public override void OnWindowLongHidden()",
            adapter,
            StringComparison.Ordinal);
        Assert.Contains(
            "ViewModel.ReleaseUnrenderedItemIcons();",
            adapter,
            StringComparison.Ordinal);

        string surface = ReadRepositoryFile(
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.xaml.cs");
        Assert.DoesNotContain(
            "OnWindowLongHidden()",
            surface,
            StringComparison.Ordinal);
    }

    [Fact]
    public void GroupContentParking_NotifiesLongHiddenSoParkedMembersReleaseToo()
    {
        // TryRetainGroupContent parks an inactive member while the host is
        // still visible; that call is the parked member's only long-hidden
        // notification, so losing it would keep parked tails pinned.
        string switching = ReadRepositoryFile(
            "src/DeskBox/Views/ContentWidgetWindow.ContentSwitching.cs");

        Assert.Contains(
            "content.OnWindowLongHidden();",
            switching,
            StringComparison.Ordinal);
    }

    [Fact]
    public void DisplayTransitions_OwnTheAcquisitionHalfOfThePairing()
    {
        string hydration = ReadRepositoryFile(
            "src/DeskBox/ViewModels/WidgetViewModel.ItemHydration.cs");
        Assert.Contains(
            "internal void HealReleasedItemIconsIfIdle()",
            hydration,
            StringComparison.Ordinal);

        // The render-window reconcile callback is the single model-level
        // funnel for display-set changes — stack expansion folds members
        // into the projection through it.
        string windowing = ReadRepositoryFile(
            "src/DeskBox/ViewModels/WidgetViewModel.Windowing.cs");
        Assert.Contains(
            "HealReleasedItemIconsIfIdle();",
            windowing,
            StringComparison.Ordinal);
        // The growth triggers hydrate unconditionally now: the old
        // stack-projection skip is what stranded released icons, and the
        // funnel pin above is the durable guarantee of that fix.

        // The popover displays every folded member at once, so opening it
        // must acquire their icons.
        string popover = ReadRepositoryFile(
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.StackPopover.cs");
        Assert.Contains(
            "ViewModel.HealReleasedItemIconsIfIdle();",
            popover,
            StringComparison.Ordinal);

        // Parked-member reuse and window reveal are display transitions too.
        string surface = ReadRepositoryFile(
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.xaml.cs");
        Assert.Contains(
            "ViewModel.HealReleasedItemIconsIfIdle();",
            surface,
            StringComparison.Ordinal);
    }

    private static WidgetItem[] CreateMembers(int count)
    {
        var members = new WidgetItem[count];
        for (int index = 0; index < count; index++)
        {
            members[index] = new WidgetItem
            {
                Name = $"Member{index + 1}",
                Path = $"C:\\Member{index + 1}.txt"
            };
        }

        return members;
    }

    private static void UpdateMembers(
        WidgetStackItem stack,
        IReadOnlyList<WidgetItem> members)
    {
        stack.Update(
            members,
            "Documents",
            $"{members.Count} items",
            string.Empty,
            string.Empty,
            isExpanded: false,
            tileWidth: 80,
            tileHeight: 80,
            tileMargin: new Thickness(0),
            tilePadding: new Thickness(0),
            previewSize: 48,
            previewItemSize: 40,
            labelMaxWidth: 72,
            labelFontSize: 12,
            listMargin: new Thickness(0),
            listPadding: new Thickness(0),
            listIconSize: 32);
    }

    private static string ReadRepositoryFile(string relativePath) =>
        File.ReadAllText(TestPaths.FromRepository(relativePath));
}
