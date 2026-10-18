using System.Runtime.InteropServices;

namespace DeskBox.Helpers;

internal enum RecycleBinNativeOperation : uint
{
    Query = 1,
    Restore = 2
}

internal enum RecycleBinNativeCallFailure
{
    None,
    ModuleUnavailable,
    CapabilityUnavailable,
    MissingExport,
    InvalidInput,
    NativeFailure,
    InvalidNativeResult
}

internal sealed record RecycleBinNativeCallResult(
    RecycleBinNativeCallFailure Failure,    string Detail,    uint Status,    int OperationHResult,    uint AttemptedPhases,    int ComHResult,    int CreateHResult,    int NamespaceHResult,    int ItemsHResult,    int EnumerateHResult,    int ItemNameHResult,    int PropertyHResult,    int InvokeHResult,    uint MatchedCount,    uint RestoredCount)
{
    internal bool Success => Failure == RecycleBinNativeCallFailure.None;
}

internal sealed unsafe partial class ShortcutNativeModule
{
    internal const ulong RecycleBinCapability = 1UL << 8;

    private const uint RecycleBinStructVersion = 1;
    private const uint RecycleBinAttemptedPhasesMask = (1U << 8) - 1;
    private const int RecycleBinMaxInputChars = 32_767;
    private const string RecycleBinExport = "deskbox_recycle_bin_v1";

    internal RecycleBinNativeCallResult InvokeRecycleBin(
        RecycleBinNativeOperation operation,        string originalParent,        string originalName)
    {
        if ((Capabilities & RecycleBinCapability) == 0)
        {
            return RecycleBinCallFailure(
                RecycleBinNativeCallFailure.CapabilityUnavailable,                $"Native Recycle Bin capability 0x{RecycleBinCapability:X} is unavailable; module mask is 0x{Capabilities:X}.");
        }

        if (!Enum.IsDefined(operation) ||
            !IsValidRecycleBinInput(originalParent) ||
            !IsValidRecycleBinInput(originalName) ||
            Path.IsPathFullyQualified(originalName) ||
            originalName.Contains(Path.DirectorySeparatorChar) ||
            originalName.Contains(Path.AltDirectorySeparatorChar))
        {
            return RecycleBinCallFailure(
                RecycleBinNativeCallFailure.InvalidInput,                "Recycle Bin identity is empty, too long, contains an embedded NUL, or uses a rooted item name.");
        }

        if (!NativeLibrary.TryGetExport(
                _module,                RecycleBinExport,                out nint exportAddress))
        {
            return RecycleBinCallFailure(
                RecycleBinNativeCallFailure.MissingExport,                $"The DeskBox native export '{RecycleBinExport}' is missing.");
        }

        var recycleBinExport =
            (delegate* unmanaged[Cdecl]<NativeRecycleBinRequest*, NativeRecycleBinResult*, uint>)
            (void*)exportAddress;
        fixed (char* parentPointer = originalParent)
        fixed (char* namePointer = originalName)
        {
            var request = new NativeRecycleBinRequest
            {
                StructSize = (uint)sizeof(NativeRecycleBinRequest),
                StructVersion = RecycleBinStructVersion,
                Operation = (uint)operation,
                OriginalParent = new NativeUtf16String(
                    parentPointer,                    originalParent.Length),
                OriginalName = new NativeUtf16String(
                    namePointer,                    originalName.Length)
            };
            var result = new NativeRecycleBinResult
            {
                StructSize = (uint)sizeof(NativeRecycleBinResult),
                StructVersion = RecycleBinStructVersion
            };

            uint returnedStatus = recycleBinExport(&request,&result);
            if (result.StructSize != (uint)sizeof(NativeRecycleBinResult) ||
                result.StructVersion != RecycleBinStructVersion)
            {
                return FromRecycleBinResult(
                    RecycleBinNativeCallFailure.InvalidNativeResult,                    $"Native Recycle Bin result envelope mismatch: size={result.StructSize}, version={result.StructVersion}.",                    result);
            }

            bool restore = operation == RecycleBinNativeOperation.Restore;
            if (returnedStatus != result.Status ||
                (result.AttemptedPhases & ~RecycleBinAttemptedPhasesMask) != 0 ||
                result.OperationSucceeded > 1 ||
                result.Reserved0 != 0 ||
                result.Reserved1 != 0 ||
                result.Reserved2 != 0 ||
                result.Reserved3 != 0 ||
                result.Reserved4 != 0 ||
                result.Reserved5 != 0 ||
                result.RestoredCount > result.MatchedCount ||
                (result.Status == StatusOk) != (result.OperationSucceeded == 1) ||
                (!restore && result.RestoredCount != 0) ||
                (restore && result.Status == StatusOk &&
                    (result.MatchedCount != 1 || result.RestoredCount != 1)))
            {
                return FromRecycleBinResult(
                    RecycleBinNativeCallFailure.InvalidNativeResult,                    $"Native Recycle Bin result is inconsistent: return={returnedStatus}, result={result.Status}.",                    result);
            }

            if (result.Status != StatusOk)
            {
                return FromRecycleBinResult(
                    RecycleBinNativeCallFailure.NativeFailure,                    $"Native Recycle Bin operation failed: status={result.Status}, HRESULT=0x{result.OperationHResult:X8}, phases=0x{result.AttemptedPhases:X}.",                    result);
            }

            return FromRecycleBinResult(
                RecycleBinNativeCallFailure.None,                string.Empty,                result);
        }
    }

    private static bool IsValidRecycleBinInput(string? value)
    {
        return !string.IsNullOrEmpty(value) &&
            value.Length <= RecycleBinMaxInputChars &&
            !value.Contains('\0');
    }

    private static RecycleBinNativeCallResult RecycleBinCallFailure(
        RecycleBinNativeCallFailure failure,        string detail)
    {
        return new RecycleBinNativeCallResult(
            failure,            detail,            0,            HResultNotAttempted,            0,            HResultNotAttempted,            HResultNotAttempted,            HResultNotAttempted,            HResultNotAttempted,            HResultNotAttempted,            HResultNotAttempted,            HResultNotAttempted,            HResultNotAttempted,            0,            0);
    }

    private static RecycleBinNativeCallResult FromRecycleBinResult(
        RecycleBinNativeCallFailure failure,        string detail,        NativeRecycleBinResult result)
    {
        return new RecycleBinNativeCallResult(
            failure,            detail,            result.Status,            result.OperationHResult,            result.AttemptedPhases,            result.ComHResult,            result.CreateHResult,            result.NamespaceHResult,            result.ItemsHResult,            result.EnumerateHResult,            result.ItemNameHResult,            result.PropertyHResult,            result.InvokeHResult,            result.MatchedCount,            result.RestoredCount);
    }

    [StructLayout(LayoutKind.Sequential)]
    internal struct NativeRecycleBinRequest
    {
        internal uint StructSize;
        internal uint StructVersion;
        internal uint Operation;
        internal uint Flags;
        internal NativeUtf16String OriginalParent;
        internal NativeUtf16String OriginalName;
        internal ulong Reserved1;
        internal ulong Reserved2;
        internal ulong Reserved3;
        internal ulong Reserved4;
    }

    [StructLayout(LayoutKind.Sequential)]
    internal struct NativeRecycleBinResult
    {
        internal uint StructSize;
        internal uint StructVersion;
        internal uint Status;
        internal int OperationHResult;
        internal uint AttemptedPhases;
        internal int ComHResult;
        internal int CreateHResult;
        internal int NamespaceHResult;
        internal int ItemsHResult;
        internal int EnumerateHResult;
        internal int ItemNameHResult;
        internal int PropertyHResult;
        internal int InvokeHResult;
        internal uint MatchedCount;
        internal uint RestoredCount;
        internal uint OperationSucceeded;
        internal uint Reserved0;
        internal uint Reserved1;
        internal ulong Reserved2;
        internal ulong Reserved3;
        internal ulong Reserved4;
        internal ulong Reserved5;
    }
}

internal static class RecycleBinNativeBackend
{
    internal static RecycleBinNativeCallResult Invoke(
        RecycleBinNativeOperation operation,        string originalParent,        string originalName)
    {
        ShortcutNativeLoadResult load = ShortcutNativeModule.Default;
        if (!load.Success)
        {
            return new RecycleBinNativeCallResult(
                RecycleBinNativeCallFailure.ModuleUnavailable,                $"{load.Failure}: {load.Detail}",                0,                ShortcutNativeModule.HResultNotAttempted,                0,                ShortcutNativeModule.HResultNotAttempted,                ShortcutNativeModule.HResultNotAttempted,                ShortcutNativeModule.HResultNotAttempted,                ShortcutNativeModule.HResultNotAttempted,                ShortcutNativeModule.HResultNotAttempted,                ShortcutNativeModule.HResultNotAttempted,                ShortcutNativeModule.HResultNotAttempted,                ShortcutNativeModule.HResultNotAttempted,                0,                0);
        }

        return load.Module!.InvokeRecycleBin(
            operation,            originalParent,            originalName);
    }
}
