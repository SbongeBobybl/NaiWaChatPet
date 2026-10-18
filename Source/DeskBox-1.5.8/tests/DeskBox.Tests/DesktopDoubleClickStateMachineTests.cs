using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class DesktopDoubleClickStateMachineTests
{
    [Fact]
    public void ActivationService_DoesNotBlanketRejectInjectedUserClicks()
    {
        string service = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/DesktopDoubleClickActivationService.cs"));

        Assert.Contains("bool isInjected =", service, StringComparison.Ordinal);
        Assert.Contains("LLMHF_INJECTED", service, StringComparison.Ordinal);
        Assert.DoesNotContain(
            "if ((data.flags & MouseInjectedFlag) != 0)",
            service,
            StringComparison.Ordinal);
        Assert.Contains("DesktopBlankHitTest.IsBlankDesktopPoint", service, StringComparison.Ordinal);
    }

    [Fact]
    public void ActivationService_DefersExplorerHitTestOutsideLowLevelHook()
    {
        string service = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/DesktopDoubleClickActivationService.cs"));
        string hook = ExtractMethod(service, "private IntPtr MouseHookProc", "private void QueueMouseDownForValidation");

        Assert.Contains("QueueMouseDownForValidation", hook, StringComparison.Ordinal);
        Assert.Contains("CallNextHookEx", hook, StringComparison.Ordinal);
        Assert.DoesNotContain("DesktopBlankHitTest", hook, StringComparison.Ordinal);
        Assert.Contains("ThreadPool.UnsafeQueueUserWorkItem", service, StringComparison.Ordinal);
        Assert.Contains("MaximumQueuedMouseDowns", service, StringComparison.Ordinal);
    }

    [Fact]
    public void NearbyClicksWithinSystemInterval_TriggerOnce()
    {
        var state = new DesktopDoubleClickStateMachine(500, 8, 8);

        Assert.False(state.Process(100, 100, 1000, true, out _));
        Assert.True(state.Process(103, 97, 1400, true, out var sequence));
        Assert.Equal(new DesktopDoubleClickSequence(100, 100, 1000, 103, 97, 1400), sequence);
        Assert.False(state.Process(103, 97, 1410, true, out _));
    }

    [Fact]
    public void LateSecondClick_BecomesFirstClickOfANewPair()
    {
        var state = new DesktopDoubleClickStateMachine(300, 8, 8);

        Assert.False(state.Process(100, 100, 1000, true, out _));
        Assert.False(state.Process(100, 100, 1400, true, out _));
        Assert.True(state.Process(100, 100, 1600, true, out _));
    }

    [Fact]
    public void DistantSecondClick_DoesNotTrigger()
    {
        var state = new DesktopDoubleClickStateMachine(500, 8, 8);

        Assert.False(state.Process(100, 100, 1000, true, out _));
        Assert.False(state.Process(105, 100, 1200, true, out _));
    }

    [Fact]
    public void NonBlankClick_CannotStartOrCompleteDesktopDoubleClick()
    {
        var state = new DesktopDoubleClickStateMachine(500, 8, 8);

        Assert.False(state.Process(100, 100, 1000, false, out _));
        Assert.False(state.Process(100, 100, 1100, true, out _));
        Assert.False(state.Process(100, 100, 1200, false, out _));
        Assert.False(state.Process(100, 100, 1300, true, out _));
        Assert.True(state.Process(100, 100, 1400, true, out _));
    }

    private static string ExtractMethod(string source, string startToken, string endToken)
    {
        int start = source.IndexOf(startToken, StringComparison.Ordinal);
        int end = source.IndexOf(endToken, start, StringComparison.Ordinal);
        Assert.True(start >= 0 && end > start);
        return source[start..end];
    }
}
