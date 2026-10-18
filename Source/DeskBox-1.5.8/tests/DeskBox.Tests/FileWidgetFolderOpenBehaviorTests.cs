using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class FileWidgetFolderOpenBehaviorTests
{
    [Fact]
    public void DefaultBehavior_RemainsExplorer()
    {
        var settings = new AppSettings();
        var widget = new WidgetConfig();

        Assert.Equal(
            FileWidgetFolderOpenBehaviorNames.Explorer,
            settings.FileWidgetFolderOpenBehavior);
        Assert.Equal(
            FileWidgetFolderOpenBehaviorNames.Explorer,
            FileWidgetFolderOpenBehaviorNames.Resolve(settings, widget));
    }

    [Fact]
    public void WidgetOverride_TakesPrecedenceAndCanFollowGlobalAgain()
    {
        var settings = new AppSettings
        {
            FileWidgetFolderOpenBehavior =
                FileWidgetFolderOpenBehaviorNames.Embedded
        };
        var widget = new WidgetConfig();

        FileWidgetFolderOpenBehaviorNames.SetOverride(
            widget,
            FileWidgetFolderOpenBehaviorNames.Explorer);
        Assert.Equal(
            FileWidgetFolderOpenBehaviorNames.Explorer,
            FileWidgetFolderOpenBehaviorNames.Resolve(settings, widget));

        FileWidgetFolderOpenBehaviorNames.SetOverride(widget, null);
        Assert.Equal(
            FileWidgetFolderOpenBehaviorNames.Embedded,
            FileWidgetFolderOpenBehaviorNames.Resolve(settings, widget));
    }

    [Fact]
    public void InvalidValues_NormalizeWithoutChangingSupportedChoices()
    {
        var widget = new WidgetConfig
        {
            Metadata =
            {
                [FileWidgetFolderOpenBehaviorNames.MetadataKey] = "Legacy"
            }
        };

        Assert.Equal(
            FileWidgetFolderOpenBehaviorNames.Explorer,
            FileWidgetFolderOpenBehaviorNames.NormalizeGlobal("Legacy"));
        Assert.Equal(
            FileWidgetFolderOpenBehaviorNames.Embedded,
            FileWidgetFolderOpenBehaviorNames.NormalizeGlobal(
                FileWidgetFolderOpenBehaviorNames.Embedded));
        Assert.True(
            FileWidgetFolderOpenBehaviorNames.NormalizeOverride(widget));
        Assert.False(
            widget.Metadata.ContainsKey(
                FileWidgetFolderOpenBehaviorNames.MetadataKey));
    }
}
