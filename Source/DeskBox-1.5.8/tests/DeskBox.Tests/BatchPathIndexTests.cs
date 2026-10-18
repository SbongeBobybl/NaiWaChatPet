using DeskBox.Models;
using DeskBox.ViewModels;

namespace DeskBox.Tests;

/// <summary>
/// Phase 2 of the import-performance plan: kill the per-file full-list
/// scans. The binary insert search must be exactly equivalent to the linear
/// first-strictly-greater scan it replaced over sorted input (ties land
/// after the equal run - upper-bound semantics, not classic lower_bound),
/// and the batch path dictionary must stay a scoped membership authority of
/// live references - not the permanent path-to-index map that every prior
/// review rejected as a bug nursery.
/// </summary>
public sealed class BatchPathIndexTests
{
    private static WidgetItem ItemWithSortOrder(int sortOrder, string name) =>
        new() { SortOrder = sortOrder, Name = name, Path = $@"C:\f\{name}" };

    private static readonly Comparison<WidgetItem> BySortOrder =
        (left, right) => left.SortOrder.CompareTo(right.SortOrder);

    private static int LinearUpperBound(
        List<WidgetItem> items,
        WidgetItem candidate,
        Comparison<WidgetItem> compare)
    {
        for (int index = 0; index < items.Count; index++)
        {
            if (compare(candidate, items[index]) < 0)
            {
                return index;
            }
        }

        return items.Count;
    }

    [Fact]
    public void BinarySearch_MatchesLinearLowerBound_IncludingTies()
    {
        // Sorted list with duplicate keys; candidates probe every gap and
        // every element, including keys inside the duplicate run. The binary
        // search and the retired linear scan must agree everywhere - the
        // import path depends on the two being interchangeable.
        List<WidgetItem> items =
        [
            ItemWithSortOrder(1, "a"),
            ItemWithSortOrder(3, "b"),
            ItemWithSortOrder(3, "c"),
            ItemWithSortOrder(3, "d"),
            ItemWithSortOrder(7, "e"),
            ItemWithSortOrder(9, "f"),
        ];

        foreach (int key in new[] { 0, 1, 2, 3, 4, 7, 8, 9, 10 })
        {
            WidgetItem candidate = ItemWithSortOrder(key, "probe");
            Assert.Equal(
                LinearUpperBound(items, candidate, BySortOrder),
                WidgetViewModel.BinarySearchSortedInsertIndex(
                    items.Count,
                    index => items[index],
                    candidate,
                    BySortOrder));
        }
    }

    [Theory]
    [InlineData(new[] { 5 }, 4, 0)]     // before the only element
    [InlineData(new[] { 5 }, 5, 1)]     // equal: after the tie
    [InlineData(new[] { 5 }, 6, 1)]     // after the only element
    [InlineData(new[] { 1, 3, 5 }, 0, 0)]
    [InlineData(new[] { 1, 3, 5 }, 2, 1)]
    [InlineData(new[] { 1, 3, 5 }, 4, 2)]
    [InlineData(new[] { 1, 3, 5 }, 6, 3)]
    [InlineData(new[] { 1, 2, 2, 2, 9 }, 2, 4)]  // ties land after the run
    public void BinarySearch_FindsTheUpperBoundPosition(
        int[] keys,
        int candidateKey,
        int expected)
    {
        var items = keys.Select(key => ItemWithSortOrder(key, $"n{key}")).ToList();
        WidgetItem candidate = ItemWithSortOrder(candidateKey, "probe");

        Assert.Equal(
            expected,
            WidgetViewModel.BinarySearchSortedInsertIndex(
                items.Count,
                index => items[index],
                candidate,
                BySortOrder));
    }

    [Fact]
    public void BinarySearch_EmptyListInsertsAtZero()
    {
        var items = new List<WidgetItem>();
        Assert.Equal(
            0,
            WidgetViewModel.BinarySearchSortedInsertIndex(
                0,
                index => items[index],
                ItemWithSortOrder(1, "a"),
                BySortOrder));
    }

    [Fact]
    public void InsertingByBinarySearch_KeepsTheListSorted()
    {
        // The property the import path relies on: repeated binary-search
        // inserts produce the same sorted sequence the per-file linear scan
        // used to produce.
        List<WidgetItem> sorted = [];
        int[] arrivingKeys = [5, 1, 9, 3, 3, 7, 2, 8, 4, 6, 2, 0];
        foreach (int key in arrivingKeys)
        {
            WidgetItem candidate = ItemWithSortOrder(key, $"n{key}");
            int position = WidgetViewModel.BinarySearchSortedInsertIndex(
                sorted.Count,
                index => sorted[index],
                candidate,
                BySortOrder);
            sorted.Insert(position, candidate);
        }

        int[] expected = arrivingKeys.OrderBy(key => key).ToArray();
        Assert.Equal(expected, sorted.Select(item => item.SortOrder));
        for (int index = 1; index < sorted.Count; index++)
        {
            Assert.True(
                BySortOrder(sorted[index - 1], sorted[index]) <= 0,
                "the sequence must stay non-decreasing after every insert");
        }
    }
}
