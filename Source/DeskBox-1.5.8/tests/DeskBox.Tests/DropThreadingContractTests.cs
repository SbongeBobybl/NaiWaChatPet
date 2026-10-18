namespace DeskBox.Tests;

public sealed class DropThreadingContractTests
{
    [Fact]
    public void DropPreparationAndItemSnapshots_RunOutsideTheUiThread()
    {
        string organizer = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/OrganizerService.cs"));
        Assert.Contains(
            "Task.Run(\n            () => PrepareDrop(",
            organizer.ReplaceLineEndings("\n"),
            StringComparison.Ordinal);
        Assert.Contains(
            "Task.Run(\n                () => CreateTransferPlans(",
            organizer.ReplaceLineEndings("\n"),
            StringComparison.Ordinal);

        string fileService = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/FileService.cs")).ReplaceLineEndings("\n");
        Assert.Contains(
            "FileSystemEntrySnapshot entry = await Task.Run(\n            () => CaptureEntrySnapshot(",
            fileService,
            StringComparison.Ordinal);
        Assert.Contains(
            "FileSystemEntrySnapshot? entry = await Task.Run(() =>\n            ShouldDisplayEntry(path)",
            fileService,
            StringComparison.Ordinal);
        Assert.Contains(
            "BoundedBackgroundWorkScheduler.SharedShell.RunAsync(",
            fileService,
            StringComparison.Ordinal);
    }
}
