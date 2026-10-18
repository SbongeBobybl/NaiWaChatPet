using System.Text.Json;
using System.Text.Json.Serialization;
using DeskBox.Services;

namespace DeskBox.Core.Persistence;

[JsonSourceGenerationOptions(
    GenerationMode = JsonSourceGenerationMode.Metadata,
    PropertyNamingPolicy = JsonKnownNamingPolicy.CamelCase,
    WriteIndented = true)]
[JsonSerializable(
    typeof(SyncStateDocument),
    TypeInfoPropertyName = "SyncStateDocument")]
internal sealed partial class SyncStateJsonContext : JsonSerializerContext
{
}

/// <summary>
/// data/sync/state.json — per-domain pull cursors, the account's epoch, and
/// the account summary (sync-protocol-contract §8). Device-domain protocol
/// state: never backed up, never synced.
/// </summary>
public sealed class SyncStateDocument
{
    public int SchemaVersion { get; set; } = 1;

    /// <summary>Server-side user id once an account is linked; null when
    /// sync is unconfigured.</summary>
    public string? AccountId { get; set; }

    /// <summary>Per-domain pull state keyed by the wire domain name
    /// (todo-data / quick-capture-data / widget-style).</summary>
    public Dictionary<string, SyncDomainState> Domains { get; set; } = new();
}

public sealed class SyncDomainState
{
    /// <summary>Opaque per-domain pull cursor issued by the server (§4).</summary>
    public string Cursor { get; set; } = string.Empty;

    /// <summary>Account epoch last seen on pull; a mismatch switches the
    /// domain to full-snapshot replace (§4).</summary>
    public long Epoch { get; set; }

    public DateTimeOffset? LastSyncAtUtc { get; set; }
}

/// <summary>
/// File owner for <see cref="SyncStateDocument"/>. Dumb durable holder —
/// the engine (a later module) owns the semantics; this class owns atomic
/// reads/writes plus corruption recovery.
/// </summary>
public sealed class SyncStateStore
{
    private const int CurrentSchemaVersion = 1;

    private readonly string _statePath;
    private int _loadedSchemaVersion = CurrentSchemaVersion;

    public SyncStateStore(string? statePath = null)
    {
        _statePath = statePath ?? Path.Combine(
            DeskBoxDataPathService.Current.DataDirectory,            "sync",            "state.json");
    }

    public Task<SyncStateDocument> LoadAsync() =>
        ResilientJsonStore.LoadAsync(
            _statePath,            json =>
            {
                // Fail closed on "parses but is not a document": `null` or a
                // record missing its required collections is corrupt state,
                // not an empty store — throwing routes the file through
                // quarantine and .bak recovery instead of silently starting
                // over with account id, cursors and epoch all dropped.
                SyncStateDocument? document = JsonSerializer.Deserialize(
                    json,SyncStateJsonContext.Default.SyncStateDocument);
                if (document is null || document.Domains is null)
                {
                    throw new InvalidDataException(
                        "sync state.json is structurally empty.");
                }

                _loadedSchemaVersion = document.SchemaVersion;
                return document;
            },            static () => new SyncStateDocument(),            "SyncState");

    public async Task SaveAsync(SyncStateDocument document)
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
                _statePath,CurrentSchemaVersion));
        if (onDiskSchema > CurrentSchemaVersion)
        {
            throw new InvalidDataException(
                $"sync state.json schema {onDiskSchema} is newer than " +
                $"this build understands ({CurrentSchemaVersion}); " +
                "refusing to overwrite it.");
        }

        await ResilientJsonStore.SaveAsync(
            _statePath,            JsonSerializer.SerializeToUtf8Bytes(
                document,SyncStateJsonContext.Default.SyncStateDocument));
    }

    /// <inheritdoc cref="WidgetLayoutStore.SaveCheckedAsync"/>
    public async Task<bool> SaveCheckedAsync(SyncStateDocument document)
    {
        try
        {
            await SaveAsync(document);
            return true;
        }
        catch (Exception ex) when (ex is not OperationCanceledException)
        {
            App.Log($"[SyncState] Save failed: {ex}");
            return false;
        }
    }
}

/// <summary>
/// Write-time schema gate shared by the sync durable stores: reads just the
/// schemaVersion stamp of the file currently on disk. An absent or
/// unreadable file reports the fallback — corruption recovery and the
/// normal save path own those cases; this gate only refuses a file that
/// clearly declares a schema newer than the build understands.
/// </summary>
internal static class SyncStoreSchemaGate
{
    public static async Task<int> ReadDiskSchemaVersionAsync(
        string storePath,        int fallbackVersion)
    {
        if (!File.Exists(storePath))
        {
            return fallbackVersion;
        }

        try
        {
            string json = await File.ReadAllTextAsync(storePath);
            using var document = JsonDocument.Parse(json);
            return document.RootElement.TryGetProperty("schemaVersion",out JsonElement element) &&
                   element.TryGetInt32(out int version)
                ? version
                : fallbackVersion;
        }
        catch
        {
            return fallbackVersion;
        }
    }
}
