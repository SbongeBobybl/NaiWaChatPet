using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class BoundedPathChangeBufferTests
{
    [Fact]
    public void Buffer_CoalescesExistingPathsWithoutConsumingMoreCapacity()
    {
        var buffer = new BoundedPathChangeBuffer<int>(
            2,
            StringComparer.OrdinalIgnoreCase);

        Assert.Equal(
            BoundedPathChangeWriteResult.AddedOrUpdated,
            buffer.Set(@"C:\one.txt", 1));
        Assert.Equal(
            BoundedPathChangeWriteResult.AddedOrUpdated,
            buffer.Set(@"C:\two.txt", 2));
        Assert.Equal(
            BoundedPathChangeWriteResult.AddedOrUpdated,
            buffer.Set(@"c:\ONE.txt", 3));

        Assert.Equal(2, buffer.Count);
        Assert.False(buffer.IsOverflowed);
        Assert.Equal(3, buffer.Entries.Single(entry =>
            entry.Key.Equals(@"C:\one.txt", StringComparison.OrdinalIgnoreCase)).Value);
    }

    [Fact]
    public void Buffer_OverflowDiscardsPartialStateUntilReset()
    {
        var buffer = new BoundedPathChangeBuffer<int>(2);
        buffer.Set(@"C:\one.txt", 1);
        buffer.Set(@"C:\two.txt", 2);

        Assert.Equal(
            BoundedPathChangeWriteResult.Overflowed,
            buffer.Set(@"C:\three.txt", 3));
        Assert.True(buffer.IsOverflowed);
        Assert.Equal(0, buffer.Count);
        Assert.Equal(
            BoundedPathChangeWriteResult.IgnoredAfterOverflow,
            buffer.Set(@"C:\four.txt", 4));

        buffer.Reset();

        Assert.False(buffer.IsOverflowed);
        Assert.Equal(
            BoundedPathChangeWriteResult.AddedOrUpdated,
            buffer.Set(@"C:\five.txt", 5));
        Assert.Equal(1, buffer.Count);
    }
}
