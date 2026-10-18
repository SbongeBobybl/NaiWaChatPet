namespace DeskBox.Services;

/// <summary>Removes obsolete DeskBox-owned filename-index artifacts.</summary>
internal static class LegacySearchIndexCleanupService
{
    private static readonly string[] s_legacyBaseNames =
    [
        "search-index.json",        "search-index-v2.json"
    ];

    private static readonly string[] s_suffixes =
    [
        string.Empty,        ".tmp",        ".dirty",        ".roots",        ".roots.tmp"
    ];

    internal static int TryCleanup(string? rootPath = null)
    {
        string cacheDirectory = Path.Combine(
            rootPath ?? DeskBoxDataPathService.Current.RootPath,            "cache");
        int removed = 0;
        foreach (string baseName in s_legacyBaseNames)
        {
            foreach (string suffix in s_suffixes)
            {
                string path = Path.Combine(cacheDirectory,baseName + suffix);
                try
                {
                    if (!File.Exists(path))
                    {
                        continue;
                    }

                    File.Delete(path);
                    removed++;
                    App.Log($"[SearchMigration] Removed obsolete local index artifact '{path}'.");
                }
                catch (Exception ex) when (ex is IOException or UnauthorizedAccessException)
                {
                    App.Log($"[SearchMigration] Could not remove obsolete index artifact '{path}': {ex.Message}");
                }
            }
        }

        return removed;
    }
}
