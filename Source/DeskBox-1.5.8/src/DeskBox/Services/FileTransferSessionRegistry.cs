namespace DeskBox.Services;

/// <summary>
/// Describes the part of a transfer that a visible file item belongs to.
/// The registry is deliberately UI-agnostic: it only tracks paths and lets
/// each surface decide how to present or gate the state.
/// </summary>
public enum FileTransferPathKind
{
    None,
    Source,
    Destination,
    DestinationFolder
}

/// <summary>
/// A snapshot of the transfer state for one filesystem path.
/// </summary>
public readonly record struct FileTransferPathState(
    FileTransferPathKind Kind,    bool IsMove,    string? OperationId)
{
    public static FileTransferPathState None =>
        new(FileTransferPathKind.None,false,null);

    public bool IsActive => Kind != FileTransferPathKind.None;

    /// <summary>
    /// Mutating operations must not race an in-flight shell operation.
    /// </summary>
    public bool BlocksMutation => IsActive;

    /// <summary>
    /// A source being moved and a destination that is still being written are
    /// not safe to open from the DeskBox surface. A copied source remains
    /// readable, so copy-source opening is intentionally allowed.
    /// </summary>
    public bool BlocksOpen =>
        Kind == FileTransferPathKind.Destination ||
        Kind == FileTransferPathKind.Source && IsMove;

    public bool IsSource => Kind == FileTransferPathKind.Source;
}

/// <summary>
/// A planned transfer entry registered before the filesystem operation starts.
/// </summary>
public sealed record FileTransferRegistration(
    string SourcePath,    string DestinationPath,    bool SourceIsDirectory = false);

/// <summary>
/// Process-wide, per-FileService registry for active file transfers.
/// Multiple file surfaces can observe the same source and destination paths,
/// including surfaces hosted in different windows.
/// </summary>
public sealed class FileTransferSessionRegistry
{
    private readonly object _gate = new();
    private readonly Dictionary<string, Session> _sessions =
        new(StringComparer.Ordinal);

    /// <summary>
    /// Raised whenever a session is added or removed. Subscribers may receive
    /// this callback from a worker thread and should marshal UI work themselves.
    /// </summary>
    public event Action? StateChanged;

    public int ActiveSessionCount
    {
        get
        {
            lock (_gate)
            {
                return _sessions.Count;
            }
        }
    }

    /// <summary>
    /// Starts tracking a planned transfer. The returned lease removes the
    /// session when the caller leaves its transfer pipeline, including errors
    /// and cancellation.
    /// </summary>
    public FileTransferSessionLease Begin(
        IEnumerable<FileTransferRegistration> registrations,        bool isMove)
    {
        ArgumentNullException.ThrowIfNull(registrations);

        TransferEntry[] entries = registrations
            .Select(registration => CreateEntry(registration))
            .OfType<TransferEntry>()
            .GroupBy(entry => entry.SourcePath,StringComparer.OrdinalIgnoreCase)
            .Select(group => group.Last())
            .ToArray();

        if (entries.Length == 0)
        {
            return FileTransferSessionLease.Empty;
        }

        string operationId = Guid.NewGuid().ToString("N");
        var session = new Session(operationId,isMove,entries);
        lock (_gate)
        {
            _sessions[operationId] = session;
        }

        RaiseStateChanged();
        return new FileTransferSessionLease(this,operationId);
    }

    /// <summary>
    /// Returns the most specific active role for a path. Exact destination or
    /// source matches win over a parent-folder match; this keeps a folder being
    /// transferred distinct from the folder that receives it.
    /// </summary>
    public FileTransferPathState GetState(string? path)
    {
        string? normalizedPath = NormalizePath(path);
        if (normalizedPath is null)
        {
            return FileTransferPathState.None;
        }

        Session[] sessions;
        lock (_gate)
        {
            sessions = _sessions.Values.ToArray();
        }

        Match? best = null;
        foreach (Session session in sessions)
        {
            foreach (TransferEntry entry in session.Entries)
            {
                Match? candidate = MatchEntry(
                    normalizedPath,                    session,                    entry);
                if (candidate is null ||
                    best is not null && candidate.Score <= best.Score)
                {
                    continue;
                }

                best = candidate;
            }
        }

        return best?.State ?? FileTransferPathState.None;
    }

    public bool IsPathActive(string? path) => GetState(path).IsActive;

    internal void End(string operationId)
    {
        bool removed;
        lock (_gate)
        {
            removed = _sessions.Remove(operationId);
        }

        if (removed)
        {
            RaiseStateChanged();
        }
    }

    private static TransferEntry? CreateEntry(
        FileTransferRegistration registration)
    {
        string? sourcePath = NormalizePath(registration.SourcePath);
        string? destinationPath = NormalizePath(registration.DestinationPath);
        if (sourcePath is null || destinationPath is null)
        {
            return null;
        }

        string? destinationFolderPath = Path.GetDirectoryName(destinationPath);
        if (string.IsNullOrWhiteSpace(destinationFolderPath))
        {
            return null;
        }

        return new TransferEntry(
            sourcePath,            destinationPath,            NormalizePath(destinationFolderPath),            registration.SourceIsDirectory);
    }

    private static Match? MatchEntry(
        string path,        Session session,        TransferEntry entry)
    {
        if (PathsEqual(path,entry.DestinationPath))
        {
            return new Match(
                new FileTransferPathState(
                    FileTransferPathKind.Destination,                    session.IsMove,                    session.OperationId),                Score: 500);
        }

        if (entry.SourceIsDirectory &&
            IsPathUnderDirectory(path,entry.DestinationPath))
        {
            return new Match(
                new FileTransferPathState(
                    FileTransferPathKind.Destination,                    session.IsMove,                    session.OperationId),                Score: 350);
        }

        if (PathsEqual(path,entry.SourcePath))
        {
            return new Match(
                new FileTransferPathState(
                    FileTransferPathKind.Source,                    session.IsMove,                    session.OperationId),                Score: 400);
        }

        if (entry.SourceIsDirectory &&
            IsPathUnderDirectory(path,entry.SourcePath))
        {
            return new Match(
                new FileTransferPathState(
                    FileTransferPathKind.Source,                    session.IsMove,                    session.OperationId),                Score: 300);
        }

        if (entry.DestinationFolderPath is not null &&
            PathsEqual(path,entry.DestinationFolderPath))
        {
            return new Match(
                new FileTransferPathState(
                    FileTransferPathKind.DestinationFolder,                    session.IsMove,                    session.OperationId),                Score: 200);
        }

        return null;
    }

    private static bool PathsEqual(string first,string second) =>
        string.Equals(first,second,StringComparison.OrdinalIgnoreCase);

    private static bool IsPathUnderDirectory(
        string candidatePath,        string directoryPath)
    {
        try
        {
            string candidate = Path.TrimEndingDirectorySeparator(
                Path.GetFullPath(candidatePath));
            string directory = Path.TrimEndingDirectorySeparator(
                Path.GetFullPath(directoryPath));
            if (PathsEqual(candidate,directory))
            {
                return false;
            }

            string prefix = directory.EndsWith(
                    Path.DirectorySeparatorChar)
                ? directory
                : directory + Path.DirectorySeparatorChar;
            return candidate.StartsWith(
                prefix,                StringComparison.OrdinalIgnoreCase);
        }
        catch
        {
            return false;
        }
    }

    private static string? NormalizePath(string? path)
    {
        try
        {
            return string.IsNullOrWhiteSpace(path)
                ? null
                : Path.TrimEndingDirectorySeparator(Path.GetFullPath(path));
        }
        catch
        {
            return null;
        }
    }

    private void RaiseStateChanged()
    {
        Action? changed = StateChanged;
        if (changed is null)
        {
            return;
        }

        foreach (Delegate subscriber in changed.GetInvocationList())
        {
            try
            {
                ((Action)subscriber)();
            }
            catch (Exception ex)
            {
                App.LogVerbose($"[FileTransfer] State subscriber failed: {ex.Message}");
            }
        }
    }

    private sealed record Session(
        string OperationId,        bool IsMove,        IReadOnlyList<TransferEntry> Entries);

    private sealed record TransferEntry(
        string SourcePath,        string DestinationPath,        string? DestinationFolderPath,        bool SourceIsDirectory);

    private sealed record Match(FileTransferPathState State,int Score);
}

/// <summary>
/// Removes one active transfer session when disposed.
/// </summary>
public sealed class FileTransferSessionLease : IDisposable
{
    private readonly FileTransferSessionRegistry? _registry;
    private readonly string? _operationId;
    private int _disposed;

    internal static FileTransferSessionLease Empty { get; } =
        new(null,null);

    internal FileTransferSessionLease(
        FileTransferSessionRegistry? registry,        string? operationId)
    {
        _registry = registry;
        _operationId = operationId;
    }

    public string? OperationId => _operationId;

    public void Dispose()
    {
        if (Interlocked.Exchange(ref _disposed,1) != 0 ||
            _registry is null ||
            _operationId is null)
        {
            return;
        }

        _registry.End(_operationId);
    }
}
