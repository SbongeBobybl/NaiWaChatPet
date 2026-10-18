using DeskBox.Models;

namespace DeskBox.Tests;

public sealed class QuickLookNavigationPolicyTests
{
    [Fact]
    public void ResolveAdjacentSurface_Right_PrefersAlignedNeighbor()
    {
        QuickLookSurfaceNavigationSnapshot[] surfaces =
        [
            Surface("current", 0, 0, "a"),
            Surface("diagonal", 120, 250, "b"),
            Surface("aligned", 300, 20, "c")
        ];

        QuickLookNavigationTarget? target =
            QuickLookNavigationPolicy.ResolveAdjacentSurface(
                surfaces,
                "current",
                QuickLookNavigationDirection.Right);

        Assert.Equal(new QuickLookNavigationTarget("aligned", "c"), target);
    }

    [Fact]
    public void ResolveAdjacentSurface_Down_UsesFirstItemInTargetSurface()
    {
        QuickLookSurfaceNavigationSnapshot[] surfaces =
        [
            Surface("current", 0, 0, "a"),
            Surface("below", 10, 300, "b", "c")
        ];

        QuickLookNavigationTarget? target =
            QuickLookNavigationPolicy.ResolveAdjacentSurface(
                surfaces,
                "current",
                QuickLookNavigationDirection.Down);

        Assert.Equal(new QuickLookNavigationTarget("below", "b"), target);
    }

    [Fact]
    public void ResolveAdjacentSurface_Left_UsesLastItemInTargetSurface()
    {
        QuickLookSurfaceNavigationSnapshot[] surfaces =
        [
            Surface("left", 0, 0, "a", "b"),
            Surface("current", 300, 0, "c")
        ];

        QuickLookNavigationTarget? target =
            QuickLookNavigationPolicy.ResolveAdjacentSurface(
                surfaces,
                "current",
                QuickLookNavigationDirection.Left);

        Assert.Equal(new QuickLookNavigationTarget("left", "b"), target);
    }

    [Fact]
    public void ResolveAdjacentSurface_IgnoresEmptySurfaces()
    {
        QuickLookSurfaceNavigationSnapshot[] surfaces =
        [
            Surface("current", 0, 0, "a"),
            Surface("empty", 150, 0),
            Surface("next", 300, 0, "b")
        ];

        QuickLookNavigationTarget? target =
            QuickLookNavigationPolicy.ResolveAdjacentSurface(
                surfaces,
                "current",
                QuickLookNavigationDirection.Right);

        Assert.Equal(new QuickLookNavigationTarget("next", "b"), target);
    }

    [Fact]
    public void ResolveAdjacentSurface_AtOuterBoundary_DoesNotWrap()
    {
        QuickLookSurfaceNavigationSnapshot[] surfaces =
        [
            Surface("left", 0, 0, "a"),
            Surface("current", 300, 0, "b")
        ];

        Assert.Null(QuickLookNavigationPolicy.ResolveAdjacentSurface(
            surfaces,
            "current",
            QuickLookNavigationDirection.Right));
    }

    private static QuickLookSurfaceNavigationSnapshot Surface(
        string id,
        double x,
        double y,
        params string[] paths) =>
        new(id, x, y, 200, 200, paths);
}
