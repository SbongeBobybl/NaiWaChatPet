using DeskBox.Models;
using DeskBox.Services;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Input;
using Microsoft.UI.Xaml.Media.Animation;

namespace DeskBox.Controls;

public sealed partial class DesktopOrganizationTaskView
{
    private bool _smartClassifyRunning;
    private CancellationTokenSource? _smartCts;
    private const string LightRailBackground = "#FCF8EE";
    private const string DarkRailBackground = "#2C281E";
    private const string LightIndexBarBackground = "#FFF7E4";
    private const string DarkIndexBarBackground = "#332E24";

    private async void SmartClassify_Click(object sender,RoutedEventArgs e) =>
        await RunSmartClassifyAsync();

    /// <summary>
    /// Regroups the current scan into 6–10 fine categories and publishes the
    /// result to the visible preview immediately. The local taxonomy always
    /// runs; the model only refines names/assignments after explicit consent.
    /// </summary>
    private async Task RunSmartClassifyAsync()
    {
        if (_plan is null || _isScanning || _isExecuting || _smartClassifyRunning) return;
        var source = AiOrganizePlanner.IncludeReferenceCandidates(_plan);
        var items = source.Targets.SelectMany(target => target.Items)
            .Where(item => item.IsEligible && !_excludedSourcePaths.Contains(item.SourcePath))
            .DistinctBy(item => item.SourcePath,StringComparer.OrdinalIgnoreCase)
            .ToList();
        if (items.Count == 0)
        {
            SetSmartStatus("没有可分类的项目；请先勾选要整理的桌面内容。");
            return;
        }

        _smartClassifyRunning = true;
        SetOrganizerStep(1);
        SmartClassifyButton.IsEnabled = false;
        SmartProgress.Visibility = Visibility.Visible;
        SmartProgress.IsActive = true;
        SetSmartStatus("正在按名称细分…");
        try
        {
            var result = NaiWaSmartCategoryClassifier.Classify(items);
            string note = $"已按名称分为 {result.Categories.Count} 类 · {items.Count} 项";
            bool aiApplied = false;
            string classifierLabel = "本地分类（未勾选 AI 细化）";
            bool? modelConfigured = null;
            if (AiRefineCheckBox.IsChecked == true)
            {
                // Ask the pet whether a model key exists before claiming any AI
                // work: an unconfigured install must never look like an AI run.
                SetSmartStatus($"已分 {result.Categories.Count} 类，检查模型配置…");
                modelConfigured = await NaiWaAgentHost.ModelConfiguredAsync();
                if (modelConfigured is false)
                {
                    classifierLabel = "本地分类 · 未配置模型，未使用 AI";
                }
                else if (modelConfigured is null)
                {
                    classifierLabel = "本地分类 · 奶蛙未连接，未使用 AI";
                }
                else
                {
                    SetSmartStatus($"已分 {result.Categories.Count} 类，AI 细化中…");
                    _smartCts?.Dispose();
                    _smartCts = new CancellationTokenSource();
                    _smartCts.CancelAfter(TimeSpan.FromSeconds(120));
                    var refined = await TryCreateAiRefinementAsync(items,_smartCts.Token);
                    if (refined is not null)
                    {
                        result = refined;
                        aiApplied = true;
                        note = $"奶蛙 AI 已按语义细分为 {result.Categories.Count} 类（{items.Count} 项）";
                    }
                    else
                    {
                        classifierLabel = "本地分类 · AI 细化失败，未使用 AI";
                    }
                }
            }

            var mode = SourceBehaviorBox.SelectedIndex == 1 ? AiOrganizationMode.Move
                : SourceBehaviorBox.SelectedIndex == 2 ? AiOrganizationMode.Shortcuts
                : AiOrganizationMode.ViewOnly;
            _plan = Coordinator.CreateAiPreview(source,result.CategoryByPath,result.Categories,mode,null);
            _targetSelections.Clear();
            RenderPlan(_plan);
            SourceBehaviorBox.SelectedIndex = mode == AiOrganizationMode.Move ? 1 : mode == AiOrganizationMode.Shortcuts ? 2 : 0;
            SetSmartStatus(aiApplied ? note : note + " · " + classifierLabel);
            SetOrganizerStep(2);
            ResultInfo.Severity = InfoBarSeverity.Informational;
            ResultInfo.Title = aiApplied ? "AI 已细化分类" : "已按本地规则分类";
            ResultInfo.Message = (aiApplied ? "AI 语义分类" : "本地规则分类（未使用 AI）") +
                $" · 共 {result.Categories.Count} 类 · " +
                string.Join("、",result.Categories.Take(4).Select(category =>
                    category.DisplayName + " " + result.Counts.GetValueOrDefault(category.Id))) +
                (result.Categories.Count > 4 ? " 等" : string.Empty) +
                (modelConfigured is false ? "；在奶蛙设置填写 API Key 后可用 AI 细化" : string.Empty);
            ResultInfo.IsOpen = true;
        }
        catch (Exception ex)
        {
            App.Log($"[NaiWa] Smart classification failed: {ex}");
            SetSmartStatus("分类暂时失败：" + ex.Message);
            ResultInfo.Severity = InfoBarSeverity.Warning;
            ResultInfo.Title = "分类未完成";
            ResultInfo.Message = "原文件没有被移动；可以重试或继续使用当前预览。";
            ResultInfo.IsOpen = true;
        }
        finally
        {
            _smartClassifyRunning = false;
            SmartClassifyButton.IsEnabled = true;
            SmartProgress.IsActive = false;
            SmartProgress.Visibility = Visibility.Collapsed;
            _smartCts?.Dispose();
            _smartCts = null;
        }
    }

    private static async Task<NaiWaSmartClassification?> TryCreateAiRefinementAsync(
        IReadOnlyList<DesktopOrganizationFileSnapshot> items,CancellationToken cancellationToken)
    {
        try
        {
            if (!await NaiWaAgentHost.EnsureConnectedAsync(cancellationToken)) return null;
            var client = new AiOrganizeClient();
            var payload = new List<AiOrganizeItem>(items.Count);
            var snapshotById = new Dictionary<string,DesktopOrganizationFileSnapshot>(StringComparer.Ordinal);
            foreach (var item in items)
            {
                string id = Guid.NewGuid().ToString("N");
                snapshotById[id] = item;
                payload.Add(new AiOrganizeItem(id,item.Name,item.IsDirectory ? string.Empty : item.Extension,item.IsDirectory ? "folder" : "file"));
            }

            AiOrganizeCategory[]? categories = null;
            var categoryIdByPath = new Dictionary<string,string>(StringComparer.OrdinalIgnoreCase);
            foreach (var batch in payload.Chunk(40))
            {
                cancellationToken.ThrowIfCancellationRequested();
                var response = categories is null
                    ? await client.DiscoverCategoriesAsync(batch,cancellationToken)
                    : await client.ClassifyAsync(batch,categories,true,cancellationToken);
                if (categories is null)
                {
                    categories = response.Categories ?? [];
                    if (categories.Length is < NaiWaSmartCategoryClassifier.MinimumCategories or
                        > NaiWaSmartCategoryClassifier.MaximumCategories) return null;
                }

                string fallback = categories[^1].Id;
                foreach (var suggestion in response.Items ?? [])
                {
                    if (snapshotById.TryGetValue(suggestion.ItemId,out var snapshot))
                        categoryIdByPath[snapshot.SourcePath] = suggestion.CategoryId;
                }

                foreach (var item in batch)
                    if (snapshotById.TryGetValue(item.ItemId,out var snapshot))
                        categoryIdByPath.TryAdd(snapshot.SourcePath,fallback);
            }

            if (categories is null || categories.Length == 0 ||
                categoryIdByPath.Count != items.Count) return null;
            var counts = items
                .GroupBy(item => categoryIdByPath[item.SourcePath],StringComparer.Ordinal)
                .ToDictionary(group => group.Key,group => group.Count(),StringComparer.Ordinal);
            var ordered = categories
                .Where(category => counts.ContainsKey(category.Id))
                .OrderByDescending(category => counts[category.Id])
                .ToList();
            if (ordered.Count < NaiWaSmartCategoryClassifier.MinimumCategories) return null;
            return new NaiWaSmartClassification(ordered,categoryIdByPath,counts);
        }
        catch (OperationCanceledException) { return null; }
        catch (Exception ex) when (ex is IOException or InvalidOperationException or UnauthorizedAccessException
            or TimeoutException or System.Text.Json.JsonException)
        {
            App.Log($"[NaiWa] AI refinement unavailable: {ex.GetType().Name}: {ex.Message}");
            return null;
        }
    }

    private async void UndoAiViewsButton_Click(object sender,RoutedEventArgs e)
    {
        if (_isExecuting || _isScanning) return;
        try
        {
            int count = await Coordinator.UndoLatestReferenceViewsAsync();
            ResultInfo.Title = "视图撤销";
            ResultInfo.Message = $"已移除 {count} 个最近创建的分组视图，原文件保持原位。";
            ResultInfo.Severity = InfoBarSeverity.Informational;
            ResultInfo.IsOpen = true;
        }
        catch (Exception ex)
        {
            App.Log($"[DesktopOrganization] Reference view undo failed: {ex}");
            ResultInfo.Title = "视图撤销未完成";
            ResultInfo.Message = T("DesktopOrganization.Result.FailedBody");
            ResultInfo.Severity = InfoBarSeverity.Error;
            ResultInfo.IsOpen = true;
        }
    }

    private void MoreButton_Click(object sender,RoutedEventArgs e)
    {
        var settings = App.Current.SettingsService.Settings;
        DarkThemeMenuItem.IsChecked = settings.NaiWaWidgetTheme == "dark";
        TransparentBodyMenuItem.IsChecked = settings.NaiWaTransparentBody;
    }

    private async void NaiWaDarkTheme_Click(object sender,RoutedEventArgs e)
    {
        var service = App.Current.SettingsService;
        service.Settings.NaiWaWidgetTheme = DarkThemeMenuItem.IsChecked ? "dark" : "light";
        ApplyNaiWaAppearance();
        if (!await service.SaveCheckedAsync())
            SetSmartStatus("外观已切换，但保存失败；重启后会恢复。");
    }

    private async void NaiWaTransparency_Click(object sender,RoutedEventArgs e)
    {
        var service = App.Current.SettingsService;
        service.Settings.NaiWaTransparentBody = TransparentBodyMenuItem.IsChecked;
        ApplyNaiWaAppearance();
        if (!await service.SaveCheckedAsync())
            SetSmartStatus("透明度已切换，但保存失败；重启后会恢复。");
    }

    private async void ExitOrganizer_Click(object sender,RoutedEventArgs e)
    {
        if (_isExecuting || _isScanning)
        {
            ResultInfo.Severity = InfoBarSeverity.Warning;
            ResultInfo.Title = "暂时不能退出";
            ResultInfo.Message = "请等待当前整理或扫描完成。";
            ResultInfo.IsOpen = true;
            return;
        }

        var dialog = new ContentDialog
        {
            XamlRoot = XamlRoot, Title = "退出整理组件",
            Content = "桌面上的文件格子会一并关闭；下次点“桌面整理”会自动重新启动。",
            PrimaryButtonText = "退出", CloseButtonText = "取消",
            DefaultButton = ContentDialogButton.Close
        };
        if (await dialog.ShowAsync() == ContentDialogResult.Primary)
            App.Current.RequestApplicationExit();
    }

    /// <summary>
    /// Shows progress or an error only. An empty message hides the row again,
    /// so the resting hero carries no hint text at all.
    /// </summary>
    private void SetSmartStatus(string? text)
    {
        if (SmartClassifyStatus is null || SmartStatusRow is null) return;
        SmartClassifyStatus.Text = text ?? string.Empty;
        SmartStatusRow.Visibility = string.IsNullOrWhiteSpace(text)
            ? Visibility.Collapsed
            : Visibility.Visible;
    }

    private int _bubbleSequence;
    /// <summary>
    /// The rail no longer renders a step indicator (it duplicated the status
    /// line and the bottom bar); the call sites stay so progress reporting does
    /// not change shape.
    /// </summary>
    private void SetOrganizerStep(int step)
    {
        _ = step;
    }

    private void HeroPet_PointerEntered(object sender,PointerRoutedEventArgs e) => AnimateHeroHover(1.045);

    private void HeroPet_PointerExited(object sender,PointerRoutedEventArgs e) => AnimateHeroHover(1.0);

    private async void HeroPet_Tapped(object sender,TappedRoutedEventArgs e)
    {
        if (Resources["MascotClickStoryboard"] is Storyboard click) click.Begin();
        if (Resources["MascotBurstStoryboard"] is Storyboard burst) burst.Begin();
        HeroPetBubble.Opacity = 0;
        HeroPetBubble.Visibility = Visibility.Visible;
        if (Resources["BubbleStoryboard"] is Storyboard bubble) bubble.Begin();
        int sequence = ++_bubbleSequence;
        await Task.Delay(1600);
        if (sequence == _bubbleSequence)
        {
            HeroPetBubble.Visibility = Visibility.Collapsed;
        }
    }

    private void AnimateHeroHover(double value)
    {
        var storyboard = new Storyboard();
        foreach (string property in new[] { "ScaleX","ScaleY" })
        {
            var animation = new DoubleAnimation
            {
                To = value, Duration = new Duration(TimeSpan.FromMilliseconds(160)),
                EnableDependentAnimation = true
            };
            Storyboard.SetTarget(animation,HeroPetHoverScale);
            Storyboard.SetTargetProperty(animation,property);
            storyboard.Children.Add(animation);
        }
        storyboard.Begin();
    }

    internal void ApplyNaiWaAppearance()
    {
        var settings = App.Current.SettingsService.Settings;
        bool dark = settings.NaiWaWidgetTheme == "dark";
        RequestedTheme = dark ? ElementTheme.Dark : ElementTheme.Light;
        TaskRoot.Background = Gradient(dark ? "#221F18" : "#FFFDF7",dark ? "#2C2619" : "#FFEFC4");
        IndexRail.Background = Gradient(dark ? "#2E2A20" : "#FCF8EE",dark ? "#241F17" : "#FFF9EC");
        IndexRail.BorderBrush = Brush(dark ? "#47402F" : "#EEE7D7");
        IndexBar.Background = Gradient(dark ? "#3A3426" : "#FFFCF2",dark ? "#2F2A1F" : "#FFF1CE");
        IndexBar.BorderBrush = Brush(dark ? "#4A4331" : "#F4E6C2");
        BottomBar.Background = Gradient(dark ? "#332E23" : "#FFFFF9EA",dark ? "#252016" : "#FFF4DA");
        BottomBar.BorderBrush = Brush(dark ? "#4A4331" : "#F0E2BE");
        OrbOne.Opacity = dark ? 0.22 : 0.5;
        OrbTwo.Opacity = dark ? 0.18 : 0.42;
        PendingButton.Background = Brush(dark ? "#3C3524" : "#FFF1C9");
        PendingButton.BorderBrush = Brush(dark ? "#5A4F33" : "#F0D98C");
        PendingButton.Foreground = Brush(dark ? "#F5EAD0" : "#3A2F12");
        PendingButtonText.Foreground = Brush(dark ? "#F5EAD0" : "#3A2F12");
        PendingButtonIcon.Foreground = Brush(dark ? "#F5EAD0" : "#3A2F12");
        foreach (var card in _targetCards) card.RefreshAppearance();
        foreach (var card in _retainedCards) card.RefreshAppearance();
        foreach (var card in _completedCards) card.RefreshAppearance();
    }

    private static Microsoft.UI.Xaml.Media.LinearGradientBrush Gradient(string from,string to)
    {
        var brush = new Microsoft.UI.Xaml.Media.LinearGradientBrush
        {
            StartPoint = new Windows.Foundation.Point(0,0),
            EndPoint = new Windows.Foundation.Point(1,1)
        };
        brush.GradientStops.Add(new Microsoft.UI.Xaml.Media.GradientStop { Color = ColorOf(from),Offset = 0 });
        brush.GradientStops.Add(new Microsoft.UI.Xaml.Media.GradientStop { Color = ColorOf(to),Offset = 1 });
        return brush;
    }

    private static Microsoft.UI.Xaml.Media.SolidColorBrush Brush(string hex) => new(ColorOf(hex));

    private static Windows.UI.Color ColorOf(string hex)
    {
        string value = hex.TrimStart('#');
        byte a = 255,r = 0,g = 0,b = 0;
        if (value.Length == 8)
        {
            a = Convert.ToByte(value[..2],16);
            value = value[2..];
        }
        if (value.Length == 6)
        {
            r = Convert.ToByte(value[..2],16);
            g = Convert.ToByte(value[2..4],16);
            b = Convert.ToByte(value[4..6],16);
        }
        return Windows.UI.Color.FromArgb(a,r,g,b);
    }
}
