namespace DeskBox.Services;

/// <summary>
/// UI facts the quiescence trim needs. Interaction, in-flight animation, and
/// transient windows block a trim outright; foreground/pointer presence and
/// looping ambient animation only lengthen the required quiet period.
/// </summary>
internal readonly record struct QuiescenceTrimActivitySnapshot(
    bool HasVisibleWidgets,    bool IsWidgetInteractionActive,    bool HasActiveVisualWork,    bool IsTransientUiOpen,    bool IsDeskBoxForeground,    bool IsPointerOverDeskBox,    bool HasAmbientVisualWork = false);

internal enum QuiescenceTrimTier
{
    Hidden,
    VisibleBackground,
    VisibleForeground
}

internal readonly record struct QuiescenceTrimDecision(
    bool ShouldTrim,    QuiescenceTrimTier Tier,    string Reason,    TimeSpan QuietDuration)
{
    internal static QuiescenceTrimDecision Skip(
        QuiescenceTrimTier tier,        string reason,        TimeSpan quiet = default) =>
        new(false,tier,reason,quiet);
}

/// <summary>
/// Decides when a process-wide working-set trim is worth running after the
/// UI has gone quiet. Every operation (closing settings, collapsing a
/// capsule, hiding widgets, a drag) ends in the same quiet state, so one
/// tracker replaces per-operation trim hooks. A trim only fires once the
/// quiet period for the current tier has elapsed, the previous trim is older
/// than the minimum interval, and the working set has regrown enough for the
/// trim to be meaningful. Visible tiers additionally demand an absolute
/// working-set floor on every trim (the first one included) so a small
/// visible session is never paged out from under the user; the hidden tier
/// skips that floor to match the immediate-hidden trim. The tracker owns no
/// timer so callers can drive it with a fake clock in tests.
/// </summary>
internal sealed class QuiescenceWorkingSetTrimTracker
{
    internal static readonly TimeSpan HiddenQuietDuration = TimeSpan.FromSeconds(3);
    internal static readonly TimeSpan VisibleBackgroundQuietDuration = TimeSpan.FromSeconds(5);
    internal static readonly TimeSpan VisibleForegroundQuietDuration = TimeSpan.FromSeconds(15);
    internal static readonly TimeSpan MinimumTrimInterval = TimeSpan.FromSeconds(20);
    internal const long MinimumWorkingSetGrowthBytes = 32L * 1024 * 1024;
    internal const long MaximumWorkingSetGrowthBytes = 128L * 1024 * 1024;
    internal const double MinimumWorkingSetGrowthRatio = 0.25;

    // Visible tiers (user present, widgets on screen) carry an absolute
    // working-set floor on every trim, the first one included: paging a
    // small visible working set out would fault the pages straight back
    // onto the visible UI, so the process must genuinely be bloated before
    // the trim is worth the churn. The 240 MB value matches the visible-idle
    // working-set threshold in MemoryCleanupPolicy
    // (VisibleIdleWorkingSetThresholdBytes) but is re-declared here so the
    // quiescence path owns its own number rather than reaching into another
    // policy's constants; unlike that policy this gate checks a single
    // metric — the working set read via readWorkingSetBytes — and does not
    // stack the private-bytes companion threshold on top. The hidden tier
    // deliberately has no absolute floor: hidden pages are cheap to re-fault
    // and the immediate-hidden trim already trims unconditionally on hide.
    internal const long VisibleTierMinimumWorkingSetBytes = 240L * 1024 * 1024;

    // Right after a trim the working set is near zero; the pages the render
    // loop and timers actually need fault back within seconds. Measuring the
    // growth baseline only after that settling period keeps steady-state
    // residency from being mistaken for growth and re-trimmed every cooldown.
    internal static readonly TimeSpan BaselineSettleDelay = TimeSpan.FromSeconds(8);

    private DateTimeOffset? _quietSince;
    private QuiescenceTrimTier? _quietTier;
    private DateTimeOffset? _lastTrimAt;
    private long? _settledWorkingSetBaseline;

    internal void NoteActivity()
    {
        _quietSince = null;
        _quietTier = null;
    }

    internal QuiescenceTrimDecision Observe(
        DateTimeOffset now,        QuiescenceTrimActivitySnapshot activity,        Func<long> readWorkingSetBytes)
    {
        if (_settledWorkingSetBaseline is null &&
            _lastTrimAt is DateTimeOffset trimmedAt &&
            now - trimmedAt >= BaselineSettleDelay)
        {
            _settledWorkingSetBaseline = readWorkingSetBytes();
        }

        QuiescenceTrimTier tier = ResolveTier(activity);
        if (activity.IsWidgetInteractionActive ||
            activity.HasActiveVisualWork ||
            activity.IsTransientUiOpen)
        {
            _quietSince = null;
            _quietTier = null;
            return QuiescenceTrimDecision.Skip(tier,DescribeBlocker(activity));
        }

        if (_quietTier != tier)
        {
            // A tier change restarts quiet counting: quiet accumulated under
            // a cheaper tier must not satisfy a stricter one, and a
            // downgrade gives the immediate-hidden trim its grace first.
            _quietSince = null;
            _quietTier = tier;
        }

        _quietSince ??= now;
        TimeSpan quiet = now - _quietSince.Value;
        // Looping ambient animation (compact marquee, vinyl) is steady-state
        // decoration — it cannot block outright or the trim would never fire
        // — but it earns the strictest quiet duration.
        TimeSpan requiredQuiet = activity.HasAmbientVisualWork
            ? VisibleForegroundQuietDuration
            : GetRequiredQuietDuration(tier);
        if (quiet < requiredQuiet)
        {
            return QuiescenceTrimDecision.Skip(tier,"waiting-for-quiet",quiet);
        }

        if (_lastTrimAt is DateTimeOffset lastTrimAt &&
            now - lastTrimAt < MinimumTrimInterval)
        {
            return QuiescenceTrimDecision.Skip(tier,"trim-cooldown",quiet);
        }

        if (_lastTrimAt is not null && _settledWorkingSetBaseline is null)
        {
            return QuiescenceTrimDecision.Skip(tier,"baseline-settling",quiet);
        }

        if (tier is not QuiescenceTrimTier.Hidden &&
            readWorkingSetBytes() < VisibleTierMinimumWorkingSetBytes)
        {
            // Visible tiers must clear the absolute floor on every trim:
            // before the first trim there is no baseline for the growth
            // gate to lean on, and a small settled baseline could let a
            // re-trim fire well below a size worth paging out while the
            // user is watching. The hidden tier skips this gate.
            return QuiescenceTrimDecision.Skip(tier,"below-visible-floor",quiet);
        }

        if (_settledWorkingSetBaseline is long baseline)
        {
            long growth = readWorkingSetBytes() - baseline;
            long requiredGrowth = Math.Clamp(
                (long)(baseline * MinimumWorkingSetGrowthRatio),                MinimumWorkingSetGrowthBytes,                MaximumWorkingSetGrowthBytes);
            if (growth < requiredGrowth)
            {
                return QuiescenceTrimDecision.Skip(tier,"insufficient-growth",quiet);
            }
        }

        return new QuiescenceTrimDecision(true,tier,"quiet",quiet);
    }

    /// <summary>
    /// Records a completed trim from any path (quiescence, immediate hidden,
    /// or deep cleanup) so cooldown and growth accounting stay unified. The
    /// growth baseline is re-sampled once the working set has settled.
    /// </summary>
    internal void CommitTrim(DateTimeOffset now)
    {
        _lastTrimAt = now;
        _settledWorkingSetBaseline = null;
        _quietSince = null;
        _quietTier = null;
    }

    /// <summary>
    /// Whether any trim path committed within <paramref name="interval"/>.
    /// Other trim callers use this to skip redundant back-to-back trims.
    /// </summary>
    internal bool LastTrimWithin(DateTimeOffset now,TimeSpan interval) =>
        _lastTrimAt is DateTimeOffset trimmedAt && now - trimmedAt < interval;

    internal static TimeSpan GetRequiredQuietDuration(QuiescenceTrimTier tier) => tier switch
    {
        QuiescenceTrimTier.Hidden => HiddenQuietDuration,        QuiescenceTrimTier.VisibleForeground => VisibleForegroundQuietDuration,        _ => VisibleBackgroundQuietDuration
    };

    private static QuiescenceTrimTier ResolveTier(QuiescenceTrimActivitySnapshot activity)
    {
        if (!activity.HasVisibleWidgets)
        {
            return QuiescenceTrimTier.Hidden;
        }

        return activity.IsDeskBoxForeground || activity.IsPointerOverDeskBox
            ? QuiescenceTrimTier.VisibleForeground
            : QuiescenceTrimTier.VisibleBackground;
    }

    private static string DescribeBlocker(QuiescenceTrimActivitySnapshot activity)
    {
        if (activity.IsWidgetInteractionActive)
        {
            return "blocked:interaction";
        }

        return activity.HasActiveVisualWork
            ? "blocked:visual-work"
            : "blocked:transient-ui";
    }
}
