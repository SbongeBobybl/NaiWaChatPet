// Copyright (c) DeskBox. All rights reserved.

using System.Runtime.InteropServices;

namespace DeskBox.Platform;

/// <summary>
/// Hand-rolled IFileOpenDialog wrapper (direct vtable calls, RCW-free, so it
/// works under Native AOT like the IFileOperation interop).
/// </summary>
/// <remarks>
/// The WinAppSDK picker exposes no FOS_NODEREFERENCELINKS switch, and the
/// shell file dialog follows shortcuts by default: picking a .lnk silently
/// returns the target's path, which turned "add file" into a move of the
/// target entity (issue 458). This dialog pins the no-dereference option so
/// a selected shortcut is added as the .lnk itself, matching Explorer's own
/// picker semantics.
/// </remarks>
internal static unsafe class NativeFileOpenDialog
{
    private static readonly Guid FileOpenDialogClassId =
        new("DC1C5A9C-E88A-4dde-A5A1-60F82A20AEF7");
    private static readonly Guid FileOpenDialogInterfaceId =
        new("D57C7288-D4AD-4768-BE02-9D969532D960");
    private static readonly Guid ShellItemInterfaceId =
        new("43826D1E-E718-42EE-BC55-A1E261C37BFE");

    private const uint FosForceFileSystem = 0x00000040;
    private const uint FosAllowMultiSelect = 0x00000200;
    private const uint FosPathMustExist = 0x00000800;
    private const uint FosFileMustExist = 0x00001000;
    private const uint FosNoDereferenceLinks = 0x00100000;
    private const uint SigdnFileSysPath = 0x80058000;
    private const int HResultCancelled = unchecked((int)0x800704C5);

    [DllImport("ole32.dll")]
    private static extern int CoCreateInstance(
        ref Guid classId,        IntPtr outer,        uint context,        ref Guid interfaceId,        out IntPtr instance);

    [DllImport("shell32.dll", CharSet = CharSet.Unicode, EntryPoint = "SHCreateItemFromParsingName")]
    private static extern int CreateShellItemFromParsingName(
        string path,        IntPtr bindContext,        ref Guid interfaceId,        out IntPtr shellItem);

    [DllImport("ole32.dll")]
    private static extern void CoTaskMemFree(IntPtr block);

    /// <summary>
    /// Shows the modal multi-select open dialog on the caller's STA thread
    /// (the owner window's UI thread). Returns the picked file-system
    /// paths; cancelling returns an empty list.
    /// </summary>
    public static IReadOnlyList<string> ShowPickFiles(
        IntPtr ownerHwnd,        string? defaultFolder)
    {
        Guid classId = FileOpenDialogClassId;
        Guid interfaceId = FileOpenDialogInterfaceId;
        int hr = CoCreateInstance(
            ref classId,            IntPtr.Zero,            1,            ref interfaceId,            out IntPtr dialogPointer);
        if (hr != 0)
        {
            Marshal.ThrowExceptionForHR(hr);
        }

        using var dialog = new FileOpenDialogNative(dialogPointer);
        uint options = FosForceFileSystem |
            FosAllowMultiSelect |
            FosPathMustExist |
            FosFileMustExist |
            FosNoDereferenceLinks;
        Marshal.ThrowExceptionForHR(dialog.SetOptions(options));

        if (!string.IsNullOrWhiteSpace(defaultFolder))
        {
            Guid shellItemId = ShellItemInterfaceId;
            hr = CreateShellItemFromParsingName(
                defaultFolder,                IntPtr.Zero,                ref shellItemId,                out IntPtr folderItem);
            if (hr == 0 && folderItem != IntPtr.Zero)
            {
                dialog.SetDefaultFolder(folderItem);
                Marshal.Release(folderItem);
            }
            // A suggested folder that cannot be shelved (recently deleted,
            // detached drive) falls back to the dialog default location.
        }

        hr = dialog.Show(ownerHwnd);
        if (hr == HResultCancelled)
        {
            return [];
        }

        Marshal.ThrowExceptionForHR(hr);
        Marshal.ThrowExceptionForHR(dialog.GetResults(out IntPtr resultsPointer));
        using var results = new ShellItemArrayNative(resultsPointer);

        Marshal.ThrowExceptionForHR(results.GetCount(out uint count));
        var paths = new List<string>(capacity: (int)count);
        for (uint index = 0; index < count; index++)
        {
            hr = results.GetItemAt(index,out IntPtr item);
            if (hr != 0)
            {
                Marshal.ThrowExceptionForHR(hr);
            }

            hr = ShellItemNative.GetDisplayName(
                item,                SigdnFileSysPath,                out IntPtr name);
            Marshal.Release(item);
            if (hr != 0 || name == IntPtr.Zero)
            {
                continue;
            }

            try
            {
                if (Marshal.PtrToStringUni(name) is { Length: > 0 } path)
                {
                    paths.Add(path);
                }
            }
            finally
            {
                CoTaskMemFree(name);
            }
        }

        return paths;
    }

    /// <summary>An owned native wrapper avoids RCWs and works in Native AOT too.</summary>
    private sealed unsafe class FileOpenDialogNative(IntPtr pointer) : IDisposable
    {
        private void** Table => *(void***)pointer;

        public int Show(IntPtr owner) =>
            ((delegate* unmanaged[Stdcall]<IntPtr, IntPtr, int>)Table[3])(pointer,owner);

        public int SetOptions(uint options) =>
            ((delegate* unmanaged[Stdcall]<IntPtr, uint, int>)Table[9])(pointer,options);

        public int SetDefaultFolder(IntPtr folder) =>
            ((delegate* unmanaged[Stdcall]<IntPtr, IntPtr, int>)Table[11])(pointer,folder);

        public int GetResults(out IntPtr results)
        {
            fixed (IntPtr* value = &results)
            {
                return ((delegate* unmanaged[Stdcall]<IntPtr, IntPtr*, int>)Table[27])(
                    pointer,                    value);
            }
        }

        public void Dispose() => Marshal.Release(pointer);
    }

    private sealed unsafe class ShellItemArrayNative(IntPtr pointer) : IDisposable
    {
        private void** Table => *(void***)pointer;

        public int GetCount(out uint count)
        {
            fixed (uint* value = &count)
            {
                return ((delegate* unmanaged[Stdcall]<IntPtr, uint*, int>)Table[7])(
                    pointer,                    value);
            }
        }

        public int GetItemAt(uint index,out IntPtr item)
        {
            fixed (IntPtr* value = &item)
            {
                return ((delegate* unmanaged[Stdcall]<IntPtr, uint, IntPtr*, int>)Table[8])(
                    pointer,                    index,                    value);
            }
        }

        public void Dispose() => Marshal.Release(pointer);
    }

    private static class ShellItemNative
    {
        public static int GetDisplayName(IntPtr item,uint kind,out IntPtr name)
        {
            fixed (IntPtr* value = &name)
            {
                return ((delegate* unmanaged[Stdcall]<IntPtr, uint, IntPtr*, int>)(
                    *(void***)item)[5])(item,kind,value);
            }
        }
    }
}
