namespace DeskBox.Tests;

public sealed class FolderEnumerationPerfTests
{
    [Fact]
    public async Task EnumerateDirectory_LargeFolderFastPath_CompletesQuickly()
    {
        string root = Path.Combine(Path.GetTempPath(), "deskbox-enum-perf", Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(root);
        try
        {
            for (int index = 0; index < 2100; index++)
            {
                File.WriteAllText(Path.Combine(root, $"img-{index:D5}.png"), "x");
            }

            var service = new DeskBox.Services.FileService();
            var stopwatch = System.Diagnostics.Stopwatch.StartNew();
            var result = await service.EnumerateDirectoryForRefreshAsync(
                root,
                loadIcons: false,
                loadFolderItemCounts: false);
            stopwatch.Stop();

            Assert.True(
                DeskBox.Services.FolderSnapshotStatusPolicy.IsSuccessful(result.Status),
                $"enumeration status was {result.Status}");
            Assert.Equal(2100, result.Items.Count);
            // The directory-stream rewrite targets ~30 ms for 2100 entries
            // (the per-entry re-stat version measured ~208 ms); 800 ms keeps
            // generous margin for CI variance while still catching stat
            // storms.
            Assert.True(
                stopwatch.ElapsedMilliseconds < 800,
                $"enumeration of 2100 synthetic files took {stopwatch.ElapsedMilliseconds} ms");
        }
        finally
        {
            Directory.Delete(root, recursive: true);
        }
    }
}
