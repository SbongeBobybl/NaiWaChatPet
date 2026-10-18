namespace DeskBox.Tests;

public sealed class LocalDumpsSupportContractTests
{
    private const string EnableScriptPath = "scripts/Enable-DeskBoxLocalDumps.ps1";
    private const string DisableScriptPath = "scripts/Disable-DeskBoxLocalDumps.ps1";
    private const string DocumentationPath = "docs/support/crash-dumps.md";

    [Fact]
    public void Scripts_UseCurrentUserPerExecutableLocalDumpsOnly()
    {
        string enable = File.ReadAllText(TestPaths.FromRepository(EnableScriptPath));
        string disable = File.ReadAllText(TestPaths.FromRepository(DisableScriptPath));
        string combined = enable + Environment.NewLine + disable;

        Assert.Contains(
            "HKCU:\\Software\\Microsoft\\Windows\\Windows Error Reporting\\LocalDumps",
            combined,
            StringComparison.Ordinal);
        Assert.Contains(
            "[ValidateSet('DeskBox.exe', 'DeskBox.Updater.exe')]",
            enable,
            StringComparison.Ordinal);
        Assert.Contains("Join-Path $localDumpsRoot $exeName", combined, StringComparison.Ordinal);
        Assert.DoesNotContain("HKLM:", combined, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("HKEY_LOCAL_MACHINE", combined, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("CurrentControlSet", combined, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("-Verb RunAs", combined, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void EnableScript_DefaultsToBoundedMiniDumpsInLocalAppData()
    {
        string enable = File.ReadAllText(TestPaths.FromRepository(EnableScriptPath));

        Assert.Contains("GetFolderPath('LocalApplicationData')", enable, StringComparison.Ordinal);
        Assert.Contains("'DeskBox\\CrashDumps'", enable, StringComparison.Ordinal);
        Assert.Contains("[int]$DumpCount = 5", enable, StringComparison.Ordinal);
        Assert.Contains("[string]$DumpType = 'Mini'", enable, StringComparison.Ordinal);
        Assert.Contains("[ValidateRange(1, 50)]", enable, StringComparison.Ordinal);
        Assert.Contains("[IO.Path]::IsPathRooted", enable, StringComparison.Ordinal);
    }

    [Fact]
    public void Scripts_PreserveUnmanagedOrExternallyChangedSettings()
    {
        string enable = File.ReadAllText(TestPaths.FromRepository(EnableScriptPath));
        string disable = File.ReadAllText(TestPaths.FromRepository(DisableScriptPath));

        Assert.Contains("are not managed by this script", enable, StringComparison.Ordinal);
        Assert.Contains("changed since DeskBox configured it", enable, StringComparison.Ordinal);
        Assert.Contains("Test-RegistryValueEqual", enable, StringComparison.Ordinal);
        Assert.Contains("DeskBoxManagedBy", enable, StringComparison.Ordinal);
        Assert.Contains("Test-ManagedValueMatch", disable, StringComparison.Ordinal);
        Assert.Contains("was changed after DeskBox configured it and was left unchanged", disable, StringComparison.Ordinal);
        Assert.Contains("Remove-Item -LiteralPath $targetKey", disable, StringComparison.Ordinal);
        Assert.DoesNotContain("Remove-Item -LiteralPath $localDumpsRoot", disable, StringComparison.Ordinal);
        Assert.DoesNotContain("Remove-Item -LiteralPath $managementRoot", disable, StringComparison.Ordinal);
        Assert.DoesNotContain("Remove-Item -Recurse", disable, StringComparison.OrdinalIgnoreCase);
    }

    [Fact]
    public void SupportTool_DoesNotUploadOrJoinTheDiagnosticsBundle()
    {
        string enable = File.ReadAllText(TestPaths.FromRepository(EnableScriptPath));
        string disable = File.ReadAllText(TestPaths.FromRepository(DisableScriptPath));
        string documentation = File.ReadAllText(TestPaths.FromRepository(DocumentationPath));
        string combinedScripts = enable + Environment.NewLine + disable;

        Assert.DoesNotContain("Invoke-WebRequest", combinedScripts, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("Invoke-RestMethod", combinedScripts, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("HttpClient", combinedScripts, StringComparison.OrdinalIgnoreCase);
        Assert.DoesNotContain("DeskBoxDiagnosticsBundle", combinedScripts, StringComparison.Ordinal);
        Assert.Contains("不会自动上传、打包或发送转储", documentation, StringComparison.Ordinal);
        Assert.Contains("不会把转储加入现有诊断包", documentation, StringComparison.Ordinal);
    }
}
