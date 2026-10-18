using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WidgetCompactExpansionDirectionPolicyTests
{
    [Fact]
    public void Auto_PreservesAutomaticAnchorOrder()
    {
        WidgetCompactExpansionAnchor[] automatic =
        [
            WidgetCompactExpansionAnchor.RightBottom,
            WidgetCompactExpansionAnchor.RightTop,
            WidgetCompactExpansionAnchor.LeftBottom
        ];

        IReadOnlyList<WidgetCompactExpansionAnchor> result =
            WidgetCompactExpansionDirectionPolicy.Apply(
                SettingsService.WidgetCompactExpansionDirectionAuto,
                automatic);

        Assert.Equal(automatic, result);
    }

    [Theory]
    [InlineData(
        SettingsService.WidgetCompactExpansionDirectionDown,
        WidgetCompactExpansionAnchor.RightTop,
        WidgetCompactExpansionAnchor.LeftTop)]
    [InlineData(
        SettingsService.WidgetCompactExpansionDirectionUp,
        WidgetCompactExpansionAnchor.RightBottom,
        WidgetCompactExpansionAnchor.LeftBottom)]
    public void FixedDirection_ChangesOnlyVerticalAnchorAndKeepsHorizontalPreference(
        string direction,
        WidgetCompactExpansionAnchor expectedFirst,
        WidgetCompactExpansionAnchor expectedSecond)
    {
        IReadOnlyList<WidgetCompactExpansionAnchor> result =
            WidgetCompactExpansionDirectionPolicy.Apply(
                direction,
                [
                    WidgetCompactExpansionAnchor.RightBottom,
                    WidgetCompactExpansionAnchor.RightTop,
                    WidgetCompactExpansionAnchor.LeftBottom,
                    WidgetCompactExpansionAnchor.LeftTop
                ]);

        Assert.Equal(new[] { expectedFirst, expectedSecond }, result);
    }

    [Theory]
    [InlineData(
        SettingsService.WidgetCompactExpansionDirectionDown,
        WidgetCompactExpansionAnchor.LeftTop,
        WidgetCompactExpansionAnchor.RightTop)]
    [InlineData(
        SettingsService.WidgetCompactExpansionDirectionUp,
        WidgetCompactExpansionAnchor.LeftBottom,
        WidgetCompactExpansionAnchor.RightBottom)]
    public void FixedDirection_UsesBothHorizontalFallbacksWhenAutomaticOrderIsEmpty(
        string direction,
        WidgetCompactExpansionAnchor expectedFirst,
        WidgetCompactExpansionAnchor expectedSecond)
    {
        IReadOnlyList<WidgetCompactExpansionAnchor> result =
            WidgetCompactExpansionDirectionPolicy.Apply(direction, []);

        Assert.Equal(new[] { expectedFirst, expectedSecond }, result);
    }

    [Theory]
    [InlineData(SettingsService.WidgetCompactExpansionDirectionAuto, false)]
    [InlineData(SettingsService.WidgetCompactExpansionDirectionDown, true)]
    [InlineData(SettingsService.WidgetCompactExpansionDirectionUp, true)]
    public void DirectionPolicy_OnlyMarksFixedDirectionsAsStrict(
        string direction,
        bool expected)
    {
        Assert.Equal(
            expected,
            WidgetCompactExpansionDirectionPolicy.RequiresFullSize(direction));
    }

    [Fact]
    public void ResolveAdaptive_FixedDownNearBottom_KeepsDirectionAndShrinksSize()
    {
        var workArea = new Windows.Graphics.RectInt32(0, 0, 1920, 1080);
        var compact = new Windows.Graphics.RectInt32(800, 1020, 300, 52);
        var requested = new Windows.Graphics.SizeInt32(600, 500);

        var layout = WidgetCompactExpansionDirectionPolicy.ResolveAdaptive(
            compact,
            requested,
            workArea,
            [WidgetCompactExpansionAnchor.LeftTop, WidgetCompactExpansionAnchor.RightTop],
            SettingsService.WidgetCompactExpansionDirectionDown);

        Assert.True(layout.CanExpand);
        Assert.True(layout.IsSizeConstrained);
        Assert.Equal(WidgetCompactExpansionAnchor.LeftTop, layout.Anchor);
        Assert.Equal(60, layout.ExpandedBounds.Height);
    }

    [Fact]
    public void ResolveAdaptive_AutoNearBottom_FlipsToUpwardAnchorAtFullSize()
    {
        var workArea = new Windows.Graphics.RectInt32(0, 0, 1920, 1080);
        var compact = new Windows.Graphics.RectInt32(800, 1020, 300, 52);
        var requested = new Windows.Graphics.SizeInt32(600, 500);

        var layout = WidgetCompactExpansionDirectionPolicy.ResolveAdaptive(
            compact,
            requested,
            workArea,
            [
                WidgetCompactExpansionAnchor.LeftBottom,
                WidgetCompactExpansionAnchor.RightBottom,
                WidgetCompactExpansionAnchor.LeftTop,
                WidgetCompactExpansionAnchor.RightTop
            ],
            SettingsService.WidgetCompactExpansionDirectionAuto);

        Assert.True(layout.CanExpand);
        Assert.False(layout.IsSizeConstrained);
        Assert.Equal(500, layout.ExpandedBounds.Height);
        Assert.True(layout.Anchor is
            WidgetCompactExpansionAnchor.LeftBottom or
            WidgetCompactExpansionAnchor.RightBottom);
    }

    [Fact]
    public void PerWidgetOverride_RoundTripsThroughConfigMetadata()
    {
        var config = new DeskBox.Models.WidgetConfig();

        Assert.Null(WidgetCompactExpansionDirectionPolicy.GetOverride(config));
        Assert.Equal(
            SettingsService.WidgetCompactExpansionDirectionDown,
            WidgetCompactExpansionDirectionPolicy.ResolveEffective(config, "Down"));

        WidgetCompactExpansionDirectionPolicy.SetOverride(
            config, SettingsService.WidgetCompactExpansionDirectionUp);

        Assert.Equal(
            SettingsService.WidgetCompactExpansionDirectionUp,
            WidgetCompactExpansionDirectionPolicy.GetOverride(config));
        Assert.Equal(
            SettingsService.WidgetCompactExpansionDirectionUp,
            WidgetCompactExpansionDirectionPolicy.ResolveEffective(config, "Down"));

        WidgetCompactExpansionDirectionPolicy.SetOverride(config, null);

        Assert.Null(WidgetCompactExpansionDirectionPolicy.GetOverride(config));
        Assert.False(config.Metadata.ContainsKey(
            WidgetCompactExpansionDirectionPolicy.OverrideMetadataKey));
    }

    [Fact]
    public void SettingsPage_ExposesThreeDirectionOptionsAndBindsTheSelection()
    {
        string xaml = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Views/SettingsSections/CapsuleModeSettingsSection.xaml"));
        string contracts = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Contracts/CapsuleOptionKinds.cs"));

        Assert.Contains("Settings.Capsule.ExpansionDirection.Title", xaml, StringComparison.Ordinal);
        Assert.Contains("AvailableExpansionDirectionOptions", xaml, StringComparison.Ordinal);
        Assert.Contains("controls:SettingsComboBox.Value=\"{Binding ExpansionDirection, Mode=TwoWay}\"", xaml, StringComparison.Ordinal);
        Assert.Contains("ExpansionDirectionAuto", contracts, StringComparison.Ordinal);
        Assert.Contains("ExpansionDirectionDown", contracts, StringComparison.Ordinal);
        Assert.Contains("ExpansionDirectionUp", contracts, StringComparison.Ordinal);
    }
}
