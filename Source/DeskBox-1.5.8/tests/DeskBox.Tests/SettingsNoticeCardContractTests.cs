using System.Xml.Linq;

namespace DeskBox.Tests;

/// <summary>
/// Notice cards in the settings surfaces must be native InfoBars driven
/// through the Title/Message properties (HeaderKey/DescriptionKey). Three
/// historical traps are pinned here:
/// 1. Rich content placed inside the InfoBar content region collided with
///    the template action-button slot (a button-shaped outline across the
///    text).
/// 2. The elevated-hotkey notice must sit OUTSIDE the hotkey expander
///    entirely (a sibling below it, unified across the interaction and
///    search pages): long wrapped text fails to lay out inside the nested
///    item card and renders as an empty strip.
/// 3. ThemeResource severity brushes fail to resolve inside the
///    late-created section templates (DataTemplate.LoadContent), painting
///    the notice exactly like the card behind it with near-invisible text;
///    the severity brushes must be pinned from the application resources
///    in code-behind, and each notice needs an x:Name so the shell can
///    reach it.
/// </summary>
public sealed class SettingsNoticeCardContractTests
{
    [Fact]
    public void SettingsSurfaces_DoNotPlaceRichContentInsideInfoBarContent()
    {
        string window = ReadRepositoryFile("src/DeskBox/Views/SettingsWindow.xaml");
        string search = ReadRepositoryFile(
            "src/DeskBox/Views/SettingsSections/SearchSettingsSection.xaml");

        Assert.DoesNotContain("InfoBar.Content", window, StringComparison.Ordinal);
        Assert.DoesNotContain("InfoBar.Content", search, StringComparison.Ordinal);
    }

    [Theory]
    [InlineData("src/DeskBox/Views/SettingsWindow.xaml", 3)]
    [InlineData("src/DeskBox/Views/SettingsSections/SearchSettingsSection.xaml", 1)]
    public void Notices_AreNativeInfoBarsWithTitleAndMessageProperties(string path, int expectedCount)
    {
        string xaml = ReadRepositoryFile(path);

        int count = CountOccurrences(xaml, "IsClosable=\"False\"");
        Assert.True(count >= expectedCount,
            $"{path}: expected at least {expectedCount} notice InfoBars, found {count}");

        int headerKeyNotices = CountOccurrences(xaml,
            "svc:Localized.DescriptionKey=\"Settings.GlobalHotkey.ElevatedNotice.Description\"");
        int storeNotice = CountOccurrences(xaml,
            "svc:Localized.HeaderKey=\"Settings.About.Channel.Store\"");
        int cloudNotice = CountOccurrences(xaml,
            "svc:Localized.HeaderKey=\"Settings.CloudBackup.SyncNotice.Title\"");
        Assert.True(headerKeyNotices + storeNotice + cloudNotice >= expectedCount,
            $"{path}: notice InfoBars must carry HeaderKey/DescriptionKey");
    }

    [Theory]
    [InlineData("src/DeskBox/Views/SettingsWindow.xaml")]
    [InlineData("src/DeskBox/Views/SettingsSections/SearchSettingsSection.xaml")]
    public void ElevatedNoticeInfoBars_AreSiblingsOfTheHotkeyExpander(string path)
    {
        XDocument document = XDocument.Parse(ReadRepositoryFile(path));
        XNamespace presentation = document.Root!.Name.Namespace;
        XNamespace toolkit = "using:CommunityToolkit.WinUI.Controls";
        XNamespace xaml = "http://schemas.microsoft.com/winfx/2006/xaml";

        List<XElement> notices = document
            .Descendants(presentation + "InfoBar")
            .Where(element => element.Attributes().Any(attribute =>
                attribute.Name.LocalName.EndsWith(".HeaderKey", StringComparison.Ordinal) &&
                string.Equals(
                    attribute.Value,
                    "Settings.GlobalHotkey.ElevatedNotice.Title",
                    StringComparison.Ordinal)))
            .ToList();
        Assert.NotEmpty(notices);

        foreach (XElement notice in notices)
        {
            Assert.False(notice.Ancestors(toolkit + "SettingsExpander").Any(),
                $"{path}: the elevated-hotkey notice InfoBar must sit outside the hotkey " +
                "expander (a sibling below it), not inside its item cards.");
            Assert.NotNull(notice.Attribute(xaml + "Name") ?? notice.Attribute("Name"));
        }
    }

    [Fact]
    public void NoticeInfoBarSeverityBrushes_ResolvePerActualTheme()
    {
        // The informational wash is pinned per ActualTheme with the library's
        // own per-theme values. The previous pin read
        // Application.Current.Resources, whose themed lookup served the stale
        // dictionary on Windows 10 after a live theme switch, painting the
        // light wash over the dark theme.
        string code = ReadRepositoryFile("src/DeskBox/Views/SettingsWindow.Navigation.cs");

        Assert.Contains("NoticeInfoBackgroundLight", code, StringComparison.Ordinal);
        Assert.Contains("0x80, 0xF6, 0xF6, 0xF6", code, StringComparison.Ordinal);
        Assert.Contains("NoticeInfoBackgroundDark", code, StringComparison.Ordinal);
        Assert.Contains("0x08, 0xFF, 0xFF, 0xFF", code, StringComparison.Ordinal);
        Assert.Contains("infoBar.ActualTheme == ElementTheme.Dark", code, StringComparison.Ordinal);
        Assert.Contains("AccessibilitySettings().HighContrast", code, StringComparison.Ordinal);
        // WinRT activation must stay lazy: static initializers would run on
        // headless test hosts without WinAppSDK activation.
        Assert.DoesNotContain("static readonly Windows.UI.ViewManagement", code, StringComparison.Ordinal);
        Assert.DoesNotContain("InfoBarInformationalSeverityBackgroundBrush\" out", code, StringComparison.Ordinal);

        // The template's default text colors already follow the theme; the
        // foreground pin (whose key never existed in the library) is gone.
        Assert.DoesNotContain("InfoBarInformationalSeverityForegroundBrush", code, StringComparison.Ordinal);
    }

    [Fact]
    public void SearchSectionNoticeBrushes_DelegateToTheSharedPerThemePin()
    {
        string code = ReadRepositoryFile(
            "src/DeskBox/Views/SettingsSections/SearchSettingsSection.xaml.cs");

        Assert.Contains(
            "SettingsWindow.ApplyInformationalSeverityBrushes(SearchHotkeyElevatedNoticeInfoBar)",
            code,
            StringComparison.Ordinal);
        Assert.DoesNotContain("Application.Current.Resources", code, StringComparison.Ordinal);
    }

    [Fact]
    public void LocalizedService_MapsInfoBarHeaderAndDescriptionKeys()
    {
        string code = ReadRepositoryFile("src/DeskBox/Services/Localized.cs");

        Assert.Contains("case InfoBar infoBar:", code, StringComparison.Ordinal);
        Assert.Contains("infoBar.Title = value;", code, StringComparison.Ordinal);
        Assert.Contains("infoBar.Message = value;", code, StringComparison.Ordinal);
    }

    private static int CountOccurrences(string content, string needle)
    {
        int count = 0;
        int index = 0;
        while ((index = content.IndexOf(needle, index, StringComparison.Ordinal)) >= 0)
        {
            count++;
            index += needle.Length;
        }
        return count;
    }

    private static string ReadRepositoryFile(string relativePath)
    {
        return File.ReadAllText(TestPaths.FromRepository(relativePath));
    }
}
