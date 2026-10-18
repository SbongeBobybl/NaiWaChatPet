using Windows.Graphics;

namespace DeskBox.Services;

internal static class WidgetGroupDropHitTestPolicy
{
    public static bool Contains(RectInt32? bounds,int screenX,int screenY)
    {
        if (bounds is not { Width: > 0, Height: > 0 } value)
        {
            return false;
        }

        long right = (long)value.X + value.Width;
        long bottom = (long)value.Y + value.Height;
        return screenX >= value.X &&
               screenX < right &&
               screenY >= value.Y &&
               screenY < bottom;
    }
}
