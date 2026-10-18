namespace DeskBox.Tests;

/// <summary>
/// InsertLineBreak must insert the single-char "\r" break the RichEdit-based
/// TextBox actually preserves (#178): a "\r\n" insert gets collapsed to "\r"
/// on Text assignment, so caret math based on the 2-char break landed one
/// character too far right after a newline.
/// </summary>
public sealed class TextBoxEditorShortcutHelperTests
{
    [Fact]
    public void InsertLineBreak_UsesTheSingleCharBreakTheTextBoxPreserves()
    {
        string source = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Helpers/TextBoxEditorShortcutHelper.cs"));

        Assert.Contains("""string lineBreak = "\r";""", source, StringComparison.Ordinal);
        Assert.DoesNotContain("Environment.NewLine", source, StringComparison.Ordinal);
        Assert.Contains(
            "textBox.Select(selectionStart + lineBreak.Length, 0);",
            source,
            StringComparison.Ordinal);
    }
}
