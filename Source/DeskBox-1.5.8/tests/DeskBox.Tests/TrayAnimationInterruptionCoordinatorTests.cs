using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class TrayAnimationInterruptionCoordinatorTests
{
    [Fact]
    public void CancelAndRestore_continuesAcrossWindowsWhenOneHostFails()
    {
        int[] windows = [1, 2, 3, 4];
        var restored = new List<int>();
        var failed = new List<int>();

        int count = TrayAnimationInterruptionCoordinator.CancelAndRestore(
            windows,
            window =>
            {
                if (window == 2)
                {
                    throw new InvalidOperationException("simulated host failure");
                }

                restored.Add(window);
            },
            (window, _) => failed.Add(window));

        Assert.Equal(3, count);
        Assert.Equal([1, 3, 4], restored);
        Assert.Equal([2], failed);
    }
}
