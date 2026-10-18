using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WidgetTitleIconCustomizationTests : IDisposable
{
    private readonly string _tempRoot;

    public WidgetTitleIconCustomizationTests()
    {
        _tempRoot = Path.Combine(
            Path.GetTempPath(),
            "DeskBox.Tests",
            "title-icon",
            Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(_tempRoot);
    }

    public void Dispose()
    {
        try
        {
            Directory.Delete(_tempRoot, recursive: true);
        }
        catch (IOException)
        {
        }
        catch (UnauthorizedAccessException)
        {
        }
    }

    // ── Emoji validation ────────────────────────────────────────

    [Theory]
    [InlineData(null)]
    [InlineData("")]
    [InlineData("   ")]
    [InlineData("abc")]
    [InlineData("folder icon")]
    public void IsValidEmoji_RejectsNonEmojiText(string? value)
    {
        Assert.False(WidgetTitleIconCustomization.IsValidEmoji(value));
    }

    [Fact]
    public void IsValidEmoji_RejectsControlCharactersAndOverlongValues()
    {
        Assert.False(WidgetTitleIconCustomization.IsValidEmoji("😀\u0007"));
        string overlong = string.Concat(
            Enumerable.Repeat("😀", WidgetTitleIconCustomization.MaxEmojiLength + 1));
        Assert.False(WidgetTitleIconCustomization.IsValidEmoji(overlong));
    }

    [Theory]
    [InlineData("😀")]
    [InlineData("📁")]
    [InlineData("👨‍👩‍👧‍👦")]
    [InlineData("❤️")]
    public void IsValidEmoji_AcceptsEmojiSequences(string value)
    {
        Assert.True(WidgetTitleIconCustomization.IsValidEmoji(value));
    }

    // ── Override storage semantics ──────────────────────────────

    [Fact]
    public void EmojiOverride_RoundTripsAndClearsImageOverride()
    {
        var config = new WidgetConfig();
        WidgetTitleIconCustomization.SetImageOverride(config, "title-icon.png");
        WidgetTitleIconCustomization.SetEmojiOverride(config, "😀");

        Assert.Equal("😀", WidgetTitleIconCustomization.GetEmojiOverride(config));
        Assert.Null(WidgetTitleIconCustomization.GetImageFileNameOverride(config));
        Assert.True(WidgetTitleIconCustomization.HasCustomIcon(config));
    }

    [Fact]
    public void ImageOverride_RoundTripsAndClearsEmojiOverride()
    {
        var config = new WidgetConfig();
        WidgetTitleIconCustomization.SetEmojiOverride(config, "😀");
        WidgetTitleIconCustomization.SetImageOverride(config, "title-icon.png");

        Assert.Equal("title-icon.png", WidgetTitleIconCustomization.GetImageFileNameOverride(config));
        Assert.Null(WidgetTitleIconCustomization.GetEmojiOverride(config));
    }

    [Fact]
    public void Clear_RemovesBothOverrides()
    {
        var config = new WidgetConfig();
        WidgetTitleIconCustomization.SetEmojiOverride(config, "😀");
        WidgetTitleIconCustomization.Clear(config);

        Assert.False(WidgetTitleIconCustomization.HasCustomIcon(config));
        Assert.Empty(config.Metadata);
    }

    [Fact]
    public void NormalizeOverrides_DropsInvalidValuesAndDuplicateSelection()
    {
        var config = new WidgetConfig();
        config.Metadata[WidgetTitleIconCustomization.EmojiMetadataKey] = "not emoji";
        config.Metadata[WidgetTitleIconCustomization.ImageMetadataKey] = "title-icon.gif";

        bool changed = WidgetTitleIconCustomization.NormalizeOverrides(config);

        Assert.True(changed);
        Assert.False(WidgetTitleIconCustomization.HasCustomIcon(config));

        config.Metadata[WidgetTitleIconCustomization.EmojiMetadataKey] = "😀";
        config.Metadata[WidgetTitleIconCustomization.ImageMetadataKey] = "title-icon.png";
        Assert.True(WidgetTitleIconCustomization.NormalizeOverrides(config));
        Assert.Equal("😀", WidgetTitleIconCustomization.GetEmojiOverride(config));
        Assert.Null(WidgetTitleIconCustomization.GetImageFileNameOverride(config));
    }

    [Theory]
    [InlineData("title-icon.png", true)]
    [InlineData("title-icon.PNG", true)]
    [InlineData("title-icon.jpeg", true)]
    [InlineData("title-icon.svg", true)]
    [InlineData("title-icon.gif", false)]
    [InlineData("title-icon.webp", false)]
    [InlineData("other.png", false)]
    [InlineData("folder/title-icon.png", false)]
    [InlineData("title-icon", false)]
    [InlineData("", false)]
    public void ImageFileNameValidation_FollowsSupportedFormats(string fileName, bool expected)
    {
        Assert.Equal(expected, WidgetTitleIconCustomization.IsValidImageFileName(fileName));
    }

    // ── Asset store ─────────────────────────────────────────────

    [Fact]
    public void GetStoredFileName_NormalizesExtension()
    {
        Assert.Equal(
            "title-icon.png",
            WidgetTitleIconAssetStore.GetStoredFileName(@"C:\pics\Icon.PNG"));
    }

    [Fact]
    public async Task CopyImageAsync_ValidatesBeforeCopying()
    {
        var store = new WidgetTitleIconAssetStore(_tempRoot);

        string gif = Path.Combine(_tempRoot, "sample.gif");
        await File.WriteAllTextAsync(gif, "x");
        Assert.Equal(
            WidgetTitleIconAssetResult.UnsupportedFormat,
            await store.CopyImageAsync("widget-1", gif));

        Assert.Equal(
            WidgetTitleIconAssetResult.FileMissing,
            await store.CopyImageAsync("widget-1", Path.Combine(_tempRoot, "absent.png")));

        string large = Path.Combine(_tempRoot, "large.png");
        await using (FileStream stream = File.Create(large))
        {
            stream.SetLength(WidgetTitleIconCustomization.MaxImageFileBytes + 1);
        }

        Assert.Equal(
            WidgetTitleIconAssetResult.TooLarge,
            await store.CopyImageAsync("widget-1", large));
    }

    [Fact]
    public async Task CopyImageAsync_CopiesAndReplacesStaleExtensionFiles()
    {
        var store = new WidgetTitleIconAssetStore(_tempRoot);

        string png = Path.Combine(_tempRoot, "one.png");
        string jpg = Path.Combine(_tempRoot, "two.jpg");
        await File.WriteAllTextAsync(png, "png-bytes");
        await File.WriteAllTextAsync(jpg, "jpg-bytes");

        Assert.Equal(
            WidgetTitleIconAssetResult.Copied,
            await store.CopyImageAsync("widget-1", png));
        Assert.Equal(
            WidgetTitleIconAssetResult.Copied,
            await store.CopyImageAsync("widget-1", jpg));

        string assetDirectory = store.GetAssetDirectory("widget-1");
        Assert.Equal(["title-icon.jpg"], Directory.GetFiles(assetDirectory).Select(Path.GetFileName));
        Assert.NotNull(store.ResolveImagePath("widget-1", "title-icon.jpg"));
        Assert.Null(store.ResolveImagePath("widget-1", "title-icon.png"));
        Assert.Equal(
            "jpg-bytes",
            await File.ReadAllTextAsync(store.ResolveImagePath("widget-1", "title-icon.jpg")!));
    }

    [Fact]
    public async Task DeleteImage_AndDeleteWidgetAssets_RemoveFiles()
    {
        var store = new WidgetTitleIconAssetStore(_tempRoot);
        string png = Path.Combine(_tempRoot, "icon.png");
        await File.WriteAllTextAsync(png, "x");
        await store.CopyImageAsync("widget-1", png);

        store.DeleteImage("widget-1", "title-icon.png");
        Assert.Null(store.ResolveImagePath("widget-1", "title-icon.png"));

        await store.CopyImageAsync("widget-1", png);
        store.DeleteWidgetAssets("widget-1");
        Assert.False(Directory.Exists(store.GetAssetDirectory("widget-1")));
    }

    [Fact]
    public async Task RecentEmojis_MoveToFront_Dedupe_AndStayBounded()
    {
        var store = new WidgetTitleIconAssetStore(_tempRoot);
        Assert.Empty(await store.LoadRecentEmojisAsync());

        await store.AddRecentEmojiAsync("😀");
        await store.AddRecentEmojiAsync("📁");
        await store.AddRecentEmojiAsync("😀");
        Assert.Equal(["😀", "📁"], await store.LoadRecentEmojisAsync());

        foreach (string emoji in WidgetTitleIconEmojiCatalog.GetAllEmojis()
                     .Take(WidgetTitleIconAssetStore.MaxRecentEmojis + 4))
        {
            await store.AddRecentEmojiAsync(emoji);
        }

        IReadOnlyList<string> recent = await store.LoadRecentEmojisAsync();
        Assert.Equal(WidgetTitleIconAssetStore.MaxRecentEmojis, recent.Count);
        Assert.Distinct(recent);
    }

    // ── Emoji catalog ───────────────────────────────────────────

    [Fact]
    public void EmojiCatalog_EntriesAreValidAndUnique()
    {
        Assert.NotEmpty(WidgetTitleIconEmojiCatalog.Categories);
        string[] all = WidgetTitleIconEmojiCatalog.GetAllEmojis();
        Assert.NotEmpty(all);
        Assert.Equal(all.Length, all.Distinct().Count());

        foreach (WidgetEmojiCategory category in WidgetTitleIconEmojiCatalog.Categories)
        {
            Assert.NotEmpty(category.Id);
            Assert.NotEmpty(category.Emojis);
            Assert.All(category.Emojis, emoji =>
                Assert.True(WidgetTitleIconCustomization.IsValidEmoji(emoji)));
        }
    }

    // ── Wiring contracts ────────────────────────────────────────

    [Fact]
    public void TheWidgetMenu_ExposesTheIconCustomizer()
    {
        Assert.Contains(
            "Widget.CustomIcon.MenuLabel",
            File.ReadAllText(TestPaths.FromRepository(
                "src/DeskBox/Views/ContentWidgetWindow.Commands.cs")),
            StringComparison.Ordinal);
    }

    [Fact]
    public void ShellBindsCustomIconOntoEveryTitleIconHost()
    {
        string xaml = File.ReadAllText(
            TestPaths.FromRepository("src/DeskBox/Controls/WidgetShell.xaml"));

        Assert.Equal(3, CountOccurrences(xaml, "CustomEmoji=\"{x:Bind TitleIconCustomEmoji"));
        Assert.Equal(3, CountOccurrences(xaml, "CustomImageSource=\"{x:Bind TitleIconCustomSource"));

        string iconXaml = File.ReadAllText(
            TestPaths.FromRepository("src/DeskBox/Controls/WidgetTitleIcon.xaml"));
        Assert.Contains("CustomEmojiElement", iconXaml, StringComparison.Ordinal);
        Assert.Contains("CustomImage", iconXaml, StringComparison.Ordinal);
    }

    [Fact]
    public void SettingsNormalization_CoversCustomTitleIconOverrides()
    {
        string settings = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/SettingsService.cs"));
        Assert.Contains(
            "WidgetTitleIconCustomization.NormalizeOverrides",
            settings,
            StringComparison.Ordinal);
    }

    private static int CountOccurrences(string text, string value)
    {
        int count = 0;
        int index = 0;
        while ((index = text.IndexOf(value, index, StringComparison.Ordinal)) >= 0)
        {
            count++;
            index += value.Length;
        }

        return count;
    }
}
