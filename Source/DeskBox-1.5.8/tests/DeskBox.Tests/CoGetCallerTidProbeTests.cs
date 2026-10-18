using System.Runtime.InteropServices;
using Xunit.Abstractions;

namespace DeskBox.Tests;

/// <summary>
/// Records the no-active-call behavior of CoGetCallerTID for diagnostics.
/// File drag safety no longer depends on caller detection.
/// </summary>
public sealed class CoGetCallerTidProbeTests
{
    private const int S_FALSE = 1;
    private const uint CoinitMultithreaded = 0x0;

    private readonly ITestOutputHelper _output;

    public CoGetCallerTidProbeTests(ITestOutputHelper output)
    {
        _output = output;
    }

    // Classic DllImport on purpose: the test project is not compiled with
    // unsafe blocks for LibraryImport source generation, and the Platform
    // interop ratchet only scans src/DeskBox.
    [DllImport("ole32.dll")]
    private static extern int CoGetCallerTID(out uint threadId);

    [DllImport("ole32.dll")]
    private static extern int CoInitializeEx(nint reserved, uint coInit);

    [DllImport("ole32.dll")]
    private static extern void CoUninitialize();

    [Fact]
    public void PlainThread_NoComContext_IsNotCrossProcess()
    {
        int hr = CoGetCallerTID(out uint threadId);
        _output.WriteLine($"no-context CoGetCallerTID hr=0x{hr:X8} tid={threadId}");
        Assert.NotEqual(S_FALSE, hr);
    }

    [Fact]
    public void CoInitializedThread_NoActiveCall_IsNotCrossProcess()
    {
        int initHr = CoInitializeEx(0, CoinitMultithreaded);
        try
        {
            int hr = CoGetCallerTID(out uint threadId);
            _output.WriteLine(
                $"coinitialized(no active call) CoInitializeEx=0x{initHr:X8} " +
                $"CoGetCallerTID hr=0x{hr:X8} tid={threadId}");
            Assert.NotEqual(S_FALSE, hr);
        }
        finally
        {
            if (initHr >= 0)
            {
                CoUninitialize();
            }
        }
    }
}
