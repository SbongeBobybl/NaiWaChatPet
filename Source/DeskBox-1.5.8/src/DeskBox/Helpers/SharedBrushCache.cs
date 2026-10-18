using Windows.UI;
using Microsoft.UI.Xaml.Media;

namespace DeskBox.Helpers;

/// <summary>
/// Process-wide cache of solid color brushes for hot paths that re-apply
/// appearance (theme, accent, compact overlay, drop-preview states) many
/// times per interaction. <see cref="SolidColorBrush"/> instances can be
/// shared across elements, so callers get one instance per distinct color
/// instead of allocating a new wrapper on every apply. Brush colors are
/// never mutated through this cache; re-apply resolves a new color and
/// looks it up again.
/// </summary>
internal static class SharedBrushCache
{
    // Distinct colors are bounded by theme/accent palettes in practice; the
    // cap only guards pathological growth (e.g. per-frame alpha ramps) by
    // resetting wholesale.
    private const int Capacity = 512;

    private static readonly object s_gate = new();
    private static readonly Dictionary<Color, SolidColorBrush> s_brushes = new();

    public static SolidColorBrush GetOrCreate(Color color)
    {
        lock (s_gate)
        {
            if (s_brushes.TryGetValue(color,out SolidColorBrush? cached))
            {
                return cached;
            }

            if (s_brushes.Count >= Capacity)
            {
                s_brushes.Clear();
            }

            var brush = new SolidColorBrush(color);
            s_brushes[color] = brush;
            return brush;
        }
    }
}
