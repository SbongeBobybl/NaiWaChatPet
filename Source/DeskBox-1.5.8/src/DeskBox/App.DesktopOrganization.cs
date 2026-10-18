using DeskBox.Views;

namespace DeskBox;

public partial class App
{
    private DesktopOrganizationWindow? _desktopOrganizationWindow;
    private bool _trayHiddenForPetOrganizer;
    private bool _widgetsHiddenForOrganizer;

    public DesktopOrganizationWindow ShowDesktopOrganizationWindow(IntPtr ownerHwnd = default,bool openAiPreview = false)
    {
        CancelBackgroundMemoryCleanup();
        if (_desktopOrganizationWindow is null)
        {
            var window = new DesktopOrganizationWindow();
            _desktopOrganizationWindow = window;
            ThemeService.TrackWindow(window);
            window.OrganizationCompleted += DesktopOrganizationWindow_StateChanged;
            window.OrganizationUndone += DesktopOrganizationWindow_StateChanged;
            window.Closed += DesktopOrganizationWindow_Closed;
        }

        if (ownerHwnd != IntPtr.Zero)
        {
            _desktopOrganizationWindow.SetOwner(ownerHwnd);
        }

        _desktopOrganizationWindow.ShowWindow();
        HideWidgetsForOrganizer();
        if (openAiPreview)
        {
            _desktopOrganizationWindow.RequestAiPreview();
            // Hide only after the organizer has a live window; tearing the
            // tray host down first made the fresh XAML window unload.
            HideTrayForPetOrganizer();
        }

        return _desktopOrganizationWindow;
    }

    private void DesktopOrganizationWindow_StateChanged(object? sender,EventArgs e)
    {
    }

    private void DesktopOrganizationWindow_Closed(object sender,Microsoft.UI.Xaml.WindowEventArgs args)
    {
        if (sender is not DesktopOrganizationWindow window)
        {
            return;
        }

        window.OrganizationCompleted -= DesktopOrganizationWindow_StateChanged;
        window.OrganizationUndone -= DesktopOrganizationWindow_StateChanged;
        window.Closed -= DesktopOrganizationWindow_Closed;
        if (ReferenceEquals(_desktopOrganizationWindow,window))
        {
            _desktopOrganizationWindow = null;
        }

        ScheduleLightMemoryCleanup(completedHeavyOperation: true);
        ScheduleBackgroundMemoryCleanup("desktop-organization-closed");
        RestoreWidgetsAfterOrganizer();
        if (_trayHiddenForPetOrganizer &&
            _onboardingWindow is null &&
            WidgetManager?.LoadedWidgetCount == 0)
        {
            // Launched by NaiWa and nothing left to host: leave no hidden
            // component process behind.
            Log("[Tray] Organizer closed with no widgets; exiting the hidden component");
            ExitApplication();
        }
    }

    private void HideWidgetsForOrganizer()
    {
        if (WidgetManager is null || _widgetsHiddenForOrganizer) return;
        _widgetsHiddenForOrganizer = true;
        _ = HideWidgetsAsync();

        async Task HideWidgetsAsync()
        {
            try
            {
                await WidgetManager.SetAllWidgetsVisibleAsync(false);
                Log("[DesktopOrganization] Existing widgets hidden while the organizer is open");
            }
            catch (Exception ex)
            {
                Log($"[DesktopOrganization] Hiding widgets failed: {ex.Message}");
            }
        }
    }

    private void RestoreWidgetsAfterOrganizer()
    {
        if (!_widgetsHiddenForOrganizer || WidgetManager is null) return;
        _widgetsHiddenForOrganizer = false;
        _ = RestoreWidgetsAsync();

        async Task RestoreWidgetsAsync()
        {
            try
            {
                await WidgetManager.SetAllWidgetsVisibleAsync(true);
                Log("[DesktopOrganization] Widgets restored after the organizer closed");
            }
            catch (Exception ex)
            {
                Log($"[DesktopOrganization] Restoring widgets failed: {ex.Message}");
            }
        }
    }

    internal void HideTrayForPetOrganizer()
    {
        if (_trayIcon is null || _trayHiddenForPetOrganizer)
        {
            return;
        }

        _trayHiddenForPetOrganizer = true;
        try { _trayIcon.Dispose(); }
        catch (Exception ex) { Log($"[Tray] Hiding the component icon failed: {ex.Message}"); }
        _trayIcon = null;
        // Keep the 1x1 tray host window alive: it owns the hotkey subclass and
        // the app's activation plumbing, and closing it tore down the freshly
        // opened organizer before it could render.
        Log("[Tray] Hidden; the pet's tray is the single NaiWa entry");
    }

    internal void RequestApplicationExit()
    {
        if (!IsShuttingDown)
        {
            ExitApplication();
        }
    }
}
