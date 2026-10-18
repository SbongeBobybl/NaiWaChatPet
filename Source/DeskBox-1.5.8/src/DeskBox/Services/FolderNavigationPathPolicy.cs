using DeskBox.Helpers;
using DeskBox.Models;

namespace DeskBox.Services;

internal sealed record FolderNavigationPathResolution(
    string RequestedPath,    string RootPath,    string TargetPath,    bool MappedRootRequested);

/// <summary>
/// Resolves folder-navigation targets against the mapped folder's physical
/// directory tree. This policy is intentionally separate from drag/drop: a
/// shortcut can be activated as a folder without becoming a folder item.
/// </summary>
internal static class FolderNavigationPathPolicy
{
    internal static bool IsFolderShortcutCandidate(WidgetItem? item) =>
        item is not null &&
        !item.IsFolder &&
        item.IsShortcut &&
        ShortcutHelper.IsShellLinkPath(item.Path);

    internal static bool TryNormalizeShortcutTargetPath(
        string? storedTargetPath,        out string normalizedTargetPath)
    {
        normalizedTargetPath = string.Empty;
        if (string.IsNullOrWhiteSpace(storedTargetPath))
        {
            return false;
        }

        try
        {
            string expandedTargetPath = Environment.ExpandEnvironmentVariables(
                storedTargetPath.Trim());
            if (!Path.IsPathFullyQualified(expandedTargetPath))
            {
                return false;
            }

            normalizedTargetPath = Path.GetFullPath(expandedTargetPath);
            return true;
        }
        catch
        {
            return false;
        }
    }

    internal static bool TryResolve(
        string folderPath,        string mappedFolderPath,        string? mappedFolderTraversalPath,        out FolderNavigationPathResolution resolution)
    {
        resolution = null!;
        if (string.IsNullOrWhiteSpace(folderPath) ||
            string.IsNullOrWhiteSpace(mappedFolderPath))
        {
            return false;
        }

        try
        {
            string logicalRootPath = Path.GetFullPath(mappedFolderPath);
            string requestedPath = Path.GetFullPath(folderPath);
            bool mappedRootRequested = ArePathsEqual(
                requestedPath,                logicalRootPath);

            string rootCandidate = !mappedRootRequested &&
                                   !string.IsNullOrWhiteSpace(
                                       mappedFolderTraversalPath)
                ? mappedFolderTraversalPath
                : logicalRootPath;
            if (!FileService.TryResolveExistingPathForTraversal(
                    rootCandidate,                    out string rootPath) ||
                !Directory.Exists(rootPath))
            {
                return false;
            }

            string targetPath;
            if (mappedRootRequested)
            {
                targetPath = rootPath;
            }
            else if (!FileService.TryResolveExistingPathForTraversal(
                         requestedPath,                         out targetPath))
            {
                return false;
            }

            if (!Directory.Exists(targetPath) ||
                !FileService.TryIsPathUnderDirectoryResolved(
                    targetPath,                    rootPath,                    out bool isUnderMappedRoot) ||
                !isUnderMappedRoot)
            {
                return false;
            }

            resolution = new FolderNavigationPathResolution(
                requestedPath,                rootPath,                targetPath,                mappedRootRequested);
            return true;
        }
        catch
        {
            return false;
        }
    }

    internal static bool ArePathsEqual(string? first,string? second)
    {
        if (string.IsNullOrWhiteSpace(first) ||
            string.IsNullOrWhiteSpace(second))
        {
            return false;
        }

        try
        {
            return string.Equals(
                Path.TrimEndingDirectorySeparator(Path.GetFullPath(first)),                Path.TrimEndingDirectorySeparator(Path.GetFullPath(second)),                StringComparison.OrdinalIgnoreCase);
        }
        catch
        {
            return false;
        }
    }
}
