using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class ExclusiveDropHighlightCoordinatorTests
{
    [Fact]
    public void Activate_ReplacesAndReturnsThePreviousOwner()
    {
        var coordinator = new ExclusiveDropHighlightCoordinator<object>();
        var first = new object();
        var second = new object();

        Assert.Null(coordinator.Activate(first));
        Assert.Null(coordinator.Activate(first));
        Assert.Same(first, coordinator.Activate(second));
    }

    [Fact]
    public void Deactivate_OnlyClearsTheMatchingOwner()
    {
        var coordinator = new ExclusiveDropHighlightCoordinator<object>();
        var first = new object();
        var second = new object();
        var third = new object();

        coordinator.Activate(first);
        coordinator.Activate(second);
        coordinator.Deactivate(first);

        Assert.Same(second, coordinator.Activate(third));
        coordinator.Deactivate(third);
        Assert.Null(coordinator.DeactivateActive());
    }

    [Fact]
    public void DeactivateActive_ReturnsAndClearsTheCurrentOwner()
    {
        var coordinator = new ExclusiveDropHighlightCoordinator<object>();
        var owner = new object();

        coordinator.Activate(owner);

        Assert.Same(owner, coordinator.DeactivateActive());
        Assert.Null(coordinator.DeactivateActive());
    }

    [Fact]
    public void NativeDropBridge_TracksSurfaceCleanupWithoutWindowHighlight()
    {
        string source = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Views/ContentWidgetWindow.NativeDragDrop.cs"));

        Assert.Contains(
            "DragEnterEvent +=",
            source,
            StringComparison.Ordinal);
        Assert.Contains(
            "DragOverEvent +=",
            source,
            StringComparison.Ordinal);
        Assert.Contains(
            "DragLeaveEvent +=",
            source,
            StringComparison.Ordinal);
        Assert.Contains(
            "ObserveNativeDragPointer",
            source,
            StringComparison.Ordinal);
        Assert.Contains(
            "ClearDragSessionVisualState",
            source,
            StringComparison.Ordinal);
        Assert.DoesNotContain(
            "ClearNativeFileDropHighlight",
            source,
            StringComparison.Ordinal);
        Assert.DoesNotContain(
            "SetExclusiveWindowDropTarget",
            source,
            StringComparison.Ordinal);
        Assert.Contains(
            "UIElement.DragLeaveEvent",
            source,
            StringComparison.Ordinal);
        Assert.Contains(
            "GroupFileDrop_DragLeave",
            source,
            StringComparison.Ordinal);
        Assert.Contains(
            "IsPointerOverContentWindow()",
            source,
            StringComparison.Ordinal);
        Assert.Contains(
            "Group drag leave cleared tracking",
            source,
            StringComparison.Ordinal);
    }
}
