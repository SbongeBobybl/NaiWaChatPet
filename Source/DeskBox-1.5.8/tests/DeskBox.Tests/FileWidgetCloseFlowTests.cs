using DeskBox.Models;

namespace DeskBox.Tests;

public sealed class FileWidgetCloseFlowTests
{
    [Theory]
    [InlineData(true, 0, true)]   // proven-empty managed folder: close directly
    [InlineData(true, 1, false)]  // folder has entries: keep the selection flyout
    [InlineData(true, null, false)] // count failed: fail closed, keep the flyout
    [InlineData(false, 0, false)] // not a managed folder: keep the flyout
    [InlineData(false, null, false)]
    public void ShouldCloseEmptyManagedFolderDirectly_OnlyForProvenEmptyManagedFolder(
        bool canCleanupManagedStorage,
        int? managedFolderEntryCount,
        bool expected)
    {
        Assert.Equal(
            expected,
            FileWidgetCloseFlowPolicy.ShouldCloseEmptyManagedFolderDirectly(
                canCleanupManagedStorage,
                managedFolderEntryCount));
    }
}
