using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WidgetBorderVisualCalculatorTests
{
    [Theory]
    [InlineData(SettingsService.WidgetBorderStyleThin, 0.8, 0x18)]
    [InlineData(SettingsService.WidgetBorderStyleMedium, 1.2, 0x30)]
    [InlineData(SettingsService.WidgetBorderStyleThick, 1.6, 0x48)]
    public void NeutralBorder_UsesConfiguredWidgetThicknessAndTone(
        string style,
        double expectedThickness,
        int expectedAlpha)
    {
        WidgetBorderVisuals dark = WidgetBorderVisualCalculator.Resolve(
            style,
            SettingsService.WidgetBorderColorModeNeutral,
            isDark: true,
            Windows.UI.Color.FromArgb(255, 20, 40, 60));
        WidgetBorderVisuals light = WidgetBorderVisualCalculator.Resolve(
            style,
            SettingsService.WidgetBorderColorModeNeutral,
            isDark: false,
            Windows.UI.Color.FromArgb(255, 20, 40, 60));

        Assert.Equal(expectedThickness, dark.Thickness);
        Assert.Equal((byte)expectedAlpha, dark.BorderColor.A);
        Assert.Equal((byte)0xFF, dark.BorderColor.R);
        Assert.Equal(expectedThickness, light.Thickness);
        Assert.Equal((byte)expectedAlpha, light.BorderColor.A);
        Assert.Equal((byte)0x00, light.BorderColor.R);
    }

    [Fact]
    public void AccentBorder_UsesWidgetAccentColorAndAlphaBoost()
    {
        var accent = Windows.UI.Color.FromArgb(255, 20, 40, 60);

        WidgetBorderVisuals visuals = WidgetBorderVisualCalculator.Resolve(
            SettingsService.WidgetBorderStyleThin,
            SettingsService.WidgetBorderColorModeAccent,
            isDark: true,
            accent);

        Assert.Equal(0.8, visuals.Thickness);
        Assert.Equal((byte)0x20, visuals.BorderColor.A);
        Assert.Equal(accent.R, visuals.BorderColor.R);
        Assert.Equal(accent.G, visuals.BorderColor.G);
        Assert.Equal(accent.B, visuals.BorderColor.B);
    }

    [Fact]
    public void NoBorderMode_SuppressesThicknessAndAlpha()
    {
        WidgetBorderVisuals visuals = WidgetBorderVisualCalculator.Resolve(
            SettingsService.WidgetBorderStyleThick,
            SettingsService.WidgetBorderColorModeNone,
            isDark: true,
            Windows.UI.Color.FromArgb(255, 20, 40, 60));

        Assert.Equal(0, visuals.Thickness);
        Assert.Equal(0, visuals.BorderColor.A);
        Assert.Equal(0, visuals.DividerColor.A);
    }
}
