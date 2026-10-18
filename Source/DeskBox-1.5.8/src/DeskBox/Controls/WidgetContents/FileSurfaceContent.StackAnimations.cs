using System.Numerics;
using DeskBox.Services;
using DeskBox.ViewModels;
using Microsoft.UI.Composition;
using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using Microsoft.UI.Xaml.Hosting;

namespace DeskBox.Controls.WidgetContents;

public sealed partial class FileSurfaceContent
{
    private const int StackCollapseDurationMs = 130;
    private readonly HashSet<FrameworkElement> _animatedStackElements = [];
    private int _stackTransitionGeneration;
    private CancellationTokenSource? _stackTransitionCancellation;
    private string? _pendingStackTransitionKey;
    private bool? _pendingStackExpanded;

    private void RequestStackState(
        WidgetStackItem stack,        bool expanded)
    {
        var cancellation = new CancellationTokenSource();
        CancelStackTransition(
            Interlocked.Exchange(
                ref _stackTransitionCancellation,                cancellation));
        _ = ObserveStackTransitionAsync(stack,expanded,cancellation);
    }

    private static void CancelStackTransition(
        CancellationTokenSource? cancellation)
    {
        try
        {
            cancellation?.Cancel();
        }
        catch (ObjectDisposedException)
        {
            // The previous transition completed while a new input event was
            // being routed.
        }
    }

    private async Task ObserveStackTransitionAsync(
        WidgetStackItem stack,        bool expanded,        CancellationTokenSource cancellation)
    {
        try
        {
            await RunStackTransitionAsync(
                stack,                expanded,                cancellation.Token);
        }
        catch (OperationCanceledException) when (
            cancellation.IsCancellationRequested)
        {
            // A newer stack input or a lifecycle reset owns the transition.
        }
        catch (Exception ex)
        {
            bool isCurrentRequest = string.Equals(
                    _pendingStackTransitionKey,                    stack.StackKey,                    StringComparison.Ordinal) &&
                _pendingStackExpanded == expanded;
            App.Log(
                $"[FileStack] Transition failed widget={WidgetId} " +
                $"stack={stack.StackKey}: {ex}");
            if (!isCurrentRequest)
            {
                return;
            }

            StopAndRestoreStackAnimations();
            _pendingStackTransitionKey = null;
            _pendingStackExpanded = null;
            if (!_isDisposed)
            {
                ApplyStackProjectionChange(() =>
                    ViewModel.SetStackExpanded(stack,expanded));
            }
        }
        finally
        {
            Interlocked.CompareExchange(
                ref _stackTransitionCancellation,                null,                cancellation);
            cancellation.Dispose();
        }
    }

    private bool GetDesiredStackState(WidgetStackItem stack) =>
        string.Equals(
            _pendingStackTransitionKey,            stack.StackKey,            StringComparison.Ordinal) &&
        _pendingStackExpanded is { } pending
            ? pending
            : stack.IsExpanded;

    private async Task RunStackTransitionAsync(
        WidgetStackItem stack,        bool expanded,        CancellationToken cancellationToken)
    {
        int generation = ++_stackTransitionGeneration;
        _pendingStackTransitionKey = stack.StackKey;
        _pendingStackExpanded = expanded;
        StopAndRestoreStackAnimations();

        bool animate = WindowsCompatibilityService.AreAnimationsEnabled &&
            IsLoaded &&
            XamlRoot is not null;
        bool playedExitAnimation = false;
        WidgetStackItem? expandedStack = ViewModel.GetExpandedStack();
        if (animate &&
            expanded &&
            expandedStack is not null &&
            !string.Equals(
                expandedStack.StackKey,                stack.StackKey,                StringComparison.Ordinal))
        {
            StartStackMemberExitAnimations(expandedStack);
            playedExitAnimation = true;
            await Task.Delay(
                StackCollapseDurationMs,                cancellationToken);
            if (generation != _stackTransitionGeneration)
            {
                return;
            }
        }
        else if (animate && stack.IsExpanded && !expanded)
        {
            StartStackMemberExitAnimations(stack);
            playedExitAnimation = true;
            await Task.Delay(
                StackCollapseDurationMs,                cancellationToken);
            if (generation != _stackTransitionGeneration)
            {
                return;
            }
        }

        if (stack.IsExpanded != expanded ||
            expanded &&
            expandedStack is not null &&
            !string.Equals(
                expandedStack.StackKey,                stack.StackKey,                StringComparison.Ordinal))
        {
            if (playedExitAnimation)
            {
                // Restore the recycled element before removing it from the
                // projection. Otherwise WinUI can reuse an opacity-zero visual
                // for an unrelated file on the next layout pass.
                StopAndRestoreStackAnimations();
            }
            ApplyStackProjectionChange(() =>
                ViewModel.SetStackExpanded(stack,expanded));
        }

        if (generation == _stackTransitionGeneration)
        {
            _pendingStackTransitionKey = null;
            _pendingStackExpanded = null;
            _animatedStackElements.Clear();
        }
    }

    private void StartStackMemberExitAnimations(
        WidgetStackItem stack)
    {
        Border? anchor = FindStackSurface(stack.StackKey);
        foreach (FrameworkElement element in
                 GetRealizedStackMemberElements(stack))
        {
            StartStackElementAnimation(
                element,                fromOpacity: 1,                toOpacity: 0,                fromScale: 1,                toScale: 0.96f,                fromTranslation: Vector3.Zero,                toTranslation: GetStackTransitionTranslation(
                    anchor,                    element),                durationMs: StackCollapseDurationMs,                delayMs: 0);
        }
    }

    private FrameworkElement[] GetRealizedStackMemberElements(
        WidgetStackItem stack)
    {
        return stack.Members
            .Select(FindItemSurface)
            .OfType<FrameworkElement>()
            .Where(element => element.XamlRoot is not null)
            .ToArray();
    }

    private Vector3 GetStackTransitionTranslation(
        FrameworkElement? anchor,        FrameworkElement element)
    {
        if (anchor is null ||
            anchor.XamlRoot is null ||
            element.XamlRoot is null)
        {
            return new Vector3(0,-8,0);
        }

        try
        {
            Windows.Foundation.Point anchorCenter = anchor
                .TransformToVisual(Root)
                .TransformPoint(new(
                    anchor.ActualWidth / 2,                    anchor.ActualHeight / 2));
            Windows.Foundation.Point elementCenter = element
                .TransformToVisual(Root)
                .TransformPoint(new(
                    element.ActualWidth / 2,                    element.ActualHeight / 2));
            return new Vector3(
                (float)Math.Clamp(
                    (anchorCenter.X - elementCenter.X) * 0.18,                    -18,                    18),                (float)Math.Clamp(
                    (anchorCenter.Y - elementCenter.Y) * 0.18,                    -18,                    18),                0);
        }
        catch (InvalidOperationException)
        {
            return new Vector3(0,-8,0);
        }
    }

    private void StartStackElementAnimation(
        FrameworkElement element,        float fromOpacity,        float toOpacity,        float fromScale,        float toScale,        Vector3 fromTranslation,        Vector3 toTranslation,        int durationMs,        int delayMs)
    {
        Visual visual = ElementCompositionPreview.GetElementVisual(element);
        Compositor compositor = visual.Compositor;
        visual.CenterPoint = new Vector3(
            (float)(element.ActualWidth / 2),            (float)(element.ActualHeight / 2),            0);
        visual.Opacity = fromOpacity;
        visual.Scale = new Vector3(fromScale,fromScale,1);
        ElementCompositionPreview.SetIsTranslationEnabled(element,true);
        visual.Properties.InsertVector3("Translation",fromTranslation);

        CubicBezierEasingFunction easing =
            compositor.CreateCubicBezierEasingFunction(
                new Vector2(0.16f,0.84f),                new Vector2(0.44f,1));
        TimeSpan duration = TimeSpan.FromMilliseconds(durationMs);
        TimeSpan delay = TimeSpan.FromMilliseconds(delayMs);

        ScalarKeyFrameAnimation opacity =
            compositor.CreateScalarKeyFrameAnimation();
        opacity.Duration = duration;
        opacity.DelayTime = delay;
        opacity.DelayBehavior = AnimationDelayBehavior.SetInitialValueBeforeDelay;
        opacity.InsertKeyFrame(1,toOpacity,easing);

        Vector3KeyFrameAnimation scale =
            compositor.CreateVector3KeyFrameAnimation();
        scale.Duration = duration;
        scale.DelayTime = delay;
        scale.DelayBehavior = AnimationDelayBehavior.SetInitialValueBeforeDelay;
        scale.InsertKeyFrame(
            1,            new Vector3(toScale,toScale,1),            easing);

        Vector3KeyFrameAnimation translation =
            compositor.CreateVector3KeyFrameAnimation();
        translation.Duration = duration;
        translation.DelayTime = delay;
        translation.DelayBehavior =
            AnimationDelayBehavior.SetInitialValueBeforeDelay;
        translation.InsertKeyFrame(1,toTranslation,easing);

        visual.StartAnimation(nameof(Visual.Opacity),opacity);
        visual.StartAnimation(nameof(Visual.Scale),scale);
        visual.Properties.StartAnimation("Translation",translation);
        _animatedStackElements.Add(element);
    }

    private void StopAndRestoreStackAnimations()
    {
        foreach (FrameworkElement element in
                 _animatedStackElements.ToArray())
        {
            try
            {
                RestoreStackAnimationElement(element);
            }
            catch (Exception)
            {
                // The element can be torn down between a window visibility
                // change and this cleanup pass. A newly realized container is
                // initialized independently by the item template.
            }
        }

        _animatedStackElements.Clear();
    }

    private static void RestoreStackAnimationElement(
        FrameworkElement element)
    {
        Visual visual =
            ElementCompositionPreview.GetElementVisual(element);
        visual.StopAnimation(nameof(Visual.Opacity));
        visual.StopAnimation(nameof(Visual.Scale));
        try
        {
            visual.Properties.StopAnimation("Translation");
        }
        catch (ArgumentException)
        {
            // Freshly realized containers do not have the optional
            // Translation property until their first stack animation.
        }
        visual.Opacity = 1;
        visual.Scale = Vector3.One;
        visual.Properties.InsertVector3(
            "Translation",            Vector3.Zero);
    }
}
