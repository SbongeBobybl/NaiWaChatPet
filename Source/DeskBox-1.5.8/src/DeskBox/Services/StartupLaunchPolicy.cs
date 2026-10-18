namespace DeskBox.Services;

internal static class StartupLaunchPolicy
{
    private const string StartupArgument = "--startup";

    public static bool IsStartupLaunch(
        IEnumerable<string> processArguments,        string? activationArguments = null,        bool isStartupTaskActivation = false)
    {
        return isStartupTaskActivation ||
               processArguments.Any(IsStartupArgument) ||
               ParseActivationArguments(activationArguments).Any(IsStartupArgument);
    }

    private static IEnumerable<string> ParseActivationArguments(string? arguments)
    {
        return string.IsNullOrWhiteSpace(arguments)
            ? []
            : arguments.Split(' ',StringSplitOptions.RemoveEmptyEntries);
    }

    private static bool IsStartupArgument(string argument)
    {
        return string.Equals(
            argument.Trim().Trim('"'),            StartupArgument,            StringComparison.OrdinalIgnoreCase);
    }
}
