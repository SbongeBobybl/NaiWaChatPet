namespace DeskBox.Models;

/// <summary>
/// Decides whether closing a file widget can bypass the close-mode selection
/// flyout. The bypass is only allowed when the widget owns a default managed
/// storage folder AND that folder is proven empty, in which case the folder is
/// recycled together with the widget without further confirmation. An unknown
/// entry count must never bypass the selection: the count fails closed so the
/// user always gets the explicit choice whenever emptiness is not proven.
/// </summary>
public static class FileWidgetCloseFlowPolicy
{
    public static bool ShouldCloseEmptyManagedFolderDirectly(
        bool canCleanupManagedStorage,        int? managedFolderEntryCount) =>
        canCleanupManagedStorage &&
        managedFolderEntryCount == 0;
}
