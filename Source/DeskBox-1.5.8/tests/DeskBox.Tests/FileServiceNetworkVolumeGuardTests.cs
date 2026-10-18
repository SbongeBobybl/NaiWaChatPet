using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class FileServiceNetworkVolumeGuardTests
{
    [Fact]
    public void IsNetworkVolumePath_IdentifiesUncShares()
    {
        Assert.True(FileService.IsNetworkVolumePath(
            @"\\server\share\folder\file.txt"));
    }

    [Fact]
    public void IsNetworkVolumePath_IdentifiesMappedNetworkDrives()
    {
        // Drive letters whose drive type is Network must be detected through
        // DriveInfo — a mapped Z: is exactly the leak the audit found, since
        // it can be NTFS and pass the migration layer's file-system check.
        string? networkRoot = Directory.GetLogicalDrives()
            .FirstOrDefault(drive =>
            {
                try
                {
                    return new DriveInfo(drive).DriveType == DriveType.Network;
                }
                catch
                {
                    return false;
                }
            });

        if (networkRoot is null)
        {
            // No mapped network drive on this host: the UNC case above and
            // the local negative case below still pin the classification.
            return;
        }

        Assert.True(FileService.IsNetworkVolumePath(
            Path.Combine(networkRoot, "folder", "file.txt")));
    }

    [Fact]
    public void IsNetworkVolumePath_LocalVolumesAreNotNetwork()
    {
        string localPath = Path.Combine(Path.GetTempPath(), "file.txt");

        Assert.False(FileService.IsNetworkVolumePath(localPath));
    }
}
