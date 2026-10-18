namespace DeskBox.Sync;

/// <summary>
/// Wire names for the syncable domains and their collection/entity
/// conventions (sync-protocol-contract §2). The strings intentionally match
/// the cloud-backup manifest names — the same semantic boundaries carry both
/// transports; a contract test pins the equality so they cannot drift.
/// </summary>
/// <remarks>
/// The <c>todo-data</c> and <c>quick-capture-data</c> domains are retired
/// together with the Todo and QuickCapture widgets: their payload DTOs no
/// longer exist, so no envelope can be projected for them. Only
/// <c>widget-style</c> remains syncable. An outbox directory left over from an
/// older build still loads — unknown domain files are skipped rather than
/// rewritten — so no queued record is destroyed by the retirement.
/// </remarks>
public static class SyncDomains
{
    public const string WidgetStyle = "widget-style";

    /// <summary>Singleton collection ids (§2.1): non-widget collections that
    /// merge across devices by construction.</summary>
    public const string WidgetStyleCollection = "widget-style";

    /// <summary>The shell entity inside the widget-style singleton collection.</summary>
    public const string ShellEntityId = "shell";

    public static bool IsKnownDomain(string? domain) =>
        domain is WidgetStyle;
}
