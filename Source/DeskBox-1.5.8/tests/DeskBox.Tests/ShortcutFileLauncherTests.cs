using DeskBox.Helpers;

namespace DeskBox.Tests;

public sealed class ShortcutFileLauncherTests
{
    [Fact]
    public void BuildArgumentString_QuotesEveryDroppedPath()
    {
        string arguments = ShortcutFileLauncher.BuildArgumentString(
            [@"C:\notes\a.md", @"C:\notes\b.txt"]);

        Assert.Equal("\"C:\\notes\\a.md\" \"C:\\notes\\b.txt\"", arguments);
    }

    [Fact]
    public void BuildArgumentString_KeepsPathsWithSpacesAsOneArgument()
    {
        string arguments = ShortcutFileLauncher.BuildArgumentString(
            [@"E:\DeskBox\AI 工具\deskbox 复盘.md"]);

        Assert.Equal("\"E:\\DeskBox\\AI 工具\\deskbox 复盘.md\"", arguments);
    }

    [Fact]
    public void BuildArgumentString_SkipsEmptyEntries()
    {
        string arguments = ShortcutFileLauncher.BuildArgumentString(
            ["", "   ", @"C:\notes\a.md"]);

        Assert.Equal("\"C:\\notes\\a.md\"", arguments);
    }

    [Fact]
    public void BuildArgumentString_EmptyInputProducesNoArguments()
    {
        Assert.Equal(string.Empty, ShortcutFileLauncher.BuildArgumentString([]));
    }

    [Fact]
    public void SanitizeDroppedPaths_RejectsPathsACannotExistWindowsNameContains()
    {
        // A double quote cannot appear in a Windows file or folder name, so such
        // a value was forged. Dropping it keeps every quoted argument
        // unbreakable; the surviving path must still be usable.
        string[] sanitized = ShortcutFileLauncher.SanitizeDroppedPaths(
        [
            "C:\\notes\\a\".md",
            "C:\\notes\\b.md"
        ]);

        Assert.Equal([@"C:\notes\b.md"], sanitized);
        Assert.Equal(
            "\"C:\\notes\\b.md\"",
            ShortcutFileLauncher.BuildArgumentString(sanitized));
    }

    [Fact]
    public void SanitizeDroppedPaths_KeepsLegalAwkwardNames()
    {
        // Spaces, ampersands and carets are legal in Windows names and must
        // survive: quoting keeps them inside a single argument.
        string[] sanitized = ShortcutFileLauncher.SanitizeDroppedPaths(
            [@"C:\a & b\c ^ d.md"]);

        Assert.Equal([@"C:\a & b\c ^ d.md"], sanitized);
        Assert.Equal(
            "\"C:\\a & b\\c ^ d.md\"",
            ShortcutFileLauncher.BuildArgumentString(sanitized));
    }

    [Fact]
    public void TryLaunchWithFiles_RejectsEmptyInput()
    {
        Assert.False(ShortcutFileLauncher.TryLaunchWithFiles(string.Empty, [@"C:\a.md"]));
        Assert.False(ShortcutFileLauncher.TryLaunchWithFiles(@"C:\a.lnk", []));
    }

    [Fact]
    public void TryLaunchWithFiles_RejectsWhenEveryPathWasUnusable()
    {
        Assert.False(ShortcutFileLauncher.TryLaunchWithFiles(
            @"C:\a.lnk",
            ["C:\\notes\\a\".md"]));
    }
}
