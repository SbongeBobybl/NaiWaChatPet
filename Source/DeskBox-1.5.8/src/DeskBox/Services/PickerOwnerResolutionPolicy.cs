namespace DeskBox.Services;

internal enum PickerOwnerSource
{
    TrayWindow,
    CachedHelperWindow,
    FreshHelperWindow
}

internal static class PickerOwnerResolutionPolicy
{
    public static PickerOwnerSource Resolve(
        bool trayWindowUsable,        bool cachedHelperWindowUsable)
    {
        // An owned picker dialog is born on its owner's virtual desktop and
        // inherits the owner's DWM cloak state, so a cloaked owner — the tray
        // host left on a desktop the user has switched away from — makes the
        // dialog unreachable (issue 489). The tray host stays the normal
        // owner; every fallback must be a window that is provably on the
        // active desktop, and a freshly created window always is.
        if (trayWindowUsable)
        {
            return PickerOwnerSource.TrayWindow;
        }

        return cachedHelperWindowUsable
            ? PickerOwnerSource.CachedHelperWindow
            : PickerOwnerSource.FreshHelperWindow;
    }
}
