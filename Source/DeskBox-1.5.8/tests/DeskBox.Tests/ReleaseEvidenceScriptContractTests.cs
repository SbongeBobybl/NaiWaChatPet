using System.Diagnostics;
using System.Security.Cryptography;
using System.Text.Json;

namespace DeskBox.Tests;

public sealed class ReleaseEvidenceScriptContractTests : IDisposable
{
    private readonly string _testRoot = Path.Combine(
        Path.GetTempPath(),
        "deskbox-release-evidence-tests",
        Guid.NewGuid().ToString("N"));

    [Fact]
    public void Evidence_IsStableSortedAndHashesArtifactsAndProvenance()
    {
        string artifactRoot = Path.Combine(_testRoot, "release");
        string provenanceRoot = Path.Combine(_testRoot, "source");
        Directory.CreateDirectory(Path.Combine(artifactRoot, "nested"));
        Directory.CreateDirectory(Path.Combine(provenanceRoot, "src", "DeskBox"));
        File.WriteAllText(Path.Combine(artifactRoot, "z-last.txt"), "z");
        File.WriteAllText(Path.Combine(artifactRoot, "nested", "a-first.bin"), "alpha");
        File.WriteAllText(
            Path.Combine(provenanceRoot, "src", "DeskBox", "packages.lock.json"),
            "{\"version\":1}");

        ProcessResult first = RunScript(
            "-ArtifactRoot", artifactRoot,
            "-ProductVersion", "1.4.5",
            "-Commit", "0123456789abcdef",
            "-RuntimeIdentifier", "win-x64",
            "-Channel", "direct",
            "-Dirty",
            "-ProvenanceRoot", provenanceRoot,
            "-ProvenancePath", "src\\DeskBox\\packages.lock.json");
        Assert.Equal(0, first.ExitCode);

        string manifestPath = Path.Combine(artifactRoot, "release-manifest.json");
        string checksumsPath = Path.Combine(artifactRoot, "SHA256SUMS");
        string firstManifest = File.ReadAllText(manifestPath);
        string firstChecksums = File.ReadAllText(checksumsPath);

        using JsonDocument manifest = JsonDocument.Parse(firstManifest);
        JsonElement root = manifest.RootElement;
        Assert.Equal("1.4.5", root.GetProperty("productVersion").GetString());
        Assert.Equal("0123456789abcdef", root.GetProperty("commit").GetString());
        Assert.True(root.GetProperty("dirty").GetBoolean());
        Assert.Equal("win-x64", root.GetProperty("runtimeIdentifier").GetString());
        Assert.Equal("direct", root.GetProperty("channel").GetString());

        JsonElement[] artifacts = root.GetProperty("artifacts").EnumerateArray().ToArray();
        Assert.Equal(["nested/a-first.bin", "z-last.txt"],
            artifacts.Select(item => item.GetProperty("path").GetString()).ToArray());
        Assert.Equal(Sha256(Path.Combine(artifactRoot, "nested", "a-first.bin")),
            artifacts[0].GetProperty("sha256").GetString());
        Assert.Equal(5, artifacts[0].GetProperty("size").GetInt64());
        Assert.Equal(
            $"{Sha256(Path.Combine(artifactRoot, "nested", "a-first.bin"))} *nested/a-first.bin\n" +
            $"{Sha256(Path.Combine(artifactRoot, "z-last.txt"))} *z-last.txt\n",
            firstChecksums);

        JsonElement[] provenance = root.GetProperty("provenance").EnumerateArray().ToArray();
        Assert.Equal(["src/DeskBox/packages.lock.json"],
            provenance.Select(item => item.GetProperty("path").GetString()).ToArray());
        Assert.Equal("nuget-lock", provenance[0].GetProperty("kind").GetString());

        ProcessResult second = RunScript(
            "-ArtifactRoot", artifactRoot,
            "-ProductVersion", "1.4.5",
            "-Commit", "0123456789abcdef",
            "-RuntimeIdentifier", "win-x64",
            "-Channel", "direct",
            "-Dirty",
            "-ProvenanceRoot", provenanceRoot,
            "-ProvenancePath", "src\\DeskBox\\packages.lock.json");
        Assert.Equal(0, second.ExitCode);
        Assert.Equal(firstManifest, File.ReadAllText(manifestPath));
        Assert.Equal(firstChecksums, File.ReadAllText(checksumsPath));
        Assert.DoesNotContain("release-manifest.json", firstChecksums, StringComparison.Ordinal);
        Assert.DoesNotContain("SHA256SUMS", firstChecksums, StringComparison.Ordinal);
    }

    [Fact]
    public void ArtifactPath_RejectsEscapeOutsideExplicitRootWithoutWritingEvidence()
    {
        string artifactRoot = Path.Combine(_testRoot, "release");
        Directory.CreateDirectory(artifactRoot);
        File.WriteAllText(Path.Combine(_testRoot, "outside.txt"), "outside");

        ProcessResult result = RunScript(
            "-ArtifactRoot", artifactRoot,
            "-ProductVersion", "1.4.5",
            "-Commit", "0123456789abcdef",
            "-RuntimeIdentifier", "win-x64",
            "-Channel", "direct",
            "-ArtifactPath", "..\\outside.txt");

        Assert.NotEqual(0, result.ExitCode);
        Assert.Contains("escapes its supplied root", result.StandardError, StringComparison.Ordinal);
        Assert.False(File.Exists(Path.Combine(artifactRoot, "release-manifest.json")));
        Assert.False(File.Exists(Path.Combine(artifactRoot, "SHA256SUMS")));
    }

    public void Dispose()
    {
        if (Directory.Exists(_testRoot))
        {
            Directory.Delete(_testRoot, recursive: true);
        }
    }

    private static ProcessResult RunScript(params string[] arguments)
    {
        string powershell = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.System),
            "WindowsPowerShell",
            "v1.0",
            "powershell.exe");
        ProcessStartInfo startInfo = new()
        {
            FileName = powershell,
            UseShellExecute = false,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            CreateNoWindow = true
        };
        startInfo.ArgumentList.Add("-NoProfile");
        startInfo.ArgumentList.Add("-NonInteractive");
        startInfo.ArgumentList.Add("-ExecutionPolicy");
        startInfo.ArgumentList.Add("Bypass");
        startInfo.ArgumentList.Add("-File");
        startInfo.ArgumentList.Add(TestPaths.FromRepository(
            "scripts/New-DeskBoxReleaseEvidence.ps1"));
        foreach (string argument in arguments)
        {
            startInfo.ArgumentList.Add(argument);
        }

        using Process process = Process.Start(startInfo)
            ?? throw new InvalidOperationException("Unable to start Windows PowerShell.");
        string standardOutput = process.StandardOutput.ReadToEnd();
        string standardError = process.StandardError.ReadToEnd();
        process.WaitForExit();
        return new ProcessResult(process.ExitCode, standardOutput, standardError);
    }

    private static string Sha256(string path) =>
        Convert.ToHexString(SHA256.HashData(File.ReadAllBytes(path))).ToLowerInvariant();

    private sealed record ProcessResult(
        int ExitCode,
        string StandardOutput,
        string StandardError);
}
