namespace DeskBox.Tests;

public sealed class TrayEfficiencyModeContractTests
{
    [Fact]
    public void TrayStartup_NeverEnablesProcessEfficiencyMode()
    {
        string source = File.ReadAllText(TestPaths.FromRepository("src/DeskBox/App.Tray.cs"));

        Assert.Contains("ForceCreate(enablesEfficiencyMode: false)", source, StringComparison.Ordinal);
        Assert.Contains(
            "WindowExtensions.Hide(_trayWindow, enableEfficiencyMode: false)",
            source,
            StringComparison.Ordinal);
        Assert.DoesNotContain("SetEfficiencyMode(", source, StringComparison.Ordinal);
        Assert.DoesNotContain("ProcessEfficiencyModeStartup", source, StringComparison.Ordinal);
    }
}
