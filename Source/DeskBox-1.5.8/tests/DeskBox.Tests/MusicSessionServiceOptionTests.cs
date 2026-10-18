using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class MusicSessionServiceOptionTests
{
    [Theory]
    [InlineData("QQMusic.exe", "QQ音乐")]
    [InlineData("cloudmusic.exe", "网易云音乐")]
    [InlineData("msedge.exe", "Microsoft Edge")]
    [InlineData("chrome.exe", "Google Chrome")]
    [InlineData("firefox.exe", "Mozilla Firefox")]
    [InlineData("Contoso.Player.exe", "Contoso.Player")]
    public void GetSourceDisplayName_ProducesFriendlyPlayerNames(
        string sourceAppUserModelId,
        string expected)
    {
        Assert.Equal(expected, MusicSessionService.GetSourceDisplayName(sourceAppUserModelId));
    }

    [Fact]
    public void CreateSessionId_DisambiguatesSessionsFromTheSameApplication()
    {
        string first = MusicSessionService.CreateSessionId("chrome.exe", 0);
        string second = MusicSessionService.CreateSessionId("chrome.exe", 1);

        Assert.NotEqual(first, second);
        Assert.StartsWith("chrome.exe", first, StringComparison.Ordinal);
        Assert.StartsWith("chrome.exe", second, StringComparison.Ordinal);
    }

    [Fact]
    public void DisambiguateSourceDisplayNames_NumbersOnlyDuplicateSources()
    {
        IReadOnlyList<string> result = MusicSessionService.DisambiguateSourceDisplayNames(
        [
            "Google Chrome",
            "QQ音乐",
            "Google Chrome"
        ]);

        Assert.Equal(
        [
            "Google Chrome (1)",
            "QQ音乐",
            "Google Chrome (2)"
        ], result);
    }
}
