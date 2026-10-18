namespace DeskBox.Tests;

public sealed class MultiSelectionKeyboardContractTests
{
    [Fact]
    public void QuickCaptureDeleteKey_RoutesCustomMultiSelectionToBatchDelete()
    {
        string source = ReadRepositoryFile(
            "src/DeskBox/Controls/WidgetContents/QuickCaptureSurfaceContent.xaml.cs");

        Assert.Contains(
            "GetSelectedQuickCaptureItemsInVisibleOrder()",
            source,
            StringComparison.Ordinal);
        Assert.Contains(
            "DeleteSelectedQuickCaptureItemsAsync(",
            source,
            StringComparison.Ordinal);
    }

    [Fact]
    public void TodoDeleteKey_RoutesCopySelectionToBatchDelete()
    {
        string source = ReadRepositoryFile(
            "src/DeskBox/Controls/WidgetContents/TodoWidgetContent.ListInteraction.cs");

        Assert.Contains("e.Key == VirtualKey.Delete", source, StringComparison.Ordinal);
        Assert.Contains(
            "GetSelectedCopyItemsInVisibleOrder()",
            source,
            StringComparison.Ordinal);
        Assert.Contains(
            "DeleteSelectedItemsAsync(",
            source,
            StringComparison.Ordinal);
    }

    private static string ReadRepositoryFile(string relativePath)
    {
        var directory = new DirectoryInfo(AppContext.BaseDirectory);
        while (directory is not null &&
               !Directory.Exists(Path.Combine(directory.FullName, "src", "DeskBox")))
        {
            directory = directory.Parent;
        }

        string repositoryRoot = directory?.FullName ??
            throw new DirectoryNotFoundException();
        return File.ReadAllText(Path.Combine(
            repositoryRoot,
            relativePath.Replace('/', Path.DirectorySeparatorChar)));
    }
}
