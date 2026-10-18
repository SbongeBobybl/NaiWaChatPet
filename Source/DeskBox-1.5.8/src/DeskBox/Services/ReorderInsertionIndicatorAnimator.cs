using System.Numerics;
using Microsoft.UI.Composition;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Hosting;

namespace DeskBox.Services;

/// <summary>
/// Owns the low-cost breathing pulse used by reorder insertion indicators.
/// The animation only affects the overlay visual; it never touches the item
/// collection or the scroll container.
/// </summary>
public static class ReorderInsertionIndicatorAnimator
{
    public static void Start(UIElement element)
    {
        var visual = ElementCompositionPreview.GetElementVisual(element);
        if (visual is null)
        {
            return;
        }

        visual.StopAnimation("Opacity");
        if (!WindowsCompatibilityService.ShouldAnimate)
        {
            visual.Opacity = 1f;
            return;
        }

        Compositor compositor = visual.Compositor;
        ScalarKeyFrameAnimation animation =
            compositor.CreateScalarKeyFrameAnimation();
        animation.Duration = TimeSpan.FromMilliseconds(1450);
        animation.IterationBehavior = AnimationIterationBehavior.Forever;

        CubicBezierEasingFunction easing =
            compositor.CreateCubicBezierEasingFunction(
                new Vector2(0.42f,0.0f),                new Vector2(0.58f,1.0f));
        animation.InsertKeyFrame(0.0f,0.72f);
        animation.InsertKeyFrame(0.5f,1.0f,easing);
        animation.InsertKeyFrame(1.0f,0.72f,easing);
        visual.StartAnimation("Opacity",animation);
    }

    public static void Stop(UIElement element)
    {
        var visual = ElementCompositionPreview.GetElementVisual(element);
        visual?.StopAnimation("Opacity");
        if (visual is not null)
        {
            visual.Opacity = 1f;
        }
    }
}
