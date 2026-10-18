using System.Text.Json;
using System.Text.Json.Serialization;
using DeskBox.Services;

namespace DeskBox.Core.Persistence;

[JsonSourceGenerationOptions(
    GenerationMode = JsonSourceGenerationMode.Metadata,
    PropertyNamingPolicy = JsonKnownNamingPolicy.CamelCase,
    WriteIndented = true)]
[JsonSerializable(
    typeof(SyncRevisionsDocument),
    TypeInfoPropertyName = "SyncRevisionsDocument")]
internal sealed partial class SyncRevisionsJsonContext : JsonSerializerContext
{
}

/// <summary>
/// data/sync/revisions.json — per-entity protocol state keyed by
/// <c>domain/collection_id/entity_id</c> (sync-protocol-contract §2.3/§8).
/// This is where revisions live; domain records never carry them.
/// </summary>
public sealed class SyncRevisionsDocument
{
    public int SchemaVersion { get; set; } = 1;

    public Dictionary<string, SyncRevisionRecord> Entities { get; set; } = new();
}

public sealed class SyncRevisionRecord
{
    /// <summary>
    /// Composite key used in <see cref="SyncRevisionsDocument.Entities"/>.
    /// Length-prefixed so ids containing '/' stay unambiguous — a plain
    /// "{a}/{b}/{c}" join cannot tell "a/b"+"c" from "a"+"b/c".
    /// </summary>
    public static string Key(string domain,string collectionId,string entityId) =>
        $"{domain.Length}:{domain}{collectionId.Length}:{collectionId}{entityId}";

    /// <summary>Server-issued monotonically increasing revision of the
    /// last envelope the server holds for this entity.</summary>
    public long ServerRevision { get; set; }

    /// <summary>The server_revision this client last confirmed — the
    /// optimistic-concurrency base for the next push (§3.2).</summary>
    public long BaseRevision { get; set; }

    /// <summary>Idempotency key of the last accepted push.</summary>
    public string? OperationId { get; set; }
}

/// <summary>
/// File owner for <see cref="SyncRevisionsDocument"/> — dumb durable holder;
/// the engine owns the semantics, this class owns atomic reads/writes plus
/// corruption recovery.
/// </summary>
public sealed class SyncRevisionsStore
{
    private const int CurrentSchemaVersion = 1;

    private readonly string _revisionsPath;
    private int _loadedSchemaVersion = CurrentSchemaVersion;

    public SyncRevisionsStore(string? revisionsPath = null)
    {
        _revisionsPath = revisionsPath ?? Path.Combine(
            DeskBoxDataPathService.Current.DataDirectory,            "sync",            "revisions.json");
    }

    public Task<SyncRevisionsDocument> LoadAsync() =>
        ResilientJsonStore.LoadAsync(
            _revisionsPath,            json =>
            {
                // Fail closed on "parses but is not a document": `null` or a
                // record missing its entity map is corrupt state — throwing
                // routes the file through quarantine and .bak recovery
                // instead of silently dropping every recorded revision.
                SyncRevisionsDocument? document = JsonSerializer.Deserialize(
                    json,SyncRevisionsJsonContext.Default.SyncRevisionsDocument);
                if (document is null || document.Entities is null)
                {
                    throw new InvalidDataException(
                        "sync revisions.json is structurally empty.");
                }

                _loadedSchemaVersion = document.SchemaVersion;
                return document;
            },            static () => new SyncRevisionsDocument(),            "SyncRevisions");

    public async Task SaveAsync(SyncRevisionsDocument document)
    {
        // Same read-only stance as WidgetLayoutStore.CanWrite: a file stamped
        // by a newer build is pristine to this one — overwriting it would
        // drop fields the newer schema introduced. The cached stamp alone is
        // not enough: it only knows what THIS instance loaded, so a fresh
        // store saving before LoadAsync, or an external replace after our
        // load, would stamp v1 over a newer file. Read the on-disk stamp at
        // write time; an unreadable file falls through to the normal save
        // path (corruption recovery owns it, not the schema gate).
        int onDiskSchema = Math.Max(
            _loadedSchemaVersion,            await SyncStoreSchemaGate.ReadDiskSchemaVersionAsync(
                _revisionsPath,CurrentSchemaVersion));
        if (onDiskSchema > CurrentSchemaVersion)
        {
            throw new InvalidDataException(
                $"sync revisions.json schema {onDiskSchema} is newer " +
                $"than this build understands ({CurrentSchemaVersion}); " +
                "refusing to overwrite it.");
        }

        await ResilientJsonStore.SaveAsync(
            _revisionsPath,            JsonSerializer.SerializeToUtf8Bytes(
                document,SyncRevisionsJsonContext.Default.SyncRevisionsDocument));
    }

    /// <inheritdoc cref="WidgetLayoutStore.SaveCheckedAsync"/>
    public async Task<bool> SaveCheckedAsync(SyncRevisionsDocument document)
    {
        try
        {
            await SaveAsync(document);
            return true;
        }
        catch (Exception ex) when (ex is not OperationCanceledException)
        {
            App.Log($"[SyncRevisions] Save failed: {ex}");
            return false;
        }
    }
}
