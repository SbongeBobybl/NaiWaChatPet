using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class FileTransferSessionRegistryTests
{
    [Fact]
    public void MoveSessionMarksSourceDestinationAndReceivingFolder()
    {
        string root = Path.Combine(
            Path.GetTempPath(),
            "DeskBox-transfer-registry",
            Guid.NewGuid().ToString("N"));
        string source = Path.Combine(root, "source");
        string destination = Path.Combine(root, "target", "source");
        var registry = new FileTransferSessionRegistry();

        using FileTransferSessionLease lease = registry.Begin(
            [new FileTransferRegistration(
                source,
                destination,
                SourceIsDirectory: true)],
            isMove: true);

        Assert.Equal(1, registry.ActiveSessionCount);

        FileTransferPathState sourceState = registry.GetState(source);
        Assert.Equal(FileTransferPathKind.Source, sourceState.Kind);
        Assert.True(sourceState.IsMove);
        Assert.True(sourceState.BlocksMutation);
        Assert.True(sourceState.BlocksOpen);

        Assert.Equal(
            FileTransferPathKind.Source,
            registry.GetState(Path.Combine(source, "nested", "file.docx")).Kind);
        Assert.Equal(
            FileTransferPathKind.Destination,
            registry.GetState(destination).Kind);
        Assert.Equal(
            FileTransferPathKind.Destination,
            registry.GetState(Path.Combine(destination, "nested", "file.docx")).Kind);
        Assert.Equal(
            FileTransferPathKind.DestinationFolder,
            registry.GetState(Path.GetDirectoryName(destination)).Kind);
        Assert.Equal(
            FileTransferPathKind.None,
            registry.GetState(Path.Combine(root, "target-other")).Kind);
    }

    [Fact]
    public void CopySourceCanOpenButStillCannotBeMutated()
    {
        string root = Path.Combine(
            Path.GetTempPath(),
            "DeskBox-transfer-registry",
            Guid.NewGuid().ToString("N"));
        string source = Path.Combine(root, "report.pdf");
        string destination = Path.Combine(root, "target", "report.pdf");
        var registry = new FileTransferSessionRegistry();

        using FileTransferSessionLease lease = registry.Begin(
            [new FileTransferRegistration(source, destination)],
            isMove: false);

        FileTransferPathState sourceState = registry.GetState(source);
        Assert.Equal(FileTransferPathKind.Source, sourceState.Kind);
        Assert.False(sourceState.BlocksOpen);
        Assert.True(sourceState.BlocksMutation);
        Assert.Equal(
            FileTransferPathKind.DestinationFolder,
            registry.GetState(Path.GetDirectoryName(destination)).Kind);
    }

    [Fact]
    public void DisposingLeaseClearsStateAndRaisesChange()
    {
        string source = Path.Combine(
            Path.GetTempPath(),
            "DeskBox-transfer-registry",
            Guid.NewGuid().ToString("N"),
            "source.txt");
        string destination = Path.Combine(
            Path.GetTempPath(),
            "DeskBox-transfer-registry",
            Guid.NewGuid().ToString("N"),
            "destination.txt");
        var registry = new FileTransferSessionRegistry();
        int changes = 0;
        registry.StateChanged += () => changes++;

        FileTransferSessionLease lease = registry.Begin(
            [new FileTransferRegistration(source, destination)],
            isMove: true);
        Assert.Equal(1, changes);
        Assert.True(registry.IsPathActive(source));

        lease.Dispose();
        lease.Dispose();

        Assert.Equal(2, changes);
        Assert.Equal(0, registry.ActiveSessionCount);
        Assert.False(registry.IsPathActive(source));
    }

    [Fact]
    public async Task FileServiceKeepsSessionActiveForWholeTransferPipeline()
    {
        string root = Path.Combine(
            Path.GetTempPath(),
            "DeskBox-transfer-registry",
            Guid.NewGuid().ToString("N"));
        string source = Path.Combine(root, "source.txt");
        string destination = Path.Combine(root, "target", "source.txt");
        Directory.CreateDirectory(root);
        await File.WriteAllTextAsync(source, "transfer session");
        var service = new FileService();
        var progressEntered = new TaskCompletionSource<bool>(
            TaskCreationOptions.RunContinuationsAsynchronously);
        using var releaseProgress = new ManualResetEventSlim();
        var progress = new InlineProgress<FileService.FileTransferProgress>(
            update =>
            {
                if (update.Phase != FileService.FileTransferPhase.Transferring)
                {
                    return;
                }

                progressEntered.TrySetResult(true);
                Assert.True(releaseProgress.Wait(TimeSpan.FromSeconds(10)));
            });
        Task<IReadOnlyList<FileService.FileTransferResult>>? transfer = null;

        try
        {
            transfer = service.ExecuteTransferPlanAsync(
                    [new FileService.FileTransferPlan(source, destination)],
                    move: false,
                    progress: progress);

            await progressEntered.Task.WaitAsync(TimeSpan.FromSeconds(10));
            Assert.Equal(
                FileTransferPathKind.Source,
                service.TransferSessions.GetState(source).Kind);
            Assert.Equal(
                FileTransferPathKind.Destination,
                service.TransferSessions.GetState(destination).Kind);

            releaseProgress.Set();
            FileService.FileTransferResult result = Assert.Single(
                await transfer.WaitAsync(TimeSpan.FromSeconds(10)));
            Assert.Equal(destination, result.DestinationPath);
            Assert.Equal(0, service.TransferSessions.ActiveSessionCount);
        }
        finally
        {
            releaseProgress.Set();
            if (transfer is not null)
            {
                try
                {
                    await transfer.WaitAsync(TimeSpan.FromSeconds(10));
                }
                catch
                {
                }
            }

            if (Directory.Exists(root))
            {
                Directory.Delete(root, recursive: true);
            }
        }
    }

    private sealed class InlineProgress<T>(Action<T> report) : IProgress<T>
    {
        public void Report(T value) => report(value);
    }
}
