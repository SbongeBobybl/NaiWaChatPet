using System.Diagnostics;
using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class FolderNavigationPathPolicyTests : IDisposable
{
    private readonly string _tempRoot;

    public FolderNavigationPathPolicyTests()
    {
        _tempRoot = Path.Combine(
            Path.GetTempPath(),
            "DeskBox.Tests",
            Guid.NewGuid().ToString("N"));
        Directory.CreateDirectory(_tempRoot);
    }

    [Fact]
    public void IsFolderShortcutCandidate_RequiresShellLinkFileIdentity()
    {
        Assert.True(FolderNavigationPathPolicy.IsFolderShortcutCandidate(
            new WidgetItem
            {
                Path = Path.Combine(_tempRoot, "folder.lnk"),
                IsShortcut = true
            }));
        Assert.False(FolderNavigationPathPolicy.IsFolderShortcutCandidate(
            new WidgetItem
            {
                Path = Path.Combine(_tempRoot, "website.url"),
                IsShortcut = true
            }));
        Assert.False(FolderNavigationPathPolicy.IsFolderShortcutCandidate(
            new WidgetItem
            {
                Path = Path.Combine(_tempRoot, "folder.lnk"),
                IsShortcut = true,
                IsFolder = true
            }));
    }

    [Fact]
    public void TryNormalizeShortcutTargetPath_RequiresAbsoluteFilesystemPath()
    {
        string expected = Path.Combine(_tempRoot, "child");

        Assert.True(FolderNavigationPathPolicy.TryNormalizeShortcutTargetPath(
            expected,
            out string normalized));
        Assert.Equal(Path.GetFullPath(expected), normalized, ignoreCase: true);
        Assert.False(FolderNavigationPathPolicy.TryNormalizeShortcutTargetPath(
            @"relative\child",
            out _));
        Assert.False(FolderNavigationPathPolicy.TryNormalizeShortcutTargetPath(
            "::{645FF040-5081-101B-9F08-00AA002F954E}",
            out _));
    }

    [Fact]
    public void TryResolve_AllowsOnlyExistingDirectoriesInsideMappedRoot()
    {
        string mappedRoot = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "mapped")).FullName;
        string child = Directory.CreateDirectory(
            Path.Combine(mappedRoot, "child")).FullName;
        string sibling = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "sibling")).FullName;

        Assert.True(FolderNavigationPathPolicy.TryResolve(
            child,
            mappedRoot,
            mappedFolderTraversalPath: null,
            out FolderNavigationPathResolution resolution));
        Assert.Equal(child, resolution.TargetPath, ignoreCase: true);
        Assert.False(resolution.MappedRootRequested);

        Assert.False(FolderNavigationPathPolicy.TryResolve(
            sibling,
            mappedRoot,
            mappedFolderTraversalPath: null,
            out _));
        Assert.False(FolderNavigationPathPolicy.TryResolve(
            Path.Combine(mappedRoot, "missing"),
            mappedRoot,
            mappedFolderTraversalPath: null,
            out _));
    }

    [Fact]
    public void TryResolve_RejectsJunctionThatEscapesMappedRoot()
    {
        string mappedRoot = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "mapped-junction-root")).FullName;
        string outside = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "outside")).FullName;
        string escapingJunction = Path.Combine(mappedRoot, "escape");

        Assert.True(
            TryCreateDirectoryJunction(escapingJunction, outside),
            "The Windows test host must support creating a directory junction.");
        try
        {
            Assert.False(FolderNavigationPathPolicy.TryResolve(
                escapingJunction,
                mappedRoot,
                mappedFolderTraversalPath: null,
                out _));
        }
        finally
        {
            TryDeleteDirectoryJunction(escapingJunction);
        }
    }

    [Fact]
    public void TryResolve_AllowsExternalAliasToPhysicalMappedDescendant()
    {
        string mappedRoot = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "physical-mapped-root")).FullName;
        string child = Directory.CreateDirectory(
            Path.Combine(mappedRoot, "child")).FullName;
        string alias = Path.Combine(_tempRoot, "child-alias");

        Assert.True(
            TryCreateDirectoryJunction(alias, child),
            "The Windows test host must support creating a directory junction.");
        try
        {
            Assert.True(FolderNavigationPathPolicy.TryResolve(
                alias,
                mappedRoot,
                mappedFolderTraversalPath: null,
                out FolderNavigationPathResolution resolution));
            Assert.Equal(child, resolution.TargetPath, ignoreCase: true);
        }
        finally
        {
            TryDeleteDirectoryJunction(alias);
        }
    }

    private static bool TryCreateDirectoryJunction(
        string junction,
        string target)
    {
        try
        {
            using var process = Process.Start(new ProcessStartInfo
            {
                FileName = "cmd.exe",
                Arguments = $"/d /c mklink /J \"{junction}\" \"{target}\"",
                CreateNoWindow = true,
                UseShellExecute = false,
                RedirectStandardOutput = true,
                RedirectStandardError = true
            });
            process?.WaitForExit();
            return process?.ExitCode == 0 && Directory.Exists(junction);
        }
        catch (Exception ex) when (
            ex is IOException or UnauthorizedAccessException or
            System.ComponentModel.Win32Exception)
        {
            return false;
        }
    }

    private static void TryDeleteDirectoryJunction(string junction)
    {
        try
        {
            Directory.Delete(junction, recursive: false);
        }
        catch
        {
        }
    }

    public void Dispose()
    {
        try
        {
            Directory.Delete(_tempRoot, recursive: true);
        }
        catch
        {
        }
    }
}
