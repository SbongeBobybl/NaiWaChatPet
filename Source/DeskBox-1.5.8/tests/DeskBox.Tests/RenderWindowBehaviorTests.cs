using System.Collections.ObjectModel;
using DeskBox.Models;
using DeskBox.ViewModels;

namespace DeskBox.Tests;

/// <summary>
/// Real behavior coverage for the render window. A reveal must grow the
/// window just far enough to cover the target item — never to the whole
/// folder. The original loop-based implementation expanded to the full item
/// count whenever the target sat outside the rendered prefix, which is the
/// measured ~50 s UI hang for large folders all over again.
///
/// The budget/target split carries its own regressions here: every 1.5.2
/// dead end ("files exist, ~30 tiles render, nothing scrolls") was a budget
/// corruption — a small folder's full render written back into the budget,
/// or a viewport page clamped to a not-yet-loaded item count collapsing it
/// to zero. The tests below drive the state machine the view model drives:
/// derive the target, reconcile, and only ever raise the budget.
/// </summary>
public sealed class RenderWindowBehaviorTests
{
    [Theory]
    [InlineData(0, 30, 1000, 200)]      // first item: aligned chunk boundary
    [InlineData(49, 30, 1000, 200)]     // just past the window: next chunk boundary
    [InlineData(199, 30, 1000, 200)]
    [InlineData(200, 30, 1000, 400)]    // exactly one past the boundary: two chunks
    [InlineData(949, 30, 1000, 1000)]   // deep item: clamped to the visible count
    [InlineData(949, 30, 2000, 1000)]
    [InlineData(1999, 1999, 2000, 2000)]
    [InlineData(-1, 30, 1000, 30)]      // not in the projection: unchanged
    public void ComputeRenderWindowTargetCount_AlignsToChunksAndClamps(
        int itemIndex,
        int currentCount,
        int visibleCount,
        int expected)
    {
        Assert.Equal(
            expected,
            WidgetViewModel.ComputeRenderWindowTargetCount(itemIndex, currentCount, visibleCount));
    }

    [Theory]
    [InlineData(30, 40, 40)]        // small folder: full render regardless of budget
    [InlineData(30, 299, 299)]      // the activation threshold is inclusive
    [InlineData(30, 301, 30)]       // above the threshold the budget applies
    [InlineData(180, 301, 180)]
    [InlineData(5000, 301, 301)]    // a budget past the item count clamps to it
    [InlineData(180, 0, 0)]         // an empty folder renders nothing
    public void DeriveRenderTarget_FullBelowThresholdAndBudgetClampedAbove(
        int budget,
        int visibleItemCount,
        int expected)
    {
        Assert.Equal(
            expected,
            WidgetViewModel.DeriveRenderTarget(budget, visibleItemCount));
    }

    [Fact]
    public void RevealDeepItem_GrowsTheWindowJustEnoughToCoverIt()
    {
        List<WidgetItem> visible = CreateItems(5000);
        var rendered = new ObservableCollection<WidgetItem>(visible.Take(30));

        WidgetViewModel.ReconcileRenderWindowPrefix(
            visible,
            rendered,
            WidgetViewModel.ComputeRenderWindowTargetCount(949, 30, 5000),
            visible.Count);

        Assert.Equal(1000, rendered.Count);
        Assert.Same(visible[949], rendered[949]);
        Assert.Contains(visible[949], rendered);
        // The tail beyond the grown window must stay unrealized.
        Assert.DoesNotContain(visible[1500], rendered);
        for (int index = 0; index < rendered.Count; index++)
        {
            Assert.Same(visible[index], rendered[index]);
        }
    }

    [Fact]
    public void RevealShallowItem_KeepsTheFolderLargelyUnrendered()
    {
        List<WidgetItem> visible = CreateItems(5000);
        var rendered = new ObservableCollection<WidgetItem>(visible.Take(30));

        WidgetViewModel.ReconcileRenderWindowPrefix(
            visible,
            rendered,
            WidgetViewModel.ComputeRenderWindowTargetCount(49, 30, 5000),
            visible.Count);

        Assert.Equal(200, rendered.Count);
        Assert.Contains(visible[49], rendered);
        Assert.DoesNotContain(visible[200], rendered);
    }

    [Fact]
    public void SmallFolders_AlwaysRenderInFull()
    {
        List<WidgetItem> visible = CreateItems(250);
        var rendered = new ObservableCollection<WidgetItem>(visible.Take(30));

        // The full render is a derived target, not a written-back budget: a
        // 250-item folder derives 250 while the budget itself stays at 30.
        WidgetViewModel.ReconcileRenderWindowPrefix(
            visible,
            rendered,
            WidgetViewModel.DeriveRenderTarget(budget: 30, visibleItemCount: visible.Count),
            visible.Count);

        Assert.Equal(250, rendered.Count);
    }

    [Fact]
    public void SmallFolderThenBulkGrowth_DoesNotKeepSmallWindow()
    {
        // Regression for the 1.5.2 dead end: a 40-item folder rendered in
        // full wrote 40 back into the window budget, so dropping 2000+ files
        // into the same widget reconciled to that stale prefix and nothing
        // could scroll. The budget must survive small-folder full renders
        // untouched; only the viewport coverage path raises it.
        List<WidgetItem> all = CreateItems(2088);
        List<WidgetItem> small = all.Take(40).ToList();
        var rendered = new ObservableCollection<WidgetItem>();
        int budget = 30;

        WidgetViewModel.ReconcileRenderWindowPrefix(
            small,
            rendered,
            WidgetViewModel.DeriveRenderTarget(budget, small.Count),
            small.Count);
        Assert.Equal(40, rendered.Count);

        // The bulk import lands: the coalesced source reconcile still sees
        // the original budget...
        WidgetViewModel.ReconcileRenderWindowPrefix(
            all,
            rendered,
            WidgetViewModel.DeriveRenderTarget(budget, all.Count),
            all.Count);
        Assert.Equal(30, rendered.Count);

        // ...and the viewport check the source change triggers raises the
        // budget to a page, which must clear the small folder's full-render
        // prefix rather than being capped by it.
        int page = WidgetViewModel.ComputeViewportRenderMinimum(
            viewportWidth: 1100,
            viewportHeight: 800,
            itemWidth: 75,
            itemHeight: 85,
            bufferRows: 2);
        budget = Math.Max(budget, page);
        WidgetViewModel.ReconcileRenderWindowPrefix(
            all,
            rendered,
            WidgetViewModel.DeriveRenderTarget(budget, all.Count),
            all.Count);

        Assert.Equal(180, rendered.Count);
        Assert.True(
            WidgetViewModel.DeriveRenderTarget(budget, all.Count) < all.Count,
            "the window must stay growable after covering the viewport");
    }

    [Fact]
    public void ViewportEnsure_BeforeItemsLoad_NeverCollapsesTheBudget()
    {
        // The viewport check can run while the widget is still empty (Loaded
        // fires before a bulk import lands). The old ensure clamped the page
        // to the item count, writing 0 into the budget and leaving the
        // populated folder permanently unrendered. Ensure must be a pure max
        // against the budget; it must not consult the item count.
        int budget = 30;
        int page = WidgetViewModel.ComputeViewportRenderMinimum(
            viewportWidth: 1100,
            viewportHeight: 800,
            itemWidth: 75,
            itemHeight: 85,
            bufferRows: 2);

        budget = Math.Max(budget, page);
        Assert.Equal(0, WidgetViewModel.DeriveRenderTarget(budget, 0));
        Assert.Equal(180, WidgetViewModel.DeriveRenderTarget(budget, 2088));
    }

    [Fact]
    public void CrossActivationThreshold_299To301_StaysGrowableToTheEnd()
    {
        // Crossing 300 activates windowing mid-life: the reconcile trims to
        // the budget, the viewport check the source change triggers regrows
        // past what the smaller folder showed, and chunk growth can still
        // reach the whole folder — the threshold must never strand a cap.
        List<WidgetItem> visible = CreateItems(301);
        var rendered = new ObservableCollection<WidgetItem>();
        int budget = 30;

        WidgetViewModel.ReconcileRenderWindowPrefix(
            visible.Take(299).ToList(),
            rendered,
            WidgetViewModel.DeriveRenderTarget(budget, 299),
            299);
        Assert.Equal(299, rendered.Count);

        WidgetViewModel.ReconcileRenderWindowPrefix(
            visible,
            rendered,
            WidgetViewModel.DeriveRenderTarget(budget, visible.Count),
            visible.Count);
        Assert.Equal(30, rendered.Count);

        budget = Math.Max(budget, 180);
        WidgetViewModel.ReconcileRenderWindowPrefix(
            visible,
            rendered,
            WidgetViewModel.DeriveRenderTarget(budget, visible.Count),
            visible.Count);
        Assert.Equal(180, rendered.Count);

        budget += 200;
        WidgetViewModel.ReconcileRenderWindowPrefix(
            visible,
            rendered,
            WidgetViewModel.DeriveRenderTarget(budget, visible.Count),
            visible.Count);
        Assert.Equal(301, rendered.Count);
    }

    [Fact]
    public void Reconcile_ReordersInPlaceWithoutReset()
    {
        List<WidgetItem> visible = CreateItems(500);
        var rendered = new ObservableCollection<WidgetItem>(visible.Take(200));

        // Simulate a re-sort: swap the first two visible items and reverse a
        // slice well inside the window. The prefix mirror must reuse existing
        // entries (move/insert) so container realization survives.
        (visible[0], visible[1]) = (visible[1], visible[0]);
        visible.Reverse(10, 20);

        WidgetViewModel.ReconcileRenderWindowPrefix(
            visible,
            rendered,
            renderTarget: 200,
            visibleItemCount: visible.Count);

        Assert.Equal(200, rendered.Count);
        for (int index = 0; index < rendered.Count; index++)
        {
            Assert.Same(visible[index], rendered[index]);
        }
    }

    [Fact]
    public void Reconcile_TrimsTheTailWhenTheFolderShrinks()
    {
        List<WidgetItem> visible = CreateItems(500);
        var rendered = new ObservableCollection<WidgetItem>(visible.Take(200));

        visible.RemoveRange(100, 400);

        WidgetViewModel.ReconcileRenderWindowPrefix(
            visible,
            rendered,
            renderTarget: 200,
            visibleItemCount: visible.Count);

        Assert.Equal(100, rendered.Count);
        Assert.Same(visible[99], rendered[99]);
    }

    [Fact]
    public void ComputeViewportRenderMinimum_CoversViewportPlusBuffer()
    {
        // A prefix that never overflows leaves unrendered items unreachable
        // with no scrollbar; the minimum window must come from the viewport
        // itself, not from a fixed constant.
        int minimum = WidgetViewModel.ComputeViewportRenderMinimum(
            viewportWidth: 1100,
            viewportHeight: 800,
            itemWidth: 75,
            itemHeight: 85,
            bufferRows: 2);
        // 15 columns x (10 visible + 2 buffer) rows.
        Assert.Equal(15 * 12, minimum);
    }

    [Fact]
    public void ComputeViewportRenderMinimum_NeverShrinksBelowInitialSize()
    {
        // A tiny widget (small viewport) must not force hundreds of tiles
        // through the growth path either.
        int minimum = WidgetViewModel.ComputeViewportRenderMinimum(
            viewportWidth: 150,
            viewportHeight: 80,
            itemWidth: 75,
            itemHeight: 85,
            bufferRows: 2);
        Assert.True(minimum >= 30);
    }

    [Fact]
    public void ComputeViewportRenderMinimum_InvalidDimensionsKeepInitialSize()
    {
        Assert.Equal(
            30,
            WidgetViewModel.ComputeViewportRenderMinimum(0, 800, 75, 85, 2));
        Assert.Equal(
            30,
            WidgetViewModel.ComputeViewportRenderMinimum(1100, 0, 75, 85, 2));
        Assert.Equal(
            30,
            WidgetViewModel.ComputeViewportRenderMinimum(1100, 800, 0, 85, 2));
        Assert.Equal(
            30,
            WidgetViewModel.ComputeViewportRenderMinimum(1100, 800, 75, 0, 2));
    }

    private static List<WidgetItem> CreateItems(int count)
    {
        var items = new List<WidgetItem>(count);
        for (int index = 0; index < count; index++)
        {
            items.Add(new WidgetItem { Path = $@"C:\folder\item{index:D5}.txt" });
        }

        return items;
    }
}
