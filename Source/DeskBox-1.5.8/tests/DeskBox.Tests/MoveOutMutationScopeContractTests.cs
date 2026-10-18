namespace DeskBox.Tests;

/// <summary>
/// Source contract for the move-out mutation scope. Allocation tracing
/// (2026-09-17 memory investigation) convicted the external drag-out path:
/// HandleItemsMovedOutAsync's per-item RemoveItemByPath paid the AddedAt
/// persistence - one UpdateWidget/SaveDebounced reschedule - once per
/// departing file, 2000 times for a bulk drag-out. The removals (and the
/// stack-override cleanup that follows them) must run inside the batch
/// mutation scope so the derived work folds into one finalization, exactly
/// as the import direction has done since the phase-1 batch scope.
/// </summary>
public sealed class MoveOutMutationScopeContractTests
{
    [Fact]
    public void MoveOut_RemovalsRunInsideTheMutationScope()
    {
        string operations = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.Operations.cs"))
            .Replace("\r\n", "\n");

        int handler = operations.IndexOf(
            "public Task HandleItemsMovedOutAsync(",
            StringComparison.Ordinal);
        Assert.True(handler > 0, "the move-out handler must exist");

        int scope = operations.IndexOf(
            "using (EnterItemMutationScope())",
            handler,
            StringComparison.Ordinal);
        int removalLoop = operations.IndexOf(
            "RemoveItemByPath(path);",
            handler,
            StringComparison.Ordinal);
        int stackOverrides = operations.IndexOf(
            "RemoveStackMemberOverridePaths(normalizedPaths);",
            handler,
            StringComparison.Ordinal);

        // The scope must open before the per-item removal loop and still be
        // open around the stack-override cleanup: both mutate the item
        // universe, so both belong to the batch.
        Assert.True(scope > handler && removalLoop > scope,
            "the per-item removal loop must run inside the mutation scope");
        Assert.True(stackOverrides > removalLoop,
            "the stack-override cleanup must also run inside the scope");
        string body = operations[handler..stackOverrides];
        Assert.DoesNotContain("Dispose", body[..^0] + "}", StringComparison.Ordinal);
    }

    [Fact]
    public void MoveOut_IsTheThirdScopeConsumer_BesidesTheTwoImportLoops()
    {
        // The import loops (transfer results + manual insertion) and the
        // move-out handler are the three bulk mutation entry points; a fourth
        // caller means new derived-work bypass risk and deserves its own
        // review.
        string operations = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/WidgetViewModel.Operations.cs"));

        int count = 0;
        int index = 0;
        while ((index = operations.IndexOf(
            "EnterItemMutationScope()",
            index,
            StringComparison.Ordinal)) >= 0)
        {
            count++;
            index += "EnterItemMutationScope()".Length;
        }

        Assert.Equal(3, count);
    }
}
