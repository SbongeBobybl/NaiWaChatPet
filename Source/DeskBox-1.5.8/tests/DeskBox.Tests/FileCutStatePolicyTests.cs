using DeskBox.Models;

namespace DeskBox.Tests;

public sealed class FileCutStatePolicyTests
{
    [Fact]
    public void RemoveDepartedPaths_RemovesOnlyFilesThatLeftTheSurface()
    {
        string[] remaining = FileCutStatePolicy.RemoveDepartedPaths(
            [@"C:\Box\one.txt", @"C:\Box\two.txt", @"C:\Box\three.txt"],
            [@"C:\Box\one.txt", @"C:\Box\unrelated.txt"],
            []);

        Assert.Equal(
            [@"C:\Box\two.txt", @"C:\Box\three.txt"],
            remaining);
    }

    [Fact]
    public void RemoveDepartedPaths_PreservesSamePathReplacement()
    {
        string[] remaining = FileCutStatePolicy.RemoveDepartedPaths(
            [@"C:\Box\one.txt"],
            [@"C:\Box\one.txt"],
            [@"c:\box\ONE.txt"]);

        Assert.Equal([@"C:\Box\one.txt"], remaining);
    }
}
