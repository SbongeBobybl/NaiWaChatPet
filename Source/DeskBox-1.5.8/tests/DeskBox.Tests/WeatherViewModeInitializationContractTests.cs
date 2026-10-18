namespace DeskBox.Tests;

public sealed class WeatherViewModeInitializationContractTests
{
    [Fact]
    public void WeatherSegmented_DoesNotPersistItsTemplateDefaultAsUserIntent()
    {
        string root = FindRepositoryRoot();
        string xaml = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Controls/WidgetContents/WeatherWidgetContent.xaml"));
        string code = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Controls/WidgetContents/WeatherWidgetContent.xaml.cs"));

        Assert.DoesNotContain("SelectedIndex=\"0\"", xaml, StringComparison.Ordinal);
        Assert.Contains("_isSynchronizingViewSelection", code, StringComparison.Ordinal);
        Assert.Contains("_isViewLoaded", code, StringComparison.Ordinal);
        Assert.Contains(
            "WeatherViewSegmented.SelectedIndex = selectedIndex",
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
