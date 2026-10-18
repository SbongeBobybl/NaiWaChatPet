using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class SystemFontCatalogServiceTests
{
    [Fact]
    public void NormalizeFontFamilies_RemovesVerticalAliases_BlanksAndDuplicates()
    {
        var result = SystemFontCatalogService.NormalizeFontFamilies(
            [" Arial ", "arial", "@Vertical", "", "  ", null, "Segoe UI"]);

        Assert.Equal(["Arial", "Segoe UI"], result);
    }

    [Fact]
    public void GetFontFamilies_ReturnsStableSortedCollection()
    {
        var service = new SystemFontCatalogService();

        var first = service.GetFontFamilies();
        var second = service.GetFontFamilies();

        Assert.Same(first, second);
        Assert.Equal(first.OrderBy(static value => value, StringComparer.CurrentCultureIgnoreCase), first);
        Assert.DoesNotContain(first, static value => string.IsNullOrWhiteSpace(value));
        Assert.DoesNotContain(first, static value => value.StartsWith("@", StringComparison.Ordinal));
    }
}
