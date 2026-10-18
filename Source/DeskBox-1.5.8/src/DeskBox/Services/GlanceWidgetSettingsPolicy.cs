using DeskBox.Models;

namespace DeskBox.Services;

internal static class GlanceWidgetSettingsPolicy
{
    public static void SetLocalImageFiles(
        GlanceWidgetData settings,        IEnumerable<string> imagePaths)
    {
        ArgumentNullException.ThrowIfNull(settings);
        ArgumentNullException.ThrowIfNull(imagePaths);

        settings.BackgroundSource = GlanceBackgroundSource.LocalFiles;
        settings.LocalImagePaths = imagePaths
            .Where(path => !string.IsNullOrWhiteSpace(path))
            .Select(path => path.Trim())
            .Distinct(StringComparer.OrdinalIgnoreCase)
            .ToList();
    }

    public static void ClearLocalSource(GlanceWidgetData settings)
    {
        ArgumentNullException.ThrowIfNull(settings);
        if (settings.BackgroundSource == GlanceBackgroundSource.LocalFolder)
        {
            settings.LocalFolderPath = null;
        }
        else
        {
            settings.LocalImagePaths.Clear();
        }
    }

    public static bool IsDisplayElementVisible(
        GlanceWidgetData settings,        GlanceDisplayElement element)
    {
        ArgumentNullException.ThrowIfNull(settings);
        return element switch
        {
            GlanceDisplayElement.Time => settings.ShowTime,            GlanceDisplayElement.Date => settings.ShowDate,            GlanceDisplayElement.Year => settings.ShowYear,            GlanceDisplayElement.Weekday => settings.ShowWeekday,            GlanceDisplayElement.Calendar => settings.ShowCalendar,            _ => false
        };
    }

    public static void SetDisplayElement(
        GlanceWidgetData settings,        GlanceDisplayElement element,        bool isVisible)
    {
        ArgumentNullException.ThrowIfNull(settings);
        switch (element)
        {
            case GlanceDisplayElement.Time:
                settings.ShowTime = isVisible;
                break;
            case GlanceDisplayElement.Date:
                settings.ShowDate = isVisible;
                if (!isVisible)
                {
                    settings.ShowYear = false;
                }
                break;
            case GlanceDisplayElement.Year:
                settings.ShowYear = isVisible;
                if (isVisible)
                {
                    settings.ShowDate = true;
                }
                break;
            case GlanceDisplayElement.Weekday:
                settings.ShowWeekday = isVisible;
                break;
            case GlanceDisplayElement.Calendar:
                settings.ShowCalendar = isVisible;
                if (isVisible)
                {
                    settings.Layout = GlanceLayoutMode.Calendar;
                }
                else if (settings.Layout == GlanceLayoutMode.Calendar)
                {
                    settings.Layout = GlanceLayoutMode.Centered;
                }
                break;
        }
    }

    public static void SetLayout(
        GlanceWidgetData settings,        GlanceLayoutMode layout)
    {
        ArgumentNullException.ThrowIfNull(settings);
        settings.Layout = layout;
        settings.ShowCalendar = layout == GlanceLayoutMode.Calendar;
    }

    public static void SetPhotoPlayback(
        GlanceWidgetData settings,        double rotationIntervalMinutes,        bool randomOrder,        GlanceTransitionMode transition,        GlanceTransitionSpeed transitionSpeed,        GlanceReadabilityMode readability,        bool showPhotoControls)
    {
        ArgumentNullException.ThrowIfNull(settings);
        settings.RotationIntervalMinutes = rotationIntervalMinutes;
        settings.RandomOrder = randomOrder;
        settings.Transition = transition;
        settings.TransitionSpeed = transitionSpeed;
        settings.Readability = readability;
        settings.ShowPhotoControls = showPhotoControls;
    }
}
