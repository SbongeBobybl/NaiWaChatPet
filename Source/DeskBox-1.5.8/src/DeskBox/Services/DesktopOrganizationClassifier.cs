using DeskBox.Models;

namespace DeskBox.Services;

public sealed class DesktopOrganizationClassifier
{
    private static readonly HashSet<string> ShortcutExtensions =
        CreateSet(".lnk",".url",".appref-ms");

    private static readonly HashSet<string> ImageExtensions =
        CreateSet(".jpg",".jpeg",".png",".gif",".bmp",".webp",".svg",".heic",".heif",".tif",".tiff",".raw",".psd");

    private static readonly HashSet<string> AudioExtensions =
        CreateSet(".mp3",".wav",".flac",".aac",".m4a",".ogg",".wma");

    private static readonly HashSet<string> VideoExtensions =
        CreateSet(".mp4",".mkv",".mov",".avi",".wmv",".webm",".m4v",".flv");

    private static readonly HashSet<string> PackageExtensions =
        CreateSet(".zip",".7z",".rar",".tar",".gz",".bz2",".xz",".iso",".exe",".msi",".msix",".appx",".appxbundle",".msixbundle");

    private static readonly Dictionary<string, string> DocumentSubtypeByExtension =
        new(StringComparer.OrdinalIgnoreCase)
        {
            [".pdf"] = DesktopOrganizationSubtypeIds.Pdf,
            [".doc"] = DesktopOrganizationSubtypeIds.Word,
            [".docx"] = DesktopOrganizationSubtypeIds.Word,
            [".rtf"] = DesktopOrganizationSubtypeIds.Word,
            [".xls"] = DesktopOrganizationSubtypeIds.Excel,
            [".xlsx"] = DesktopOrganizationSubtypeIds.Excel,
            [".csv"] = DesktopOrganizationSubtypeIds.Excel,
            [".ppt"] = DesktopOrganizationSubtypeIds.PowerPoint,
            [".pptx"] = DesktopOrganizationSubtypeIds.PowerPoint,
            [".txt"] = DesktopOrganizationSubtypeIds.Text,
            [".md"] = DesktopOrganizationSubtypeIds.Text,
            [".markdown"] = DesktopOrganizationSubtypeIds.Text,
            [".odt"] = DesktopOrganizationSubtypeIds.Word,
            [".ods"] = DesktopOrganizationSubtypeIds.Excel,
            [".odp"] = DesktopOrganizationSubtypeIds.PowerPoint
        };

    public DesktopOrganizationClassification Classify(string path)
    {
        string extension = NormalizeExtension(Path.GetExtension(path));

        if (ShortcutExtensions.Contains(extension))
        {
            return new(DesktopOrganizationCategoryIds.Shortcuts,null,extension);
        }

        if (DocumentSubtypeByExtension.TryGetValue(extension,out string? documentSubtype))
        {
            return new(DesktopOrganizationCategoryIds.Documents,documentSubtype,extension);
        }

        if (ImageExtensions.Contains(extension))
        {
            return new(DesktopOrganizationCategoryIds.Images,null,extension);
        }

        if (AudioExtensions.Contains(extension))
        {
            return new(DesktopOrganizationCategoryIds.Media,DesktopOrganizationSubtypeIds.Audio,extension);
        }

        if (VideoExtensions.Contains(extension))
        {
            return new(DesktopOrganizationCategoryIds.Media,DesktopOrganizationSubtypeIds.Video,extension);
        }

        if (PackageExtensions.Contains(extension))
        {
            return new(DesktopOrganizationCategoryIds.Packages,null,extension);
        }

        return new(DesktopOrganizationCategoryIds.Other,null,extension);
    }

    public static IReadOnlyList<string> GetCategoryExtensions(string categoryId) =>
        categoryId switch
        {
            DesktopOrganizationCategoryIds.Shortcuts => ShortcutExtensions.OrderBy(value => value).ToArray(),            DesktopOrganizationCategoryIds.Documents => DocumentSubtypeByExtension.Keys.OrderBy(value => value).ToArray(),            DesktopOrganizationCategoryIds.Images => ImageExtensions.OrderBy(value => value).ToArray(),            DesktopOrganizationCategoryIds.Media => AudioExtensions.Concat(VideoExtensions).OrderBy(value => value).ToArray(),            DesktopOrganizationCategoryIds.Packages => PackageExtensions.OrderBy(value => value).ToArray(),            _ => []
        };

    public static IReadOnlyList<string> GetSubtypeExtensions(string subtypeId) =>
        subtypeId switch
        {
            DesktopOrganizationSubtypeIds.Audio => AudioExtensions.OrderBy(value => value).ToArray(),            DesktopOrganizationSubtypeIds.Video => VideoExtensions.OrderBy(value => value).ToArray(),            _ => DocumentSubtypeByExtension
                .Where(pair => string.Equals(pair.Value,subtypeId,StringComparison.Ordinal))
                .Select(pair => pair.Key)
                .OrderBy(value => value)
                .ToArray()
        };

    public static string NormalizeExtension(string? extension)
    {
        if (string.IsNullOrWhiteSpace(extension))
        {
            return string.Empty;
        }

        string trimmed = extension.Trim();
        return (trimmed.StartsWith('.') ? trimmed : $".{trimmed}").ToLowerInvariant();
    }

    private static HashSet<string> CreateSet(params string[] extensions) =>
        new(extensions,StringComparer.OrdinalIgnoreCase);
}

public sealed record DesktopOrganizationClassification(
    string CategoryId,    string? SubtypeId,    string Extension);
