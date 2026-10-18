using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WinSpaceHotkeyStateMachineTests
{
    private const uint LeftWindows = 0x5B;
    private const uint RightWindows = 0x5C;
    private const uint LeftControl = 0xA2;
    private const uint LeftShift = 0xA0;
    private const uint LeftAlt = 0xA4;
    private const uint Space = 0x20;

    [Theory]
    [InlineData(LeftWindows)]
    [InlineData(RightWindows)]
    public void ExactWinSpace_TriggersOnceAndSuppressesItsKeyPair(uint windowsKey)
    {
        var state = new WinSpaceHotkeyStateMachine();

        Assert.Equal(ReservedHotkeyEventDisposition.PassThrough, state.Process(windowsKey, true));
        Assert.Equal(ReservedHotkeyEventDisposition.TriggerAndSuppress, state.Process(Space, true));
        Assert.Equal(ReservedHotkeyEventDisposition.Suppress, state.Process(Space, true));
        Assert.Equal(ReservedHotkeyEventDisposition.Suppress, state.Process(Space, false));
        Assert.Equal(ReservedHotkeyEventDisposition.PassThrough, state.Process(windowsKey, false));
    }

    [Fact]
    public void SpaceWithoutWindows_PassesThrough()
    {
        var state = new WinSpaceHotkeyStateMachine();

        Assert.Equal(ReservedHotkeyEventDisposition.PassThrough, state.Process(Space, true));
        Assert.Equal(ReservedHotkeyEventDisposition.PassThrough, state.Process(Space, false));
    }

    [Theory]
    [InlineData(LeftControl)]
    [InlineData(LeftShift)]
    public void AdditionalModifier_DoesNotTriggerReservedGesture(uint additionalModifier)
    {
        var state = new WinSpaceHotkeyStateMachine();

        state.Process(LeftWindows, true);
        state.Process(additionalModifier, true);

        Assert.Equal(ReservedHotkeyEventDisposition.PassThrough, state.Process(Space, true));
        Assert.Equal(ReservedHotkeyEventDisposition.PassThrough, state.Process(Space, false));
    }

    [Fact]
    public void FailedNotification_CanRestoreNormalSpaceDelivery()
    {
        var state = new WinSpaceHotkeyStateMachine();
        state.Process(LeftWindows, true);
        Assert.Equal(ReservedHotkeyEventDisposition.TriggerAndSuppress, state.Process(Space, true));

        state.CancelSuppression();

        Assert.Equal(ReservedHotkeyEventDisposition.PassThrough, state.Process(Space, false));
    }

    [Fact]
    public void ReleasingWindowsFirst_StillSuppressesMatchedSpaceUp()
    {
        var state = new WinSpaceHotkeyStateMachine();
        state.Process(LeftWindows, true);
        state.Process(Space, true);

        Assert.Equal(ReservedHotkeyEventDisposition.PassThrough, state.Process(LeftWindows, false));
        Assert.Equal(ReservedHotkeyEventDisposition.Suppress, state.Process(Space, false));
    }

    [Fact]
    public void ExactAltSpace_TriggersAndSuppressesItsKeyPair()
    {
        var state = new WinSpaceHotkeyStateMachine(ReservedHotkeyMode.AltSpace);

        Assert.Equal(ReservedHotkeyEventDisposition.PassThrough, state.Process(LeftAlt, true));
        Assert.Equal(ReservedHotkeyEventDisposition.TriggerAndSuppress, state.Process(Space, true));
        Assert.Equal(ReservedHotkeyEventDisposition.Suppress, state.Process(Space, false));
        Assert.Equal(ReservedHotkeyEventDisposition.PassThrough, state.Process(LeftAlt, false));
    }

    [Fact]
    public void AltSpace_WithWindowsModifier_PassesThrough()
    {
        var state = new WinSpaceHotkeyStateMachine(ReservedHotkeyMode.AltSpace);
        state.Process(LeftAlt, true);
        state.Process(LeftWindows, true);

        Assert.Equal(ReservedHotkeyEventDisposition.PassThrough, state.Process(Space, true));
        Assert.Equal(ReservedHotkeyEventDisposition.PassThrough, state.Process(Space, false));
    }
}
