using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WidgetTemporaryRaiseLeasePolicyTests
{
    [Theory]
    [InlineData(true, false, false, true)]
    [InlineData(false, false, false, false)]
    [InlineData(true, true, false, false)]
    [InlineData(true, false, true, false)]
    public void DesktopLayerRestore_RequiresVisibleStableWindow(
        bool isVisible,
        bool isHideAnimationRunning,
        bool isClosing,
        bool expected)
    {
        Assert.Equal(expected,
            WidgetTemporaryRaiseLeasePolicy.CanRestoreDesktopLayer(
                isVisible,
                isHideAnimationRunning,
                isClosing));
    }

    [Fact]
    public void SafetyRestore_ArmsForLogicalRaiseEvenWithoutTopMostState()
    {
        Assert.True(WidgetTemporaryRaiseLeasePolicy.ShouldArmSafetyRestore(
            isAtDesktopLayer: false));
        Assert.False(WidgetTemporaryRaiseLeasePolicy.ShouldArmSafetyRestore(
            isAtDesktopLayer: true));
    }

    [Theory]
    [InlineData(true, false, false, false)]
    [InlineData(false, true, false, false)]
    [InlineData(false, false, true, false)]
    [InlineData(false, false, false, true)]
    public void SafetyRestore_DefersOnlyForActiveInteractionState(
        bool isDragging,
        bool isResizing,
        bool hasBlockingFlyout,
        bool isManagerInteractionActive)
    {
        Assert.True(WidgetTemporaryRaiseLeasePolicy.ShouldDeferSafetyRestore(
            isDragging,
            isResizing,
            hasBlockingFlyout,
            isManagerInteractionActive));
        Assert.False(WidgetTemporaryRaiseLeasePolicy.ShouldDeferSafetyRestore(
            false,
            false,
            false,
            false));
    }

    [Fact]
    public void AcquiringNewBatch_PreservesAlreadyDetachedPeersAndAdvancesGeneration()
    {
        WidgetTemporaryRaiseLease first =
            WidgetTemporaryRaiseLeasePolicy.Acquire(
                default,
                [new IntPtr(101), new IntPtr(202)]);
        WidgetTemporaryRaiseLease second =
            WidgetTemporaryRaiseLeasePolicy.Acquire(
                first,
                [new IntPtr(202), new IntPtr(303)]);

        Assert.Equal(new[] { new IntPtr(101), new IntPtr(202), new IntPtr(303) },
            second.ActiveWindowHandles);
        Assert.True(second.Generation > first.Generation);
    }

    [Fact]
    public void StaleDelayedRestore_CannotReleaseNewerTitleInteraction()
    {
        WidgetTemporaryRaiseLease startup =
            WidgetTemporaryRaiseLeasePolicy.Acquire(
                default,
                [new IntPtr(101)]);
        WidgetTemporaryRaiseLease titleInteraction =
            WidgetTemporaryRaiseLeasePolicy.Acquire(
                startup,
                [new IntPtr(202)]);

        WidgetTemporaryRaiseLease afterStaleRestore =
            WidgetTemporaryRaiseLeasePolicy.Release(
                titleInteraction,
                startup.Generation);

        Assert.Equal(titleInteraction, afterStaleRestore);
        Assert.True(afterStaleRestore.IsActive);
    }

    [Fact]
    public void CurrentRestore_ReleasesWholeTemporaryBatch()
    {
        WidgetTemporaryRaiseLease current =
            WidgetTemporaryRaiseLeasePolicy.Acquire(
                default,
                [new IntPtr(101), new IntPtr(202)]);

        WidgetTemporaryRaiseLease released =
            WidgetTemporaryRaiseLeasePolicy.Release(
                current,
                current.Generation);

        Assert.False(released.IsActive);
        Assert.Empty(released.ActiveWindowHandles);
        Assert.Equal(current.Generation, released.Generation);
    }

    [Fact]
    public void ClosedWindow_IsForgottenWithoutInvalidatingRemainingBatch()
    {
        WidgetTemporaryRaiseLease current =
            WidgetTemporaryRaiseLeasePolicy.Acquire(
                default,
                [new IntPtr(101), new IntPtr(202)]);

        WidgetTemporaryRaiseLease remaining =
            WidgetTemporaryRaiseLeasePolicy.Forget(
                current,
                new IntPtr(101));

        Assert.Equal(new[] { new IntPtr(202) }, remaining.ActiveWindowHandles);
        Assert.Equal(current.Generation, remaining.Generation);
    }
}
