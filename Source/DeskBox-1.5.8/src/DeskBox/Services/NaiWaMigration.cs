using System.Security.Cryptography;

namespace DeskBox.Services;

public static class NaiWaMigration
{
    public static string LegacyJournalDirectory => Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData),        "NaiWaChatPet","versions","13.4.0","tidy");
    public static int LegacyJournalCount => Directory.Exists(LegacyJournalDirectory)
        ? Directory.EnumerateFiles(LegacyJournalDirectory,"*.json").Take(1001).Count() : 0;

    public static async Task SnapshotBeforeSettingsLoadAsync()
    {
        string data = DeskBoxDataPathService.Current.DataDirectory;
        string migration = Path.Combine(DeskBoxDataPathService.Current.RootPath,"naiwa-migration-v1");
        string complete = Path.Combine(migration,"completed.txt");
        if (File.Exists(complete)) return;
        Directory.CreateDirectory(migration);
        string attempt = Path.Combine(migration,Guid.NewGuid().ToString("N"));Directory.CreateDirectory(attempt);
        var manifest = new List<string> { "NaiWa integration v1: original settings and journals only; no file relocation." };
        foreach (string name in new[]{"settings.json","widget-layout.json","desktop-organization-history.json","desktop-organization-recovery.json"})
        {
            string source = Path.Combine(data,name);if (!File.Exists(source)) continue;
            string copy = Path.Combine(attempt,name);File.Copy(source,copy,false);
            using var stream = File.OpenRead(copy);manifest.Add(name+" "+Convert.ToHexString(SHA256.HashData(stream)));
        }
        if (Directory.Exists(LegacyJournalDirectory))
        {
            string legacy = Path.Combine(attempt,"legacy-naiwa-journals");Directory.CreateDirectory(legacy);
            foreach (var source in Directory.EnumerateFiles(LegacyJournalDirectory,"*.json").Take(1000))
            {
                if ((File.GetAttributes(source)&FileAttributes.ReparsePoint)!=0) throw new IOException("旧日记包含重解析点，已停止迁移备份。");
                File.Copy(source,Path.Combine(legacy,Path.GetFileName(source)),false);
            }
        }
        await File.WriteAllLinesAsync(Path.Combine(attempt,"manifest.txt"),manifest);
        // Completion is written last. A failed snapshot never permits settings migration.
        await File.WriteAllTextAsync(complete,attempt);
    }
}
