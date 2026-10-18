using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class QuickCaptureSidebarFoundationTests : IDisposable
{
    private readonly string _root = Path.Combine(
        Path.GetTempPath(),
        $"DeskBox-QuickCapture-Sidebar-{Guid.NewGuid():N}");

    [Fact]
    public async Task LegacyV3Record_MigratesToPlainTextWithoutChangingBody()
    {
        Directory.CreateDirectory(_root);
        string path = Path.Combine(_root, "quick-capture.json");
        await File.WriteAllTextAsync(path, """
            {
              "version": 3,
              "currentView": "Records",
              "items": [
                {
                  "id": "legacy",
                  "type": "Text",
                  "body": "Legacy **literal** text",
                  "createdAt": "2026-01-01T00:00:00+00:00",
                  "updatedAt": "2026-01-01T00:00:00+00:00"
                }
              ],
              "recentItems": []
            }
            """);

        QuickCaptureStoreData data = await new QuickCaptureStore(_root).LoadAsync();

        QuickCaptureItem item = Assert.Single(data.Items);
        Assert.Equal(4, data.Version);
        Assert.Equal(TextContentFormat.PlainText, item.ContentFormat);
        Assert.Equal("Legacy **literal** text", item.Body);
    }

    [Fact]
    public async Task MarkdownRecord_RoundTripsLosslessSource()
    {
        const string source = "# Heading\n\n- [ ] item\n";
        var service = new QuickCaptureService(new QuickCaptureStore(_root));

        QuickCaptureItem item = await service.AddDetailedItemAsync(
            null,
            source,
            QuickCaptureAppearancePreset.Paper,
            TextContentFormat.Markdown);
        QuickCaptureItem stored = Assert.Single((await new QuickCaptureStore(_root).LoadAsync()).Items);

        Assert.Equal(TextContentFormat.Markdown, item.ContentFormat);
        Assert.Equal(TextContentFormat.Markdown, stored.ContentFormat);
        Assert.Equal(source, stored.Body);
        Assert.Equal(QuickCaptureAppearancePreset.Paper, stored.AppearancePreset);
    }

    [Fact]
    public async Task ClipboardHistory_IsAlwaysPlainText()
    {
        var store = new QuickCaptureStore(_root);
        await store.SaveAsync(new QuickCaptureStoreData
        {
            RecentItems =
            [
                new QuickCaptureItem
                {
                    Body = "# copied",
                    ContentFormat = TextContentFormat.Markdown,
                    IsRecent = true,
                    SourceKind = QuickCaptureSourceKind.Clipboard
                }
            ]
        });

        QuickCaptureItem recent = Assert.Single((await store.LoadAsync()).RecentItems);

        Assert.Equal(TextContentFormat.PlainText, recent.ContentFormat);
    }

    [Theory]
    [InlineData(null, SettingsService.QuickCaptureFormatMarkdown)]
    [InlineData("invalid", SettingsService.QuickCaptureFormatMarkdown)]
    [InlineData("plaintext", SettingsService.QuickCaptureFormatPlainText)]
    public void Settings_NormalizeQuickCaptureFormat(string? value, string expected) =>
        Assert.Equal(expected, SettingsService.NormalizeQuickCaptureFormat(value));

    [Fact]
    public void Settings_DefaultEditorFormat_IsMarkdown() =>
        Assert.Equal(
            SettingsService.QuickCaptureFormatMarkdown,
            new AppSettings().QuickCaptureDefaultFormat);

    [Theory]
    [InlineData(null, TextContentFormat.Markdown)]
    [InlineData("invalid", TextContentFormat.Markdown)]
    [InlineData("Markdown", TextContentFormat.Markdown)]
    [InlineData("plaintext", TextContentFormat.PlainText)]
    public void Settings_ResolveEditorContentFormat(
        string? value,
        TextContentFormat expected) =>
        Assert.Equal(
            expected,
            SettingsService.ResolveQuickCaptureEditorContentFormat(value));

    [Theory]
    [InlineData(null, SettingsService.QuickCaptureWideLayoutAuto)]
    [InlineData("invalid", SettingsService.QuickCaptureWideLayoutAuto)]
    [InlineData("singlepane", SettingsService.QuickCaptureWideLayoutSinglePane)]
    [InlineData("dualPane", SettingsService.QuickCaptureWideLayoutDualPane)]
    public void Settings_NormalizeWideLayout(string? value, string expected) =>
        Assert.Equal(expected, SettingsService.NormalizeQuickCaptureWideLayout(value));

    public void Dispose()
    {
        if (Directory.Exists(_root))
        {
            Directory.Delete(_root, recursive: true);
        }
    }
}
