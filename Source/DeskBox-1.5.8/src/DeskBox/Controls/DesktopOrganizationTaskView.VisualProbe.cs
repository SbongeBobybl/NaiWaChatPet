using Microsoft.UI.Xaml.Media.Imaging;
using Windows.Graphics.Imaging;
using Windows.Storage;
using Windows.Storage.Streams;

namespace DeskBox.Controls;

public sealed partial class DesktopOrganizationTaskView
{
    private void TaskRoot_SizeChanged(object sender, Microsoft.UI.Xaml.SizeChangedEventArgs e)
    {
        TaskRoot.Clip = new Microsoft.UI.Xaml.Media.RectangleGeometry
        { Rect = new Windows.Foundation.Rect(0,0,e.NewSize.Width,e.NewSize.Height) };
    }

    private async Task CapturePreviewProbeAsync(CancellationToken token)
    {
        string? path = Environment.GetEnvironmentVariable("NAIWA_PREVIEW_SCREENSHOT");
        if (string.IsNullOrWhiteSpace(path)) return;
        await Task.Delay(1500, token);
        var bitmap = new RenderTargetBitmap();
        await bitmap.RenderAsync(TaskRoot);
        var pixels = await bitmap.GetPixelsAsync();
        byte[] bytes = new byte[pixels.Length];
        using (var reader = DataReader.FromBuffer(pixels)) reader.ReadBytes(bytes);
        await File.WriteAllBytesAsync(path, [], token);
        var file = await StorageFile.GetFileFromPathAsync(path);
        using var stream = await file.OpenAsync(Windows.Storage.FileAccessMode.ReadWrite);
        var encoder = await BitmapEncoder.CreateAsync(BitmapEncoder.PngEncoderId,stream);
        encoder.SetPixelData(BitmapPixelFormat.Bgra8,BitmapAlphaMode.Premultiplied,
            (uint)bitmap.PixelWidth,(uint)bitmap.PixelHeight,96,96,bytes);
        await encoder.FlushAsync();
        App.Log($"[NaiWa] Preview image saved {bitmap.PixelWidth}x{bitmap.PixelHeight}");
    }
}
