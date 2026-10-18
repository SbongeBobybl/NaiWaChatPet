using DeskBox.Services;
using DeskBox.ViewModels;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;

namespace DeskBox.Controls.WidgetContents;

public sealed partial class FileSurfaceContent
{
    private void FileSurfaceContent_TextScaleLoaded(
        object sender,        RoutedEventArgs e)
    {
        WindowsCompatibilityService.TextScaleFactorChanged -=
            FileSurfaceContent_TextScaleFactorChanged;
        WindowsCompatibilityService.TextScaleFactorChanged +=
            FileSurfaceContent_TextScaleFactorChanged;
        ViewModel.PropertyChanged -= FileSurfaceContent_LayoutPropertyChanged;
        ViewModel.PropertyChanged += FileSurfaceContent_LayoutPropertyChanged;
        RefreshFileTextScaleFactor();
        ApplyUniformIconCellSize();
    }

    private void FileSurfaceContent_TextScaleUnloaded(
        object sender,        RoutedEventArgs e)
    {
        WindowsCompatibilityService.TextScaleFactorChanged -=
            FileSurfaceContent_TextScaleFactorChanged;
        ViewModel.PropertyChanged -= FileSurfaceContent_LayoutPropertyChanged;
    }

    private void ItemsGrid_UniformPanelLoaded(
        object sender,        RoutedEventArgs e) =>
        ApplyUniformIconCellSize();

    private void FileSurfaceContent_LayoutPropertyChanged(
        object? sender,        System.ComponentModel.PropertyChangedEventArgs e)
    {
        if (e.PropertyName is nameof(WidgetViewModel.IconCellWidth) or
            nameof(WidgetViewModel.IconCellHeight))
        {
            if (DispatcherQueue.HasThreadAccess)
            {
                ApplyUniformIconCellSize();
            }
            else
            {
                _ = DispatcherQueue.TryEnqueue(ApplyUniformIconCellSize);
            }
        }
    }

    private void FileSurfaceContent_TextScaleFactorChanged()
    {
        if (_isDisposed)
        {
            return;
        }

        if (!DispatcherQueue.HasThreadAccess)
        {
            _ = DispatcherQueue.TryEnqueue(RefreshFileTextScaleFactor);
            return;
        }

        RefreshFileTextScaleFactor();
    }

    private void RefreshFileTextScaleFactor()
    {
        if (_isDisposed)
        {
            return;
        }

        ViewModel.UpdateSystemTextScaleFactor(
            WindowsCompatibilityService.ResolveSystemTextScaleFactor());
    }

    private void ApplyUniformIconCellSize()
    {
        if (_isDisposed ||
            ItemsGrid.ItemsPanelRoot is not ItemsWrapGrid panel)
        {
            return;
        }

        panel.Orientation = Orientation.Horizontal;
        panel.ItemWidth = ViewModel.IconCellWidth;
        panel.ItemHeight = ViewModel.IconCellHeight;
    }
}
