using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class TrayToggleDecisionPolicyTests
{
    [Theory]
    [InlineData(true, false, false, true)]
    [InlineData(true, true, false, true)]
    [InlineData(false, false, false, false)]
    [InlineData(false, false, true, false)]
    [InlineData(false, true, true, true)]
    [InlineData(false, true, false, false)]
    public void ShouldHide_coversRaisedHiddenForegroundAndBehindStates(
        bool raised,
        bool visible,
        bool foregroundLocal,
        bool expected)
    {
        Assert.Equal(
            expected,
            TrayToggleDecisionPolicy.ShouldHide(
                new TrayToggleDecisionContext(
                    IsDesktopPinnedMode: false,
                    IsQuickRevealMode: false,
                    raised,
                    visible,
                    foregroundLocal)));
    }

    [Theory]
    [InlineData(true, false, false, true)]
    [InlineData(true, true, false, true)]
    [InlineData(false, false, false, false)]
    [InlineData(false, true, false, false)]
    public void ShouldHide_inDesktopPinnedMode_dependsOnlyOnActualVisibility(
        bool visible,
        bool raised,
        bool foregroundLocal,
        bool expected)
    {
        Assert.Equal(
            expected,
            TrayToggleDecisionPolicy.ShouldHide(
                new TrayToggleDecisionContext(
                    IsDesktopPinnedMode: true,
                    IsQuickRevealMode: false,
                    raised,
                    visible,
                    foregroundLocal)));
    }

    [Theory]
    [InlineData(true, false, false, true)]
    [InlineData(true, true, true, true)]
    [InlineData(false, false, false, false)]
    [InlineData(false, true, true, false)]
    public void ShouldHide_inQuickRevealMode_dependsOnlyOnActualVisibility(
        bool visible,
        bool raised,
        bool foregroundLocal,
        bool expected)
    {
        Assert.Equal(
            expected,
            TrayToggleDecisionPolicy.ShouldHide(
                new TrayToggleDecisionContext(
                    IsDesktopPinnedMode: false,
                    IsQuickRevealMode: true,
                    raised,
                    visible,
                    foregroundLocal)));
    }
}
