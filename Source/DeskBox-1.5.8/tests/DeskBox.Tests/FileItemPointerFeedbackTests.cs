using DeskBox.Controls;

namespace DeskBox.Tests;

public sealed class FileItemPointerFeedbackTests
{
    [Theory]
    [InlineData(false)]
    [InlineData(true)]
    public void SuccessfulOpen_ClearsPressedLayerEvenWithoutPointerRelease(bool isDark)
    {
        var feedback = new FileItemPointerFeedback();
        feedback.OnPointerEntered();
        FileItemSurfaceVisualState state = feedback.OnPointerPressed();

        // Reproduce the reported gap: removing selection alone still paints
        // an opaque pressed layer after Shell takes focus from the widget.
        Assert.NotEqual((byte)0,
            FileItemSurfaceStyleCache.GetNeutralStateLayer(isDark, state, isSelected: false).A);

        state = feedback.OnOpenDispatched();

        Assert.Equal(FileItemSurfaceVisualState.Normal, state);
        Assert.Equal((byte)0,
            FileItemSurfaceStyleCache.GetNeutralStateLayer(isDark, state, isSelected: false).A);
    }

    [Theory]
    [InlineData(false)]
    [InlineData(true)]
    public void SuccessfulOpen_ClearsHoverAfterPointerRelease(bool isDark)
    {
        var feedback = new FileItemPointerFeedback();
        feedback.OnPointerPressed();
        Assert.Equal(FileItemSurfaceVisualState.Hover, feedback.OnPointerReleased(inside: true));

        FileItemSurfaceVisualState state = feedback.OnOpenDispatched();

        Assert.Equal((byte)0,
            FileItemSurfaceStyleCache.GetNeutralStateLayer(isDark, state, isSelected: false).A);
    }

    [Theory]
    [InlineData(false)]
    [InlineData(true)]
    public void LatePointerRelease_DoesNotRestoreHighlight(bool inside)
    {
        var feedback = new FileItemPointerFeedback();
        feedback.OnPointerPressed();
        feedback.OnOpenDispatched();

        Assert.Equal(FileItemSurfaceVisualState.Normal, feedback.OnPointerReleased(inside));
        Assert.Equal(FileItemSurfaceVisualState.Normal, feedback.OnPointerReleased(inside));
    }

    [Fact]
    public void NewPointerEntry_RestoresNormalHoverFeedback()
    {
        var feedback = new FileItemPointerFeedback();
        feedback.OnOpenDispatched();

        Assert.Equal(FileItemSurfaceVisualState.Hover, feedback.OnPointerEntered());
        Assert.Equal(FileItemSurfaceVisualState.Hover, feedback.OnPointerReleased(inside: true));
    }

    [Fact]
    public void NewPointerPress_WithoutLeavingItem_RestoresNormalClickFeedback()
    {
        var feedback = new FileItemPointerFeedback();
        feedback.OnOpenDispatched();

        Assert.Equal(FileItemSurfaceVisualState.Pressed, feedback.OnPointerPressed());
        Assert.Equal(FileItemSurfaceVisualState.Hover, feedback.OnPointerReleased(inside: true));
    }

    [Fact]
    public void ResetForReuse_DoesNotSuppressFeedbackForNextItem()
    {
        var feedback = new FileItemPointerFeedback();
        feedback.OnOpenDispatched();
        feedback.ResetForReuse();

        Assert.Equal(FileItemSurfaceVisualState.Hover, feedback.OnPointerReleased(inside: true));
    }

    [Fact]
    public void OrdinaryClick_WithoutSuccessfulOpen_KeepsItsFeedback()
    {
        var feedback = new FileItemPointerFeedback();

        Assert.Equal(FileItemSurfaceVisualState.Hover, feedback.OnPointerEntered());
        Assert.Equal(FileItemSurfaceVisualState.Pressed, feedback.OnPointerPressed());
        Assert.Equal(FileItemSurfaceVisualState.Hover, feedback.OnPointerReleased(inside: true));
        Assert.Equal(FileItemSurfaceVisualState.Normal, feedback.OnPointerReleased(inside: false));
    }

    [Fact]
    public void SuccessfulOpen_DoesNotChangeAnotherItemsPointerFeedback()
    {
        var openedItem = new FileItemPointerFeedback();
        var otherItem = new FileItemPointerFeedback();
        openedItem.OnPointerPressed();
        otherItem.OnPointerPressed();

        Assert.Equal(FileItemSurfaceVisualState.Normal, openedItem.OnOpenDispatched());
        Assert.Equal(FileItemSurfaceVisualState.Hover, otherItem.OnPointerReleased(inside: true));
    }

    [Theory]
    [InlineData(false)]
    [InlineData(true)]
    public void PointerMovement_WithoutEnteredOrClick_RestoresHover(bool recycled)
    {
        var feedback = new FileItemPointerFeedback();
        if (recycled)
        {
            feedback.OnOpenDispatched();
            feedback.ResetForReuse();
        }

        Assert.Equal(FileItemSurfaceVisualState.Hover,
            feedback.OnPointerMoved(FileItemSurfaceVisualState.Normal, isInContact: false, x: 10, y: 10));
    }

    [Fact]
    public void MovementAfterOpen_RearmsHoverButLateReleaseAloneDoesNot()
    {
        var feedback = new FileItemPointerFeedback();
        feedback.RecordPointerPosition(10, 10);
        FileItemSurfaceVisualState state = feedback.OnOpenDispatched();
        Assert.Equal(FileItemSurfaceVisualState.Normal, feedback.OnPointerReleased(inside: true));

        state = feedback.OnPointerMoved(state, isInContact: false, x: 20, y: 10);

        Assert.Equal(FileItemSurfaceVisualState.Hover, state);
        Assert.Equal(FileItemSurfaceVisualState.Hover, feedback.OnPointerReleased(inside: true));
    }

    [Theory]
    [InlineData(FileItemSurfaceVisualState.Normal)]
    [InlineData(FileItemSurfaceVisualState.Pressed)]
    [InlineData(FileItemSurfaceVisualState.DropTarget)]
    public void MovementInContact_DoesNotChangeDragOrPressedFeedback(FileItemSurfaceVisualState state)
    {
        var feedback = new FileItemPointerFeedback();
        feedback.OnOpenDispatched();

        Assert.Equal(state, feedback.OnPointerMoved(state, isInContact: true, x: 20, y: 10));
        Assert.Equal(FileItemSurfaceVisualState.Normal, feedback.OnPointerReleased(inside: true));
    }

    [Fact]
    public void StationaryPointerSampleAfterOpen_DoesNotRestoreHighlight()
    {
        var feedback = new FileItemPointerFeedback();
        feedback.RecordPointerPosition(10, 10);
        FileItemSurfaceVisualState state = feedback.OnOpenDispatched();

        Assert.Equal(FileItemSurfaceVisualState.Normal,
            feedback.OnPointerMoved(state, isInContact: false, x: 10, y: 10));
        Assert.Equal(FileItemSurfaceVisualState.Normal, feedback.OnPointerReleased(inside: true));
    }
}
