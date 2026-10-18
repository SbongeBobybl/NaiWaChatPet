using DeskBox.Services;
using Microsoft.UI.Xaml.Media;
using Microsoft.UI.Xaml.Media.Imaging;

namespace DeskBox.Helpers;

/// <summary>
/// Builds the XAML image source for a per-widget custom background. Decodes
/// are bounded so a full-resolution photo never enters the decoded-bitmap
/// budget: 1280 physical pixels covers a maximally sized widget at 2x DPI
/// while keeping each background at a few MB regardless of source size.
/// </summary>
public static class WidgetBackgroundImageSourceFactory
{
    public const int MaxDecodedBackgroundPixels = 1280;

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

            string extension = Path.GetExtension(filePath).ToLowerInvariant();
            if (!WidgetTitleIconCustomization.IsSupportedImageExtension(extension))
            {
                return null;
            }

            if (extension == ".svg")
            {
                return new SvgImageSource(new Uri(filePath))
                {
                    RasterizePixelWidth = MaxDecodedBackgroundPixels
                };
            }

            var bitmap = new BitmapImage
            {
                DecodePixelType = DecodePixelType.Physical,
                DecodePixelWidth = MaxDecodedBackgroundPixels
            };
            bitmap.UriSource = new Uri(filePath);
            return bitmap;
        }
        catch (Exception ex)
        {
            App.Log($"[WidgetBackground] Failed to create image source for '{filePath}': {ex.Message}");
            return null;
        }
    }
}
