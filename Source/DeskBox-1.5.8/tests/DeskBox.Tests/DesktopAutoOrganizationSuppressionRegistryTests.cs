using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class DesktopAutoOrganizationSuppressionRegistryTests : IDisposable
{
    private readonly string _root = Path.Combine(
        Path.GetTempPath(),
        "DeskBox.Tests",
        Guid.NewGuid().ToString("N"));

    [Fact]
    public void PendingRestore_IsConsumedOnlyForTheExactDestination()
    {
        Directory.CreateDirectory(_root);
        string source = Path.Combine(_root, "source.txt");
        string destination = Path.Combine(_root, "desktop", "source.txt");
        var registry = new DesktopAutoOrganizationSuppressionRegistry();
        registry.BeginOperation(
            "restore",
            [new FileService.FileTransferPlan(source, destination)]);

        Assert.False(registry.TryConsume(Path.Combine(_root, "desktop", "other.txt")));

        // The watcher only evaluates real arrivals: the destination must
        // exist, and the restore (a move) proves itself because the source
        // path vanished.
        Directory.CreateDirectory(Path.GetDirectoryName(destination)!);
        File.WriteAllText(destination, "restored");
        Assert.True(registry.TryConsume(destination));
        Assert.False(registry.TryConsume(destination));
    }

    [Fact]
    public void PendingRestore_RejectsUnrelatedPreExistingOccupant()
    {
        Directory.CreateDirectory(_root);
        string source = Path.Combine(_root, "source.txt");
        File.WriteAllText(source, "still here");
        string destination = Path.Combine(_root, "desktop", "source.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(destination)!);
        File.WriteAllText(destination, "unrelated occupant");
        File.SetCreationTimeUtc(destination, DateTime.UtcNow.AddDays(-30));
        var registry = new DesktopAutoOrganizationSuppressionRegistry();
        registry.BeginOperation(
            "restore",
            [new FileService.FileTransferPlan(source, destination)]);

        // The occupant pre-dates the claim and the restore source still
        // exists, so it is not the operation's arrival. The claim survives
        // for the real arrival within the pending window.
        Assert.False(registry.TryConsume(destination));

        File.Move(source, destination, overwrite: true);
        Assert.True(registry.TryConsume(destination));
    }

    [Fact]
    public void CompletedRestore_RequiresTheSameFileFingerprint()
    {
        Directory.CreateDirectory(_root);
        string destination = Path.Combine(_root, "restored.txt");
        File.WriteAllText(destination, "original");
        string source = Path.Combine(_root, "source.txt");
        File.WriteAllText(source, "source still present");
        var registry = new DesktopAutoOrganizationSuppressionRegistry();
        var plan = new FileService.FileTransferPlan(source, destination);
        registry.BeginOperation("restore", [plan]);
        registry.CompleteOperation("restore", [destination]);
        File.AppendAllText(destination, " replacement");

        // Fingerprint changed while the source is still present and the
        // destination predates the claim: not the operation's arrival.
        Assert.False(registry.TryConsume(destination));
    }

    [Fact]
    public void CompletedRestore_EditedArrival_StaysSuppressed()
    {
        Directory.CreateDirectory(_root);
        string source = Path.Combine(_root, "widget", "restored.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(source)!);
        File.WriteAllText(source, "original");
        string destination = Path.Combine(_root, "desktop", "restored.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(destination)!);
        var registry = new DesktopAutoOrganizationSuppressionRegistry();
        registry.BeginOperation(
            "restore",
            [new FileService.FileTransferPlan(source, destination)]);
        File.Move(source, destination);
        registry.CompleteOperation("restore", [destination]);

        // A user edit changes the fingerprint, but the vanished source still
        // proves this file is the operation's arrival.
        File.AppendAllText(destination, " edited");
        Assert.True(registry.TryConsume(destination));
    }

    [Fact]
    public void ExpiredRestore_IsNotConsumed()
    {
        Directory.CreateDirectory(_root);
        DateTimeOffset now = DateTimeOffset.UtcNow;
        string destination = Path.Combine(_root, "expired.txt");
        var registry = new DesktopAutoOrganizationSuppressionRegistry(
            () => now,
            TimeSpan.FromSeconds(5));
        registry.BeginOperation(
            "restore",
            [new FileService.FileTransferPlan("source", destination)]);
        now += TimeSpan.FromSeconds(6);

        Assert.False(registry.TryConsume(destination));
    }

    [Fact]
    public void DraggedArrival_ExistingDestination_IsSuppressedUntilFingerprintChanges()
    {
        Directory.CreateDirectory(_root);
        string source = Path.Combine(_root, "widget", "a.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(source)!);
        File.WriteAllText(source, "source");
        string destination = Path.Combine(_root, "desktop", "a.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(destination)!);
        File.WriteAllText(destination, "arrived copy");
        var registry = new DesktopAutoOrganizationSuppressionRegistry();

        registry.SuppressDraggedArrivals([(source, destination)]);

        Assert.True(registry.TryConsume(destination));
        Assert.False(registry.TryConsume(destination));
    }

    [Fact]
    public void DraggedArrival_PendingCopy_IsSuppressedOnceItArrives()
    {
        Directory.CreateDirectory(_root);
        string source = Path.Combine(_root, "widget", "b.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(source)!);
        File.WriteAllText(source, "source");
        string destination = Path.Combine(_root, "desktop", "b.txt");
        var registry = new DesktopAutoOrganizationSuppressionRegistry();

        // The destination does not exist at registration: only a pending
        // claim exists until the copy materializes.
        registry.SuppressDraggedArrivals([(source, destination)]);
        Directory.CreateDirectory(Path.GetDirectoryName(destination)!);
        File.WriteAllText(destination, "arrived copy");

        Assert.True(registry.TryConsume(destination));
    }

    [Fact]
    public void DraggedArrival_PendingMove_IsSuppressedWhenSourceVanishes()
    {
        Directory.CreateDirectory(_root);
        string source = Path.Combine(_root, "widget", "c.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(source)!);
        File.WriteAllText(source, "source");
        // A same-volume move keeps the original creation time, so simulate it
        // by aging the file before the pending claim is consumed.
        File.SetCreationTimeUtc(source, DateTime.UtcNow.AddDays(-30));
        string destination = Path.Combine(_root, "desktop", "c.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(destination)!);
        var registry = new DesktopAutoOrganizationSuppressionRegistry();

        registry.SuppressDraggedArrivals([(source, destination)]);
        File.Move(source, destination);

        Assert.True(registry.TryConsume(destination));
    }

    [Fact]
    public void DraggedArrival_FingerprintMismatchWithoutEvidence_IsRejected()
    {
        Directory.CreateDirectory(_root);
        string source = Path.Combine(_root, "widget", "d.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(source)!);
        File.WriteAllText(source, "source");
        string destination = Path.Combine(_root, "desktop", "d.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(destination)!);
        File.WriteAllText(destination, "older file");
        File.SetCreationTimeUtc(destination, DateTime.UtcNow.AddDays(-30));
        var registry = new DesktopAutoOrganizationSuppressionRegistry();

        // An existing destination registers fingerprinted, not pending: a
        // pre-existing same-named file is not this drag's arrival.
        registry.SuppressDraggedArrivals([(source, destination)]);

        // The fingerprinted entry matches whatever occupies the path at
        // registration; replacing it afterwards fails the identity check and
        // the evidence fallback (the source still exists and the destination
        // creation time predates the claim).
        File.WriteAllText(destination, "replacement");
        Assert.False(registry.TryConsume(destination));
    }

    [Fact]
    public void DraggedArrival_PendingClaim_Expires()
    {
        Directory.CreateDirectory(_root);
        DateTimeOffset now = DateTimeOffset.UtcNow;
        string source = Path.Combine(_root, "widget", "e.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(source)!);
        File.WriteAllText(source, "source");
        string destination = Path.Combine(_root, "desktop", "e.txt");
        var registry = new DesktopAutoOrganizationSuppressionRegistry(
            () => now,
            pendingLifetime: TimeSpan.FromSeconds(90),
            evaluationMargin: TimeSpan.Zero);
        registry.SuppressDraggedArrivals(
            [(source, destination)],
            TimeSpan.Zero);

        // A name that never materializes must not hold the claim forever.
        now += TimeSpan.FromSeconds(120);
        Directory.CreateDirectory(Path.GetDirectoryName(destination)!);
        File.WriteAllText(destination, "unrelated same-named file");

        Assert.False(registry.TryConsume(destination));
    }

    [Fact]
    public void DraggedArrival_PendingClaim_OutlivesConfiguredDelay()
    {
        Directory.CreateDirectory(_root);
        DateTimeOffset now = DateTimeOffset.UtcNow;
        string source = Path.Combine(_root, "widget", "f.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(source)!);
        File.WriteAllText(source, "source");
        string destination = Path.Combine(_root, "desktop", "f.txt");
        var registry = new DesktopAutoOrganizationSuppressionRegistry(
            () => now);
        // A five-minute delay tier defers the evaluation far past the old
        // 90-second pending floor.
        registry.SuppressDraggedArrivals(
            [(source, destination)],
            TimeSpan.FromMinutes(5));

        now += TimeSpan.FromMinutes(4);
        Directory.CreateDirectory(Path.GetDirectoryName(destination)!);
        File.WriteAllText(destination, "arrived after the delay");

        Assert.True(registry.TryConsume(destination));
    }

    [Fact]
    public void DraggedArrival_PendingEvidenceFailure_KeepsClaimForRealArrival()
    {
        Directory.CreateDirectory(_root);
        DateTimeOffset now = DateTimeOffset.UtcNow;
        string source = Path.Combine(_root, "widget", "g.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(source)!);
        File.WriteAllText(source, "source");
        string destination = Path.Combine(_root, "desktop", "g.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(destination)!);
        var registry = new DesktopAutoOrganizationSuppressionRegistry(
            () => now,
            evaluationMargin: TimeSpan.Zero);
        registry.SuppressDraggedArrivals(
            [(source, destination)],
            TimeSpan.Zero);

        // An unrelated old file at the claimed path is rejected without
        // burning the claim: the real arrival can still prove itself later.
        File.WriteAllText(destination, "unrelated occupant");
        File.SetCreationTimeUtc(destination, DateTime.UtcNow.AddDays(-30));
        Assert.False(registry.TryConsume(destination));

        File.SetCreationTimeUtc(destination, now.UtcDateTime);
        Assert.True(registry.TryConsume(destination));
    }

    [Fact]
    public void DraggedArrival_DoesNotOverwritePlannedRestoreClaim()
    {
        Directory.CreateDirectory(_root);
        string source = Path.Combine(_root, "widget", "h.txt");
        string restoreSource = Path.Combine(_root, "managed", "h.txt");
        string destination = Path.Combine(_root, "desktop", "h.txt");
        var registry = new DesktopAutoOrganizationSuppressionRegistry();
        registry.BeginOperation(
            "restore",
            [new FileService.FileTransferPlan(restoreSource, destination)]);

        // A drag-out to the same destination must not downgrade the planned
        // restore's claim into drag evidence rules.
        registry.SuppressDraggedArrivals([(source, destination)]);

        // The pending restore claim still requires its own evidence.
        Assert.False(registry.TryConsume(destination));
    }

    [Fact]
    public void Ledger_RoundTripsSurvivingClaims()
    {
        Directory.CreateDirectory(_root);
        DateTimeOffset now = DateTimeOffset.UtcNow;
        string source = Path.Combine(_root, "widget", "i.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(source)!);
        File.WriteAllText(source, "source");
        string destination = Path.Combine(_root, "desktop", "i.txt");
        Directory.CreateDirectory(Path.GetDirectoryName(destination)!);
        File.WriteAllText(destination, "arrived");
        string ledger = Path.Combine(_root, "suppressions.json");
        var first = new DesktopAutoOrganizationSuppressionRegistry(
            () => now,
            ledgerPath: ledger);
        first.SuppressDraggedArrivals([(source, destination)]);

        // The chained persist write is asynchronous; flush it by polling for
        // the ledger file the same way a restart would observe it.
        SpinWait.SpinUntil(
            () => File.Exists(ledger),
            TimeSpan.FromSeconds(10));
        var second = new DesktopAutoOrganizationSuppressionRegistry(
            () => now,
            ledgerPath: ledger);

        Assert.True(second.TryConsume(destination));
        Assert.False(second.TryConsume(destination));

        // TryConsume queues another chained persist after the SpinWait above
        // observed the first one; drain both registries or the in-flight
        // temp-file write races this test's directory teardown (seen as an
        // IOException on slower CI runners).
        Assert.True(first.WaitForPendingPersistAsync().Wait(TimeSpan.FromSeconds(10)));
        Assert.True(second.WaitForPendingPersistAsync().Wait(TimeSpan.FromSeconds(10)));
    }

    [Fact]
    public void Ledger_DropsExpiredClaims()
    {
        Directory.CreateDirectory(_root);
        DateTimeOffset now = DateTimeOffset.UtcNow;
        string destination = Path.Combine(_root, "desktop", "j.txt");
        string ledger = Path.Combine(_root, "suppressions.json");
        var first = new DesktopAutoOrganizationSuppressionRegistry(
            () => now,
            lifetime: TimeSpan.FromSeconds(5),
            ledgerPath: ledger);
        first.BeginOperation(
            "restore",
            [new FileService.FileTransferPlan(
                Path.Combine(_root, "j.txt"),
                destination)]);
        SpinWait.SpinUntil(
            () => File.Exists(ledger),
            TimeSpan.FromSeconds(10));

        now += TimeSpan.FromSeconds(6);
        var second = new DesktopAutoOrganizationSuppressionRegistry(
            () => now,
            lifetime: TimeSpan.FromSeconds(5),
            ledgerPath: ledger);

        Directory.CreateDirectory(Path.GetDirectoryName(destination)!);
        File.WriteAllText(destination, "arrived after expiry");
        Assert.False(second.TryConsume(destination));

        Assert.True(first.WaitForPendingPersistAsync().Wait(TimeSpan.FromSeconds(10)));
        Assert.True(second.WaitForPendingPersistAsync().Wait(TimeSpan.FromSeconds(10)));
    }

    public void Dispose()
    {
        if (!Directory.Exists(_root))
        {
            return;
        }

        // Pool-thread handle release can trail the drained persist task by a
        // scheduling hiccup on loaded runners; a short bounded retry keeps
        // teardown deterministic without hiding a stuck writer (a real stall
        // still throws after the retries).
        for (int attempt = 0; ; attempt++)
        {
            try
            {
                Directory.Delete(_root, recursive: true);
                return;
            }
            catch (IOException) when (attempt < 3)
            {
                Thread.Sleep(200);
            }
        }
    }
}
