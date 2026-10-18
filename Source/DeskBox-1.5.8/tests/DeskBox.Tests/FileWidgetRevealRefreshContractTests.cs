namespace DeskBox.Tests;

public sealed class FileWidgetRevealRefreshContractTests
{
    [Fact]
    public void DiskReconciliation_PreservesHydratedIconsWhenSnapshotOmitsThem()
    {
        string hydration = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.ItemHydration.cs"));
        string merge = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.SortingAndWatchers.cs"));

        Assert.Equal(
            2,
            CountOccurrences(
                hydration,
                "preserveExistingIconWhenMissing: true"));
        Assert.Contains(
            "if (!preserveExistingIconWhenMissing || source.Icon is not null)",
            merge,
            StringComparison.Ordinal);
        Assert.Contains("target.Icon = source.Icon;", merge, StringComparison.Ordinal);
    }

    private static int CountOccurrences(string source, string value)
    {
        int count = 0;
        int index = 0;
        while ((index = source.IndexOf(value, index, StringComparison.Ordinal)) >= 0)
        {
            count++;
            index += value.Length;
        }

        return count;
    }
}
