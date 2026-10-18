using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class RaisedBandGuestPolicyTests
{
    [Theory]
    [InlineData(true, true, true)]
    [InlineData(true, false, false)]
    [InlineData(false, true, false)]
    [InlineData(false, false, false)]
    public void ShouldHoldGuest_onlyDuringQuickRevealRaisedSessions(
        bool usesQuickRevealMode,
        bool widgetsRaisedFromTray,
        bool expected)
    {
        Assert.Equal(
            expected,
            RaisedBandGuestPolicy.ShouldHoldGuest(
                usesQuickRevealMode,
                widgetsRaisedFromTray));
    }

    [Theory]
    [InlineData(true, true)]
    [InlineData(false, false)]
    public void ResolveReleasePlacement_dropsBelowForeignAppOnlyWhenForeignAppIsActive(
        bool foreignWindowIsForeground,
        bool expectedBelowForeignForeground)
    {
        RaisedBandReleasePlacement placement =
            RaisedBandGuestPolicy.ResolveReleasePlacement(foreignWindowIsForeground);
        Assert.Equal(
            expectedBelowForeignForeground,
            placement == RaisedBandReleasePlacement.BelowForeignForeground);
    }

    [Theory]
    [InlineData(3, 3, true)]
    [InlineData(3, 4, true)]
    [InlineData(2, 3, true)]
    [InlineData(4, 3, false)]
    [InlineData(5, 3, false)]
    public void ShouldSweepGuest_onlyForEndedOrOlderSessions(
        long guestSessionGeneration,
        long endedSessionGeneration,
        bool expected)
    {
        Assert.Equal(
            expected,
            RaisedBandGuestPolicy.ShouldSweepGuest(
                guestSessionGeneration,
                endedSessionGeneration));
    }
}
