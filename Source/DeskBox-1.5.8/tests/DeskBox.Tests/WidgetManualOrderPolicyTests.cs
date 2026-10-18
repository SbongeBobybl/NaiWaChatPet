using DeskBox.Models;

namespace DeskBox.Tests;

public sealed class WidgetManualOrderPolicyTests
{
    [Fact]
    public void ColdStart_restoresPersistedOrder_andAppendsNewItems()
    {
        string[] snapshot = ["C", "A", "D", "B"];
        WidgetItemConfig[] persisted =
        [
            new() { Path = "B", SortOrder = 0 },
            new() { Path = "A", SortOrder = 1 },
            new() { Path = "C", SortOrder = 2 }
        ];

        IReadOnlyList<string> result = WidgetManualOrderPolicy.Reconcile(
            snapshot,
            liveOrderPaths: [],
            persisted,
            path => path);

        Assert.Equal(["B", "A", "C", "D"], result);
    }

    [Fact]
    public void RunningSession_prefersLiveOrder_overPersistedOrder()
    {
        string[] snapshot = ["A", "B", "C", "D"];
        WidgetItemConfig[] persisted =
        [
            new() { Path = "A", SortOrder = 0 },
            new() { Path = "B", SortOrder = 1 },
            new() { Path = "C", SortOrder = 2 }
        ];

        IReadOnlyList<string> result = WidgetManualOrderPolicy.Reconcile(
            snapshot,
            liveOrderPaths: ["C", "A", "B"],
            persisted,
            path => path);

        Assert.Equal(["C", "A", "B", "D"], result);
    }

    [Fact]
    public void ForeignLiveOrder_fallsBackToPersistedOrder()
    {
        // After navigating into a subfolder the live collection still holds
        // that folder's contents; when the mapped root reloads those foreign
        // paths must not outrank the persisted manual order.
        string[] snapshot = ["C", "A", "D", "B"];
        WidgetItemConfig[] persisted =
        [
            new() { Path = "B", SortOrder = 0 },
            new() { Path = "A", SortOrder = 1 },
            new() { Path = "C", SortOrder = 2 }
        ];

        IReadOnlyList<string> result = WidgetManualOrderPolicy.Reconcile(
            snapshot,
            liveOrderPaths: [@"Sub\X", @"Sub\Y"],
            persisted,
            path => path);

        Assert.Equal(["B", "A", "C", "D"], result);
    }

    [Fact]
    public void PartiallyMatchingLiveOrder_keepsLiveRank_forSurvivingItems()
    {
        string[] snapshot = ["A", "B", "C", "D"];
        WidgetItemConfig[] persisted =
        [
            new() { Path = "A", SortOrder = 0 },
            new() { Path = "B", SortOrder = 1 },
            new() { Path = "C", SortOrder = 2 }
        ];

        IReadOnlyList<string> result = WidgetManualOrderPolicy.Reconcile(
            snapshot,
            liveOrderPaths: ["C", "A", @"Other\Gone"],
            persisted,
            path => path);

        Assert.Equal(["C", "A", "B", "D"], result);
    }

    [Fact]
    public void MissingAndDuplicatePaths_areRemoved_withoutDisturbingKnownOrder()
    {
        string[] snapshot = ["B", "B", "C"];
        WidgetItemConfig[] persisted =
        [
            new() { Path = "A", SortOrder = 0 },
            new() { Path = "C", SortOrder = 1 },
            new() { Path = "B", SortOrder = 2 }
        ];

        IReadOnlyList<string> result = WidgetManualOrderPolicy.Reconcile(
            snapshot,
            liveOrderPaths: [],
            persisted,
            path => path);

        Assert.Equal(["C", "B"], result);
    }

    [Fact]
    public void Paths_areMatchedCaseInsensitively()
    {
        string[] snapshot = [@"C:\Files\B.txt", @"C:\Files\A.txt"];
        WidgetItemConfig[] persisted =
        [
            new() { Path = @"c:\files\a.txt", SortOrder = 0 },
            new() { Path = @"c:\files\b.txt", SortOrder = 1 }
        ];

        IReadOnlyList<string> result = WidgetManualOrderPolicy.Reconcile(
            snapshot,
            liveOrderPaths: [],
            persisted,
            path => path);

        Assert.Equal([@"C:\Files\A.txt", @"C:\Files\B.txt"], result);
    }
}
