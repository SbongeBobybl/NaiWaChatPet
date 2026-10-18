namespace DeskBox.Services;

internal readonly record struct TrayToggleDecisionContext(
    bool IsDesktopPinnedMode,    bool IsQuickRevealMode,    bool IsRaisedSession,    bool HasVisibleWidgets,    bool IsForegroundLocal);

internal static class TrayToggleDecisionPolicy
{
    public static bool ShouldHide(TrayToggleDecisionContext context)
    {
        // Desktop-pinned widgets deliberately never enter a raised session.
        // In that mode the hotkey is a pure visibility toggle, independent of
        // which application currently owns the foreground window.
        if (context.IsDesktopPinnedMode || context.IsQuickRevealMode)
        {
            return context.HasVisibleWidgets;
        }

        if (context.IsRaisedSession)
        {
            return true;
        }

        if (!context.HasVisibleWidgets)
        {
            return false;
        }

        return context.IsForegroundLocal;
    }
}
