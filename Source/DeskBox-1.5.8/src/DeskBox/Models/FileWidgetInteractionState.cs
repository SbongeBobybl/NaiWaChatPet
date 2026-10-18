namespace DeskBox.Models;

public sealed record FileSelectionCommandState(
    int SelectedCount,    bool HasSelection,    bool CanOpen,    bool CanRename,    bool CanCopy,    bool CanCut,    bool CanDelete)
{
    public static FileSelectionCommandState Resolve(int selectedCount)
    {
        int count = Math.Max(0,selectedCount);
        bool hasSelection = count > 0;
        return new(
            count,            hasSelection,            count == 1,            count == 1,            hasSelection,            hasSelection,            hasSelection);
    }
}

public enum FileDropVisualState
{
    None,
    Waiting,
    Accept,
    Reject
}

public sealed record FileDropPresentation(
    FileDropVisualState State,    bool DimContent,    bool ShowAccentBorder,    bool ShowWarning);

public static class FileDropVisualPolicy
{
    public static FileDropPresentation Resolve(
        bool isDragging,        bool containsStorageItems,        bool canAccept)
    {
        if (!isDragging)
        {
            return new(FileDropVisualState.None,false,false,false);
        }

        if (!containsStorageItems)
        {
            return new(FileDropVisualState.Reject,true,false,true);
        }

        return canAccept
            ? new(FileDropVisualState.Accept,true,true,false)
            : new(FileDropVisualState.Reject,true,false,true);
    }
}
