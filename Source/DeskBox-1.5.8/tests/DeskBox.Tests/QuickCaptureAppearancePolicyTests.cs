using DeskBox.Models;

namespace DeskBox.Tests;

public sealed class QuickCaptureAppearancePolicyTests
{
    [Theory]
    [InlineData(QuickCaptureAppearancePreset.Default)]
    [InlineData(QuickCaptureAppearancePreset.Paper)]
    [InlineData(QuickCaptureAppearancePreset.StickyYellow)]
    [InlineData(QuickCaptureAppearancePreset.Rose)]
    [InlineData(QuickCaptureAppearancePreset.Mint)]
    [InlineData(QuickCaptureAppearancePreset.MistBlue)]
    public void RecentClipboardItem_AlwaysUsesDefaultListMaterial(
        QuickCaptureAppearancePreset requestedPreset)
    {
        Assert.Equal(
            QuickCaptureAppearancePreset.Default,
            QuickCaptureAppearancePolicy.ResolveListPreset(
                requestedPreset,
                isRecent: true));
    }

    [Theory]
    [InlineData(QuickCaptureAppearancePreset.Default)]
    [InlineData(QuickCaptureAppearancePreset.Paper)]
    [InlineData(QuickCaptureAppearancePreset.StickyYellow)]
    [InlineData(QuickCaptureAppearancePreset.Rose)]
    [InlineData(QuickCaptureAppearancePreset.Mint)]
    [InlineData(QuickCaptureAppearancePreset.MistBlue)]
    public void SavedRecord_PreservesItsSelectedListMaterial(
        QuickCaptureAppearancePreset requestedPreset)
    {
        Assert.Equal(
            requestedPreset,
            QuickCaptureAppearancePolicy.ResolveListPreset(
                requestedPreset,
                isRecent: false));
    }
}
