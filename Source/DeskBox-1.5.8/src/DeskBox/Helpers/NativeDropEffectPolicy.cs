namespace DeskBox.Helpers;

/// <summary>
/// Chooses OLE drag feedback separately from the completion effect. DeskBox
/// imports native file drops asynchronously and owns the actual move/copy, so
/// the source must never be told to perform move cleanup after Drop returns.
/// </summary>
internal static class NativeDropEffectPolicy
{
    internal const uint None = 0;
    internal const uint Copy = 1;
    internal const uint Move = 2;
    internal const uint Link = 4;
    internal const uint ControlKeyState = 0x0008;
    internal const uint ShiftKeyState = 0x0004;
    // OLE IDropTarget key-state flags (MK_* in oleidl.h).
    internal const uint RightButtonKeyState = 0x0002;
    internal const uint AltKeyState = 0x0020;

    public static bool IsVirtualOnlyFileData(
        bool hasPhysicalPathData,        bool hasVirtualDescriptorData)
    {
        return !hasPhysicalPathData && hasVirtualDescriptorData;
    }

    public static uint ResolveFeedbackEffect(
        bool hasFileData,        bool hasVirtualFileData,        uint keyState,        uint allowedEffects,        bool hasShellApplicationData = false,        bool defaultMove = true,        bool followWindows = false,        bool sameVolume = true)
    {
        if (hasShellApplicationData)
        {
            if ((allowedEffects & Link) != 0)
            {
                return Link;
            }

            // Some third-party Start replacements expose application objects
            // as copy-only payloads. DeskBox still creates a shortcut and never
            // authorizes source cleanup.
            return (allowedEffects & Copy) != 0 ? Copy : None;
        }

        if (!hasFileData)
        {
            return None;
        }

        FileDropIntent intent = FileDropIntentPolicy.ResolveMappedTransfer(
            hasMappedFolder: true,            forceCopy: hasVirtualFileData,            controlDown: (keyState & ControlKeyState) != 0,            shiftDown: (keyState & ShiftKeyState) != 0,            defaultMove: defaultMove,            canCopy: (allowedEffects & Copy) != 0,            canMove: (allowedEffects & Move) != 0,            altDown: (keyState & AltKeyState) != 0,            followWindows: followWindows,            sameVolume: sameVolume,            // The native source may omit DROPEFFECT_LINK even though DeskBox
            // can safely create a local .lnk from the extracted path. The
            // visual still falls back to Copy when Link is not advertised.
            canLink: true);
        return intent switch
        {
            FileDropIntent.Copy => Copy,            FileDropIntent.Move => Move,            FileDropIntent.Shortcut when (allowedEffects & Link) != 0 => Link,            FileDropIntent.Shortcut when (allowedEffects & Copy) != 0 => Copy,            FileDropIntent.Shortcut when (allowedEffects & Move) != 0 => Move,            _ => None
        };
    }

    public static bool ShouldCopyMappedTransfer(
        bool containsTemporaryFiles,        uint keyState,        bool defaultMove,        bool followWindows = false,        bool sameVolume = true,        bool canCopy = true,        bool canMove = true)
    {
        FileDropIntent intent = FileDropIntentPolicy.ResolveMappedTransfer(
            hasMappedFolder: true,            forceCopy: containsTemporaryFiles,            controlDown: (keyState & ControlKeyState) != 0,            shiftDown: (keyState & ShiftKeyState) != 0,            defaultMove: defaultMove,            canCopy: canCopy,            canMove: canMove,            altDown: (keyState & AltKeyState) != 0,            followWindows: followWindows,            sameVolume: sameVolume);
        return intent != FileDropIntent.Move;
    }

    public static bool ShouldCreateMappedShortcut(
        bool containsTemporaryFiles,        uint keyState)
    {
        return !containsTemporaryFiles &&
               ((keyState & AltKeyState) != 0 ||
                (keyState & (ControlKeyState | ShiftKeyState)) ==
                    (ControlKeyState | ShiftKeyState));
    }

    public static bool IsRightButtonDrag(uint keyState)
    {
        return (keyState & RightButtonKeyState) != 0;
    }

    public static uint ResolveCompletionEffect(
        bool hasExtractedPaths,        uint allowedEffects,        bool createdShellApplicationLinks = false)
    {
        if (!hasExtractedPaths)
        {
            return None;
        }

        if (createdShellApplicationLinks)
        {
            return (allowedEffects & Link) != 0
                ? Link
                : (allowedEffects & Copy) != 0
                    ? Copy
                    : None;
        }

        // Returning MOVE would authorize the drag source (notably Explorer)
        // to delete its source after this callback returns. DeskBox has only
        // queued its own asynchronous transfer at that point, producing a
        // check-then-disappear race. Report COPY so source cleanup stays off.
        return (allowedEffects & Copy) != 0 ? Copy : None;
    }
}
