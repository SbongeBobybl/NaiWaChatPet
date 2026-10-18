// Copyright (c) DeskBox. All rights reserved.

namespace DeskBox.Services;

internal readonly record struct DesktopDoubleClickSequence(
    int FirstX,    int FirstY,    uint FirstTime,    int SecondX,    int SecondY,    uint SecondTime);

internal sealed class DesktopDoubleClickStateMachine
{
    private readonly uint _maximumIntervalMilliseconds;
    private readonly int _maximumDeltaX;
    private readonly int _maximumDeltaY;
    private bool _hasFirstClick;
    private int _firstX;
    private int _firstY;
    private uint _firstTime;

    public DesktopDoubleClickStateMachine(
        uint maximumIntervalMilliseconds,        int doubleClickWidth,        int doubleClickHeight)
    {
        _maximumIntervalMilliseconds = Math.Max(1,maximumIntervalMilliseconds);
        _maximumDeltaX = Math.Max(1,doubleClickWidth / 2);
        _maximumDeltaY = Math.Max(1,doubleClickHeight / 2);
    }

    public bool Process(
        int screenX,        int screenY,        uint eventTime,        bool isDesktopBlank,        out DesktopDoubleClickSequence sequence)
    {
        sequence = default;
        if (!isDesktopBlank)
        {
            Reset();
            return false;
        }

        if (_hasFirstClick)
        {
            uint elapsed = unchecked(eventTime - _firstTime);
            if (elapsed <= _maximumIntervalMilliseconds &&
                Math.Abs(screenX - _firstX) <= _maximumDeltaX &&
                Math.Abs(screenY - _firstY) <= _maximumDeltaY)
            {
                sequence = new DesktopDoubleClickSequence(
                    _firstX,                    _firstY,                    _firstTime,                    screenX,                    screenY,                    eventTime);
                Reset();
                return true;
            }
        }

        _hasFirstClick = true;
        _firstX = screenX;
        _firstY = screenY;
        _firstTime = eventTime;
        return false;
    }

    public void Reset()
    {
        _hasFirstClick = false;
        _firstX = 0;
        _firstY = 0;
        _firstTime = 0;
    }
}
