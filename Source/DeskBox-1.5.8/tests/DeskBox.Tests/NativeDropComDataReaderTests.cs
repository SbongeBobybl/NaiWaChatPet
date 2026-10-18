using System.Runtime.InteropServices;
using DeskBox.Helpers;

namespace DeskBox.Tests;

public sealed class NativeDropComDataReaderTests
{
    [Fact]
    public void OleStructLayouts_MatchTheWin64Abi()
    {
        Assert.Equal(32, Marshal.SizeOf<NativeFormatEtc>());
        Assert.Equal(
            0,
            Marshal.OffsetOf<NativeFormatEtc>(nameof(NativeFormatEtc.ClipboardFormat)).ToInt32());
        Assert.Equal(
            8,
            Marshal.OffsetOf<NativeFormatEtc>(nameof(NativeFormatEtc.TargetDevice)).ToInt32());
        Assert.Equal(
            16,
            Marshal.OffsetOf<NativeFormatEtc>(nameof(NativeFormatEtc.Aspect)).ToInt32());
        Assert.Equal(
            20,
            Marshal.OffsetOf<NativeFormatEtc>(nameof(NativeFormatEtc.Index)).ToInt32());
        Assert.Equal(
            24,
            Marshal.OffsetOf<NativeFormatEtc>(nameof(NativeFormatEtc.MediumType)).ToInt32());

        Assert.Equal(24, Marshal.SizeOf<NativeStorageMedium>());
        Assert.Equal(
            0,
            Marshal.OffsetOf<NativeStorageMedium>(nameof(NativeStorageMedium.MediumType)).ToInt32());
        Assert.Equal(
            8,
            Marshal.OffsetOf<NativeStorageMedium>(nameof(NativeStorageMedium.Content)).ToInt32());
        Assert.Equal(
            16,
            Marshal.OffsetOf<NativeStorageMedium>(nameof(NativeStorageMedium.ReleaseUnknown)).ToInt32());
    }

    [Fact]
    public void DataObject_GetDataUsesSlotThreeAndReturnsTheNativeMedium()
    {
        using var fake = new FakeComObject(slotCount: 6);
        NativeFormatEtc capturedFormat = default;
        nint capturedSelf = 0;
        var expectedMedium = new NativeStorageMedium
        {
            MediumType = 4,
            Content = (nint)0x1234,
            ReleaseUnknown = (nint)0x5678,
        };
        GetDataCallback callback = (
            nint self,
            ref NativeFormatEtc format,
            out NativeStorageMedium medium) =>
        {
            capturedSelf = self;
            capturedFormat = format;
            medium = expectedMedium;
            return 0;
        };
        fake.SetSlot(3, callback);

        var format = new NativeFormatEtc
        {
            ClipboardFormat = 15,
            TargetDevice = (nint)0x42,
            Aspect = 1,
            Index = -1,
            MediumType = 5,
        };
        var dataObject = new NativeOleDataObject(fake.Pointer);

        int result = dataObject.GetData(ref format, out NativeStorageMedium medium);

        Assert.Equal(0, result);
        Assert.Equal(fake.Pointer, capturedSelf);
        Assert.Equal(format.ClipboardFormat, capturedFormat.ClipboardFormat);
        Assert.Equal(format.TargetDevice, capturedFormat.TargetDevice);
        Assert.Equal(format.Aspect, capturedFormat.Aspect);
        Assert.Equal(format.Index, capturedFormat.Index);
        Assert.Equal(format.MediumType, capturedFormat.MediumType);
        Assert.Equal(expectedMedium.MediumType, medium.MediumType);
        Assert.Equal(expectedMedium.Content, medium.Content);
        Assert.Equal(expectedMedium.ReleaseUnknown, medium.ReleaseUnknown);
    }

    [Fact]
    public void DataObject_QueryGetDataUsesSlotFiveAndPreservesHresult()
    {
        const int expectedResult = unchecked((int)0x80040064); // DV_E_FORMATETC
        using var fake = new FakeComObject(slotCount: 6);
        NativeFormatEtc capturedFormat = default;
        QueryGetDataCallback callback = (nint _, ref NativeFormatEtc format) =>
        {
            capturedFormat = format;
            return expectedResult;
        };
        fake.SetSlot(5, callback);

        var format = new NativeFormatEtc
        {
            ClipboardFormat = 49152,
            Aspect = 1,
            Index = 7,
            MediumType = 1,
        };
        var dataObject = new NativeOleDataObject(fake.Pointer);

        int result = dataObject.QueryGetData(ref format);

        Assert.Equal(expectedResult, result);
        Assert.Equal(format.ClipboardFormat, capturedFormat.ClipboardFormat);
        Assert.Equal(format.Index, capturedFormat.Index);
        Assert.Equal(format.MediumType, capturedFormat.MediumType);
    }

    [Fact]
    public void DataObject_SetDataUsesSlotSevenAndTransfersTheNativeMedium()
    {
        using var fake = new FakeComObject(slotCount: 8);
        NativeFormatEtc capturedFormat = default;
        NativeStorageMedium capturedMedium = default;
        int capturedRelease = 0;
        SetDataCallback callback = (
            nint _,
            ref NativeFormatEtc format,
            ref NativeStorageMedium medium,
            int release) =>
        {
            capturedFormat = format;
            capturedMedium = medium;
            capturedRelease = release;
            return 0;
        };
        fake.SetSlot(7, callback);

        var format = new NativeFormatEtc
        {
            ClipboardFormat = 49153,
            Aspect = 1,
            Index = -1,
            MediumType = 1,
        };
        var medium = new NativeStorageMedium
        {
            MediumType = 1,
            Content = (nint)0x1234,
            ReleaseUnknown = 0,
        };
        var dataObject = new NativeOleDataObject(fake.Pointer);

        int result = dataObject.SetData(
            ref format,
            ref medium,
            release: true);

        Assert.Equal(0, result);
        Assert.Equal(format.ClipboardFormat, capturedFormat.ClipboardFormat);
        Assert.Equal(format.Index, capturedFormat.Index);
        Assert.Equal(medium.Content, capturedMedium.Content);
        Assert.Equal(1, capturedRelease);
    }

    [Fact]
    public void StreamReader_CopiesChunksAndStopsOnSFalse()
    {
        byte[] payload = "native virtual drop"u8.ToArray();
        int offset = 0;
        int calls = 0;
        using var fake = new FakeComObject(slotCount: 4);
        ReadCallback callback = (nint _, nint buffer, uint bufferSize, nint bytesRead) =>
        {
            calls++;
            int count = Math.Min(4, payload.Length - offset);
            if (bufferSize < count)
            {
                return unchecked((int)0x8007007A); // HRESULT_FROM_WIN32(ERROR_INSUFFICIENT_BUFFER)
            }
            if (count > 0)
            {
                Marshal.Copy(payload, offset, buffer, count);
                offset += count;
            }

            Marshal.WriteInt32(bytesRead, count);
            return offset == payload.Length ? 1 : 0;
        };
        fake.SetSlot(3, callback);
        using var destination = new MemoryStream();

        NativeComStreamReader.CopyTo(fake.Pointer, destination);

        Assert.Equal(payload, destination.ToArray());
        Assert.Equal(5, calls);
    }

    [Fact]
    public void StreamReader_ThrowsForFailedHresult()
    {
        const int expectedResult = unchecked((int)0x80004005);
        using var fake = new FakeComObject(slotCount: 4);
        ReadCallback callback = (nint _, nint _, uint _, nint bytesRead) =>
        {
            Marshal.WriteInt32(bytesRead, 0);
            return expectedResult;
        };
        fake.SetSlot(3, callback);
        using var destination = new MemoryStream();

        COMException exception = Assert.Throws<COMException>(
            () => NativeComStreamReader.CopyTo(fake.Pointer, destination));

        Assert.Equal(expectedResult, exception.HResult);
    }

    [Fact]
    public void StreamReader_RejectsAnOverreportedByteCount()
    {
        using var fake = new FakeComObject(slotCount: 4);
        ReadCallback callback = (nint _, nint _, uint bufferSize, nint bytesRead) =>
        {
            Marshal.WriteInt32(bytesRead, checked((int)bufferSize + 1));
            return 0;
        };
        fake.SetSlot(3, callback);
        using var destination = new MemoryStream();

        IOException exception = Assert.Throws<IOException>(
            () => NativeComStreamReader.CopyTo(fake.Pointer, destination));

        Assert.Contains("more bytes", exception.Message, StringComparison.Ordinal);
    }

    [Fact]
    public void BorrowedPointerBoundariesRejectNullPointers()
    {
        Assert.Throws<ArgumentException>(() => new NativeOleDataObject(0));
        Assert.Throws<ArgumentException>(
            () => NativeComStreamReader.CopyTo(0, new MemoryStream()));
    }

    [Theory]
    [InlineData(
        "OpenAI.Codex_2p2nqsd0c76g0!App",
        "OpenAI.Codex_2p2nqsd0c76g0!App")]
    [InlineData(
        "shell:AppsFolder\\Microsoft.WindowsStore_8wekyb3d8bbwe!App",
        "Microsoft.WindowsStore_8wekyb3d8bbwe!App")]
    [InlineData(
        "shell:::{4234d49b-0245-4df3-b780-3893943456e1}\\Microsoft.WindowsStore_8wekyb3d8bbwe!App",
        "Microsoft.WindowsStore_8wekyb3d8bbwe!App")]
    public void PackagedApplicationIds_AcceptAppsFolderParsingNames(
        string parsingName,
        string expected)
    {
        Assert.True(
            NativeDropTarget.TryNormalizePackagedApplicationId(
                parsingName,
                out string actual));
        Assert.Equal(expected, actual);
    }

    [Theory]
    [InlineData("")]
    [InlineData("C:\\Program Files\\App.exe")]
    [InlineData("shell:RecycleBinFolder")]
    [InlineData("PackageWithoutApplicationSeparator")]
    [InlineData("Package!App\\child")]
    [InlineData("Package!App with spaces")]
    public void PackagedApplicationIds_RejectFilesystemAndOtherShellObjects(
        string parsingName)
    {
        Assert.False(
            NativeDropTarget.TryNormalizePackagedApplicationId(
                parsingName,
                out string actual));
        Assert.Empty(actual);
    }

    [UnmanagedFunctionPointer(CallingConvention.StdCall)]
    private delegate int GetDataCallback(
        nint self,
        ref NativeFormatEtc format,
        out NativeStorageMedium medium);

    [UnmanagedFunctionPointer(CallingConvention.StdCall)]
    private delegate int QueryGetDataCallback(nint self, ref NativeFormatEtc format);

    [Fact]
    public void VirtualDescriptorReader_ClampsMaliciousCountToBufferSize()
    {
        // A drag source controls both the declared count and the HGLOBAL
        // size. A small allocation with a huge count used to read past the
        // buffer; the reader must clamp instead of trusting the header.
        nint memory = GlobalAlloc(0x0042 /* GMEM_MOVEABLE | GMEM_ZEROINIT */, 8);
        Assert.NotEqual(0, memory);
        try
        {
            nint pointer = GlobalLock(memory);
            Assert.NotEqual(0, pointer);
            try
            {
                Marshal.WriteInt32(pointer, 4096);
            }
            finally
            {
                _ = GlobalUnlock(memory);
            }

            var descriptors = NativeDropTarget.ReadVirtualFileDescriptors(memory);
            Assert.Empty(descriptors);
        }
        finally
        {
            Assert.Equal(0, GlobalFree(memory));
        }
    }

    [Fact]
    public void VirtualDescriptorReader_ClampsToPartialDescriptorCapacity()
    {
        int descriptorSize = Marshal.SizeOf<NativeDropTarget.FILEDESCRIPTORW>();
        nint memory = GlobalAlloc(
            0x0042,
            (nuint)(sizeof(uint) + descriptorSize + (descriptorSize / 2)));
        Assert.NotEqual(0, memory);
        try
        {
            nint pointer = GlobalLock(memory);
            Assert.NotEqual(0, pointer);
            try
            {
                Marshal.WriteInt32(pointer, 2);
                Marshal.WriteInt32(pointer + sizeof(uint), 0);
                Marshal.WriteInt32(
                    pointer + sizeof(uint) + descriptorSize,
                    0);
            }
            finally
            {
                _ = GlobalUnlock(memory);
            }

            var descriptors = NativeDropTarget.ReadVirtualFileDescriptors(memory);
            // Capacity is one full descriptor; the second declared entry is
            // beyond the buffer and must be dropped rather than read.
            Assert.Single(descriptors);
        }
        finally
        {
            Assert.Equal(0, GlobalFree(memory));
        }
    }

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern nint GlobalAlloc(uint flags, nuint bytes);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern nint GlobalLock(nint memory);

    [DllImport("kernel32.dll", SetLastError = true)]
    [return: MarshalAs(UnmanagedType.Bool)]
    private static extern bool GlobalUnlock(nint memory);

    [DllImport("kernel32.dll", SetLastError = true)]
    private static extern nint GlobalFree(nint memory);

    [UnmanagedFunctionPointer(CallingConvention.StdCall)]
    private delegate int SetDataCallback(
        nint self,
        ref NativeFormatEtc format,
        ref NativeStorageMedium medium,
        int release);

    [UnmanagedFunctionPointer(CallingConvention.StdCall)]
    private delegate int ReadCallback(
        nint self,
        nint buffer,
        uint bufferSize,
        nint bytesRead);

    private sealed class FakeComObject : IDisposable
    {
        private readonly List<Delegate> _callbacks = [];
        private readonly nint _vtable;

        public FakeComObject(int slotCount)
        {
            _vtable = Marshal.AllocHGlobal(checked(slotCount * IntPtr.Size));
            Pointer = Marshal.AllocHGlobal(IntPtr.Size);
            for (int slot = 0; slot < slotCount; slot++)
            {
                Marshal.WriteIntPtr(_vtable, slot * IntPtr.Size, IntPtr.Zero);
            }

            Marshal.WriteIntPtr(Pointer, _vtable);
        }

        public nint Pointer { get; }

        public void SetSlot<TDelegate>(int slot, TDelegate callback)
            where TDelegate : Delegate
        {
            _callbacks.Add(callback);
            Marshal.WriteIntPtr(
                _vtable,
                checked(slot * IntPtr.Size),
                Marshal.GetFunctionPointerForDelegate(callback));
        }

        public void Dispose()
        {
            Marshal.FreeHGlobal(Pointer);
            Marshal.FreeHGlobal(_vtable);
            GC.KeepAlive(_callbacks);
        }
    }
}
