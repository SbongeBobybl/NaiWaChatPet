namespace DeskBox.Models;

public sealed record FileDeleteFailure(
    string Path,    string Name,    string ErrorMessage);

public sealed record FileDeleteBatchResult(
    int DeletedCount,    IReadOnlyList<FileDeleteFailure> Failures)
{
    public bool IsComplete => Failures.Count == 0;
}
