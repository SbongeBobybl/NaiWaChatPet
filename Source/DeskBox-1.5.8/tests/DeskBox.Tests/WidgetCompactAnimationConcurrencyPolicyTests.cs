using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WidgetCompactAnimationConcurrencyPolicyTests
{
    [Theory]
    [InlineData(0, 2, true)]
    [InlineData(1, 2, true)]
    [InlineData(2, 2, false)]
    [InlineData(3, 2, false)]
    [InlineData(0, 0, false)]
    [InlineData(-1, 2, false)]
    public void ShouldAnimate_EnforcesConcurrentBoundsTransitionBudget(
        int activeTransitions,
        int maximumConcurrentTransitions,
        bool expected)
    {
        Assert.Equal(
            expected,
            WidgetCompactAnimationConcurrencyPolicy.ShouldAnimate(
                activeTransitions,
                maximumConcurrentTransitions));
    }
}
