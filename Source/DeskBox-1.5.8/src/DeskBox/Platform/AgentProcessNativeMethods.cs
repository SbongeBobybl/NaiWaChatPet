using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;

namespace DeskBox.Platform;

/// <summary>
/// kernel32/advapi32 entry points used to verify that a local named-pipe server
/// really is the agent process the handshake claims: pipe-to-process, process
/// to Windows session, then token user. Signatures moved verbatim from the AI
/// organize client so P/Invoke stays inside the Platform domain.
/// </summary>
internal static partial class AgentProcessNativeMethods
{
    [LibraryImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    internal static partial bool GetNamedPipeServerProcessId(SafePipeHandle pipe,out uint pid);

    [LibraryImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    internal static partial bool ProcessIdToSessionId(uint pid,out uint session);

    [LibraryImport("kernel32.dll", SetLastError = true)]
    internal static partial SafeProcessHandle OpenProcess(uint access,[MarshalAs(UnmanagedType.Bool)] bool inherit,uint pid);

    [LibraryImport("advapi32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    internal static partial bool OpenProcessToken(SafeProcessHandle process,uint access,out SafeAccessTokenHandle token);
}
