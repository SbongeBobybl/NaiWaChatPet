using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WidgetExpandedLayerLeasePolicyTests
{
    [Fact]
    public void AcquiringAnotherWidget_InvalidatesThePreviousWidgetsCallbacks()
    {
        WidgetExpandedLayerLease first = WidgetExpandedLayerLeasePolicy.Acquire(
            default,
            new IntPtr(101));
        WidgetExpandedLayerLease second = WidgetExpandedLayerLeasePolicy.Acquire(
            first,
            new IntPtr(202));

        Assert.False(WidgetExpandedLayerLeasePolicy.Owns(
            second,
            first.WindowHandle,
            first.Generation));
        Assert.True(WidgetExpandedLayerLeasePolicy.Owns(
            second,
            second.WindowHandle,
            second.Generation));
    }

    [Fact]
    public void StaleCollapseRelease_CannotClearTheNewExpandedWidget()
    {
        WidgetExpandedLayerLease first = WidgetExpandedLayerLeasePolicy.Acquire(
            default,
            new IntPtr(101));
        WidgetExpandedLayerLease second = WidgetExpandedLayerLeasePolicy.Acquire(
            first,
            new IntPtr(202));

        WidgetExpandedLayerLease afterStaleRelease =
            WidgetExpandedLayerLeasePolicy.Release(
                second,
                first.WindowHandle,
                first.Generation);

        Assert.Equal(second, afterStaleRelease);
    }

    [Fact]
    public void CurrentOwnerRelease_ClearsTheHandleButKeepsGenerationMonotonic()
    {
        WidgetExpandedLayerLease current = WidgetExpandedLayerLeasePolicy.Acquire(
            default,
            new IntPtr(101));
        WidgetExpandedLayerLease released = WidgetExpandedLayerLeasePolicy.Release(
            current,
            current.WindowHandle,
            current.Generation);
        WidgetExpandedLayerLease next = WidgetExpandedLayerLeasePolicy.Acquire(
            released,
            new IntPtr(202));

        Assert.False(released.IsActive);
        Assert.True(next.Generation > current.Generation);
        Assert.Equal(new IntPtr(202), next.WindowHandle);
    }

    [Fact]
    public void ReacquiringTheSameWidget_InvalidatesItsOlderRestoreGeneration()
    {
        WidgetExpandedLayerLease first = WidgetExpandedLayerLeasePolicy.Acquire(
            default,
            new IntPtr(101));
        WidgetExpandedLayerLease second = WidgetExpandedLayerLeasePolicy.Acquire(
            first,
            first.WindowHandle);

        Assert.False(WidgetExpandedLayerLeasePolicy.Owns(
            second,
            first.WindowHandle,
            first.Generation));
        Assert.True(WidgetExpandedLayerLeasePolicy.Owns(
            second,
            second.WindowHandle,
            second.Generation));
    }
}
