// Copyright (c) DeskBox. All rights reserved.

using DeskBox.Models;
using DeskBox.Helpers;
using DeskBox.Controls.WidgetContents;
using DeskBox.Platform;
using DeskBox.ViewModels;
using DeskBox.Views;
using Microsoft.UI.Dispatching;
using Microsoft.UI.Windowing;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Media;

namespace DeskBox.Services;

public sealed record GlanceWidgetInstanceInfo(
    string Id,    string Name,    bool IsEnabled);

/// <summary>
/// Partial class containing FeatureWidgets logic for WidgetManager.
/// </summary>
public sealed partial class WidgetManager
{

    private readonly Dictionary<WidgetKind, bool> _lastFeatureWidgetEnabledStates = new();
    private readonly Dictionary<WidgetKind, SemaphoreSlim> _featureWidgetUpdateLocks =
        FeatureWidgetSettings.FeatureKinds.ToDictionary(
            kind => kind,            _ => new SemaphoreSlim(1,1));
    private readonly Dictionary<WidgetKind, FeatureWidgetHandler> _featureWidgetHandlers;
    private readonly Dictionary<WidgetKind, WidgetWindowProvider> _windowProviders;
    private bool _isApplyingAppearancePreview;

    private void ApplyFeatureWidgetEnabledState(WidgetKind kind,bool enabled)
    {
        if (!UiDispatch.HasAccess)
        {
            UiDispatch.RunOrDefer(() => ApplyFeatureWidgetEnabledState(kind,enabled));
            return;
        }

        if (!enabled)
        {
            if (_featureWidgetHandlers.TryGetValue(kind,out var handler))
            {
                handler.HideLoaded();
            }
            else
            {
                HideAndCloseFeatureWidgetAsync(kind);
            }

            return;
        }

        CreateOrShowFeatureWidgetAsync(kind).ContinueWith(
            task =>
            {
                if (task.Exception is not null)
                {
                    App.Log($"[WidgetManager] Failed to show feature widget after enabling kind={kind}: {task.Exception}");
                }
            },            TaskContinuationOptions.OnlyOnFaulted);
    }

    private string GetDefaultFeatureWidgetTitle(WidgetKind kind,WidgetContentDescriptor descriptor)
    {
        string key = kind switch
        {
            WidgetKind.Tags => "Tags.Title",            WidgetKind.SystemMonitor => "SystemMonitor.Title",            _ => string.Empty
        };

        if (!string.IsNullOrWhiteSpace(key))
        {
            string localized = _localizationService.T(key);
            if (!string.IsNullOrWhiteSpace(localized))
            {
                return localized;
            }
        }

        return descriptor.DefaultTitle;
    }

    private async Task<ContentWidgetWindow> CreateSingletonContentFeatureWidgetAsync(WidgetKind kind)
    {
        if (!IsContentFeatureWidgetKind(kind))
        {
            throw new NotSupportedException($"Widget kind '{kind}' is not a content feature widget.");
        }

        await CommitFeatureWidgetStateAsync(kind,true);

        var existingConfig = _settingsService.Settings.Widgets
            .FirstOrDefault(w => w.WidgetKind == kind && !IsDeleted(w.Id));
        if (existingConfig is not null)
        {
            await ShowWidgetAsync(existingConfig.Id,reveal: true,autoRestoreOnReveal: false);
            if (_contentWidgets.TryGetValue(existingConfig.Id,out var existing))
            {
                return existing;
            }
        }

        var descriptor = new WidgetContentFactory(_localizationService).GetDescriptor(kind);
        var config = new WidgetConfig
        {
            Name = GetDefaultFeatureWidgetTitle(kind,descriptor),
            WidgetKind = kind,
            BoundsCoordinateVersion = WidgetConfig.CurrentBoundsCoordinateVersion,
            Width = kind switch
            {
_ => Math.Max(_settingsService.Settings.DefaultWidgetWidth,320)
            },
            Height = kind switch
            {
_ => Math.Max(_settingsService.Settings.DefaultWidgetHeight,360)
            }
        };
        ApplyDefaultFeatureWidgetChromeMode(config,kind);

        ApplyNewWidgetPlacement(config);
        _settingsService.Settings.Widgets.Add(config);
        await _settingsService.SaveAsync();

        return await CreateContentWidgetFromConfigAsync(config,revealAfterCreate: true);
    }

    private bool TryGetFileWidgetFolderPath(WidgetConfig widget,out string folderPath)
    {
        folderPath = string.Empty;
        if (widget.WidgetKind != WidgetKind.File)
        {
            return false;
        }

        if (!string.IsNullOrWhiteSpace(widget.MappedFolderPath))
        {
            folderPath = Path.GetFullPath(widget.MappedFolderPath);
            return true;
        }

        if (!widget.FollowsDefaultStoragePath || string.IsNullOrWhiteSpace(widget.ManagedFolderName))
        {
            return false;
        }

        folderPath = Path.Combine(GetManagedStorageRootPath(),widget.ManagedFolderName);
        return true;
    }

    private void DeduplicateFeatureWidgets()
    {
        var seen = new HashSet<WidgetKind>();
        var toRemove = new List<string>();

        foreach (var config in _settingsService.Settings.Widgets.ToList())
        {
            if (!RequiresSingletonFeatureWidgetConfig(config.WidgetKind)) continue;
            if (IsDeleted(config.Id)) continue;

            if (!seen.Add(config.WidgetKind))
            {
                toRemove.Add(config.Id);
                App.Log($"[WidgetManager] Dedup: removing duplicate {config.WidgetKind} widget {config.Id}");
            }
        }

        if (toRemove.Count > 0)
        {
            foreach (var id in toRemove)
            {
                _settingsService.Settings.Widgets.RemoveAll(w => w.Id == id);
                _settingsService.Settings.DeletedWidgetIds.Add(id);
            }
            _settingsService.SaveDebounced();
        }
    }

    internal static bool RequiresSingletonFeatureWidgetConfig(WidgetKind kind)
    {
        return FeatureWidgetSettings.IsFeatureWidget(kind) &&
               kind != WidgetKind.Glance;
    }

    internal IDesktopWidgetWindow? GetFeatureWidget(WidgetKind kind)
    {
        return _contentWidgets.Values
            .FirstOrDefault(w => w.Config.WidgetKind == kind);
    }

    internal bool IsFeatureWidgetEnabled(WidgetKind kind)
    {
        return FeatureWidgetSettings.IsFeatureWidget(kind)
            ? GetFeatureWidgetEnabledState(kind)
            : GetFeatureWidget(kind)?.Visible == true;
    }

    internal async Task<IDesktopWidgetWindow?> CreateOrShowFeatureWidgetAsync(WidgetKind kind)
    {
        if (!UiDispatch.HasXamlApp)
        {
            // Test hosts: no XAML Application exists, so window creation
            // cannot succeed. Skip instead of attempting it on the inline
            // fiction thread (which produced swallowed RPC_E_WRONG_THREAD
            // noise in the user log).
            App.LogVerbose($"[WidgetManager] CreateOrShowFeatureWidget skipped (no XAML app): kind={kind}");
            return null;
        }

        if (!HasUiThreadAccess())
        {
            return await RunOnUiThreadAsync(() => CreateOrShowFeatureWidgetAsync(kind));
        }

        if (_featureWidgetHandlers.TryGetValue(kind,out var handler))
        {
            return await handler.CreateOrShowAsync(true);
        }

        App.Log($"[WidgetManager] CreateOrShowFeatureWidget: unsupported kind={kind}");
        return null;
    }

    public async Task SetFeatureWidgetEnabledAsync(WidgetKind kind,bool enabled,bool reveal = true)
    {
        if (!HasUiThreadAccess())
        {
            await RunOnUiThreadAsync(() => SetFeatureWidgetEnabledAsync(kind,enabled,reveal));
            return;
        }

        if (_featureWidgetHandlers.TryGetValue(kind,out var handler) &&
            _featureWidgetUpdateLocks.TryGetValue(kind,out var updateLock))
        {
            await updateLock.WaitAsync();
            try
            {
                await handler.SetEnabledAsync(enabled,reveal);
            }
            finally
            {
                updateLock.Release();
            }

            return;
        }

        App.Log($"[WidgetManager] SetFeatureWidgetEnabled: unsupported kind={kind}");
    }

    public async Task ResetFeatureWidgetAsync(WidgetKind kind)
    {
        if (!HasUiThreadAccess())
        {
            await RunOnUiThreadAsync(() => ResetFeatureWidgetAsync(kind));
            return;
        }

        if (!FeatureWidgetSettings.IsFeatureWidget(kind))
        {
            App.Log($"[WidgetManager] ResetFeatureWidget: unsupported kind={kind}");
            return;
        }

        await DetachFeatureWidgetsFromGroupsAsync(kind);
        var suppressedClosedIds = _settingsService.Settings.Widgets
            .Where(widget => widget.WidgetKind == kind)
            .Select(widget => widget.Id)
            .ToList();
        foreach (string id in suppressedClosedIds)
        {
            _suppressClosedVisibilityPersistence.Add(id);
        }

        try
        {
            CloseLoadedFeatureWidgetWindows(kind);

            var configs = _settingsService.Settings.Widgets
                .Where(widget => widget.WidgetKind == kind)
                .ToList();
            foreach (WidgetConfig existingConfig in configs)
            {
                ClearWidgetGroupTransientState(existingConfig.Id);
            }


            await CommitFeatureWidgetStateAsync(kind,false);
            var config = configs.FirstOrDefault(widget => !IsDeleted(widget.Id)) ??
                         configs.FirstOrDefault();

            foreach (var duplicate in configs.Where(widget => !ReferenceEquals(widget,config)).ToList())
            {
                _settingsService.Settings.Widgets.Remove(duplicate);
                if (!_settingsService.Settings.DeletedWidgetIds.Contains(duplicate.Id))
                {
                    _settingsService.Settings.DeletedWidgetIds.Add(duplicate.Id);
                }

                _deletedWidgetIds.Remove(duplicate.Id);
                App.Log($"[WidgetManager] ResetFeatureWidget removed duplicate kind={kind} id={duplicate.Id}");
            }

            if (config is null)
            {
                config = CreateDefaultFeatureWidgetConfig(kind,isEnabled: false);
                ApplyNewWidgetPlacement(config);
                _settingsService.Settings.Widgets.Add(config);
            }
            else
            {
                ResetFeatureWidgetConfig(config,kind,isEnabled: false);
            }

            _settingsService.Settings.DeletedWidgetIds.RemoveAll(id =>
                string.Equals(id,config.Id,StringComparison.Ordinal));
            _deletedWidgetIds.Remove(config.Id);

            _ = config;

            await _settingsService.SaveAsync();
            App.Log($"[WidgetManager] ResetFeatureWidget kind={kind} enabled=false id={config.Id}");
        }
        finally
        {
            foreach (string id in suppressedClosedIds)
            {
                _suppressClosedVisibilityPersistence.Remove(id);
            }
        }
    }

    private WidgetConfig CreateDefaultFeatureWidgetConfig(WidgetKind kind,bool isEnabled)
    {
        var config = new WidgetConfig();
        ResetFeatureWidgetConfig(config,kind,isEnabled);
        return config;
    }

    private void ResetFeatureWidgetConfig(WidgetConfig config,WidgetKind kind,bool isEnabled)
    {
        var descriptor = new WidgetContentFactory(_localizationService).GetDescriptor(kind);
        config.WidgetKind = kind;
        config.Name = GetDefaultFeatureWidgetTitle(kind,descriptor);
        config.IsDefaultTitle = true;
        config.X = 100;
        config.Y = 100;
        config.PositionAnchor = null;
        config.PositionMarginX = 0;
        config.PositionMarginY = 0;
        config.PositionMonitorKey = null;
        config.PositionMonitorDeviceName = null;
        config.PositionMonitorStableId = null;
        config.PositionMonitorWasPrimary = null;
        config.ScreenBindingMode = WidgetScreenBindingMode.Unbound;
        config.BoundScreenId = null;
        config.BoundsCoordinateVersion = WidgetConfig.CurrentBoundsCoordinateVersion;
        (config.Width, config.Height) = GetDefaultFeatureWidgetSize(kind);
        config.ViewMode = ViewMode.Icon;
        config.IsVisible = isEnabled;
        config.IsDisabled = false;
        config.IsPositionLocked = false;
        config.IsSizeLocked = false;
        config.Metadata ??= [];
        config.Metadata.Clear();
        ApplyDefaultFeatureWidgetChromeMode(config,kind);
        config.MappedFolderPath = null;
        config.FollowsDefaultStoragePath = false;
        config.ManagedFolderName = null;
        config.SortMode = WidgetSortMode.Name;
        config.SortDescending = false;
        config.Items ??= [];
        config.Items.Clear();
    }

    internal static void ApplyDefaultFeatureWidgetChromeMode(WidgetConfig config,WidgetKind kind)
    {
    }

    private void CloseLoadedFeatureWidgetWindows(WidgetKind kind)
    {
        foreach (var window in _contentWidgets.Values
                     .Where(window => window.Config.WidgetKind == kind)
                     .ToList())
        {
            CloseFeatureWidgetInstance(window);
        }
    }

    private async Task SetContentFeatureWidgetEnabledAsync(WidgetKind kind,bool enabled,bool reveal = true)
    {
        if (enabled)
        {
            await CommitFeatureWidgetStateAsync(kind,true);
            if (reveal)
            {
                await CreateSingletonContentFeatureWidgetAsync(kind);
            }
            else
            {
                var config = _settingsService.Settings.Widgets
                    .FirstOrDefault(w => w.WidgetKind == kind && !IsDeleted(w.Id));
                if (config is not null)
                {
                    config.IsDisabled = false;
                    config.IsVisible = true;
                }

                await _settingsService.SaveAsync();
            }

            return;
        }

        await DetachFeatureWidgetsFromGroupsAsync(kind);
        foreach (var config in _settingsService.Settings.Widgets.Where(widget =>
                     widget.WidgetKind == kind &&
                     !IsDeleted(widget.Id)))
        {
            config.IsVisible = false;
            config.IsDisabled = false;
        }

        HideAndCloseFeatureWidgetAsync(kind);
        // Close the content window while feature-owned services are still
        // available. Search content, for example, must unsubscribe from the
        // exact SearchHistoryService instance before that service is released.
        await CommitFeatureWidgetStateAsync(kind,false);
        await _settingsService.SaveAsync();
    }

    private async Task DetachFeatureWidgetsFromGroupsAsync(WidgetKind kind)
    {
        List<string> groupedIds = _settingsService.Settings.Widgets
            .Where(widget =>
                widget.WidgetKind == kind &&
                WidgetGroupSettings.FindByMember(_settingsService.Settings,widget.Id) is not null)
            .Select(widget => widget.Id)
            .ToList();

        foreach (string widgetId in groupedIds)
        {
            await RemoveWidgetFromGroupAsync(widgetId,revealStandalone: false);
        }
    }

    private Task SetWeatherFeatureWidgetEnabledAsync(bool enabled,bool reveal)
    {
        return SetContentFeatureWidgetEnabledAsync(WidgetKind.Weather,enabled,reveal);
    }

    private Task SetSearchFeatureWidgetEnabledAsync(bool enabled,bool reveal)
    {
        return SetContentFeatureWidgetEnabledAsync(WidgetKind.Search,enabled,reveal);
    }

    private bool GetFeatureWidgetEnabledState(WidgetKind? kind)
    {
        return kind is { } featureKind &&
               FeatureWidgetSettings.IsFeatureWidget(featureKind) &&
               FeatureWidgetSettings.IsEnabled(_settingsService.Settings,featureKind);
    }

    private static bool IsContentFeatureWidgetKind(WidgetKind kind)
    {
        return FeatureWidgetSettings.IsFeatureWidget(kind);
    }

    private async Task CommitFeatureWidgetStateAsync(WidgetKind kind,bool enabled)
    {
        SetFeatureWidgetEnabledState(kind,enabled);
    }

    private void SetFeatureWidgetEnabledState(WidgetKind kind,bool enabled)
    {
        _lastFeatureWidgetEnabledStates[kind] = enabled;
        FeatureWidgetSettings.SetEnabled(_settingsService.Settings,kind,enabled);
    }

    public void HideAndCloseFeatureWidgetAsync(WidgetKind kind)
    {
        var existing = GetFeatureWidget(kind);
        if (existing is not null)
        {
            CloseFeatureWidgetInstance(existing);
        }
    }

    private void CloseFeatureWidgetInstance(IDesktopWidgetWindow window)
    {
        if (!HasUiThreadAccess())
        {
            _ = RunOnUiThreadAsync(() =>
            {
                CloseFeatureWidgetInstance(window);
                return Task.CompletedTask;
            });
            return;
        }

        window.Config.IsVisible = false;

        RemoveFileWidgetSessionsForHost(window);

        if (window is ContentWidgetWindow contentWindow)
            _contentWindowRegistration.Unregister(contentWindow);

        try
        {
            window.CloseWindow();
        }
        catch
        {
        }

        _settingsService.SaveDebounced();
        if (FeatureWidgetSettings.IsFeatureWidget(window.Config.WidgetKind))
        {
            App.ScheduleLightMemoryCleanup(completedHeavyOperation: true);
        }
    }

}
