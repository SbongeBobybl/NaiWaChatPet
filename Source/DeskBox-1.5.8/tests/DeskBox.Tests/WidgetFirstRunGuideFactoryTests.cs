using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WidgetFirstRunGuideFactoryTests : IDisposable
{
    private readonly string _dataRoot = Path.Combine(
        Path.GetTempPath(),
        "DeskBoxTests",
        Guid.NewGuid().ToString("N"));

    [Fact]
    public void QuickCaptureGuide_UsesLocalizedMarkdownCopy()
    {
        var localization = TestServices.CreateLocalizationService(
            SettingsService.LanguageChinese);

        (string title, string body) =
            WidgetFirstRunGuideFactory.CreateQuickCaptureGuide(localization);

        Assert.Equal("从这里开始记录", title);
        Assert.Contains("剪贴板", body, StringComparison.Ordinal);
        Assert.Contains("Markdown", body, StringComparison.Ordinal);
        Assert.True(WidgetFirstRunGuideFactory.ShouldSeedQuickCapture(
            new QuickCaptureStoreData()));
        Assert.False(WidgetFirstRunGuideFactory.ShouldSeedQuickCapture(
            new QuickCaptureStoreData
            {
                Items = [new QuickCaptureItem { Body = "existing" }]
            }));
    }

    [Fact]
    public void TodoGuide_CreatesTaskStepsAndDetailedNotes()
    {
        var localization = TestServices.CreateLocalizationService(
            SettingsService.LanguageChinese);

        TodoItem guide = WidgetFirstRunGuideFactory.CreateTodoGuide(localization);

        Assert.Equal("开始管理第一项任务", guide.Text);
        Assert.Equal(3, guide.Steps.Count);
        Assert.Equal("勾选这一步，完成任务", guide.Steps[0].Text);
        Assert.Contains("较长的背景、决策和执行记录写在这里", guide.Notes, StringComparison.Ordinal);
        Assert.Contains("Markdown", guide.Notes, StringComparison.Ordinal);
        Assert.True(WidgetFirstRunGuideFactory.ShouldSeedTodo(new TodoWidgetData()));
        Assert.False(WidgetFirstRunGuideFactory.ShouldSeedTodo(
            new TodoWidgetData { Items = [guide] }));
    }

    [Fact]
    public void Guides_RenderMarkdownTablesAndCodeBlocks()
    {
        var localization = TestServices.CreateLocalizationService(
            SettingsService.LanguageChinese);
        var markdown = new MarkdownDocumentService();
        (string _, string quickCaptureBody) =
            WidgetFirstRunGuideFactory.CreateQuickCaptureGuide(localization);
        TodoItem todoGuide = WidgetFirstRunGuideFactory.CreateTodoGuide(localization);

        string quickCaptureHtml = markdown.ToSafeHtml(quickCaptureBody);
        string todoHtml = markdown.ToSafeHtml(todoGuide.Notes);

        Assert.Contains("<table>", quickCaptureHtml, StringComparison.Ordinal);
        Assert.Contains("<pre><code", quickCaptureHtml, StringComparison.Ordinal);
        Assert.Contains("<table>", todoHtml, StringComparison.Ordinal);
        Assert.Contains("<pre><code", todoHtml, StringComparison.Ordinal);
        Assert.Contains("- [ ]", todoGuide.Notes, StringComparison.Ordinal);
    }

    [Fact]
    public async Task EnsureQuickCaptureGuideAsync_RestoresGuideAfterDataReset()
    {
        var localization = TestServices.CreateLocalizationService(
            SettingsService.LanguageChinese);
        var service = new QuickCaptureService(new QuickCaptureStore(_dataRoot));
        var existing = await service.AddItemAsync("existing item");

        await service.ClearAsync();

        Assert.True(await WidgetFirstRunGuideFactory.EnsureQuickCaptureGuideAsync(
            service,
            localization));
        Assert.False(await WidgetFirstRunGuideFactory.EnsureQuickCaptureGuideAsync(
            service,
            localization));

        QuickCaptureStoreData data = await service.GetDataAsync();
        // The wipe left a tombstone stub behind: only live entries count, so
        // exactly the guide is visible.
        QuickCaptureItem guide = Assert.Single(data.Items.Where(item => !item.IsDeleted));
        Assert.Equal("从这里开始记录", guide.Title);
        Assert.Equal(TextContentFormat.Markdown, guide.ContentFormat);
        // The stub stays on disk and does not block the guide from reseeding.
        QuickCaptureItem tombstone = Assert.Single(data.Items.Where(item => item.IsDeleted));
        Assert.Equal(existing.Id, tombstone.Id);
        Assert.True(string.IsNullOrWhiteSpace(tombstone.Body));
    }

    [Fact]
    public async Task EnsureTodoGuideAsync_RestoresGuideAfterDataReset()
    {
        var localization = TestServices.CreateLocalizationService(
            SettingsService.LanguageChinese);
        var store = new TodoWidgetStore(_dataRoot, "todo-guide-reset");
        await store.SaveAsync(new TodoWidgetData
        {
            Items = [new TodoItem { Id = "existing", Text = "existing item" }]
        });

        await store.ClearAsync();

        Assert.True(await WidgetFirstRunGuideFactory.EnsureTodoGuideAsync(
            store,
            localization));
        Assert.False(await WidgetFirstRunGuideFactory.EnsureTodoGuideAsync(
            store,
            localization));

        TodoItem guide = Assert.Single((await store.LoadAsync()).Items);
        Assert.Equal("开始管理第一项任务", guide.Text);
        Assert.Equal(3, guide.Steps.Count);
        Assert.Contains("较长的背景、决策和执行记录写在这里", guide.Notes, StringComparison.Ordinal);
    }

    public void Dispose()
    {
        try
        {
            if (Directory.Exists(_dataRoot))
            {
                Directory.Delete(_dataRoot, recursive: true);
            }
        }
        catch
        {
            // A failed cleanup must not hide the test result.
        }
    }
}
