using DeskBox.Controls;
using DeskBox.Models;
using DeskBox.Services;
using DeskBox.ViewModels;
using Windows.ApplicationModel.DataTransfer;

namespace DeskBox.Tests;

public sealed class QuickCaptureMultiDragTests
{
    [Fact]
    public void ResolveDraggedItems_UsesFullSelectionWhenAnchorIsSelected()
    {
        QuickCaptureItemViewModel first = CreateItem("first", "First");
        QuickCaptureItemViewModel second = CreateItem("second", "Second");
        QuickCaptureItemViewModel third = CreateItem("third", "Third");

        Assert.Equal(
            [first, second, third],
            QuickCaptureDragPackage.ResolveDraggedItems(
                [second],
                [first, second, third]));
        Assert.Equal(
            [second],
            QuickCaptureDragPackage.ResolveDraggedItems(
                [second],
                [first, third]));
    }

    [Fact]
    public void ResolveManualDropTargetIndex_AccountsForRemovalBeforeInsertion()
    {
        QuickCaptureItemViewModel first = CreateItem("first", "First");
        QuickCaptureItemViewModel second = CreateItem("second", "Second");
        QuickCaptureItemViewModel third = CreateItem("third", "Third");
        QuickCaptureItemViewModel fourth = CreateItem("fourth", "Fourth");
        QuickCaptureItemViewModel[] items = [first, second, third, fourth];

        Assert.Equal(
            2,
            QuickCaptureDragPackage.ResolveManualDropTargetIndex(
                items,
                second.Id,
                fourth.Id,
                insertAfter: false));
        Assert.Equal(
            3,
            QuickCaptureDragPackage.ResolveManualDropTargetIndex(
                items,
                second.Id,
                fourth.Id,
                insertAfter: true));
        Assert.Equal(
            1,
            QuickCaptureDragPackage.ResolveManualDropTargetIndex(
                items,
                fourth.Id,
                second.Id,
                insertAfter: false));
        Assert.Equal(
            1,
            QuickCaptureDragPackage.ResolveManualDropTargetIndex(
                items,
                second.Id,
                second.Id,
                insertAfter: true));
        Assert.Equal(
            -1,
            QuickCaptureDragPackage.ResolveManualDropTargetIndex(
                items,
                "missing",
                second.Id,
                insertAfter: false));
        Assert.Equal(
            -1,
            QuickCaptureDragPackage.ResolveManualDropTargetIndex(
                items,
                second.Id,
                null,
                insertAfter: false));
    }

    [Fact]
    public void TryPrepare_MultiSelectionCreatesOneBatchPayload()
    {
        QuickCaptureItemViewModel first = CreateItem("first", "First");
        QuickCaptureItemViewModel second = CreateItem("second", "Second");
        var dataPackage = new DataPackage();

        bool prepared = QuickCaptureDragPackage.TryPrepare(
            dataPackage,
            [first, second],
            TestServices.CreateLocalizationService());

        Assert.True(prepared);
        Assert.Contains(DeskBoxDragData.TextFormat, dataPackage.GetView().AvailableFormats);
        Assert.Equal(
            DataPackageOperation.Copy,
            dataPackage.RequestedOperation);
    }

    [Fact]
    public void GroupedSurface_AdvertisesExtendedSelectionAndDragHandlers()
    {
        string repositoryRoot = FindRepositoryRoot();
        string xaml = File.ReadAllText(Path.Combine(
            repositoryRoot,
            "src/DeskBox/Controls/WidgetContents/QuickCaptureSurfaceContent.xaml"));
        string source = File.ReadAllText(Path.Combine(
            repositoryRoot,
            "src/DeskBox/Controls/WidgetContents/QuickCaptureSurfaceContent.xaml.cs"));

        Assert.Contains("SelectionMode=\"Extended\"", xaml, StringComparison.Ordinal);
        Assert.Contains("AllowDrop=\"True\"", xaml, StringComparison.Ordinal);
        Assert.Contains("CanDragItems=\"True\"", xaml, StringComparison.Ordinal);
        Assert.Contains("CanReorderItems=\"False\"", xaml, StringComparison.Ordinal);
        Assert.Contains("DragItemsStarting=\"ItemsList_DragItemsStarting\"", xaml, StringComparison.Ordinal);
        Assert.Contains("QuickCaptureDragPackage.ResolveDraggedItems", source, StringComparison.Ordinal);
        Assert.Contains("ApplyTabDropAsync", source, StringComparison.Ordinal);
        Assert.Contains("ItemsList.CanReorderItems = false", source, StringComparison.Ordinal);
        Assert.Contains("ResolveManualDropTargetIndex", source, StringComparison.Ordinal);
        Assert.DoesNotContain("ResolveReorderedTargetIndex", source, StringComparison.Ordinal);
        Assert.DoesNotContain("ViewModel.Items.IndexOf(item)", source, StringComparison.Ordinal);
        Assert.Contains("MovePinnedItemToIndexAsync(", source, StringComparison.Ordinal);
        Assert.Contains("MoveItemAsync(", source, StringComparison.Ordinal);
    }

    [Fact]
    public void TodoCopy_OnlyReportsFailureWhenSetContentFails()
    {
        string repositoryRoot = FindRepositoryRoot();
        string source = File.ReadAllText(Path.Combine(
            repositoryRoot,
            "src/DeskBox/Controls/WidgetContents/TodoWidgetContent.ClipboardSelection.cs"))
            .ReplaceLineEndings("\n");

        int setContent = source.IndexOf("Clipboard.SetContent(dataPackage);", StringComparison.Ordinal);
        int flushTry = source.IndexOf("try\n        {\n            // SetContent", StringComparison.Ordinal);
        int flush = source.IndexOf("Clipboard.Flush();", StringComparison.Ordinal);
        int flushCatch = source.IndexOf("Clipboard content was set but flush failed", StringComparison.Ordinal);

        Assert.True(setContent >= 0 && setContent < flushTry);
        Assert.True(flushTry < flush && flush < flushCatch);
        Assert.Contains("DeskBoxClipboardWriteScope.MarkWrite(text: text)", source, StringComparison.Ordinal);
    }

    private static QuickCaptureItemViewModel CreateItem(string id, string body)
    {
        return new QuickCaptureItemViewModel(
            new QuickCaptureItem
            {
                Id = id,
                Body = body
            },
            TestServices.CreateLocalizationService(),
            textSize: 14,
            iconSize: 16,
            searchText: null);
    }

    private static string FindRepositoryRoot()
    {
        var directory = new DirectoryInfo(AppContext.BaseDirectory);
        while (directory is not null &&
               !Directory.Exists(Path.Combine(directory.FullName, "src", "DeskBox")))
        {
            directory = directory.Parent;
        }

        return directory?.FullName ?? throw new DirectoryNotFoundException();
    }
}
