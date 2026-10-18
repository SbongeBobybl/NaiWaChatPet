using Windows.ApplicationModel;

namespace DeskBox.Services;

public sealed class StoreStartupService : IStartupService
{
    private const string StartupTaskId = "DeskBoxStartupTask";

    public StartupRegistrationState GetState()
    {
        try
        {
            var task = GetStartupTask();
            return task.State switch
            {
                StartupTaskState.Enabled => StartupRegistrationState.Enabled,                StartupTaskState.EnabledByPolicy => StartupRegistrationState.Enabled,                StartupTaskState.DisabledByUser => StartupRegistrationState.DisabledByUser,                StartupTaskState.Disabled => StartupRegistrationState.NotRegistered,                StartupTaskState.DisabledByPolicy => StartupRegistrationState.BlockedOrFailed,                _ => StartupRegistrationState.BlockedOrFailed
            };
        }
        catch (Exception ex)
        {
            global::DeskBox.App.Log($"[StoreStartupService] Failed to query startup state: {ex.Message}");
            return StartupRegistrationState.BlockedOrFailed;
        }
    }

    public bool IsEnabled() => GetState() == StartupRegistrationState.Enabled;

    public string? GetRunValue() => null;

    public StartupOperationResult Enable()
    {
        try
        {
            var task = GetStartupTask();
            if (task.State is StartupTaskState.Enabled or StartupTaskState.EnabledByPolicy)
            {
                return new StartupOperationResult(StartupRegistrationState.Enabled);
            }

            if (task.State == StartupTaskState.Disabled)
            {
                // Fire-and-forget: RequestEnableAsync may show a consent dialog
                // that requires the UI thread. Blocking with GetAwaiter().GetResult()
                // would dead-lock the UI thread.
                _ = task.RequestEnableAsync().AsTask().ContinueWith(t =>
                {
                    if (t.IsCompletedSuccessfully)
                    {
                        global::DeskBox.App.Log($"[StoreStartupService] StartupTask enable requested: {t.Result}");
                    }
                    else if (t.IsFaulted)
                    {
                        global::DeskBox.App.Log($"[StoreStartupService] StartupTask enable failed: {t.Exception?.GetBaseException()?.Message}");
                    }
                },TaskScheduler.Default);
                return new StartupOperationResult(StartupRegistrationState.Pending);
            }

            if (task.State == StartupTaskState.DisabledByUser)
            {
                const string message =
                    "Windows Startup apps has disabled the DeskBox startup task.";
                global::DeskBox.App.Log($"[StoreStartupService] {message}");
                return new StartupOperationResult(
                    StartupRegistrationState.DisabledByUser,                    message);
            }

            string failure =
                $"StartupTask cannot be enabled from app state: {task.State}";
            global::DeskBox.App.Log($"[StoreStartupService] {failure}");
            return new StartupOperationResult(
                StartupRegistrationState.BlockedOrFailed,                failure);
        }
        catch (Exception ex)
        {
            global::DeskBox.App.Log($"[StoreStartupService] Failed to enable startup: {ex.Message}");
            return new StartupOperationResult(
                StartupRegistrationState.BlockedOrFailed,                ex.Message);
        }
    }

    public StartupOperationResult Disable()
    {
        try
        {
            var task = GetStartupTask();
            if (task.State == StartupTaskState.Enabled)
            {
                task.Disable();
            }

            return new StartupOperationResult(GetState());
        }
        catch (Exception ex)
        {
            global::DeskBox.App.Log($"[StoreStartupService] Failed to disable startup: {ex.Message}");
            return new StartupOperationResult(
                StartupRegistrationState.BlockedOrFailed,                ex.Message);
        }
    }

    public StartupOperationResult SetEnabled(bool enabled)
    {
        if (enabled)
        {
            return Enable();
        }

        return Disable();
    }

    private static StartupTask GetStartupTask()
    {
        return StartupTask.GetAsync(StartupTaskId).AsTask().GetAwaiter().GetResult();
    }
}
