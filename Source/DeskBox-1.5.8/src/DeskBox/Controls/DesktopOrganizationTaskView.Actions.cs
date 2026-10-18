using DeskBox.Models;
using DeskBox.Services;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace DeskBox.Controls;

public sealed partial class DesktopOrganizationTaskView
{
    public IntPtr OwnerWindowHandle { get; set; }

    private async void ExecuteButton_Click(object sender,RoutedEventArgs e)
    {
        if (!_isPreviewReady || _isScanning || _isExecuting || _hasCompletedExecution || !ExecuteButton.IsEnabled || _plan is not { EligibleItemCount: > 0 } previewPlan)
        {
            return;
        }

        DesktopOrganizationPlan plan;
        if (previewPlan.IsAiPlan && previewPlan.OrganizationMode is AiOrganizationMode.Move or AiOrganizationMode.Shortcuts)
        {
            // A previous reference/shortcut run left boxes on the desktop.
            // Remove them before the execution plan measures free space, or
            // the move fails and the old boxes stay open.
            await Coordinator.PrepareNonViewExecutionAsync(CancellationToken.None);
        }
        try
        {
            plan = Coordinator.CreateExecutionPlan(
                previewPlan,                _targetSelections.Values.ToList(),_excludedSourcePaths);
            if (plan.EligibleItemCount == 0)
            {
                ResultInfo.Severity = InfoBarSeverity.Warning;
                ResultInfo.Title = T("DesktopOrganization.Preview.NothingSelectedTitle");
                ResultInfo.Message = T("DesktopOrganization.Preview.NothingSelectedBody");
                ResultInfo.IsOpen = true;
                return;
            }
        }
        catch (Exception ex)
        {
            App.Log($"[DesktopOrganization] Failed to create execution plan: {ex}");
            ResultInfo.Severity = InfoBarSeverity.Error;
            ResultInfo.Title = T("DesktopOrganization.Result.FailedTitle");
            ResultInfo.Message = T("DesktopOrganization.Result.FailedBody");
            ResultInfo.IsOpen = true;
            return;
        }

        SetOrganizerStep(2);
        if (plan.IsAiPlan && plan.OrganizationMode == AiOrganizationMode.Shortcuts)
        {
            _isExecuting = true; ExecuteButton.IsEnabled = false;
            try
            {
                CarryVisibilityChanged?.Invoke(true);
                _executionCts?.Dispose(); _executionCts = new CancellationTokenSource();
                var result = await Coordinator.CreateShortcutViewsAsync(plan,_executionCts.Token,rememberRules: false);
                ResultInfo.Title = "快捷方式引用已创建";
                ResultInfo.Message = $"创建 {result.Created} 个引用，{result.Retained} 项因状态变化保留。原文件未移动。" +
                    (Coordinator.LastCarryAnimationSkipped
                        ? $" 未播放搬运动画：{Coordinator.LastCarrySkipReason ?? "奶蛙当前正忙"}"
                        : string.Empty);
                ResultInfo.Severity = InfoBarSeverity.Success; ResultInfo.IsOpen = true; _hasCompletedExecution = true;
            }
            catch (Exception ex)
            {
                App.Log($"[DesktopOrganization] Shortcut reference creation failed: {ex}");
                ResultInfo.Severity = InfoBarSeverity.Error;
                ResultInfo.Title = "引用创建未完成";
                ResultInfo.Message = T("DesktopOrganization.Result.FailedBody");
                ResultInfo.IsOpen = true;
            }
            finally { CarryVisibilityChanged?.Invoke(false); _isExecuting = false; UpdateSummary(_plan); }
            return;
        }
        if (plan.IsAiPlan && plan.OrganizationMode == AiOrganizationMode.ViewOnly)
        {
            _isExecuting = true; ExecuteButton.IsEnabled = false;
            try
            {
                CarryVisibilityChanged?.Invoke(true);
                _executionCts?.Dispose(); _executionCts = new CancellationTokenSource();
                int count = await Coordinator.CreateReferenceViewsAsync(plan,_executionCts.Token,rememberRules: false);
                ResultInfo.Title = "分组视图已创建";
                ResultInfo.Message = $"创建了 {count} 个原生文件格子，原文件未移动。" +
                    (Coordinator.LastCarryAnimationSkipped
                        ? $" 未播放搬运动画：{Coordinator.LastCarrySkipReason ?? "奶蛙当前正忙"}"
                        : string.Empty);
                ResultInfo.Severity = InfoBarSeverity.Success; ResultInfo.IsOpen = true;
                _hasCompletedExecution = true;
            }
            catch (Exception ex)
            {
                App.Log($"[DesktopOrganization] Reference view creation failed: {ex}");
                ResultInfo.Severity = InfoBarSeverity.Error;
                ResultInfo.Title = "视图创建未完成";
                ResultInfo.Message = T("DesktopOrganization.Result.FailedBody");
                ResultInfo.IsOpen = true;
            }
            finally { CarryVisibilityChanged?.Invoke(false); _isExecuting = false; UpdateSummary(_plan); }
            return;
        }
        if (plan.IsAiPlan)
        {
            var confirm = new ContentDialog
            {
                XamlRoot = XamlRoot, Title = "确认真实移动文件",
                Content = $"将按当前预览移动 {plan.EligibleItemCount} 个文件。仅已选项目会进入 DeskBox 的恢复日记，完成后可使用文件撤销。",
                PrimaryButtonText = "确认移动", CloseButtonText = "返回预览", DefaultButton = ContentDialogButton.Close
            };
            if (await confirm.ShowAsync() != ContentDialogResult.Primary) return;
        }
        // A writing run replaces the boxes of the previous (view-only) run,
        // so the same category never exists twice with different behaviour.
        if (plan.IsAiPlan && plan.OrganizationMode != AiOrganizationMode.ViewOnly)
        {
            await Coordinator.RemovePreviousNaiWaViewsAsync(CancellationToken.None);
            _hasCompletedExecution = false;
            _lastExecutionPlan = null;
        }

        _lastExecutionPlan = plan;
        try { CarryVisibilityChanged?.Invoke(true); await RunPlanAsync(plan); }
        finally { CarryVisibilityChanged?.Invoke(false); }
    }

    private async Task RunPlanAsync(DesktopOrganizationPlan plan)
    {
        _executionCts?.Dispose();
        var cts = new CancellationTokenSource();
        _executionCts = cts;
        _isExecuting = true;
        SelectionFeedbackInfo.IsOpen = false;
        RetainedSelectionToolbar.IsEnabled = false;
        PreviewContent.IsEnabled = false;
        MoreButton.IsEnabled = false;
        SourceSelectionHost.IsEnabled = false;
        TargetSelectionHost.IsEnabled = false;
        RetryPublicButton.IsEnabled = false;
        UndoButton.IsEnabled = false;
        AbandonUndoButton.IsEnabled = false;
        DoneButton.IsEnabled = false;
        ExcludedItemsButton.IsEnabled = false;
        RefreshButton.IsEnabled = false;
        ChangePathButton.IsEnabled = false;
        CancelButton.IsEnabled = false;
        ExecuteButton.IsEnabled = false;
        ExecutionProgressPanel.Visibility = Visibility.Visible;
        ExecutionProgressBar.Maximum = Math.Max(1,plan.EligibleItemCount);
        ExecutionProgressBar.Value = 0;
        ExecutionProgressText.Text = T("DesktopOrganization.Preview.Preparing");
        try
        {
            var progress = new Progress<DesktopOrganizationProgress>(value =>
            {
                ExecutionProgressBar.Value = value.CompletedCount;
                ExecutionProgressText.Text = Format(
                    "DesktopOrganization.Preview.Progress",                    value.CompletedCount,                    value.TotalCount,                    T(value.SourceScope == DesktopOrganizationSourceScope.Public
                        ? "DesktopOrganization.Public.SharedLabel" : "DesktopOrganization.Public.PersonalLabel") +
                    (string.IsNullOrWhiteSpace(value.TargetDisplayName) ? string.Empty : " · " + value.TargetDisplayName));
            });
            DesktopOrganizationExecutionResult result =
                await Coordinator.ExecuteAsync(plan,progress,cts.Token,OwnerWindowHandle);
            var attempted = plan.Targets.SelectMany(target => target.Items).Select(item => item.SourcePath).ToHashSet(StringComparer.OrdinalIgnoreCase);
            _runtimeRetainedItems.RemoveAll(item => attempted.Contains(item.SourcePath));
            _runtimeRetainedItems.AddRange(result.RetainedItems);
            int retainedCount = _runtimeRetainedItems.Count;
            _lastHistoryId = result.History.CanUndo
                ? result.History.Id
                : null;
            ResultInfo.Severity = retainedCount > 0
                ? InfoBarSeverity.Warning
                : InfoBarSeverity.Success;
            ResultInfo.Title = retainedCount > 0
                // ItemCount reports the transaction's cumulative total
                // (TotalItemCount survives summary compaction), not just
                // this run's receipts — a retry after a compacted first
                // run must still count the whole transaction.
                ? Format("DesktopOrganization.Layout.PartialResult",result.History.ItemCount,retainedCount)
                : T("DesktopOrganization.Result.SuccessTitle");
            ResultInfo.Message = string.Join("\n",new[]
            {
                BuildSourceResult(result.CompletedItems,DesktopOrganizationSourceScope.Personal),                BuildSourceResult(result.CompletedItems,DesktopOrganizationSourceScope.Public)
            }.Where(text => !string.IsNullOrWhiteSpace(text)));
            ResultInfo.IsOpen = true;
            ExecutionProgressPanel.Visibility = Visibility.Collapsed;
            _hasCompletedExecution = true;
            _optionalIncludedPaths.Clear();
            RenderExecutionResult(result.CompletedItems);
            RetryPublicButton.Visibility = _runtimeRetainedItems.Any(item => item.Reason != DesktopOrganizationRetentionReason.SourceChanged)
                ? Visibility.Visible : Visibility.Collapsed;
            RefreshButton.Visibility = Visibility.Visible;
            CancelButton.Visibility = Visibility.Collapsed;
            ExecuteButton.Visibility = Visibility.Collapsed;
            UndoButton.Visibility = result.History.CanUndo
                ? Visibility.Visible
                : Visibility.Collapsed;
            DoneButton.Visibility = Visibility.Visible;
            DoneButton.Style = RetryPublicButton.Visibility == Visibility.Visible
                ? null : (Style)Application.Current.Resources["AccentButtonStyle"];
            OrganizationCompleted?.Invoke(this,EventArgs.Empty);
        }
        catch (OperationCanceledException)
        {
            if (!_closeAfterExecutionStops)
            {
                ResultInfo.Severity = InfoBarSeverity.Warning;
                ResultInfo.Title = T("DesktopOrganization.Result.FailedTitle");
                ResultInfo.Message = string.Empty;
                ResultInfo.IsOpen = true;
                ExecuteButton.IsEnabled = true;
            }
        }
        catch (DesktopOrganizationInsufficientSpaceException ex)
        {
            App.Log($"[DesktopOrganization] Execution failed: {ex}");
            ResultInfo.Severity = InfoBarSeverity.Error;
            ResultInfo.Title = T("DesktopOrganization.Result.FailedTitle");
            ResultInfo.Message = Format("DesktopOrganization.Error.NoSpace",ex.DriveName);
            ResultInfo.IsOpen = true;
            ExecuteButton.IsEnabled = true;
        }
        catch (DesktopOrganizationPendingRecoveryException ex)
        {
            App.Log($"[DesktopOrganization] Execution failed: {ex}");
            ResultInfo.Severity = InfoBarSeverity.Error;
            ResultInfo.Title = T("DesktopOrganization.Result.FailedTitle");
            ResultInfo.Message = T("DesktopOrganization.Error.PendingRecovery");
            ResultInfo.IsOpen = true;
            ExecuteButton.IsEnabled = true;
        }
        catch (DesktopOrganizationInvalidStorageRootException ex)
        {
            App.Log($"[DesktopOrganization] Execution failed: {ex}");
            ResultInfo.Severity = InfoBarSeverity.Error;
            ResultInfo.Title = T("DesktopOrganization.Result.FailedTitle");
            ResultInfo.Message = T("DesktopOrganization.Error.StorageRootOnDesktop");
            ResultInfo.IsOpen = true;
            ExecuteButton.IsEnabled = true;
        }
        catch (Exception ex)
        {
            App.Log($"[DesktopOrganization] Execution failed: {ex}");
            ResultInfo.Severity = InfoBarSeverity.Error;
            ResultInfo.Title = T("DesktopOrganization.Result.FailedTitle");
            ResultInfo.Message = T("DesktopOrganization.Result.FailedBody");
            ResultInfo.IsOpen = true;
            ExecuteButton.IsEnabled = true;
        }
        finally
        {
            _isExecuting = false;
            RetainedSelectionToolbar.IsEnabled = true;
            PreviewContent.IsEnabled = true;
            MoreButton.IsEnabled = true;
            RefreshButton.IsEnabled = true;
            ChangePathButton.IsEnabled = !_hasCompletedExecution;
            SourceSelectionHost.IsEnabled = !_hasCompletedExecution;
            TargetSelectionHost.IsEnabled = !_hasCompletedExecution;
            RetryPublicButton.IsEnabled = true;
            UndoButton.IsEnabled = true;
            AbandonUndoButton.IsEnabled = true;
            DoneButton.IsEnabled = true;
            ExcludedItemsButton.IsEnabled = true;
            CancelButton.IsEnabled = true;
            UpdateRecoveryState();
            UpdateSummary(_plan);
            if (_closeAfterExecutionStops)
            {
                _closeAfterExecutionStops = false;
                CloseRequested?.Invoke(this,EventArgs.Empty);
            }
        }
    }

    private async void UndoButton_Click(object sender,RoutedEventArgs e)
    {
        if (_isExecuting || string.IsNullOrWhiteSpace(_lastHistoryId))
        {
            return;
        }

        _isExecuting = true;
        UndoButton.IsEnabled = false;
        AbandonUndoButton.IsEnabled = false;
        SourceSelectionHost.IsEnabled = false;
        TargetSelectionHost.IsEnabled = false;
        ExecuteButton.IsEnabled = false;
        ChangePathButton.IsEnabled = false;
        DoneButton.IsEnabled = false;
        RetryPublicButton.IsEnabled = false;
        RefreshButton.IsEnabled = false;
        try
        {
            bool allowPublicRestore = false;
            var historyForUndo = App.Current.SettingsService.OrganizationHistory.Entries.FirstOrDefault(h => h.Id == _lastHistoryId);
            var publicItems = historyForUndo?.Items.Where(i => !i.IsRestored && i.SourceScope == DesktopOrganizationSourceScope.Public).ToList() ?? [];
            if (publicItems.Count > 0)
            {
                var consent = new ContentDialog { XamlRoot = XamlRoot, RequestedTheme = ElementTheme.Light,
                    Title = "确认恢复原公用桌面文件",
                    Content = $"有 {publicItems.Count} 项的历史原始位置是公用桌面。只有选择原路恢复才会写入公用桌面。个人桌面文件始终恢复到个人原目录。\n" + string.Join("\n", publicItems.Select(i => i.Name)),
                    PrimaryButtonText = "确认这几项原路恢复", CloseButtonText = "仅恢复个人桌面", DefaultButton = ContentDialogButton.Close };
                allowPublicRestore = await consent.ShowAsync() == ContentDialogResult.Primary;
            }
            await Coordinator.UndoAsync(_lastHistoryId,OwnerWindowHandle,allowPublicRestore);
            ResultInfo.Severity = InfoBarSeverity.Success;
            ResultInfo.Title = T("DesktopOrganization.Undo.Success");
            ResultInfo.Message = string.Empty;
            ResultInfo.IsOpen = true;
            OrganizationUndone?.Invoke(this,EventArgs.Empty);
            RefreshButton.Visibility = Visibility.Visible;
            CancelButton.Visibility = Visibility.Visible;
            ExecuteButton.Visibility = Visibility.Visible;
            UndoButton.Visibility = Visibility.Collapsed;
            DoneButton.Visibility = Visibility.Collapsed;
            await ScanAsync();
        }
        catch (DesktopOrganizationIncompleteUndoException ex)
        {
            ResultInfo.Severity = InfoBarSeverity.Warning;
            ResultInfo.Title = T("DesktopOrganization.Public.UndoPendingTitle");
            ResultInfo.Message = Format("DesktopOrganization.Public.UndoPending",ex.RestoredCount,ex.RemainingCount);
            ResultInfo.IsOpen = true;
            // An undo attempt ends forward retries. Only the remaining undo
            // items can be continued, using their persisted receipts.
            RetryPublicButton.Visibility = Visibility.Collapsed;
            UndoButton.Content = T("DesktopOrganization.Public.ContinueUndo");
            AbandonUndoButton.Visibility = Visibility.Visible;
            var history = App.Current.SettingsService.OrganizationHistory.Entries
                .FirstOrDefault(entry => entry.Id == _lastHistoryId);
            if (_hasCompletedExecution && history is not null) RenderExecutionResult(history.Items);
        }
        catch (Exception ex)
        {
            App.Log($"[DesktopOrganization] Undo failed: {ex}");
            ResultInfo.Severity = InfoBarSeverity.Error;
            ResultInfo.Title = T("DesktopOrganization.Undo.Failed");
            ResultInfo.Message = T("DesktopOrganization.Undo.FailedBody");
            ResultInfo.IsOpen = true;
        }
        finally
        {
            _isExecuting = false;
            UndoButton.IsEnabled = true;
            AbandonUndoButton.IsEnabled = true;
            DoneButton.IsEnabled = true;
            RetryPublicButton.IsEnabled = true;
            RefreshButton.IsEnabled = true;
            SourceSelectionHost.IsEnabled = !_hasCompletedExecution;
            TargetSelectionHost.IsEnabled = !_hasCompletedExecution;
            ChangePathButton.IsEnabled = !_hasCompletedExecution;
            // An interrupted undo can leave a recovery journal behind; refresh
            // the cached hint so the banner and execute button reflect it.
            UpdateRecoveryState();
            UpdateSummary(_plan);
        }
    }

    private async void AbandonUndoButton_Click(object sender,RoutedEventArgs e)
    {
        if (_isExecuting || string.IsNullOrWhiteSpace(_lastHistoryId))
        {
            return;
        }

        var history = App.Current.SettingsService.OrganizationHistory.Entries
            .FirstOrDefault(entry => entry.Id == _lastHistoryId);
        if (history is null || !history.CanUndo)
        {
            // The entry was resolved elsewhere; resync the banner state.
            await ScanAsync();
            return;
        }

        if (!await DesktopOrganizationAbandonDialog.ConfirmAsync(
                XamlRoot,                App.Current.LocalizationService,                DesktopOrganizationAbandonDialog.BuildItemDetails(App.Current.LocalizationService,history)))
        {
            return;
        }

        _isExecuting = true;
        UndoButton.IsEnabled = false;
        AbandonUndoButton.IsEnabled = false;
        try
        {
            await Coordinator.AbandonUndoAsync(history.Id);
        }
        catch (Exception ex)
        {
            App.Log($"[DesktopOrganization] Abandoning restore failed: {ex}");
            ResultInfo.Severity = InfoBarSeverity.Error;
            ResultInfo.Title = T("DesktopOrganization.Result.FailedTitle");
            ResultInfo.Message = T("DesktopOrganization.Result.FailedBody");
            ResultInfo.IsOpen = true;
            return;
        }
        finally
        {
            _isExecuting = false;
            UndoButton.IsEnabled = true;
            AbandonUndoButton.IsEnabled = true;
        }

        UndoButton.Visibility = Visibility.Collapsed;
        UndoButton.Content = T("DesktopOrganization.Layout.Undo");
        AbandonUndoButton.Visibility = Visibility.Collapsed;
        _lastHistoryId = null;
        ResultInfo.Severity = InfoBarSeverity.Success;
        ResultInfo.Title = T("DesktopOrganization.Public.AbandonDone");
        ResultInfo.Message = string.Empty;
        ResultInfo.IsOpen = true;
        // The desktop itself did not change; refresh the gates without a
        // rescan, which would wipe this confirmation message.
        UpdateRecoveryState();
        UpdateSummary(_plan);
    }

    private async void AbandonRecoveryButton_Click(object sender,RoutedEventArgs e)
    {
        if (_isExecuting || !HasPendingRecovery)
        {
            return;
        }

        if (!await DesktopOrganizationAbandonDialog.ConfirmAsync(
                XamlRoot,                App.Current.LocalizationService,                itemDetails: null))
        {
            return;
        }

        _isExecuting = true;
        RecoverButton.IsEnabled = false;
        AbandonRecoveryButton.IsEnabled = false;
        try
        {
            await Coordinator.AbandonPendingRecoveryAsync();
            ResultInfo.Severity = InfoBarSeverity.Success;
            ResultInfo.Title = T("DesktopOrganization.Public.AbandonDone");
            ResultInfo.Message = string.Empty;
            ResultInfo.IsOpen = true;
        }
        catch (Exception ex)
        {
            App.Log($"[DesktopOrganization] Abandoning pending recovery failed: {ex}");
            ResultInfo.Severity = InfoBarSeverity.Error;
            ResultInfo.Title = T("DesktopOrganization.Result.FailedTitle");
            ResultInfo.Message = T("DesktopOrganization.Result.FailedBody");
            ResultInfo.IsOpen = true;
        }
        finally
        {
            _isExecuting = false;
            RecoverButton.IsEnabled = true;
            AbandonRecoveryButton.IsEnabled = true;
            UpdateRecoveryState();
            UpdateSummary(_plan);
        }
    }

    private async void ChangePathButton_Click(object sender,RoutedEventArgs e)
    {
        LocationsFlyout.Hide();
        if (_isExecuting || _isScanning) return;
        try
        {
            string? folder = await FolderPickerService.PickFolderAsync(OwnerWindowHandle);
            if (string.IsNullOrWhiteSpace(folder)) return;
            string normalized = SettingsService.NormalizeManagedStorageRootPath(folder);
            string desktop = Environment.GetFolderPath(Environment.SpecialFolder.DesktopDirectory);
            if (string.Equals(normalized,desktop,StringComparison.OrdinalIgnoreCase) ||
                normalized.StartsWith(desktop + Path.DirectorySeparatorChar,StringComparison.OrdinalIgnoreCase))
            {
                ResultInfo.Severity = InfoBarSeverity.Warning;
                ResultInfo.Title = "收纳位置不可用";
                ResultInfo.Message = T("DesktopOrganization.Error.StorageRootOnDesktop");
                ResultInfo.IsOpen = true;
                return;
            }

            App.Current.SettingsService.Settings.DefaultManagedStorageRootPath = normalized;
            if (!await App.Current.SettingsService.SaveCheckedAsync())
            {
                ResultInfo.Severity = InfoBarSeverity.Warning;
                ResultInfo.Title = "收纳位置未保存";
                ResultInfo.Message = "请重试；原文件没有被移动。";
                ResultInfo.IsOpen = true;
                return;
            }

            await ScanAsync(preserveSelection: false);
        }
        catch (Exception ex)
        {
            App.Log($"[DesktopOrganization] Changing the storage root failed: {ex}");
            ResultInfo.Severity = InfoBarSeverity.Error;
            ResultInfo.Title = T("DesktopOrganization.Result.FailedTitle");
            ResultInfo.Message = T("DesktopOrganization.Result.FailedBody");
            ResultInfo.IsOpen = true;
        }
    }

    private async void RefreshButton_Click(object sender,RoutedEventArgs e)
    {
        if (!_isExecuting && !_isScanning) await ScanAsync(preserveSelection: true);
    }

    private void ResetSelectionMenuItem_Click(object sender,RoutedEventArgs e)
    {
        if (_isExecuting || _isScanning || _hasCompletedExecution || _basePlan is null || _plan is null) return;
        try
        {
            var plan = Coordinator.CreatePreviewPlanWithOptionalItems(_basePlan,[],                _plan.IncludePersonalDesktop,_plan.IncludePublicDesktop);
            _excludedSourcePaths.Clear();
            _optionalIncludedPaths.Clear();
            _retainedSelection.Clear();
            _targetSelections.Clear();
            _newPaths.Clear();
            _plan = plan;
            RenderPlan(plan);
            ShowSelectionFeedback(T("DesktopOrganization.Layout.ResetDone"),showPendingLink: false);
        }
        catch (Exception ex)
        {
            App.Log($"[DesktopOrganization] Reset preview failed: {ex}");
            ResultInfo.Severity = InfoBarSeverity.Error;
            ResultInfo.Title = T("DesktopOrganization.Result.FailedTitle");
            ResultInfo.Message = T("DesktopOrganization.Result.FailedBody");
            ResultInfo.IsOpen = true;
        }
    }

    private void CancelButton_Click(object sender,RoutedEventArgs e) =>
        CloseRequested?.Invoke(this,EventArgs.Empty);

    private void DoneButton_Click(object sender,RoutedEventArgs e) =>
        CloseRequested?.Invoke(this,EventArgs.Empty);
}
