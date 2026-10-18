using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

/// <summary>
/// Orphaned todo store adoption: when a new Todo widget is created on a
/// device that holds foreign widget stores (left by a cloud restore's
/// unmapped branch), the newest one is adopted so the restored data
/// becomes visible instead of staying orphaned forever.
/// </summary>
public sealed class OrphanedTodoStoreAdoptionTests : IDisposable
{
    private readonly string _tempRoot;
    private readonly string _widgetsRoot;

    public OrphanedTodoStoreAdoptionTests()
    {
        _tempRoot = Path.Combine(Path.GetTempPath(), "DeskBox.Tests", Guid.NewGuid().ToString("N"));
        _widgetsRoot = Directory.CreateDirectory(Path.Combine(_tempRoot, "widgets")).FullName;
    }

    [Fact]
    public async Task AdoptsNewestOrphanStore_AndRebasesManagedPaths()
    {
        string oldDir = Path.Combine(_widgetsRoot, "foreign-old");
        string newDir = Path.Combine(_widgetsRoot, "new-widget");
        string attachmentDir = Directory.CreateDirectory(
            Path.Combine(oldDir, "attachments")).FullName;
        string attachmentFile = Path.Combine(attachmentDir, "note.png");
        await File.WriteAllTextAsync(attachmentFile, "png");
        var orphan = new TodoWidgetStore(_widgetsRoot, "foreign-old");
        await orphan.SaveAsync(new TodoWidgetData
        {
            Items =
            [
                new TodoItem
                {
                    Id = "restored",
                    Text = "cloud task",
                    Attachments =
                    [
                        new TodoAttachment
                        {
                            FilePath = attachmentFile,
                            StorageMode = TodoAttachment.ManagedStorageMode
                        }
                    ]
                }
            ]
        });

        // An older orphan with live items must lose to the newest one.
        var olderOrphan = new TodoWidgetStore(_widgetsRoot, "foreign-older");
        await olderOrphan.SaveAsync(new TodoWidgetData
        {
            Items = [new TodoItem { Id = "older", Text = "older task" }]
        });
        File.SetLastWriteTimeUtc(
            Path.Combine(_widgetsRoot, "foreign-old", "todo.json"),
            DateTime.UtcNow);
        File.SetLastWriteTimeUtc(
            Path.Combine(_widgetsRoot, "foreign-older", "todo.json"),
            DateTime.UtcNow.AddDays(-1));

        string? adopted = await TodoWidgetStore.TryAdoptOrphanedStoreAsync(
            _widgetsRoot,
            "new-widget",
            new HashSet<string>(StringComparer.OrdinalIgnoreCase) { "new-widget" });

        Assert.Equal("foreign-old", adopted);
        Assert.False(Directory.Exists(oldDir));
        Assert.True(Directory.Exists(newDir));
        Assert.True(File.Exists(Path.Combine(newDir, "attachments", "note.png")));

        TodoWidgetData adoptedData = await new TodoWidgetStore(_widgetsRoot, "new-widget").LoadAsync();
        TodoAttachment attachment = adoptedData.Items.Single().Attachments.Single();
        Assert.StartsWith(newDir + Path.DirectorySeparatorChar, attachment.FilePath);
        Assert.EndsWith("note.png", attachment.FilePath);

        // The older orphan is untouched — a future widget can adopt it.
        Assert.True(Directory.Exists(Path.Combine(_widgetsRoot, "foreign-older")));
    }

    [Fact]
    public async Task SkipsClaimedAndDeletedIds()
    {
        foreach (string id in new[] { "live-widget", "deleted-widget", "foreign-orphan" })
        {
            var store = new TodoWidgetStore(_widgetsRoot, id);
            await store.SaveAsync(new TodoWidgetData
            {
                Items = [new TodoItem { Id = id, Text = id }]
            });
        }

        string? adopted = await TodoWidgetStore.TryAdoptOrphanedStoreAsync(
            _widgetsRoot,
            "new-widget",
            new HashSet<string>(StringComparer.OrdinalIgnoreCase)
            {
                "new-widget", "live-widget", "deleted-widget"
            });

        Assert.Equal("foreign-orphan", adopted);
        Assert.True(Directory.Exists(Path.Combine(_widgetsRoot, "live-widget")));
        Assert.True(Directory.Exists(Path.Combine(_widgetsRoot, "deleted-widget")));
    }

    [Fact]
    public async Task SkipsEmptyAndAllDeletedStores()
    {
        var empty = new TodoWidgetStore(_widgetsRoot, "empty-orphan");
        await empty.SaveAsync(new TodoWidgetData { Items = [] });
        var tombstoned = new TodoWidgetStore(_widgetsRoot, "tombstoned-orphan");
        await tombstoned.SaveAsync(new TodoWidgetData
        {
            Items = [new TodoItem { Id = "gone", IsDeleted = true }]
        });

        string? adopted = await TodoWidgetStore.TryAdoptOrphanedStoreAsync(
            _widgetsRoot,
            "new-widget",
            new HashSet<string>(StringComparer.OrdinalIgnoreCase) { "new-widget" });

        Assert.Null(adopted);
        Assert.False(Directory.Exists(Path.Combine(_widgetsRoot, "new-widget")));
    }

    [Fact]
    public async Task ReturnsNull_WhenTargetDirIsNonEmpty()
    {
        // Defensive: never merge into or overwrite an existing directory,
        // even though a fresh widget id should never collide.
        var orphan = new TodoWidgetStore(_widgetsRoot, "foreign-orphan");
        await orphan.SaveAsync(new TodoWidgetData
        {
            Items = [new TodoItem { Id = "cloud", Text = "cloud task" }]
        });
        var target = new TodoWidgetStore(_widgetsRoot, "new-widget");
        await target.SaveAsync(new TodoWidgetData
        {
            Items = [new TodoItem { Id = "local", Text = "local task" }]
        });

        string? adopted = await TodoWidgetStore.TryAdoptOrphanedStoreAsync(
            _widgetsRoot,
            "new-widget",
            new HashSet<string>(StringComparer.OrdinalIgnoreCase) { "new-widget" });

        Assert.Null(adopted);
        Assert.True(Directory.Exists(Path.Combine(_widgetsRoot, "foreign-orphan")));
    }

    public void Dispose()
    {
        try
        {
            if (Directory.Exists(_tempRoot))
            {
                Directory.Delete(_tempRoot, recursive: true);
            }
        }
        catch
        {
            // Temp cleanup is best-effort.
        }
    }
}
