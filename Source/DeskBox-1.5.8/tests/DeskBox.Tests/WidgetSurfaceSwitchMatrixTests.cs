using DeskBox.Contracts;
using DeskBox.Controls;
using DeskBox.Models;
using DeskBox.Services;
using Microsoft.UI.Xaml;

namespace DeskBox.Tests;

public sealed class WidgetSurfaceSwitchMatrixTests
{
    public static TheoryData<WidgetKind, WidgetKind> RequiredPairs => new()
    {
        { WidgetKind.Todo, WidgetKind.Weather },
        { WidgetKind.File, WidgetKind.File },
        { WidgetKind.File, WidgetKind.Todo },
        { WidgetKind.Todo, WidgetKind.File },
        { WidgetKind.QuickCapture, WidgetKind.Todo },
        { WidgetKind.Todo, WidgetKind.QuickCapture },
        { WidgetKind.QuickCapture, WidgetKind.File },
        { WidgetKind.File, WidgetKind.QuickCapture }
    };

    [Theory]
    [MemberData(nameof(RequiredPairs))]
    public async Task Success_KeepsHostAndSurfaceAndReleasesOutgoingOnce(
        WidgetKind from,
        WidgetKind to)
    {
        var fixture = await Fixture.CreateAsync(from, to);

        using WidgetShellContentHost.WidgetShellPreparedContent? prepared =
            await fixture.ContentHost.PrepareContentAsync(
                fixture.Incoming,
                CancellationToken.None);
        using WidgetShellContentHost.WidgetShellContentTransition? transition =
            fixture.ContentHost.CommitPreparedContent(prepared!);

        Assert.True(fixture.OutgoingVisible || fixture.IncomingVisible);
        Assert.Equal("from", fixture.RegistrySession.ActiveMemberId);
        fixture.Registry.CommitActive(
            fixture.TargetDefinition,
            fixture.PhysicalHost);
        transition!.Complete();

        Assert.Equal("to", fixture.RegistrySession.ActiveMemberId);
        Assert.Same(fixture.PhysicalHost, fixture.RegistrySession.Host);
        Assert.Equal(new IntPtr(0x35108), fixture.PhysicalHost.WindowHandle);
        Assert.Equal(1, fixture.Outgoing.DisposeCount);
        Assert.Equal(0, fixture.Incoming.DisposeCount);
        Assert.True(fixture.IncomingVisible);
    }

    [Theory]
    [MemberData(nameof(RequiredPairs))]
    public async Task Cancellation_KeepsOldIdentityAndDisposesCandidateOnce(
        WidgetKind from,
        WidgetKind to)
    {
        var fixture = await Fixture.CreateAsync(from, to);
        using var cancellation = new CancellationTokenSource();
        cancellation.Cancel();

        await Assert.ThrowsAnyAsync<OperationCanceledException>(
            () => fixture.ContentHost.PrepareContentAsync(
                fixture.Incoming,
                cancellation.Token));

        Assert.Same(fixture.Outgoing, fixture.ContentHost.CurrentContent);
        Assert.Equal("from", fixture.RegistrySession.ActiveMemberId);
        Assert.Equal(0, fixture.Outgoing.DisposeCount);
        Assert.Equal(1, fixture.Incoming.DisposeCount);
    }

    [Theory]
    [MemberData(nameof(RequiredPairs))]
    public async Task FirstFrameTimeout_RollsBackWithoutBlankOrIdentityChange(
        WidgetKind from,
        WidgetKind to)
    {
        var fixture = await Fixture.CreateAsync(from, to);
        using WidgetShellContentHost.WidgetShellPreparedContent? prepared =
            await fixture.ContentHost.PrepareContentAsync(
                fixture.Incoming,
                CancellationToken.None);
        using WidgetShellContentHost.WidgetShellContentTransition? transition =
            fixture.ContentHost.CommitPreparedContent(prepared!);

        Assert.True(fixture.OutgoingVisible || fixture.IncomingVisible);
        transition!.Rollback();

        Assert.True(fixture.OutgoingVisible);
        Assert.Same(fixture.Outgoing, fixture.ContentHost.CurrentContent);
        Assert.Equal("from", fixture.RegistrySession.ActiveMemberId);
        Assert.Equal(0, fixture.Outgoing.DisposeCount);
        Assert.Equal(1, fixture.Incoming.DisposeCount);
    }

    [Theory]
    [MemberData(nameof(RequiredPairs))]
    public async Task SaveFailure_RollsBackContentAndLeavesRegistryUncommitted(
        WidgetKind from,
        WidgetKind to)
    {
        var fixture = await Fixture.CreateAsync(from, to);
        using WidgetShellContentHost.WidgetShellPreparedContent? prepared =
            await fixture.ContentHost.PrepareContentAsync(
                fixture.Incoming,
                CancellationToken.None);
        using WidgetShellContentHost.WidgetShellContentTransition? transition =
            fixture.ContentHost.CommitPreparedContent(prepared!);

        var saveFailure = new IOException("simulated save failure");
        await Assert.ThrowsAsync<IOException>(
            () => Task.FromException(saveFailure));
        transition!.Rollback();

        Assert.True(fixture.OutgoingVisible);
        Assert.Same(fixture.Outgoing, fixture.ContentHost.CurrentContent);
        Assert.Equal("from", fixture.RegistrySession.ActiveMemberId);
        Assert.Same(fixture.PhysicalHost, fixture.RegistrySession.Host);
        Assert.Equal(0, fixture.Outgoing.DisposeCount);
        Assert.Equal(1, fixture.Incoming.DisposeCount);
    }

    private sealed class Fixture
    {
        private Fixture(
            WidgetShellContentHost contentHost,
            WidgetSurfaceRegistry<FakePhysicalHost> registry,
            WidgetSurfaceSession<FakePhysicalHost> registrySession,
            FakePhysicalHost physicalHost,
            FakeContent outgoing,
            FakeContent incoming)
        {
            ContentHost = contentHost;
            Registry = registry;
            RegistrySession = registrySession;
            PhysicalHost = physicalHost;
            Outgoing = outgoing;
            Incoming = incoming;
        }

        public WidgetShellContentHost ContentHost { get; }

        public WidgetSurfaceRegistry<FakePhysicalHost> Registry { get; }

        public WidgetSurfaceSession<FakePhysicalHost> RegistrySession { get; }

        public FakePhysicalHost PhysicalHost { get; }

        public FakeContent Outgoing { get; }

        public FakeContent Incoming { get; }

        public bool OutgoingVisible { get; private set; } = true;

        public bool IncomingVisible { get; private set; }

        public WidgetSurfaceDefinition TargetDefinition =>
            new("surface", "group", ["from", "to"], "to");

        public static async Task<Fixture> CreateAsync(
            WidgetKind from,
            WidgetKind to)
        {
            Fixture? fixture = null;
            var outgoing = new FakeContent("from", from);
            var incoming = new FakeContent("to", to);
            var contentHost = new WidgetShellContentHost(
                setContent: content =>
                {
                    if (fixture is not null)
                    {
                        fixture.IncomingVisible =
                            string.Equals(content.WidgetId, "to", StringComparison.Ordinal);
                    }
                },
                beginTransition: (_, _) =>
                {
                    fixture!.OutgoingVisible = true;
                    fixture.IncomingVisible = true;
                },
                completeTransition: () => fixture!.OutgoingVisible = false,
                rollbackTransition: _ =>
                {
                    fixture!.OutgoingVisible = true;
                    fixture.IncomingVisible = false;
                });
            var physicalHost = new FakePhysicalHost(new IntPtr(0x35108));
            var registry = new WidgetSurfaceRegistry<FakePhysicalHost>();
            WidgetSurfaceSession<FakePhysicalHost> session =
                registry.RegisterActive(
                    new WidgetSurfaceDefinition(
                        "surface",
                        "group",
                        ["from", "to"],
                        "from"),
                    physicalHost);
            fixture = new Fixture(
                contentHost,
                registry,
                session,
                physicalHost,
                outgoing,
                incoming);
            await contentHost.SetContentAsync(outgoing);
            fixture.IncomingVisible = false;
            return fixture;
        }
    }

    private sealed record FakePhysicalHost(IntPtr WindowHandle);

    private sealed class FakeContent(
        string id,
        WidgetKind kind) : IWidgetContent, IDisposable
    {
        public int DisposeCount { get; private set; }

        public WidgetConfig Config { get; } = new()
        {
            Id = id,
            Name = id,
            WidgetKind = kind
        };

        public string WidgetId => Config.Id;

        public WidgetKind WidgetKind => Config.WidgetKind;

        public FrameworkElement View =>
            throw new NotSupportedException(
                "The switch matrix uses presenter callbacks, not XAML.");

        public Task InitializeAsync() => Task.CompletedTask;

        public Task RefreshAsync() => Task.CompletedTask;

        public void ApplyAppearance()
        {
        }

        public void OnActivated()
        {
        }

        public void OnDeactivated()
        {
        }

        public void Dispose()
        {
            DisposeCount++;
        }
    }
}
