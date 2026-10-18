using DeskBox.Models;

namespace DeskBox.Services;

/// <summary>
/// Shared read-only diagnostics for widget host windows.
/// This does not own z-order, animation, DWM, or visibility decisions.
/// </summary>
public sealed class WidgetWindowDiagnostics
{
    private const double MinAnimationExtent = 1.0;
    private WidgetConfig _config;
    private readonly Func<IntPtr> _windowHandleProvider;
    private string _surfaceId;

    public WidgetWindowDiagnostics(string logKind,WidgetConfig config,Func<IntPtr> windowHandleProvider)
    {
        LogKind = logKind;
        _config = config;
        _surfaceId = config.Id;
        _windowHandleProvider = windowHandleProvider;
    }

    public string LogKind { get; }

    public string ShortWidgetId => ShortId(_config.Id);

    public string SurfaceId =>
        string.IsNullOrWhiteSpace(_surfaceId) ? _config.Id : _surfaceId;

    public string ShortSurfaceId => ShortId(SurfaceId);

    public WidgetWindowIdentity Identity => new(
        WidgetId: _config.Id,        SurfaceId: SurfaceId,        WidgetKind: _config.WidgetKind,        Name: _config.Name,        LogKind: LogKind,        ShortWidgetId: ShortWidgetId,        ShortSurfaceId: ShortSurfaceId,        WindowHandle: _windowHandleProvider(),        AnimationBounds: AnimationBounds);

    internal void SetWidgetContext(WidgetConfig config)
    {
        ArgumentNullException.ThrowIfNull(config);
        _config = config;
    }

    internal void SetSurfaceId(string? surfaceId)
    {
        _surfaceId = string.IsNullOrWhiteSpace(surfaceId)
            ? _config.Id
            : surfaceId;
    }

    public Windows.Foundation.Rect AnimationBounds => new(
        _config.X,        _config.Y,        Math.Max(MinAnimationExtent,_config.Width),        Math.Max(MinAnimationExtent,_config.Height));

    public string FormatTrayWindowMessage(string message)
    {
        return $"[TrayWindow] {LogKind} {_config.Name}#{ShortWidgetId} hwnd=0x{_windowHandleProvider().ToInt64():X} {message}";
    }

    internal static string ShortId(string id)
    {
        return string.IsNullOrWhiteSpace(id)
            ? "none"
            : id.Length <= 8 ? id : id[..8];
    }
}

public sealed record WidgetWindowIdentity(
    string WidgetId,    string SurfaceId,    WidgetKind WidgetKind,    string Name,    string LogKind,    string ShortWidgetId,    string ShortSurfaceId,    IntPtr WindowHandle,    Windows.Foundation.Rect AnimationBounds)
{
    public bool IsGroupSurface =>
        !string.Equals(SurfaceId,WidgetId,StringComparison.Ordinal);

    public string DisplayName => $"{Name}#{ShortWidgetId}";

    public string SurfaceDisplayName => $"{Name}#{ShortSurfaceId}";

    public string LogDisplayName => $"{LogKind} {DisplayName}";
}
