using DeskBox.Helpers;

namespace DeskBox.Tests;

public sealed class ShellDesktopDropTargetTests
{
    [Fact]
    public void IsPointerOverDesktop_CanInvokeNativeShellDetection()
    {
        Exception? exception = Record.Exception(
            () => ShellDesktopDropTarget.IsPointerOverDesktop());

        Assert.Null(exception);
    }

    [Fact]
    public void IsDesktopWindow_RejectsMissingWindow()
    {
        Assert.False(ShellDesktopDropTarget.IsDesktopWindow(IntPtr.Zero));
    }

    [Theory]
    [InlineData("Progman")]
    [InlineData("WorkerW")]
    [InlineData("SHELLDLL_DefView")]
    [InlineData("workerw")]
    public void IsDesktopClassChain_AcceptsWindowsShellDesktopClasses(
        string className)
    {
        Assert.True(
            ShellDesktopDropTarget.IsDesktopClassChain([className]));
    }

    [Theory]
    [InlineData("ApplicationFrameWindow")]
    [InlineData("CabinetWClass")]
    [InlineData("SysListView32")]
    [InlineData("")]
    public void IsDesktopClassChain_RejectsNonDesktopWindows(
        string className)
    {
        Assert.False(
            ShellDesktopDropTarget.IsDesktopClassChain([className]));
    }

    [Theory]
    [InlineData("Shell_TrayWnd")]
    [InlineData("Shell_SecondaryTrayWnd")]
    [InlineData("NotifyIconOverflowWindow")]
    public void IsDesktopClassChain_RejectsTaskbarEvenWithDesktopAncestor(
        string taskbarClass)
    {
        Assert.False(ShellDesktopDropTarget.IsDesktopClassChain(
            [taskbarClass, "WorkerW"]));
    }

    [Fact]
    public void IsDesktopClassChain_AcceptsDesktopIconHierarchy()
    {
        Assert.True(ShellDesktopDropTarget.IsDesktopClassChain(
            ["SysListView32", "SHELLDLL_DefView", "WorkerW"]));
    }
}
