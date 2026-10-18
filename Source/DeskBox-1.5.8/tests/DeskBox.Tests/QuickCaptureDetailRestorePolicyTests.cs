using DeskBox.Models;

namespace DeskBox.Tests;

public sealed class QuickCaptureDetailRestorePolicyTests
{
    [Fact]
    public void SinglePaneList_DoesNotCaptureRememberedDetail()
    {
        Assert.False(QuickCaptureDetailRestorePolicy.ShouldCaptureDetail(
            isDualPane: false,
            isDetailVisibleInSinglePane: false,
            hasDetail: true));
    }

    [Fact]
    public void SinglePaneOpenDetail_CapturesAndRestoresDetail()
    {
        Assert.True(QuickCaptureDetailRestorePolicy.ShouldCaptureDetail(
            isDualPane: false,
            isDetailVisibleInSinglePane: true,
            hasDetail: true));
        Assert.True(QuickCaptureDetailRestorePolicy.ShouldRestoreDetail(
            isDualPane: false,
            wasDetailVisibleInSinglePane: true));
    }

    [Fact]
    public void DualPane_CapturesAndRestoresSelectedDetail()
    {
        Assert.True(QuickCaptureDetailRestorePolicy.ShouldCaptureDetail(
            isDualPane: true,
            isDetailVisibleInSinglePane: false,
            hasDetail: true));
        Assert.True(QuickCaptureDetailRestorePolicy.ShouldRestoreDetail(
            isDualPane: true,
            wasDetailVisibleInSinglePane: false));
    }

    [Fact]
    public void DetailCapturedFromDualPane_DoesNotOpenAfterSwitchingToSinglePane()
    {
        Assert.False(QuickCaptureDetailRestorePolicy.ShouldRestoreDetail(
            isDualPane: false,
            wasDetailVisibleInSinglePane: false));
    }
}
