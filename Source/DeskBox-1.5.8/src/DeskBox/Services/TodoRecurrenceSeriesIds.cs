using DeskBox.Models;

namespace DeskBox.Services;

/// <summary>
/// Retired Todo recurrence helper, trimmed to its data-contract surface.
///
/// Only <see cref="NormalizeSeriesId"/> remains: the retired Todo store calls
/// it while normalizing a historical document's recurrence chain ids. Every
/// scheduling surface (next-occurrence creation, due-date arithmetic,
/// localization keys) belonged to the deleted reminder runtime and is not
/// restored.
/// </summary>
public static class TodoRecurrenceService
{
    public static string? NormalizeSeriesId(string? seriesId)
    {
        return string.IsNullOrWhiteSpace(seriesId) ? null : seriesId.Trim();
    }
}
