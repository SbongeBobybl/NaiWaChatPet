namespace DeskBox.Tests;

public sealed class QuickCaptureMaterialRefreshContractTests
{
    [Fact]
    public void UnifiedSurface_RefreshesMaterialForRecyclingDataAndThemeChanges()
    {
        string root = FindRepositoryRoot();
        string xaml = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Controls/WidgetContents/QuickCaptureSurfaceContent.xaml"));
        string code = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Controls/WidgetContents/QuickCaptureSurfaceContent.xaml.cs"));

        Assert.Contains(
            "DataContextChanged=\"QuickCaptureItem_DataContextChanged\"",
            xaml,
            StringComparison.Ordinal);
        Assert.Contains(
            "ActualThemeChanged += QuickCaptureSurfaceContent_ActualThemeChanged",
            code,
            StringComparison.Ordinal);
        Assert.Contains(
            "DispatcherQueue.TryEnqueue(RefreshItemMaterialSurfaces)",
            code,
            StringComparison.Ordinal);
        Assert.Contains(
            "QuickCaptureAppearancePolicy.ResolveListPreset",
            code,
            StringComparison.Ordinal);
    }

    private static string FindRepositoryRoot()
    {
        DirectoryInfo? current = new(AppContext.BaseDirectory);
        while (current is not null)
        {
            if (Directory.Exists(Path.Combine(current.FullName, "src", "DeskBox")))
            {
                return current.FullName;
            }

            current = current.Parent;
        }

        throw new DirectoryNotFoundException("Could not locate repository root.");
    }
}
