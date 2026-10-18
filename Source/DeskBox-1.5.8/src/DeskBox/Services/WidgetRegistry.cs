using DeskBox.Models;

namespace DeskBox.Services;

public sealed record WidgetKindRegistration(
    WidgetKind WidgetKind,    bool CanCreateWindow,    bool IsImplemented);

/// <summary>
/// Central registry for widget kinds known to DeskBox.
/// It keeps future kinds persistable without making them creatable before their windows exist.
/// </summary>
public sealed class WidgetRegistry
{
    private readonly IReadOnlyDictionary<WidgetKind, WidgetKindRegistration> _registrations;

    public static WidgetRegistry Default { get; } = CreateDefault();

    public WidgetRegistry(IEnumerable<WidgetKindRegistration> registrations)
    {
        _registrations = registrations.ToDictionary(registration => registration.WidgetKind);
    }

    public bool IsKnown(WidgetKind widgetKind)
    {
        // Recognize retired kinds during configuration migration without
        // registering a window provider. Otherwise SettingsService converts
        // old feature records into File widgets on startup.
        return _registrations.ContainsKey(widgetKind) || widgetKind is
            WidgetKind.Music or WidgetKind.Weather or WidgetKind.Glance or WidgetKind.Search or
            WidgetKind.Todo or WidgetKind.QuickCapture;
    }

    public bool CanCreateWindow(WidgetKind widgetKind)
    {
        return _registrations.TryGetValue(widgetKind,out var registration) &&
               registration.CanCreateWindow;
    }

    public bool IsImplemented(WidgetKind widgetKind)
    {
        return _registrations.TryGetValue(widgetKind,out var registration) &&
               registration.IsImplemented;
    }

    public bool IsAvailableForSession(WidgetConfig widget,AppSettings settings)
    {
        if (!CanCreateWindow(widget.WidgetKind))
        {
            return false;
        }

        if (FeatureWidgetSettings.IsFeatureWidget(widget.WidgetKind))
        {
            return FeatureWidgetSettings.IsEnabled(settings,widget.WidgetKind);
        }

        return true;
    }

    private static WidgetRegistry CreateDefault()
    {
        return new WidgetRegistry(
        [
            new(WidgetKind.File,CanCreateWindow: true,IsImplemented: true),            new(WidgetKind.Tags,CanCreateWindow: false,IsImplemented: false),            new(WidgetKind.SystemMonitor,CanCreateWindow: false,IsImplemented: false)
        ]);
    }
}
