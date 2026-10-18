namespace DeskBox.Services;

using DeskBox.Models;

public enum WidgetTitleIconMode
{
    FilledMono,
    LineMono,
    Color,
    Hidden,
    TextLabel
}

public static class WidgetTitleIconModeNames
{
    public const string FilledMono = Contracts.WidgetTitleIconKinds.FilledMono;
    public const string LineMono = Contracts.WidgetTitleIconKinds.LineMono;
    public const string Color = Contracts.WidgetTitleIconKinds.Color;
    public const string Hidden = Contracts.WidgetTitleIconKinds.Hidden;
    public const string TextLabel = Contracts.WidgetTitleIconKinds.TextLabel;

    public static WidgetTitleIconMode NormalizeMode(string? value)
    {
        return Enum.TryParse(value,ignoreCase: true,out WidgetTitleIconMode mode)
            ? mode
            : WidgetTitleIconMode.Color;
    }

    public static string NormalizeSettingValue(string? value)
    {
        return ToSettingValue(NormalizeMode(value));
    }

    public static string ToSettingValue(WidgetTitleIconMode mode)
    {
        return mode switch
        {
            WidgetTitleIconMode.FilledMono => FilledMono,            WidgetTitleIconMode.LineMono => LineMono,            WidgetTitleIconMode.Color => Color,            WidgetTitleIconMode.Hidden => Hidden,            WidgetTitleIconMode.TextLabel => TextLabel,            _ => Color
        };
    }
}

public enum WidgetTitleIconKind
{
    Default,
    ManagedStorage,
    MappedFolder,
    QuickCapture,
    Todo,
    Music,
    Weather,
    Glance,
    Tags,
    Search,
    SystemMonitor,

    // NaiWa organizer category boxes. Six distinct palettes so a row of
    // categories is readable at a glance instead of six identical folders.
    NaiWa1,
    NaiWa2,
    NaiWa3,
    NaiWa4,
    NaiWa5,
    NaiWa6
}

public static class WidgetTitleIconKindNames
{
    public const string Default = nameof(WidgetTitleIconKind.Default);
    public const string ManagedStorage = nameof(WidgetTitleIconKind.ManagedStorage);
    public const string MappedFolder = nameof(WidgetTitleIconKind.MappedFolder);
    public const string QuickCapture = nameof(WidgetTitleIconKind.QuickCapture);
    public const string Todo = nameof(WidgetTitleIconKind.Todo);
    public const string Music = nameof(WidgetTitleIconKind.Music);
    public const string Weather = nameof(WidgetTitleIconKind.Weather);
    public const string Glance = nameof(WidgetTitleIconKind.Glance);
    public const string Tags = nameof(WidgetTitleIconKind.Tags);
    public const string Search = nameof(WidgetTitleIconKind.Search);
    public const string SystemMonitor = nameof(WidgetTitleIconKind.SystemMonitor);
    public const string NaiWa1 = nameof(WidgetTitleIconKind.NaiWa1);
    public const string NaiWa2 = nameof(WidgetTitleIconKind.NaiWa2);
    public const string NaiWa3 = nameof(WidgetTitleIconKind.NaiWa3);
    public const string NaiWa4 = nameof(WidgetTitleIconKind.NaiWa4);
    public const string NaiWa5 = nameof(WidgetTitleIconKind.NaiWa5);
    public const string NaiWa6 = nameof(WidgetTitleIconKind.NaiWa6);

    /// <summary>The six NaiWa category icon variants, in display order.</summary>
    public static readonly WidgetTitleIconKind[] NaiWaCategoryKinds =
    [
        WidgetTitleIconKind.NaiWa1, WidgetTitleIconKind.NaiWa2, WidgetTitleIconKind.NaiWa3,
        WidgetTitleIconKind.NaiWa4, WidgetTitleIconKind.NaiWa5, WidgetTitleIconKind.NaiWa6
    ];

    /// <summary>Stable variant for a category slot, so a re-run keeps its look.</summary>
    public static WidgetTitleIconKind NaiWaCategoryKindFor(int index) =>
        NaiWaCategoryKinds[((index % NaiWaCategoryKinds.Length) + NaiWaCategoryKinds.Length)
            % NaiWaCategoryKinds.Length];

    public static bool IsNaiWaCategoryKind(WidgetTitleIconKind kind) =>
        kind is WidgetTitleIconKind.NaiWa1 or WidgetTitleIconKind.NaiWa2
            or WidgetTitleIconKind.NaiWa3 or WidgetTitleIconKind.NaiWa4
            or WidgetTitleIconKind.NaiWa5 or WidgetTitleIconKind.NaiWa6;

    public static WidgetTitleIconKind NormalizeKind(string? value)
    {
        return Enum.TryParse(value,ignoreCase: true,out WidgetTitleIconKind kind)
            ? kind
            : WidgetTitleIconKind.Default;
    }

    public static string ToSettingValue(WidgetTitleIconKind kind)
    {
        return kind switch
        {
            WidgetTitleIconKind.ManagedStorage => ManagedStorage,            WidgetTitleIconKind.MappedFolder => MappedFolder,            WidgetTitleIconKind.QuickCapture => QuickCapture,            WidgetTitleIconKind.Todo => Todo,            WidgetTitleIconKind.Music => Music,            WidgetTitleIconKind.Weather => Weather,            WidgetTitleIconKind.Glance => Glance,            WidgetTitleIconKind.Tags => Tags,            WidgetTitleIconKind.Search => Search,            WidgetTitleIconKind.SystemMonitor => SystemMonitor,            WidgetTitleIconKind.NaiWa1 => NaiWa1,            WidgetTitleIconKind.NaiWa2 => NaiWa2,            WidgetTitleIconKind.NaiWa3 => NaiWa3,            WidgetTitleIconKind.NaiWa4 => NaiWa4,            WidgetTitleIconKind.NaiWa5 => NaiWa5,            WidgetTitleIconKind.NaiWa6 => NaiWa6,            _ => Default
        };
    }

    public static string FromFileWidget(bool followsDefaultStoragePath)
    {
        return followsDefaultStoragePath ? ManagedStorage : MappedFolder;
    }

    public static string FromWidgetKind(WidgetKind kind)
    {
        return kind switch
        {
            WidgetKind.File => ManagedStorage,            WidgetKind.QuickCapture => QuickCapture,            WidgetKind.Todo => Todo,            WidgetKind.Tags => Tags,            WidgetKind.SystemMonitor => SystemMonitor,            _ => Default
        };
    }

    public static string FromLegacyGlyph(string? glyph)
    {
        return glyph switch
        {
            "\uE8B7" => ManagedStorage,            "\uE71B" => MappedFolder,            "\uE70F" => QuickCapture,            "\uE9D5" => Todo,            "\uEC4F" => Music,            "\uE706" => Weather,            "\uE8EC" => Tags,            "\uE721" => Search,            "\uE9D9" => SystemMonitor,            _ => Default
        };
    }

    public static string GetLocalizationKey(WidgetTitleIconKind kind)
    {
        return kind switch
        {
            WidgetTitleIconKind.ManagedStorage => "WidgetTitleIcon.Label.ManagedStorage",            WidgetTitleIconKind.MappedFolder => "WidgetTitleIcon.Label.MappedFolder",            WidgetTitleIconKind.QuickCapture => "WidgetTitleIcon.Label.QuickCapture",            WidgetTitleIconKind.Todo => "WidgetTitleIcon.Label.Todo",            WidgetTitleIconKind.Music => "WidgetTitleIcon.Label.Music",            WidgetTitleIconKind.Weather => "WidgetTitleIcon.Label.Weather",            WidgetTitleIconKind.Glance => "Glance.Title",            WidgetTitleIconKind.Tags => "WidgetTitleIcon.Label.Tags",            WidgetTitleIconKind.Search => "WidgetTitleIcon.Label.Search",            WidgetTitleIconKind.SystemMonitor => "WidgetTitleIcon.Label.SystemMonitor",            _ => "WidgetTitleIcon.Label.Default"
        };
    }

    public static string GetColorAssetName(WidgetTitleIconKind kind)
    {
        return kind switch
        {
            WidgetTitleIconKind.ManagedStorage => "managed-storage",            WidgetTitleIconKind.MappedFolder => "mapped-folder",            WidgetTitleIconKind.QuickCapture => "quick-capture",            WidgetTitleIconKind.Todo => "todo",            WidgetTitleIconKind.Music => "music",            WidgetTitleIconKind.Weather => "weather",            WidgetTitleIconKind.Glance => "glance",            WidgetTitleIconKind.Tags => "tags",            WidgetTitleIconKind.Search => "search",            WidgetTitleIconKind.SystemMonitor => "system-monitor",            WidgetTitleIconKind.NaiWa1 => "naiwa-1",            WidgetTitleIconKind.NaiWa2 => "naiwa-2",            WidgetTitleIconKind.NaiWa3 => "naiwa-3",            WidgetTitleIconKind.NaiWa4 => "naiwa-4",            WidgetTitleIconKind.NaiWa5 => "naiwa-5",            WidgetTitleIconKind.NaiWa6 => "naiwa-6",            _ => "default"
        };
    }
}
