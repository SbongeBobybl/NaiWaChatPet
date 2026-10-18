using System.Security.Cryptography;
using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class TodoNotificationActivationRouterTests : IDisposable
{
    private static readonly DateTimeOffset FixedNow =
        new(2026, 8, 25, 8, 15, 0, TimeSpan.FromHours(8));
    private static readonly TimeZoneInfo FixedTimeZone =
        TimeZoneInfo.CreateCustomTimeZone(
            "DeskBox.Tests.UTC+08",
            TimeSpan.FromHours(8),
            "DeskBox Tests UTC+08",
            "DeskBox Tests UTC+08");

    private readonly string _tempRoot;
    private readonly string _settingsRoot;
    private readonly string _widgetsRoot;

    public TodoNotificationActivationRouterTests()
    {
        _tempRoot = Path.Combine(
            Path.GetTempPath(),
            "DeskBox.Tests",
            Guid.NewGuid().ToString("N"));
        _settingsRoot = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "settings")).FullName;
        _widgetsRoot = Directory.CreateDirectory(
            Path.Combine(_tempRoot, "widgets")).FullName;
    }

    [Fact]
    public async Task RouteAsync_BodyRequestsExactTodayTargetWithoutMutation()
    {
        const string widgetId = "todo-widget";
        const string itemId = "open-item";
        var harness = await CreateHarnessAsync(widgetId, itemId);
        string beforeHash = GetStoreHash(harness.Store);

        TodoNotificationActivationRouteResult result = await harness.RouteAsync(
            new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
            {
                ["source"] = TodoNotificationActivationRouter.SourceValue,
                ["widgetId"] = widgetId,
                ["itemId"] = itemId,
                ["view"] = "today"
            },
            new Dictionary<string, string>());

        Assert.Equal(TodoNotificationActivationRouter.DispositionOpened, result.Disposition);
        Assert.True(result.Succeeded);
        Assert.True(result.TargetRequested);
        Assert.True(result.TargetPresented);
        Assert.Equal((widgetId, itemId, true), Assert.Single(harness.Targets));
        Assert.Empty(harness.Refreshes);
        Assert.Empty(harness.Confirmations);
        Assert.Equal(beforeHash, GetStoreHash(harness.Store));
    }

    [Fact]
    public async Task RouteAsync_BodyReportsUnavailableWhenSurfaceDoesNotPresentTarget()
    {
        TodoNotificationActivationRouteResult result =
            await TodoNotificationActivationRouter.RouteAsync(
                new Dictionary<string, string>
                {
                    ["source"] = TodoNotificationActivationRouter.SourceValue,
                    ["widgetId"] = "missing-widget",
                    ["itemId"] = "missing-item"
                },
                new Dictionary<string, string>(),
                reminderService: null,
                () => FixedNow,
                FixedTimeZone,
                (_, _, _) => Task.FromResult(false),
                _ => Task.FromResult(false),
                _ => Task.CompletedTask);

        Assert.Equal(
            TodoNotificationActivationRouter.DispositionTargetUnavailable,
            result.Disposition);
        Assert.False(result.Succeeded);
        Assert.True(result.TargetRequested);
        Assert.False(result.TargetPresented);
    }

    [Fact]
    public async Task RouteAsync_CompleteIsPersistedAndIdempotent()
    {
        const string widgetId = "todo-widget";
        const string itemId = "complete-item";
        var harness = await CreateHarnessAsync(widgetId, itemId);
        var arguments = CreateActionArguments(
            widgetId,
            itemId,
            TodoNotificationActivationRouter.ActionComplete);

        TodoNotificationActivationRouteResult first = await harness.RouteAsync(
            arguments,
            new Dictionary<string, string>());
        string firstHash = GetStoreHash(harness.Store);
        TodoNotificationActivationRouteResult second = await harness.RouteAsync(
            arguments,
            new Dictionary<string, string>());

        Assert.Equal(TodoNotificationActivationRouter.DispositionCompleted, first.Disposition);
        Assert.Equal(TodoNotificationActivationRouter.DispositionCompleted, second.Disposition);
        Assert.True(first.Succeeded);
        Assert.True(second.Succeeded);
        Assert.True(first.RefreshCompleted);
        Assert.True(second.RefreshCompleted);
        TodoItem item = Assert.Single((await harness.Store.LoadAsync()).Items);
        Assert.True(item.IsCompleted);
        Assert.Equal(FixedNow.ToUniversalTime(), item.CompletedAt);
        Assert.Equal(firstHash, GetStoreHash(harness.Store));
        Assert.Equal([widgetId, widgetId], harness.Refreshes);
        Assert.Empty(harness.Targets);
        Assert.Empty(harness.Confirmations);
    }

    [Theory]
    [InlineData(TodoNotificationActivationRouter.Snooze10Minutes, 10)]
    [InlineData(TodoNotificationActivationRouter.Snooze30Minutes, 30)]
    [InlineData(TodoNotificationActivationRouter.Snooze1Hour, 60)]
    public async Task RouteAsync_RelativeSnoozePersistsExactSelection(
        string selection,
        int expectedMinutes)
    {
        const string widgetId = "todo-widget";
        const string itemId = "snooze-item";
        var harness = await CreateHarnessAsync(widgetId, itemId);
        var userInput = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
        {
            [TodoNotificationActivationRouter.SnoozeInputId] = selection
        };

        TodoNotificationActivationRouteResult first = await harness.RouteAsync(
            CreateActionArguments(
                widgetId,
                itemId,
                TodoNotificationActivationRouter.ActionSnooze),
            userInput);
        string firstHash = GetStoreHash(harness.Store);
        TodoNotificationActivationRouteResult second = await harness.RouteAsync(
            CreateActionArguments(
                widgetId,
                itemId,
                TodoNotificationActivationRouter.ActionSnooze),
            userInput);

        DateTimeOffset expectedUntil = FixedNow.AddMinutes(expectedMinutes);
        Assert.Equal(TodoNotificationActivationRouter.DispositionSnoozed, first.Disposition);
        Assert.Equal(expectedUntil, first.SnoozedUntil);
        Assert.Equal(expectedUntil, second.SnoozedUntil);
        TodoItem item = Assert.Single((await harness.Store.LoadAsync()).Items);
        Assert.Equal(expectedUntil, item.SnoozedUntil);
        Assert.Equal(item.DueDate, item.ReminderDismissedForDueDate);
        Assert.Equal(firstHash, GetStoreHash(harness.Store));
        Assert.Equal([selection, selection], harness.Confirmations);
        Assert.Equal([widgetId, widgetId], harness.Refreshes);
        Assert.True(first.RefreshCompleted);
        Assert.True(second.RefreshCompleted);
    }

    [Fact]
    public async Task RouteAsync_TomorrowUsesInjectedLocalCalendar()
    {
        const string widgetId = "todo-widget";
        const string itemId = "tomorrow-item";
        var harness = await CreateHarnessAsync(widgetId, itemId);

        TodoNotificationActivationRouteResult result = await harness.RouteAsync(
            CreateActionArguments(
                widgetId,
                itemId,
                TodoNotificationActivationRouter.ActionSnooze),
            new Dictionary<string, string>
            {
                [TodoNotificationActivationRouter.SnoozeInputId] =
                    TodoNotificationActivationRouter.SnoozeTomorrow
            });

        DateTimeOffset expected =
            new(2026, 8, 26, 9, 0, 0, TimeSpan.FromHours(8));
        Assert.Equal(TodoNotificationActivationRouter.DispositionSnoozed, result.Disposition);
        Assert.Equal(expected, result.SnoozedUntil);
        Assert.Equal(
            expected,
            Assert.Single((await harness.Store.LoadAsync()).Items).SnoozedUntil);
    }

    [Fact]
    public async Task RouteAsync_LegacySnooze10DoesNotRequireUserInput()
    {
        const string widgetId = "todo-widget";
        const string itemId = "legacy-item";
        var harness = await CreateHarnessAsync(widgetId, itemId);

        TodoNotificationActivationRouteResult result = await harness.RouteAsync(
            CreateActionArguments(
                widgetId,
                itemId,
                TodoNotificationActivationRouter.LegacyActionSnooze10),
            new Dictionary<string, string>());

        Assert.True(result.Succeeded);
        Assert.Equal(
            TodoNotificationActivationRouter.Snooze10Minutes,
            result.SnoozeSelection);
        Assert.Equal(FixedNow.AddMinutes(10), result.SnoozedUntil);
    }

    [Fact]
    public async Task RouteAsync_RejectsMissingOrUnsupportedSelectionWithoutMutation()
    {
        const string widgetId = "todo-widget";
        const string itemId = "invalid-item";
        var harness = await CreateHarnessAsync(widgetId, itemId);
        var arguments = CreateActionArguments(
            widgetId,
            itemId,
            TodoNotificationActivationRouter.ActionSnooze);
        string beforeHash = GetStoreHash(harness.Store);

        TodoNotificationActivationRouteResult missing = await harness.RouteAsync(
            arguments,
            new Dictionary<string, string>());
        TodoNotificationActivationRouteResult unsupported = await harness.RouteAsync(
            arguments,
            new Dictionary<string, string>
            {
                [TodoNotificationActivationRouter.SnoozeInputId] = "next-week"
            });

        Assert.Equal(
            TodoNotificationActivationRouter.DispositionRejectedUnsupportedSnooze,
            missing.Disposition);
        Assert.Equal(
            TodoNotificationActivationRouter.DispositionRejectedUnsupportedSnooze,
            unsupported.Disposition);
        Assert.False(missing.Succeeded);
        Assert.False(unsupported.Succeeded);
        Assert.Equal(beforeHash, GetStoreHash(harness.Store));
        Assert.Empty(harness.Refreshes);
        Assert.Empty(harness.Confirmations);
        Assert.Empty(harness.Targets);
    }

    [Fact]
    public async Task RouteAsync_RejectsUnknownActionAndMissingTarget()
    {
        const string widgetId = "todo-widget";
        const string itemId = "invalid-item";
        var harness = await CreateHarnessAsync(widgetId, itemId);
        string beforeHash = GetStoreHash(harness.Store);

        TodoNotificationActivationRouteResult unknown = await harness.RouteAsync(
            CreateActionArguments(widgetId, itemId, "delete"),
            new Dictionary<string, string>());
        TodoNotificationActivationRouteResult missingTarget = await harness.RouteAsync(
            new Dictionary<string, string>
            {
                ["source"] = TodoNotificationActivationRouter.SourceValue,
                ["action"] = TodoNotificationActivationRouter.ActionComplete,
                ["widgetId"] = widgetId
            },
            new Dictionary<string, string>());

        Assert.Equal(
            TodoNotificationActivationRouter.DispositionRejectedUnsupportedAction,
            unknown.Disposition);
        Assert.Equal(
            TodoNotificationActivationRouter.DispositionRejectedMissingTarget,
            missingTarget.Disposition);
        Assert.Equal(beforeHash, GetStoreHash(harness.Store));
        Assert.Empty(harness.Refreshes);
        Assert.Empty(harness.Targets);
    }

    public void Dispose()
    {
        try
        {
            if (Directory.Exists(_tempRoot))
            {
                Directory.Delete(_tempRoot, recursive: true);
            }
        }
        catch
        {
        }
    }

    private async Task<RouterHarness> CreateHarnessAsync(
        string widgetId,
        string itemId)
    {
        var settingsService = new SettingsService(_settingsRoot);
        settingsService.Settings.TodoReminderEnabled = true;
        settingsService.Settings.Widgets =
        [
            new WidgetConfig
            {
                Id = widgetId,
                Name = "Todo",
                WidgetKind = WidgetKind.Todo
            }
        ];
        FeatureWidgetSettings.SetEnabled(
            settingsService.Settings,
            WidgetKind.Todo,
            true);

        var store = new TodoWidgetStore(_widgetsRoot, widgetId);
        await store.SaveAsync(new TodoWidgetData
        {
            Items =
            [
                new TodoItem
                {
                    Id = itemId,
                    Text = itemId,
                    DueDate = FixedNow.AddHours(2),
                    ReminderOffsetMinutes = 5,
                    CreatedAt = FixedNow.AddDays(-1),
                    UpdatedAt = FixedNow.AddDays(-1)
                }
            ]
        });

        var service = new TodoReminderService(
            settingsService,
            TestServices.CreateLocalizationService(),
            dispatcherQueue: null,
            _ => { },
            id => new TodoWidgetStore(_widgetsRoot, id),
            () => FixedNow);
        return new RouterHarness(store, service);
    }

    private static Dictionary<string, string> CreateActionArguments(
        string widgetId,
        string itemId,
        string action)
    {
        return new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
        {
            ["source"] = TodoNotificationActivationRouter.SourceValue,
            ["action"] = action,
            ["widgetId"] = widgetId,
            ["itemId"] = itemId
        };
    }

    private static string GetStoreHash(TodoWidgetStore store)
    {
        return Convert.ToHexString(SHA256.HashData(File.ReadAllBytes(store.StorePath)));
    }

    private sealed class RouterHarness(
        TodoWidgetStore store,
        TodoReminderService service)
    {
        public TodoWidgetStore Store { get; } = store;
        public List<(string? WidgetId, string? ItemId, bool PreferToday)> Targets { get; } = [];
        public List<string?> Refreshes { get; } = [];
        public List<string> Confirmations { get; } = [];

        public Task<TodoNotificationActivationRouteResult> RouteAsync(
            IReadOnlyDictionary<string, string> arguments,
            IReadOnlyDictionary<string, string> userInput)
        {
            return TodoNotificationActivationRouter.RouteAsync(
                arguments,
                userInput,
                service,
                () => FixedNow,
                FixedTimeZone,
                (widgetId, itemId, preferToday) =>
                {
                    Targets.Add((widgetId, itemId, preferToday));
                    return Task.FromResult(true);
                },
                widgetId =>
                {
                    Refreshes.Add(widgetId);
                    return Task.FromResult(true);
                },
                selection =>
                {
                    Confirmations.Add(selection);
                    return Task.CompletedTask;
                });
        }
    }
}
