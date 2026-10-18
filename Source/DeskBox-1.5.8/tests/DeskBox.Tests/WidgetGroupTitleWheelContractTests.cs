namespace DeskBox.Tests;

public sealed class WidgetGroupTitleWheelContractTests
{
    [Fact]
    public void WheelNavigation_CommitsAtMostOncePerGesture()
    {
        string root = FindRepositoryRoot();
        string interaction = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Controls/WidgetGroupTitleSwitcher.Interaction.cs"));
        string host = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Views/WidgetWindowBase.Grouping.cs"));
        string manager = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Services/WidgetManager.Groups.cs"));
        string coordinator = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Services/WidgetGroupSwitchRequestCoordinator.cs"));

        Assert.Contains(
            "wrap: origin is WidgetGroupSwitchOrigin.Keyboard or",
            interaction,
            StringComparison.Ordinal);
        Assert.Contains(
            "WidgetGroupSwitchOrigin.Wheel",
            interaction,
            StringComparison.Ordinal);
        Assert.DoesNotContain(
            "WheelCooldown",
            interaction,
            StringComparison.Ordinal);
        Assert.DoesNotContain("_lastWheelSwitchAt", interaction, StringComparison.Ordinal);
        Assert.DoesNotContain("TryAcceptWheelStep", manager, StringComparison.Ordinal);
        Assert.DoesNotContain("WheelGestureQuietPeriod", coordinator, StringComparison.Ordinal);
        Assert.DoesNotContain(
            "ResetWheelStepCoalescing",
            interaction,
            StringComparison.Ordinal);
        Assert.DoesNotContain("valleyOpacity", interaction, StringComparison.Ordinal);
        Assert.DoesNotContain("echoOpacity", interaction, StringComparison.Ordinal);
        Assert.DoesNotContain("_wheelFeedbackBurst", interaction, StringComparison.Ordinal);
        Assert.Contains(
            "TryConsumeWheelGestureStep",
            interaction,
            StringComparison.Ordinal);
        Assert.True(
            interaction.IndexOf("TryConsumeWheelGestureStep", StringComparison.Ordinal) <
            interaction.IndexOf("AnimateWheelDirectionFeedback", StringComparison.Ordinal));
        Assert.Contains(
            "NotifyMemberInvocationCompleted",
            interaction,
            StringComparison.Ordinal);
        Assert.DoesNotContain(
            "!string.Equals(e.WidgetId, Config.Id",
            host,
            StringComparison.Ordinal);
    }

    private static string FindRepositoryRoot()
    {
        DirectoryInfo? current = new(AppContext.BaseDirectory);
        while (current is not null)
        {
            if (Directory.Exists(Path.Combine(current.FullName, "src", "DeskBox")))
            {
                return current.FullName;
            }

            current = current.Parent;
        }

        throw new DirectoryNotFoundException("Could not locate repository root.");
    }
}
