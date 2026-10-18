namespace DeskBox.Helpers;

internal enum FileDropIntent
{
    None,
    Copy,
    Move,
    Shortcut,
    Reference,
    Reorder,
    Organize
}

/// <summary>
/// Resolves the semantic intent of a file drop before individual XAML and OLE
/// adapters translate it into their platform-specific operation flags.
/// </summary>
internal static class FileDropIntentPolicy
{
    public static FileDropIntent ResolveMappedTransfer(
        bool hasMappedFolder,        bool forceCopy,        bool controlDown,        bool shiftDown,        bool defaultMove,        bool canCopy = true,        bool canMove = true,        bool altDown = false,        bool followWindows = false,        bool sameVolume = true,        bool canLink = true)
    {
        if (!hasMappedFolder)
        {
            // The widget has no managed folder yet, but the import creates
            // one on demand, so an explicit modifier must still decide the
            // transfer semantics. Only the unmodified default defers to the
            // reference/settings-backed decision; swallowing Ctrl/Shift here
            // would let a Ctrl (copy) gesture run the default move and delete
            // the source against the user's explicit intent.
            if (forceCopy)
            {
                return canCopy ? FileDropIntent.Copy : FileDropIntent.None;
            }

            if ((altDown || (controlDown && shiftDown)) && canLink)
            {
                return FileDropIntent.Shortcut;
            }

            if (controlDown)
            {
                return canCopy ? FileDropIntent.Copy : FileDropIntent.None;
            }

            if (shiftDown)
            {
                return canMove ? FileDropIntent.Move : FileDropIntent.None;
            }

            return FileDropIntent.Reference;
        }

        // Temporary and virtual payloads must outlive their provider-owned
        // staging directory, so they can never be moved from the source.
        if (forceCopy)
        {
            return canCopy ? FileDropIntent.Copy : FileDropIntent.None;
        }

        // Windows documents Ctrl+Shift as the link gesture. DeskBox also
        // accepts Alt as a discoverable shortcut gesture because it is the
        // modifier users commonly use when dragging from Explorer. Keep this
        // decision in the shared policy so XAML and native OLE paths agree.
        if ((altDown || (controlDown && shiftDown)) && canLink)
        {
            return FileDropIntent.Shortcut;
        }

        if (controlDown)
        {
            return canCopy ? FileDropIntent.Copy : FileDropIntent.None;
        }

        if (shiftDown)
        {
            return canMove ? FileDropIntent.Move : FileDropIntent.None;
        }

        // Explorer chooses Move for a same-volume drop and Copy when the
        // source and destination are on different volumes. A caller that has
        // enough path information can opt into that exact behaviour.
        if (followWindows)
        {
            if (sameVolume && canMove)
            {
                return FileDropIntent.Move;
            }

            if (!sameVolume && canCopy)
            {
                return FileDropIntent.Copy;
            }
        }

        if (defaultMove && canMove)
        {
            return FileDropIntent.Move;
        }

        return canCopy
            ? FileDropIntent.Copy
            : canMove
                ? FileDropIntent.Move
                : FileDropIntent.None;
    }

    public static bool AreSameVolume(string sourcePath,string destinationPath)
    {
        try
        {
            string sourceRoot = Path.GetPathRoot(Path.GetFullPath(sourcePath)) ??
                string.Empty;
            string destinationRoot = Path.GetPathRoot(
                Path.GetFullPath(destinationPath)) ?? string.Empty;
            return sourceRoot.Length > 0 &&
                   destinationRoot.Length > 0 &&
                   string.Equals(
                       sourceRoot,                       destinationRoot,                       StringComparison.OrdinalIgnoreCase);
        }
        catch
        {
            // Unknown roots are treated as cross-volume. That is the safe
            // choice because a move may otherwise delete the source after a
            // provider has copied only part of a large payload.
            return false;
        }
    }

    public static bool AreAllOnSameVolume(
        IEnumerable<string> sourcePaths,        string destinationPath)
    {
        return sourcePaths
            .Where(path => !string.IsNullOrWhiteSpace(path))
            .All(path => AreSameVolume(path,destinationPath));
    }
}
