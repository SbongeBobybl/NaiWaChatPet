using DeskBox.Controls;
using DeskBox.Controls.WidgetContents;
using Windows.ApplicationModel.DataTransfer;

namespace DeskBox.Tests;

public sealed class FileDragReleaseRecoveryTests
{
    [Fact]
    public void ButtonUpBeforeSystemCompletion_DefersRepeatedRecoveryProbes()
    {
        var session = new FileDragSessionState();
        session.Begin("source-drag");

        // The incident sequence: a pointer probe runs while the Shell is
        // still negotiating Drop. Repeated probes must not end that operation.
        for (int probe = 0; probe < 10; probe++)
        {
            Assert.True(session.DeferReleaseRecovery());
            Assert.True(session.IsSystemDragInProgress);
        }
        Assert.True(session.ReleaseRecoveryPending);

        session.Complete("source-drag");

        Assert.False(session.IsSystemDragInProgress);
        Assert.False(session.DeferReleaseRecovery());
        Assert.False(session.ReleaseRecoveryPending);
    }

    [Fact]
    public void LateCompletionOfPreviousDrag_DoesNotReleaseCurrentDrag()
    {
        var session = new FileDragSessionState();
        session.Begin("previous");
        Assert.True(session.DeferReleaseRecovery());
        session.Begin("current");

        Assert.False(session.ReleaseRecoveryPending);
        session.Complete("previous");

        Assert.True(session.DeferReleaseRecovery());
        session.Complete("current");
        Assert.False(session.DeferReleaseRecovery());
    }

    [Fact]
    public void NoSourceOperation_DoesNotPreventStaleVisualRecovery()
    {
        var session = new FileDragSessionState();

        Assert.False(session.DeferReleaseRecovery());
        session.Begin("normal-drop");
        session.Complete("normal-drop");
        Assert.False(session.DeferReleaseRecovery());
    }

    [Theory]
    [InlineData(DataPackageOperation.None, false, false)] // Escape/cancel.
    [InlineData(DataPackageOperation.None, true, false)] // Already handled.
    [InlineData(DataPackageOperation.Copy, true, false)]
    [InlineData(DataPackageOperation.Link, true, false)]
    [InlineData(DataPackageOperation.Move, true, false)]
    [InlineData(DataPackageOperation.Copy, false, true)]
    [InlineData(DataPackageOperation.Link, false, true)]
    [InlineData(DataPackageOperation.Move, false, true)]
    public void SourceCompletion_CancelOrHandledDropNeverCommitsAnotherReorder(
        DataPackageOperation result, bool handled, bool expected)
    {
        Assert.Equal(expected,
            FileSurfaceContent.ShouldRecoverUnhandledSourceDrop(result, handled));
    }

    [Fact]
    public void PointerRecovery_GatesBeforeChangingShellOrSurfaceState()
    {
        string surface = Source("Controls/WidgetContents/FileSurfaceContent.xaml.cs");
        string recovery = Section(surface, "internal bool CompleteReleasedDragSession()",
            "internal static bool ShouldCommitReleasedSurfaceReorder(");
        AssertBefore(recovery, "ShouldDeferReleasedDragSessionRecovery()", "ClearFolderDropTarget();");
        AssertBefore(recovery, "ShouldDeferReleasedDragSessionRecovery()", "CommitSurfaceReorder(releasePosition);");

        string shell = Section(Source("Controls/WidgetShell.xaml.cs"),
            "internal bool TryClearStaleShellDragSessionAfterPointerRelease()",
            "internal bool TryEndShellDragSessionAfterNativePointerExit()");
        AssertBefore(shell, "ShouldDeferReleasedDragSessionRecovery()", "_isShellDragActive = false;");

        string completed = Section(surface, "private void CompleteDragItemsSession(",
            "internal static bool ShouldRecoverUnhandledSourceDrop(");
        AssertBefore(completed, "_sourceDragSession.Complete(dragSessionId);",
            "TryCompleteReleasedStackPopoverReorder(");
        AssertBefore(completed, "ShouldRecoverUnhandledSourceDrop(", "CompleteReleasedDragSession()");
    }

    [Theory]
    [InlineData("Controls/WidgetContents/FileSurfaceContent.xaml.cs", "private async void Root_Drop(",
        "ClearFolderDropTarget();")]
    [InlineData("Controls/WidgetContents/FileSurfaceContent.ItemVisuals.cs", "private async void StackSurface_Drop(",
        "HideStackPopoverReorderIndicator();")]
    [InlineData("Controls/WidgetContents/FileSurfaceContent.StackPopover.cs", "private void StackPopoverItems_Drop(",
        "ReorderStackPopoverMembers(")]
    public void Drop_RevokesProvisionalMoveBeforeAnyProjectionMutation(
        string path, string start, string mutation)
    {
        string source = Source(path);
        int startIndex = source.IndexOf(start, StringComparison.Ordinal);
        Assert.True(startIndex >= 0);
        string drop = source[startIndex..];
        AssertBefore(drop, "e.AcceptedOperation = DataPackageOperation.None;", mutation);
    }

    private static string Source(string relative) =>
        File.ReadAllText(TestPaths.FromRepository("src/DeskBox/" + relative));

    private static string Section(string source, string start, string end)
    {
        int first = source.IndexOf(start, StringComparison.Ordinal);
        Assert.True(first >= 0);
        int last = source.IndexOf(end, first + start.Length, StringComparison.Ordinal);
        Assert.True(last > first);
        return source[first..last];
    }

    private static void AssertBefore(string source, string first, string later)
    {
        int firstIndex = source.IndexOf(first, StringComparison.Ordinal);
        int laterIndex = source.IndexOf(later, StringComparison.Ordinal);
        Assert.True(firstIndex >= 0 && laterIndex > firstIndex,
            $"Expected '{first}' before '{later}'.");
    }
}
