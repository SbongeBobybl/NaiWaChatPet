namespace DeskBox.Services;

/// <summary>
/// Registration creates the native deserializer's wait handle. Never read rich
/// activation arguments before registration succeeds, or repeatedly read a cold
/// launch payload. The event callback is subscribed by the registration delegate.
/// </summary>
internal sealed class NativeNotificationActivationBootstrap(
    Func<bool> register,    Func<NativeAppNotificationActivation?> read,    Action<Exception> logFailure)
{
    private bool _readAttempted;
    private NativeAppNotificationActivation? _activation;

    internal NativeAppNotificationActivation? Capture()
    {
        try
        {
            if (!register())
                return null;
            if (!_readAttempted)
            {
                _readAttempted = true;
                _activation = read();
            }
            return _activation;
        }
        catch (Exception ex)
        {
            logFailure(ex);
            return null;
        }
    }

    internal static bool IsNotificationLaunch(IEnumerable<string> arguments) =>
        arguments.Any(argument => argument.StartsWith("----AppNotificationActivated:",StringComparison.Ordinal));
}
