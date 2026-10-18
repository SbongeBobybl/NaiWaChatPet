using DeskBox.Controls;

namespace DeskBox.Tests;

public sealed class MasterDetailLayoutPolicyTests
{
    private readonly MasterDetailLayoutPolicy _policy = new();

    [Theory]
    [InlineData(679, false, MasterDetailLayoutMode.SinglePane)]
    [InlineData(700, false, MasterDetailLayoutMode.SinglePane)]
    [InlineData(720, false, MasterDetailLayoutMode.DualPane)]
    [InlineData(700, true, MasterDetailLayoutMode.DualPane)]
    [InlineData(679, true, MasterDetailLayoutMode.SinglePane)]
    public void Resolve_AppliesEnterExitHysteresis(
        double width,
        bool wasDualPane,
        MasterDetailLayoutMode expected)
    {
        MasterDetailLayoutSnapshot result = _policy.Resolve(width, wasDualPane);

        Assert.Equal(expected, result.Mode);
    }

    [Fact]
    public void Resolve_ClampsPersistedWidthAndProtectsDetailPane()
    {
        MasterDetailLayoutSnapshot narrowMaster = _policy.Resolve(720, false, 100);
        MasterDetailLayoutSnapshot wideMaster = _policy.Resolve(800, false, 700);

        Assert.Equal(240, narrowMaster.MasterWidth);
        Assert.Equal(420, wideMaster.MasterWidth);
        Assert.Equal(8, wideMaster.SplitterWidth);
        Assert.Equal(372, wideMaster.DetailWidth);
    }

    [Fact]
    public void Resolve_UsesDefaultWidthAndSupportsForcedSinglePane()
    {
        MasterDetailLayoutSnapshot automatic = _policy.Resolve(900, false, double.NaN);
        MasterDetailLayoutSnapshot forced = _policy.Resolve(
            900,
            true,
            300,
            MasterDetailLayoutPreference.SinglePane);

        Assert.Equal(300, automatic.MasterWidth);
        Assert.Equal(592, automatic.DetailWidth);
        Assert.False(forced.IsDualPane);
        Assert.Equal(0, forced.SplitterWidth);
    }

    [Fact]
    public void Resolve_ForcedDualPaneNeverFallsBackToSinglePane()
    {
        MasterDetailLayoutSnapshot atSafetyMinimum = _policy.Resolve(
            588,
            false,
            forceDualPane: true);
        MasterDetailLayoutSnapshot narrow = _policy.Resolve(
            300,
            false,
            forceDualPane: true);

        Assert.True(atSafetyMinimum.IsDualPane);
        Assert.Equal(240, atSafetyMinimum.MasterWidth);
        Assert.Equal(340, atSafetyMinimum.DetailWidth);
        Assert.True(narrow.IsDualPane);
        Assert.Equal(8, narrow.SplitterWidth);
        Assert.Equal(292, narrow.MasterWidth + narrow.DetailWidth, precision: 6);
    }

    [Theory]
    [InlineData(0, MasterDetailLayoutMode.DualPane)]
    [InlineData(150, MasterDetailLayoutMode.DualPane)]
    [InlineData(587, MasterDetailLayoutMode.DualPane)]
    [InlineData(588, MasterDetailLayoutMode.DualPane)]
    [InlineData(700, MasterDetailLayoutMode.DualPane)]
    public void Resolve_ForcedDualPaneUsesPhysicalSafetyLimit(
        double width,
        MasterDetailLayoutMode expected)
    {
        MasterDetailLayoutSnapshot result = _policy.Resolve(
            width,
            false,
            300,
            MasterDetailLayoutPreference.DualPane);

        Assert.Equal(expected, result.Mode);
    }

    [Fact]
    public void Resolve_ForcedDualPaneCompressesBothPanesBelowMinimumWidth()
    {
        MasterDetailLayoutSnapshot result = _policy.Resolve(
            320,
            false,
            300,
            MasterDetailLayoutPreference.DualPane);

        Assert.True(result.IsDualPane);
        Assert.Equal(320, result.MasterWidth + result.SplitterWidth + result.DetailWidth, 6);
        Assert.True(result.MasterWidth > 0);
        Assert.True(result.DetailWidth > 0);
    }

    [Theory]
    [InlineData(null, 300)]
    [InlineData(double.NaN, 300)]
    [InlineData(100.0, 240.0)]
    [InlineData(500.0, 420.0)]
    [InlineData(318.0, 318.0)]
    public void NormalizePersistedMasterWidth_UsesSafeRange(double? value, double expected)
    {
        Assert.Equal(expected, _policy.NormalizePersistedMasterWidth(value));
    }

    [Fact]
    public void InvalidOptions_AreRejected()
    {
        var options = new MasterDetailLayoutOptions(
            DualPaneEnterWidth: 680,
            DualPaneExitWidth: 720);

        Assert.Throws<ArgumentOutOfRangeException>(() => new MasterDetailLayoutPolicy(options));
    }
}
