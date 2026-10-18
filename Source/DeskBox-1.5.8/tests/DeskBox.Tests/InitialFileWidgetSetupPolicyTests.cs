using DeskBox.Models;
using DeskBox.Services;
using Windows.Graphics;

namespace DeskBox.Tests;

public sealed class InitialFileWidgetSetupPolicyTests
{
    [Theory]
    [InlineData(false, SettingsLoadRecoveryState.DefaultsForMissingFile, false, false,
        (int)InitialFileWidgetSetupDecision.DeferUntilInteractiveLaunch)]
    [InlineData(true, SettingsLoadRecoveryState.DefaultsAfterFailure, false, false,
        (int)InitialFileWidgetSetupDecision.None)]
    [InlineData(true, SettingsLoadRecoveryState.Primary, true, false,
        (int)InitialFileWidgetSetupDecision.None)]
    [InlineData(true, SettingsLoadRecoveryState.Primary, false, true,
        (int)InitialFileWidgetSetupDecision.ResolveExistingConfiguration)]
    [InlineData(true, SettingsLoadRecoveryState.Primary, false, false,
        (int)InitialFileWidgetSetupDecision.CreateDefaultWidget)]
    [InlineData(true, SettingsLoadRecoveryState.DefaultsForMissingFile, false, false,
        (int)InitialFileWidgetSetupDecision.CreateDefaultWidget)]
    public void Evaluate_ReturnsExpectedDecision(
        bool isInteractiveLaunch,
        SettingsLoadRecoveryState loadState,
        bool hasResolvedSetup,
        bool hasConfiguredFileWidget,
        int expected)
    {
        var snapshot = new InitialFileWidgetSetupSnapshot(
            isInteractiveLaunch,
            loadState,
            hasResolvedSetup,
            hasConfiguredFileWidget);

        Assert.Equal(expected, (int)InitialFileWidgetSetupPolicy.Evaluate(snapshot));
    }

    [Fact]
    public void HasConfiguredFileWidget_TreatsDisabledWidgetAsExistingConfiguration()
    {
        var settings = new AppSettings
        {
            Widgets =
            [
                new WidgetConfig
                {
                    Id = "disabled-file-widget",
                    WidgetKind = WidgetKind.File,
                    IsDisabled = true
                }
            ]
        };

        Assert.True(InitialFileWidgetSetupPolicy.HasConfiguredFileWidget(settings));
    }

    [Fact]
    public void HasConfiguredFileWidget_IgnoresDeletedConfiguration()
    {
        var settings = new AppSettings
        {
            Widgets =
            [
                new WidgetConfig
                {
                    Id = "deleted-file-widget",
                    WidgetKind = WidgetKind.File
                }
            ],
            DeletedWidgetIds = ["deleted-file-widget"]
        };

        Assert.False(InitialFileWidgetSetupPolicy.HasConfiguredFileWidget(settings));
    }

    [Fact]
    public void InitialPlacement_AlignsWidgetToRightSideWithSafeMargins()
    {
        var workArea = new RectInt32(0, 0, 1920, 1040);

        RectInt32 bounds = InitialFileWidgetPlacementPolicy.CalculateRightAlignedBounds(
            workArea,
            logicalWidth: 300,
            logicalHeight: 400,
            dpiScale: 1.0);

        Assert.Equal(1596, bounds.X);
        Assert.Equal(72, bounds.Y);
        Assert.Equal(300, bounds.Width);
        Assert.Equal(400, bounds.Height);
    }

    [Fact]
    public void InitialPlacement_UsesSelectedDisplayOriginAndDpiScale()
    {
        var workArea = new RectInt32(1920, -200, 2560, 1400);

        RectInt32 bounds = InitialFileWidgetPlacementPolicy.CalculateRightAlignedBounds(
            workArea,
            logicalWidth: 320,
            logicalHeight: 420,
            dpiScale: 1.5);

        Assert.Equal(3964, bounds.X);
        Assert.Equal(-92, bounds.Y);
        Assert.Equal(480, bounds.Width);
        Assert.Equal(630, bounds.Height);
    }

    [Fact]
    public void InitialPlacement_ClampsOversizedWidgetInsideWorkArea()
    {
        var workArea = new RectInt32(-1280, 0, 420, 300);

        RectInt32 bounds = InitialFileWidgetPlacementPolicy.CalculateRightAlignedBounds(
            workArea,
            logicalWidth: 600,
            logicalHeight: 500,
            dpiScale: 1.0);

        Assert.Equal(workArea.X, bounds.X);
        Assert.Equal(workArea.Y, bounds.Y);
    }
}
