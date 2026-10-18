namespace DeskBox.Models;

[Flags]
public enum HotkeyModifierKeys
{
    None = 0,
    Alt = 1,
    Control = 2,
    Shift = 4,
    Windows = 8
}

public readonly record struct GlobalHotkeyGesture(HotkeyModifierKeys Modifiers,int VirtualKey);

public enum HotkeyActivationKind
{
    Chord = 0,
    DoubleControl = 1,
    WindowsTap = 2
}

public readonly record struct GlobalHotkeyActivation(
    HotkeyActivationKind Kind,    GlobalHotkeyGesture Gesture)
{
    public static GlobalHotkeyActivation FromChord(GlobalHotkeyGesture gesture)
    {
        return new GlobalHotkeyActivation(HotkeyActivationKind.Chord,gesture);
    }
}
