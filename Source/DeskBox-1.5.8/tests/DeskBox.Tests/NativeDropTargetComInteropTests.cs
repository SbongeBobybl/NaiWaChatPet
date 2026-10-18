using System.Runtime.InteropServices;
using DeskBox.Helpers;

namespace DeskBox.Tests;

public sealed class NativeDropTargetComInteropTests
{
    private static readonly Guid DropTargetInterfaceId =
        new("00000122-0000-0000-C000-000000000046");

    [Fact]
    public void PointLayout_MatchesTheOlePointlAbi()
    {
        Assert.Equal(8, Marshal.SizeOf<NativeDropTarget.POINT>());
        Assert.Equal(
            0,
            Marshal.OffsetOf<NativeDropTarget.POINT>(nameof(NativeDropTarget.POINT.X))
                .ToInt32());
        Assert.Equal(
            4,
            Marshal.OffsetOf<NativeDropTarget.POINT>(nameof(NativeDropTarget.POINT.Y))
                .ToInt32());
    }

    [Fact]
    public void GeneratedCcw_ExposesIdropTargetWithFourCallableSlots()
    {
        using var owner = new NativeDropTarget(0);
        var target = new NativeDropTargetComObject(owner);
        nint pointer = NativeDropTargetComInterop.AcquireInterfacePointer(target);
        nint queriedPointer = 0;
        try
        {
            Guid iid = DropTargetInterfaceId;
            int result = Marshal.QueryInterface(pointer, in iid, out queriedPointer);

            Assert.Equal(0, result);
            Assert.NotEqual(0, queriedPointer);
            nint vtable = Marshal.ReadIntPtr(queriedPointer);
            Assert.NotEqual(0, vtable);
            for (int slot = 0; slot <= 6; slot++)
            {
                Assert.NotEqual(0, Marshal.ReadIntPtr(vtable, slot * IntPtr.Size));
            }
        }
        finally
        {
            if (queriedPointer != 0)
            {
                Marshal.Release(queriedPointer);
            }

            NativeDropTargetComInterop.ReleaseInterfacePointer(pointer);
            GC.KeepAlive(target);
        }
    }

    [Fact]
    public void GeneratedCcw_ForwardsCallbacksAndPreservesEffectPolicy()
    {
        using var owner = new NativeDropTarget(0);
        var target = new NativeDropTargetComObject(owner);
        var events = new List<string>();
        owner.DragEnterEvent += (x, y, hasFiles) =>
            events.Add($"enter:{x}:{y}:{hasFiles}");
        owner.DragOverEvent += (x, y) => events.Add($"over:{x}:{y}");
        owner.DragLeaveEvent += () => events.Add("leave");
        owner.DropEvent += (_, _, _, _, _) => events.Add("drop");

        nint pointer = NativeDropTargetComInterop.AcquireInterfacePointer(target);
        try
        {
            nint vtable = Marshal.ReadIntPtr(pointer);
            DragEnterCallback dragEnter = GetSlot<DragEnterCallback>(vtable, 3);
            DragOverCallback dragOver = GetSlot<DragOverCallback>(vtable, 4);
            DragLeaveCallback dragLeave = GetSlot<DragLeaveCallback>(vtable, 5);
            DropCallback drop = GetSlot<DropCallback>(vtable, 6);
            var point = new NativeDropTarget.POINT { X = 41, Y = 73 };

            uint effect = NativeDropEffectPolicy.Copy | NativeDropEffectPolicy.Move;
            Assert.Equal(0, dragEnter(pointer, 0, 0, point, ref effect));
            Assert.Equal(NativeDropEffectPolicy.None, effect);

            effect = NativeDropEffectPolicy.Copy | NativeDropEffectPolicy.Move;
            Assert.Equal(0, dragOver(pointer, 0, point, ref effect));
            Assert.Equal(NativeDropEffectPolicy.None, effect);

            Assert.Equal(0, dragLeave(pointer));

            effect = NativeDropEffectPolicy.Copy | NativeDropEffectPolicy.Move;
            Assert.Equal(0, drop(pointer, 0, 0, point, ref effect));
            Assert.Equal(NativeDropEffectPolicy.None, effect);

            Assert.Equal(
                ["enter:41:73:False", "over:41:73", "leave"],
                events);
            Assert.False(owner.HasFileData);
            Assert.False(owner.HasVirtualFileData);
        }
        finally
        {
            NativeDropTargetComInterop.ReleaseInterfacePointer(pointer);
            GC.KeepAlive(target);
        }
    }

    [Fact]
    public void LocalPointerCanBeReleasedWhileAnOleStyleReferenceRemains()
    {
        using var owner = new NativeDropTarget(0);
        var target = new NativeDropTargetComObject(owner);
        bool dragLeaveRaised = false;
        owner.DragLeaveEvent += () => dragLeaveRaised = true;

        nint pointer = NativeDropTargetComInterop.AcquireInterfacePointer(target);
        Marshal.AddRef(pointer); // RegisterDragDrop owns this simulated reference.
        NativeDropTargetComInterop.ReleaseInterfacePointer(pointer);
        try
        {
            nint vtable = Marshal.ReadIntPtr(pointer);
            DragLeaveCallback dragLeave = GetSlot<DragLeaveCallback>(vtable, 5);

            Assert.Equal(0, dragLeave(pointer));
            Assert.True(dragLeaveRaised);
        }
        finally
        {
            Marshal.Release(pointer); // RevokeDragDrop releases the OLE reference.
            GC.KeepAlive(target);
        }
    }

    [Fact]
    public void PointerHelpers_RejectNullTargetsAndAcceptNullPointersForRelease()
    {
        Assert.Throws<ArgumentNullException>(
            () => NativeDropTargetComInterop.AcquireInterfacePointer(null!));

        NativeDropTargetComInterop.ReleaseInterfacePointer(0);
    }

    private static TDelegate GetSlot<TDelegate>(nint vtable, int slot)
        where TDelegate : Delegate
    {
        nint functionPointer = Marshal.ReadIntPtr(vtable, slot * IntPtr.Size);
        return Marshal.GetDelegateForFunctionPointer<TDelegate>(functionPointer);
    }

    [UnmanagedFunctionPointer(CallingConvention.StdCall)]
    private delegate int DragEnterCallback(
        nint self,
        nint dataObject,
        uint keyState,
        NativeDropTarget.POINT point,
        ref uint effect);

    [UnmanagedFunctionPointer(CallingConvention.StdCall)]
    private delegate int DragOverCallback(
        nint self,
        uint keyState,
        NativeDropTarget.POINT point,
        ref uint effect);

    [UnmanagedFunctionPointer(CallingConvention.StdCall)]
    private delegate int DragLeaveCallback(nint self);

    [UnmanagedFunctionPointer(CallingConvention.StdCall)]
    private delegate int DropCallback(
        nint self,
        nint dataObject,
        uint keyState,
        NativeDropTarget.POINT point,
        ref uint effect);
}
