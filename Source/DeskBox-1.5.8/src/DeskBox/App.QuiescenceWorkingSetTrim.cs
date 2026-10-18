using System.Diagnostics;
using DeskBox.Platform;
using DeskBox.Services;
using Microsoft.UI.Dispatching;

namespace DeskBox;

public partial class App
{
    // Poll rather than wire every UI event: the snapshot is a handful of
    // cheap Win32 queries and the quiet tiers are measured in seconds.
    private static readonly TimeSpan QuiescenceWorkingSetTrimPollInterval = TimeSpan.FromSeconds(2);

    private readonly QuiescenceWorkingSetTrimTracker _quiescenceWorkingSetTrimTracker = new();
    private DispatcherQueueTimer? _quiescenceWorkingSetTrimTimer;
    private int _quiescenceWorkingSetTrimRunning;

    private bool IsQuiescenceWorkingSetTrimEnabled =>
        SettingsService.Settings.Performance is
        {
            IdleWorkingSetTrimEnabled: true,
            QuiescenceWorkingSetTrimEnabled: true
        };

    private void StartQuiescenceWorkingSetTrim()
    {
        if (_quiescenceWorkingSetTrimTimer is null)
        {
            _quiescenceWorkingSetTrimTimer = UiDispatcherQueue.CreateTimer();
            _quiescenceWorkingSetTrimTimer.IsRepeating = true;
            _quiescenceWorkingSetTrimTimer.Interval = QuiescenceWorkingSetTrimPollInterval;
            _quiescenceWorkingSetTrimTimer.Tick += QuiescenceWorkingSetTrimTimer_Tick;
        }

        // The toggle is read on every tick so a settings change takes effect
        // without any timer re-arming; an idle tick is a few cheap queries.
        _quiescenceWorkingSetTrimTimer.Start();
    }

    private void StopQuiescenceWorkingSetTrim()
    {
        if (_quiescenceWorkingSetTrimTimer is null)
        {
            return;
        }

        _quiescenceWorkingSetTrimTimer.Stop();
        _quiescenceWorkingSetTrimTimer.Tick -= QuiescenceWorkingSetTrimTimer_Tick;
        _quiescenceWorkingSetTrimTimer = null;
    }

    private void NoteQuiescenceWorkingSetTrimActivity() =>
        _quiescenceWorkingSetTrimTracker.NoteActivity();

    private QuiescenceTrimActivitySnapshot CaptureQuiescenceTrimActivity()
    {
        MemoryCleanupActivitySnapshot activity = CaptureMemoryCleanupActivity();
        return new QuiescenceTrimActivitySnapshot(
            HasVisibleWidgets: activity.HasVisibleWidgets,            IsWidgetInteractionActive: activity.IsWidgetInteractionActive,            HasActiveVisualWork: WidgetManager?.HasActiveVisualWork == true,            IsTransientUiOpen:
                activity.IsSettingsOpen ||
                activity.IsOnboardingOpen ||
                activity.IsSearchPopupVisible ||
                activity.IsDesktopOrganizationOpen,            IsDeskBoxForeground: activity.IsDeskBoxForeground,            IsPointerOverDeskBox: activity.IsPointerOverDeskBox,            HasAmbientVisualWork: WidgetManager?.HasAmbientVisualWork == true);
    }

    private void QuiescenceWorkingSetTrimTimer_Tick(DispatcherQueueTimer sender,object args)
    {
        if (Volatile.Read(ref _quiescenceWorkingSetTrimRunning) != 0)
        {
            return;
        }

        if (!IsQuiescenceWorkingSetTrimEnabled ||
            WidgetManager is not { } manager ||
            manager.CaptureMemoryCleanupVisibilitySnapshot().LoadedWindowCount == 0)
        {
            // Disabled ticks and empty desktops must not accumulate quiet —
            // re-enabling (or loading the first widget) would otherwise trim
            // instantly on the strength of stale quiet time.
            _quiescenceWorkingSetTrimTracker.NoteActivity();
            return;
        }

        QuiescenceTrimDecision decision = _quiescenceWorkingSetTrimTracker.Observe(
            DateTimeOffset.UtcNow,            CaptureQuiescenceTrimActivity(),            static () => Environment.WorkingSet);
        if (decision.ShouldTrim)
        {
            SafeFireAndForget(() => RunQuiescenceWorkingSetTrimAsync(decision));
        }
    }

    private async Task RunQuiescenceWorkingSetTrimAsync(QuiescenceTrimDecision decision)
    {
        if (Interlocked.Exchange(ref _quiescenceWorkingSetTrimRunning,1) != 0)
        {
            return;
        }

        try
        {
            // Re-validate right before the trim: the decision snapshot was
            // captured at tick time and a flyout or drag may have started
            // in between.  Re-observing also keeps tracker state honest.
            QuiescenceTrimDecision recheck = _quiescenceWorkingSetTrimTracker.Observe(
                DateTimeOffset.UtcNow,                CaptureQuiescenceTrimActivity(),                static () => Environment.WorkingSet);
            if (!recheck.ShouldTrim)
            {
                Log(
                    $"[Memory] Quiescence working-set trim skipped " +
                    $"reason={recheck.Reason} tier={decision.Tier}");
                return;
            }

            long workingSetBefore = Environment.WorkingSet;
            long started = Stopwatch.GetTimestamp();
            bool trimmed = await Task.Run(Win32Helper.TrimWorkingSet);
            long workingSetAfter = CompleteWorkingSetTrim(
                trimmed,                $"quiescence:{decision.Tier}");
            Log(
                $"[Memory] Quiescence working-set trim completed trimmed={trimmed} " +
                $"tier={decision.Tier} quietSeconds={decision.QuietDuration.TotalSeconds:F1} " +
                $"durationMs={Stopwatch.GetElapsedTime(started).TotalMilliseconds:F1} " +
                $"workingSetBeforeMB={workingSetBefore / (1024.0 * 1024):F1} " +
                $"workingSetAfterMB={workingSetAfter / (1024.0 * 1024):F1}");
        }
        finally
        {
            Volatile.Write(ref _quiescenceWorkingSetTrimRunning,0);
        }
    }

    /// <summary>
    /// Shared bookkeeping for every working-set trim path. Advances the
    /// cleanup epoch and tells the quiescence tracker a trim happened so its
    /// cooldown and growth gates account for trims it did not initiate.
    /// Returns the working set measured right after the trim.
    /// </summary>
    private long CompleteWorkingSetTrim(bool trimmed,string reason)
    {
        long workingSetAfter = Environment.WorkingSet;
        if (trimmed)
        {
            AdvanceMemoryCleanupEpoch($"working-set-trim:{reason}");
            _quiescenceWorkingSetTrimTracker.CommitTrim(DateTimeOffset.UtcNow);
        }

        return workingSetAfter;
    }
}
