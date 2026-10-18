using System.Diagnostics;

namespace DeskBox.Tests;

public sealed class TransferIntegrityScriptTests : IDisposable
{
    private readonly string _tempRoot = Path.Combine(
        Path.GetTempPath(),
        "DeskBox.TransferIntegrity.Tests",
        Guid.NewGuid().ToString("N"));

    [Fact]
    public async Task CompareFileTransferRoots_ReportsExpectedDifferencesReadOnly()
    {
        string sourceRoot = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "source")).FullName;
        string destinationRoot = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "destination")).FullName;
        string outputDirectory = Path.Combine(_tempRoot, "reports");

        string matchingSource = Path.Combine(sourceRoot, "match.pdf");
        string matchingDestination = Path.Combine(
            destinationRoot,
            "match.pdf");
        await File.WriteAllTextAsync(matchingSource, "same");
        File.Copy(matchingSource, matchingDestination);
        File.SetLastWriteTimeUtc(
            matchingDestination,
            File.GetLastWriteTimeUtc(matchingSource));
        await File.WriteAllTextAsync(
            Path.Combine(sourceRoot, "source-only.docx"),
            "source");
        await File.WriteAllTextAsync(
            Path.Combine(destinationRoot, "destination-only.zip"),
            "destination");
        await File.WriteAllTextAsync(
            Path.Combine(sourceRoot, "different.zip"),
            "short");
        await File.WriteAllTextAsync(
            Path.Combine(destinationRoot, "different.zip"),
            "much-longer");

        string powerShellPath = Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.System),
            "WindowsPowerShell",
            "v1.0",
            "powershell.exe");
        string scriptPath = TestPaths.FromRepository(
            "scripts/compare-file-transfer-roots.ps1");
        var startInfo = new ProcessStartInfo(powerShellPath)
        {
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true
        };
        startInfo.ArgumentList.Add("-NoProfile");
        startInfo.ArgumentList.Add("-ExecutionPolicy");
        startInfo.ArgumentList.Add("Bypass");
        startInfo.ArgumentList.Add("-File");
        startInfo.ArgumentList.Add(scriptPath);
        startInfo.ArgumentList.Add("-SourceRoot");
        startInfo.ArgumentList.Add(sourceRoot);
        startInfo.ArgumentList.Add("-DestinationRoot");
        startInfo.ArgumentList.Add(destinationRoot);
        startInfo.ArgumentList.Add("-OutputDirectory");
        startInfo.ArgumentList.Add(outputDirectory);

        using Process process = Process.Start(startInfo) ??
            throw new InvalidOperationException("Could not start Windows PowerShell.");
        Task<string> outputTask = process.StandardOutput.ReadToEndAsync();
        Task<string> errorTask = process.StandardError.ReadToEndAsync();
        await process.WaitForExitAsync();
        string output = await outputTask;
        string error = await errorTask;

        Assert.True(
            process.ExitCode == 0,
            $"Integrity script failed with exit code {process.ExitCode}.\n" +
            $"stdout:\n{output}\nstderr:\n{error}");
        string reportPath = Assert.Single(Directory.EnumerateFiles(
            outputDirectory,
            "DeskBox-Transfer-Integrity-*.csv"));
        string report = await File.ReadAllTextAsync(reportPath);
        Assert.Contains("\"Match\"", report, StringComparison.Ordinal);
        Assert.Contains("\"SourceOnly\"", report, StringComparison.Ordinal);
        Assert.Contains("\"DestinationOnly\"", report, StringComparison.Ordinal);
        Assert.Contains("\"SizeMismatch\"", report, StringComparison.Ordinal);

        Assert.Equal("same", await File.ReadAllTextAsync(matchingSource));
        Assert.Equal("same", await File.ReadAllTextAsync(matchingDestination));
    }

    public void Dispose()
    {
        if (Directory.Exists(_tempRoot))
        {
            Directory.Delete(_tempRoot, recursive: true);
        }
    }
}
