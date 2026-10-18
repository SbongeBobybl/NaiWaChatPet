namespace DeskBox.Models;

public static class FileCutStatePolicy
{
    public static string[] RemoveDepartedPaths(
        IEnumerable<string> cutPaths,        IEnumerable<string> removedPaths,        IEnumerable<string> replacementPaths)
    {
        var replacements = replacementPaths.ToHashSet(
            StringComparer.OrdinalIgnoreCase);
        var departed = removedPaths
            .Where(path => !replacements.Contains(path))
            .ToHashSet(StringComparer.OrdinalIgnoreCase);

        return cutPaths
            .Where(path => !departed.Contains(path))
            .Distinct(StringComparer.OrdinalIgnoreCase)
            .ToArray();
    }
}
