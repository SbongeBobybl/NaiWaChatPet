namespace DeskBox.Models;

/// <summary>
/// Persisted title identity layouts for a widget group. FollowDefault is valid
/// only in a stored group override; controls always receive a resolved mode.
/// </summary>
public static class WidgetGroupTitleDisplayModes
{
    public const string FollowDefault = "FollowDefault";
    public const string IconAndText = "IconAndText";
    public const string IconOnly = "IconOnly";
    public const string TextOnly = "TextOnly";

    public static string Normalize(string? value,bool allowFollowDefault)
    {
        return value switch
        {
            FollowDefault when allowFollowDefault => FollowDefault,            IconOnly => IconOnly,            TextOnly => TextOnly,            _ => IconAndText
        };
    }

    public static string Resolve(string? groupValue,string? defaultValue)
    {
        string normalized = Normalize(
            groupValue,            allowFollowDefault: true);
        return normalized == FollowDefault
            ? Normalize(defaultValue,allowFollowDefault: false)
            : normalized;
    }
}
