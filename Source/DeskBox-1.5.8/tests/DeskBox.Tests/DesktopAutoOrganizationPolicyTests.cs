using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class DesktopAutoOrganizationPolicyTests
{
    [Fact]
    public void Presets_CoverTheOfferedTimingChoicesInAscendingOrder()
    {
        Assert.Equal(
            [10, 60, 300, 1800, 3600, 43200],
            DesktopAutoOrganizationPolicy.SupportedDelaySeconds);
    }

    [Theory]
    [InlineData(10)]
    [InlineData(60)]
    [InlineData(300)]
    [InlineData(1800)]
    [InlineData(3600)]
    [InlineData(43200)]
    public void PresetValues_PassThroughUntouched(int seconds)
    {
        Assert.Equal(seconds, DesktopAutoOrganizationPolicy.NormalizeDelaySeconds(seconds));
    }

    [Theory]
    [InlineData(0)]
    [InlineData(3)]
    [InlineData(45)]
    [InlineData(-60)]
    public void HandEditedValues_SnapBackToTheRealtimeDefault(int seconds)
    {
        Assert.Equal(
            DesktopAutoOrganizationPolicy.DefaultDelaySeconds,
            DesktopAutoOrganizationPolicy.NormalizeDelaySeconds(seconds));
    }

    [Fact]
    public void GetDelay_ReadsTheSliceAndNeverDropsBelowTenSeconds()
    {
        var settings = new AppSettings();
        Assert.Equal(
            TimeSpan.FromSeconds(10),
            DesktopAutoOrganizationPolicy.GetDelay(settings));

        settings.DesktopOrganization.DesktopAutoOrganizationDelaySeconds = 1800;
        Assert.Equal(
            TimeSpan.FromMinutes(30),
            DesktopAutoOrganizationPolicy.GetDelay(settings));

        settings.DesktopOrganization.DesktopAutoOrganizationDelaySeconds = 7;
        Assert.Equal(
            TimeSpan.FromSeconds(10),
            DesktopAutoOrganizationPolicy.GetDelay(settings));
    }
}
