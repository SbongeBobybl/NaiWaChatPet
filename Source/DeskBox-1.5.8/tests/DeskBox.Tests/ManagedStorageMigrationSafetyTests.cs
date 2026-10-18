using System.Collections.Concurrent;
using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

/// <summary>
/// Safety contracts for the storage-migration source cleanup: nothing that
/// was not verifiably copied may ever be deleted, read-only files must not
/// fail a move, and a cleanup failure must complete the migration with a
/// visible residue instead of rolling back onto a gutted source folder.
/// </summary>
public sealed class ManagedStorageMigrationSafetyTests : IDisposable
{
    private readonly string _tempRoot;
    private readonly string _storageRoot;
    private readonly string _newStorageRoot;
    private readonly SettingsService _settingsService;
    private readonly FileService _fileService;
    private readonly WidgetManager _widgetManager;

    public ManagedStorageMigrationSafetyTests()
    {
        _tempRoot = Path.Combine(Path.GetTempPath(), "DeskBox.Tests", Guid.NewGuid().ToString("N"));
        _storageRoot = Directory.CreateDirectory(Path.Combine(_tempRoot, "storage")).FullName;
        _newStorageRoot = Path.Combine(_tempRoot, "storage-new");

        _settingsService = new SettingsService(Path.Combine(_tempRoot, "settings"));
        _settingsService.Settings.DefaultManagedStorageRootPath = _storageRoot;

        _fileService = new FileService();
        _widgetManager = new WidgetManager(
            _settingsService,
            _fileService,
            TestOrganizerServices.Create(_settingsService, _fileService),
            new ThemeService(_settingsService),
            new QuickCaptureService(new QuickCaptureStore(Path.Combine(_tempRoot, "quick-capture"))),
            () => Path.Combine(_tempRoot, "desktop"),
            recycleManagedFolderDeletes: false);
    }

    public void Dispose()
    {
        // Tests intentionally leave read-only files behind (copied
        // attributes), which block a plain recursive delete.
        foreach (string filePath in Directory.EnumerateFiles(
                     _tempRoot,
                     "*",
                     SearchOption.AllDirectories))
        {
            File.SetAttributes(filePath, FileAttributes.Normal);
        }

        Directory.Delete(_tempRoot, recursive: true);
    }

    [Theory]
    [InlineData(false)]
    [InlineData(true)]
    public async Task ExecuteTransferPlanAsync_DirectoryMoveWithReadOnlyNestedFileSucceeds(
        bool reportProgress)
    {
        string sourceDirectory = Directory.CreateDirectory(
            Path.Combine(_tempRoot, $"ro-source-{reportProgress}")).FullName;
        string nestedDirectory = Directory.CreateDirectory(
            Path.Combine(sourceDirectory, "documents")).FullName;
        string readOnlyFile = Path.Combine(nestedDirectory, "report.docx");
        File.WriteAllText(readOnlyFile, "attachment content");
        File.SetAttributes(readOnlyFile, FileAttributes.ReadOnly);
        string destinationDirectory = Directory.CreateDirectory(
            Path.Combine(_tempRoot, $"ro-destination-{reportProgress}")).FullName;

        IProgress<FileService.FileTransferProgress>? progress = reportProgress
            ? new InlineProgress<FileService.FileTransferProgress>(_ => { })
            : null;

        await _fileService.ExecuteTransferPlanAsync(
            [new FileService.FileTransferPlan(sourceDirectory, destinationDirectory)],
            move: true,
            progress: progress);

        Assert.False(Directory.Exists(sourceDirectory));
        string copiedFile = Path.Combine(destinationDirectory, "documents", "report.docx");
        Assert.Equal("attachment content", File.ReadAllText(copiedFile));
        Assert.True(
            File.GetAttributes(copiedFile).HasFlag(FileAttributes.ReadOnly),
            "The copy must preserve the source read-only attribute.");
    }

    [Fact]
    public void DeleteSourceTreeByManifest_ThrowsAndDeletesNothingWhenFileAppearedAfterCopy()
    {
        string sourceDirectory = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "toctou-source")).FullName;
        string copiedFile = Path.Combine(sourceDirectory, "copied.txt");
        string lateFile = Path.Combine(sourceDirectory, "late.txt");
        File.WriteAllText(copiedFile, "copied before the delete");
        File.WriteAllText(lateFile, "written after the copy enumeration");

        var manifest = new List<FileService.CopiedSourceFileRecord>
        {
            new(copiedFile, new FileInfo(copiedFile).Length, new FileInfo(copiedFile).LastWriteTimeUtc, null)
        };

        Assert.Throws<FileService.FileTransferSourceChangedException>(
            () => FileService.DeleteSourceTreeByManifest(
                sourceDirectory,
                Path.Combine(_tempRoot, "toctou-destination"),
                manifest));

        Assert.True(File.Exists(lateFile), "A file that appeared after the copy must survive.");
        Assert.True(File.Exists(copiedFile), "Verification failure must not delete anything.");
        Assert.True(Directory.Exists(sourceDirectory));
    }

    [Fact]
    public void DeleteSourceTreeByManifest_ThrowsWhenCopiedFileChangedAfterCopy()
    {
        string sourceDirectory = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "changed-source")).FullName;
        string changedFile = Path.Combine(sourceDirectory, "changed.txt");
        File.WriteAllText(changedFile, "stale copy captured");

        var manifest = new List<FileService.CopiedSourceFileRecord>
        {
            new(changedFile, 1, DateTime.UtcNow - TimeSpan.FromHours(1), null)
        };

        Assert.Throws<FileService.FileTransferSourceChangedException>(
            () => FileService.DeleteSourceTreeByManifest(
                sourceDirectory,
                Path.Combine(_tempRoot, "changed-destination"),
                manifest));

        Assert.True(File.Exists(changedFile), "A modified source file must survive.");
    }

    [Fact]
    public void DeleteSourceTreeByManifest_ClearsReadOnlyAndDeletesWholeTree()
    {
        string sourceDirectory = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "manifest-source")).FullName;
        string nestedDirectory = Directory.CreateDirectory(
            Path.Combine(sourceDirectory, "nested", "deeper")).FullName;
        string readOnlyTop = Path.Combine(sourceDirectory, "ro-top.txt");
        string readOnlyNested = Path.Combine(nestedDirectory, "ro-nested.txt");
        string plainFile = Path.Combine(sourceDirectory, "plain.txt");
        File.WriteAllText(readOnlyTop, "a");
        File.WriteAllText(readOnlyNested, "b");
        File.WriteAllText(plainFile, "c");
        File.SetAttributes(readOnlyTop, FileAttributes.ReadOnly);
        File.SetAttributes(readOnlyNested, FileAttributes.ReadOnly);

        FileService.DeleteSourceTreeByManifest(
            sourceDirectory,
            Path.Combine(_tempRoot, "manifest-destination"),
            FileService.CollectCurrentFileManifest(sourceDirectory));

        Assert.False(Directory.Exists(sourceDirectory), "The fully verified tree must be removed.");
    }

    [Fact]
    public async Task DeleteSourceTreeByManifest_LockedReadOnlyFileKeepsFileAndAttribute()
    {
        string sourceDirectory = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "locked-source")).FullName;
        string lockedFile = Path.Combine(sourceDirectory, "locked.txt");
        File.WriteAllText(lockedFile, "locked content");
        File.SetAttributes(lockedFile, FileAttributes.ReadOnly);

        await using var lockHandle = new FileStream(
            lockedFile,
            FileMode.Open,
            FileAccess.Read,
            FileShare.Read);

        // A file locked by another process cannot be deleted through a
        // verified handle: fail closed as a cleanup failure (both copies
        // stay) instead of a raw delete error.
        Assert.Throws<FileService.FileTransferSourceCleanupException>(
            () => FileService.DeleteSourceTreeByManifest(
                sourceDirectory,
                Path.Combine(_tempRoot, "locked-destination"),
                FileService.CollectCurrentFileManifest(sourceDirectory)));

        Assert.True(File.Exists(lockedFile));
        Assert.True(
            File.GetAttributes(lockedFile).HasFlag(FileAttributes.ReadOnly),
            "A failed delete must restore the original read-only attribute.");
    }

    [Fact]
    public async Task RestoreMigratedDirectoryPreservingExisting_MovesOnlyMissingChildren()
    {
        string copiedDirectory = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "merge-copy")).FullName;
        string originalDirectory = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "merge-original")).FullName;
        string twinFile = Path.Combine(copiedDirectory, "twin.txt");
        string twinOriginal = Path.Combine(originalDirectory, "twin.txt");
        string uniqueFile = Path.Combine(copiedDirectory, "unique.txt");
        string matchingFile = Path.Combine(copiedDirectory, "matching.txt");
        string matchingOriginal = Path.Combine(originalDirectory, "matching.txt");
        File.WriteAllText(twinFile, "stale copy");
        File.WriteAllText(twinOriginal, "possibly newer original");
        File.WriteAllText(uniqueFile, "only in the copy");
        File.WriteAllText(matchingFile, "identical twin");
        File.WriteAllText(matchingOriginal, "identical twin");
        DateTime stamp = DateTime.UtcNow.AddDays(-1);
        File.SetLastWriteTimeUtc(matchingFile, stamp);
        File.SetLastWriteTimeUtc(matchingOriginal, stamp);

        await FileService.RestoreMigratedDirectoryPreservingExistingAsync(
            copiedDirectory,
            originalDirectory);

        Assert.Equal(
            "possibly newer original",
            File.ReadAllText(twinOriginal));
        Assert.Equal(
            "only in the copy",
            File.ReadAllText(Path.Combine(originalDirectory, "unique.txt")));
        Assert.True(
            File.Exists(twinFile),
            "A diverged duplicate is kept: either side may hold the user's " +
            "latest edit, and ownership cannot be proven without a match.");
        Assert.True(
            File.Exists(matchingFile),
            "Duplicates are never auto-deleted: content equality from size " +
            "and timestamp cannot authorize a delete in a rollback.");
        Assert.True(Directory.Exists(copiedDirectory), "The copy stays for its duplicate content.");
    }

    [Fact]
    public async Task RestoreMigratedDirectory_NestedSameNamedDirectoriesMergeConservatively()
    {
        // The failed source cleanup already deleted some files from the
        // original subtree; the copied subtree holds the ONLY remaining copy
        // of those files. A same-named nested directory must merge
        // child-by-child, never delete the copied subtree wholesale.
        string copiedDirectory = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "nested-copy")).FullName;
        string originalDirectory = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "nested-original")).FullName;
        string copiedSub = Directory.CreateDirectory(
            Path.Combine(copiedDirectory, "documents")).FullName;
        string originalSub = Directory.CreateDirectory(
            Path.Combine(originalDirectory, "documents")).FullName;
        // Original side after partial cleanup: only b.txt remains.
        File.WriteAllText(Path.Combine(originalSub, "b.txt"), "b");
        // Copied side: the complete tree (a.txt deleted from original).
        File.WriteAllText(Path.Combine(copiedSub, "a.txt"), "a");
        File.WriteAllText(Path.Combine(copiedSub, "b.txt"), "b");

        await FileService.RestoreMigratedDirectoryPreservingExistingAsync(
            copiedDirectory,
            originalDirectory);

        Assert.True(
            File.Exists(Path.Combine(originalSub, "a.txt")),
            "a.txt must move back: the copied subtree held its last copy");
        Assert.True(
            File.Exists(Path.Combine(originalSub, "b.txt")),
            "b.txt survives on the original side");
        Assert.True(
            File.Exists(Path.Combine(copiedSub, "b.txt")),
            "the duplicate b.txt stays: duplicates are never auto-deleted");
    }

    private sealed class InlineProgress<T>(Action<T> callback) : IProgress<T>
    {
        public void Report(T value) => callback(value);
    }
}