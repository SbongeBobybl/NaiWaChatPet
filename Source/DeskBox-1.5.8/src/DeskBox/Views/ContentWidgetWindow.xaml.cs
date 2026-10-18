using DeskBox.Contracts;
using DeskBox.Controls;
using DeskBox.Controls.WidgetContents;
using DeskBox.Helpers;
using DeskBox.Models;
using DeskBox.Platform;
using DeskBox.Services;
using DeskBox.ViewModels;
using System.ComponentModel;
using Microsoft.UI;
using Microsoft.UI.Composition;
using Microsoft.UI.Composition.SystemBackdrops;
using Microsoft.UI.Input;
using Microsoft.UI.Windowing;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Automation;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Controls.Primitives;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media;
using Windows.Graphics;
using WinRT;
using WinRT.Interop;

namespace DeskBox.Views;

/// <summary>
/// Lightweight host window for future non-file widget content.
/// User-facing creation remains gated by WidgetRegistry.
/// </summary>
public sealed partial class ContentWidgetWindow : WidgetWindowBase, IDesktopWidgetWindow
{
    private WidgetConfig _config;
    private WidgetContentDescriptor _descriptor;
    private readonly WidgetChromeModeResolver _chromeModeResolver;
    private readonly WidgetShellContentHost _contentHost;
    private readonly ContentWidgetTitleViewModel _titleViewModel;
    private readonly Task _contentLoadTask;
    private const int RevealCompletedBackgroundDelayMs = 240;
    private readonly Dictionary<string, IWidgetContent> _cachedGroupContents =
        new(StringComparer.Ordinal);
    private readonly LinkedList<string> _cachedGroupContentOrder = [];

    private bool _isHidePrepared;
    private bool _isCommittingTitleRename;
    private bool _isCancellingTitleRename;
    private long _titleRenameOpenedAtTick;
    private bool _compactPresentationRefreshQueued;
    private INotifyPropertyChanged? _compactPresentationSource;
    private IWidgetFeedbackSource? _feedbackSource;
    private IWidgetHostContextMenuSource? _hostContextMenuSource;
    private Microsoft.UI.Dispatching.DispatcherQueueTimer? _autoRestoreTimer;

    private bool _isVisibleOnDesktop;
    private int _contentVisibilityGeneration;
    private int _queuedContentRevealGeneration = -1;

    public ContentWidgetWindow(
        WidgetConfig config,        IWidgetContent content,        SettingsService settingsService,        WidgetContentDescriptor descriptor)
    {
        _config = config;
        _descriptor = descriptor;
        _chromeModeResolver = new WidgetChromeModeResolver(settingsService);

        InitializeComponent();

        SettingsService = settingsService;
        ApplyNaiWaTheme();
        HWnd = WindowNative.GetWindowHandle(this);
        var windowId = Win32Interop.GetWindowIdFromWindow(HWnd);
        AppWindow = AppWindow.GetFromWindowId(windowId);
        Diagnostics = new WidgetWindowDiagnostics("Content",_config,() => HWnd);
        TrayAnimation = new WidgetTrayAnimationController(
            AppWindow,            RootGrid,            DispatcherQueue,            HWnd,            GetCurrentAnimationBounds,            LogTrayWindow);
        _contentHost = new WidgetShellContentHost(
            ContentWidgetShell,            TryRetainGroupContent);

        _titleViewModel = new ContentWidgetTitleViewModel(_config,settingsService);
        ContentWidgetShell.DataContext = _titleViewModel;
        ApplyTitleIdentity(_config,descriptor,content);
        ContentWidgetShell.ShowHoverButtons = settingsService.Settings.ShowHoverButtons;
        ContentWidgetShell.IsTitleEditable = true;
        ApplyLocalizedTitleActionTooltips();

        ConfigureWindowCore();
        ApplyTitleBarLayout();
        SetupEventHandlers();
        
        // ✅ Set initial title
        this.Title = App.Current.LocalizationService.T("Window.ContentWidget.Title");
        
        _contentLoadTask = LoadContentAsync(content);

        App.Current.LocalizationService.LanguageChanged += OnLanguageChanged;
    }

    // ── Abstract member overrides ──────────────────────────────

    public override WidgetConfig Config => _config;
    protected override double WidgetOpacity => _config.WidgetKind == WidgetKind.File
        ? (SettingsService.Settings.NaiWaTransparentBody ? 0 : 1) : SettingsService.Settings.WidgetOpacity;
    protected override string EffectiveWidgetMaterial => _config.WidgetKind == WidgetKind.File
        ? DeskBox.Services.SettingsService.WidgetMaterialTypeSolid : base.EffectiveWidgetMaterial;
    protected override FrameworkElement RootElement => RootGrid;
    protected override WidgetShell WidgetShellControl => ContentWidgetShell;
    protected override string LogPrefix => "Content";
    protected override bool IsSizeLocked => _config.IsSizeLocked;
    protected override bool IsPositionLocked => _config.IsPositionLocked;
    protected override bool IsCompactExpansionWarmupContentReady =>
        _contentHost.CurrentContent is not null;

    protected override bool HasBlockingFlyoutOpen() =>
        base.HasBlockingFlyoutOpen() ||
        CurrentContent is FileWidgetContentAdapter
        {
            IsStackPopoverBlockingSurfaceOpen: true
        };

    protected override void OnCompactBoundsTransitionActiveChanged(bool isActive)
    {
        CurrentContent?.OnCompactBoundsTransitionActiveChanged(isActive);
    }

    protected override void OnResizeStart()
    {
        if (CurrentContent is IWidgetInteractiveResizeContent resizeContent)
        {
            double titleHeight = Math.Max(0,ContentWidgetShell.ActualTitleBarHeight);
            resizeContent.BeginInteractiveResize(
                Math.Max(1,RootGrid.ActualWidth),                Math.Max(1,RootGrid.ActualHeight - titleHeight));
        }
    }

    protected override WidgetCompactPresentation CreateCompactPresentation()
    {
        var localization = App.Current.LocalizationService;
        string contentMode = ResolveEffectiveCompactContentMode();
        return CurrentContent switch
        {
            FileWidgetContentAdapter file =>
                CreateFileCompactPresentation(file,contentMode),            _ => new WidgetCompactPresentation(
                _titleViewModel.DisplayName,                string.Empty,                _descriptor.DefaultGlyph,                localization.T("Widget.Compact.DropHint"),                EnableMarquee: true,                LiveStateKey: _titleViewModel.DisplayName)
        };
    }

    protected override void UpdateConfigBoundsFromPhysical(
        int x,int y,int width,int height,bool persist)
    {
        if (persist && !CanPersistBoundsChange(persist))
        {
            return;
        }

        if (IsCompactBoundsStateActive)
        {
            if (persist)
            {
                SettingsService.UpdateWidget(_config,notifySubscribers: false);
                SettingsService.SaveDebounced(notifySubscribers: false);
                SynchronizeWidgetGroupLayout();
            }
            return;
        }

        var bounds = CollapseHostBoundsToContent(
            new RectInt32(x,y,width,height));
        // Use center point for consistent monitor determination across drag/resize.
        var center = new PointInt32(
            bounds.X + Math.Max(1,bounds.Width) / 2,            bounds.Y + Math.Max(1,bounds.Height) / 2);
        var workArea = DisplayArea.GetFromPoint(center,DisplayAreaFallback.Nearest).WorkArea;
        WidgetPositioningService.UpdateConfigFromPhysicalBounds(_config,bounds,workArea);
        if (persist)
        {
            SettingsService.UpdateWidget(_config,notifySubscribers: false);
            SettingsService.SaveDebounced();
            SynchronizeWidgetGroupLayout();
        }
    }

    protected override Windows.UI.Color BuildNativeBackdropTintColor(bool isDark)
    {
        var accentColor = App.Current.ThemeService?.GetEffectiveAccentColor()
            ?? AccentColorHelper.DefaultAccentColor;
        return WidgetMaterialVisualCalculator.BuildContentTintColor(isDark,accentColor);
    }

    protected override Windows.UI.Color BuildSolidColorBackdropTintColor(
        bool isDark,        double surfaceOpacity)
    {
        if (_config.WidgetKind == WidgetKind.File)
            return SettingsService.Settings.NaiWaTransparentBody ? Colors.Transparent : NaiWaSurfaceColor;
        var accentColor = App.Current.ThemeService?.GetEffectiveAccentColor()
            ?? AccentColorHelper.DefaultAccentColor;
        return WidgetMaterialVisualCalculator.BuildContentSolidSurfaceColor(
            isDark,            accentColor,            surfaceOpacity);
    }

    // ── Virtual hooks ──────────────────────────────────────────

    protected override void ApplySurfaceStyle()
    {
        bool isDark = RootGrid.ActualTheme == ElementTheme.Dark;
        double surfaceOpacity = Math.Clamp(SettingsService.Settings.WidgetOpacity,0.0,1.0);
        var accentColor = App.Current.ThemeService?.GetEffectiveAccentColor()
            ?? AccentColorHelper.DefaultAccentColor;
        string materialType = WindowsCompatibilityService.ResolveWidgetMaterialType(
            SettingsService.Settings.WidgetMaterialType);

        // Simplified layering: only apply surface color overlay for Solid mode.
        if (materialType is SettingsService.WidgetMaterialTypeSolid && !IsSolidColorBackdropActive)
        {
            var surfaceColor = WidgetMaterialVisualCalculator.BuildContentSolidSurfaceColor(
                isDark,                accentColor,                surfaceOpacity);
            ContentWidgetShell.BackgroundSurface.Background = GetOrUpdateSolidColorBrush(
                ContentWidgetShell.BackgroundSurface.Background,                surfaceColor);
        }
        else if (WindowsCompatibilityService.UsesLegacyWindowAcrylic &&
            SettingsService.IsAcrylicMaterial(materialType))
        {
            var overlayColor = WidgetMaterialVisualCalculator.BuildLegacyAcrylicSurfaceOverlayColor(
                isDark,                accentColor,                materialType == SettingsService.WidgetMaterialTypeAcrylicBase,                surfaceOpacity,                SettingsService.Settings.WidgetMaterialIntensity);
            ContentWidgetShell.BackgroundSurface.Background = GetOrUpdateSolidColorBrush(
                ContentWidgetShell.BackgroundSurface.Background,                overlayColor);
        }
        else
        {
            ContentWidgetShell.BackgroundSurface.Background = GetOrUpdateSolidColorBrush(
                ContentWidgetShell.BackgroundSurface.Background,                Colors.Transparent);
        }

        var (borderThickness, borderColor, dividerColor) = GetWidgetBorderVisuals(isDark,accentColor);
        var iconForeground = ColorHelper.FromArgb(
            isDark ? (byte)0xE2 : (byte)0xCC,            accentColor.R,            accentColor.G,            accentColor.B);

        ContentWidgetShell.BackgroundSurface.BorderThickness = new Thickness(borderThickness);
        ContentWidgetShell.BackgroundSurface.BorderBrush = GetOrUpdateSolidColorBrush(
            ContentWidgetShell.BackgroundSurface.BorderBrush,            borderColor);
        ContentWidgetShell.BackgroundSurface.CornerRadius = new CornerRadius(GetCurrentSurfaceCornerRadius());
        ContentWidgetShell.Divider.Background = GetOrUpdateSolidColorBrush(
            ContentWidgetShell.Divider.Background,            dividerColor);
        ContentWidgetShell.TitleIconAccentColor = iconForeground;
        ContentWidgetShell.TitleIconMode = SettingsService.Settings.WidgetTitleIconMode;
        ApplyCustomTitleIcon();
        ApplyCustomWidgetBackground();
        if (_config.WidgetKind == WidgetKind.File)
        {
            ContentWidgetShell.BackgroundSurface.Background = new SolidColorBrush(
                SettingsService.Settings.NaiWaTransparentBody ? Colors.Transparent : NaiWaSurfaceColor);
            ContentWidgetShell.BackgroundSurface.BorderThickness = new Thickness(0);
            ContentWidgetShell.TitleBar.Background = new SolidColorBrush(NaiWaSurfaceColor);
            ContentWidgetShell.TitleBar.CornerRadius = new CornerRadius(12);
        }
        WidgetTextShadow.SetEnabled(
            SettingsService.Settings.WidgetShell.WidgetTextShadowEnabled);
    }

    protected override void OnRootElementLoaded()
    {
        RootGrid.Focus(FocusState.Programmatic);
        QueueNativeFileDropTargetRegistration();
    }

    protected override void OnCustomWidgetBackgroundChanged(bool active)
    {
    }

    // ── IDesktopWidgetWindow implementation ────────────────────

    public IntPtr WindowHandle => HWnd;
    public WidgetWindowIdentity Identity => Diagnostics.Identity;
    public Windows.Foundation.Rect AnimationBounds => GetCurrentAnimationBounds();
        public Windows.Foundation.Rect RestingAnimationBounds => TrayAnimation.GetRestingAnimationBounds();

    public new bool Visible
    {
        get => _isVisibleOnDesktop;
        private set => _isVisibleOnDesktop = value;
    }

    internal IWidgetContent? CurrentContent => _contentHost.CurrentContent;

    internal Task ContentReadyTask => _contentLoadTask;

    public void ApplyAppearancePreview()
    {
        ApplyAppearancePreview(invalidateContent: true);
    }

    private void ApplyAppearancePreview(bool invalidateContent)
    {
        if (!DispatcherQueue.HasThreadAccess)
        {
            DispatcherQueue.TryEnqueue(() => ApplyAppearancePreview(invalidateContent));
            return;
        }

        if (IsClosing)
        {
            return;
        }

        ApplyNaiWaTheme();
        ApplyWindowCornerPreference();
        ApplyWidgetForegroundAppearance();
        ApplyBackdropPreference();
        ContentWidgetShell.ShowHoverButtons = SettingsService.Settings.ShowHoverButtons;
        ApplyTitleBarLayout();
        _contentHost.ApplyAppearance(invalidate: invalidateContent);
    }

    public void SetTrayAnimationOffsetOverride(double? offsetX,double? offsetY)
    {
        TrayAnimation.SetOffsetOverride(offsetX,offsetY);
    }

    public void CancelTrayAnimationAndRestorePosition()
    {
        if (!Visible && IsHideAnimationRunning)
        {
            CompleteTrayHideAnimation();
            return;
        }

        long animationGeneration = TrayAnimation.NextGeneration();
        TrayAnimation.Stop();
        SetTrayAnimationOffsetOverride(null,null);
        TrayAnimation.RestoreVisualState();
        TrayAnimation.RestoreWindowPosition();
        TrayAnimation.RevealWindowForTrayShow();
        SetTrayHideInputSuppressed(false);
        IsHideAnimationRunning = false;
        _isHidePrepared = false;
        LogTrayWindow($"CancelAnimationAndRestore gen={animationGeneration}");
    }

public void PrepareTrayShowAnimation()
{
PrepareTrayShowAnimationCore(restoreBoundsForCurrentTopology: false);
}

internal bool PrepareTrayShowAnimationForCurrentTopology()
{
return PrepareTrayShowAnimationCore(restoreBoundsForCurrentTopology: true);
}

private bool PrepareTrayShowAnimationCore(bool restoreBoundsForCurrentTopology)
{
        bool boundsRestored = false;
        bool prepared = WidgetTrayAnimationPreparation.TryPrepare(() =>
        {
            SetTrayHideInputSuppressed(false);
            TrayAnimation.NextGeneration();
            TrayAnimation.StopAndRestoreWindowPosition();
            TrayAnimation.CloakWindowForTrayShow();
            _isHidePrepared = false;
            IsHideAnimationRunning = false;

            // A group detach changes this persistent HWND's topology identity.
            // Retarget it only after DWM cloak is active and before preparing the
            // animation offset; otherwise TryRestoreBounds sees an active position
            // transition and intentionally skips the move.
            boundsRestored = !restoreBoundsForCurrentTopology ||
                TryRestoreBoundsForCurrentTopology(allowHidden: true);

            var profile = GetTrayAnimationProfile();
            LogTrayWindow(
                $"PrepareShow gen={TrayAnimation.Generation} topologyRetarget={restoreBoundsForCurrentTopology} " +
                $"boundsRestored={boundsRestored} effect={SettingsService.Settings.WidgetAnimationEffect} " +
                $"speed={SettingsService.Settings.WidgetAnimationSpeed} enabled={profile.IsEnabled} durationMs={profile.DurationMs}");
            TrayAnimation.PrepareVisualState(
                profile.ShowOffsetX,                profile.ShowOffsetY,                profile.ShowStartOpacity,                profile.ShowStartScale);
        },CompleteTrayShowWithoutAnimation,            ex => LogTrayWindow($"PrepareShow failed: {ex.Message}"));
        return prepared && boundsRestored;
    }

    public void ShowPreparedAtDesktopLayer(bool persistVisibility = true)
    {
        ShowWithoutActivation(persistVisibility);
        TrayAnimation.PrepareHiddenState();
        PushToBottom(showWindow: false);
        TrayAnimation.RevealWindowForTrayShow();
    }

    public void ShowPreparedRaisedFromTray(bool persistVisibility = true)
    {
        ShowWithoutActivation(persistVisibility);
        TrayAnimation.PrepareHiddenState();
        HoldTemporaryTopMost(showWindow: false);
        TrayAnimation.RevealWindowForTrayShow();
    }

    public void PlayTrayShowAnimation()
    {
        PlayTrayRaiseAnimationAfterFirstFrame();
    }

    public void CompleteTrayShowWithoutAnimation()
    {
        IsHideAnimationRunning = false;
        _isHidePrepared = false;
        SetTrayHideInputSuppressed(false);
        TrayAnimation.NextGeneration();
        LogTrayWindow($"CompleteShowWithoutAnimation gen={TrayAnimation.Generation}");
        TrayAnimation.Stop();
        SetTrayAnimationOffsetOverride(null,null);
        TrayAnimation.RestoreVisualState();
        WidgetTrayAnimationPreparation.CompleteShow(
            TrayAnimation.RestoreWindowPosition,            TrayAnimation.RevealWindowForTrayShow,            NotifyVisibleContentRevealCompleted,            ex => LogTrayWindow($"CompleteShowWithoutAnimation cleanup failed: {ex.Message}"));
    }

    public void RevealFromTray(bool autoRestore = true)
    {
        PrepareTrayShowAnimation();
        ShowPreparedRaisedFromTray();
        ActivateRaisedFromTrayBatch();
        PlayTrayShowAnimation();

        if (!autoRestore)
        {
            _autoRestoreTimer?.Stop();
            return;
        }

        if (_autoRestoreTimer is null)
        {
            _autoRestoreTimer = DispatcherQueue.CreateTimer();
            _autoRestoreTimer.IsRepeating = false;
            _autoRestoreTimer.Tick += AutoRestoreTimer_Tick;
            PerformanceLogger.RecordTransientUiTimerCreated();
        }
        else
        {
            _autoRestoreTimer.Stop();
        }

        _autoRestoreTimer.Interval = TimeSpan.FromMilliseconds(1200);
        _autoRestoreTimer.Start();
    }

    private void AutoRestoreTimer_Tick(
        Microsoft.UI.Dispatching.DispatcherQueueTimer sender,        object args)
    {
        sender.Stop();
        if (Visible &&
            !IsHideAnimationRunning &&
            !IsDragging &&
            !IsResizing)
        {
            RestoreDesktopLayer(force: true);
        }
    }

    private void ReleaseAutoRestoreTimer()
    {
        Microsoft.UI.Dispatching.DispatcherQueueTimer? timer =
            _autoRestoreTimer;
        if (timer is null)
        {
            return;
        }

        _autoRestoreTimer = null;
        timer.Stop();
        timer.Tick -= AutoRestoreTimer_Tick;
        PerformanceLogger.RecordTransientUiTimerReleased();
    }

    public bool PrepareTrayHideAnimation(bool persistVisibility = true)
    {
        if (!Visible || IsHideAnimationRunning)
        {
            LogTrayWindow($"PrepareHide skipped visible={Visible} hideRunning={IsHideAnimationRunning}");
            return false;
        }

        PrepareCompactHostForTrayHide();
        _autoRestoreTimer?.Stop();
        CancelPendingDesktopLayerRestore();
TrayAnimation.NextGeneration();
TrayAnimation.RevealWindowForTrayShow();
if (TrayAnimation.IsPositionTransitionActive)
{
    TrayAnimation.StopAndRestoreWindowPosition();
}
else
{
    TrayAnimation.Stop();
}
IsHideAnimationRunning = true;
        _isHidePrepared = true;
        SetTrayHideInputSuppressed(true);
        Visible = false;
        NotifyCompactHostVisibilityChanged(false);
        UpdatePersistedVisibility(isVisible: false,persistVisibility);

        LogTrayWindow($"PrepareHide gen={TrayAnimation.Generation}");
        return WidgetTrayAnimationPreparation.TryPrepare(
            () => TrayAnimation.PrepareVisualState(0,0,WidgetTrayAnimationController.RestingOpacity,WidgetTrayAnimationController.RestingScale),            CompleteTrayHideAnimation,            ex => LogTrayWindow($"PrepareHide failed: {ex.Message}"));
    }

    public void PlayPreparedTrayHideAnimation()
    {
        if (!_isHidePrepared || !IsHideAnimationRunning)
        {
            return;
        }

        PlayTrayHideAnimation(CompleteTrayHideAnimation);
    }

    public void ActivateRaisedFromTrayBatch()
    {
        if (!Visible)
        {
            return;
        }

        HoldTemporaryTopMost();
        base.Activate();
        Win32Helper.SetForegroundWindow(HWnd);
        RootGrid.Focus(FocusState.Programmatic);
        _contentHost.OnActivated();
    }

    public void EnsureRaisedFromTrayTopMost()
    {
        if (!Visible)
        {
            return;
        }

        AppWindow.Show();
        Win32Helper.ShowWindow(HWnd,Win32Helper.SW_SHOWNORMAL);
        WidgetLayerService.BringToFront(HWnd);
        HoldTemporaryTopMost();
    }

    public void ForceRestoreDesktopLayerFromManager()
    {
        if (!Visible)
        {
            return;
        }

        // Manager teardown always wins over an expanded capsule lease: the
        // whole group is returning to its resting layer, so ending the lease
        // here keeps later interaction elevates working after the teardown.
        EndExpandedWidgetLayerLease();
        RestoreDesktopLayer(force: true);
        _contentHost.OnDeactivated();
    }

    public void RestoreDesktopLayerFromManager()
    {
        if (!Visible)
        {
            return;
        }

        // An expanded capsule lease owns this window's layer until its
        // collapse; a forced demote here would bury it beneath sibling
        // widgets the moment a flyout closes mid-use.
        if (!HasExpandedWidgetLayerLease)
        {
            RestoreDesktopLayer(force: true);
        }

        _contentHost.OnDeactivated();
    }

    public void HideWindow()
    {
        App.Current?.WidgetManager?.CancelWidgetSurfaceSwitch(_config.Id);
        TrayAnimation.Stop();
        TrayAnimation.RevealWindowForTrayShow();
        IsHideAnimationRunning = false;
        _isHidePrepared = false;
        SetTrayHideInputSuppressed(false);
        Visible = false;
        _autoRestoreTimer?.Stop();
        CancelPendingDesktopLayerRestore();
        UpdatePersistedVisibility(isVisible: false,persistVisibility: true);
        WidgetLayerService.ClearTopMost(HWnd);
        Win32Helper.ShowWindow(HWnd,Win32Helper.SW_HIDE);
        AppWindow.Hide();
        WidgetShellControl.SuspendVisualActivity();
        NotifyCompactHostVisibilityChanged(false);
        TrayAnimation.RestoreVisualState();
        TrayAnimation.RestoreWindowPosition();
        _contentHost.OnDeactivated();
        NotifyVisibleContentSuspended();
    }

    public void CloseWindow()
    {
        if (!DispatcherQueue.HasThreadAccess)
        {
            DispatcherQueue.TryEnqueue(CloseWindow);
            return;
        }

        if (IsClosing)
        {
            return;
        }

        IsClosing = true;
        TrayAnimation.RevealWindowForTrayShow();
        WidgetLayerService.ReleaseWindow(HWnd);
        Close();
    }

    // ── Event setup ────────────────────────────────────────────

    private void OnLanguageChanged()
    {
        _titleViewModel.RefreshDisplayName();
        ApplyLocalizedTitleActionTooltips();
        RefreshCompactPresentation();
    }

    private async Task LoadContentAsync(IWidgetContent content)
    {
        await _contentHost.SetContentAsync(content);
        if (!ReferenceEquals(_contentHost.CurrentContent,content))
        {
            return;
        }

        AttachCompactPresentationSource(content);
        AttachFeedbackSource(content);
        AttachHostContextMenuSource(content);
        RefreshCompactPresentation();
        ApplyTitleActionButtonConfiguration();
    }

    private void AttachFeedbackSource(IWidgetContent content)
    {
        if (_feedbackSource is not null)
        {
            _feedbackSource.FeedbackRequested -= FeedbackSource_FeedbackRequested;
        }

        _feedbackSource = content as IWidgetFeedbackSource;
        if (_feedbackSource is not null)
        {
            _feedbackSource.FeedbackRequested += FeedbackSource_FeedbackRequested;
        }
    }

    private void FeedbackSource_FeedbackRequested(
        object? sender,        WidgetFeedbackRequestedEventArgs e)
    {
        ContentWidgetShell.ShowFeedback(e.Request);
    }

    private void AttachHostContextMenuSource(IWidgetContent content)
    {
        if (content is FileWidgetContentAdapter fileSurface)
        {
            fileSurface.SetHostWindowHandle(HWnd);
        }

        if (_hostContextMenuSource is not null)
        {
            _hostContextMenuSource.HostContextMenuOpening -=
                HostContextMenuSource_HostContextMenuOpening;
        }

        _hostContextMenuSource = content as IWidgetHostContextMenuSource;
        if (_hostContextMenuSource is not null)
        {
            _hostContextMenuSource.HostContextMenuOpening +=
                HostContextMenuSource_HostContextMenuOpening;
        }
    }

    private void HostContextMenuSource_HostContextMenuOpening(
        object? sender,        WidgetHostContextMenuOpeningEventArgs e)
    {
        if (ReferenceEquals(sender,_contentHost.CurrentContent))
        {
            ProvideWidgetActionsForContentMenu(e);
        }
    }

    private void AttachCompactPresentationSource(IWidgetContent content)
    {
        if (_compactPresentationSource is not null)
        {
            _compactPresentationSource.PropertyChanged -= CompactPresentationSource_PropertyChanged;
        }

        _compactPresentationSource = content switch
        {
            FileWidgetContentAdapter file => file.ViewModel,            _ => null
        };

        if (_compactPresentationSource is not null)
        {
            _compactPresentationSource.PropertyChanged += CompactPresentationSource_PropertyChanged;
        }
    }

    private void OnRecentQueriesChanged()
    {
        QueueCompactPresentationRefresh();
    }

    private void CompactPresentationSource_PropertyChanged(object? sender,PropertyChangedEventArgs e)
    {
        // A runtime folder remap rewrites the file view model's managed/mapped
        // identity; push it to the shell immediately instead of waiting for a
        // content switch or window recreation.
        if (sender is WidgetViewModel fileViewModel &&
            e.PropertyName is nameof(WidgetViewModel.TitleIconKind)
                or nameof(WidgetViewModel.IconGlyph))
        {
            ContentWidgetShell.TitleGlyph = fileViewModel.IconGlyph;
            ContentWidgetShell.TitleIconKind = fileViewModel.TitleIconKind;
        }

        if (!IsCompactPresentationPropertyRelevant(Config.WidgetKind,e.PropertyName))
        {
            return;
        }

        QueueCompactPresentationRefresh();
    }

    private void QueueCompactPresentationRefresh()
    {
        // The compact surface is refreshed synchronously when a collapse begins,
        // so expanded widgets do not need to keep rewriting their hidden capsule.
        if (IsClosing || !IsWidgetCollapsed || _compactPresentationRefreshQueued)
        {
            return;
        }

        _compactPresentationRefreshQueued = true;
        if (!DispatcherQueue.TryEnqueue(() =>
        {
            _compactPresentationRefreshQueued = false;
            if (!IsClosing && IsWidgetCollapsed)
            {
                RefreshCompactPresentation();
            }
        }))
        {
            _compactPresentationRefreshQueued = false;
        }
    }

    internal static bool IsCompactPresentationPropertyRelevant(
        WidgetKind widgetKind,        string? propertyName)
    {
        if (string.IsNullOrEmpty(propertyName))
        {
            return true;
        }

        return widgetKind switch
        {
            _ => true
        };
    }

    private void ApplyLocalizedTitleActionTooltips()
    {
        var localization = App.Current.LocalizationService;
        ToolTipService.SetToolTip(ContentWidgetShell.PositionLockActionButton,localization.T("Widget.LockPosition"));
        ToolTipService.SetToolTip(ContentWidgetShell.SizeLockActionButton,localization.T("Widget.LockSize"));
        ToolTipService.SetToolTip(ContentWidgetShell.AddActionButton,localization.T("Widget.Tooltip.Add"));
        ToolTipService.SetToolTip(ContentWidgetShell.MoreActionButton,localization.T("Widget.Tooltip.More"));
        ToolTipService.SetToolTip(ContentWidgetShell.CloseActionButton,localization.T("Widget.FeatureWidget.Disable"));
        ApplyTitleActionAccessibleNames();
    }

    /// <summary>
    /// Accessible names for the shell title action buttons. The lock buttons are
    /// state-aware (lock/unlock wording), so this runs on language change and
    /// whenever the lock state is reapplied via ApplyLockActionIconState.
    /// </summary>
    private void ApplyTitleActionAccessibleNames()
    {
        var localization = App.Current.LocalizationService;
        AutomationProperties.SetName(
            ContentWidgetShell.PositionLockActionButton,            localization.T(_config.IsPositionLocked ? "Widget.UnlockPosition" : "Widget.LockPosition"));
        AutomationProperties.SetName(
            ContentWidgetShell.SizeLockActionButton,            localization.T(_config.IsSizeLocked ? "Widget.UnlockSize" : "Widget.LockSize"));
        AutomationProperties.SetName(ContentWidgetShell.AddActionButton,localization.T("Widget.Tooltip.Add"));
        AutomationProperties.SetName(ContentWidgetShell.MoreActionButton,localization.T("Widget.Tooltip.More"));
        AutomationProperties.SetName(ContentWidgetShell.CloseActionButton,localization.T("Widget.FeatureWidget.Disable"));
        AutomationProperties.SetName(ContentWidgetShell.CollapseActionButton,localization.T("Widget.Shell.Collapse"));
    }

    private void SetupEventHandlers()
    {
        SettingsService.SettingsChanged += OnSettingsChanged;
        Activated += ContentWidgetWindow_Activated;
        AppWindow.Changed += OnAppWindowChanged;
        DisplayChangeWatcher = new WidgetDisplayChangeWatcher(HWnd,DispatcherQueue,RestoreBoundsAfterDisplayChange);
        ContentWidgetShell.RightTapped += ContentWidgetShell_RightTapped;
        ContentWidgetShell.TitleDoubleTapped += ContentWidgetShell_TitleDoubleTapped;
        InstallGroupFileDropFallbackHandlers();

        foreach (var child in ResizeGrid.Children.OfType<FrameworkElement>())
        {
            if (child.Tag is string tag && !string.IsNullOrWhiteSpace(tag))
            {
                child.PointerMoved += ResizeBorder_PointerMoved;
                child.PointerReleased += ResizeBorder_PointerReleased;
                child.PointerEntered += ResizeBorder_PointerEntered;
                child.PointerCaptureLost += ResizeBorder_PointerCaptureLost;
            }
        }

        Closed += (_,_) =>
        {
            IsClosing = true;
            Visible = false;
            ReleaseAutoRestoreTimer();
            try { RemoveGroupFileDropFallbackHandlers(); } catch (Exception ex) { App.Log($"[ContentWidget] Remove group drop fallback failed during close: {ex.Message}"); }
            try { RemoveNativeFileDropBridge(); } catch (Exception ex) { App.Log($"[ContentWidget] Remove native file drop bridge failed during close: {ex.Message}"); }
            App.Current.LocalizationService.LanguageChanged -= OnLanguageChanged;
            SettingsService.SettingsChanged -= OnSettingsChanged;
            AppWindow.Changed -= OnAppWindowChanged;
            ContentWidgetShell.RightTapped -= ContentWidgetShell_RightTapped;
            ContentWidgetShell.TitleDoubleTapped -= ContentWidgetShell_TitleDoubleTapped;
            if (_compactPresentationSource is not null)
            {
                _compactPresentationSource.PropertyChanged -= CompactPresentationSource_PropertyChanged;
                _compactPresentationSource = null;
            }
            try { TrayAnimation.RevealWindowForTrayShow(); } catch { }
            try { CleanupBase(); } catch (Exception ex) { App.Log($"[ContentWidget] CleanupBase failed during close: {ex.Message}"); }
            try { _contentHost.DisposeContent(); } catch (Exception ex) { App.Log($"[ContentWidget] DisposeContent failed during close: {ex.Message}"); }
            try { DisposeCachedGroupContents(); } catch (Exception ex) { App.Log($"[ContentWidget] Dispose cached group content failed during close: {ex.Message}"); }

            foreach (var child in ResizeGrid.Children.OfType<FrameworkElement>())
            {
                if (child.Tag is string tag && !string.IsNullOrWhiteSpace(tag))
                {
                    child.PointerMoved -= ResizeBorder_PointerMoved;
                    child.PointerReleased -= ResizeBorder_PointerReleased;
                    child.PointerEntered -= ResizeBorder_PointerEntered;
                    child.PointerCaptureLost -= ResizeBorder_PointerCaptureLost;
                }
            }
        };
    }

    private void OnSettingsChanged()
    {
        bool appearanceOnly =
            SettingsService.LastNotifiedChangeKind == SettingsChangeKind.Appearance;
        if (!DispatcherQueue.HasThreadAccess)
        {
            DispatcherQueue.TryEnqueue(() => ApplySettingsChanged(appearanceOnly));
            return;
        }

        ApplySettingsChanged(appearanceOnly);
    }

    private void ApplySettingsChanged(bool appearanceOnly)
    {
        if (appearanceOnly)
        {
            // The appearance preview channel already applied corner,
            // foreground, backdrop, title layout, and content appearance to
            // this window; the settings refreshed here are unchanged.
            return;
        }

        ContentWidgetShell.ShowHoverButtons = SettingsService.Settings.ShowHoverButtons;
        _titleViewModel.RefreshMetrics();
        ApplyAppearancePreview();
    }

    // ── Drag handlers (delegate to base) ───────────────────────

    private static Windows.UI.Color WithAlpha(Windows.UI.Color color,byte alpha)
    {
        return Windows.UI.Color.FromArgb(alpha,color.R,color.G,color.B);
    }

    // ── Nested: title view model ───────────────────────────────

#if DESKBOX_NATIVE_AOT
    [WinRT.GeneratedBindableCustomProperty([
        nameof(DisplayName),
        nameof(TitleIconSize),
        nameof(TitleTextSize)
    ], [])]
#endif
    private sealed partial class ContentWidgetTitleViewModel : System.ComponentModel.INotifyPropertyChanged
    {
        private readonly SettingsService _settingsService;

        public ContentWidgetTitleViewModel(WidgetConfig config,SettingsService settingsService)
        {
            Config = config;
            _settingsService = settingsService;
        }

        public event System.ComponentModel.PropertyChangedEventHandler? PropertyChanged;

        public WidgetConfig Config { get; private set; }

        public string DisplayName
        {
            get
            {
                if (Config.IsDefaultTitle)
                {
                    var localization = App.Current.LocalizationService;
                    var key = Config.WidgetKind switch
                    {
                        WidgetKind.Todo => "Todo.Title",                        WidgetKind.Tags => "Tags.Title",                        WidgetKind.SystemMonitor => "SystemMonitor.Title",                        _ => ""
                    };
                    if (!string.IsNullOrEmpty(key))
                    {
                        var localized = localization.T(key);
                        if (!string.IsNullOrEmpty(localized))
                            return localized;
                    }
                }

                return string.IsNullOrWhiteSpace(Config.Name)
                    ? Config.WidgetKind.ToString()
                    : Config.Name;
            }
        }

        public double TitleIconSize
        {
            get
            {
                double iconSize = SettingsService.NormalizeIconSize(_settingsService.Settings.IconSize);
                return Math.Clamp(Math.Round(iconSize * 0.72 * 0.56 * 0.54),11,18);
            }
        }

        public double TitleTextSize
        {
            get
            {
                double textSize = SettingsService.NormalizeTextSize(_settingsService.Settings.TextSize);
                return Math.Min(SettingsService.MaxTextSize + 2,textSize + 3);
            }
        }

        public void SetConfig(WidgetConfig config)
        {
            Config = config;
            RefreshDisplayName();
        }

        public void RefreshDisplayName()
        {
            PropertyChanged?.Invoke(this,new System.ComponentModel.PropertyChangedEventArgs(nameof(DisplayName)));
        }

        public void RefreshMetrics()
        {
            PropertyChanged?.Invoke(this,new System.ComponentModel.PropertyChangedEventArgs(nameof(TitleIconSize)));
            PropertyChanged?.Invoke(this,new System.ComponentModel.PropertyChangedEventArgs(nameof(TitleTextSize)));
        }
    }
}
