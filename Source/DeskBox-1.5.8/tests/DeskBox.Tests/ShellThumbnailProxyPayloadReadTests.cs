using DeskBox.Helpers;

namespace DeskBox.Tests;

public sealed class ShellThumbnailProxyPayloadReadTests
{
    private const int MaximumBytes = 2 * 1024 * 1024;

    [Theory]
    [InlineData(1)]
    [InlineData(17)]
    [InlineData(16384)]
    public async Task ReadPayload_PreservesBytesAcrossShortPipeReads(int chunkSize)
    {
        byte[] payload = CreatePayload(138 + 256 * 256 * 4);
        using var stream = new FragmentedStream(payload, chunkSize);

        byte[] result = await ShellThumbnailProxy.ReadBoundedOutputAsync(stream, MaximumBytes);

        Assert.Equal(payload, result);
    }

    [Fact]
    public async Task ReadPayload_EmptyOutputRemainsAnEmptyResult()
    {
        using var stream = new MemoryStream();

        Assert.Empty(await ShellThumbnailProxy.ReadBoundedOutputAsync(stream, MaximumBytes));
    }

    [Theory]
    [InlineData(0L)]
    [InlineData(137L)]
    [InlineData(MaximumBytes + 1L)]
    [InlineData(4294967295L)]
    public async Task ReadPayload_RejectsInvalidDeclaredLengthBeforeReadingBody(long length)
    {
        byte[] header = CreatePayload(14);
        BitConverter.GetBytes((uint)length).CopyTo(header, 2);
        using var stream = new MemoryStream(header);

        await Assert.ThrowsAsync<InvalidDataException>(() =>
            ShellThumbnailProxy.ReadBoundedOutputAsync(stream, MaximumBytes));
        Assert.Equal(14, stream.Position);
    }

    [Fact]
    public async Task ReadPayload_AcceptsTheExactSizeLimit()
    {
        byte[] payload = CreatePayload(MaximumBytes);
        using var stream = new MemoryStream(payload);

        Assert.Equal(payload, await ShellThumbnailProxy.ReadBoundedOutputAsync(stream, MaximumBytes));
    }

    [Fact]
    public async Task ReadPayload_RejectsUnexpectedFormat()
    {
        byte[] payload = CreatePayload(142);
        payload[0] = (byte)'P';
        using var stream = new MemoryStream(payload);

        await Assert.ThrowsAsync<InvalidDataException>(() =>
            ShellThumbnailProxy.ReadBoundedOutputAsync(stream, MaximumBytes));
    }

    [Theory]
    [InlineData(1)]
    [InlineData(13)]
    public async Task ReadPayload_RejectsTruncatedHeader(int availableBytes)
    {
        byte[] payload = CreatePayload(142);
        using var stream = new MemoryStream(payload, 0, availableBytes);

        await Assert.ThrowsAsync<InvalidDataException>(() =>
            ShellThumbnailProxy.ReadBoundedOutputAsync(stream, MaximumBytes));
    }

    [Fact]
    public async Task ReadPayload_RejectsTruncatedPixels()
    {
        byte[] payload = CreatePayload(142);
        using var stream = new MemoryStream(payload, 0, payload.Length - 1);

        await Assert.ThrowsAsync<EndOfStreamException>(() =>
            ShellThumbnailProxy.ReadBoundedOutputAsync(stream, MaximumBytes));
    }

    [Fact]
    public async Task ReadPayload_RejectsTrailingBytes()
    {
        byte[] payload = CreatePayload(142);
        BitConverter.GetBytes(141).CopyTo(payload, 2);
        using var stream = new MemoryStream(payload);

        await Assert.ThrowsAsync<InvalidDataException>(() =>
            ShellThumbnailProxy.ReadBoundedOutputAsync(stream, MaximumBytes));
    }

    private static byte[] CreatePayload(int length)
    {
        byte[] payload = new byte[length];
        for (int i = 0; i < payload.Length; i++)
        {
            payload[i] = (byte)i;
        }

        payload[0] = (byte)'B';
        payload[1] = (byte)'M';
        BitConverter.GetBytes(length).CopyTo(payload, 2);
        return payload;
    }

    private sealed class FragmentedStream(byte[] bytes, int chunkSize) : MemoryStream(bytes)
    {
        public override ValueTask<int> ReadAsync(Memory<byte> buffer, CancellationToken cancellationToken = default)
        {
            return base.ReadAsync(buffer[..Math.Min(buffer.Length, chunkSize)], cancellationToken);
        }
    }
}
