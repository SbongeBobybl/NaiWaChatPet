using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class VirtualDropFileNameResolverTests : IDisposable
{
    private readonly string _tempRoot = Path.Combine(
        Path.GetTempPath(),
        "DeskBox.Tests",
        Guid.NewGuid().ToString("N"));

    public VirtualDropFileNameResolverTests()
    {
        Directory.CreateDirectory(_tempRoot);
    }

    [Theory]
    [MemberData(nameof(ImageHeaders))]
    public void TryGetExtension_RecognizesCommonVirtualImagePayloads(
        byte[] header,
        string expectedExtension)
    {
        Assert.Equal(
            expectedExtension,
            VirtualDropFileNameResolver.TryGetExtension(header));
    }

    [Fact]
    public void AddMissingExtensionFromContent_RenamesWebpVirtualFile()
    {
        string sourcePath = Path.Combine(_tempRoot, "browser-image");
        File.WriteAllBytes(sourcePath,
        [
            0x52, 0x49, 0x46, 0x46, 0x24, 0x00, 0x00, 0x00,
            0x57, 0x45, 0x42, 0x50, 0x56, 0x50, 0x38, 0x20
        ]);

        string result = VirtualDropFileNameResolver.AddMissingExtensionFromContent(sourcePath);

        Assert.Equal(Path.Combine(_tempRoot, "browser-image.webp"), result);
        Assert.False(File.Exists(sourcePath));
        Assert.True(File.Exists(result));
    }

    [Fact]
    public void AddMissingExtensionFromContent_PreservesExistingExtension()
    {
        string sourcePath = Path.Combine(_tempRoot, "browser-image.bin");
        File.WriteAllBytes(sourcePath, [0x52, 0x49, 0x46, 0x46]);

        string result = VirtualDropFileNameResolver.AddMissingExtensionFromContent(sourcePath);

        Assert.Equal(sourcePath, result);
        Assert.True(File.Exists(sourcePath));
    }

    public static IEnumerable<object[]> ImageHeaders =>
    [
        [
            new byte[]
            {
                0x52, 0x49, 0x46, 0x46, 0x24, 0x00, 0x00, 0x00,
                0x57, 0x45, 0x42, 0x50
            },
            ".webp"
        ],
        [
            new byte[] { 0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A },
            ".png"
        ],
        [new byte[] { 0xFF, 0xD8, 0xFF, 0xE0 }, ".jpg"],
        [new byte[] { 0x47, 0x49, 0x46, 0x38, 0x39, 0x61 }, ".gif"],
        [
            new byte[]
            {
                0x00, 0x00, 0x00, 0x1C, 0x66, 0x74, 0x79, 0x70,
                0x61, 0x76, 0x69, 0x66
            },
            ".avif"
        ]
    ];

    public void Dispose()
    {
        try
        {
            if (Directory.Exists(_tempRoot))
            {
                Directory.Delete(_tempRoot, recursive: true);
            }
        }
        catch
        {
        }
    }
}
