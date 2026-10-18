using DeskBox.Models;
using DeskBox.Platform;

namespace DeskBox.Services;

public interface IDesktopOrganizationCarry
{
    Task<string> BeforeMoveAsync(DesktopOrganizationFileSnapshot item,DesktopOrganizationTargetPlan target,CancellationToken cancellationToken);
    Task AfterMoveAsync(string carryId,bool completed);
}

public interface IDesktopOrganizationBatchCarry : IDesktopOrganizationCarry
{
    Task<string> BeforeBatchAsync(IReadOnlyList<DesktopOrganizationFileSnapshot> items,DesktopOrganizationTargetPlan target,CancellationToken cancellationToken);
}

public sealed class NaiWaCarryCoordinator(SettingsService settings) : IDesktopOrganizationBatchCarry
{
    private readonly AiOrganizeClient _client = new();

    // One budget per organization run. It is deliberately NOT a field of the
    // coordinator's owner: the coordinator instance lives for the whole
    // process, and run state kept here used to make the second organize in the
    // same session silently skip the animation.
    private NaiWaAnimationRun _run = new(NaiWaAnimationPolicy.Full);

    /// <summary>
    /// Starts a fresh animation budget for one organization run. Every entry
    /// point calls this exactly once before its first batch.
    /// </summary>
    public void BeginRun()
    {
        _run = new NaiWaAnimationRun(NaiWaAnimationPolicy.Read(settings.Settings));
        App.Log($"[NaiWa] Carry animation level={_run.Level}");
    }

    public async Task<string> BeforeMoveAsync(DesktopOrganizationFileSnapshot item,DesktopOrganizationTargetPlan target,CancellationToken cancellationToken)
        => await BeforeBatchAsync([item],target,cancellationToken);

    public async Task<string> BeforeBatchAsync(IReadOnlyList<DesktopOrganizationFileSnapshot> items,DesktopOrganizationTargetPlan target,CancellationToken cancellationToken)
    {
        if (items.Count is < 1 or > 40) throw new ArgumentException("搬运批次大小无效。");

        // No animation requested: return an empty receipt. AfterMoveAsync
        // ignores it, so the caller keeps its normal file-commit path.
        if (!_run.ShouldAnimate())
        {
            return string.Empty;
        }

        if (!await NaiWaAgentHost.EnsureConnectedAsync(cancellationToken)) throw new IOException("奶蛙尚未连接，未移动文件。");
        // Resolve the live category box, including placement and DPI corrections.
        // Preview bounds are only a fallback before a widget exists.
        var widget = settings.Settings.Widgets.FirstOrDefault(w => w.Id == target.TargetWidgetId);
        var bounds = target.PlannedBounds;
        if (widget is not null)
        {
            double scale = Win32Helper.GetDpiScaleForPoint((int)widget.X,(int)widget.Y);
            bounds = new(widget.X,widget.Y,widget.Width * scale,widget.Height * scale);
        }
        if (bounds is null) throw new IOException("The destination category is no longer available.");
        var box = bounds.Value;
        var move = new AiCarryMove(items[0].SourcePath,box.X + box.Width / 2,            box.Y + ((widget?.IsCollapsed ?? target.Collapsed) ? 20 : Math.Min(box.Height / 2,70)),target.SuggestedDisplayName,items.Select(i => i.SourcePath).ToArray());
        // The pet rejects a carry while it is mid-animation (its own trip
        // teardown, a click-triggered box or punch, the previous category's
        // stow). Retrying turns that transient state into a normal delay.
        // The window is deliberately wider than one trip's tail plus one
        // "organize" emote: the reported defect was that only the very first
        // category ever animated because later batches gave up too early.
        const int maxAttempts = 8;
        for (int attempt = 1; attempt <= maxAttempts; attempt++)
        {
            cancellationToken.ThrowIfCancellationRequested();
            try
            {
                string receipt = await _client.CarryAsync(move,cancellationToken);
                _run.RecordTrip();
                return receipt;
            }
            catch (NaiWaCarryRejectedException ex) when (ex.Status == "busy" && attempt < maxAttempts)
            {
                App.Log($"[NaiWa] Carry busy, retry {attempt}/{maxAttempts - 1}: {ex.Message}");
                await Task.Delay(600 * attempt,cancellationToken);
            }
        }
        throw new IOException("奶蛙未到达目标位置，此文件不会移动。");
    }

    public async Task AfterMoveAsync(string carryId,bool completed)
    {
        // An empty receipt means this batch played no animation.
        if (string.IsNullOrEmpty(carryId)) return;
        using var timeout = new CancellationTokenSource(TimeSpan.FromSeconds(15));
        await _client.CompleteCarryAsync(carryId,completed,timeout.Token);
    }
}
