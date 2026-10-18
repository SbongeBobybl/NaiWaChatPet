namespace DeskBox.Tests;

public sealed class WidgetZOrderRestoreContractTests
{
    [Fact]
    public void IdleNormalization_OnlyReordersWidgetPeers()
    {
        string source = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetManager.ZOrder.cs"));
        string method = SliceMethod(
            source,
            "private bool NormalizeIdleWidgetZOrder",
            "private static IReadOnlyList<IDesktopWidgetWindow> GetWindowsInIdleHighestFirstOrder");

        Assert.Contains("ApplyPeerOrderHighestToLowest", method, StringComparison.Ordinal);
        Assert.DoesNotContain("MoveToDesktopBottom", method, StringComparison.Ordinal);
        Assert.DoesNotContain("SetWindowToBottom", method, StringComparison.Ordinal);
    }

    [Fact]
    public void TransientDeskBoxActivation_ReassertsTheWholeRaisedGroupWithoutEndingSession()
    {
        string manager = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetManager.ZOrder.cs"));
        string method = SliceMethod(
            manager,
            "internal bool ReassertRaisedWidgetGroupAfterDeskBoxActivation",
            "public bool RequestRestoreRaisedWidgetsToDesktopLayer");
        string layerService = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetLayerService.cs"));
        string reassertOrder = SliceMethod(
            layerService,
            "public static bool ApplyRaisedGroupReassertOrder",
            "internal static bool IsForeignWindowAboveRaisedPeers");
        string contentWindow = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Views/ContentWidgetWindow.WindowInteraction.cs"));
        string stackPopover = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.StackPopover.cs"));

        Assert.Contains("_widgetsRaisedFromTray", method, StringComparison.Ordinal);
        Assert.Contains("IsForegroundDeskBoxWindow()", method, StringComparison.Ordinal);
        Assert.Contains("ApplyRaisedGroupReassertOrder", method, StringComparison.Ordinal);
        Assert.DoesNotContain("RestoreRaisedWidgetsToDesktopLayer", method, StringComparison.Ordinal);

        // The reassert order must escalate: a tray raise that ran while
        // DeskBox was a background process pins the peers below the
        // then-foreground window, and only a repeated group lift can repair
        // that split band once DeskBox owns the foreground. An intact band
        // keeps the repaint-free peer reorder.
        Assert.Contains("RaisedGroupReassertPolicy.Resolve", reassertOrder, StringComparison.Ordinal);
        Assert.Contains("BringGroupTemporarilyToFront", reassertOrder, StringComparison.Ordinal);
        Assert.Contains("ApplyPeerOrderHighestToLowest", reassertOrder, StringComparison.Ordinal);
        Assert.Contains(
            "ReassertRaisedWidgetGroupAfterDeskBoxActivation",
            contentWindow,
            StringComparison.Ordinal);
        Assert.Contains(
            "ReassertRaisedWidgetGroupAfterDeskBoxActivation",
            stackPopover,
            StringComparison.Ordinal);
    }

    [Fact]
    public void RaisedSessionRestore_ReanchorsTheCompleteGroupBehindForeground()
    {
        string manager = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetManager.cs"));
        string restore = SliceMethod(
            manager,
            "private void RestoreRaisedWidgetsToDesktopLayer(bool force)",
            "public bool SetWidgetPositionLocked");
        string layerService = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetLayerService.cs"));

        Assert.Contains("RestoreGroupPreservingForeground", restore, StringComparison.Ordinal);
        Assert.DoesNotContain("NormalizeIdleWidgetZOrder(\"raised-session-restored\")", restore, StringComparison.Ordinal);
        Assert.Contains("case RelativeLayerRestoreDisposition.BehindForeground:", layerService, StringComparison.Ordinal);
        Assert.Contains("ApplyWindowOrderHighestToLowest", layerService, StringComparison.Ordinal);
    }

    [Fact]
    public void DesktopPinnedPointerActivation_IsBlockedBeforeDefaultWindowActivation()
    {
        string bounds = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Views/WidgetWindowBase.Bounds.cs"));
        string setup = SliceMethod(
            bounds,
            "private void InstallDesktopPinnedActivationGuard",
            "private void RemoveDesktopPinnedActivationGuard");
        string callback = SliceMethod(
            bounds,
            "private IntPtr DesktopPinnedActivationSubclassProc",
            "// ── Bounds management");

        Assert.Contains("SetWindowSubclass", setup, StringComparison.Ordinal);
        Assert.Contains("Win32Helper.WM_MOUSEACTIVATE", callback, StringComparison.Ordinal);
        Assert.Contains("WidgetLayerService.ShouldSuppressPointerActivation", callback, StringComparison.Ordinal);
        Assert.Contains("Win32Helper.MA_NOACTIVATE", callback, StringComparison.Ordinal);
        Assert.Contains("RestoreDesktopPinnedBottomState", callback, StringComparison.Ordinal);
        Assert.DoesNotContain("HoldTemporaryTopMost", callback, StringComparison.Ordinal);
    }

    [Fact]
    public void DesktopPinnedBlankAreaClick_UsesNoActivateRestingStyleAndRoutedGuard()
    {
        string bounds = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Views/WidgetWindowBase.Bounds.cs"));
        string layerService = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetLayerService.cs"));
        string routedHandler = SliceMethod(
            bounds,
            "private void RootElement_PointerPressedForDesktopPinnedLayer",
            "private void WidgetWindowBase_ActivatedForDesktopPinnedLayer");

        Assert.Contains("WS_EX_NOACTIVATE", layerService, StringComparison.Ordinal);
        Assert.Contains("ApplyDesktopPinnedActivationStyle(windowHandle)", layerService, StringComparison.Ordinal);
        Assert.Contains("UIElement.PointerPressedEvent", bounds, StringComparison.Ordinal);
        Assert.Contains("TryAllowDesktopPinnedPointerActivation", routedHandler, StringComparison.Ordinal);
        Assert.Contains("IsKeyboardInputTarget(args.OriginalSource)", routedHandler, StringComparison.Ordinal);
        Assert.Contains("PrepareForDesktopPinnedKeyboardInput", routedHandler, StringComparison.Ordinal);
        Assert.Contains("RestoreDesktopPinnedBottomState", routedHandler, StringComparison.Ordinal);
        Assert.Contains("ActivateDesktopPinnedWindow", routedHandler, StringComparison.Ordinal);
        Assert.DoesNotContain("args.Handled = true", routedHandler, StringComparison.Ordinal);
    }

    [Fact]
    public void DesktopPinnedKeyboardInput_BypassesBlankAreaSuppressionAndRestoresBottomLayer()
    {
        string bounds = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Views/WidgetWindowBase.Bounds.cs"));
        string layerService = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetLayerService.cs"));
        string setup = SliceMethod(
            bounds,
            "private void InstallDesktopPinnedPointerRouting",
            "private void RemoveDesktopPinnedPointerRouting");
        string gotFocus = SliceMethod(
            bounds,
            "private void RootElement_GotFocusForDesktopPinnedLayer",
            "private static bool IsKeyboardInputTarget");
        string keyboardTarget = SliceMethod(
            bounds,
            "private static bool IsKeyboardInputTarget",
            "private void ActivateDesktopPinnedWindow");
        string activate = SliceMethod(
            bounds,
            "private void ActivateDesktopPinnedWindow",
            "private void WidgetWindowBase_ActivatedForDesktopPinnedLayer");
        string prepare = SliceMethod(
            layerService,
            "public static void PrepareForDesktopPinnedKeyboardInput",
            "public static bool IsWindowNoActivate");

        Assert.Contains("RootElement.GotFocus +=", setup, StringComparison.Ordinal);
        Assert.Contains("IsKeyboardInputTarget(args.OriginalSource)", gotFocus, StringComparison.Ordinal);
        Assert.Contains("PrepareForDesktopPinnedKeyboardInput", gotFocus, StringComparison.Ordinal);
        Assert.Contains("TextBox", keyboardTarget, StringComparison.Ordinal);
        Assert.Contains("RichEditBox", keyboardTarget, StringComparison.Ordinal);
        Assert.Contains("PasswordBox", keyboardTarget, StringComparison.Ordinal);
        Assert.Contains("SetWindowNoActivate(windowHandle, enabled: false)", prepare, StringComparison.Ordinal);
        Assert.Contains("base.Activate()", activate, StringComparison.Ordinal);
        Assert.Contains("SetForegroundWindow(HWnd)", activate, StringComparison.Ordinal);
        Assert.Contains("RestoreDesktopPinnedBottomState(reason)", activate, StringComparison.Ordinal);
    }

    [Fact]
    public void DesktopPinnedInteraction_RestoresBottomWhileExpandedCapsuleRaisesAbovePeers()
    {
        string bounds = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Views/WidgetWindowBase.Bounds.cs"));
        string activated = SliceMethod(
            bounds,
            "private void WidgetWindowBase_ActivatedForDesktopPinnedLayer",
            "private void RestoreDesktopPinnedBottomState");
        string restore = SliceMethod(
            bounds,
            "private void RestoreDesktopPinnedBottomState",
            "private IntPtr DesktopPinnedActivationSubclassProc");
        string collapse = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Views/WidgetWindowBase.Collapse.cs"));
        string expand = SliceMethod(
            collapse,
            "private void RaiseForExpandedState()",
            "private void AcquireExpandedWidgetLayerLease");
        string manager = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetManager.ZOrder.cs"));
        string acquire = SliceMethod(
            manager,
            "internal long AcquireExpandedWidgetLayer",
            "internal bool ReleaseExpandedWidgetLayer");

        Assert.Contains("RestoreDesktopPinnedBottomState", activated, StringComparison.Ordinal);
        Assert.DoesNotContain(
            "WindowActivationState.Deactivated",
            activated,
            StringComparison.Ordinal);
        Assert.Contains(
            "WidgetLayerService.MoveToDesktopBottom(HWnd, showWindow: false)",
            restore,
            StringComparison.Ordinal);
        Assert.Contains("TopMostSafetyTimer?.Stop()", restore, StringComparison.Ordinal);

        // The expanded capsule must not be flattened to the desktop bottom:
        // plain interaction reasserts the bottom, but an expand raises the
        // window to the top of the widget band through the shared helpers.
        // Desktop-pinned expansion additionally re-beds a floating group
        // through the manager guard before raising (owner-band promotion
        // residue must not carry the expanded capsule above apps, #574) —
        // the guard excludes active leases, so an expanded capsule is never
        // flattened by it, and the expand flow itself still never calls
        // MoveToDesktopBottom directly.
        Assert.Contains(
            "TryRebedDesktopPinnedGroupBeforeExpand",
            expand,
            StringComparison.Ordinal);
        Assert.DoesNotContain("MoveToDesktopBottom", expand, StringComparison.Ordinal);
        Assert.Contains(
            "TryBringAbovePeerWidgetsAtDesktopLayer",
            expand,
            StringComparison.Ordinal);
        Assert.Contains(
            "AcquireExpandedWidgetLayerLease(\"expanded-state-desktop\")",
            expand,
            StringComparison.Ordinal);

        // The expanded-layer lease applies in desktop-pinned mode too: the
        // acquire path must order the capsule above its peers without ever
        // moving it to the desktop bottom.
        Assert.Contains(
            "WidgetExpandedLayerLeasePolicy.Acquire(",
            acquire,
            StringComparison.Ordinal);
        Assert.Contains("EnsurePeerOrderHighestToLowest", acquire, StringComparison.Ordinal);
        Assert.DoesNotContain("MoveToDesktopBottom", acquire, StringComparison.Ordinal);
    }

    [Fact]
    public void DesktopPinnedActivationRestore_CannotReshowHiddenWindow()
    {
        string bounds = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Views/WidgetWindowBase.Bounds.cs"));
        string restore = SliceMethod(
            bounds,
            "private void RestoreDesktopPinnedBottomState",
            "private IntPtr DesktopPinnedActivationSubclassProc");

        int visibilityGuard = restore.IndexOf(
            "WidgetTemporaryRaiseLeasePolicy.CanRestoreDesktopLayer(",
            StringComparison.Ordinal);
        int layerRepair = restore.IndexOf(
            "WidgetLayerService.MoveToDesktopBottom(HWnd, showWindow: false)",
            StringComparison.Ordinal);

        Assert.InRange(visibilityGuard, 0, layerRepair - 1);
        Assert.Contains("Visible,", restore, StringComparison.Ordinal);
        Assert.Contains("IsHideAnimationRunning,", restore, StringComparison.Ordinal);
        Assert.Contains("IsClosing", restore, StringComparison.Ordinal);
        Assert.Contains("CancelPendingDesktopLayerRestore();", restore, StringComparison.Ordinal);
        Assert.DoesNotContain(
            "WidgetLayerService.MoveToDesktopBottom(HWnd);",
            restore,
            StringComparison.Ordinal);
    }

    [Fact]
    public void DesktopPinnedPeerRaiseEntryPoints_SplitTrayRaiseBottomFromCapsuleBandTop()
    {
        string layerService = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetLayerService.cs"));
        string bring = SliceMethod(
            layerService,
            "public static void BringAbovePeerWidgets",
            "public static bool TryBringAbovePeerWidgetsAtDesktopLayer");
        string tryBring = SliceMethod(
            layerService,
            "public static bool TryBringAbovePeerWidgetsAtDesktopLayer",
            "public static bool EnsurePeerOrderHighestToLowest");

        // Tray-raise keeps desktop-pinned widgets resting at the band bottom.
        string pinnedBring = bring[..bring.IndexOf(
            "DetachFromDesktopIconLayerIfNeeded",
            StringComparison.Ordinal)];
        Assert.Contains("MoveToDesktopBottom(windowHandle)", pinnedBring, StringComparison.Ordinal);
        Assert.DoesNotContain("HWND_TOP", pinnedBring, StringComparison.Ordinal);

        // The capsule expand raise must stay inside the desktop band while
        // taking the top of it: attach without bottom placement, then HWND_TOP.
        Assert.DoesNotContain(
            "UsesDesktopPinnedMode()",
            tryBring,
            StringComparison.Ordinal);
        Assert.Contains("placeAtBottom: false", tryBring, StringComparison.Ordinal);
        Assert.Contains("HWND_TOP", tryBring, StringComparison.Ordinal);
        Assert.DoesNotContain("MoveToDesktopBottom", tryBring, StringComparison.Ordinal);
    }

    [Fact]
    public void IdleNormalization_IsShadowGatedAndNeverAppliesAReorderThatIsAlreadyCurrent()
    {
        string zorder = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetManager.ZOrder.cs"));
        string method = SliceMethod(
            zorder,
            "private bool NormalizeIdleWidgetZOrder",
            "private static IReadOnlyList<IDesktopWidgetWindow> GetWindowsInIdleHighestFirstOrder");
        string queue = SliceMethod(
            zorder,
            "internal void QueueIdleWidgetZOrderNormalization",
            "private long TrackTemporarilyRaisedWidgets");
        string layerService = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetLayerService.cs"));
        string apply = SliceMethod(
            layerService,
            "public static bool ApplyPeerOrderHighestToLowest",
            "private static bool ApplyWindowOrderHighestToLowest");

        // The idle order exists only to keep upper shadows off lower widgets:
        // with Windows drop shadows disabled both the queue and the executor
        // must bail before touching HWND z-order.
        Assert.Contains("TryGetWindowDropShadowEnabled", method, StringComparison.Ordinal);
        Assert.Contains("ShouldNormalizeIdlePeerOrder", method, StringComparison.Ordinal);
        Assert.Contains("ShouldNormalizeIdlePeerOrder", queue, StringComparison.Ordinal);

        // Ordering on transient animation bounds replays as a second, visible
        // reorder once the windows settle, so normalization defers instead.
        Assert.Contains("IsBoundsTransitionActive", method, StringComparison.Ordinal);

        // Reordering still repaints overlap regions even when nothing changes,
        // so an already-correct order must short-circuit before SetWindowPos.
        Assert.Contains("IsPeerOrderAlreadyApplied", apply, StringComparison.Ordinal);
        Assert.DoesNotContain("MoveToDesktopBottom", method, StringComparison.Ordinal);
        Assert.DoesNotContain("SetWindowToBottom", method, StringComparison.Ordinal);
    }

    [Fact]
    public void DesktopPinnedCoveredPointerActivation_SuppressesEvenOnDesktopForeground()
    {
        string layerService = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetLayerService.cs"));
        string managerZOrder = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetManager.ZOrder.cs"));
        string decision = SliceMethod(
            layerService,
            "public static bool ShouldSuppressPointerActivation",
            "public static bool ShouldPreserveQuickRevealActivatingClick");
        string coveredWalk = SliceMethod(
            layerService,
            "public static bool IsWindowCoveredByForeignWindow",
            "private static bool RectsIntersect");
        // The activation decision must consider geometric coverage: activating
        // a DefView-owned HWND promotes the whole owner band above covering
        // applications even while the desktop shell owns the foreground (#574).
        Assert.Contains("IsWindowCoveredByForeignWindow(windowHandle)", decision, StringComparison.Ordinal);

        // The coverage walk looks upward in Z order and requires a rect
        // intersection, so promotion over non-overlapping windows stays
        // allowed (keyboard flows keep working on exposed widgets).
        Assert.Contains("Win32Helper.GW_HWNDPREV", coveredWalk, StringComparison.Ordinal);
        Assert.Contains("ClassifyWindowForBedding", coveredWalk, StringComparison.Ordinal);
        Assert.Contains("RectsIntersect", coveredWalk, StringComparison.Ordinal);
        Assert.Contains("MaxBeddingWalkWindows", coveredWalk, StringComparison.Ordinal);

        // The taskbar is neutral: an auto-hide taskbar overlapping a widget
        // must not read as a covering application. The classifier lives on
        // the manager and is shared, not duplicated per walk.
        Assert.Contains("WidgetManager.IsTaskbarWindow", coveredWalk, StringComparison.Ordinal);
        string taskbarClassifier = SliceMethod(
            managerZOrder,
            "internal static bool IsTaskbarWindow",
            "private static bool IsDesktopShellWindow");
        Assert.Contains("Shell_TrayWnd", taskbarClassifier, StringComparison.Ordinal);
        Assert.Contains("Shell_SecondaryTrayWnd", taskbarClassifier, StringComparison.Ordinal);
        Assert.Contains("NotifyIconOverflowWindow", taskbarClassifier, StringComparison.Ordinal);
    }

    [Fact]
    public void DesktopPinnedExpand_RebedsFloatingGroupBeforeRaisingCapsule()
    {
        string collapse = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Views/WidgetWindowBase.Collapse.cs"));
        string expand = SliceMethod(
            collapse,
            "private void RaiseForExpandedState",
            "private void AcquireExpandedWidgetLayerLease");
        string manager = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetManager.ZOrder.cs"));
        string rebed = SliceMethod(
            manager,
            "internal bool TryRebedDesktopPinnedGroupBeforeExpand",
            "private void VerifyDesktopPinnedBedding");

        // A pinned group floats only through owner-band promotion residue; the
        // expanded capsule must not ride that band above covering apps (#574).
        int rebedBranch = expand.IndexOf(
            "TryRebedDesktopPinnedGroupBeforeExpand",
            StringComparison.Ordinal);
        int floatingBranch = expand.IndexOf(
            "expanded-state-floating",
            StringComparison.Ordinal);
        Assert.True(rebedBranch >= 0, "Expand flow must consult the pinned re-bed guard");
        Assert.True(
            floatingBranch > rebedBranch,
            "The pinned re-bed guard must run before the float-preserving branch");

        // The guard repairs through the same evidence-based funnel as the
        // watchdog and stays out of the way of active sessions.
        Assert.Contains("WidgetLayerService.UsesDesktopPinnedMode()", rebed, StringComparison.Ordinal);
        Assert.Contains("_widgetsRaisedFromTray", rebed, StringComparison.Ordinal);
        Assert.Contains("_sessionManager.IsInteractionActive", rebed, StringComparison.Ordinal);
        Assert.Contains("HasActiveExpandedWidgetLayerLease()", rebed, StringComparison.Ordinal);
        Assert.Contains("WidgetLayerService.FindForeignWindowBelow", rebed, StringComparison.Ordinal);
        Assert.Contains("RebedDesktopPinnedWidgetGroup", rebed, StringComparison.Ordinal);
    }

    [Fact]
    public void DesktopLayerPeerRaise_InsertsBelowNearestForeignWindowNotGlobalTop()
    {
        string layerService = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetLayerService.cs"));
        string peerRaise = SliceMethod(
            layerService,
            "public static bool TryBringAbovePeerWidgetsAtDesktopLayer",
            "private static IntPtr FindInsertBelowAnchorForDesktopLayerRaise");
        string anchor = SliceMethod(
            layerService,
            "private static IntPtr FindInsertBelowAnchorForDesktopLayerRaise",
            "public static bool EnsurePeerOrderHighestToLowest");

        // DefView ownership enforces only a minimum z position; HWND_TOP on a
        // pinned window reaches the top of the whole normal band and floats
        // the expanded capsule above every application (#574). The raise must
        // instead insert below the nearest visible foreign window above the
        // group — above sibling widgets, beneath every application.
        Assert.Contains(
            "FindInsertBelowAnchorForDesktopLayerRaise(windowHandle)",
            peerRaise,
            StringComparison.Ordinal);
        Assert.DoesNotContain("Win32Helper.HWND_TOP", peerRaise, StringComparison.Ordinal);

        // The anchor walk skips DeskBox/shell/invisible windows and treats
        // the taskbar as neutral; HWND_TOP survives only as the no-foreign-
        // window fallback where nothing can be jumped over.
        Assert.Contains("Win32Helper.GW_HWNDPREV", anchor, StringComparison.Ordinal);
        Assert.Contains("ClassifyWindowForBedding", anchor, StringComparison.Ordinal);
        Assert.Contains("WidgetManager.IsTaskbarWindow", anchor, StringComparison.Ordinal);
        Assert.Contains("return Win32Helper.HWND_TOP", anchor, StringComparison.Ordinal);
    }

    [Fact]
    public void DesktopPinnedActivation_RebedsSiblingsInTheSameCycle()
    {
        string bounds = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Views/WidgetWindowBase.Bounds.cs"));
        string explicitActivation = SliceMethod(
            bounds,
            "private void ActivateDesktopPinnedWindow",
            "private static bool IsDesktopShellRootWindow");
        string activatedHandler = SliceMethod(
            bounds,
            "private void WidgetWindowBase_ActivatedForDesktopPinnedLayer",
            "private void RestoreDesktopPinnedBottomState");
        string manager = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/WidgetManager.ZOrder.cs"));
        string rebed = SliceMethod(
            manager,
            "internal void RebedPinnedSiblingsAfterActivation",
            "private void VerifyDesktopPinnedBedding");

        // Activating any DefView-owned HWND promotes the whole owner band;
        // both the deliberate (keyboard) and the native activation paths must
        // drop the siblings back under covering applications in the same
        // cycle (#574), or the group lingers above apps until idle
        // normalization and reads as "all widgets jump to the front".
        Assert.Contains(
            "RebedPinnedSiblingsAfterActivation",
            explicitActivation,
            StringComparison.Ordinal);
        Assert.Contains(
            "RebedPinnedSiblingsAfterActivation",
            activatedHandler,
            StringComparison.Ordinal);
        Assert.Contains(
            "WindowActivationState.CodeActivated",
            activatedHandler,
            StringComparison.Ordinal);

        // The sibling pass reuses the evidence-based group restore, never
        // touches the activating window itself, and exempts the
        // expanded-capsule lease owner — bedding it would bury the expanded
        // capsule beneath sibling widgets mid-use.
        Assert.Contains(
            "WidgetLayerService.RestoreGroupPreservingForeground",
            rebed,
            StringComparison.Ordinal);
        Assert.Contains("window.WindowHandle != activeWindowHandle", rebed, StringComparison.Ordinal);
        Assert.Contains(
            "_expandedWidgetLayerLease.WindowHandle",
            rebed,
            StringComparison.Ordinal);
    }

    private static string SliceMethod(string source, string startMarker, string endMarker)
    {
        int start = source.IndexOf(startMarker, StringComparison.Ordinal);
        Assert.True(start >= 0, $"Missing source marker: {startMarker}");
        int end = source.IndexOf(endMarker, start, StringComparison.Ordinal);
        Assert.True(end > start, $"Missing source marker: {endMarker}");
        return source[start..end];
    }
}
