namespace DeskBox.Services;

/// <summary>
/// Where a released raised-band guest should land in the Z order. The band is
/// only meaningful during a live quick-reveal raised session; after release,
/// ordinary window ordering applies again.
/// </summary>
internal enum RaisedBandReleasePlacement
{
    /// <summary>
    /// The session ended because the user activated a foreign application.
    /// Drop the guest below that window so it never covers the app the user
    /// just switched to.
    /// </summary>
    BelowForeignForeground,

    /// <summary>
    /// The guest is still the active surface (or another DeskBox window owns
    /// the foreground). Clearing TOPMOST places it at the top of the normal
    /// band, which matches its role as the window being used.
    /// </summary>
    TopOfNormalBand,
}

/// <summary>
/// Pure decisions for quick-reveal raised-band guest windows (search popup,
/// settings, desktop organization). A raised quick-reveal group is held
/// WS_EX_TOPMOST, so DeskBox-owned interactive surfaces opened during the
/// session must join the topmost band above the widgets instead of pulsing
/// back into the normal band below them.
/// </summary>
internal static class RaisedBandGuestPolicy
{
    /// <summary>
    /// A window joins the raised band only while a quick-reveal raised session
    /// is live. Dynamic-layer raises end NOTOPMOST (guests already float above
    /// widgets) and desktop-pinned raises never leave the desktop layer.
    /// </summary>
    public static bool ShouldHoldGuest(
        bool usesQuickRevealMode,        bool widgetsRaisedFromTray)
    {
        return usesQuickRevealMode && widgetsRaisedFromTray;
    }

    public static RaisedBandReleasePlacement ResolveReleasePlacement(
        bool foreignWindowIsForeground)
    {
        return foreignWindowIsForeground
            ? RaisedBandReleasePlacement.BelowForeignForeground
            : RaisedBandReleasePlacement.TopOfNormalBand;
    }

    /// <summary>
    /// A guest recorded during session N must not be swept by the completion
    /// of an older session's dismissal: a fast dismiss-then-reraise renews the
    /// guest's generation. Guests from the ended session (and any stale older
    /// records) are swept.
    /// </summary>
    public static bool ShouldSweepGuest(
        long guestSessionGeneration,        long endedSessionGeneration)
    {
        return guestSessionGeneration <= endedSessionGeneration;
    }
}
