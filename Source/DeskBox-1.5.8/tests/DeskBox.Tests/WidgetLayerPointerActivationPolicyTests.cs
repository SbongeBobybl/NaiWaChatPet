using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WidgetLayerPointerActivationPolicyTests
{
    [Theory]
    [InlineData(false, true, false, false, false, false)]
    [InlineData(true, false, false, false, false, false)]
    [InlineData(true, true, true, false, false, false)]
    [InlineData(true, true, false, true, false, false)]
    [InlineData(true, true, false, false, false, true)]
    [InlineData(true, true, true, false, true, true)]
    [InlineData(true, true, false, true, true, true)]
    [InlineData(true, false, false, false, true, true)]
    [InlineData(false, true, false, false, true, false)]
    public void PointerActivation_IsSuppressedForCoveredPinnedWidgetOrForeignForeground(
        bool usesDesktopPinnedMode,
        bool hasForegroundWindow,
        bool foregroundIsDesktopShell,
        bool foregroundIsWidget,
        bool coveredByForeignWindow,
        bool expected)
    {
        Assert.Equal(
            expected,
            WidgetLayerPointerActivationPolicy.ShouldSuppress(
                usesDesktopPinnedMode,
                hasForegroundWindow,
                foregroundIsDesktopShell,
                foregroundIsWidget,
                coveredByForeignWindow));
    }

    [Theory]
    [InlineData(false, false, false)]
    [InlineData(false, true, false)]
    [InlineData(true, false, false)]
    [InlineData(true, true, true)]
    public void ActivatingClick_IsPreservedOnlyDuringAnActiveQuickReveal(
        bool usesQuickRevealMode,
        bool widgetsRaisedFromTray,
        bool expected)
    {
        Assert.Equal(
            expected,
            WidgetLayerPointerActivationPolicy.ShouldPreserveActivatingClick(
                usesQuickRevealMode,
                widgetsRaisedFromTray));
    }
}
