namespace DeskBox.Tests;

public sealed class AotStage7C0ContractTests
{
    [Fact]
    public void RustBuilds_UseExplicitRestoredCrtFlagsAndReportPeDependencies()
    {
        string script = Read("scripts/build-rust-native.ps1");
        foreach (string token in new[]
                 {
                     "[ValidateSet(\"Dynamic\", \"Static\")]",
                     "[string]$CrtLinkage = \"Static\"",
                     "target-feature=+crt-static",
                     "target-feature=-crt-static",
                     "CARGO_ENCODED_RUSTFLAGS",
                     "previousEncodedRustFlags",
                     "previousRustFlags",
                     "VcRuntimeImports",
                     "SizeOfImage"
                 })
        {
            Assert.Contains(token, script, StringComparison.Ordinal);
        }

        string pe = Read("scripts/native-pe-contract.ps1");
        Assert.Contains("importRva", pe, StringComparison.Ordinal);
        Assert.Contains("import descriptor", pe, StringComparison.Ordinal);
        Assert.Contains("ImportedModules", pe, StringComparison.Ordinal);
        Assert.Contains("SizeOfImage", pe, StringComparison.Ordinal);
    }

    [Fact]
    public void ManagedBuilds_IsolateAndForwardTheSelectedCrtLinkage()
    {
        foreach (string relativePath in new[]
                 {
                     "src/DeskBox/DeskBox.csproj",
                     "tests/DeskBox.Tests/DeskBox.Tests.csproj"
                 })
        {
            string project = Read(relativePath);
            Assert.Contains("DeskBoxRustCrtLinkage", project, StringComparison.Ordinal);
            Assert.Contains(">Static</DeskBoxRustCrtLinkage>", project, StringComparison.Ordinal);
            Assert.Contains("-CrtLinkage $(DeskBoxRustCrtLinkage)", project, StringComparison.Ordinal);
            Assert.Contains("$(DeskBoxRustCrtLinkage)", project, StringComparison.Ordinal);
            Assert.Contains("must be Dynamic or Static", project, StringComparison.Ordinal);
        }
    }

    [Fact]
    public void Audit_SeparatesArchitectureRecommendationFromProductionDecision()
    {
        string audit = Read("scripts/audit-rust-crt-distribution.ps1");

        foreach (string token in new[]
                 {
                     "deskbox.rust-crt-stage7c0-evidence.v1",
                     "isolated-host-process-load-and-abi-delta",
                     "runtime-ab-plus-static-pe",
                     "cross-compiled-static-pe-only",
                     "staticMinusDynamicFileBytes",
                     "staticMinusDynamicImageBytes",
                     "boundedBelowOneMiB",
                     "recommendationForAuditedPlatforms",
                     "productionDecision = $productionDecision",
                     "combine native x64 and ARM64 runtime evidence",
                     "DeskBoxRustCrtLinkage=Static"
                 })
        {
            Assert.Contains(token, audit, StringComparison.Ordinal);
        }
        Assert.Contains("$productionDecision = \"Pending\"", audit, StringComparison.Ordinal);
    }

    [Fact]
    public void Arm64Workflow_RunsRuntimeAndCrtGatesAndUploadsSeparateEvidence()
    {
        string workflow = Read(".github/workflows/arm64-runtime.yml");

        Assert.Contains("Run Stage 7B native ARM64 gate", workflow, StringComparison.Ordinal);
        Assert.Contains("Run Stage 7C0 ARM64 CRT A/B gate", workflow, StringComparison.Ordinal);
        Assert.Contains("audit-rust-crt-distribution.ps1", workflow, StringComparison.Ordinal);
        Assert.Contains("-Platforms ARM64", workflow, StringComparison.Ordinal);
        Assert.Contains("arm64-stage7c0-crt-evidence", workflow, StringComparison.Ordinal);

        string runtimeGate = Read("scripts/run-arm64-stage-7b-runtime.ps1");
        Assert.Contains("-CrtLinkage Static", runtimeGate, StringComparison.Ordinal);
        Assert.Contains("-p:DeskBoxRustCrtLinkage=Static", runtimeGate, StringComparison.Ordinal);
        Assert.Contains("$nativeResult.CrtLinkage -ne \"Static\"", runtimeGate, StringComparison.Ordinal);
        Assert.Contains("$nativeResult.VcRuntimeImports.Count -ne 0", runtimeGate, StringComparison.Ordinal);
    }

    private static string Read(string relativePath) =>
        File.ReadAllText(TestPaths.FromRepository(relativePath));
}
