using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class PerformanceLoggerTests
{
    [Theory]
    [InlineData(null, false)]
    [InlineData("", false)]
    [InlineData("0", false)]
    [InlineData("false", false)]
    [InlineData("1", true)]
    [InlineData("true", true)]
    [InlineData("TRUE", true)]
    [InlineData("yes", true)]
    [InlineData("on", true)]
    [InlineData("enabled", true)]
    public void IsEnabledSetting_ParsesOptInValues(string? value, bool expected)
    {
        Assert.Equal(expected, PerformanceLogger.IsEnabledSetting(value));
    }

    [Theory]
    [InlineData(0, 0, 0.0)]
    [InlineData(3, 1, 75.0)]
    [InlineData(1, 3, 25.0)]
    [InlineData(-1, 2, 0.0)]
    public void CalculateHitRatePercent_UsesNonNegativeLookupCounts(
        long hits,
        long misses,
        double expected)
    {
        Assert.Equal(
            expected,
            PerformanceLogger.CalculateHitRatePercent(hits, misses),
            precision: 3);
    }

    [Theory]
    [InlineData(0, 0, 0.0)]
    [InlineData(200_000, 2, 10.0)]
    [InlineData(-1, 2, 0.0)]
    public void CalculateAverageDurationMilliseconds_UsesCompletedLoadCount(
        long totalDurationTicks,
        long sampleCount,
        double expected)
    {
        Assert.Equal(
            expected,
            PerformanceLogger.CalculateAverageDurationMilliseconds(
                totalDurationTicks,
                sampleCount),
            precision: 3);
    }
}
