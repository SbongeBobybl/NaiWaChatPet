using System.Diagnostics;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace DeskBox.Services;

[JsonSourceGenerationOptions(
    GenerationMode = JsonSourceGenerationMode.Metadata,
    PropertyNamingPolicy = JsonKnownNamingPolicy.CamelCase,
    WriteIndented = true)]
[JsonSerializable(
    typeof(NativeNotificationActivationEnvelope),
    TypeInfoPropertyName = "Envelope")]
internal sealed partial class NativeNotificationActivationEnvelopeJsonContext :
    JsonSerializerContext
{
}

internal sealed class NativeNotificationActivationEnvelope
{
    internal const int CurrentSchemaVersion = 1;

    public int SchemaVersion { get; set; } = CurrentSchemaVersion;
    public string EnvelopeId { get; set; } = string.Empty;
    public DateTimeOffset CreatedAtUtc { get; set; }
    public int SourceProcessId { get; set; }
    public NativeAppNotificationActivationSource ActivationSource { get; set; }
    public string Arguments { get; set; } = string.Empty;
    public Dictionary<string, string> UserInput { get; set; } =
        new(StringComparer.OrdinalIgnoreCase);
    public bool IsLegacyArgumentsOnly { get; set; }
}

internal enum NativeNotificationActivationEnvelopeWriteDisposition
{
    Stored,
    Duplicate,
    Rejected,
    Failed
}

internal sealed record NativeNotificationActivationEnvelopeWriteResult(
    NativeNotificationActivationEnvelopeWriteDisposition Disposition,    NativeNotificationActivationEnvelope? Envelope,    string? Path,    string? Error);

internal enum NativeNotificationActivationEnvelopeTakeDisposition
{
    Empty,
    Consumed,
    Rejected,
    Failed
}

internal sealed record NativeNotificationActivationEnvelopeTakeResult(
    NativeNotificationActivationEnvelopeTakeDisposition Disposition,    NativeNotificationActivationEnvelope? Envelope,    string? Path,    string? Error);

/// <summary>
/// Durable, bounded hand-off for notification activations received by a
/// secondary process. Each activation gets its own atomically published file
/// so concurrent button presses cannot overwrite each other.
/// </summary>
internal sealed class NativeNotificationActivationEnvelopeStore
{
    internal const string SpoolDirectoryName = "pending-notification-activations";
    internal const string LegacyFileName = "pending-notification-activation.txt";
    private const string EnvelopeExtension = ".json";
    private const int MaxEnvelopeBytes = 64 * 1024;
    private const int MaxArgumentsLength = 8 * 1024;
    private const int MaxUserInputEntries = 16;
    private const int MaxUserInputKeyLength = 128;
    private const int MaxUserInputValueLength = 8 * 1024;

    private readonly string _rootPath;
    private readonly string _spoolPath;
    private readonly string _legacyPath;
    private readonly object _takeGate = new();

    internal NativeNotificationActivationEnvelopeStore(string rootPath)
    {
        if (string.IsNullOrWhiteSpace(rootPath))
        {
            throw new ArgumentException("Activation root cannot be empty.",nameof(rootPath));
        }

        _rootPath = Path.GetFullPath(rootPath);
        _spoolPath = Path.Combine(_rootPath,SpoolDirectoryName);
        _legacyPath = Path.Combine(_rootPath,LegacyFileName);
    }

    internal string SpoolPath => _spoolPath;
    internal string LegacyPath => _legacyPath;

    internal int PendingFileCount => Directory.Exists(_spoolPath)
        ? Directory.EnumerateFiles(_spoolPath,$"*{EnvelopeExtension}",SearchOption.TopDirectoryOnly).Count()
        : 0;

    internal bool HasPendingActivation
    {
        get
        {
            try
            {
                if (File.Exists(_legacyPath))
                {
                    return true;
                }

                string legacyDirectory = Path.GetDirectoryName(_legacyPath) ?? _rootPath;
                if (Directory.Exists(legacyDirectory) &&
                    Directory.EnumerateFiles(
                        legacyDirectory,                        $"{LegacyFileName}.claim.*",                        SearchOption.TopDirectoryOnly).Any())
                {
                    return true;
                }

                return Directory.Exists(_spoolPath) &&
                    (Directory.EnumerateFiles(
                            _spoolPath,                            $"*{EnvelopeExtension}",                            SearchOption.TopDirectoryOnly).Any() ||
                        Directory.EnumerateFiles(
                            _spoolPath,                            $"*{EnvelopeExtension}.claim.*",                            SearchOption.TopDirectoryOnly).Any());
            }
            catch
            {
                return false;
            }
        }
    }

    internal NativeNotificationActivationEnvelopeWriteResult Store(
        NativeAppNotificationActivation activation)
    {
        ArgumentNullException.ThrowIfNull(activation);
        return Store(new NativeNotificationActivationEnvelope
        {
            EnvelopeId = Guid.NewGuid().ToString("N"),
            CreatedAtUtc = activation.CapturedAtUtc == default
                ? DateTimeOffset.UtcNow
                : activation.CapturedAtUtc,
            SourceProcessId = activation.SourceProcessId > 0
                ? activation.SourceProcessId
                : Environment.ProcessId,
            ActivationSource = activation.Source,
            Arguments = activation.Arguments ?? string.Empty,
            UserInput = CopyUserInput(activation.UserInput),
            IsLegacyArgumentsOnly = false
        });
    }

    internal NativeNotificationActivationEnvelopeWriteResult Store(
        NativeNotificationActivationEnvelope envelope)
    {
        ArgumentNullException.ThrowIfNull(envelope);

        string? validationError = ValidateAndNormalize(envelope);
        if (validationError is not null)
        {
            return new NativeNotificationActivationEnvelopeWriteResult(
                NativeNotificationActivationEnvelopeWriteDisposition.Rejected,                envelope,                null,                validationError);
        }

        try
        {
            Directory.CreateDirectory(_spoolPath);
            string duplicatePattern = $"*-{envelope.EnvelopeId}{EnvelopeExtension}";
            if (Directory.EnumerateFiles(
                    _spoolPath,                    duplicatePattern,                    SearchOption.TopDirectoryOnly).Any())
            {
                return new NativeNotificationActivationEnvelopeWriteResult(
                    NativeNotificationActivationEnvelopeWriteDisposition.Duplicate,                    envelope,                    null,                    null);
            }

            string fileName =
                $"{envelope.CreatedAtUtc.UtcTicks:D19}-{envelope.EnvelopeId}{EnvelopeExtension}";
            string finalPath = Path.Combine(_spoolPath,fileName);
            string tempPath = Path.Combine(
                _spoolPath,                $".{fileName}.{Environment.ProcessId}.{Guid.NewGuid():N}.tmp");
            string json = JsonSerializer.Serialize(
                envelope,                NativeNotificationActivationEnvelopeJsonContext.Default.Envelope);
            if (System.Text.Encoding.UTF8.GetByteCount(json) > MaxEnvelopeBytes)
            {
                return new NativeNotificationActivationEnvelopeWriteResult(
                    NativeNotificationActivationEnvelopeWriteDisposition.Rejected,                    envelope,                    null,                    "Serialized activation envelope exceeds the size limit.");
            }

            try
            {
                File.WriteAllText(tempPath,json);
                File.Move(tempPath,finalPath,overwrite: false);
            }
            finally
            {
                TryDelete(tempPath);
            }

            return new NativeNotificationActivationEnvelopeWriteResult(
                NativeNotificationActivationEnvelopeWriteDisposition.Stored,                envelope,                finalPath,                null);
        }
        catch (IOException)
        {
            // A matching final path means another process published the same
            // deterministic envelope first.
            if (Directory.Exists(_spoolPath) && Directory.EnumerateFiles(
                    _spoolPath,                    $"*-{envelope.EnvelopeId}{EnvelopeExtension}",                    SearchOption.TopDirectoryOnly).Any())
            {
                return new NativeNotificationActivationEnvelopeWriteResult(
                    NativeNotificationActivationEnvelopeWriteDisposition.Duplicate,                    envelope,                    null,                    null);
            }

            return new NativeNotificationActivationEnvelopeWriteResult(
                NativeNotificationActivationEnvelopeWriteDisposition.Failed,                envelope,                null,                "Failed to publish the activation envelope.");
        }
        catch (Exception ex)
        {
            return new NativeNotificationActivationEnvelopeWriteResult(
                NativeNotificationActivationEnvelopeWriteDisposition.Failed,                envelope,                null,                ex.Message);
        }
    }

    internal NativeNotificationActivationEnvelopeTakeResult TryTakeNext()
    {
        lock (_takeGate)
        {
            RecoverAbandonedClaims();
            string? pendingPath = GetNextPendingPath();
            return pendingPath is null
                ? TryTakeLegacy()
                : TryTakeEnvelope(pendingPath);
        }
    }

    private string? GetNextPendingPath()
    {
        if (!Directory.Exists(_spoolPath))
        {
            return null;
        }

        return Directory.EnumerateFiles(
                _spoolPath,                $"*{EnvelopeExtension}",                SearchOption.TopDirectoryOnly)
            .OrderBy(Path.GetFileName,StringComparer.Ordinal)
            .FirstOrDefault();
    }

    private NativeNotificationActivationEnvelopeTakeResult TryTakeEnvelope(string path)
    {
        string claimPath = path + $".claim.{Environment.ProcessId}.{Guid.NewGuid():N}";
        try
        {
            File.Move(path,claimPath,overwrite: false);
        }
        catch (FileNotFoundException)
        {
            return TryTakeNext();
        }
        catch (IOException ex)
        {
            return new NativeNotificationActivationEnvelopeTakeResult(
                NativeNotificationActivationEnvelopeTakeDisposition.Failed,                null,                path,                ex.Message);
        }

        try
        {
            var info = new FileInfo(claimPath);
            if (info.Length <= 0 || info.Length > MaxEnvelopeBytes)
            {
                return Reject(claimPath,"Activation envelope has an invalid file size.");
            }

            NativeNotificationActivationEnvelope? envelope = JsonSerializer.Deserialize(
                File.ReadAllText(claimPath),                NativeNotificationActivationEnvelopeJsonContext.Default.Envelope);
            if (envelope is null)
            {
                return Reject(claimPath,"Activation envelope JSON was empty.");
            }

            string? validationError = ValidateAndNormalize(envelope);
            if (validationError is not null)
            {
                return Reject(claimPath,validationError);
            }

            TryDelete(claimPath);
            return new NativeNotificationActivationEnvelopeTakeResult(
                NativeNotificationActivationEnvelopeTakeDisposition.Consumed,                envelope,                path,                null);
        }
        catch (JsonException ex)
        {
            return Reject(claimPath,ex.Message);
        }
        catch (Exception ex)
        {
            TryRestoreClaim(claimPath,path);
            return new NativeNotificationActivationEnvelopeTakeResult(
                NativeNotificationActivationEnvelopeTakeDisposition.Failed,                null,                path,                ex.Message);
        }
    }

    private NativeNotificationActivationEnvelopeTakeResult TryTakeLegacy()
    {
        if (!File.Exists(_legacyPath))
        {
            return new NativeNotificationActivationEnvelopeTakeResult(
                NativeNotificationActivationEnvelopeTakeDisposition.Empty,                null,                null,                null);
        }

        string claimPath = _legacyPath + $".claim.{Environment.ProcessId}.{Guid.NewGuid():N}";
        try
        {
            File.Move(_legacyPath,claimPath,overwrite: false);
            var info = new FileInfo(claimPath);
            if (info.Length <= 0 || info.Length > MaxArgumentsLength * 2L)
            {
                return Reject(claimPath,"Legacy activation arguments have an invalid file size.");
            }

            string arguments = File.ReadAllText(claimPath);
            var envelope = new NativeNotificationActivationEnvelope
            {
                EnvelopeId = Guid.NewGuid().ToString("N"),
                CreatedAtUtc = info.CreationTimeUtc == default
                    ? DateTimeOffset.UtcNow
                    : new DateTimeOffset(info.CreationTimeUtc),
                SourceProcessId = 0,
                ActivationSource = NativeAppNotificationActivationSource.Unknown,
                Arguments = arguments,
                UserInput = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase),
                IsLegacyArgumentsOnly = true
            };
            string? validationError = ValidateAndNormalize(envelope);
            if (validationError is not null)
            {
                return Reject(claimPath,validationError);
            }

            TryDelete(claimPath);
            return new NativeNotificationActivationEnvelopeTakeResult(
                NativeNotificationActivationEnvelopeTakeDisposition.Consumed,                envelope,                _legacyPath,                null);
        }
        catch (FileNotFoundException)
        {
            return new NativeNotificationActivationEnvelopeTakeResult(
                NativeNotificationActivationEnvelopeTakeDisposition.Empty,                null,                null,                null);
        }
        catch (Exception ex)
        {
            TryRestoreClaim(claimPath,_legacyPath);
            return new NativeNotificationActivationEnvelopeTakeResult(
                NativeNotificationActivationEnvelopeTakeDisposition.Failed,                null,                _legacyPath,                ex.Message);
        }
    }

    private static NativeNotificationActivationEnvelopeTakeResult Reject(
        string claimPath,        string error)
    {
        TryDelete(claimPath);
        return new NativeNotificationActivationEnvelopeTakeResult(
            NativeNotificationActivationEnvelopeTakeDisposition.Rejected,            null,            claimPath,            error);
    }

    private static string? ValidateAndNormalize(
        NativeNotificationActivationEnvelope envelope)
    {
        if (envelope.SchemaVersion != NativeNotificationActivationEnvelope.CurrentSchemaVersion)
        {
            return $"Unsupported activation envelope schema {envelope.SchemaVersion}.";
        }

        if (!Guid.TryParseExact(envelope.EnvelopeId,"N",out _))
        {
            return "Activation envelope id must be a lowercase-free GUID in N format.";
        }

        envelope.EnvelopeId = envelope.EnvelopeId.ToLowerInvariant();
        envelope.CreatedAtUtc = envelope.CreatedAtUtc == default
            ? DateTimeOffset.UtcNow
            : envelope.CreatedAtUtc.ToUniversalTime();
        envelope.Arguments = envelope.Arguments?.Trim() ?? string.Empty;
        if (string.IsNullOrWhiteSpace(envelope.Arguments) ||
            envelope.Arguments.Length > MaxArgumentsLength)
        {
            return "Activation arguments are empty or exceed the length limit.";
        }

        if (envelope.SourceProcessId < 0)
        {
            return "Activation source process id is invalid.";
        }

        if (!Enum.IsDefined(envelope.ActivationSource))
        {
            return "Activation source is invalid.";
        }

        envelope.UserInput = CopyUserInput(envelope.UserInput);
        if (envelope.UserInput.Count > MaxUserInputEntries)
        {
            return "Activation user input exceeds the entry limit.";
        }

        foreach ((string key, string value) in envelope.UserInput)
        {
            if (string.IsNullOrWhiteSpace(key) ||
                key.Length > MaxUserInputKeyLength ||
                value.Length > MaxUserInputValueLength)
            {
                return "Activation user input exceeds key or value limits.";
            }
        }

        return null;
    }

    private static Dictionary<string, string> CopyUserInput(
        IReadOnlyDictionary<string, string>? userInput)
    {
        var copy = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
        if (userInput is null)
        {
            return copy;
        }

        foreach ((string key, string value) in userInput)
        {
            if (!string.IsNullOrWhiteSpace(key))
            {
                copy[key.Trim()] = value ?? string.Empty;
            }
        }

        return copy;
    }

    private static void TryRestoreClaim(string claimPath,string originalPath)
    {
        try
        {
            if (File.Exists(claimPath) && !File.Exists(originalPath))
            {
                File.Move(claimPath,originalPath,overwrite: false);
            }
        }
        catch
        {
        }
    }

    private void RecoverAbandonedClaims()
    {
        if (Directory.Exists(_spoolPath))
        {
            foreach (string claimPath in Directory.EnumerateFiles(
                         _spoolPath,                         $"*{EnvelopeExtension}.claim.*",                         SearchOption.TopDirectoryOnly))
            {
                TryRecoverAbandonedClaim(claimPath,_spoolPath);
            }
        }

        string legacyDirectory = Path.GetDirectoryName(_legacyPath) ?? _rootPath;
        if (Directory.Exists(legacyDirectory))
        {
            foreach (string claimPath in Directory.EnumerateFiles(
                         legacyDirectory,                         $"{LegacyFileName}.claim.*",                         SearchOption.TopDirectoryOnly))
            {
                TryRecoverAbandonedClaim(claimPath,legacyDirectory);
            }
        }
    }

    private static void TryRecoverAbandonedClaim(string claimPath,string expectedDirectory)
    {
        const string claimMarker = ".claim.";
        string fileName = Path.GetFileName(claimPath);
        int markerIndex = fileName.IndexOf(claimMarker,StringComparison.Ordinal);
        if (markerIndex <= 0)
        {
            return;
        }

        string[] ownerParts = fileName[(markerIndex + claimMarker.Length)..].Split('.');
        if (ownerParts.Length != 2 ||
            !int.TryParse(ownerParts[0],out int ownerProcessId) ||
            !Guid.TryParseExact(ownerParts[1],"N",out _))
        {
            return;
        }

        DateTime lastWriteTimeUtc;
        try
        {
            lastWriteTimeUtc = File.GetLastWriteTimeUtc(claimPath);
        }
        catch
        {
            return;
        }

        if (IsClaimOwnerStillActive(ownerProcessId,lastWriteTimeUtc))
        {
            return;
        }

        string originalPath = Path.Combine(expectedDirectory,fileName[..markerIndex]);
        try
        {
            if (File.Exists(originalPath))
            {
                TryDelete(claimPath);
            }
            else if (File.Exists(claimPath))
            {
                File.Move(claimPath,originalPath,overwrite: false);
            }
        }
        catch
        {
            // Another consumer may recover or publish the same path first.
        }
    }

    private static bool IsClaimOwnerStillActive(
        int processId,        DateTime claimLastWriteTimeUtc)
    {
        if (processId <= 0)
        {
            return false;
        }

        try
        {
            using Process process = Process.GetProcessById(processId);
            if (process.HasExited)
            {
                return false;
            }

            try
            {
                // A process ID may have been reused after the claim was written.
                return process.StartTime.ToUniversalTime() <= claimLastWriteTimeUtc.AddSeconds(1);
            }
            catch
            {
                // If start-time inspection is denied, do not steal from a live PID.
                return true;
            }
        }
        catch (ArgumentException)
        {
            return false;
        }
        catch
        {
            // An unexpected inspection failure is safer to treat as a live owner.
            return true;
        }
    }

    private static void TryDelete(string path)
    {
        try
        {
            if (File.Exists(path))
            {
                File.Delete(path);
            }
        }
        catch
        {
        }
    }
}
