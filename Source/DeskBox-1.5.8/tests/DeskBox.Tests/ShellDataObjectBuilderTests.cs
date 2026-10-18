using System.Runtime.InteropServices;
using System.Text;
using DeskBox.Helpers;

namespace DeskBox.Tests;

public sealed class ShellDataObjectBuilderTests
{
    // DROPFILES: DWORD pFiles; POINT pt; BOOL fNC; BOOL fWide.
    private const int DropFilesHeaderSize = 20;
    private const int WideCharactersOffset = 16;

    [Fact]
    public void SingleFile_ProducesWideDoubleNullTerminatedHdrop()
    {
        byte[] payload = ShellDataObjectBuilder.BuildHdropBytes(
            [@"C:\Tools\image.png"]);

        Assert.Equal(DropFilesHeaderSize, Marshal.ReadInt32(payload, 0)); // pFiles
        Assert.Equal(1, Marshal.ReadInt32(payload, WideCharactersOffset)); // fWide

        Assert.Equal(
            "C:\\Tools\\image.png\0\0",
            DecodePathList(payload));
    }

    [Fact]
    public void DirectoryPathsWithTrailingBackslash_AreKeptVerbatim()
    {
        // HDROP is a raw path list, not a command line: the classic quoting
        // hazard of argument construction does not exist here, and a trailing
        // backslash must survive byte for byte.
        byte[] payload = ShellDataObjectBuilder.BuildHdropBytes(
            [@"C:\Screenshots\Folder\"]);

        Assert.Equal(
            @"C:\Screenshots\Folder\" + "\0\0",
            DecodePathList(payload));
    }

    [Fact]
    public void MultipleFiles_AllCarriedInOrderWithListTerminator()
    {
        byte[] payload = ShellDataObjectBuilder.BuildHdropBytes(
        [
            @"C:\a.png",
            @"C:\dir with spaces\b.jpg",
            @"C:\c.pdf"
        ]);

        string[] entries = DecodePathList(payload)
            .Split('\0', StringSplitOptions.RemoveEmptyEntries);
        Assert.Equal(
            new[] { @"C:\a.png", @"C:\dir with spaces\b.jpg", @"C:\c.pdf" },
            entries);
        // The final wide NUL (list terminator) must be present after the last
        // path terminator.
        Assert.Equal(0, payload[^1]);
        Assert.Equal(0, payload[^2]);
        Assert.Equal(0, payload[^3]);
        Assert.Equal(0, payload[^4]);
    }

    [Fact]
    public void DuplicatePaths_AreCollapsedOrdinalIgnoreCase()
    {
        byte[] payload = ShellDataObjectBuilder.BuildHdropBytes(
        [
            @"C:\a.png",
            @"c:\A.PNG"
        ]);

        string[] entries = DecodePathList(payload)
            .Split('\0', StringSplitOptions.RemoveEmptyEntries);
        Assert.Single(entries);
    }

    [Fact]
    public void EmptyAndWhitespaceEntries_AreDropped()
    {
        byte[] payload = ShellDataObjectBuilder.BuildHdropBytes(
            [string.Empty, "   ", @"C:\a.png"]);

        Assert.Equal("C:\\a.png\0\0", DecodePathList(payload));
    }

    [Fact]
    public void EmptyList_ProducesHeaderPlusListTerminator()
    {
        // An empty list still carries its double-NUL terminator; the builder's
        // public entry point rejects empty inputs before reaching this.
        byte[] payload = ShellDataObjectBuilder.BuildHdropBytes([]);

        Assert.Equal(DropFilesHeaderSize + sizeof(char), payload.Length);
        Assert.Equal(DropFilesHeaderSize, Marshal.ReadInt32(payload, 0));
    }

    private static string DecodePathList(byte[] payload) =>
        Encoding.Unicode.GetString(
            payload,
            DropFilesHeaderSize,
            payload.Length - DropFilesHeaderSize);
}
