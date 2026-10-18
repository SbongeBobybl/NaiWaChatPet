using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class NativeNotificationActivationBootstrapTests
{
    [Fact]
    public void RegistrationPrecedesNativeDeserialize_AndColdArgsAreReadOnlyOnce()
    {
        var calls = new List<string>();
        var expected = new NativeAppNotificationActivation("source=todoReminder", new Dictionary<string, string>());
        var bootstrap = new NativeNotificationActivationBootstrap(
            () => { calls.Add("register"); return true; },
            () => { calls.Add("deserialize"); return expected; },
            _ => Assert.Fail("Unexpected failure"));
        Assert.Same(expected, bootstrap.Capture());
        Assert.Same(expected, bootstrap.Capture());
        Assert.Equal(new[] { "register", "deserialize", "register" }, calls);
    }

    [Theory]
    [InlineData(false)]
    [InlineData(true)]
    public void UnavailableNotificationsNeverEnterNativeDeserialize_AndCanRecover(bool throws)
    {
        bool available = false;
        int reads = 0;
        var failures = new List<Exception>();
        var bootstrap = new NativeNotificationActivationBootstrap(
            () => available ? true : throws ? throw new InvalidOperationException("registration unavailable") : false,
            () => { reads++; return null; }, failures.Add);
        Assert.Null(bootstrap.Capture());
        Assert.Equal(0, reads);
        Assert.Equal(throws ? 1 : 0, failures.Count);
        available = true;
        Assert.Null(bootstrap.Capture());
        Assert.Equal(1, reads);
    }

    [Fact]
    public void FailedActivationReadIsNotRepeatedDuringUiInitialization()
    {
        int reads = 0;
        var failures = new List<Exception>();
        var bootstrap = new NativeNotificationActivationBootstrap(() => true,
            () => { reads++; throw new TimeoutException(); }, failures.Add);
        Assert.Null(bootstrap.Capture());
        Assert.Null(bootstrap.Capture());
        Assert.Equal(1, reads);
        Assert.Single(failures);
    }

    [Theory]
    [InlineData("----AppNotificationActivated:", true)]
    [InlineData("----AppNotificationActivated:payload", true)]
    [InlineData("--startup", false)]
    [InlineData("--open-settings", false)]
    public void NotificationLaunchIsDistinguishedFromBareAndStartupLaunches(string argument, bool expected)
    {
        Assert.Equal(expected, NativeNotificationActivationBootstrap.IsNotificationLaunch(["DeskBox.exe", argument]));
    }
}
