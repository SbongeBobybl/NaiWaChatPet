using DeskBox.Services;
using Microsoft.UI.Xaml.Media;
using Microsoft.UI.Xaml.Media.Imaging;

namespace DeskBox.Helpers;

/// <summary>
/// Builds the XAML image source for a custom widget title icon. Raster
/// formats decode bounded to <see cref="WidgetTitleIconCustomization.MaxDecodedIconPixels"/>
/// so a huge source photo never lands in the decoded-bitmap budget; SVG
/// rasterizes at the same bound.
/// </summary>
public static class WidgetTitleIconImageSourceFactory
{
    public static ImageSource? TryCreate(string? filePath)
    {
        if (string.IsNullOrWhiteSpace(filePath))
        {
            return null;
        }

        try
        {
            if (!File.Exists(filePath))
            {
                return null;
            }

            if (string.Equals(
                    Path.GetExtension(filePath),                    ".svg",                    StringComparison.OrdinalIgnoreCase))
            {
                return new SvgImageSource(new Uri(filePath))
                {
                    RasterizePixelWidth = WidgetTitleIconCustomization.MaxDecodedIconPixels
                };
            }

            var bitmap = new BitmapImage
            {
                DecodePixelType = DecodePixelType.Physical,
                DecodePixelWidth = WidgetTitleIconCustomization.MaxDecodedIconPixels
            };
            bitmap.UriSource = new Uri(filePath);
            return bitmap;
        }
        catch (Exception ex)
        {
            App.Log($"[TitleIconAsset] Failed to create image source for '{filePath}': {ex.Message}");
            return null;
        }
    }
}
