using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Security.Principal;
using System.Text;
using System.Xml.Linq;
using DeskBox.Services;

// Opt-in integration probe: run this EXE, including its DeskBox manifest.
// Creates only a disabled, GUID-scoped task and deletes it in finally.
// Never calls Enable/TryRegister, which would change the user's real startup task.
internal static partial class Program
{
    private static int Main()
    {
        string taskName = $"DeskBox Unicode Probe-小-{Guid.NewGuid():N}";
        string xmlPath = Path.Combine(Path.GetTempPath(), taskName + ".xml");
        bool registered = false;
        try
        {
            uint codePage = GetACP();
            Check(codePage == 65001, $"The DeskBox UTF-8 manifest was not applied: ACP={codePage}.");
            using WindowsIdentity identity = WindowsIdentity.GetCurrent();
            string sid = identity.User!.Value;
            const string executable = @"C:\DeskBox Unicode Probe\小 桌面 café 日本語 😀\DeskBox.exe";
            XDocument document = XDocument.Parse(DirectStartupTaskBackend.BuildTaskXml(executable, sid));
            XNamespace ns = document.Root!.Name.Namespace;
            document.Root.Element(ns + "Settings")!.Element(ns + "Enabled")!.Value = "false";
            document.Root.Element(ns + "RegistrationInfo")!.Element(ns + "URI")!.Value = "\\" + taskName;
            File.WriteAllText(xmlPath, document.ToString(), Encoding.Unicode);
            RunSchtasks("/Create", "/TN", taskName, "/XML", xmlPath);
            registered = true;

            foreach (ApartmentState apartment in new[] { ApartmentState.STA, ApartmentState.MTA })
            {
                Exception? failure = null;
                var thread = new Thread(() =>
                {
                    try
                    {
                        string xml = DirectStartupTaskXmlReader.Read(taskName);
                        DirectStartupTaskRegistration registration = DirectStartupTaskBackend.ParseTaskXml(xml);
                        Check(registration.ExecutablePath == executable, "Unicode executable path changed.");
                        Check(!registration.Enabled, "The probe task must stay disabled.");
                        Check(registration.TaskName == taskName, "Unicode task name changed.");

                        // The fixture has an isolated name and is disabled. Check
                        // the other production invariants on an in-memory copy.
                        registration = registration with
                        {
                            TaskName = DirectStartupTaskBackend.GetTaskName(sid),
                            Enabled = true
                        };
                        Check(new DirectStartupTaskBackend().IsPreferred(registration, executable),
                            DirectStartupTaskBackend.DescribePreferenceCheckResults(registration, executable));

                        // Rollback uses this exact Unicode snapshot, written as UTF-16.
                        File.WriteAllText(xmlPath, xml, Encoding.Unicode);
                        RunSchtasks("/Create", "/TN", taskName, "/XML", xmlPath, "/F");
                        Check(DirectStartupTaskBackend.ParseTaskXml(
                            DirectStartupTaskXmlReader.Read(taskName)).ExecutablePath == executable,
                            "Unicode snapshot restore changed the path.");
                    }
                    catch (Exception ex)
                    {
                        failure = ex;
                    }
                }) { IsBackground = true };
                thread.SetApartmentState(apartment);
                thread.Start();
                Check(thread.Join(TimeSpan.FromSeconds(30)), $"{apartment} read timed out.");
                if (failure is not null)
                {
                    throw new InvalidOperationException($"{apartment} probe failed.", failure);
                }
                Console.WriteLine($"PASS ACP={codePage} apartment={apartment} UnicodeQuery=True Contract=True SnapshotRestore=True");
            }

            try
            {
                _ = DirectStartupTaskXmlReader.Read(taskName + "-missing");
                throw new InvalidOperationException("Missing task unexpectedly read successfully.");
            }
            catch (Exception ex) when (ex.HResult == unchecked((int)0x80070002))
            {
                Console.WriteLine("PASS MissingTask=0x80070002");
            }
            return 0;
        }
        catch (Exception ex)
        {
            Console.Error.WriteLine(ex);
            return 1;
        }
        finally
        {
            if (registered)
            {
                RunSchtasks("/Delete", "/TN", taskName, "/F");
                Console.WriteLine("PASS ProbeTaskDeleted=True");
            }
            File.Delete(xmlPath);
        }
    }

    private static void Check(bool condition, string message)
    {
        if (!condition)
        {
            throw new InvalidOperationException(message);
        }
    }

    private static void RunSchtasks(params string[] arguments)
    {
        var start = new ProcessStartInfo(Path.Combine(Environment.SystemDirectory, "schtasks.exe"))
        {
            UseShellExecute = false,
            CreateNoWindow = true,
            RedirectStandardOutput = true,
            RedirectStandardError = true
        };
        foreach (string argument in arguments)
        {
            start.ArgumentList.Add(argument);
        }
        using Process process = Process.Start(start)!;
        Task<string> output = process.StandardOutput.ReadToEndAsync();
        Task<string> error = process.StandardError.ReadToEndAsync();
        if (!process.WaitForExit(10_000))
        {
            process.Kill();
            throw new TimeoutException("Probe schtasks command timed out.");
        }
        Task.WaitAll(output, error);
        Check(process.ExitCode == 0, $"schtasks exit={process.ExitCode}: {error.Result} {output.Result}");
    }

    [LibraryImport("kernel32.dll")]
    private static partial uint GetACP();
}
