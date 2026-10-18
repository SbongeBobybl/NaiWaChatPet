using DeskBox.Models;
using DeskBox.Services;
using System.Net;
using System.Text;
using System.Text.Json;

namespace DeskBox.Tests;

public sealed class ReleaseNotesTests : IDisposable
{
    private readonly string _cacheRoot = Path.Combine(
        Path.GetTempPath(),
        "DeskBox.Tests",
        Guid.NewGuid().ToString("N"));

    [Fact]
    public void LocalizedReleaseNotes_ChineseVariantsPreferMatchingScriptAndFallBack()
    {
        var manifest = new AppUpdateManifest
        {
            ReleaseNotes = new Dictionary<string, string>
            {
                ["en-US"] = "English content",
                ["zh-CN"] = "简体内容",
                ["zh-TW"] = "繁體內容"
            }
        };

        Assert.Equal("简体内容", manifest.GetLocalizedReleaseNotes("zh-CN"));
        Assert.Equal("繁體內容", manifest.GetLocalizedReleaseNotes("zh-TW"));
        Assert.Equal("繁體內容", manifest.GetLocalizedReleaseNotes("zh-HK"));
        Assert.Equal("繁體內容", manifest.GetLocalizedReleaseNotes("zh-Hant"));
        Assert.Equal("English content", manifest.GetLocalizedReleaseNotes("de-DE"));

        manifest.ReleaseNotes.Remove("zh-TW");
        Assert.Equal("简体内容", manifest.GetLocalizedReleaseNotes("zh-TW"));
        manifest.ReleaseNotes.Remove("zh-CN");
        Assert.Equal("English content", manifest.GetLocalizedReleaseNotes("zh-CN"));
    }

    [Fact]
    public void SimpleMarkdownRenderer_ParsesSafeSubsetAndRejectsUnsafeLinks()
    {
        IReadOnlyList<SimpleMarkdownBlock> blocks = SimpleMarkdownRenderer.Parse(
            "# Heading\n\n- **bold** and *italic* with `code`\n\n[docs](https://example.com) [unsafe](javascript:alert(1))");

        Assert.Contains(blocks, block => block.Kind == SimpleMarkdownBlockKind.Heading);
        SimpleMarkdownBlock list = Assert.Single(blocks.Where(block => block.Kind == SimpleMarkdownBlockKind.ListItem));
        Assert.Contains(list.Inlines, inline => inline.Kind == SimpleMarkdownInlineKind.Bold);
        Assert.Contains(list.Inlines, inline => inline.Kind == SimpleMarkdownInlineKind.Italic);
        Assert.Contains(list.Inlines, inline => inline.Kind == SimpleMarkdownInlineKind.Code);

        SimpleMarkdownBlock paragraph = Assert.Single(blocks.Where(block => block.Kind == SimpleMarkdownBlockKind.Paragraph));
        Assert.Contains(paragraph.Inlines, inline => inline.Kind == SimpleMarkdownInlineKind.Link &&
            inline.LinkUrl == "https://example.com");
        Assert.DoesNotContain(paragraph.Inlines, inline => inline.Kind == SimpleMarkdownInlineKind.Link &&
            inline.Text == "unsafe");
    }

    [Fact]
    public async Task ReleaseNotesService_UsesInlineContentAndCachesIt()
    {
        var manifest = new AppUpdateManifest
        {
            Version = "1.2.3",
            ReleaseNotes = new Dictionary<string, string>
            {
                ["en-US"] = "## Cached content"
            }
        };
        var service = new ReleaseNotesService(_cacheRoot);

        ReleaseNotesLoadResult first = await service.LoadAsync(manifest, "en-US");
        Assert.Equal("## Cached content", first.Content);
        Assert.False(first.IsFromCache);

        manifest.ReleaseNotes.Clear();
        ReleaseNotesLoadResult second = await service.LoadAsync(manifest, "en-US");
        Assert.Equal("## Cached content", second.Content);
        Assert.True(second.IsFromCache);
    }

    [Fact]
    public async Task ReleaseNotesService_UnifiedLoadCombinesDistinctInlineLocales()
    {
        var manifest = new AppUpdateManifest
        {
            Version = "1.2.4",
            ReleaseNotes = new Dictionary<string, string>
            {
                ["en-US"] = "## English section",
                ["zh-CN"] = "## 中文部分",
                ["de-DE"] = "## English section"
            }
        };
        var service = new ReleaseNotesService(_cacheRoot);

        ReleaseNotesLoadResult result = await service.LoadAsync(manifest);

        Assert.Contains("## English section", result.Content);
        Assert.Contains("## 中文部分", result.Content);
        Assert.Equal(1, result.Content.Split("## English section").Length - 1);
    }

    [Fact]
    public async Task ReleaseNotesService_ResolvesGitHubReleasePageToApiBody()
    {
        Uri? requestedUri = null;
        using var httpClient = new HttpClient(new StubHttpMessageHandler(request =>
        {
            requestedUri = request.RequestUri;
            return new HttpResponseMessage(HttpStatusCode.OK)
            {
                Content = new StringContent(
                    JsonSerializer.Serialize(new { body = "## Latest\n- Fixed update content" }),
                    Encoding.UTF8,
                    "application/json")
            };
        }));
        var service = new ReleaseNotesService(_cacheRoot, httpClient);
        var manifest = new AppUpdateManifest
        {
            Version = "1.3.4",
            ReleaseNotesUrl = "https://github.com/Tianyu199509/DeskBox/releases/tag/v1.3.4"
        };

        ReleaseNotesLoadResult result = await service.LoadAsync(manifest, "en-US");

        Assert.Equal("https://api.github.com/repos/Tianyu199509/DeskBox/releases/tags/v1.3.4", requestedUri?.ToString());
        Assert.Equal("## Latest\n- Fixed update content", result.Content);
    }

    public void Dispose()
    {
        try
        {
            if (Directory.Exists(_cacheRoot))
            {
                Directory.Delete(_cacheRoot, recursive: true);
            }
        }
        catch
        {
        }
    }

    private sealed class StubHttpMessageHandler(Func<HttpRequestMessage, HttpResponseMessage> responder) : HttpMessageHandler
    {
        protected override Task<HttpResponseMessage> SendAsync(
            HttpRequestMessage request,
            CancellationToken cancellationToken) =>
            Task.FromResult(responder(request));
    }
}
