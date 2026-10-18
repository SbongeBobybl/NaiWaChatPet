using System.Diagnostics;
using System.Globalization;

namespace DeskBox.Services;

/// <summary>Opt-in per-request timings; never includes filenames or full paths.</summary>
internal sealed class FileOpenTrace
{
    private static long s_nextId;
    private readonly long _id = Interlocked.Increment(ref s_nextId);
    private readonly long _started = Stopwatch.GetTimestamp();
    private readonly string _kind;

    private FileOpenTrace(string path,bool shortcut)
    {
        _kind = GetPathKind(path,shortcut);
    }

    internal static FileOpenTrace? Start(string path,bool shortcut) =>
        PerformanceLogger.IsEnabled ? new(path,shortcut) : null;

    internal IDisposable Measure(string stage) => new Stage(this,stage);

    internal void Mark(string stage,string? details = null) =>
        PerformanceLogger.Mark("FileOpen",string.Create(
            CultureInfo.InvariantCulture,            $"request={_id} kind={_kind} stage={stage} sinceInputMs={Stopwatch.GetElapsedTime(_started).TotalMilliseconds:F1} {details}"));

    internal static string GetPathKind(string path,bool shortcut)
    {
        string location = path.StartsWith(@"\\",StringComparison.Ordinal) &&
            (!path.StartsWith(@"\\?\",StringComparison.Ordinal) ||
             path.StartsWith(@"\\?\UNC\",StringComparison.OrdinalIgnoreCase))
            ? "unc"
            : Uri.TryCreate(path,UriKind.Absolute,out Uri? uri) && !uri.IsFile
                ? "uri"
                : "filesystem";
        return shortcut ? $"shortcut-{location}" : location;
    }

    private sealed class Stage(FileOpenTrace trace,string stage) : IDisposable
    {
        private readonly long _started = Stopwatch.GetTimestamp();

        public void Dispose() => trace.Mark(stage,string.Create(
            CultureInfo.InvariantCulture,            $"durationMs={Stopwatch.GetElapsedTime(_started).TotalMilliseconds:F1}"));
    }
}
