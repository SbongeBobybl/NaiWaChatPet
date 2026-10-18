namespace DeskBox.Tests;

public sealed class WindowsAppRuntimeInsightsPayloadContractTests
{
    [Fact]
    public void RetailSelfContainedPublish_ExtractsAndAuditsRestoreLockedInsightsResource()
    {
        string script = File.ReadAllText(
            TestPaths.FromRepository("scripts/publish-aot-retail.ps1"));
        string distributionScript = File.ReadAllText(
            TestPaths.FromRepository("scripts/build-stage-7c1-distribution.ps1"));

        foreach (string token in new[]
                 {
                     "Copy-WindowsAppRuntimeInsightsResource",
                     "obj\\DeskBox\\project.assets.json",
                     "Microsoft.WindowsAppSDK.Runtime/*",
                     "tools/MSIX/win10-$NativePlatform/Microsoft.WindowsAppRuntime.2.msix",
                     "Microsoft.WindowsAppRuntime.Insights.Resource.dll",
                     "[System.IO.Compression.ZipFile]::OpenRead",
                     "windowsAppRuntimeInsightsResource = $windowsAppRuntimeInsightsResource"
                 })
        {
            Assert.Contains(token, script, StringComparison.Ordinal);
        }

        Assert.True(
            script.Split("Microsoft.WindowsAppRuntime.Insights.Resource.dll").Length - 1 >= 3,
            "The retail script must extract, require, and architecture-check the resource DLL.");

        foreach (string token in new[]
                 {
                     "windowsAppRuntimeInsightsResource",
                     "Microsoft.WindowsAppRuntime.Insights.Resource.dll",
                     "$windowsAppRuntimeInsightsMachine = Get-PeMachine",
                     "windowsAppRuntimeInsightsMachine ="
                 })
        {
            Assert.Contains(token, distributionScript, StringComparison.Ordinal);
        }
    }
}
