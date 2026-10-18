using DeskBox.Controls;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WidgetGroupSwitchRequestCoordinatorTests
{
    [Fact]
    public void Begin_CancelsPreviousRequestForTheSameGroup()
    {
        var coordinator = new WidgetGroupSwitchRequestCoordinator();
        WidgetGroupSwitchRequest first = coordinator.Begin("group", "b");

        WidgetGroupSwitchRequest second = coordinator.Begin("group", "c");

        Assert.True(first.CancellationToken.IsCancellationRequested);
        Assert.False(second.CancellationToken.IsCancellationRequested);
        Assert.False(coordinator.IsCurrent(first));
        Assert.True(coordinator.IsCurrent(second));
    }

    [Fact]
    public void Begin_PreservesTheInputOriginForTheLatestIntent()
    {
        var coordinator = new WidgetGroupSwitchRequestCoordinator();

        WidgetGroupSwitchRequest request = coordinator.Begin(
            "group",
            "b",
            WidgetGroupSwitchOrigin.Wheel);

        Assert.Equal(WidgetGroupSwitchOrigin.Wheel, request.Origin);
    }

    [Fact]
    public void Begin_DoesNotCancelRequestsForOtherGroups()
    {
        var coordinator = new WidgetGroupSwitchRequestCoordinator();
        WidgetGroupSwitchRequest first = coordinator.Begin("group-1", "b");

        WidgetGroupSwitchRequest second = coordinator.Begin("group-2", "y");

        Assert.False(first.CancellationToken.IsCancellationRequested);
        Assert.False(second.CancellationToken.IsCancellationRequested);
        Assert.True(coordinator.IsCurrent(first));
        Assert.True(coordinator.IsCurrent(second));
    }

    [Fact]
    public void Begin_SameTarget_CancelsThePreviousRequest()
    {
        var coordinator = new WidgetGroupSwitchRequestCoordinator();
        WidgetGroupSwitchRequest first = coordinator.Begin("surface", "member-a");

        WidgetGroupSwitchRequest second = coordinator.Begin("surface", "member-a");

        Assert.True(first.CancellationToken.IsCancellationRequested);
        Assert.False(coordinator.IsCurrent(first));
        Assert.False(second.CancellationToken.IsCancellationRequested);
        Assert.True(coordinator.IsCurrent(second));
    }

    [Fact]
    public void IsCurrentTarget_RecognizesDuplicateIntentWithoutCancelingIt()
    {
        var coordinator = new WidgetGroupSwitchRequestCoordinator();
        WidgetGroupSwitchRequest request = coordinator.Begin(
            "surface",
            "member-a",
            WidgetGroupSwitchOrigin.Wheel);

        Assert.True(coordinator.IsCurrentTarget("surface", "member-a"));
        Assert.False(coordinator.IsCurrentTarget("surface", "member-b"));
        Assert.False(request.CancellationToken.IsCancellationRequested);
        Assert.True(coordinator.IsCurrent(request));
    }

    [Fact]
    public void CompletingSupersededRequest_DoesNotRemoveLatestRequest()
    {
        var coordinator = new WidgetGroupSwitchRequestCoordinator();
        WidgetGroupSwitchRequest first = coordinator.Begin("group", "b");
        WidgetGroupSwitchRequest second = coordinator.Begin("group", "c");

        coordinator.Complete(first);

        Assert.True(coordinator.IsCurrent(second));
        Assert.False(second.CancellationToken.IsCancellationRequested);
    }

    [Fact]
    public void Cancel_CancelsAndRemovesCurrentRequest()
    {
        var coordinator = new WidgetGroupSwitchRequestCoordinator();
        WidgetGroupSwitchRequest request = coordinator.Begin("group", "b");

        coordinator.Cancel("group");

        Assert.True(request.CancellationToken.IsCancellationRequested);
        Assert.False(coordinator.IsCurrent(request));
    }

    [Fact]
    public void CancelAll_CancelsIndependentSurfaceRequests()
    {
        var coordinator = new WidgetGroupSwitchRequestCoordinator();
        WidgetGroupSwitchRequest first = coordinator.Begin("surface-1", "b");
        WidgetGroupSwitchRequest second = coordinator.Begin("surface-2", "y");

        coordinator.CancelAll();

        Assert.True(first.CancellationToken.IsCancellationRequested);
        Assert.True(second.CancellationToken.IsCancellationRequested);
        Assert.False(coordinator.IsCurrent(first));
        Assert.False(coordinator.IsCurrent(second));
    }

    [Fact]
    public void OneHundredRapidRequests_LeaveOnlyTheLastIntentCurrent()
    {
        var coordinator = new WidgetGroupSwitchRequestCoordinator();
        var requests = Enumerable.Range(0, 100)
            .Select(index => coordinator.Begin("surface", $"member-{index}"))
            .ToList();

        Assert.All(
            requests.Take(99),
            request =>
            {
                Assert.True(request.CancellationToken.IsCancellationRequested);
                Assert.False(coordinator.IsCurrent(request));
            });
        Assert.False(requests[^1].CancellationToken.IsCancellationRequested);
        Assert.True(coordinator.IsCurrent(requests[^1]));
    }

}
