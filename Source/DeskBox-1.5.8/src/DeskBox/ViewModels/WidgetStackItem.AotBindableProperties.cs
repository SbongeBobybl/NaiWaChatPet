#if DESKBOX_NATIVE_AOT
namespace DeskBox.ViewModels;

// FileSurfaceContent uses runtime DataTemplate bindings for both the icon and
// list stack presentations. Preserve the complete stack-specific surface so
// NativeAOT cannot collapse its geometry, preview, or expanded-state visuals.
[WinRT.GeneratedBindableCustomProperty([
    nameof(AutomationState),
    nameof(ChevronGlyph),
    nameof(CollapsedPreviewVisibility),
    nameof(CountText),
    nameof(ExpandedAnchorVisibility),
    nameof(FourthPreviewVisibility),
    nameof(LabelFontSize),
    nameof(LabelMaxWidth),
    nameof(ListIconSize),
    nameof(ListMargin),
    nameof(ListPadding),
    nameof(Name),
    nameof(PreviewFour),
    nameof(PreviewItemSize),
    nameof(PreviewOne),
    nameof(PreviewSize),
    nameof(PreviewThree),
    nameof(PreviewTwo),
    nameof(Summary),
    nameof(ThirdPreviewVisibility),
    nameof(TileHeight),
    nameof(TileMargin),
    nameof(TilePadding),
    nameof(TileWidth)
], [])]
public sealed partial class WidgetStackItem
{
}
#endif
