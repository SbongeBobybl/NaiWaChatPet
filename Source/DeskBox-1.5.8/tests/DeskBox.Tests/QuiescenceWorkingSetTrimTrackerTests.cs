using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class QuiescenceWorkingSetTrimTrackerTests
{
    private static readonly DateTimeOffset T0 = new(2026, 1, 1, 0, 0, 0, TimeSpan.Zero);
    private const long MB = 1024 * 1024;

    private static readonly QuiescenceTrimActivitySnapshot VisibleBackground = new(
        HasVisibleWidgets: true,
        IsWidgetInteractionActive: false,
        HasActiveVisualWork: false,
        IsTransientUiOpen: false,
        IsDeskBoxForeground: false,
        IsPointerOverDeskBox: false);

    private static readonly QuiescenceTrimActivitySnapshot Hidden =
        VisibleBackground with { HasVisibleWidgets = false };

    private static readonly QuiescenceTrimActivitySnapshot VisibleForeground =
        VisibleBackground with { IsDeskBoxForeground = true };

    private static QuiescenceTrimDecision Observe(
        QuiescenceWorkingSetTrimTracker tracker,
        DateTimeOffset now,
        QuiescenceTrimActivitySnapshot activity,
        long workingSet = 400 * MB) =>
        tracker.Observe(now, activity, () => workingSet);

    [Fact]
    public void FirstTrim_FiresOnceVisibleBackgroundQuietPeriodElapses()
    {
        var tracker = new QuiescenceWorkingSetTrimTracker();

        var early = Observe(tracker, T0, VisibleBackground);
        Assert.False(early.ShouldTrim);
        Assert.Equal("waiting-for-quiet", early.Reason);
        Assert.Equal(QuiescenceTrimTier.VisibleBackground, early.Tier);

        var due = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.VisibleBackgroundQuietDuration,
            VisibleBackground);
        Assert.True(due.ShouldTrim);
        Assert.Equal("quiet", due.Reason);
    }

    [Theory]
    [InlineData(true, false, false, "blocked:interaction")]
    [InlineData(false, true, false, "blocked:visual-work")]
    [InlineData(false, false, true, "blocked:transient-ui")]
    public void Blockers_ResetQuietPeriod(
        bool interaction,
        bool visualWork,
        bool transientUi,
        string expectedReason)
    {
        var tracker = new QuiescenceWorkingSetTrimTracker();
        Observe(tracker, T0, VisibleBackground);

        var blocked = Observe(
            tracker,
            T0 + TimeSpan.FromSeconds(4),
            VisibleBackground with
            {
                IsWidgetInteractionActive = interaction,
                HasActiveVisualWork = visualWork,
                IsTransientUiOpen = transientUi
            });
        Assert.False(blocked.ShouldTrim);
        Assert.Equal(expectedReason, blocked.Reason);

        // Quiet restarts from the blocker, so 5 s after T0 is not yet due.
        var afterBlock = Observe(tracker, T0 + TimeSpan.FromSeconds(5), VisibleBackground);
        Assert.False(afterBlock.ShouldTrim);
        Assert.Equal("waiting-for-quiet", afterBlock.Reason);
    }

    [Fact]
    public void ForegroundOrPointer_OnlyLengthensQuietPeriod()
    {
        var tracker = new QuiescenceWorkingSetTrimTracker();
        Observe(tracker, T0, VisibleForeground);

        var atBackgroundThreshold = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.VisibleBackgroundQuietDuration,
            VisibleForeground);
        Assert.False(atBackgroundThreshold.ShouldTrim);
        Assert.Equal(QuiescenceTrimTier.VisibleForeground, atBackgroundThreshold.Tier);

        var atForegroundThreshold = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.VisibleForegroundQuietDuration,
            VisibleForeground);
        Assert.True(atForegroundThreshold.ShouldTrim);
    }

    [Fact]
    public void HiddenTier_UsesShortestQuietPeriod()
    {
        var tracker = new QuiescenceWorkingSetTrimTracker();
        Observe(tracker, T0, Hidden);

        var due = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.HiddenQuietDuration,
            Hidden);
        Assert.True(due.ShouldTrim);
        Assert.Equal(QuiescenceTrimTier.Hidden, due.Tier);
    }

    [Fact]
    public void NoteActivity_RestartsQuietPeriod()
    {
        var tracker = new QuiescenceWorkingSetTrimTracker();
        Observe(tracker, T0, VisibleBackground);
        tracker.NoteActivity();

        var notYet = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.VisibleBackgroundQuietDuration,
            VisibleBackground);
        Assert.False(notYet.ShouldTrim);
        Assert.Equal("waiting-for-quiet", notYet.Reason);
    }

    [Fact]
    public void AfterTrim_RequiresCooldownAndGrowthOverSettledBaseline()
    {
        var tracker = new QuiescenceWorkingSetTrimTracker();
        tracker.CommitTrim(T0);
        Observe(tracker, T0 + TimeSpan.FromSeconds(1), VisibleBackground, workingSet: 2 * MB);

        // Quiet is satisfied at T0+6s, but the 20 s cooldown is not.
        var cooldown = Observe(
            tracker,
            T0 + TimeSpan.FromSeconds(6),
            VisibleBackground,
            workingSet: 400 * MB);
        Assert.False(cooldown.ShouldTrim);
        Assert.Equal("trim-cooldown", cooldown.Reason);

        // The baseline is sampled once the working set has settled (8 s):
        // pages that fault straight back after a trim are steady state, not
        // growth, so 300 MB here becomes the reference point. The readings
        // stay above the 240 MB visible floor so the growth gate is what
        // speaks in this test.
        Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.BaselineSettleDelay,
            VisibleBackground,
            workingSet: 300 * MB);

        // Cooldown over, but the working set barely regrew: not worth it.
        var flat = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.MinimumTrimInterval,
            VisibleBackground,
            workingSet: 300 * MB + 8 * MB);
        Assert.False(flat.ShouldTrim);
        Assert.Equal("insufficient-growth", flat.Reason);

        // 25% of the 300 MB baseline (75 MB) clears the gate — the gate
        // never dips below the 32 MB floor nor above the 128 MB cap.
        var grown = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.MinimumTrimInterval,
            VisibleBackground,
            workingSet: 300 * MB + 75 * MB);
        Assert.True(grown.ShouldTrim);
    }

    [Fact]
    public void BeforeBaselineSettles_TrimIsHeldEvenIfCooldownElapsed()
    {
        var tracker = new QuiescenceWorkingSetTrimTracker();
        tracker.CommitTrim(T0);
        // Observations stop before the settle delay, so no baseline exists yet.
        Observe(tracker, T0 + TimeSpan.FromSeconds(1), VisibleBackground, workingSet: 2 * MB);

        var held = Observe(
            tracker,
            T0 + TimeSpan.FromSeconds(7),
            VisibleBackground,
            workingSet: 400 * MB);
        Assert.False(held.ShouldTrim);
    }

    [Fact]
    public void GrowthGate_NeverDropsBelowGrowthFloorOnSmallBaseline()
    {
        // Pinned on the hidden tier: below the 240 MB visible floor a
        // visible tier would be caught by the absolute floor before the
        // growth gate could speak.
        var tracker = new QuiescenceWorkingSetTrimTracker();
        tracker.CommitTrim(T0);
        Observe(tracker, T0 + QuiescenceWorkingSetTrimTracker.BaselineSettleDelay, Hidden, workingSet: 80 * MB);

        // 25% of 80 MB = 20 MB, but the floor pins the gate at 32 MB so a
        // low-water baseline cannot cause a perpetual trim loop.
        var under = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.MinimumTrimInterval,
            Hidden,
            workingSet: 100 * MB);
        Assert.False(under.ShouldTrim);
        Assert.Equal("insufficient-growth", under.Reason);

        var grown = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.MinimumTrimInterval,
            Hidden,
            workingSet: 80 * MB + QuiescenceWorkingSetTrimTracker.MinimumWorkingSetGrowthBytes);
        Assert.True(grown.ShouldTrim);
    }

    [Fact]
    public void FirstTrim_VisibleTierRequiresAbsoluteWorkingSetFloor()
    {
        var tracker = new QuiescenceWorkingSetTrimTracker();
        Observe(tracker, T0, VisibleBackground, workingSet: 150 * MB);

        // The visible floor matches the old visible-idle working-set
        // threshold (240 MB): a small visible session is never paged out
        // from under the user even though no growth baseline exists yet.
        var lowVisible = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.VisibleBackgroundQuietDuration,
            VisibleBackground,
            workingSet: 150 * MB);
        Assert.False(lowVisible.ShouldTrim);
        Assert.Equal("below-visible-floor", lowVisible.Reason);

        // Waiting longer does not help — only size clears this gate.
        var muchLater = Observe(
            tracker,
            T0 + TimeSpan.FromSeconds(60),
            VisibleBackground,
            workingSet: 150 * MB);
        Assert.False(muchLater.ShouldTrim);
        Assert.Equal("below-visible-floor", muchLater.Reason);

        // The hidden tier has no absolute floor: hidden pages are cheap to
        // re-fault and the immediate-hidden trim already trims on hide.
        var hiddenTracker = new QuiescenceWorkingSetTrimTracker();
        Observe(hiddenTracker, T0, Hidden, workingSet: 150 * MB);
        var hiddenDue = Observe(
            hiddenTracker,
            T0 + QuiescenceWorkingSetTrimTracker.HiddenQuietDuration,
            Hidden,
            workingSet: 150 * MB);
        Assert.True(hiddenDue.ShouldTrim);
    }

    [Theory]
    [InlineData(false)]
    [InlineData(true)]
    public void VisibleTiers_BelowAbsoluteFloor_NeverTrim(bool isForeground)
    {
        var snapshot = isForeground ? VisibleForeground : VisibleBackground;
        var tracker = new QuiescenceWorkingSetTrimTracker();
        Observe(tracker, T0, snapshot, workingSet: 150 * MB);

        // Far past every tier's quiet deadline: still no trim, because the
        // process is simply not big enough to be worth paging out while the
        // user is watching.
        var pastDeadline = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.VisibleForegroundQuietDuration +
                TimeSpan.FromSeconds(30),
            snapshot,
            workingSet: 150 * MB);
        Assert.False(pastDeadline.ShouldTrim);
        Assert.Equal("below-visible-floor", pastDeadline.Reason);
    }

    [Theory]
    [InlineData(false)]
    [InlineData(true)]
    public void VisibleTier_TrimsAtAbsoluteFloorOnceQuiet(bool isForeground)
    {
        var snapshot = isForeground ? VisibleForeground : VisibleBackground;
        TimeSpan requiredQuiet = isForeground
            ? QuiescenceWorkingSetTrimTracker.VisibleForegroundQuietDuration
            : QuiescenceWorkingSetTrimTracker.VisibleBackgroundQuietDuration;
        var tracker = new QuiescenceWorkingSetTrimTracker();

        // The floor is inclusive: exactly 240 MB is trimmable (first trim
        // included) once the tier's quiet period has elapsed.
        Observe(tracker, T0, snapshot);
        var due = Observe(
            tracker,
            T0 + requiredQuiet,
            snapshot,
            workingSet: QuiescenceWorkingSetTrimTracker.VisibleTierMinimumWorkingSetBytes);
        Assert.True(due.ShouldTrim);
        Assert.Equal("quiet", due.Reason);
    }

    [Fact]
    public void VisibleTier_ReTrimStillBoundByAbsoluteFloorAndGrowthGate()
    {
        var tracker = new QuiescenceWorkingSetTrimTracker();
        tracker.CommitTrim(T0);
        Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.BaselineSettleDelay,
            VisibleBackground,
            workingSet: 80 * MB);

        // Growth clears the 32 MB minimum over the small baseline, but a
        // visible re-trim must also clear the absolute floor.
        var grownButSmall = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.MinimumTrimInterval,
            VisibleBackground,
            workingSet: 80 * MB + QuiescenceWorkingSetTrimTracker.MinimumWorkingSetGrowthBytes);
        Assert.False(grownButSmall.ShouldTrim);
        Assert.Equal("below-visible-floor", grownButSmall.Reason);

        var grown = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.MinimumTrimInterval,
            VisibleBackground,
            workingSet: QuiescenceWorkingSetTrimTracker.VisibleTierMinimumWorkingSetBytes);
        Assert.True(grown.ShouldTrim);
    }

    [Fact]
    public void TierChange_RestartsQuietPeriod()
    {
        var tracker = new QuiescenceWorkingSetTrimTracker();
        Observe(tracker, T0, Hidden);

        // Quiet accumulated while hidden must not satisfy the foreground
        // tier the moment the user looks at a widget.
        var upgraded = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.HiddenQuietDuration + TimeSpan.FromSeconds(12),
            VisibleForeground);
        Assert.False(upgraded.ShouldTrim);
        Assert.Equal("waiting-for-quiet", upgraded.Reason);

        var due = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.HiddenQuietDuration +
                TimeSpan.FromSeconds(12) +
                QuiescenceWorkingSetTrimTracker.VisibleForegroundQuietDuration,
            VisibleForeground);
        Assert.True(due.ShouldTrim);
    }

    [Fact]
    public void AmbientAnimation_RequiresForegroundQuietDuration()
    {
        var tracker = new QuiescenceWorkingSetTrimTracker();
        var ambient = VisibleBackground with { HasAmbientVisualWork = true };
        Observe(tracker, T0, ambient);

        // Looping decoration does not block outright, but it escalates the
        // required quiet to the strictest tier.
        var atBackgroundThreshold = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.VisibleBackgroundQuietDuration,
            ambient);
        Assert.False(atBackgroundThreshold.ShouldTrim);
        Assert.Equal("waiting-for-quiet", atBackgroundThreshold.Reason);

        var atForegroundThreshold = Observe(
            tracker,
            T0 + QuiescenceWorkingSetTrimTracker.VisibleForegroundQuietDuration,
            ambient);
        Assert.True(atForegroundThreshold.ShouldTrim);
    }

    [Fact]
    public void CommitTrim_FromAnotherPath_ResetsQuietAccounting()
    {
        var tracker = new QuiescenceWorkingSetTrimTracker();
        Observe(tracker, T0, VisibleBackground);

        // The immediate-hidden path trimmed independently.
        tracker.CommitTrim(T0 + TimeSpan.FromSeconds(4));

        var afterExternalTrim = Observe(
            tracker,
            T0 + TimeSpan.FromSeconds(5),
            VisibleBackground,
            workingSet: 400 * MB);
        Assert.False(afterExternalTrim.ShouldTrim);
        Assert.Equal("waiting-for-quiet", afterExternalTrim.Reason);
    }
}
