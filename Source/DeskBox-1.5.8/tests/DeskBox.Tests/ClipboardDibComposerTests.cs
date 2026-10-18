using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class ClipboardDibComposerTests
{
    [Fact]
    public void ComposeFileBytes_24bppDib_wrapsWithBmpHeader()
    {
        byte[] dib = BuildDib(bitCount: 24, compression: 0, clrUsed: 0, extraBytes: 4);

        byte[]? bmp = ClipboardDibComposer.ComposeFileBytes(dib);

        Assert.NotNull(bmp);
        Assert.Equal(14 + dib.Length, bmp.Length);
        Assert.Equal((byte)'B', bmp[0]);
        Assert.Equal((byte)'M', bmp[1]);
        Assert.Equal((uint)bmp.Length, ReadUInt32(bmp, 2));
        // No palette, no masks: pixels start right after file+info header.
        Assert.Equal(14u + 40u, ReadUInt32(bmp, 10));
        // The DIB payload is copied verbatim.
        Assert.Equal(dib[..4], bmp[14..18]);
    }

    [Fact]
    public void ComposeFileBytes_8bppDib_accountsForPaletteOffset()
    {
        // 8bpp with clrUsed=0 means a full 256-entry palette.
        byte[] dib = BuildDib(bitCount: 8, compression: 0, clrUsed: 0, extraBytes: 1024 + 4);

        byte[]? bmp = ClipboardDibComposer.ComposeFileBytes(dib);

        Assert.NotNull(bmp);
        Assert.Equal(14u + 40u + 256u * 4u, ReadUInt32(bmp, 10));
    }

    [Fact]
    public void ComposeFileBytes_32bppBitfields_addsMaskBytes()
    {
        byte[] dib = BuildDib(bitCount: 32, compression: 3, clrUsed: 0, extraBytes: 12 + 4);

        byte[]? bmp = ClipboardDibComposer.ComposeFileBytes(dib);

        Assert.NotNull(bmp);
        Assert.Equal(14u + 40u + 12u, ReadUInt32(bmp, 10));
    }

    [Fact]
    public void ComposeFileBytes_v5Header_keepsMasksInsideHeaderSize()
    {
        byte[] dib = BuildDib(
            bitCount: 32,
            compression: 0,
            clrUsed: 0,
            extraBytes: 4,
            headerSize: 124);

        byte[]? bmp = ClipboardDibComposer.ComposeFileBytes(dib);

        Assert.NotNull(bmp);
        Assert.Equal(14u + 124u, ReadUInt32(bmp, 10));
    }

    [Theory]
    [InlineData(0)]
    [InlineData(12)]
    [InlineData(36)]
    public void ComposeFileBytes_unknownHeaderSize_returnsNull(int headerSize)
    {
        byte[] dib = BuildDib(
            bitCount: 24,
            compression: 0,
            clrUsed: 0,
            extraBytes: 4,
            headerSize: headerSize);

        Assert.Null(ClipboardDibComposer.ComposeFileBytes(dib));
    }

    [Fact]
    public void ComposeFileBytes_truncatedDib_returnsNull()
    {
        Assert.Null(ClipboardDibComposer.ComposeFileBytes([40, 0, 0, 0, 1, 0, 0, 0]));
        Assert.Null(ClipboardDibComposer.ComposeFileBytes(new byte[8]));
    }

    private static byte[] BuildDib(
        int bitCount,
        int compression,
        uint clrUsed,
        int extraBytes,
        int headerSize = 40)
    {
        // The buffer always carries the full 40-byte field layout so the
        // little-endian writers below stay in bounds; the declared header
        // size in the first field is what the composer must judge.
        var dib = new byte[Math.Max(40, headerSize) + Math.Max(0, extraBytes)];
        WriteInt32(dib, 0, headerSize);
        WriteInt32(dib, 4, 1); // biWidth
        WriteInt32(dib, 8, 1); // biHeight
        WriteInt16(dib, 12, 1); // biPlanes
        WriteInt16(dib, 14, (short)bitCount);
        WriteInt32(dib, 16, compression);
        WriteInt32(dib, 20, Math.Max(0, extraBytes)); // biSizeImage
        WriteUInt32(dib, 32, clrUsed);
        return dib;
    }

    private static void WriteInt32(byte[] buffer, int offset, int value)
    {
        buffer[offset] = (byte)value;
        buffer[offset + 1] = (byte)(value >> 8);
        buffer[offset + 2] = (byte)(value >> 16);
        buffer[offset + 3] = (byte)(value >> 24);
    }

    private static void WriteInt16(byte[] buffer, int offset, short value)
    {
        buffer[offset] = (byte)value;
        buffer[offset + 1] = (byte)(value >> 8);
    }

    private static void WriteUInt32(byte[] buffer, int offset, uint value)
    {
        buffer[offset] = (byte)value;
        buffer[offset + 1] = (byte)(value >> 8);
        buffer[offset + 2] = (byte)(value >> 16);
        buffer[offset + 3] = (byte)(value >> 24);
    }

    private static uint ReadUInt32(byte[] buffer, int offset)
    {
        return (uint)(buffer[offset] |
            (buffer[offset + 1] << 8) |
            (buffer[offset + 2] << 16) |
            (buffer[offset + 3] << 24));
    }
}
