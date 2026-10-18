using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class NativeNotificationActivationEnvelopeStoreTests : IDisposable
{
    private readonly string _tempRoot = Path.Combine(
        Path.GetTempPath(),
        "DeskBox.Tests",
        Guid.NewGuid().ToString("N"));

    [Fact]
    public void StoreAndTakeNext_PreservesTypedUserInputAndConsumesFile()
    {
        var store = CreateStore();
        DateTimeOffset capturedAtUtc = DateTimeOffset.Parse(
            "2026-08-23T05:06:07Z");

        NativeNotificationActivationEnvelopeWriteResult written = store.Store(
            new NativeAppNotificationActivation(
                "source=todoReminder;action=snooze;widgetId=w;itemId=i",
                new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
                {
                    [TodoNotificationActivationRouter.SnoozeInputId] =
                        TodoNotificationActivationRouter.Snooze30Minutes
                },
                NativeAppNotificationActivationSource.CurrentAppInstance,
                capturedAtUtc,
                SourceProcessId: 4242));
        Assert.Equal(
            NativeNotificationActivationEnvelopeWriteDisposition.Stored,
            written.Disposition);
        Assert.True(store.HasPendingActivation);
        NativeNotificationActivationEnvelopeTakeResult taken = store.TryTakeNext();

        Assert.Equal(
            NativeNotificationActivationEnvelopeTakeDisposition.Consumed,
            taken.Disposition);
        NativeNotificationActivationEnvelope envelope = Assert.IsType<
            NativeNotificationActivationEnvelope>(taken.Envelope);
        Assert.Equal(written.Envelope!.EnvelopeId, envelope.EnvelopeId);
        Assert.Equal(4242, envelope.SourceProcessId);
        Assert.Equal(capturedAtUtc, envelope.CreatedAtUtc);
        Assert.Equal(
            NativeAppNotificationActivationSource.CurrentAppInstance,
            envelope.ActivationSource);
        Assert.Equal(
            TodoNotificationActivationRouter.Snooze30Minutes,
            envelope.UserInput[TodoNotificationActivationRouter.SnoozeInputId]);
        Assert.False(envelope.IsLegacyArgumentsOnly);
        Assert.Equal(0, store.PendingFileCount);
        Assert.False(store.HasPendingActivation);
        Assert.Equal(
            NativeNotificationActivationEnvelopeTakeDisposition.Empty,
            store.TryTakeNext().Disposition);
    }

    [Fact]
    public void Store_RejectsDuplicateEnvelopeWithoutOverwrite()
    {
        var store = CreateStore();
        NativeNotificationActivationEnvelope envelope = CreateEnvelope(
            Guid.Parse("11111111-2222-3333-4444-555555555555"),
            DateTimeOffset.Parse("2026-08-23T01:02:03Z"),
            "first");

        NativeNotificationActivationEnvelopeWriteResult first = store.Store(envelope);
        envelope.Arguments = "second";
        NativeNotificationActivationEnvelopeWriteResult duplicate = store.Store(envelope);
        NativeNotificationActivationEnvelopeTakeResult taken = store.TryTakeNext();

        Assert.Equal(
            NativeNotificationActivationEnvelopeWriteDisposition.Stored,
            first.Disposition);
        Assert.Equal(
            NativeNotificationActivationEnvelopeWriteDisposition.Duplicate,
            duplicate.Disposition);
        Assert.Equal("first", taken.Envelope!.Arguments);
    }

    [Fact]
    public void TryTakeNext_RejectsCorruptEntryThenConsumesFollowingValidEnvelope()
    {
        var store = CreateStore();
        Directory.CreateDirectory(store.SpoolPath);
        string corruptPath = Path.Combine(
            store.SpoolPath,
            "0000000000000000000-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.json");
        File.WriteAllText(corruptPath, "{broken-json");
        Assert.Equal(
            NativeNotificationActivationEnvelopeWriteDisposition.Stored,
            store.Store(CreateEnvelope(
                Guid.Parse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"),
                DateTimeOffset.Parse("2026-08-23T02:00:00Z"),
                "valid")).Disposition);

        NativeNotificationActivationEnvelopeTakeResult rejected = store.TryTakeNext();
        NativeNotificationActivationEnvelopeTakeResult consumed = store.TryTakeNext();

        Assert.Equal(
            NativeNotificationActivationEnvelopeTakeDisposition.Rejected,
            rejected.Disposition);
        Assert.False(File.Exists(corruptPath));
        Assert.Equal(
            NativeNotificationActivationEnvelopeTakeDisposition.Consumed,
            consumed.Disposition);
        Assert.Equal("valid", consumed.Envelope!.Arguments);
        Assert.Equal(0, store.PendingFileCount);
    }

    [Fact]
    public void TryTakeNext_UsesCreatedTimestampFileOrder()
    {
        var store = CreateStore();
        Assert.Equal(
            NativeNotificationActivationEnvelopeWriteDisposition.Stored,
            store.Store(CreateEnvelope(
                Guid.NewGuid(),
                DateTimeOffset.Parse("2026-08-23T03:00:00Z"),
                "later")).Disposition);
        Assert.Equal(
            NativeNotificationActivationEnvelopeWriteDisposition.Stored,
            store.Store(CreateEnvelope(
                Guid.NewGuid(),
                DateTimeOffset.Parse("2026-08-23T02:00:00Z"),
                "earlier")).Disposition);

        Assert.Equal("earlier", store.TryTakeNext().Envelope!.Arguments);
        Assert.Equal("later", store.TryTakeNext().Envelope!.Arguments);
    }

    [Fact]
    public async Task TryTakeNext_ConcurrentConsumersConsumeEnvelopeOnlyOnce()
    {
        var store = CreateStore();
        Assert.Equal(
            NativeNotificationActivationEnvelopeWriteDisposition.Stored,
            store.Store(CreateEnvelope(
                Guid.NewGuid(),
                DateTimeOffset.UtcNow,
                "once")).Disposition);

        NativeNotificationActivationEnvelopeTakeResult[] results = await Task.WhenAll(
            Task.Run(store.TryTakeNext),
            Task.Run(store.TryTakeNext));

        Assert.Single(results, result =>
            result.Disposition == NativeNotificationActivationEnvelopeTakeDisposition.Consumed);
        Assert.Single(results, result =>
            result.Disposition == NativeNotificationActivationEnvelopeTakeDisposition.Empty);
    }

    [Fact]
    public void TryTakeNext_MigratesLegacyArgumentsOnlyFile()
    {
        var store = CreateStore();
        Directory.CreateDirectory(_tempRoot);
        File.WriteAllText(
            store.LegacyPath,
            "source=todoReminder;action=snooze10;widgetId=legacy;itemId=item");

        NativeNotificationActivationEnvelopeTakeResult result = store.TryTakeNext();

        Assert.Equal(
            NativeNotificationActivationEnvelopeTakeDisposition.Consumed,
            result.Disposition);
        Assert.True(result.Envelope!.IsLegacyArgumentsOnly);
        Assert.Equal(0, result.Envelope.SourceProcessId);
        Assert.Empty(result.Envelope.UserInput);
        Assert.False(File.Exists(store.LegacyPath));
    }

    [Fact]
    public void TryTakeNext_RecoversClaimOwnedByExitedProcess()
    {
        var store = CreateStore();
        NativeNotificationActivationEnvelopeWriteResult written = store.Store(
            CreateEnvelope(
                Guid.Parse("cccccccc-cccc-cccc-cccc-cccccccccccc"),
                DateTimeOffset.Parse("2026-08-23T04:00:00Z"),
                "recovered"));
        string originalPath = Assert.IsType<string>(written.Path);
        string claimPath = originalPath +
            ".claim.2147483647.11111111111111111111111111111111";
        File.Move(originalPath, claimPath, overwrite: false);

        NativeNotificationActivationEnvelopeTakeResult result = store.TryTakeNext();

        Assert.Equal(
            NativeNotificationActivationEnvelopeTakeDisposition.Consumed,
            result.Disposition);
        Assert.Equal("recovered", result.Envelope!.Arguments);
        Assert.False(File.Exists(claimPath));
        Assert.Equal(0, store.PendingFileCount);
    }

    [Fact]
    public void TryTakeNext_RecoversLegacyClaimWhenSpoolDirectoryDoesNotExist()
    {
        var store = CreateStore();
        File.WriteAllText(
            store.LegacyPath,
            "source=todoReminder;action=snooze10;widgetId=legacy;itemId=recovered");
        string claimPath = store.LegacyPath +
            ".claim.2147483647.22222222222222222222222222222222";
        File.Move(store.LegacyPath, claimPath, overwrite: false);
        Assert.False(Directory.Exists(store.SpoolPath));
        Assert.True(store.HasPendingActivation);

        NativeNotificationActivationEnvelopeTakeResult result = store.TryTakeNext();

        Assert.Equal(
            NativeNotificationActivationEnvelopeTakeDisposition.Consumed,
            result.Disposition);
        Assert.True(result.Envelope!.IsLegacyArgumentsOnly);
        Assert.Contains("itemId=recovered", result.Envelope.Arguments, StringComparison.Ordinal);
        Assert.False(File.Exists(claimPath));
        Assert.False(File.Exists(store.LegacyPath));
        Assert.False(store.HasPendingActivation);
    }

    [Fact]
    public void Store_RejectsInvalidOrOversizedPayloadWithoutPublishing()
    {
        var store = CreateStore();
        NativeNotificationActivationEnvelope invalid = CreateEnvelope(
            Guid.NewGuid(),
            DateTimeOffset.UtcNow,
            new string('x', 8 * 1024 + 1));

        NativeNotificationActivationEnvelopeWriteResult result = store.Store(invalid);

        Assert.Equal(
            NativeNotificationActivationEnvelopeWriteDisposition.Rejected,
            result.Disposition);
        Assert.Equal(0, store.PendingFileCount);
    }

    [Fact]
    public void Store_RejectsUnknownNumericActivationSource()
    {
        var store = CreateStore();
        NativeNotificationActivationEnvelope invalid = CreateEnvelope(
            Guid.NewGuid(),
            DateTimeOffset.UtcNow,
            "source=todoReminder;itemId=invalid-source");
        invalid.ActivationSource = (NativeAppNotificationActivationSource)999;

        NativeNotificationActivationEnvelopeWriteResult result = store.Store(invalid);

        Assert.Equal(
            NativeNotificationActivationEnvelopeWriteDisposition.Rejected,
            result.Disposition);
        Assert.Equal(0, store.PendingFileCount);
    }

    public void Dispose()
    {
        try
        {
            if (Directory.Exists(_tempRoot))
            {
                Directory.Delete(_tempRoot, recursive: true);
            }
        }
        catch
        {
        }
    }

    private NativeNotificationActivationEnvelopeStore CreateStore()
    {
        Directory.CreateDirectory(_tempRoot);
        return new NativeNotificationActivationEnvelopeStore(_tempRoot);
    }

    private static NativeNotificationActivationEnvelope CreateEnvelope(
        Guid id,
        DateTimeOffset createdAtUtc,
        string arguments)
    {
        return new NativeNotificationActivationEnvelope
        {
            EnvelopeId = id.ToString("N"),
            CreatedAtUtc = createdAtUtc,
            SourceProcessId = 42,
            Arguments = arguments,
            UserInput = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
        };
    }
}
