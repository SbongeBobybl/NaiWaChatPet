using DeskBox.Models;

namespace DeskBox.Services;

public static class NaiWaAgentHost
{
    private static readonly SemaphoreSlim Gate = new(1,1);

    // NaiWa owns the lifecycle. The optional organization engine never starts the pet.
    public static async Task<bool> EnsureConnectedAsync(CancellationToken cancellationToken = default) =>
        await ProbeAsync(cancellationToken) is not null;

    /// <summary>
    /// Null when the pet cannot be reached, otherwise whether a model key is
    /// configured. The organizer uses this before offering AI refinement so an
    /// unconfigured install is never reported as an AI result.
    /// </summary>
    public static async Task<bool?> ModelConfiguredAsync(CancellationToken cancellationToken = default) =>
        (await ProbeAsync(cancellationToken))?.Configured;

    private static async Task<AiAgentResponse?> ProbeAsync(CancellationToken cancellationToken)
    {
        await Gate.WaitAsync(cancellationToken);
        try
        {
            try
            {
                var response = await new AiOrganizeClient().HealthAsync(cancellationToken);
                return response.Status == "ok" ? response : null;
            }
            catch (Exception ex) when (ex is IOException or UnauthorizedAccessException
                or System.ComponentModel.Win32Exception or System.Text.Json.JsonException
                or TimeoutException or InvalidDataException)
            {
                App.Log("[NaiWa] Agent unavailable; offline organization remains available.");
                return null;
            }
        }
        finally { Gate.Release(); }
    }
}
