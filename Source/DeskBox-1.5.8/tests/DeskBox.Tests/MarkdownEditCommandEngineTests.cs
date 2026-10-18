using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class MarkdownEditCommandEngineTests
{
    [Fact]
    public void TaskAtCaret_AddsMarkerAndLeavesCollapsedCaretAfterMarker()
    {
        const string source = "今天处理";

        MarkdownTextEdit edit = Create(source, 2, 0, MarkdownEditCommand.Task);

        Assert.Equal("- [ ] 今天处理", edit.Apply(source));
        Assert.Equal(8, edit.SelectionStart);
        Assert.Equal(0, edit.SelectionLength);
    }

    [Fact]
    public void TaskOnList_ConvertsInsteadOfNestingListMarker()
    {
        const string source = "  - 已有列表";

        MarkdownTextEdit edit = Create(source, source.Length, 0, MarkdownEditCommand.Task);

        Assert.Equal("  - [ ] 已有列表", edit.Apply(source));
        Assert.Equal(source.Length + 4, edit.SelectionStart);
        Assert.Equal(0, edit.SelectionLength);
    }

    [Fact]
    public void TaskOnTask_ConvertsBackToOrdinaryList()
    {
        const string source = "- [x] 已完成";

        MarkdownTextEdit edit = Create(source, 6, 0, MarkdownEditCommand.Task);

        Assert.Equal("- 已完成", edit.Apply(source));
        Assert.Equal(2, edit.SelectionStart);
        Assert.Equal(0, edit.SelectionLength);
    }

    [Fact]
    public void TaskAtCaretOnThirdLine_OnlyTransformsThirdLine()
    {
        const string source = "第一行\r\n第二行\r\n第三行";
        int thirdLineCaret = source.IndexOf("第三行", StringComparison.Ordinal) + 1;

        MarkdownTextEdit edit = Create(source, thirdLineCaret, 0, MarkdownEditCommand.Task);

        Assert.Equal("第一行\r\n第二行\r\n- [ ] 第三行", edit.Apply(source));
        Assert.Equal(source.IndexOf("第三行", StringComparison.Ordinal) + 7, edit.SelectionStart);
        Assert.Equal(0, edit.SelectionLength);
    }

    [Fact]
    public void TaskAtCaretOnThirdCarriageReturnLine_OnlyTransformsThirdLine()
    {
        const string source = "- 第一行\r第二行\r第三行";
        int thirdLineCaret = source.IndexOf("第三行", StringComparison.Ordinal) + 1;

        MarkdownTextEdit edit = Create(source, thirdLineCaret, 0, MarkdownEditCommand.Task);

        Assert.Equal("- 第一行\r第二行\r- [ ] 第三行", edit.Apply(source));
        Assert.Equal(0, edit.SelectionLength);
    }

    [Fact]
    public void TaskAcrossLines_PreservesTextSelectionAndSkipsBlankLines()
    {
        const string source = "甲\r\n\r\n乙";

        MarkdownTextEdit edit = Create(source, 0, source.Length, MarkdownEditCommand.Task);
        string updated = edit.Apply(source);

        Assert.Equal("- [ ] 甲\r\n\r\n- [ ] 乙", updated);
        Assert.Equal(6, edit.SelectionStart);
        Assert.Equal("甲\r\n\r\n- [ ] 乙".Length, edit.SelectionLength);
        Assert.Equal("甲\r\n\r\n- [ ] 乙", updated.Substring(edit.SelectionStart, edit.SelectionLength));
    }

    [Fact]
    public void BoldWithoutSelection_InsertsEmptyPairAtCaret()
    {
        const string source = "内容";

        MarkdownTextEdit edit = Create(source, 1, 0, MarkdownEditCommand.Bold);

        Assert.Equal("内****容", edit.Apply(source));
        Assert.Equal(3, edit.SelectionStart);
        Assert.Equal(0, edit.SelectionLength);
    }

    [Theory]
    [InlineData(MarkdownEditCommand.Bold, "**", "**")]
    [InlineData(MarkdownEditCommand.Italic, "*", "*")]
    [InlineData(MarkdownEditCommand.Strikethrough, "~~", "~~")]
    [InlineData(MarkdownEditCommand.Code, "`", "`")]
    public void InlineCommandAtCaret_SecondClickRemovesEmptyPair(
        MarkdownEditCommand command,
        string prefix,
        string suffix)
    {
        const string source = "第三行";
        MarkdownTextEdit add = Create(source, 1, 0, command);
        string marked = add.Apply(source);
        Assert.Equal("第" + prefix + suffix + "三行", marked);

        MarkdownTextEdit remove = Create(marked, add.SelectionStart, 0, command);
        Assert.Equal(source, remove.Apply(marked));
        Assert.Equal(1, remove.SelectionStart);
        Assert.Equal(0, remove.SelectionLength);
    }

    [Theory]
    [InlineData(MarkdownEditCommand.Bold, "**第**三行")]
    [InlineData(MarkdownEditCommand.Italic, "*第*三行")]
    [InlineData(MarkdownEditCommand.Strikethrough, "~~第~~三行")]
    [InlineData(MarkdownEditCommand.Code, "`第`三行")]
    public void InlineCommandSelectionOnThirdLine_OnlyWrapsSelection(
        MarkdownEditCommand command,
        string expectedThirdLine)
    {
        const string source = "第一行\n第二行\n第三行";
        int start = source.IndexOf("第三行", StringComparison.Ordinal);

        MarkdownTextEdit edit = Create(source, start, 1, command);

        Assert.Equal("第一行\n第二行\n" + expectedThirdLine, edit.Apply(source));
    }

    [Fact]
    public void BoldSelection_TogglesWithoutChangingSelectedText()
    {
        const string source = "中文内容";

        MarkdownTextEdit add = Create(source, 0, source.Length, MarkdownEditCommand.Bold);
        string wrapped = add.Apply(source);
        Assert.Equal("**中文内容**", wrapped);
        Assert.Equal("中文内容", wrapped.Substring(add.SelectionStart, add.SelectionLength));

        MarkdownTextEdit remove = Create(
            wrapped,
            add.SelectionStart,
            add.SelectionLength,
            MarkdownEditCommand.Bold);
        Assert.Equal(source, remove.Apply(wrapped));
        Assert.Equal(source, remove.Apply(wrapped).Substring(remove.SelectionStart, remove.SelectionLength));
    }

    [Fact]
    public void LinkWithoutSelection_PutsCaretInsideLabel()
    {
        MarkdownTextEdit edit = Create(string.Empty, 0, 0, MarkdownEditCommand.Link);

        Assert.Equal("[](https://)", edit.Apply(string.Empty));
        Assert.Equal(1, edit.SelectionStart);
        Assert.Equal(0, edit.SelectionLength);
    }

    [Fact]
    public void LinkSelection_TogglesWithoutLosingLabel()
    {
        const string source = "查看文档";
        MarkdownTextEdit add = Create(source, 2, 2, MarkdownEditCommand.Link);
        string linked = add.Apply(source);
        Assert.Equal("查看[文档](https://)", linked);

        MarkdownTextEdit remove = Create(linked, 3, 2, MarkdownEditCommand.Link);
        Assert.Equal(source, remove.Apply(linked));
        Assert.Equal("文档", remove.Apply(linked).Substring(remove.SelectionStart, remove.SelectionLength));
    }

    [Fact]
    public void TableAtThirdLine_InsertsThereWithBlockSpacingAndPreservesText()
    {
        const string source = "第一行\n第二行\n第三行";
        int caret = source.IndexOf("第三行", StringComparison.Ordinal) + 1;

        MarkdownTextEdit edit = Create(source, caret, 0, MarkdownEditCommand.Table);
        string updated = edit.Apply(source);

        Assert.Equal(
            "第一行\n第二行\n| Column 1 | Column 2 |\n| --- | --- |\n| Content | Content |\n第三行",
            updated);
        Assert.Equal("Column 1", updated.Substring(edit.SelectionStart, edit.SelectionLength));
    }

    [Fact]
    public void TableAtCarriageReturnLine_PreservesNativeLineEndings()
    {
        const string source = "第一行\r第二行\r第三行";
        int caret = source.IndexOf("第三行", StringComparison.Ordinal) + 1;

        MarkdownTextEdit edit = Create(source, caret, 0, MarkdownEditCommand.Table);

        Assert.Equal(
            "第一行\r第二行\r| Column 1 | Column 2 |\r| --- | --- |\r| Content | Content |\r第三行",
            edit.Apply(source));
    }

    [Fact]
    public void TableDoesNotReplaceSelectedText()
    {
        const string source = "保留这段内容";

        MarkdownTextEdit edit = Create(source, 2, 2, MarkdownEditCommand.Table);

        Assert.Contains(source, edit.Apply(source), StringComparison.Ordinal);
        Assert.Equal(0, edit.Length);
    }

    [Fact]
    public void MultilineCodeSelection_TogglesFencedBlock()
    {
        const string source = "第一行\n第二行";
        MarkdownTextEdit add = Create(source, 0, source.Length, MarkdownEditCommand.Code);
        string fenced = add.Apply(source);
        Assert.Equal("```\n第一行\n第二行\n```", fenced);

        MarkdownTextEdit remove = Create(
            fenced,
            add.SelectionStart,
            add.SelectionLength,
            MarkdownEditCommand.Code);
        Assert.Equal(source, remove.Apply(fenced));
    }

    [Fact]
    public void CarriageReturnMultilineCode_UsesCarriageReturnFences()
    {
        const string source = "第一行\r第二行";

        MarkdownTextEdit edit = Create(source, 0, source.Length, MarkdownEditCommand.Code);

        Assert.Equal("```\r第一行\r第二行\r```", edit.Apply(source));
    }

    [Fact]
    public void HeadingPreservesCrlfAndOriginalTextSelection()
    {
        const string source = "标题一\r\n标题二";

        MarkdownTextEdit edit = Create(source, 0, source.Length, MarkdownEditCommand.Heading);
        string updated = edit.Apply(source);

        Assert.Equal("## 标题一\r\n## 标题二", updated);
        Assert.Equal("标题一\r\n## 标题二", updated.Substring(edit.SelectionStart, edit.SelectionLength));
    }

    [Theory]
    [InlineData(MarkdownEditCommand.Heading, "## 第三行")]
    [InlineData(MarkdownEditCommand.List, "- 第三行")]
    [InlineData(MarkdownEditCommand.Task, "- [ ] 第三行")]
    [InlineData(MarkdownEditCommand.Quote, "> 第三行")]
    public void BlockCommandAtThirdLine_LeavesEarlierLinesUntouched(
        MarkdownEditCommand command,
        string expectedThirdLine)
    {
        const string source = "第一行\n第二行\n第三行";
        int caret = source.IndexOf("第三行", StringComparison.Ordinal) + 1;

        MarkdownTextEdit edit = Create(source, caret, 0, command);

        Assert.Equal("第一行\n第二行\n" + expectedThirdLine, edit.Apply(source));
    }

    [Theory]
    [InlineData("- 列表", MarkdownEditCommand.Heading, "## 列表")]
    [InlineData("## 标题", MarkdownEditCommand.List, "- 标题")]
    [InlineData("> 引用", MarkdownEditCommand.Task, "- [ ] 引用")]
    [InlineData("1. 有序", MarkdownEditCommand.Quote, "> 有序")]
    public void BlockCommands_ReplaceExistingBlockStyleInsteadOfStackingMarkers(
        string source,
        MarkdownEditCommand command,
        string expected)
    {
        MarkdownTextEdit edit = Create(source, source.Length, 0, command);

        Assert.Equal(expected, edit.Apply(source));
    }

    [Fact]
    public void IndentAndOutdent_RoundTripMultilineText()
    {
        const string source = "甲\n乙";
        MarkdownTextEdit indent = Create(source, 0, source.Length, MarkdownEditCommand.Indent);
        string indented = indent.Apply(source);
        Assert.Equal("    甲\n    乙", indented);

        MarkdownTextEdit outdent = Create(
            indented,
            indent.SelectionStart,
            indent.SelectionLength,
            MarkdownEditCommand.Outdent);
        Assert.Equal(source, outdent.Apply(indented));
    }

    private static MarkdownTextEdit Create(
        string source,
        int selectionStart,
        int selectionLength,
        MarkdownEditCommand command)
    {
        Assert.True(MarkdownEditCommandEngine.TryCreateEdit(
            source,
            selectionStart,
            selectionLength,
            command,
            out MarkdownTextEdit edit));
        return edit;
    }
}
