using DeskBox.Models;
using DeskBox.Services;
using System.Text.Json;

namespace DeskBox.Tests;

public sealed class DeskBoxAttachmentHealthServiceTests : IDisposable
{
    private readonly string _tempRoot;
    private readonly string _dataRoot;

    public DeskBoxAttachmentHealthServiceTests()
    {
        _tempRoot = Path.Combine(Path.GetTempPath(), "DeskBox.Tests", Guid.NewGuid().ToString("N"));
        _dataRoot = Directory.CreateDirectory(Path.Combine(_tempRoot, "data")).FullName;
    }

    [Fact]
    public async Task ScanAsync_ReportsMissingAndOrphanedAttachments()
    {
        string quickCaptureRoot = Directory.CreateDirectory(
            Path.Combine(_dataRoot, "quick-capture")).FullName;
        string managedRoot = Directory.CreateDirectory(
            Path.Combine(quickCaptureRoot, "attachments", "note")).FullName;
        string managedExisting = Path.Combine(managedRoot, "existing.txt");
        string orphan = Path.Combine(managedRoot, "orphan.txt");
        string missingManaged = Path.Combine(managedRoot, "missing.txt");
        string missingLinked = Path.Combine(_tempRoot, "external-missing.txt");
        await File.WriteAllTextAsync(managedExisting, "referenced");
        await File.WriteAllTextAsync(orphan, "orphaned");
        var store = new QuickCaptureStore(quickCaptureRoot);
        await store.SaveAsync(new QuickCaptureStoreData
        {
            Items =
            [
                CreateItem(
                    new TodoAttachment
                    {
                        FilePath = managedExisting,
                        StorageMode = TodoAttachment.ManagedStorageMode
                    },
                    new TodoAttachment
                    {
                        FilePath = missingManaged,
                        StorageMode = TodoAttachment.ManagedStorageMode
                    },
                    new TodoAttachment
                    {
                        FilePath = missingLinked,
                        StorageMode = TodoAttachment.LinkedStorageMode
                    })
            ]
        });
        var service = new DeskBoxAttachmentHealthService(_dataRoot);

        DeskBoxAttachmentHealthReport report = await service.ScanAsync();

        Assert.Equal(3, report.ReferencedFileCount);
        Assert.Equal(missingLinked, Assert.Single(report.MissingLinkedFiles), ignoreCase: true);
        Assert.Equal(missingManaged, Assert.Single(report.MissingManagedFiles), ignoreCase: true);
        Assert.Equal(orphan, Assert.Single(report.OrphanManagedFiles), ignoreCase: true);
        Assert.Equal(0, report.UnreadableStoreCount);
        Assert.False(report.IsHealthy);
    }

    [Fact]
    public async Task ScanAsync_ReportsUnreadableTodoStoreWithoutStoppingOtherChecks()
    {
        string todoRoot = Directory.CreateDirectory(
            Path.Combine(_dataRoot, "widgets", "todo-widget")).FullName;
        await File.WriteAllTextAsync(Path.Combine(todoRoot, "todo.json"), "{ invalid json");
        var service = new DeskBoxAttachmentHealthService(_dataRoot);

        DeskBoxAttachmentHealthReport report = await service.ScanAsync();

        Assert.Equal(1, report.UnreadableStoreCount);
        Assert.False(report.IsHealthy);
    }

    [Fact]
    public async Task ScanAsync_AcceptsMixedCasePropertiesAndStringAndLegacyNumericEnums()
    {
        string quickCaptureRoot = Directory.CreateDirectory(
            Path.Combine(_dataRoot, "quick-capture")).FullName;
        string todoRoot = Directory.CreateDirectory(
            Path.Combine(_dataRoot, "widgets", "todo-widget")).FullName;
        string missingLinked = Path.Combine(_tempRoot, "mixed-linked.txt");
        string missingManaged = Path.Combine(todoRoot, "attachments", "mixed-managed.txt");
        string missingLinkedJson = JsonSerializer.Serialize(missingLinked);
        string missingManagedJson = JsonSerializer.Serialize(missingManaged);

        await File.WriteAllTextAsync(
            Path.Combine(quickCaptureRoot, "quick-capture.json"),
            $$"""
            {
              "VERSION": 4,
              "cUrReNtViEw": "Pinned",
              "ITEMS": [
                {
                  "ID": "mixed-note",
                  "tYpE": 1,
                  "APPEARANCEPRESET": "Paper",
                  "SOURCEKIND": "Clipboard",
                  "ATTACHMENTS": [
                    {
                      "FILEPATH": {{missingLinkedJson}},
                      "STORAGEMODE": "linked",
                      "FUTUREATTACHMENTFIELD": true
                    }
                  ],
                  "FUTUREITEMFIELD": "ignored"
                }
              ],
              "FUTUREROOTFIELD": { "ignored": true }
            }
            """);
        await File.WriteAllTextAsync(
            Path.Combine(todoRoot, "todo.json"),
            $$"""
            {
              "VERSION": 3,
              "iTeMs": [
                {
                  "ID": "mixed-task",
                  "TEXT": "Mixed case task",
                  "ATTACHMENTS": [
                    {
                      "FILEPATH": {{missingManagedJson}},
                      "STORAGEMODE": "managed",
                      "FUTUREATTACHMENTFIELD": true
                    }
                  ],
                  "FUTUREITEMFIELD": "ignored"
                }
              ],
              "FUTUREROOTFIELD": true
            }
            """);
        var service = new DeskBoxAttachmentHealthService(_dataRoot);

        DeskBoxAttachmentHealthReport report = await service.ScanAsync();

        Assert.Equal(2, report.ReferencedFileCount);
        Assert.Equal(missingLinked, Assert.Single(report.MissingLinkedFiles), ignoreCase: true);
        Assert.Equal(missingManaged, Assert.Single(report.MissingManagedFiles), ignoreCase: true);
        Assert.Equal(0, report.UnreadableStoreCount);
    }

    private static QuickCaptureItem CreateItem(params TodoAttachment[] attachments)
    {
        return new QuickCaptureItem
        {
            Id = "note",
            Body = "Note with attachments",
            Attachments = attachments.ToList()
        };
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
        }
    }
}
