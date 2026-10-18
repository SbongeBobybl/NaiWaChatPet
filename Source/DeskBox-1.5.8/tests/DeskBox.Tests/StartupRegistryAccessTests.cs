using DeskBox.Services;
using Microsoft.Win32;

namespace DeskBox.Tests;

public sealed class StartupRegistryAccessTests
{
    [Fact]
    public async Task DeferredAccessReadsAndRemovesTheSameUsersEntry()
    {
        string path = @"Software\DeskBox.Tests\Startup-" + Guid.NewGuid().ToString("N");
        try
        {
            using (RegistryKey key = Registry.CurrentUser.CreateSubKey(path))
                key.SetValue("Probe", "startup-command");

            var store = new RegistryStartupRunEntryStore(path, "Probe");
            Assert.Equal("startup-command", await Task.Run(store.Read));
            const string unicodeCommand = "\"C:\\小 桌面\\DeskBox.exe\" --startup";
            await Task.Run(() => store.Write(unicodeCommand));
            Assert.Equal(unicodeCommand, await Task.Run(store.Read));
            await Task.Run(store.Delete);
            using RegistryKey? verify = Registry.CurrentUser.OpenSubKey(path);
            Assert.Null(verify?.GetValue("Probe"));
        }
        finally
        {
            Registry.CurrentUser.DeleteSubKeyTree(path, throwOnMissingSubKey: false);
        }
    }
}
