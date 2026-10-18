using DeskBox.Helpers;

namespace DeskBox.Tests;

public sealed class ShellThumbnailProxyBatchTests
{
    [Fact]
    public void ManifestWriter_EncodesHeaderAndEntries()
    {
        var requests = new[]
        {
            NewRequest(ShellThumbnailProxy.ShellImageMode.Icon, @"C:\a.png", 48),
            NewRequest(ShellThumbnailProxy.ShellImageMode.Thumbnail, @"C:\b.jpg", 256),
            NewRequest(ShellThumbnailProxy.ShellImageMode.IconWithOverlays, @"C:\c.lnk", 32),
        };

        using var stream = new MemoryStream();
        ShellThumbnailProxy.WriteBatchManifest(stream, requests);
        using var reader = new BinaryReader(
            new MemoryStream(stream.ToArray()));

        Assert.Equal(0x4458_4231u, reader.ReadUInt32());
        Assert.Equal(1u, reader.ReadUInt32());
        Assert.Equal(3u, reader.ReadUInt32());

        Assert.Equal(1u, reader.ReadUInt32());
        Assert.Equal(48u, reader.ReadUInt32());
        Assert.Equal(@"C:\a.png", ReadPath(reader));

        Assert.Equal(0u, reader.ReadUInt32());
        Assert.Equal(256u, reader.ReadUInt32());
        Assert.Equal(@"C:\b.jpg", ReadPath(reader));

        Assert.Equal(2u, reader.ReadUInt32());
        Assert.Equal(32u, reader.ReadUInt32());
        Assert.Equal(@"C:\c.lnk", ReadPath(reader));

        Assert.Equal(0, reader.BaseStream.Length - reader.BaseStream.Position);
    }

    [Fact]
    public async Task FrameReader_CompletesRequestsInArrivalOrder()
    {
        byte[] okPayload = new byte[138];
        okPayload[0] = 0x42;
        okPayload[1] = 0x4D;
        byte[] frameOne = BuildFrame(1, 0, okPayload);
        byte[] frameZero = BuildFrame(0, 1, []); // failed extraction
        var requests = new[]
        {
            NewRequest(ShellThumbnailProxy.ShellImageMode.Icon, @"C:\a.png", 48),
            NewRequest(ShellThumbnailProxy.ShellImageMode.Icon, @"C:\b.png", 48),
        };

        using var stream = new MemoryStream([.. frameOne, .. frameZero]);
        var fulfilled = new bool[2];
        int failedCount = await ShellThumbnailProxy.ReadBatchFramesAsync(
            stream,
            requests,
            fulfilled,
            CancellationToken.None);

        Assert.Equal(1, failedCount);
        Assert.True(fulfilled[0] && fulfilled[1]);
        Assert.Null(requests[0].Completion.Task.Result);
        Assert.Equal(
            okPayload,
            requests[1].Completion.Task.Result);
    }

    [Fact]
    public async Task FrameReader_LeavesUnfulfilledRequestsOnEarlyExit()
    {
        byte[] frameZero = BuildFrame(0, 0, new byte[138]);
        var requests = new[]
        {
            NewRequest(ShellThumbnailProxy.ShellImageMode.Icon, @"C:\a.png", 48),
            NewRequest(ShellThumbnailProxy.ShellImageMode.Icon, @"C:\b.png", 48),
        };

        using var stream = new MemoryStream(frameZero);
        var fulfilled = new bool[2];
        int failedCount = await ShellThumbnailProxy.ReadBatchFramesAsync(
            stream,
            requests,
            fulfilled,
            CancellationToken.None);

        Assert.Equal(0, failedCount);
        Assert.True(fulfilled[0]);
        Assert.False(fulfilled[1]);
        Assert.False(requests[1].Completion.Task.IsCompleted);
    }

    [Fact]
    public async Task FrameReader_RejectsDuplicateIndex()
    {
        byte[] frame = BuildFrame(0, 0, new byte[138]);
        var requests = new[]
        {
            NewRequest(ShellThumbnailProxy.ShellImageMode.Icon, @"C:\a.png", 48),
            NewRequest(ShellThumbnailProxy.ShellImageMode.Icon, @"C:\b.png", 48),
        };

        using var stream = new MemoryStream([.. frame, .. frame]);
        await Assert.ThrowsAsync<InvalidDataException>(() =>
            ShellThumbnailProxy.ReadBatchFramesAsync(
                stream,
                requests,
                new bool[2],
                CancellationToken.None));
    }

    /// <summary>
    /// Pins the wire constants shared between the managed client and the
    /// Rust proxy; drifting either side silently breaks every icon load.
    /// </summary>
    [Fact]
    public void BatchProtocol_WireConstantsStayInSync()
    {
        string client = File.ReadAllText(GetRepoFile(
            "src/DeskBox/Helpers/ShellThumbnailProxy.cs"));
        string proxy = File.ReadAllText(GetRepoFile(
            "native/deskbox-thumbnail-proxy/src/main.rs"));

        Assert.Contains("0x4458_4231", client, StringComparison.Ordinal);
        Assert.Contains("0x4458_4231", proxy, StringComparison.Ordinal);
        Assert.Contains("--extract-batch", client, StringComparison.Ordinal);
        Assert.Contains("--extract-batch", proxy, StringComparison.Ordinal);
    }

    private static ShellThumbnailProxy.ProxyBatchRequest NewRequest(
        ShellThumbnailProxy.ShellImageMode mode,
        string path,
        int size) => new(
        mode,
        path,
        size,
        new TaskCompletionSource<byte[]?>(TaskCreationOptions.RunContinuationsAsynchronously));

    private static string ReadPath(BinaryReader reader)
    {
        int length = (int)reader.ReadUInt32();
        return System.Text.Encoding.Unicode.GetString(reader.ReadBytes(length));
    }

    private static byte[] BuildFrame(uint index, uint status, byte[] payload)
    {
        using var stream = new MemoryStream();
        using var writer = new BinaryWriter(stream);
        writer.Write(index);
        writer.Write(status);
        writer.Write((uint)payload.Length);
        writer.Write(payload);
        writer.Flush();
        return stream.ToArray();
    }

    private static string GetRepoFile(string relativePath)
    {
        string? directory = AppContext.BaseDirectory;
        while (!string.IsNullOrWhiteSpace(directory))
        {
            string candidate = Path.Combine(directory, relativePath.Replace('/', Path.DirectorySeparatorChar));
            if (File.Exists(candidate))
            {
                return candidate;
            }

            directory = Directory.GetParent(directory)?.FullName;
        }

        throw new FileNotFoundException($"Could not locate repository file: {relativePath}");
    }
}
