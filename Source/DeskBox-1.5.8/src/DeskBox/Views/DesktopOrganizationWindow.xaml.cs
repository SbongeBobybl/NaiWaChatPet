using DeskBox.Helpers;
using DeskBox.Platform;
using DeskBox.Services;
using Microsoft.UI;
using Microsoft.UI.Composition.SystemBackdrops;
using Microsoft.UI.Windowing;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media;
using System.Runtime.InteropServices;
using Windows.Graphics;
using Windows.System;
using WinRT.Interop;

namespace DeskBox.Views;

public sealed partial class DesktopOrganizationWindow : Window
{
    private const int DesiredWidth = 1180;
    private const int DesiredHeight = 820;
    private const int MinimumWidth = 720;
    private const int MinimumHeight = 500;
    private const int WorkAreaMargin = 64;
    private const uint WmGetMinMaxInfo = 0x0024;
    private const uint WmNcDestroy = 0x0082;
    private static readonly UIntPtr DesktopOrganizationWindowSubclassId = new(1);

    private readonly IntPtr _hWnd;
    private readonly AppWindow _appWindow;
    private readonly Win32Helper.SubclassProc _windowSubclassProc;
    private bool _hasStarted;
    private bool _hasAppliedInitialSize;
    private bool _allowClose;
    private IntPtr _ownerHwnd;
    private bool _isSubclassInstalled;

    public DesktopOrganizationWindow()
    {
        InitializeComponent();
        Title = "奶蛙 · 整理桌面";
        // The XAML solid background is the no-material fallback; Mica/Acrylic
        // still shows through when the platform supports it.
        WindowsCompatibilityService.ApplySafeBackdrop(this);
        ExtendsContentIntoTitleBar = true;
        SetTitleBar(AppTitleBar);

        _windowSubclassProc = WindowSubclassProc;
        _hWnd = WindowNative.GetWindowHandle(this);
        WindowId windowId = Win32Interop.GetWindowIdFromWindow(_hWnd);
        _appWindow = AppWindow.GetFromWindowId(windowId);
        AppBranding.ApplyWindowIcon(_appWindow);
        TaskView.OwnerWindowHandle = _hWnd;
        InstallMinimumSizeHook();

        if (_appWindow.Presenter is OverlappedPresenter presenter)
        {
            presenter.IsResizable = true;
            presenter.IsMaximizable = true;
            presenter.IsMinimizable = true;
        }

        TaskView.CloseRequested += TaskView_CloseRequested;
        TaskView.OrganizationCompleted += TaskView_OrganizationCompleted;
        TaskView.OrganizationUndone += TaskView_OrganizationUndone;
        _appWindow.Closing += AppWindow_Closing;
        AppTitleBar.ActualThemeChanged += AppTitleBar_ActualThemeChanged;
        Closed += DesktopOrganizationWindow_ClosedInternal;
        Closed += DesktopOrganizationWindow_Closed;
        App.Current.SettingsService.SettingsChanged += OnNaiWaAppearanceSettingsChanged;
        ResizeAndCenter(windowId);
        ApplyNaiWaWindowTheme();
    }

    private void OnNaiWaAppearanceSettingsChanged()
    {
        if (DispatcherQueue.HasThreadAccess)
        {
            ApplyNaiWaWindowTheme();
            return;
        }

        DispatcherQueue.TryEnqueue(ApplyNaiWaWindowTheme);
    }

    private void ApplyNaiWaWindowTheme()
    {
        var settings = App.Current.SettingsService.Settings;
        bool dark = settings.NaiWaWidgetTheme == "dark";
        RootGrid.RequestedTheme = dark ? ElementTheme.Dark : ElementTheme.Light;
        var background = new SolidColorBrush(dark
            ? Windows.UI.Color.FromArgb(255,36,33,26)
            : Windows.UI.Color.FromArgb(255,255,252,245));
        RootGrid.Background = background;
        ContentLayer.Background = background;
        AppTitleText.Foreground = new SolidColorBrush(dark
            ? Windows.UI.Color.FromArgb(255,245,234,208)
            : Windows.UI.Color.FromArgb(255,37,34,22));
        TaskView.ApplyNaiWaAppearance();
        ApplyTitleBarColors();
    }

    public event EventHandler? OrganizationCompleted;

    public event EventHandler? OrganizationUndone;

    public IntPtr WindowHandle => _hWnd;

    public void SetOwner(IntPtr ownerHwnd)
    {
        if (ownerHwnd == IntPtr.Zero || ownerHwnd == _hWnd || ownerHwnd == _ownerHwnd)
        {
            return;
        }

        _ownerHwnd = ownerHwnd;
        _ = Win32Helper.SetWindowLongPtr(
            _hWnd,            Win32Helper.GWLP_HWNDPARENT,            ownerHwnd);
        ResizeAndCenter(Win32Interop.GetWindowIdFromWindow(ownerHwnd));
    }

    public void ShowWindow()
    {
        if (!_hasStarted)
        {
            _hasStarted = true;
            TaskView.BeginScan();
        }

        _appWindow.Show();
        if (!_hasAppliedInitialSize)
        {
            _hasAppliedInitialSize = true;
            // XamlRoot and the window DPI are only reliable after the first
            // show; sizing in the constructor leaves a 150% display at 786 DIP.
            _ = DispatcherQueue.TryEnqueue(() =>
                ResizeAndCenter(Win32Interop.GetWindowIdFromWindow(_hWnd)));
        }
        // Route through the manager so a quick-reveal raised session (widget
        // group held topmost) lifts this window above the widgets instead of
        // leaving it in the normal band below them.
        App.Current.WidgetManager?.BringAuxiliaryWindowToFront(
            _hWnd,            "desktop-organization-shown");
        Activate();
        _ = Win32Helper.SetForegroundWindow(_hWnd);
    }

    /// <summary>
    /// The pet's "整理桌面" entry opens the AI classification/layout preview
    /// instead of the classic move-oriented preview.
    /// </summary>
    public void RequestAiPreview() => TaskView.RequestAiPreview();

    public void CloseForShutdown()
    {
        _allowClose = true;
        TaskView.CancelPendingWork();
        Close();
    }

    private void ResizeAndCenter(WindowId displayWindowId)
    {
        DisplayArea displayArea = DisplayArea.GetFromWindowId(
            displayWindowId,            DisplayAreaFallback.Primary);
        RectInt32 workArea = displayArea.WorkArea;
        double scale = Win32Helper.GetDpiScaleForWindow(
            _ownerHwnd != IntPtr.Zero ? _ownerHwnd : _hWnd,            RootGrid.XamlRoot);
        int width = Math.Clamp(
            (int)Math.Round(DesiredWidth * scale),            (int)Math.Round(MinimumWidth * scale),            Math.Max((int)Math.Round(MinimumWidth * scale),workArea.Width - (int)Math.Round(WorkAreaMargin * scale)));
        int height = Math.Clamp(
            (int)Math.Round(DesiredHeight * scale),            (int)Math.Round(MinimumHeight * scale),            Math.Max((int)Math.Round(MinimumHeight * scale),workArea.Height - (int)Math.Round(WorkAreaMargin * scale)));
        _appWindow.MoveAndResize(new RectInt32(
            workArea.X + Math.Max(0,(workArea.Width - width) / 2),            workArea.Y + Math.Max(0,(workArea.Height - height) / 2),            width,            height));
    }

    private void AppWindow_Closing(AppWindow sender,AppWindowClosingEventArgs args)
    {
        if (_allowClose)
        {
            return;
        }

        if (TaskView.IsExecutionRunning)
        {
            args.Cancel = true;
            TaskView.CancelExecutionAndCloseWhenSafe();
            return;
        }

        TaskView.CancelPendingWork();
    }

    private void TaskView_CloseRequested(object? sender,EventArgs e)
    {
        if (TaskView.IsExecutionRunning)
        {
            TaskView.CancelExecutionAndCloseWhenSafe();
            return;
        }

        _allowClose = true;
        Close();
    }

    private void TaskView_OrganizationCompleted(object? sender,EventArgs e) =>
        OrganizationCompleted?.Invoke(this,EventArgs.Empty);

    private void TaskView_OrganizationUndone(object? sender,EventArgs e) =>
        OrganizationUndone?.Invoke(this,EventArgs.Empty);

    private void RootGrid_KeyDown(object sender,KeyRoutedEventArgs e)
    {
        if (e.Key != VirtualKey.Escape)
        {
            return;
        }

        e.Handled = true;
        if (TaskView.IsExecutionRunning)
        {
            TaskView.CancelExecutionAndCloseWhenSafe();
        }
        else
        {
            _allowClose = true;
            Close();
        }
    }

    private void AppTitleBar_ActualThemeChanged(FrameworkElement sender,object args) =>
        ApplyTitleBarColors();

    private void ApplyTitleBarColors()
    {
        bool isDark = RootGrid.ActualTheme switch
        {
            ElementTheme.Dark => true,            ElementTheme.Light => false,            _ => Win32Helper.IsSystemDarkMode()
        };
        AppWindowTitleBar titleBar = _appWindow.TitleBar;
        titleBar.ButtonBackgroundColor = Colors.Transparent;
        titleBar.ButtonInactiveBackgroundColor = Colors.Transparent;
        titleBar.ButtonForegroundColor = isDark ? Colors.White : Colors.Black;
        titleBar.ButtonInactiveForegroundColor = isDark
            ? Windows.UI.Color.FromArgb(0xA0,0xFF,0xFF,0xFF)
            : Windows.UI.Color.FromArgb(0xA0,0x10,0x10,0x10);
    }

    private void InstallMinimumSizeHook()
    {
        _isSubclassInstalled = Win32Helper.SetWindowSubclass(
            _hWnd,            _windowSubclassProc,            DesktopOrganizationWindowSubclassId,            UIntPtr.Zero);
    }

    private void RemoveMinimumSizeHook()
    {
        if (!_isSubclassInstalled)
        {
            return;
        }

        Win32Helper.RemoveWindowSubclass(
            _hWnd,            _windowSubclassProc,            DesktopOrganizationWindowSubclassId);
        _isSubclassInstalled = false;
    }

    private IntPtr WindowSubclassProc(
        IntPtr hWnd,        uint message,        UIntPtr wParam,        IntPtr lParam,        UIntPtr subclassId,        UIntPtr refData)
    {
        if (message == WmGetMinMaxInfo)
        {
            var minMaxInfo = Marshal.PtrToStructure<MinMaxInfo>(lParam);
            double scale = Win32Helper.GetDpiScaleForWindow(_hWnd,RootGrid.XamlRoot);
            minMaxInfo.MinTrackSize.X = Math.Max(
                minMaxInfo.MinTrackSize.X,                ToPhysicalPixels(MinimumWidth,scale));
            minMaxInfo.MinTrackSize.Y = Math.Max(
                minMaxInfo.MinTrackSize.Y,                ToPhysicalPixels(MinimumHeight,scale));
            Marshal.StructureToPtr(minMaxInfo,lParam,false);
            return IntPtr.Zero;
        }

        if (message == WmNcDestroy)
        {
            RemoveMinimumSizeHook();
        }

        return Win32Helper.DefSubclassProc(hWnd,message,wParam,lParam);
    }

    private static int ToPhysicalPixels(int logicalPixels,double scale)
    {
        double normalizedScale = double.IsFinite(scale) && scale > 0 ? scale : 1.0;
        return Math.Max(
            1,            (int)Math.Round(logicalPixels * normalizedScale,MidpointRounding.AwayFromZero));
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct NativePoint
    {
        public int X;
        public int Y;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct MinMaxInfo
    {
        public NativePoint Reserved;
        public NativePoint MaxSize;
        public NativePoint MaxPosition;
        public NativePoint MinTrackSize;
        public NativePoint MaxTrackSize;
    }

    private void DesktopOrganizationWindow_Closed(object sender,WindowEventArgs args)
    {
        RemoveMinimumSizeHook();
        TaskView.CancelPendingWork();
        TaskView.CloseRequested -= TaskView_CloseRequested;
        TaskView.OrganizationCompleted -= TaskView_OrganizationCompleted;
        TaskView.OrganizationUndone -= TaskView_OrganizationUndone;
        _appWindow.Closing -= AppWindow_Closing;
        AppTitleBar.ActualThemeChanged -= AppTitleBar_ActualThemeChanged;
        Closed -= DesktopOrganizationWindow_Closed;
    }

    private void DesktopOrganizationWindow_ClosedInternal(object sender,WindowEventArgs args)
    {
        WindowsCompatibilityService.ReleaseTitleBarExtensions(this,_appWindow);
        App.Current.SettingsService.SettingsChanged -= OnNaiWaAppearanceSettingsChanged;

        // This window closes for real (no hide-and-reuse), so leaving the
        // quick-reveal raised band happens on teardown; the manager skips
        // destroyed handles either way, but releasing keeps the guest
        // registry exact for the sweep logs.
        App.Current.WidgetManager?.ReleaseRaisedBandGuest(
            _hWnd,            "desktop-organization-closed");
        Closed -= DesktopOrganizationWindow_ClosedInternal;
    }
}
