using System.Runtime.InteropServices;
using DeskBox.Services;

namespace DeskBox;

// This probe is installed under a unique MSIX identity. It shares the production
// registration, activation guard, argument reader and durable forwarding store.
internal static class App
{
    internal static void Log(string value) => Probe.Write("platform.log", value);
}

internal static partial class Probe
{
    private static readonly object Gate = new();
    private static string _root = string.Empty;
    private static int _received;

    [STAThread]
    private static int Main(string[] args)
    {
        try
        {
            if (args.Length >= 3 && args[0] == "--invoke")
            {
                Invoke(Guid.Parse(args[1]), args[2]);
                return 0;
            }
            if (args.Length == 2 && args[0] == "--drain")
            {
                _root = args[1];
                var store = new NativeNotificationActivationEnvelopeStore(_root);
                int count = 0;
                while (store.TryTakeNext() is { Disposition: NativeNotificationActivationEnvelopeTakeDisposition.Consumed, Envelope: { } envelope })
                {
                    Write("forwarded.log", $"source={envelope.ActivationSource} args={envelope.Arguments} input={envelope.UserInput.GetValueOrDefault("snooze")}");
                    count++;
                }
                return count == 2 ? 0 : 2;
            }

            // CommonDocuments is outside MSIX LocalAppData virtualization, so
            // the external verifier can read the same isolated evidence files.
            _root = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonDocuments),
                "DeskBox-NotificationActivationProbe", Windows.ApplicationModel.Package.Current.Id.Name);
            Directory.CreateDirectory(_root);
            Write("platform.log", $"begin pid={Environment.ProcessId}");
            using var service = new NativeAppNotificationService(Receive);
            var bootstrap = new NativeNotificationActivationBootstrap(
                () => service.Register(), service.ReadCurrentActivation,
                ex => Write("failure.log", ex.ToString()));
            NativeAppNotificationActivation? initial = bootstrap.Capture();
            if (initial is not null)
                Receive(initial);
            DateTime deadline = DateTime.UtcNow.AddSeconds(25);
            while (Volatile.Read(ref _received) < 2 && DateTime.UtcNow < deadline)
                Thread.Sleep(50);
            Write("platform.log", $"end pid={Environment.ProcessId} count={_received}");
            return _received == 2 ? 0 : 3;
        }
        catch (Exception ex)
        {
            if (!string.IsNullOrEmpty(_root)) Write("failure.log", ex.ToString());
            return 1;
        }
    }

    private static void Receive(NativeAppNotificationActivation activation)
    {
        var store = new NativeNotificationActivationEnvelopeStore(_root);
        var written = store.Store(activation);
        if (written.Disposition != NativeNotificationActivationEnvelopeWriteDisposition.Stored)
            throw new InvalidOperationException($"Forwarding failed: {written.Error}");
        Write("events.log", $"pid={Environment.ProcessId} source={activation.Source} args={activation.Arguments} input={activation.UserInput.GetValueOrDefault("snooze")}");
        Interlocked.Increment(ref _received);
    }

    internal static void Write(string name, string value)
    {
        if (string.IsNullOrEmpty(_root)) return;
        lock (Gate)
        {
            Directory.CreateDirectory(_root);
            File.AppendAllText(Path.Combine(_root, name), value + Environment.NewLine);
        }
    }

    private static unsafe void Invoke(Guid clsid, string token)
    {
        int initialized = CoInitializeEx(0, 2);
        if (initialized < 0 && initialized != unchecked((int)0x80010106))
            Marshal.ThrowExceptionForHR(initialized);
        nint callback = 0;
        nint aumid = 0, arguments = 0, key = 0, value = 0;
        try
        {
            Guid iid = new("53E31837-6600-4A81-9395-75CFFE746F94"); // INotificationActivationCallback
            Marshal.ThrowExceptionForHR(CoCreateInstance(in clsid, 0, 4, in iid, out callback));
            aumid = Marshal.StringToCoTaskMemUni("DeskBox.NotificationActivationProbe");
            arguments = Marshal.StringToCoTaskMemUni($"source=todoReminder;action=snooze;id={token}");
            key = Marshal.StringToCoTaskMemUni("snooze");
            value = Marshal.StringToCoTaskMemUni("30m");
            var input = new UserInput { Key = key, Value = value };
            var activate = (delegate* unmanaged[Stdcall]<nint, nint, nint, UserInput*, uint, int>)(*(nint**)callback)[3];
            Marshal.ThrowExceptionForHR(activate(callback, aumid, arguments, &input, 1));
        }
        finally
        {
            Marshal.FreeCoTaskMem(aumid);
            Marshal.FreeCoTaskMem(arguments);
            Marshal.FreeCoTaskMem(key);
            Marshal.FreeCoTaskMem(value);
            if (callback != 0)
                _ = ((delegate* unmanaged[Stdcall]<nint, uint>)(*(nint**)callback)[2])(callback);
            if (initialized >= 0) CoUninitialize();
        }
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct UserInput { internal nint Key; internal nint Value; }
    [LibraryImport("ole32.dll")]
    private static partial int CoInitializeEx(nint reserved, uint concurrencyModel);
    [LibraryImport("ole32.dll")]
    private static partial void CoUninitialize();
    [LibraryImport("ole32.dll")]
    private static partial int CoCreateInstance(in Guid clsid, nint outer, uint context, in Guid iid, out nint instance);
}
