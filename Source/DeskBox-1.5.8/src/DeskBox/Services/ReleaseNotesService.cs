using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using DeskBox.Models;

namespace DeskBox.Services;

public sealed record ReleaseNotesLoadResult(
    string Content,    bool IsFromCache = false,    string? ErrorMessage = null)
{
    public bool HasContent => !string.IsNullOrWhiteSpace(Content);
}

/// <summary>
/// Resolves release notes from the manifest first, then from a safe HTTPS
/// endpoint, and keeps a small version/locale keyed cache for offline viewing.
/// </summary>
public sealed class ReleaseNotesService
{
    public const int MaxCharacters = SimpleMarkdownRenderer.MaxCharacters;
    private readonly HttpClient _httpClient;
    private readonly string _cacheRootPath;

    public ReleaseNotesService(string? cacheRootPath = null,HttpClient? httpClient = null)
    {
        _cacheRootPath = string.IsNullOrWhiteSpace(cacheRootPath)
            ? Path.Combine(DeskBoxDataPathService.Current.UpdatesDirectory,"release-notes")
            : cacheRootPath;
        _httpClient = httpClient ?? CreateHttpClient();
    }

    /// <summary>
    /// Loads the complete release log as one document. A manifest may still
    /// contain locale-keyed entries for backwards compatibility; when this
    /// overload is used, all distinct non-empty entries are shown together.
    /// </summary>
    public Task<ReleaseNotesLoadResult> LoadAsync(
        AppUpdateManifest manifest,        CancellationToken cancellationToken = default) =>
        LoadAsync(manifest,locale: string.Empty,cancellationToken);

    public async Task<ReleaseNotesLoadResult> LoadAsync(
        AppUpdateManifest manifest,        string locale,        CancellationToken cancellationToken = default)
    {
        ArgumentNullException.ThrowIfNull(manifest);

        string inline = string.IsNullOrWhiteSpace(locale)
            ? CombineInlineReleaseNotes(manifest)
            : manifest.GetReleaseNotesForLocale(locale);
        if (!string.IsNullOrWhiteSpace(inline))
        {
            string normalized = NormalizeAndLimit(inline);
            await SaveCacheAsync(manifest.Version,locale,normalized,cancellationToken);
            return new ReleaseNotesLoadResult(normalized);
        }

        string? cached = await ReadCacheAsync(manifest.Version,locale,cancellationToken);
        if (!string.IsNullOrWhiteSpace(cached))
        {
            return new ReleaseNotesLoadResult(cached,IsFromCache: true);
        }

        if (!AppUpdateManifest.IsSafeReleaseNotesUrl(manifest.ReleaseNotesUrl))
        {
            return new ReleaseNotesLoadResult(string.Empty);
        }

        try
        {
            Uri sourceUri = GetContentSourceUri(manifest.ReleaseNotesUrl);
            bool isGitHubApiResponse = sourceUri.Host.Equals("api.github.com",StringComparison.OrdinalIgnoreCase);
            using var response = await _httpClient.GetAsync(
                sourceUri,                HttpCompletionOption.ResponseHeadersRead,                cancellationToken);
            response.EnsureSuccessStatusCode();

            MediaTypeHeaderValue? contentType = response.Content.Headers.ContentType;
            if (!isGitHubApiResponse && contentType is not null &&
                contentType.MediaType?.Equals("text/html",StringComparison.OrdinalIgnoreCase) == true)
            {
                return new ReleaseNotesLoadResult(string.Empty,ErrorMessage: "HTML release pages are not Markdown content.");
            }

            await using Stream stream = await response.Content.ReadAsStreamAsync(cancellationToken);
            string responseText = await ReadLimitedTextAsync(stream,cancellationToken);
            string content = isGitHubApiResponse
                ? ReadGitHubReleaseBody(responseText)
                : responseText;
            if (string.IsNullOrWhiteSpace(content))
            {
                return new ReleaseNotesLoadResult(string.Empty);
            }

            string normalized = NormalizeAndLimit(content);
            await SaveCacheAsync(manifest.Version,locale,normalized,cancellationToken);
            return new ReleaseNotesLoadResult(normalized);
        }
        catch (Exception ex) when (ex is HttpRequestException or TaskCanceledException or IOException)
        {
            return new ReleaseNotesLoadResult(string.Empty,ErrorMessage: ex.Message);
        }
    }

    internal static Uri GetContentSourceUri(string releaseNotesUrl)
    {
        if (!Uri.TryCreate(releaseNotesUrl,UriKind.Absolute,out Uri? uri) ||
            !uri.Scheme.Equals(Uri.UriSchemeHttps,StringComparison.OrdinalIgnoreCase))
        {
            throw new InvalidOperationException("Release notes URL must use HTTPS.");
        }

        if (!uri.Host.Equals("github.com",StringComparison.OrdinalIgnoreCase))
        {
            return uri;
        }

        string[] segments = uri.AbsolutePath
            .Split('/',StringSplitOptions.RemoveEmptyEntries);
        if (segments.Length < 5 ||
            !segments[2].Equals("releases",StringComparison.OrdinalIgnoreCase) ||
            !segments[3].Equals("tag",StringComparison.OrdinalIgnoreCase))
        {
            return uri;
        }

        string owner = Uri.UnescapeDataString(segments[0]);
        string repository = Uri.UnescapeDataString(segments[1]);
        string tag = Uri.UnescapeDataString(string.Join('/',segments[4..]));
        return new Uri(
            $"https://api.github.com/repos/{Uri.EscapeDataString(owner)}/{Uri.EscapeDataString(repository)}/releases/tags/{Uri.EscapeDataString(tag)}");
    }

    private static string ReadGitHubReleaseBody(string responseText)
    {
        try
        {
            using JsonDocument document = JsonDocument.Parse(responseText);
            return document.RootElement.TryGetProperty("body",out JsonElement body) &&
                body.ValueKind == JsonValueKind.String
                ? body.GetString() ?? string.Empty
                : string.Empty;
        }
        catch (JsonException)
        {
            return string.Empty;
        }
    }

    private async Task<string?> ReadCacheAsync(
        string version,        string locale,        CancellationToken cancellationToken)
    {
        string path = GetCachePath(version,locale);
        if (!File.Exists(path))
        {
            return null;
        }

        try
        {
            await using FileStream stream = new(path,FileMode.Open,FileAccess.Read,FileShare.Read);
            return await ReadLimitedTextAsync(stream,cancellationToken);
        }
        catch (Exception ex) when (ex is IOException or UnauthorizedAccessException)
        {
            return null;
        }
    }

    private async Task SaveCacheAsync(
        string version,        string locale,        string content,        CancellationToken cancellationToken = default)
    {
        if (string.IsNullOrWhiteSpace(content))
        {
            return;
        }

        try
        {
            Directory.CreateDirectory(_cacheRootPath);
            string path = GetCachePath(version,locale);
            string temporaryPath = path + ".tmp";
            await File.WriteAllTextAsync(temporaryPath,content,Encoding.UTF8,cancellationToken);
            File.Move(temporaryPath,path,overwrite: true);
        }
        catch (Exception ex) when (ex is IOException or UnauthorizedAccessException)
        {
            // The online result is still usable if the optional cache cannot be written.
        }
    }

    private string GetCachePath(string version,string locale)
    {
        string safeVersion = SanitizePathSegment(version);
        string safeLocale = string.IsNullOrWhiteSpace(locale)
            ? "all"
            : SanitizePathSegment(locale);
        return Path.Combine(_cacheRootPath,$"{safeVersion}.{safeLocale}.md");
    }

    private static string CombineInlineReleaseNotes(AppUpdateManifest manifest)
    {
        if (manifest.ReleaseNotes.Count == 0)
        {
            return string.Empty;
        }

        return string.Join(
            Environment.NewLine + Environment.NewLine,            manifest.ReleaseNotes.Values
                .Where(value => !string.IsNullOrWhiteSpace(value))
                .Distinct(StringComparer.Ordinal));
    }

    private static string NormalizeAndLimit(string content)
    {
        string normalized = content.Replace("\0",string.Empty);
        return normalized.Length > MaxCharacters
            ? normalized[..MaxCharacters]
            : normalized;
    }

    private static async Task<string> ReadLimitedTextAsync(Stream stream,CancellationToken cancellationToken)
    {
        var builder = new StringBuilder();
        using var reader = new StreamReader(stream,Encoding.UTF8,detectEncodingFromByteOrderMarks: true,leaveOpen: true);
        char[] buffer = new char[8192];
        int read;
        while ((read = await reader.ReadAsync(buffer.AsMemory(),cancellationToken)) > 0)
        {
            int remaining = MaxCharacters - builder.Length;
            if (remaining <= 0)
            {
                break;
            }

            builder.Append(buffer,0,Math.Min(read,remaining));
            if (read > remaining)
            {
                break;
            }
        }

        return builder.ToString();
    }

    private static HttpClient CreateHttpClient()
    {
        var client = new HttpClient
        {
            Timeout = TimeSpan.FromSeconds(10)
        };
        client.DefaultRequestHeaders.UserAgent.ParseAdd("DeskBox-ReleaseNotes/1.0");
        return client;
    }

    private static string SanitizePathSegment(string value)
    {
        if (string.IsNullOrWhiteSpace(value))
        {
            return "unknown";
        }

        var builder = new StringBuilder(value.Length);
        foreach (char character in value)
        {
            builder.Append(char.IsLetterOrDigit(character) || character is '-' or '_' or '.'
                ? character
                : '_');
        }

        return builder.ToString();
    }
}
