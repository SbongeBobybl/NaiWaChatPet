using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class AutoStartDefaultPolicyTests
{
    [Fact]
    public void ShouldApply_IsFalseOnceTheDefaultWasApplied()
    {
        Assert.True(AutoStartDefaultPolicy.ShouldApply(new AppSettings()));
        Assert.False(AutoStartDefaultPolicy.ShouldApply(
            new AppSettings { AutoStartDefaultApplied = true }));
    }

    [Fact]
    public void Resolve_EnablesOnlyANeverRegisteredStartup()
    {
        var startup = new RecordingStartupService(StartupRegistrationState.NotRegistered)
        {
            EnableResult = StartupRegistrationState.Enabled
        };

        StartupRegistrationState state = AutoStartDefaultPolicy.Resolve(startup);

        Assert.Equal(StartupRegistrationState.Enabled, state);
        Assert.Equal(1, startup.EnableCalls);
    }

    [Theory]
    [InlineData(StartupRegistrationState.Enabled)]
    [InlineData(StartupRegistrationState.Pending)]
    [InlineData(StartupRegistrationState.DisabledByUser)]
    [InlineData(StartupRegistrationState.DisabledByTaskScheduler)]
    [InlineData(StartupRegistrationState.BlockedOrFailed)]
    [InlineData(StartupRegistrationState.PathMismatch)]
    public void Resolve_NeverOverridesAnExistingRegistration(StartupRegistrationState current)
    {
        var startup = new RecordingStartupService(current);

        StartupRegistrationState state = AutoStartDefaultPolicy.Resolve(startup);

        Assert.Equal(current, state);
        Assert.Equal(0, startup.EnableCalls);
    }

    [Fact]
    public void Resolve_ReportsAWindowsConsentPendingEnable()
    {
        var startup = new RecordingStartupService(StartupRegistrationState.NotRegistered)
        {
            EnableResult = StartupRegistrationState.Pending
        };

        StartupRegistrationState state = AutoStartDefaultPolicy.Resolve(startup);

        Assert.Equal(StartupRegistrationState.Pending, state);
        Assert.True(AutoStartDefaultPolicy.IsEnabledState(state));
    }

    [Fact]
    public void Resolve_ReportsAFailedEnableWithoutInventingSuccess()
    {
        var startup = new RecordingStartupService(StartupRegistrationState.NotRegistered)
        {
            EnableResult = StartupRegistrationState.BlockedOrFailed
        };

        StartupRegistrationState state = AutoStartDefaultPolicy.Resolve(startup);

        Assert.Equal(StartupRegistrationState.BlockedOrFailed, state);
        Assert.False(AutoStartDefaultPolicy.IsEnabledState(state));
    }

    [Theory]
    [InlineData(StartupRegistrationState.Enabled, true)]
    [InlineData(StartupRegistrationState.Pending, true)]
    [InlineData(StartupRegistrationState.NotRegistered, false)]
    [InlineData(StartupRegistrationState.DisabledByUser, false)]
    [InlineData(StartupRegistrationState.DisabledByTaskScheduler, false)]
    [InlineData(StartupRegistrationState.BlockedOrFailed, false)]
    [InlineData(StartupRegistrationState.PathMismatch, false)]
    public void IsEnabledState_MirrorsOnlyLiveRegistrations(
        StartupRegistrationState state,
        bool expected)
    {
        Assert.Equal(expected, AutoStartDefaultPolicy.IsEnabledState(state));
    }

    private sealed class RecordingStartupService(StartupRegistrationState state)
        : IStartupService
    {
        public int EnableCalls { get; private set; }

        public StartupRegistrationState EnableResult { get; init; } =
            StartupRegistrationState.Enabled;

        public StartupRegistrationState GetState() => state;

        public bool IsEnabled() => state == StartupRegistrationState.Enabled;

        public string? GetRunValue() => null;

        public StartupOperationResult Enable()
        {
            EnableCalls++;
            return new StartupOperationResult(EnableResult);
        }

        public StartupOperationResult Disable() =>
            new(StartupRegistrationState.NotRegistered);

        public StartupOperationResult SetEnabled(bool enabled) =>
            enabled ? Enable() : Disable();
    }
}
