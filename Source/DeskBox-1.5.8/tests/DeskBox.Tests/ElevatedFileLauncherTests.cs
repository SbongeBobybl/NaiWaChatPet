using System.Diagnostics;
using System.Security.Principal;
using DeskBox.Helpers;
using DeskBox.Models;

namespace DeskBox.Tests;

public sealed class ElevatedFileLauncherTests
{
    [Fact]
    public void CanRunAsAdministrator_IsLimitedToExecutableTargets()
    {
        string root = Path.Combine(
            Path.GetTempPath(),
            "DeskBox-ElevatedLauncher-" + Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(root);
        string executable = Path.Combine(root, "tool.EXE");
        string document = Path.Combine(root, "readme.pdf");
        string executableNamedDirectory = Path.Combine(root, "folder.exe");
        File.WriteAllBytes(executable, []);
        File.WriteAllBytes(document, []);
        Directory.CreateDirectory(executableNamedDirectory);
        try
        {
            Assert.True(
                ElevatedFileLauncher.CanRunAsAdministrator(
                    new WidgetItem { Path = executable }));
            Assert.False(
                ElevatedFileLauncher.CanRunAsAdministrator(
                    new WidgetItem { Path = document }));
            Assert.False(
                ElevatedFileLauncher.CanRunAsAdministrator(
                    new WidgetItem { Path = executableNamedDirectory }));
        }
        finally
        {
            Directory.Delete(root, recursive: true);
        }
    }

    [Fact]
    public void CreateLaunchRequest_UsesCommandInterpreterForCommandScripts()
    {
        string script = Path.Combine("C:\\Tools", "maintenance task.cmd");

        ElevatedFileLaunchRequest request =
            ElevatedFileLauncher.CreateLaunchRequest(
                script,
                "--repair",
                "C:\\Tools");

        Assert.Equal("cmd.exe", Path.GetFileName(request.FileName),
            ignoreCase: true);
        Assert.Contains("/d /s /c", request.Arguments,
            StringComparison.OrdinalIgnoreCase);
        Assert.Contains($"\"{script}\"", request.Arguments,
            StringComparison.Ordinal);
        Assert.Contains("--repair", request.Arguments,
            StringComparison.Ordinal);
        Assert.False(request.DetectExistingProcess);
    }

    [Fact]
    public void CreateLaunchRequest_UsesWindowsInstallerForMsiPackages()
    {
        string package = Path.Combine("C:\\Tools", "setup package.msi");

        ElevatedFileLaunchRequest request =
            ElevatedFileLauncher.CreateLaunchRequest(
                package,
                string.Empty,
                "C:\\Tools");

        Assert.Equal("msiexec.exe", Path.GetFileName(request.FileName),
            ignoreCase: true);
        Assert.Equal($"/i \"{package}\"", request.Arguments);
        Assert.False(request.DetectExistingProcess);
    }

    [Fact]
    public void Launcher_RequestsAProcessHandleAndVerifiesElevation()
    {
        string source = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Helpers/ElevatedFileLauncher.cs"));

        Assert.Contains("ShellExecuteEx", source, StringComparison.Ordinal);
        Assert.Contains("ShellExecuteMaskNoCloseProcess", source,
            StringComparison.Ordinal);
        Assert.Contains("TryGetTokenElevation", source,
            StringComparison.Ordinal);
        Assert.Contains("TryFindRunningUnelevatedTarget", source,
            StringComparison.Ordinal);
        Assert.DoesNotContain("Win32Helper.ShellExecute(", source,
            StringComparison.Ordinal);
    }

    [Fact]
    public async Task RunAsAdministrator_BlocksExistingUnelevatedTarget()
    {
        if (new WindowsPrincipal(WindowsIdentity.GetCurrent()).IsInRole(
                WindowsBuiltInRole.Administrator))
        {
            // GitHub Actions Windows runners execute the test host inside an
            // elevated session, so a child process is never "unelevated" and
            // the guard under test cannot be observed there.
            return;
        }

        string ping = Path.Combine(Environment.SystemDirectory, "ping.exe");
        using Process process = Process.Start(new ProcessStartInfo
        {
            FileName = ping,
            Arguments = "127.0.0.1 -n 8",
            UseShellExecute = false,
            CreateNoWindow = true
        })!;

        try
        {
            await Task.Delay(200);

            ElevatedFileLaunchResult result =
                ElevatedFileLauncher.RunAsAdministrator(
                    IntPtr.Zero,
                    new WidgetItem { Path = ping });

            Assert.Equal(
                ElevatedFileLaunchStatus.AlreadyRunningUnelevated,
                result.Status);
            Assert.Equal((uint)process.Id, result.ProcessId);
        }
        finally
        {
            if (!process.HasExited)
            {
                process.Kill(entireProcessTree: true);
                await process.WaitForExitAsync();
            }
        }
    }
}
