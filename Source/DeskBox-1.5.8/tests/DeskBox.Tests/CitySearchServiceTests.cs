using DeskBox.Helpers;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class CitySearchServiceTests
{
    [Theory]
    [InlineData("New York", "newyork")]
    [InlineData("São Paulo", "saopaulo")]
    [InlineData("  New-York  ", "newyork")]
    public void NormalizeSearchText_IgnoresSpacingPunctuationAndDiacritics(
        string input,
        string expected)
    {
        Assert.Equal(expected, CitySearchService.NormalizeSearchText(input));
    }

    [Fact]
    public void SearchLocal_NewYorkWithoutSpace_ReturnsNewYorkCityFirst()
    {
        var results = CitySearchService.SearchLocal("newyork", isEn: true);

        var result = Assert.IsType<DeskBox.Models.WeatherCitySearchResult>(results.First());
        Assert.Equal("New York", result.Name);
        Assert.InRange(result.Latitude, 40.70, 40.73);
        Assert.InRange(result.Longitude, -74.02, -73.99);
    }

    [Fact]
    public void SearchLocal_TraditionalChinese_ConvertsLocalCityNames()
    {
        var results = CitySearchService.SearchLocal("harbin", isEn: false, useTraditional: true);

        var result = Assert.IsType<DeskBox.Models.WeatherCitySearchResult>(results.First());
        Assert.Equal("哈爾濱", result.Name);
    }

    [Fact]
    public void NonChineseLocale_UsesEnglishLocalCityNames()
    {
        using var service = new CitySearchService();

        var result = service.GetGlobalPopularCities("ja-JP", maxCount: 1).Single();

        Assert.Equal("Beijing", result.Name);
    }

    [Fact]
    public void ChineseTextConverter_MapsBothWritingSystems()
    {
        const string simplified = "文件夹与台湾腊月闰月";
        string traditional = ChineseTextConverter.ToTraditional(simplified);

        Assert.Equal("文件夾與臺灣臘月閏月", traditional);
        Assert.Equal(simplified, ChineseTextConverter.ToSimplified(traditional));
    }

    [Theory]
    [InlineData("春节", "春節")]
    [InlineData("重阳", "重陽")]
    [InlineData("腊八", "臘八")]
    [InlineData("文件夹与台湾腊月闰月", "文件夾與臺灣臘月閏月")]
    public void ChineseTextConverter_RepeatedConversionsDoNotAppendBufferData(
        string simplified,
        string traditional)
    {
        for (int iteration = 0; iteration < 256; iteration++)
        {
            Assert.Equal(traditional, ChineseTextConverter.ToTraditional(simplified));
            Assert.Equal(simplified, ChineseTextConverter.ToSimplified(traditional));
        }
    }

    [Theory]
    [InlineData("zh-TW", "zh")]
    [InlineData("zh-Hant", "zh")]
    [InlineData("de-DE", "de")]
    [InlineData("pt-BR", "pt")]
    public void GeocodingLanguage_UsesApiSupportedLanguageCode(string culture, string expected)
    {
        Assert.Equal(expected, WeatherService.NormalizeGeocodingLanguage(culture));
    }
}
