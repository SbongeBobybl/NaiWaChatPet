using DeskBox.Contracts;
using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WidgetBorderCustomizationTests
{
    [Fact]
    public void ModeOverride_RoundTripsAndFollowGlobalIsNotStored()
    {
        var config = new WidgetConfig();
        Assert.Null(WidgetBorderCustomization.GetModeOverride(config));

        WidgetBorderCustomization.SetModeOverride(config, "accent");
        Assert.Equal(
            WidgetBorderCustomization.ModeAccent,
            WidgetBorderCustomization.GetModeOverride(config));

        WidgetBorderCustomization.SetModeOverride(config, null);
        Assert.Null(WidgetBorderCustomization.GetModeOverride(config));
        Assert.DoesNotContain(
            WidgetBorderCustomization.ModeMetadataKey,
            config.Metadata.Keys);
    }

    [Fact]
    public void Normalize_DropsFollowGlobalEntries()
    {
        var config = new WidgetConfig();
        config.Metadata[WidgetBorderCustomization.ModeMetadataKey] = "FollowGlobal";
        Assert.True(WidgetBorderCustomization.NormalizeOverrides(config));
        Assert.Empty(config.Metadata);

        config.Metadata[WidgetBorderCustomization.ModeMetadataKey] = "None";
        Assert.False(WidgetBorderCustomization.NormalizeOverrides(config));
        Assert.NotNull(WidgetBorderCustomization.GetModeOverride(config));
    }

    [Fact]
    public void FollowGlobal_DropsTheBorderForCustomBackgrounds()
    {
        (string style, string colorMode) = WidgetBorderCustomization.ApplyOverride(
            null,
            "Thin",
            "Neutral",
            hasCustomBackground: true);
        Assert.Equal(WidgetBorderKinds.StyleNone, style);
        Assert.Equal(WidgetBorderKinds.ColorNone, colorMode);

        (style, colorMode) = WidgetBorderCustomization.ApplyOverride(
            null,
            "Thin",
            "Neutral",
            hasCustomBackground: false);
        Assert.Equal(WidgetBorderKinds.StyleThin, style);
        Assert.Equal(WidgetBorderKinds.ColorNeutral, colorMode);
    }

    [Fact]
    public void ExplicitModes_WinOverTheBackgroundHeuristic()
    {
        (string style, string colorMode) = WidgetBorderCustomization.ApplyOverride(
            WidgetBorderCustomization.ModeAccent,
            "Thick",
            "Neutral",
            hasCustomBackground: true);
        Assert.Equal(WidgetBorderKinds.StyleThick, style);
        Assert.Equal(WidgetBorderKinds.ColorAccent, colorMode);

        (style, colorMode) = WidgetBorderCustomization.ApplyOverride(
            WidgetBorderCustomization.ModeNeutral,
            "Medium",
            "Accent",
            hasCustomBackground: true);
        Assert.Equal(WidgetBorderKinds.StyleMedium, style);
        Assert.Equal(WidgetBorderKinds.ColorNeutral, colorMode);

        (style, colorMode) = WidgetBorderCustomization.ApplyOverride(
            WidgetBorderCustomization.ModeNone,
            "Thick",
            "Accent",
            hasCustomBackground: false);
        Assert.Equal(WidgetBorderKinds.StyleNone, style);
        Assert.Equal(WidgetBorderKinds.ColorNone, colorMode);
    }

    [Fact]
    public void ExplicitColor_OnAGloballyBorderlessLook_FallsBackToThin()
    {
        (string style, string colorMode) = WidgetBorderCustomization.ApplyOverride(
            WidgetBorderCustomization.ModeAccent,
            "None",
            "Neutral",
            hasCustomBackground: false);
        Assert.Equal(WidgetBorderKinds.StyleThin, style);
        Assert.Equal(WidgetBorderKinds.ColorAccent, colorMode);
    }

    // ── Wiring contracts ────────────────────────────────────────

    [Theory]
    [InlineData("src/DeskBox/Views/ContentWidgetWindow.Commands.cs")]
    public void TheWidgetMenu_GroupsAppearanceEntriesUnderTheStyleMenu(string path)
    {
        string source = File.ReadAllText(TestPaths.FromRepository(path));

        Assert.Contains("Widget.StyleMenu.Label", source, StringComparison.Ordinal);
        Assert.Contains("WidgetBorderMenuBuilder.Create", source, StringComparison.Ordinal);
        Assert.Contains("WidgetCustomIconAndBackgroundLiveUnderStyleMenu", source, StringComparison.Ordinal);
    }

    [Fact]
    public void BorderComputation_HonorsThePerWidgetOverride()
    {
        string backdrop = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Views/WidgetWindowBase.Backdrop.cs"));
        Assert.Contains(
            "WidgetBorderCustomization.ApplyOverride",
            backdrop,
            StringComparison.Ordinal);
        Assert.Contains(
            "WidgetBackgroundCustomization.HasCustomBackground",
            backdrop,
            StringComparison.Ordinal);

        string settings = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/SettingsService.cs"));
        Assert.Contains(
            "WidgetBorderCustomization.NormalizeOverrides",
            settings,
            StringComparison.Ordinal);
    }
}
