using System.Text;
using DeskBox.Models;

namespace DeskBox.Services;

public enum SimpleMarkdownBlockKind
{
    Paragraph,
    Heading,
    ListItem,
    Quote,
    CodeBlock,
    Separator
}

public enum SimpleMarkdownInlineKind
{
    Plain,
    Bold,
    Italic,
    Code,
    Link
}

public sealed record SimpleMarkdownInline(
    string Text,    SimpleMarkdownInlineKind Kind = SimpleMarkdownInlineKind.Plain,    string? LinkUrl = null);

public sealed record SimpleMarkdownBlock(
    SimpleMarkdownBlockKind Kind,    IReadOnlyList<SimpleMarkdownInline> Inlines,    int Level = 0,    bool IsOrdered = false,    int ListIndex = 0);

/// <summary>
/// Parses the small, deliberately safe Markdown subset used by release notes.
/// It never produces HTML and only accepts HTTPS links as links. The UI layer
/// is responsible for choosing the visual controls used to render the result.
/// </summary>
public static class SimpleMarkdownRenderer
{
    public const int MaxCharacters = 256 * 1024;

    public static IReadOnlyList<SimpleMarkdownBlock> Parse(string? markdown)
    {
        string source = (markdown ?? string.Empty).Replace("\0",string.Empty);
        if (source.Length > MaxCharacters)
        {
            source = source[..MaxCharacters];
        }

        string[] lines = source.Replace("\r\n","\n").Replace('\r','\n').Split('\n');
        var blocks = new List<SimpleMarkdownBlock>();
        var paragraphLines = new List<string>();
        var codeLines = new List<string>();
        bool inCodeBlock = false;

        void FlushParagraph()
        {
            if (paragraphLines.Count == 0)
            {
                return;
            }

            string paragraph = string.Join(" ",paragraphLines).Trim();
            if (paragraph.Length > 0)
            {
                blocks.Add(new SimpleMarkdownBlock(
                    SimpleMarkdownBlockKind.Paragraph,                    ParseInlines(paragraph)));
            }

            paragraphLines.Clear();
        }

        void FlushCodeBlock()
        {
            if (codeLines.Count == 0)
            {
                blocks.Add(new SimpleMarkdownBlock(
                    SimpleMarkdownBlockKind.CodeBlock,                    [new SimpleMarkdownInline(string.Empty)]));
            }
            else
            {
                blocks.Add(new SimpleMarkdownBlock(
                    SimpleMarkdownBlockKind.CodeBlock,                    [new SimpleMarkdownInline(string.Join("\n",codeLines))]));
            }

            codeLines.Clear();
        }

        for (int lineIndex = 0; lineIndex < lines.Length; lineIndex++)
        {
            string line = lines[lineIndex];
            string trimmed = line.Trim();

            if (inCodeBlock)
            {
                if (trimmed.StartsWith("```",StringComparison.Ordinal))
                {
                    FlushCodeBlock();
                    inCodeBlock = false;
                }
                else
                {
                    codeLines.Add(line);
                }

                continue;
            }

            if (trimmed.StartsWith("```",StringComparison.Ordinal))
            {
                FlushParagraph();
                inCodeBlock = true;
                continue;
            }

            if (trimmed.Length == 0)
            {
                FlushParagraph();
                continue;
            }

            if (IsSeparator(trimmed))
            {
                FlushParagraph();
                blocks.Add(new SimpleMarkdownBlock(
                    SimpleMarkdownBlockKind.Separator,                    Array.Empty<SimpleMarkdownInline>()));
                continue;
            }

            if (TryParseHeading(trimmed,out int headingLevel,out string headingText))
            {
                FlushParagraph();
                blocks.Add(new SimpleMarkdownBlock(
                    SimpleMarkdownBlockKind.Heading,                    ParseInlines(headingText),                    headingLevel));
                continue;
            }

            if (TryParseListItem(trimmed,out bool ordered,out int listIndex,out string listText))
            {
                FlushParagraph();
                blocks.Add(new SimpleMarkdownBlock(
                    SimpleMarkdownBlockKind.ListItem,                    ParseInlines(listText),                    IsOrdered: ordered,                    ListIndex: listIndex));
                continue;
            }

            if (trimmed.StartsWith(">",StringComparison.Ordinal))
            {
                FlushParagraph();
                string quoteText = trimmed[1..].TrimStart();
                blocks.Add(new SimpleMarkdownBlock(
                    SimpleMarkdownBlockKind.Quote,                    ParseInlines(quoteText)));
                continue;
            }

            paragraphLines.Add(trimmed);
        }

        if (inCodeBlock)
        {
            FlushCodeBlock();
        }

        FlushParagraph();
        return blocks;
    }

    private static bool IsSeparator(string text)
    {
        if (text.Length < 3)
        {
            return false;
        }

        char marker = text[0];
        if (marker is not ('-' or '*' or '_'))
        {
            return false;
        }

        int count = 0;
        foreach (char character in text)
        {
            if (character == marker)
            {
                count++;
            }
            else if (!char.IsWhiteSpace(character))
            {
                return false;
            }
        }

        return count >= 3;
    }

    private static bool TryParseHeading(string text,out int level,out string content)
    {
        level = 0;
        content = string.Empty;
        while (level < text.Length && level < 6 && text[level] == '#')
        {
            level++;
        }

        if (level == 0 || level == text.Length || !char.IsWhiteSpace(text[level]))
        {
            level = 0;
            return false;
        }

        content = text[level..].Trim();
        return content.Length > 0;
    }

    private static bool TryParseListItem(
        string text,        out bool ordered,        out int listIndex,        out string content)
    {
        ordered = false;
        listIndex = 0;
        content = string.Empty;

        int start = 0;
        while (start < text.Length && char.IsWhiteSpace(text[start]))
        {
            start++;
        }

        if (start + 1 < text.Length && text[start] is '-' or '*' or '+' && char.IsWhiteSpace(text[start + 1]))
        {
            content = text[(start + 1)..].Trim();
            return content.Length > 0;
        }

        int numberEnd = start;
        while (numberEnd < text.Length && char.IsDigit(text[numberEnd]))
        {
            numberEnd++;
        }

        if (numberEnd > start && numberEnd + 1 < text.Length &&
            text[numberEnd] is '.' or ')' && char.IsWhiteSpace(text[numberEnd + 1]) &&
            int.TryParse(text[start..numberEnd],out listIndex))
        {
            ordered = true;
            content = text[(numberEnd + 1)..].Trim();
            return content.Length > 0;
        }

        return false;
    }

    private static IReadOnlyList<SimpleMarkdownInline> ParseInlines(string text)
    {
        if (text.Length == 0)
        {
            return [new SimpleMarkdownInline(string.Empty)];
        }

        var result = new List<SimpleMarkdownInline>();
        var plain = new StringBuilder();

        void FlushPlain()
        {
            if (plain.Length == 0)
            {
                return;
            }

            result.Add(new SimpleMarkdownInline(plain.ToString()));
            plain.Clear();
        }

        int index = 0;
        while (index < text.Length)
        {
            if (text[index] == '`')
            {
                int end = text.IndexOf('`',index + 1);
                if (end > index + 1)
                {
                    FlushPlain();
                    result.Add(new SimpleMarkdownInline(
                        text[(index + 1)..end],                        SimpleMarkdownInlineKind.Code));
                    index = end + 1;
                    continue;
                }
            }

            if (text[index] == '[')
            {
                int labelEnd = text.IndexOf("](",index + 1,StringComparison.Ordinal);
                if (labelEnd > index + 1)
                {
                    int urlEnd = text.IndexOf(')',labelEnd + 2);
                    if (urlEnd > labelEnd + 2)
                    {
                        string url = text[(labelEnd + 2)..urlEnd];
                        if (AppUpdateManifest.IsSafeReleaseNotesUrl(url))
                        {
                            FlushPlain();
                            result.Add(new SimpleMarkdownInline(
                                text[(index + 1)..labelEnd],                                SimpleMarkdownInlineKind.Link,                                url));
                            index = urlEnd + 1;
                            continue;
                        }
                    }
                }
            }

            if (TryReadEmphasis(text,index,"**",out int boldEnd))
            {
                FlushPlain();
                result.Add(new SimpleMarkdownInline(
                    text[(index + 2)..boldEnd],                    SimpleMarkdownInlineKind.Bold));
                index = boldEnd + 2;
                continue;
            }

            if (TryReadEmphasis(text,index,"__",out int strongEnd))
            {
                FlushPlain();
                result.Add(new SimpleMarkdownInline(
                    text[(index + 2)..strongEnd],                    SimpleMarkdownInlineKind.Bold));
                index = strongEnd + 2;
                continue;
            }

            if ((text[index] == '*' || text[index] == '_') &&
                TryReadEmphasis(text,index,text[index].ToString(),out int italicEnd))
            {
                FlushPlain();
                result.Add(new SimpleMarkdownInline(
                    text[(index + 1)..italicEnd],                    SimpleMarkdownInlineKind.Italic));
                index = italicEnd + 1;
                continue;
            }

            plain.Append(text[index]);
            index++;
        }

        FlushPlain();
        return result;
    }

    private static bool TryReadEmphasis(string text,int start,string marker,out int end)
    {
        end = -1;
        if (!text.AsSpan(start).StartsWith(marker,StringComparison.Ordinal))
        {
            return false;
        }

        int candidate = text.IndexOf(marker,start + marker.Length,StringComparison.Ordinal);
        if (candidate <= start + marker.Length)
        {
            return false;
        }

        end = candidate;
        return true;
    }
}
