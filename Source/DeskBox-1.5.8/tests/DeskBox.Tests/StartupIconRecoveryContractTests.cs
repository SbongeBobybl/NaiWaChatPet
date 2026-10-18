namespace DeskBox.Tests;

public sealed class StartupIconRecoveryContractTests
{
    [Fact]
    public void StartupIconRecovery_WaitsForHydrationToSettleBeforeRefreshing()
    {
        string source = File.ReadAllText(GetRepoFile(
            "src/DeskBox/Services/WidgetManager.cs"));
        string method = ExtractMethod(
            source,
            "private void QueueVisibleGroupedFileIconRecoveryAfterStartup()");

        Assert.Contains("IsItemHydrationActive", method, StringComparison.Ordinal);
        Assert.Contains("WaitForItemHydrationToSettleAsync", method, StringComparison.Ordinal);

        // The settle gate must sit between the zero-icon predicate and the
        // refresh, so a legitimately slow cold pass is never restarted.
        int gate = method.IndexOf("IsItemHydrationActive", StringComparison.Ordinal);
        int refresh = method.IndexOf("await fileSurface.RefreshAsync()", StringComparison.Ordinal);
        Assert.True(gate >= 0 && refresh > gate,
            "Startup icon recovery must gate the refresh behind hydration settling");
    }

    [Fact]
    public void ItemHydration_ActivityCounter_IsPairedAroundThePass()
    {
        string source = File.ReadAllText(GetRepoFile(
            "src/DeskBox/ViewModels/WidgetViewModel.ItemHydration.cs"));

        string start = ExtractMethod(source, "private void StartItemHydration()");
        Assert.Contains(
            "Interlocked.Increment(ref _itemHydrationActiveCount)",
            start,
            StringComparison.Ordinal);

        string run = ExtractMethod(source, "private async Task RunItemHydrationAsync");
        Assert.Contains(
            "Interlocked.Decrement(ref _itemHydrationActiveCount)",
            run,
            StringComparison.Ordinal);
    }

    private static string ExtractMethod(string source, string signature)
    {
        int start = source.IndexOf(signature, StringComparison.Ordinal);
        Assert.True(start >= 0, $"Missing method signature: {signature}");

        int brace = source.IndexOf('{', start);
        Assert.True(brace >= 0, $"Missing method body: {signature}");
        int depth = 0;
        for (int index = brace; index < source.Length; index++)
        {
            depth += source[index] switch
            {
                '{' => 1,
                '}' => -1,
                _ => 0
            };
            if (depth == 0)
            {
                return source[start..(index + 1)];
            }
        }

        throw new InvalidOperationException($"Unterminated method body: {signature}");
    }

    private static string GetRepoFile(string relativePath)
    {
        string? directory = AppContext.BaseDirectory;
        while (!string.IsNullOrWhiteSpace(directory))
        {
            string candidate = Path.Combine(directory, relativePath.Replace('/', Path.DirectorySeparatorChar));
            if (File.Exists(candidate))
            {
                return candidate;
            }

            directory = Directory.GetParent(directory)?.FullName;
        }

        throw new FileNotFoundException($"Could not locate repository file: {relativePath}");
    }
}
