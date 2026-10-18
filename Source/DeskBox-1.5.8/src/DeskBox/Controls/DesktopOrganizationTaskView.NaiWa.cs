using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using DeskBox.Models;
using DeskBox.Services;
namespace DeskBox.Controls;
public sealed partial class DesktopOrganizationTaskView
{
    private bool _updatingSourceMode;
    private DesktopOrganizationPlan? _sourceModeBase;
    private void SourceBehaviorBox_SelectionChanged(object sender,SelectionChangedEventArgs e)
    {
        if (_updatingSourceMode || _plan is null || _isScanning || _isExecuting) return;
        if (_hasCompletedExecution)
        {
            // Changing how the sources are handled after a run must produce a
            // new, executable preview instead of being silently ignored.
            _hasCompletedExecution = false;
            _lastExecutionPlan = null;
            ResultInfo.IsOpen = false;
            ExecuteButton.IsEnabled = true;
        }
        var source = _sourceModeBase ?? _plan;
        var categories = source.Targets.Where(t => t.CreatesWidget)
            .Select(t => new AiOrganizeCategory(t.CategoryId,t.SuggestedDisplayName)).DistinctBy(c => c.Id).ToArray();
        if (categories.Length == 0) return;
        var assignments = source.Targets.SelectMany(t => t.Items.Select(i => (i.SourcePath, t.CategoryId)))
            .ToDictionary(x => x.SourcePath,x => x.CategoryId,StringComparer.OrdinalIgnoreCase);
        var mode = SourceBehaviorBox.SelectedIndex == 1 ? AiOrganizationMode.Move
            : SourceBehaviorBox.SelectedIndex == 2 ? AiOrganizationMode.Shortcuts : AiOrganizationMode.ViewOnly;
        try
        {
            _updatingSourceMode = true;
            _plan = Coordinator.CreateAiPreview(source,assignments,categories,mode,null);
            _targetSelections.Clear(); RenderPlan(_plan);
        }
        finally { _updatingSourceMode = false; }
    }
    public event Action<bool>? CarryVisibilityChanged;
    private void NaiWaPending_Click(object sender,RoutedEventArgs e) => SummaryTitle.IsSelected = true;
    private void NaiWaRetained_Click(object sender,RoutedEventArgs e) => ExcludedItemsButton.IsSelected = true;

    /// <summary>
    /// Reflects the persisted carry-animation level into the combo box. Called
    /// whenever a plan is rendered so a reopened workbench shows the saved
    /// choice instead of silently resetting to the first item.
    /// </summary>
    private void SyncCarryAnimationBox()
    {
        if (CarryAnimationBox is null) return;
        string level = NaiWaAnimationPolicy.Read(App.Current.SettingsService.Settings);
        int index = level == NaiWaAnimationPolicy.Simple ? 1
            : level == NaiWaAnimationPolicy.None ? 2 : 0;
        if (CarryAnimationBox.SelectedIndex != index) CarryAnimationBox.SelectedIndex = index;
    }

    private async void CarryAnimationBox_SelectionChanged(object sender,SelectionChangedEventArgs e)
    {
        if (CarryAnimationBox is null || _isExecuting) return;
        string level = CarryAnimationBox.SelectedIndex == 1 ? NaiWaAnimationPolicy.Simple
            : CarryAnimationBox.SelectedIndex == 2 ? NaiWaAnimationPolicy.None
            : NaiWaAnimationPolicy.Full;
        var service = App.Current.SettingsService;
        if (string.Equals(service.Settings.NaiWaCarryAnimation,level,StringComparison.Ordinal)) return;
        service.Settings.NaiWaCarryAnimation = level;
        try
        {
            if (!await service.SaveCheckedAsync())
                ReportCarryAnimationSaveFailure();
        }
        catch (Exception ex)
        {
            App.Log($"[NaiWa] Saving the carry animation level failed: {ex.GetType().Name}");
            ReportCarryAnimationSaveFailure();
        }
    }

    /// <summary>
    /// The status line may not exist yet while the XAML tree is still being
    /// built (the combo box raises SelectionChanged during load), so the
    /// message is guarded instead of dereferencing a null control.
    /// </summary>
    private void ReportCarryAnimationSaveFailure()
    {
        if (SmartClassifyStatus is not null)
            SetSmartStatus("动画档位已切换，但保存失败；重启后会恢复。");
        else
            App.Log("[NaiWa] Carry animation level changed but saving failed.");
    }
}
