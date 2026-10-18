using DeskBox.Views;

namespace DeskBox.Tests;

public sealed class SearchPopupMaterialContractTests
{
    [Fact]
    public void PopupSolidSurface_IsFullyOpaqueInBothThemes()
    {
        var accent = Windows.UI.Color.FromArgb(0xFF, 0x00, 0x78, 0xD4);

        var light = SearchPopupWindow.BuildOpaquePopupSurfaceColor(isDark: false, accent, 0.65);
        var dark = SearchPopupWindow.BuildOpaquePopupSurfaceColor(isDark: true, accent, 0.65);

        Assert.Equal(255, light.A);
        Assert.Equal(255, dark.A);
    }

    [Fact]
    public void PopupSolidSurface_ZeroIntensityKeepsThePlainBaseColor()
    {
        var accent = Windows.UI.Color.FromArgb(0xFF, 0x00, 0x78, 0xD4);

        var light = SearchPopupWindow.BuildOpaquePopupSurfaceColor(isDark: false, accent, 0.0);
        var dark = SearchPopupWindow.BuildOpaquePopupSurfaceColor(isDark: true, accent, 0.0);

        Assert.Equal(Windows.UI.Color.FromArgb(0xFF, 0xFF, 0xFF, 0xFF), light);
        Assert.Equal(Windows.UI.Color.FromArgb(0xFF, 0x21, 0x24, 0x2A), dark);
    }

    [Fact]
    public void PopupSolidSurface_BlendsAccentByIntensity()
    {
        var accent = Windows.UI.Color.FromArgb(0xFF, 0x00, 0x78, 0xD4);

        var blended = SearchPopupWindow.BuildOpaquePopupSurfaceColor(isDark: false, accent, 1.0);

        Assert.Equal(Windows.UI.Color.FromArgb(0xFF, 242, 248, 253), blended);
    }

    [Fact]
    public void PopupSolidMaterial_NoLongerAppliesSurfaceOpacity()
    {
        string source = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Views/SearchPopupWindow.xaml.cs"));

        // The solid branch must use the opaque blend; the translucent frosted
        // surface (which rendered black at zero surface opacity) is gone.
        Assert.Contains(
            "BuildOpaquePopupSurfaceColor(isDark, accentColor, materialIntensity)",
            source,
            StringComparison.Ordinal);
        Assert.DoesNotContain("BuildFrostedSurfaceColor", source, StringComparison.Ordinal);
    }
}
