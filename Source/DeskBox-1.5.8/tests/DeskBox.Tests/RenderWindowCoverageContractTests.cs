namespace DeskBox.Tests;

/// <summary>
/// Source contract for the render-window coverage loop. "Files exist but
/// only ~30 tiles render and nothing scrolls" is only preventable as a loop:
/// every path that changes the item count must re-check viewport coverage
/// (bulk imports change no geometry, so Loaded/SizeChanged alone never fire),
/// and the budget ensure must be a pure raise that cannot collapse on an
/// empty folder. These pins keep that loop wired as the code evolves.
/// </summary>
public sealed class RenderWindowCoverageContractTests
{
    [Fact]
    public void ViewModel_RaisesSourceChangedInsideTheCoalescedCallback()
    {
        string windowing = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.Windowing.cs"));

        Assert.Contains(
            "internal event Action? RenderWindowSourceChanged;",
            windowing,
            StringComparison.Ordinal);
        // The single raise site lives in the coalesced dispatcher callback,
        // after the reconcile, so the view re-checks coverage against the
        // settled rendered prefix once per dispatcher pass.
        Assert.Contains(
            "RenderWindowSourceChanged?.Invoke();",
            windowing,
            StringComparison.Ordinal);
        Assert.Contains(
            "_renderWindowReconcileQueued = false;",
            windowing,
            StringComparison.Ordinal);
    }

    [Fact]
    public void View_ReactsWithAViewportCheckOnEverySourceChange()
    {
        string renderWindow = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.RenderWindow.cs"));

        Assert.Contains(
            "ViewModel.RenderWindowSourceChanged += ViewModel_RenderWindowSourceChanged;",
            renderWindow,
            StringComparison.Ordinal);
        Assert.Contains(
            "ViewModel.RenderWindowSourceChanged -= ViewModel_RenderWindowSourceChanged;",
            renderWindow,
            StringComparison.Ordinal);
        Assert.Contains(
            "private void ViewModel_RenderWindowSourceChanged() =>",
            renderWindow,
            StringComparison.Ordinal);
        Assert.Contains(
            "QueueRenderWindowViewportCoverage();",
            renderWindow,
            StringComparison.Ordinal);
    }

    [Fact]
    public void EnsureRenderWindowCoversViewport_IsAPureRaiseOfTheBudget()
    {
        string windowing = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.Windowing.cs"));

        Assert.Contains(
            "minimumCount <= _renderWindowBudget",
            windowing,
            StringComparison.Ordinal);
        Assert.Contains(
            "_renderWindowBudget = minimumCount;",
            windowing,
            StringComparison.Ordinal);
        // The 1.5.2 regression clamped the page to the not-yet-loaded item
        // count here, collapsing the budget to zero right before the import
        // landed.
        Assert.DoesNotContain(
            "_renderWindowBudget = Math.Min(",
            windowing,
            StringComparison.Ordinal);
    }
}
