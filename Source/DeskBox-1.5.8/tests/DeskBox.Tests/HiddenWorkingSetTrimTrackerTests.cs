using System.Text.Json;
using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class HiddenWorkingSetTrimTrackerTests
{
    private static readonly WidgetMemoryVisibilitySnapshot Visible = new(3, 3, 3);
    private static readonly WidgetMemoryVisibilitySnapshot Hidden = new(3, 0, 0);

    [Fact]
    public void InitialHiddenState_DoesNotTrimAtStartupOrWhenSettingsClose()
    {
        var tracker = new HiddenWorkingSetTrimTracker();

        Assert.Null(tracker.Observe(Hidden, enabled: true));
        Assert.Null(tracker.Observe(Hidden, enabled: true));
    }

    [Theory]
    [InlineData(0, 1)] // A hide animation is still visible natively.
    [InlineData(1, 0)] // A window has already been prepared for showing.
    [InlineData(2, 2)] // Only one of several widgets was hidden.
    public void RemainingVisibleWindow_DelaysRequestUntilEverythingIsHidden(
        int logicalVisibleCount,
        int nativeVisibleCount)
    {
        var tracker = new HiddenWorkingSetTrimTracker();
        tracker.Observe(Visible, enabled: true);

        Assert.Null(tracker.Observe(
            new WidgetMemoryVisibilitySnapshot(3, logicalVisibleCount, nativeVisibleCount),
            enabled: true));
        Assert.NotNull(tracker.Observe(Hidden, enabled: true));
    }

    [Fact]
    public void NativeAndBatchCompletionNotifications_ProduceOnlyOneTrim()
    {
        var tracker = new HiddenWorkingSetTrimTracker();
        tracker.Observe(Visible, enabled: true);
        long request = tracker.Observe(Hidden, enabled: true)!.Value;

        Assert.Null(tracker.Observe(Hidden, enabled: true));
        Assert.True(tracker.TryConsume(request));
        tracker.Complete(request, trimmed: true);

        Assert.False(tracker.TryConsume(request));
        Assert.Null(tracker.Observe(Hidden, enabled: true));
        Assert.True(tracker.TrimmedCurrentHiddenSession);
    }

    [Fact]
    public void ReopeningAndHidingAgain_InvalidatesOldRequestWithoutConsumingNewOne()
    {
        var tracker = new HiddenWorkingSetTrimTracker();
        tracker.Observe(Visible, enabled: true);
        long oldRequest = tracker.Observe(Hidden, enabled: true)!.Value;
        tracker.Observe(Visible, enabled: true);
        long newRequest = tracker.Observe(Hidden, enabled: true)!.Value;

        Assert.False(tracker.TryConsume(oldRequest));
        tracker.Complete(oldRequest, trimmed: true);
        Assert.False(tracker.TrimmedCurrentHiddenSession);
        Assert.True(tracker.TryConsume(newRequest));
    }

    [Fact]
    public void CancelledRequest_DoesNotRunWhenOtherUiCloses()
    {
        var tracker = new HiddenWorkingSetTrimTracker();
        tracker.Observe(Visible, enabled: true);
        long request = tracker.Observe(Hidden, enabled: true)!.Value;

        tracker.CancelPending();

        Assert.False(tracker.TryConsume(request));
        Assert.Null(tracker.Observe(Hidden, enabled: true));
    }

    [Fact]
    public void DisabledOption_LeavesExistingCleanupInControlUntilNextHide()
    {
        var tracker = new HiddenWorkingSetTrimTracker();
        tracker.Observe(Visible, enabled: false);

        Assert.Null(tracker.Observe(Hidden, enabled: false));
        Assert.Null(tracker.Observe(Hidden, enabled: true));
        Assert.False(tracker.TrimmedCurrentHiddenSession);

        tracker.Observe(Visible, enabled: true);
        Assert.NotNull(tracker.Observe(Hidden, enabled: true));
    }

    [Fact]
    public void NoLoadedWidgets_DoesNotTrimOnShutdown()
    {
        var tracker = new HiddenWorkingSetTrimTracker();
        tracker.Observe(Visible, enabled: true);

        Assert.Null(tracker.Observe(new WidgetMemoryVisibilitySnapshot(0, 0, 0), enabled: true));
    }

    [Fact]
    public void OnlySuccessfulTrim_SuppressesLaterTrimInTheSameHiddenSession()
    {
        var tracker = new HiddenWorkingSetTrimTracker();
        tracker.Observe(Visible, enabled: true);
        long request = tracker.Observe(Hidden, enabled: true)!.Value;
        Assert.True(tracker.TryConsume(request));
        tracker.Complete(request, trimmed: false);
        Assert.False(tracker.TrimmedCurrentHiddenSession);

        tracker.Observe(Visible, enabled: true);
        request = tracker.Observe(Hidden, enabled: true)!.Value;
        Assert.True(tracker.TryConsume(request));
        tracker.Complete(request, trimmed: true);
        Assert.True(tracker.TrimmedCurrentHiddenSession);

        tracker.Observe(Visible, enabled: true);
        Assert.False(tracker.TrimmedCurrentHiddenSession);
    }

    [Fact]
    public void ExistingSettings_DefaultAndEnabledChoiceSurviveGeneratedJsonRoundTrip()
    {
        AppSettings settings = JsonSerializer.Deserialize(
            "{\"idleWorkingSetTrimEnabled\":true,\"hiddenCacheCleanupDelaySeconds\":30}",
            SettingsJsonContext.Default.AppSettings)!;
        Assert.True(settings.ImmediateHiddenWorkingSetTrimEnabled);

        settings.ImmediateHiddenWorkingSetTrimEnabled = false;
        string json = JsonSerializer.Serialize(settings, SettingsJsonContext.Default.AppSettings);
        AppSettings reloaded = JsonSerializer.Deserialize(json, SettingsJsonContext.Default.AppSettings)!;
        Assert.False(reloaded.ImmediateHiddenWorkingSetTrimEnabled);
        Assert.True(reloaded.IdleWorkingSetTrimEnabled);
        Assert.Equal(30, reloaded.HiddenCacheCleanupDelaySeconds);

        SettingsService.ApplyDefaultPreferences(reloaded);
        Assert.True(reloaded.ImmediateHiddenWorkingSetTrimEnabled);
    }
}
