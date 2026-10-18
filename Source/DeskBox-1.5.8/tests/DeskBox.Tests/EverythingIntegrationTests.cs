using System.Security.Cryptography;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class EverythingIntegrationTests
{
    [Theory]
    [InlineData("budget report", false, "\"budget report\"")]
    [InlineData("ext:pdf budget", true, "ext:pdf budget")]
    [InlineData("  notes  ", false, "\"notes\"")]
    public void ProviderQuery_IsLiteralUnlessAdvancedSyntaxIsEnabled(
        string query,
        bool advancedSyntax,
        string expected)
    {
        Assert.Equal(expected, EverythingSearchService.BuildProviderQuery(query, advancedSyntax));
    }

    [Theory]
    [InlineData("report.txt", "report.txt", 100)]
    [InlineData("report.txt", "report", 90)]
    [InlineData("report-final.txt", "report", 80)]
    [InlineData("annual-report.txt", "report", 50)]
    public void ProviderResultRelevance_PreservesDeskBoxRanking(
        string fileName,
        string query,
        double expected)
    {
        Assert.Equal(expected, EverythingSearchService.ComputeRelevance(fileName, query));
    }

    [Fact]
    public void LegacyCleanup_RemovesOnlyDeskBoxOwnedIndexArtifacts()
    {
        string root = Path.Combine(
            Path.GetTempPath(),
            "DeskBox-LegacySearchCleanup-" + Guid.NewGuid().ToString("N"));
        string cache = Path.Combine(root, "cache");
        Directory.CreateDirectory(cache);
        string[] obsolete =
        [
            Path.Combine(cache, "search-index.json"),
            Path.Combine(cache, "search-index-v2.json.tmp"),
            Path.Combine(cache, "search-index-v2.json.roots")
        ];
        string retained = Path.Combine(cache, "search-history.json");
        try
        {
            foreach (string path in obsolete)
            {
                File.WriteAllText(path, "obsolete");
            }

            File.WriteAllText(retained, "keep");
            Assert.Equal(obsolete.Length, LegacySearchIndexCleanupService.TryCleanup(root));
            Assert.All(obsolete, path => Assert.False(File.Exists(path)));
            Assert.True(File.Exists(retained));
        }
        finally
        {
            if (Directory.Exists(root))
            {
                Directory.Delete(root, recursive: true);
            }
        }
    }

    [Fact]
    public void ProjectPackagesOfficialSdkForBothSupportedArchitectures()
    {
        string project = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/DeskBox.csproj"));
        string cargo = File.ReadAllText(TestPaths.FromRepository("native/Cargo.toml"));

        Assert.Contains("ThirdParty\\Everything\\Everything64.dll", project, StringComparison.Ordinal);
        Assert.Contains("ThirdParty\\Everything\\EverythingARM64.dll", project, StringComparison.Ordinal);
        Assert.Contains("<TargetPath>EverythingSdk.dll</TargetPath>", project, StringComparison.Ordinal);
        Assert.DoesNotContain("DeskBoxSearchCore", project, StringComparison.Ordinal);
        Assert.DoesNotContain("deskbox-search-core", cargo, StringComparison.Ordinal);

        Assert.Equal(
            "81B5BE18126ACD2C2B913F8F4A821E476B18393CDD3DEBD03387C50AFD8DB88F",
            Hash("src/DeskBox/ThirdParty/Everything/Everything64.dll"));
        Assert.Equal(
            "8531EA393677DD8FD37BED7420AC93344CD458B9A1324BA65C4A75D024D61886",
            Hash("src/DeskBox/ThirdParty/Everything/EverythingARM64.dll"));
    }

    private static string Hash(string relativePath)
    {
        using FileStream stream = File.OpenRead(TestPaths.FromRepository(relativePath));
        return Convert.ToHexString(SHA256.HashData(stream));
    }
}
