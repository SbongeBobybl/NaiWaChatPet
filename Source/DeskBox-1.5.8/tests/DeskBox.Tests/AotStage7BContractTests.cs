namespace DeskBox.Tests;

public sealed class AotStage7BContractTests
{
    [Fact]
    public void ProductLoaders_AcceptX64AndArm64ButRejectOtherProcessArchitectures()
    {
        string shortcut = Read("src/DeskBox/Helpers/ShortcutNativeBackend.cs");

        Assert.Contains(
            "Architecture.X64 or Architecture.Arm64",
            shortcut,
            StringComparison.Ordinal);
        Assert.DoesNotContain("supports only x64", shortcut, StringComparison.Ordinal);
    }

    [Fact]
    public void BuildScripts_LoadTheDllWheneverHostAndTargetArchitecturesMatch()
    {
        string script = Read("scripts/build-rust-native.ps1");
        Assert.Contains("expectedProcessArchitecture", script, StringComparison.Ordinal);
        Assert.Contains("host-and-target-architectures-match", script, StringComparison.Ordinal);
        Assert.Contains("cross-architecture-static-validation-only", script, StringComparison.Ordinal);
        Assert.Contains("runtime-load-plus-static-pe", script, StringComparison.Ordinal);
    }

    [Fact]
    public void Arm64MsvcEnvironment_PrefersNativeHostToolsAndRetainsX64Fallback()
    {
        string script = Read("scripts/rust-arm64-msvc-environment.ps1");

        foreach (string token in new[]
                 {
                     "ProcessArchitecture",
                     "Hostarm64",
                     "Hostx64",
                     "WindowsSdkHostArchitecture",
                     "VSCMD_ARG_HOST_ARCH"
                 })
        {
            Assert.Contains(token, script, StringComparison.Ordinal);
        }
    }

    [Fact]
    public void TestProject_BuildsRustModulesForTheRequestedArchitectureAndConfiguration()
    {
        string project = Read("tests/DeskBox.Tests/DeskBox.Tests.csproj");

        Assert.Contains("<DeskBoxNativeTestPlatform", project, StringComparison.Ordinal);
        Assert.Contains("-Platform $(DeskBoxNativeTestPlatform)", project, StringComparison.Ordinal);
        Assert.Contains("-Configuration $(Configuration)", project, StringComparison.Ordinal);
        Assert.Contains("rust-arm64-msvc-environment.ps1", project, StringComparison.Ordinal);
        Assert.DoesNotContain("-Platform x64 -Configuration Debug", project, StringComparison.Ordinal);
    }

    [Fact]
    public void GitHubWorkflow_UsesNativeArmRunnerAndUploadsTypedRuntimeEvidence()
    {
        string workflow = Read(".github/workflows/arm64-runtime.yml");
        string runner = Read("scripts/run-arm64-stage-7b-runtime.ps1");
        string gate = Read("tests/DeskBox.Tests/Arm64NativeRuntimeGateTests.cs");

        foreach (string token in new[]
                 {
                     "windows-11-vs2026-arm",
                     "workflow_dispatch",
                     "codex/stage7b-arm64-actions",
                     "run-arm64-stage-7b-runtime.ps1",
                     "actions/upload-artifact@v4"
                 })
        {
            Assert.Contains(token, workflow, StringComparison.Ordinal);
        }
        foreach (string token in new[]
                 {
                     "deskbox.arm64-stage7b-runtime-evidence.v1",
                     "github-hosted-arm64-runtime",
                     "targetArchitectureRuntimeExecuted",
                     "physicalUserDeviceExecuted",
                     "interactiveDesktopExecuted",
                     "DESKBOX_REQUIRE_ARM64_RUNTIME_GATE"
                 })
        {
            Assert.Contains(token, runner, StringComparison.Ordinal);
        }
        Assert.Contains("Architecture.Arm64", gate, StringComparison.Ordinal);
        Assert.Contains("ProbeAbiVersion", gate, StringComparison.Ordinal);
        Assert.DoesNotContain("SearchCore", runner, StringComparison.Ordinal);
    }

    private static string Read(string relativePath) =>
        File.ReadAllText(TestPaths.FromRepository(relativePath));
}
