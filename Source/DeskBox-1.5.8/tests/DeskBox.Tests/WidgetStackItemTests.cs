using DeskBox.Models;
using DeskBox.Services;
using DeskBox.ViewModels;
using Microsoft.UI.Xaml;

namespace DeskBox.Tests;

public sealed class WidgetStackItemTests
{
    [Fact]
    public void FourMemberStack_ExposesDistinctFourthPreview()
    {
        WidgetItem[] members =
        [
            new() { Name = "One", Path = "C:\\One.txt" },
            new() { Name = "Two", Path = "C:\\Two.txt" },
            new() { Name = "Three", Path = "C:\\Three.txt" },
            new() { Name = "Four", Path = "C:\\Four.txt" }
        ];
        var stack = new WidgetStackItem
        {
            Category = WidgetStackCategory.Documents,
            StackKey = "Kind:Documents"
        };

        UpdateMembers(stack, members);

        Assert.Same(members[3], stack.PreviewFour);
        Assert.Equal(Visibility.Visible, stack.FourthPreviewVisibility);

        UpdateMembers(stack, members[..3]);

        Assert.Equal(Visibility.Collapsed, stack.FourthPreviewVisibility);
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
}
