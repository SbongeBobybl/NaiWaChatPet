#if DESKBOX_NATIVE_AOT
namespace DeskBox.Controls;

// ElementName bindings inside FileItemSurface resolve these calculated
// presentation properties through ICustomProperty under NativeAOT. Keep the
// generated provider limited to properties consumed by the real surface.
[WinRT.GeneratedBindableCustomProperty([
    nameof(ActivityBadgeVisibility),
    nameof(ActivityStatusText),
    nameof(ActivityStatusVisibility),
    nameof(IconLayoutVisibility),
    nameof(ListLayoutVisibility),
    nameof(SurfaceHorizontalAlignment),
    nameof(SurfaceMargin),
    nameof(SurfaceMaxWidth),
    nameof(SurfacePadding),
    nameof(TransferBadgeVisibility),
    nameof(IsActivityActive),
    nameof(IsTransferActive),
    nameof(TransferStatusVisibility),
    nameof(TransferStatusText),
    nameof(PathTooltipVisibility),
    nameof(ToolTipEnabled)
], [])]
public sealed partial class FileItemSurface
{
}
#endif
