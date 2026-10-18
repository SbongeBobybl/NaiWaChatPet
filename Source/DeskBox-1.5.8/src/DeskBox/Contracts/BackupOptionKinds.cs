namespace DeskBox.Contracts;

/// <summary>
/// Canonical option values, defaults and read normalization for the backup
/// settings family (local snapshots and cloud/WebDAV backups). The Services
/// policies (<c>DataBackupSettingsPolicy</c>/<c>CloudBackupSettingsPolicy</c>)
/// alias these members so the coordinator write path and the section editor
/// (batch 49, <c>Features/Backup/BackupSettingsViewModel</c>) share one
/// source of truth without the editor referencing Services.
/// </summary>
public static class BackupOptionKinds
{
    // --- Local automatic snapshots ---

    public const bool DefaultLocalEnabled = true;
    public const int DefaultLocalIntervalMinutes = 24 * 60;
    public const int DefaultLocalRetentionCount = 7;

    /// <summary>Interval presets offered in settings, in ascending order.</summary>
    public static readonly int[] LocalIntervalMinutes =
        [5,30,60,12 * 60,24 * 60,5 * 24 * 60];

    /// <summary>Retention presets offered in settings, in ascending order.</summary>
    public static readonly int[] LocalRetentionCounts = [3,5,7,14,30];

    public static int NormalizeLocalIntervalMinutes(int value) =>
        LocalIntervalMinutes.Contains(value) ? value : DefaultLocalIntervalMinutes;

    public static int NormalizeLocalRetentionCount(int value) =>
        LocalRetentionCounts.Contains(value) ? value : DefaultLocalRetentionCount;

    public static string? NormalizeLocalDirectory(string? path) =>
        string.IsNullOrWhiteSpace(path) ? null : path.Trim();

    // --- Cloud/WebDAV backups ---

    public const string ProviderNone = "none";
    public const string ProviderWebDav = "webdav";
    public const int DefaultCloudIntervalMinutes = 24 * 60;
    public const int DefaultCloudRetentionCount = 5;

    /// <summary>Preset interval choices for the settings ComboBox.</summary>
    public static readonly int[] CloudIntervalMinutes = [60,360,720,1440,10080];

    /// <summary>Preset retention choices for the settings ComboBox.</summary>
    public static readonly int[] CloudRetentionCounts = [3,5,7,10,14];

    public static int NormalizeCloudIntervalMinutes(int minutes) =>
        CloudIntervalMinutes.Contains(minutes) ? minutes : DefaultCloudIntervalMinutes;

    public static int NormalizeCloudRetentionCount(int count) =>
        CloudRetentionCounts.Contains(count) ? count : DefaultCloudRetentionCount;

    /// <summary>
    /// Normalizes a provider selection: anything that is not WebDAV reads
    /// back as off (the legacy shell setter semantics).
    /// </summary>
    public static string NormalizeProvider(string? value) =>
        value == ProviderWebDav ? ProviderWebDav : ProviderNone;
}
