using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class DoubleControlHotkeyStateMachineTests
{
    private const uint LeftControl = 0xA2;
    private const uint RightControl = 0xA3;
    private const uint C = 0x43;

    [Fact]
    public void TwoStandaloneControlTapsWithinInterval_TriggerOnce()
    {
        var state = new DoubleControlHotkeyStateMachine();

        Assert.Equal(ReservedHotkeyEventDisposition.PassThrough, state.Process(LeftControl, true, 100));
        Assert.Equal(ReservedHotkeyEventDisposition.PassThrough, state.Process(LeftControl, false, 120));
        Assert.Equal(ReservedHotkeyEventDisposition.PassThrough, state.Process(RightControl, true, 300));
        Assert.Equal(
            ReservedHotkeyEventDisposition.TriggerAndPassThrough,
            state.Process(RightControl, false, 320));
    }

    [Fact]
    public void SlowControlTaps_StartANewPairWithoutTriggering()
    {
        var state = new DoubleControlHotkeyStateMachine(200);
        state.Process(LeftControl, true, 100);
        state.Process(LeftControl, false, 110);
        state.Process(LeftControl, true, 400);

        Assert.Equal(
            ReservedHotkeyEventDisposition.PassThrough,
            state.Process(LeftControl, false, 410));
    }

    [Fact]
    public void ControlChord_CancelsPendingTapAndNeverTriggers()
    {
        var state = new DoubleControlHotkeyStateMachine();
        state.Process(LeftControl, true, 100);
        state.Process(LeftControl, false, 110);
        state.Process(LeftControl, true, 200);
        state.Process(C, true, 210);
        state.Process(C, false, 220);

        Assert.Equal(
            ReservedHotkeyEventDisposition.PassThrough,
            state.Process(LeftControl, false, 230));
    }

    [Fact]
    public void SimultaneousLeftAndRightControl_IsNotADoubleTap()
    {
        var state = new DoubleControlHotkeyStateMachine();
        state.Process(LeftControl, true, 100);
        state.Process(RightControl, true, 110);
        state.Process(LeftControl, false, 120);

        Assert.Equal(
            ReservedHotkeyEventDisposition.PassThrough,
            state.Process(RightControl, false, 130));
    }
}
