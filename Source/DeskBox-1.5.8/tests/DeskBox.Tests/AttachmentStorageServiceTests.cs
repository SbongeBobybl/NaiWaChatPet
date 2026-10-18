using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class AttachmentStorageServiceTests : IDisposable
{
    private readonly string _tempRoot = Path.Combine(
        Path.GetTempPath(),
        "DeskBox.Tests",
        Guid.NewGuid().ToString("N"));

    [Fact]
    public async Task ImportPathAsync_LinksOriginalPathByDefault()
    {
        Directory.CreateDirectory(_tempRoot);
        string sourcePath = Path.Combine(_tempRoot, "source.txt");
        string managedDirectory = Path.Combine(_tempRoot, "managed");
        await File.WriteAllTextAsync(sourcePath, "linked");

        TodoAttachment? attachment = await AttachmentStorageService.ImportPathAsync(
            sourcePath,
            managedDirectory,
            copyToManagedStorage: false);

        Assert.NotNull(attachment);
        Assert.Equal(Path.GetFullPath(sourcePath), attachment!.FilePath);
        Assert.Equal(TodoAttachment.LinkedStorageMode, attachment.StorageMode);
        Assert.False(Directory.Exists(managedDirectory));
    }

    [Fact]
    public async Task ImportPathAsync_CopiesFileIntoManagedStorage()
    {
        Directory.CreateDirectory(_tempRoot);
        string sourcePath = Path.Combine(_tempRoot, "source.pdf");
        string managedDirectory = Path.Combine(_tempRoot, "managed");
        await File.WriteAllBytesAsync(sourcePath, [1, 2, 3]);

        TodoAttachment? attachment = await AttachmentStorageService.ImportPathAsync(
            sourcePath,
            managedDirectory,
            copyToManagedStorage: true);

        Assert.NotNull(attachment);
        Assert.NotEqual(Path.GetFullPath(sourcePath), attachment!.FilePath);
        Assert.StartsWith(managedDirectory, attachment.FilePath, StringComparison.OrdinalIgnoreCase);
        Assert.Equal(TodoAttachment.ManagedStorageMode, attachment.StorageMode);
        Assert.Equal("pdf", attachment.Type);
        Assert.Equal([1, 2, 3], await File.ReadAllBytesAsync(attachment.FilePath));
    }

    [Fact]
    public async Task SaveStreamAsync_AlwaysCreatesManagedAttachment()
    {
        string managedDirectory = Path.Combine(_tempRoot, "managed");
        await using var stream = new MemoryStream([4, 5, 6]);

        TodoAttachment? attachment = await AttachmentStorageService.SaveStreamAsync(
            stream,
            "virtual.png",
            managedDirectory);

        Assert.NotNull(attachment);
        Assert.Equal(TodoAttachment.ManagedStorageMode, attachment!.StorageMode);
        Assert.Equal("image", attachment.Type);
        Assert.Equal([4, 5, 6], await File.ReadAllBytesAsync(attachment.FilePath));
    }

    [Fact]
    public async Task TryDeleteManagedCopyAsync_DeletesFileInsideManagedTree()
    {
        string managedDirectory = Path.Combine(_tempRoot, "managed", "item-1");
        Directory.CreateDirectory(managedDirectory);
        string managedFile = Path.Combine(managedDirectory, "copy.pdf");
        await File.WriteAllBytesAsync(managedFile, [1, 2, 3]);

        bool deleted = await AttachmentStorageService.TryDeleteManagedCopyAsync(
            managedFile,
            Path.Combine(_tempRoot, "managed"),
            recycleFileAsync: path =>
            {
                File.Delete(path);
                return Task.FromResult(true);
            });

        Assert.True(deleted);
        Assert.False(File.Exists(managedFile));
    }

    [Fact]
    public async Task TryDeleteManagedCopyAsync_KeepsFilesOutsideManagedTree()
    {
        string managedDirectory = Path.Combine(_tempRoot, "managed");
        Directory.CreateDirectory(managedDirectory);
        // A stale IsManagedCopy flag must never authorize deleting a path
        // the store does not own — e.g. the user's original file.
        string foreignFile = Path.Combine(_tempRoot, "Documents", "original.docx");
        Directory.CreateDirectory(Path.GetDirectoryName(foreignFile)!);
        await File.WriteAllBytesAsync(foreignFile, [4, 5, 6]);

        bool deleted = await AttachmentStorageService.TryDeleteManagedCopyAsync(
            foreignFile,
            managedDirectory,
            recycleFileAsync: path =>
            {
                File.Delete(path);
                return Task.FromResult(true);
            });

        Assert.False(deleted);
        Assert.True(File.Exists(foreignFile));
        Assert.Equal([4, 5, 6], await File.ReadAllBytesAsync(foreignFile));
    }

    [Fact]
    public async Task TryDeleteManagedCopyAsync_WithoutRecycleKernel_KeepsManagedFile()
    {
        // No recycle kernel (e.g. a unit-test host): the managed copy is
        // kept rather than permanently deleted — fail-safe.
        string managedDirectory = Path.Combine(_tempRoot, "managed");
        Directory.CreateDirectory(managedDirectory);
        string managedFile = Path.Combine(managedDirectory, "copy.pdf");
        await File.WriteAllBytesAsync(managedFile, [1, 2, 3]);

        bool deleted = await AttachmentStorageService.TryDeleteManagedCopyAsync(
            managedFile,
            managedDirectory,
            recycleFileAsync: null);

        Assert.False(deleted);
        Assert.True(File.Exists(managedFile));
    }

    public void Dispose()
    {
        if (Directory.Exists(_tempRoot))
        {
            Directory.Delete(_tempRoot, recursive: true);
        }
    }
}
