using System.Reflection;
using System.Text;
using DeskBox.Helpers;

namespace DeskBox.Tests;

public sealed class ShellClipboardHelperTests
{
    [Fact]
    public void CreateDropFilesPayload_PreservesEverySelectedPath()
    {
        string[] paths =
        [
            @"C:\Users\Test\Desktop\first.txt",
            @"C:\Users\Test\Desktop\second folder",
            @"D:\Archive\third.png"
        ];
        MethodInfo method = typeof(ShellClipboardHelper).GetMethod(
            "CreateDropFilesPayload",
            BindingFlags.NonPublic | BindingFlags.Static) ??
            throw new InvalidOperationException(
                "Shell clipboard payload builder was not found.");

        byte[] payload = Assert.IsType<byte[]>(method.Invoke(
            null,
            [paths]));

        Assert.Equal(20, BitConverter.ToInt32(payload, 0));
        Assert.Equal(1, BitConverter.ToInt32(payload, 16));
        Assert.Equal(
            string.Join('\0', paths) + "\0\0",
            Encoding.Unicode.GetString(payload, 20, payload.Length - 20));
    }

    [Fact]
    public void ClipboardFileDropDetection_DoesNotThrow()
    {
        Exception? exception = Record.Exception(
            () => _ = ShellClipboardHelper.HasFileDropList());

        Assert.Null(exception);
    }
}
