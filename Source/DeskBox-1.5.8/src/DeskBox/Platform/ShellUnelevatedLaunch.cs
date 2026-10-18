// Shared source file: compiled into DeskBox AND DeskBox.Updater (linked in
// DeskBox.Updater.csproj). Keep it self-contained — BCL and P/Invoke only,
// no app-project types. The few entry points duplicated from
// DragDropPermissionNativeMethods are deliberate: the updater build does not
// compile that file. The updater compiles this source under the
// DeskBox.UpdaterPlatform namespace so test code that references both
// assemblies never sees two internals-visible copies of the same full type
// name (CS0433).

using System.Runtime.InteropServices;
using System.Security.Principal;

#if DESKBOX_UPDATER
namespace DeskBox.UpdaterPlatform;
#else
namespace DeskBox.Platform;
#endif

/// <summary>
/// Starts a process at the shell's (standard-user) integrity by parenting it
/// to Explorer via PROC_THREAD_ATTRIBUTE_PARENT_PROCESS. The elevated-launch
/// gate uses it to hand DeskBox back to a Medium desktop, and the updater
/// uses it so a post-update restart does not inherit the installer's admin
/// token. After creation the child's process DACL is widened (SYSTEM,
/// Administrators, current user): a child spawned by an elevated creator
/// otherwise inherits a default DACL that only grants Administrators, which
/// standard-user tools (an unelevated Task Manager, taskkill) then cannot
/// terminate. The DACL is applied post-creation through the creator's own
/// handle — the CreateProcess call itself keeps the field-proven
/// IntPtr.Zero security-attributes shape.
/// </summary>
internal static class ShellUnelevatedLaunch
{
    private const uint ProcessCreateProcess = 0x0080;
    private const uint ProcessQueryLimitedInformation = 0x1000;
    private const uint TokenQuery = 0x0008;
    private const uint ExtendedStartupInfoPresent = 0x00080000;
    private static readonly IntPtr ProcThreadAttributeParentProcess = new(0x00020000);

    [DllImport("kernel32.dll")]
    private static extern IntPtr GetCurrentProcess();

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern IntPtr OpenProcess(
        uint desiredAccess,        [MarshalAs(UnmanagedType.Bool)] bool inheritHandle,        uint processId);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool CloseHandle(IntPtr handle);

    [DllImport("advapi32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool OpenProcessToken(
        IntPtr processHandle,        uint desiredAccess,        out IntPtr tokenHandle);

    [DllImport("advapi32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool GetTokenInformation(
        IntPtr tokenHandle,        int tokenInformationClass,        IntPtr tokenInformation,        int tokenInformationLength,        out int returnLength);

    [DllImport("user32.dll")]
    private static extern IntPtr GetShellWindow();

    [DllImport("user32.dll")]
    private static extern uint GetWindowThreadProcessId(IntPtr hWnd,out uint processId);

    [DllImport("advapi32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool ConvertStringSecurityDescriptorToSecurityDescriptor(
        string stringSecurityDescriptor,        uint stringSDRevision,        out IntPtr securityDescriptor,        IntPtr securityDescriptorSize);

    [DllImport("kernel32.dll")]
    private static extern void LocalFree(IntPtr hMem);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool InitializeProcThreadAttributeList(
        IntPtr lpAttributeList,        int dwAttributeCount,        int dwFlags,        ref int lpSize);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool UpdateProcThreadAttribute(
        IntPtr lpAttributeList,        uint dwFlags,        IntPtr attribute,        ref IntPtr lpValue,        IntPtr cbSize,        IntPtr lpPreviousValue,        IntPtr lpReturnSize);

    [DllImport("kernel32.dll")]
    private static extern void DeleteProcThreadAttributeList(IntPtr lpAttributeList);

    [DllImport("advapi32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool SetKernelObjectSecurity(
        IntPtr handle,        uint securityInformation,        IntPtr securityDescriptor);

    [DllImport("kernel32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool CreateProcess(
        string? lpApplicationName,        string lpCommandLine,        IntPtr lpProcessAttributes,        IntPtr lpThreadAttributes,        [MarshalAs(UnmanagedType.Bool)] bool bInheritHandles,        uint dwCreationFlags,        IntPtr lpEnvironment,        string? lpCurrentDirectory,        ref StartupInfo lpStartupInfo,        out ProcessInformation lpProcessInformation);

    [StructLayout(LayoutKind.Sequential)]
    private struct TokenElevation
    {
        public int TokenIsElevated;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct StartupInfo
    {
        public int cb;
        public IntPtr lpReserved;
        public IntPtr lpDesktop;
        public IntPtr lpTitle;
        public int dwX;
        public int dwY;
        public int dwXSize;
        public int dwYSize;
        public int dwXCountChars;
        public int dwYCountChars;
        public int dwFillAttribute;
        public int dwFlags;
        public short wShowWindow;
        public short cbReserved2;
        public IntPtr lpReserved2;
        public IntPtr hStdInput;
        public IntPtr hStdOutput;
        public IntPtr hStdError;
        public IntPtr lpAttributeList;
    }

    [StructLayout(LayoutKind.Sequential)]
    private struct ProcessInformation
    {
        public IntPtr hProcess;
        public IntPtr hThread;
        public int dwProcessId;
        public int dwThreadId;
    }

    public static bool IsCurrentProcessElevated()
    {
        try
        {
            using var identity = WindowsIdentity.GetCurrent();
            return new WindowsPrincipal(identity).IsInRole(WindowsBuiltInRole.Administrator);
        }
        catch
        {
            return false;
        }
    }

    /// <summary>
    /// Elevation of the shell (Explorer) process, or null when the shell
    /// cannot be probed (mid-restart, or no shell window exists). Callers
    /// pick their own conservative fallback for the null case.
    /// </summary>
    public static bool? IsShellWindowElevated()
    {
        IntPtr shellWindow = GetShellWindow();
        if (shellWindow == IntPtr.Zero)
        {
            return null;
        }

        _ = GetWindowThreadProcessId(shellWindow,out uint explorerProcessId);
        if (explorerProcessId == 0)
        {
            return null;
        }

        IntPtr processHandle = OpenProcess(
            ProcessCreateProcess | ProcessQueryLimitedInformation,            false,            explorerProcessId);
        if (processHandle == IntPtr.Zero)
        {
            return null;
        }

        try
        {
            if (!OpenProcessToken(processHandle,TokenQuery,out IntPtr tokenHandle))
            {
                return null;
            }

            try
            {
                if (!GetTokenInformation(
                        tokenHandle,                        20 /* TokenElevation */,                        IntPtr.Zero,                        0,                        out int length) &&
                    Marshal.GetLastWin32Error() != 122 /* ERROR_INSUFFICIENT_BUFFER */)
                {
                    return null;
                }

                IntPtr buffer = Marshal.AllocHGlobal(Math.Max(length,sizeof(int)));
                try
                {
                    if (!GetTokenInformation(tokenHandle,20,buffer,length,out _))
                    {
                        return null;
                    }

                    return Marshal.PtrToStructure<TokenElevation>(buffer).TokenIsElevated != 0;
                }
                finally
                {
                    Marshal.FreeHGlobal(buffer);
                }
            }
            finally
            {
                CloseHandle(tokenHandle);
            }
        }
        finally
        {
            CloseHandle(processHandle);
        }
    }

    /// <summary>
    /// The restart-level guard shared by the updater: hand the app to the
    /// shell's level only when the caller is elevated AND the shell really
    /// offers a standard-user context. On a fully elevated desktop (company
    /// policy) this returns false so the restart keeps inheriting — matching
    /// the shell is what keeps tray creation and drag/drop working there;
    /// forcing Medium would manufacture the ShellAboveApp breakage. A failed
    /// shell probe also returns false: the historical inherit-restart is the
    /// safe fallback, and the app-side launch gate still offers its one-click
    /// fix if the restart lands elevated.
    /// </summary>
    public static bool ShouldRestartAtShellIntegrity() =>
        DecideRestartAtShellIntegrity(IsCurrentProcessElevated(),IsShellWindowElevated());

    /// <summary>
    /// Pure form of the restart guard, so the safety property itself stays
    /// pinned by contract tests: the hand-down engages only for an elevated
    /// caller probing a genuinely unelevated shell.
    /// </summary>
    internal static bool DecideRestartAtShellIntegrity(bool callerElevated,bool? shellElevated) =>
        callerElevated && shellElevated == false;

    /// <summary>
    /// Creates the process parented to Explorer, so it runs with Explorer's
    /// (standard-user) token even though this caller is elevated. Returns
    /// false without throwing on any failure; callers fall back to their
    /// historical restart path.
    /// </summary>
    public static bool TryStartAtShellIntegrity(
        string exePath,        string? arguments,        Action<string>? log)
    {
        if (!File.Exists(exePath))
        {
            return false;
        }

        IntPtr shellWindow = GetShellWindow();
        if (shellWindow == IntPtr.Zero)
        {
            log?.Invoke("[ShellUnelevatedLaunch] No shell window; cannot parent to Explorer");
            return false;
        }

        _ = GetWindowThreadProcessId(shellWindow,out uint explorerProcessId);
        if (explorerProcessId == 0)
        {
            log?.Invoke("[ShellUnelevatedLaunch] Shell window has no process id");
            return false;
        }

        IntPtr explorerHandle = OpenProcess(
            ProcessCreateProcess | ProcessQueryLimitedInformation,            false,            explorerProcessId);
        if (explorerHandle == IntPtr.Zero)
        {
            log?.Invoke($"[ShellUnelevatedLaunch] OpenProcess(explorer) failed error={Marshal.GetLastWin32Error()}");
            return false;
        }

        try
        {
            int attributeListSize = 0;
            _ = InitializeProcThreadAttributeList(IntPtr.Zero,1,0,ref attributeListSize);
            IntPtr attributeList = Marshal.AllocHGlobal(attributeListSize);
            try
            {
                if (!InitializeProcThreadAttributeList(attributeList,1,0,ref attributeListSize))
                {
                    log?.Invoke("[ShellUnelevatedLaunch] InitializeProcThreadAttributeList failed");
                    return false;
                }

                IntPtr parentProcessValue = explorerHandle;
                if (!UpdateProcThreadAttribute(
                        attributeList,                        0,                        ProcThreadAttributeParentProcess,                        ref parentProcessValue,                        new IntPtr(IntPtr.Size),                        IntPtr.Zero,                        IntPtr.Zero))
                {
                    log?.Invoke("[ShellUnelevatedLaunch] UpdateProcThreadAttribute failed");
                    return false;
                }

                // The permissive security is applied AFTER creation, via the
                // creator's own full-access handle: an explicit
                // SECURITY_ATTRIBUTES at CreateProcess time is the least
                // exercised path and correlated with a child whose native
                // drag loop never received target events (2026-10-14 field
                // report), while the IntPtr.Zero shape is the field-proven
                // one.
                //
                // DACL: SYSTEM, Administrators and the current user each get
                // full access — without it the child inherits the elevated
                // creator's default DACL (Administrators only) and a
                // standard user cannot open the relaunched process at all.
                //
                // Mandatory label: the elevated creator stamps a High label
                // on the process object, and no-write-up then denies
                // standard-user terminate-class access even though the DACL
                // grants it (field observation 2026-10-14: taskkill denied
                // while a raw PROCESS_TERMINATE open succeeded). Setting the
                // label needs WRITE_OWNER, which the creator handle carries.
                // Normalizing it to the child's Medium integrity makes the
                // object indistinguishable from an explorer-launched process.
                string? userSid;
                try
                {
                    userSid = WindowsIdentity.GetCurrent().User?.Value;
                }
                catch
                {
                    userSid = null;
                }

                string sddl = "D:P(A;;GA;;;SY)(A;;GA;;;BA)" +
                    (userSid is null ? string.Empty : $"(A;;GA;;;{userSid})") +
                    "S:(ML;;NW;;;S-1-16-8192)";

                if (!ConvertStringSecurityDescriptorToSecurityDescriptor(
                        sddl,                        1 /* SDDL_REVISION_1 */,                        out IntPtr securityDescriptor,                        IntPtr.Zero))
                {
                    log?.Invoke("[ShellUnelevatedLaunch] SDDL conversion failed");
                    return false;
                }

                try
                {
                    var startupInfo = new StartupInfo
                    {
                        cb = Marshal.SizeOf<StartupInfo>()
                    };
                    startupInfo.lpAttributeList = attributeList;

                    string commandLine = string.IsNullOrWhiteSpace(arguments)
                        ? $"\"{exePath}\""
                        : $"\"{exePath}\" {arguments.Trim()}";

                    bool created = CreateProcess(
                        null,                        commandLine,                        IntPtr.Zero,                        IntPtr.Zero,                        false,                        ExtendedStartupInfoPresent,                        IntPtr.Zero,                        Path.GetDirectoryName(exePath),                        ref startupInfo,                        out ProcessInformation processInformation);
                    if (!created)
                    {
                        log?.Invoke($"[ShellUnelevatedLaunch] CreateProcess failed error={Marshal.GetLastWin32Error()}");
                        return false;
                    }

                    if (!SetKernelObjectSecurity(
                            processInformation.hProcess,                            4 | 16 /* DACL | LABEL_SECURITY_INFORMATION */,                            securityDescriptor))
                    {
                        // The kill-access fix is best-effort: the child still
                        // runs; only its terminability from a standard user
                        // session is unchanged from the historical behavior.
                        log?.Invoke(
                            $"[ShellUnelevatedLaunch] Post-create security failed error={Marshal.GetLastWin32Error()}");
                    }

                    CloseHandle(processInformation.hThread);
                    CloseHandle(processInformation.hProcess);
                    return true;
                }
                finally
                {
                    LocalFree(securityDescriptor);
                }
            }
            finally
            {
                DeleteProcThreadAttributeList(attributeList);
                Marshal.FreeHGlobal(attributeList);
            }
        }
        finally
        {
            CloseHandle(explorerHandle);
        }
    }
}
