using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class FileWidgetHostDiagnosticsTests
{
    [Theory]
    [InlineData(null)]
    [InlineData("")]
    [InlineData("unified")]
    [InlineData("content")]
    public void SupportedUnifiedValues_DoNotReportFallback(string? value)
    {
        var diagnostics = new FileWidgetHostDiagnostics(value);

        DeskBoxFileHostDiagnostic snapshot = diagnostics.CreateSnapshot(2, 1);

        Assert.False(snapshot.LegacyFallbackAvailable);
        Assert.Equal("UnifiedContentOnly", snapshot.Strategy);
        Assert.Equal(0, snapshot.FallbackRequestCount);
        Assert.Null(snapshot.LastFallbackReason);
        Assert.Null(snapshot.UnifiedStandaloneUsagePercent);
        Assert.Equal(2, snapshot.LoadedStandaloneCount);
        Assert.Equal(1, snapshot.LoadedGroupedCount);
    }

    [Theory]
    [InlineData("legacy")]
    [InlineData("LEGACY")]
    [InlineData(" old ")]
    [InlineData("1")]
    [InlineData("true")]
    public void LegacyOverride_IsDiagnosedAsRemovedFallback(string value)
    {
        var diagnostics = new FileWidgetHostDiagnostics(value);

        DeskBoxFileHostDiagnostic snapshot = diagnostics.CreateSnapshot(0, 0);

        Assert.Equal(1, snapshot.FallbackRequestCount);
        Assert.Equal("LegacyHostRemoved", snapshot.LastFallbackReason);
        Assert.False(snapshot.LegacyFallbackAvailable);
    }

    [Fact]
    public void UnknownOverride_IsDiagnosedWithoutChangingUnifiedStrategy()
    {
        var diagnostics = new FileWidgetHostDiagnostics("unexpected");

        DeskBoxFileHostDiagnostic snapshot = diagnostics.CreateSnapshot(0, 0);

        Assert.Equal("UnifiedContentOnly", snapshot.Strategy);
        Assert.Equal(1, snapshot.FallbackRequestCount);
        Assert.Equal("UnsupportedHostOverride", snapshot.LastFallbackReason);
    }

    [Fact]
    public void UnifiedCreationMetrics_RemainExplicitAfterLegacyRemoval()
    {
        var diagnostics = new FileWidgetHostDiagnostics(null);
        diagnostics.RecordUnifiedCreation();
        diagnostics.RecordUnifiedCreation();

        DeskBoxFileHostDiagnostic snapshot = diagnostics.CreateSnapshot(1, 1);

        Assert.Equal(2, snapshot.TotalStandaloneCreationCount);
        Assert.Equal(2, snapshot.UnifiedStandaloneCreationCount);
        Assert.Equal(0, snapshot.LegacyStandaloneCreationCount);
        Assert.Equal(100d, snapshot.UnifiedStandaloneUsagePercent);
    }

    [Fact]
    public void LegacyWidgetWindowImplementation_IsDeletedAndManagerIsHostNeutral()
    {
        string repositoryRoot = FindRepositoryRoot();
        string viewsPath = Path.Combine(repositoryRoot, "src", "DeskBox", "Views");
        string servicesPath = Path.Combine(repositoryRoot, "src", "DeskBox", "Services");
        string managerSource = string.Join(
            Environment.NewLine,
            Directory.EnumerateFiles(
                    servicesPath,
                    "WidgetManager*.cs",
                    SearchOption.TopDirectoryOnly)
                .Select(File.ReadAllText));

        Assert.Empty(Directory.EnumerateFiles(
            viewsPath,
            "WidgetWindow.*",
            SearchOption.TopDirectoryOnly));
        Assert.DoesNotMatch(@"\bWidgetWindow\b", managerSource);
        Assert.DoesNotContain("FileWidgetHostMode", managerSource, StringComparison.Ordinal);
        Assert.DoesNotContain("CreateLegacyFileWidget", managerSource, StringComparison.Ordinal);
    }

    [Fact]
    public void FileWidgetSession_HasNoLegacyLifecycleAdapters()
    {
        string source = File.ReadAllText(Path.Combine(
            FindRepositoryRoot(),
            "src",
            "DeskBox",
            "Services",
            "FileWidgetSession.cs"));

        Assert.DoesNotContain("InitializeAsync", source, StringComparison.Ordinal);
        Assert.DoesNotContain("TrackTheme", source, StringComparison.Ordinal);
        Assert.DoesNotContain("PresentInitialDesktopWindow", source, StringComparison.Ordinal);
        Assert.DoesNotContain("RegisterClosedCallback", source, StringComparison.Ordinal);
        Assert.DoesNotContain("DisposeContent", source, StringComparison.Ordinal);
    }

    private static string FindRepositoryRoot()
    {
        DirectoryInfo? current = new(AppContext.BaseDirectory);
        while (current is not null)
        {
            if (File.Exists(Path.Combine(
                    current.FullName,
                    "src",
                    "DeskBox",
                    "DeskBox.csproj")))
            {
                return current.FullName;
            }

            current = current.Parent;
        }

        throw new DirectoryNotFoundException(
            "DeskBox repository root was not found.");
    }
}
