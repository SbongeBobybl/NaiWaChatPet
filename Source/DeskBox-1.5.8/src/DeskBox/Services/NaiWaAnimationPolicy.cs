using DeskBox.Models;

namespace DeskBox.Services;

/// <summary>
/// Decides how much of the pet's carry animation an organization run plays.
///
/// The three levels are user-selectable in the organizer workbench:
/// <list type="bullet">
/// <item><c>full</c> — one trip per category: the pet walks to each file,
/// collects the batch, then carries it to that category's box. Slowest and
/// most legible.</item>
/// <item><c>simple</c> — a single trip for the whole plan: the pet collects
/// and delivers once, then the remaining categories are filled silently.</item>
/// <item><c>none</c> — no animation; the grids are filled directly.</item>
/// </list>
///
/// The level is read from settings at the start of every run. A
/// <see cref="NaiWaAnimationRun"/> instance is created per run on purpose:
/// the previous "only the first organize animated" defect came from run state
/// living on a long-lived coordinator, where one skipped trip quietly disabled
/// the animation for every later run in the same process.
/// </summary>
public static class NaiWaAnimationPolicy
{
    public const string Full = "full";
    public const string Simple = "simple";
    public const string None = "none";

    public static string Normalize(string? value) => value switch
    {
        Simple => Simple,
        None => None,
        _ => Full,
    };

    public static string Read(AppSettings settings) =>
        Normalize(settings.NaiWaCarryAnimation);

    /// <summary>Display order and labels used by the workbench combo box.</summary>
    public static IReadOnlyList<(string Value, string Label)> Options { get; } =
    [
        (Full, "完整动画 · 按类别逐个搬运"),
        (Simple, "简易动画 · 只播放一次"),
        (None, "无动画 · 直接落格"),
    ];
}

/// <summary>
/// Per-run animation budget. Create one at the start of every organization run
/// and let it answer whether the next batch still gets an animation.
/// </summary>
public sealed class NaiWaAnimationRun
{
    private int _tripsPlayed;

    public NaiWaAnimationRun(string level) => Level = NaiWaAnimationPolicy.Normalize(level);

    public string Level { get; }

    /// <summary>True when this run plays no animation at all.</summary>
    public bool IsDisabled => Level == NaiWaAnimationPolicy.None;

    /// <summary>
    /// Whether the next trip should be animated. Full mode animates every
    /// category; simple mode animates exactly one trip; none never does.
    /// </summary>
    public bool ShouldAnimate()
    {
        if (IsDisabled) return false;
        if (Level == NaiWaAnimationPolicy.Simple) return _tripsPlayed == 0;
        return true;
    }

    /// <summary>
    /// Records that a trip was actually requested. Only a trip the pet really
    /// started consumes the single simple-mode animation, so a busy pet on the
    /// first category does not silently mute the whole run.
    /// </summary>
    public void RecordTrip() => _tripsPlayed++;
}
