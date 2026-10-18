using Windows.ApplicationModel.DataTransfer;

namespace DeskBox.Controls.WidgetContents;

public sealed partial class FileSurfaceContent
{
    private async Task TraceDragSourcePresenceAsync(
        string[] sourcePaths,        string? sessionId,        DataPackageOperation reportedOperation)
    {
        // Observe off the dispatcher; this is evidence only. Never recover,
        // delete, or move a source based on a receiver's completion receipt.
        string widgetId = WidgetId;
        CancellationToken cancellationToken = _lifetimeCancellation.Token;
        try
        {
            for (int observation = 0; observation < 2; observation++)
            {
                if (observation > 0)
                {
                    await Task.Delay(1000,cancellationToken);
                }
                string[] missing = await Task.Run(() => sourcePaths
                    .Where(path => !File.Exists(path) && !Directory.Exists(path))
                    .ToArray(),cancellationToken);
                cancellationToken.ThrowIfCancellationRequested();
                App.Log($"[DragProtocol] stage=SourcePresence widget={widgetId} " +
                    $"session={FormatDragSessionId(sessionId)} policy=SourceGuard " +
                    $"reported={reportedOperation} observation={observation} " +
                    $"tracked={sourcePaths.Length} missingOrUnavailable={missing.Length} " +
                    $"pathSample='{string.Join(" | ",missing.Take(5))}'");
            }
        }
        catch (OperationCanceledException)
        {
        }
        catch (Exception ex)
        {
            App.Log($"[DragProtocol] Source presence observation failed " +
                $"widget={widgetId} session={FormatDragSessionId(sessionId)}: {ex.Message}");
        }
    }
}
