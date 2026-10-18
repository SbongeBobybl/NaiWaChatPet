namespace DeskBox.Tests;

public sealed class WidgetTitleButtonMenuTests
{
    [Fact]
    public void TitleStyleMenu_ContainsGlobalMultiSelectButtonSubmenu()
    {
        string root = FindRepositoryRoot();
        string builder = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Services/WidgetChromeMenuBuilder.cs"));

        Assert.Contains("Widget.TitleButtons.Title", builder, StringComparison.Ordinal);
        Assert.Contains("new ToggleMenuFlyoutItem", builder, StringComparison.Ordinal);
        Assert.Contains("SupportedWidgetHoverButtonActions", builder, StringComparison.Ordinal);
        Assert.Contains("TryUpdateWidgetHoverButtonAction", builder, StringComparison.Ordinal);
        Assert.Contains("settingsService.Settings.ShowHoverButtons = true", builder, StringComparison.Ordinal);
        Assert.Contains("settingsService.SaveDebounced()", builder, StringComparison.Ordinal);

    }

    private static string FindRepositoryRoot()
    {
        DirectoryInfo? current = new(AppContext.BaseDirectory);
        while (current is not null)
        {
            if (File.Exists(Path.Combine(current.FullName, "src", "DeskBox", "DeskBox.csproj")))
            {
                return current.FullName;
            }

            current = current.Parent;
        }

        throw new DirectoryNotFoundException("Could not locate the DeskBox repository root.");
    }
}
