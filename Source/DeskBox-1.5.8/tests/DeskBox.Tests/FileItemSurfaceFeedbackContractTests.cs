using System.Xml.Linq;

namespace DeskBox.Tests;

public sealed class FileItemSurfaceFeedbackContractTests
{
    [Theory]
    [InlineData("SurfaceFileIconTemplate")]
    [InlineData("SurfaceFileListTemplate")]
    [InlineData("StackPopoverFileIconTemplate")]
    [InlineData("StackPopoverFileListTemplate")]
    public void Templates_WireFeedbackBeforeLoadedAndKeepItAcrossReuse(string templateKey)
    {
        XDocument document = XDocument.Load(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.xaml"));
        XNamespace x = "http://schemas.microsoft.com/winfx/2006/xaml";
        XNamespace controls = "using:DeskBox.Controls";
        XElement template = document.Descendants().Single(element =>
            (string?)element.Attribute(x + "Key") == templateKey);
        XElement surface = template.Descendants(controls + "FileItemSurface").Single();

        Assert.Equal("ItemSurface_VisualStateChanged", (string?)surface.Attribute("VisualStateChanged"));
        Assert.Equal("ItemSurface_DataContextChanged", (string?)surface.Attribute("DataContextChanged"));
    }

    [Fact]
    public void Surface_ProvidesAHitTestBackgroundBeforeFirstPaint()
    {
        XDocument document = XDocument.Load(TestPaths.FromRepository(
            "src/DeskBox/Controls/FileItemSurface.xaml"));
        XNamespace x = "http://schemas.microsoft.com/winfx/2006/xaml";
        XElement surface = document.Descendants().Single(element =>
            (string?)element.Attribute(x + "Name") == "SurfaceBorder");

        Assert.Equal("Transparent", (string?)surface.Attribute("Background"));
    }

    [Fact]
    public void LoadedAndUnloaded_DoNotOwnFeedbackSubscriptionsOrOverwriteHover()
    {
        string source = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.ItemVisuals.cs"));
        int start = source.IndexOf("private void ItemSurface_Loaded(", StringComparison.Ordinal);
        int end = source.IndexOf("private void ItemSurface_VisualStateChanged(", StringComparison.Ordinal);
        string lifecycle = source[start..end];

        Assert.DoesNotContain("VisualStateChanged +=", lifecycle, StringComparison.Ordinal);
        Assert.DoesNotContain("VisualStateChanged -=", lifecycle, StringComparison.Ordinal);
        Assert.DoesNotContain("DataContextChanged +=", lifecycle, StringComparison.Ordinal);
        Assert.DoesNotContain("DataContextChanged -=", lifecycle, StringComparison.Ordinal);
        Assert.Contains("FileItemSurface.FindOwner(border)?.VisualState", lifecycle, StringComparison.Ordinal);
    }

    [Fact]
    public void MovementRecovery_OnlyUpdatesTheLocalVisualAndDoesNotConsumeInput()
    {
        string source = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Controls/FileItemSurface.xaml.cs"));
        Assert.Contains("UIElement.PointerMovedEvent", source, StringComparison.Ordinal);
        Assert.Contains("new PointerEventHandler(SurfaceBorder_PointerMoved)", source, StringComparison.Ordinal);
        Assert.Contains("handledEventsToo: true", source, StringComparison.Ordinal);
        int start = source.IndexOf("private void SurfaceBorder_PointerMoved(", StringComparison.Ordinal);
        int end = source.IndexOf("private void SurfaceBorder_PointerPressed(", StringComparison.Ordinal);
        string moved = source[start..end];

        Assert.Contains("_pointerFeedback.OnPointerMoved(", moved, StringComparison.Ordinal);
        Assert.Contains("point.IsInContact", moved, StringComparison.Ordinal);
        Assert.Contains("SurfaceBorder.ActualWidth", moved, StringComparison.Ordinal);
        Assert.Contains("SurfaceBorder.ActualHeight", moved, StringComparison.Ordinal);
        Assert.DoesNotContain("e.Handled", moved, StringComparison.Ordinal);
        Assert.DoesNotContain("CapturePointer", moved, StringComparison.Ordinal);
        Assert.DoesNotContain("SelectedItems", moved, StringComparison.Ordinal);
    }
}
