// Copyright (c) DeskBox. All rights reserved.

namespace DeskBox.Services;

/// <summary>
/// Recognizes two standalone Control-key taps. Control chords are always
/// passed through and cancel the pending tap so normal editing shortcuts do
/// not summon DeskBox.
/// </summary>
internal sealed class DoubleControlHotkeyStateMachine
{
    internal const uint DefaultMaximumIntervalMilliseconds = 320;

    private const uint Control = 0x11;
    private const uint LeftControl = 0xA2;
    private const uint RightControl = 0xA3;

    private readonly uint _maximumIntervalMilliseconds;
    private ControlKeys _pressedControls;
    private bool _currentPressUsedAsChord;
    private uint _lastStandaloneReleaseTime;
    private bool _hasPendingTap;

    [Flags]
    private enum ControlKeys
    {
        None = 0,
        Generic = 1,
        Left = 2,
        Right = 4
    }

    public DoubleControlHotkeyStateMachine(
        uint maximumIntervalMilliseconds = DefaultMaximumIntervalMilliseconds)
    {
        _maximumIntervalMilliseconds = maximumIntervalMilliseconds;
    }

    public ReservedHotkeyEventDisposition Process(uint virtualKey,bool isKeyDown,uint eventTime)
    {
        if (!TryGetControlKey(virtualKey,out ControlKeys controlKey))
        {
            if (isKeyDown)
            {
                if (_pressedControls != ControlKeys.None)
                {
                    _currentPressUsedAsChord = true;
                }

                _hasPendingTap = false;
            }

            return ReservedHotkeyEventDisposition.PassThrough;
        }

        if (isKeyDown)
        {
            if ((_pressedControls & controlKey) == 0 && _pressedControls != ControlKeys.None)
            {
                _currentPressUsedAsChord = true;
            }

            _pressedControls |= controlKey;
            return ReservedHotkeyEventDisposition.PassThrough;
        }

        if ((_pressedControls & controlKey) == 0)
        {
            return ReservedHotkeyEventDisposition.PassThrough;
        }

        _pressedControls &= ~controlKey;
        if (_pressedControls != ControlKeys.None)
        {
            return ReservedHotkeyEventDisposition.PassThrough;
        }

        if (_currentPressUsedAsChord)
        {
            _currentPressUsedAsChord = false;
            _hasPendingTap = false;
            return ReservedHotkeyEventDisposition.PassThrough;
        }

        if (_hasPendingTap &&
            unchecked(eventTime - _lastStandaloneReleaseTime) <= _maximumIntervalMilliseconds)
        {
            _hasPendingTap = false;
            return ReservedHotkeyEventDisposition.TriggerAndPassThrough;
        }

        _lastStandaloneReleaseTime = eventTime;
        _hasPendingTap = true;
        return ReservedHotkeyEventDisposition.PassThrough;
    }

    public void Reset()
    {
        _pressedControls = ControlKeys.None;
        _currentPressUsedAsChord = false;
        _lastStandaloneReleaseTime = 0;
        _hasPendingTap = false;
    }

    private static bool TryGetControlKey(uint virtualKey,out ControlKeys controlKey)
    {
        controlKey = virtualKey switch
        {
            Control => ControlKeys.Generic,            LeftControl => ControlKeys.Left,            RightControl => ControlKeys.Right,            _ => ControlKeys.None
        };
        return controlKey != ControlKeys.None;
    }
}
