using DeskBox.Models;
using DeskBox.Services;
using Markdig.Extensions.Tables;
using Markdig.Extensions.TaskLists;
using Markdig.Syntax;

namespace DeskBox.Tests;

public sealed class MarkdownDocumentServiceTests
{
    private readonly MarkdownDocumentService _service = new();

    [Fact]
    public void TextContentFormat_DefaultValuePreservesLegacyPlainText()
    {
        Assert.Equal(0, (int)TextContentFormat.PlainText);
        Assert.Equal(TextContentFormat.PlainText, default);
    }

    [Fact]
    public void Parse_SupportsTasksAndTablesAndRemovesNullCharacters()
    {
        MarkdownParseResult result = _service.Parse(
            "- [ ] task\0\n\n| A | B |\n| - | - |\n| 1 | 2 |");

        Assert.DoesNotContain('\0', result.Source);
        Assert.NotEmpty(result.Document.Descendants<TaskList>());
        Assert.NotEmpty(result.Document.Descendants<Table>());
        Assert.False(result.WasTruncated);
    }

    [Fact]
    public void Parse_TruncatesPathologicalDocumentsBeforeParsing()
    {
        MarkdownParseResult result = _service.Parse(
            new string('a', MarkdownDocumentService.MaxCharacters + 17));

        Assert.True(result.WasTruncated);
        Assert.Equal(MarkdownDocumentService.MaxCharacters, result.Source.Length);
    }

    [Fact]
    public void TryToggleTask_ChangesOnlyTheRequestedSourceMarker()
    {
        const string source = "- [ ] first\n- [x] second";

        Assert.True(_service.TryToggleTask(source, 1, out string updated));
        Assert.Equal("- [ ] first\n- [ ] second", updated);
        Assert.False(_service.TryToggleTask(source, 4, out _));
    }

    [Fact]
    public void ToPlainText_ProjectsVisibleContentWithoutMarkdownMarkers()
    {
        string text = _service.ToPlainText("## Heading\n\n**bold** and `code`");

        Assert.Contains("Heading", text, StringComparison.Ordinal);
        Assert.Contains("bold and code", text, StringComparison.Ordinal);
        Assert.DoesNotContain("**", text, StringComparison.Ordinal);
    }

    [Fact]
    public void ToSafeHtml_RendersMarkdownButNeverPassesRawHtmlThrough()
    {
        string html = _service.ToSafeHtml("**safe**<script>alert(1)</script>");

        Assert.Contains("<strong>safe</strong>", html, StringComparison.Ordinal);
        Assert.DoesNotContain("<script", html, StringComparison.OrdinalIgnoreCase);
    }

    [Theory]
    [InlineData("https://example.com", true)]
    [InlineData("http://example.com", true)]
    [InlineData("mailto:user@example.com", true)]
    [InlineData("deskbox-attachment://abc", true)]
    [InlineData("javascript:alert(1)", false)]
    [InlineData("file:///C:/secret.txt", false)]
    [InlineData("relative/path", false)]
    public void IsAllowedLink_UsesTheExplicitProtocolAllowList(string value, bool expected)
    {
        Assert.Equal(expected, MarkdownDocumentService.IsAllowedLink(value));
    }
}
