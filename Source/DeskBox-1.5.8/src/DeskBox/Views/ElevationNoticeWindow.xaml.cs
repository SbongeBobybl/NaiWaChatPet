using DeskBox.Helpers;
using DeskBox.Platform;
using DeskBox.Services;
using Microsoft.UI;
using Microsoft.UI.Windowing;
using Microsoft.UI.Xaml;
using WinRT.Interop;

namespace DeskBox.Views;

/// <summary>Body/button shape the elevation notice takes.</summary>
public enum ElevationNoticeVariant
{
    /// <summary>DeskBox is elevated above the shell. Offer a one-click exit
    /// of the elevated instance; the user restarts DeskBox normally (a
    /// plain launch lands at the shell's standard-user level). The notice
    /// deliberately no longer relaunches the app itself: the shell-parented
    /// hand-over spawned a Medium child whose startup state differed from a
    /// normal launch (silent-startup hide followed by a raised reveal), and
    /// that combination starved drags on the current tree. Self-serve keeps
    /// every restarted instance on the plain, field-proven path.</summary>
    ExitElevatedInstance,

    /// <summary>The shell runs above this process: tray creation and
    /// cross-level drag are UIPI-blocked and the app cannot change the
    /// shell's token. Explain the degraded session instead.</summary>
    ExplainShellAboveApp
}

/// <summary>
/// One-shot notice shown when DeskBox and the shell sit at different
/// integrity levels. An elevated DeskBox under a standard shell can neither
/// receive OLE drops nor start XAML drags, so it offers a one-click exit and
/// asks the user to restart DeskBox normally; a standard DeskBox under an
/// elevated shell has its tray blocked and gets an explanation instead.
/// Same-level sessions (including fully elevated desktops) never see this
/// window.
/// </summary>
public sealed partial class ElevationNoticeWindow : Window
{
    private const int DesiredWindowWidth = 500;
    private const int DesiredWindowHeight = 360;
    private const int WindowWorkAreaMargin = 80;

    private readonly AppWindow _appWindow;
    private readonly IntPtr _hWnd;
    private bool _exitStarted;

    /// <summary>
    /// Pure variant selection, testable without a window: the notice never
    /// offers an action the environment cannot deliver, and never claims a
    /// fixable problem on same-level desktops (those never reach this
    /// window).
    /// </summary>
    internal static ElevationNoticeVariant SelectVariant(StartupElevationAssessment assessment) =>
        assessment.Mismatch == ShellIntegrityMismatch.ShellAboveApp
            ? ElevationNoticeVariant.ExplainShellAboveApp
            : ElevationNoticeVariant.ExitElevatedInstance;

    public ElevationNoticeWindow(
        LocalizationService localizationService,        StartupElevationAssessment assessment)
    {
        InitializeComponent();

        WindowsCompatibilityService.ApplySafeBackdrop(this);

        Title = localizationService.T("Startup.ElevationNotice.Title");
        TitleText.Text = localizationService.T("Startup.ElevationNotice.Title");
        ExitButton.Content = localizationService.T("Startup.ElevationNotice.ExitButton");
        OkButton.Content = localizationService.T("Startup.ElevationNotice.OkButton");

        ElevationNoticeVariant variant = SelectVariant(assessment);
        BodyText.Text = variant == ElevationNoticeVariant.ExplainShellAboveApp
            ? localizationService.T("Startup.ElevationNotice.BodyShellAboveApp")
            : localizationService.T("Startup.ElevationNotice.BodyExitAndRestart");

        if (variant == ElevationNoticeVariant.ExitElevatedInstance)
        {
            ExitButton.Visibility = Visibility.Visible;
            OkButton.Visibility = Visibility.Collapsed;
        }
        else
        {
            ExitButton.Visibility = Visibility.Collapsed;
            OkButton.Visibility = Visibility.Visible;
        }

        _hWnd = WindowNative.GetWindowHandle(this);
        var windowId = Win32Interop.GetWindowIdFromWindow(_hWnd);
        _appWindow = AppWindow.GetFromWindowId(windowId);
        AppBranding.ApplyWindowIcon(_appWindow);

        if (_appWindow.Presenter is OverlappedPresenter presenter)
        {
            presenter.IsResizable = false;
            presenter.IsMaximizable = false;
            presenter.IsMinimizable = false;
        }

        ResizeAndCenterForDisplay(windowId);
    }

    private async void ExitButton_Click(object sender,RoutedEventArgs e)
    {
        if (_exitStarted)
        {
            return;
        }

        _exitStarted = true;
        ExitButton.IsEnabled = false;
        OkButton.IsEnabled = false;

        // The full shutdown path carries the deadline and hard-exit
        // backstops that guarantee process death (measured 0.5 s), so the
        // single-instance mutex frees for the user's normal restart.
        App.Log("[Elevation] Exiting elevated instance from notice; user restart expected");
        Close();
        await App.Current.ShutdownForRestartAsync();
        App.Log("[Elevation] ShutdownForRestartAsync returned after notice exit");
    }

    private void ContinueButton_Click(object sender,RoutedEventArgs e)
    {
        Close();
    }

    private void ResizeAndCenterForDisplay(Microsoft.UI.WindowId windowId)
    {
        var workArea = DisplayArea.GetFromWindowId(windowId,DisplayAreaFallback.Primary).WorkArea;
        double scale = Win32Helper.GetDpiScaleForWindow(_hWnd,RootGrid.XamlRoot);
        int width = ToPhysicalPixels(DesiredWindowWidth,scale);
        int height = ToPhysicalPixels(DesiredWindowHeight,scale);
        int workAreaMargin = ToPhysicalPixels(WindowWorkAreaMargin,scale);
        width = Math.Min(width,Math.Max(0,workArea.Width - workAreaMargin));
        height = Math.Min(height,Math.Max(0,workArea.Height - workAreaMargin));

        _appWindow.Resize(new Windows.Graphics.SizeInt32(width,height));
        _appWindow.Move(new Windows.Graphics.PointInt32(
            workArea.X + Math.Max(0,(workArea.Width - width) / 2),            workArea.Y + Math.Max(0,(workArea.Height - height) / 2)));
    }

    private static int ToPhysicalPixels(int logicalPixels,double scale)
    {
        return (int)Math.Round(logicalPixels * scale);
    }
}
