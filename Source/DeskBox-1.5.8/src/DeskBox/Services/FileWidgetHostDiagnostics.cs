namespace DeskBox.Services;

/// <summary>
/// Tracks adoption of the unified file-widget host. The legacy host has been
/// removed; the former environment override is read only so support bundles
/// can explain why an obsolete fallback request was ignored.
/// </summary>
internal sealed class FileWidgetHostDiagnostics
{
    internal const string LegacyOverrideEnvironmentVariableName =
        "DESKBOX_FILE_WIDGET_HOST";

    private const string UnifiedOnlyStrategy = "UnifiedContentOnly";
    private const string LegacyHostRemovedReason = "LegacyHostRemoved";
    private const string UnsupportedOverrideReason = "UnsupportedHostOverride";

    private long _unifiedCreationCount;

    internal FileWidgetHostDiagnostics()
        : this(Environment.GetEnvironmentVariable(
            LegacyOverrideEnvironmentVariableName))
    {
    }

    internal FileWidgetHostDiagnostics(string? configuredValue)
    {
        string normalized = configuredValue?.Trim().ToLowerInvariant() ?? string.Empty;
        if (normalized.Length == 0 || normalized == "unified" || normalized == "content")
        {
            return;
        }

        FallbackRequestCount = 1;
        LastFallbackReason = normalized is "legacy" or "old" or "1" or "true"
            ? LegacyHostRemovedReason
            : UnsupportedOverrideReason;
    }

    internal int FallbackRequestCount { get; }

    internal string? LastFallbackReason { get; }

    internal void RecordUnifiedCreation()
    {
        Interlocked.Increment(ref _unifiedCreationCount);
    }

    internal DeskBoxFileHostDiagnostic CreateSnapshot(
        int loadedStandaloneCount,        int loadedGroupedCount)
    {
        long unifiedCreationCount = Interlocked.Read(ref _unifiedCreationCount);
        const long legacyCreationCount = 0;
        long totalCreationCount = unifiedCreationCount + legacyCreationCount;
        double? unifiedUsagePercent = totalCreationCount == 0
            ? null
            : Math.Round(
                unifiedCreationCount * 100d / totalCreationCount,                2,                MidpointRounding.AwayFromZero);

        return new DeskBoxFileHostDiagnostic(
            UnifiedOnlyStrategy,            LegacyFallbackAvailable: false,            totalCreationCount,            unifiedCreationCount,            legacyCreationCount,            unifiedUsagePercent,            loadedStandaloneCount,            loadedGroupedCount,            FallbackRequestCount,            LastFallbackReason);
    }
}
