namespace DeskBox.Tests;

/// <summary>
/// Pins the glance image streaming contract: every rotation source — local
/// sets of any size, online sets — loads through a stream so nothing enters
/// the framework's un-evictable URI-keyed image cache. The URI path remains
/// only as the stream-failure fallback. Size thresholds cannot bound the
/// cache because small rotation sets are the common case.
/// </summary>
public sealed class GlanceImageStreamingContractTests
{
    [Fact]
    public void RotationImages_AlwaysStreamRegardlessOfSetSizeOrOrigin()
    {
        string surface = ReadRepositoryFile(
            "src/DeskBox/Controls/WidgetContents/GlanceWidgetContent.xaml.cs");

        Assert.Contains(
            "GlanceImageStreamLoader.LoadAsync(bitmap, path);",
            surface,
            StringComparison.Ordinal);
        // The size-threshold branch must not come back: a threshold only
        // moved the accumulation to the common small-set case.
        Assert.DoesNotContain(
            "LargeLocalStreamingImageSetThreshold",
            surface,
            StringComparison.Ordinal);
        Assert.DoesNotContain(
            "IsCurrentImageOnline &&",
            surface,
            StringComparison.Ordinal);
    }

    [Fact]
    public void StreamLoader_FallsBackToUriWhenRandomAccessFails()
    {
        string loader = ReadRepositoryFile(
            "src/DeskBox/Services/GlanceImageStreamLoader.cs");

        Assert.Contains(
            "bitmap.SetSourceAsync(stream);",
            loader,
            StringComparison.Ordinal);
        Assert.Contains(
            "bitmap.UriSource = new Uri(path, UriKind.Absolute);",
            loader,
            StringComparison.Ordinal);
    }

    [Fact]
    public void CompactBackground_DecodesForTheActualCapsuleWidthAndStreams()
    {
        string adapter = ReadRepositoryFile(
            "src/DeskBox/Controls/WidgetContents/GlanceWidgetContentAdapter.cs");
        string host = ReadRepositoryFile(
            "src/DeskBox/Views/ContentWidgetWindow.xaml.cs");
        string calculator = ReadRepositoryFile(
            "src/DeskBox/Services/GlanceImageDecodeSizeCalculator.cs");

        // The fixed 768 decode paid ~3x on narrow capsules and under-decoded
        // wide ones; the width now follows the capsule.
        Assert.DoesNotContain("DecodePixelWidth = 768", adapter, StringComparison.Ordinal);
        Assert.Contains(
            "UsesAlignedCompactWidth()",
            host,
            StringComparison.Ordinal);
        Assert.Contains(
            "CompactBackgroundSupersamplingFactor",
            host,
            StringComparison.Ordinal);
        Assert.Contains(
            "internal const double CompactBackgroundSupersamplingFactor",
            calculator,
            StringComparison.Ordinal);
    }

    private static string ReadRepositoryFile(string relativePath) =>
        File.ReadAllText(TestPaths.FromRepository(relativePath));
}
