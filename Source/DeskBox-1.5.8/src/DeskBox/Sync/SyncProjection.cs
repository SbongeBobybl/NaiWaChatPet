using System.Security.Cryptography;
using System.Text.Json;
using System.Text.Json.Nodes;
using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Sync;

/// <summary>
/// Local domain record → <see cref="SyncEnvelope"/> projection
/// (sync-protocol-contract §3.3). Pure transformation.
///
/// Only the <c>widget-style</c> domain survives the retirement of the Todo and
/// QuickCapture widgets; the <c>todo-data</c> and <c>quick-capture-data</c>
/// projections went away with their payload DTOs. Their queued envelopes are
/// not drained or rewritten — the outbox simply stops projecting new ones.
/// </summary>
public static class SyncProjection
{
    /// <summary>Per-domain payload schema versions (envelope.schema_version,
    /// §3.1). Bump the domain's version only when its payload shape changes.</summary>
    public const int WidgetStyleSchemaVersion = 1;

    /// <summary>widget-style: singleton collection; the "shell" entity
    /// carries every shell-whitelist key, each widget id carries its style
    /// patch. The whitelist is enforced by the same code path that builds
    /// backup style documents — there is no second list to drift (§3.3).</summary>
    public static IEnumerable<SyncEnvelope> FromWidgetStyle(AppSettings settings)
    {
        ArgumentNullException.ThrowIfNull(settings);

        JsonObject document = JsonNode
            .Parse(WidgetStyleBackupProjection.Serialize(settings))!
            .AsObject();

        if (document["shell"] is JsonObject shell)
        {
            yield return new SyncEnvelope
            {
                Domain = SyncDomains.WidgetStyle,
                CollectionId = SyncDomains.WidgetStyleCollection,
                EntityId = SyncDomains.ShellEntityId,
                SchemaVersion = WidgetStyleSchemaVersion,
                DeviceId = DeviceIdentity.Id,
                OperationId = Guid.NewGuid().ToString("N"),
                Payload = (JsonObject)shell.DeepClone()
            };
        }

        if (document["widgets"] is JsonObject widgets)
        {
            foreach ((string widgetId, JsonNode? style) in widgets)
            {
                if (style is not JsonObject styleObject)
                {
                    continue;
                }

                yield return new SyncEnvelope
                {
                    Domain = SyncDomains.WidgetStyle,
                    CollectionId = SyncDomains.WidgetStyleCollection,
                    EntityId = widgetId,
                    SchemaVersion = WidgetStyleSchemaVersion,
                    DeviceId = DeviceIdentity.Id,
                    OperationId = Guid.NewGuid().ToString("N"),
                    Payload = (JsonObject)styleObject.DeepClone()
                };
            }
        }
    }

    /// <summary>Tombstone envelope for a locally deleted entity (§4):
    /// deleted=true, no payload, no blob references.</summary>
    public static SyncEnvelope Tombstone(string domain,string collectionId,string entityId)
    {
        if (!SyncDomains.IsKnownDomain(domain))
        {
            throw new ArgumentException($"Unknown sync domain '{domain}'.",nameof(domain));
        }
        ArgumentException.ThrowIfNullOrWhiteSpace(collectionId);
        ArgumentException.ThrowIfNullOrWhiteSpace(entityId);

        return new SyncEnvelope
        {
            Domain = domain,
            CollectionId = collectionId,
            EntityId = entityId,
            Deleted = true,
            DeviceId = DeviceIdentity.Id,
            OperationId = Guid.NewGuid().ToString("N")
        };
    }

    /// <summary>Rewrites an attachments array in place and collects blob
    /// references for the files that exist locally. "managed" alone does not
    /// earn a blob: the path must provably resolve inside
    /// <paramref name="managedAttachmentRoot"/>, otherwise the record is
    /// degraded to a basename stub and its bytes are never read.</summary>
    private static bool TryGetString(JsonNode? node,out string? value)
    {
        value = node is JsonValue jsonValue && jsonValue.TryGetValue(out string? s) ? s : null;
        return node is not null;
    }
}
