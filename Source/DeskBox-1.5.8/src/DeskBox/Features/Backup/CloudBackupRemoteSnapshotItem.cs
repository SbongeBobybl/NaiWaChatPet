using DeskBox.Contracts;

namespace DeskBox.Features.Backup;

/// <summary>
/// One remote snapshot row in the cloud-backup restore list (moved from the
/// settings shell with the section's binding surface, batch 49). Title and
/// Details are rendered through compiled {x:Bind} (AOT-safe); the generated
/// bindable metadata stays as a safety net for any future {Binding} use.
/// </summary>
[WinRT.GeneratedBindableCustomProperty]
public sealed partial class CloudBackupRemoteSnapshotItem
{
    internal CloudBackupRemoteSnapshotItem(
        string name,        string title,        string details,        BackupEndpoint endpoint)
    {
        Name = name;
        Title = title;
        Details = details;
        Endpoint = endpoint;
    }

    public string Name { get; }
    public string Title { get; }
    public string Details { get; }
    internal BackupEndpoint Endpoint { get; }
}
