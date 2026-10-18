using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class TodoTitleEditorHeightPolicyTests
{
    [Fact]
    public void EmptyEditor_UsesComfortableDefaultHeight()
    {
        double height = TodoTitleEditorHeightPolicy.ResolveHeight(
            measuredContentHeight: 0,
            availableHeight: 600,
            isEmpty: true);

        Assert.Equal(TodoTitleEditorHeightPolicy.EmptyHeight, height);
        Assert.True(height > TodoTitleEditorHeightPolicy.MinimumHeight);
    }

    [Fact]
    public void ContentHeight_GrowsUntilTheWidgetRelativeMaximum()
    {
        Assert.Equal(
            140,
            TodoTitleEditorHeightPolicy.ResolveHeight(140, 600, isEmpty: false));
        Assert.Equal(
            204,
            TodoTitleEditorHeightPolicy.ResolveHeight(300, 600, isEmpty: false),
            precision: 6);
        Assert.Equal(
            102,
            TodoTitleEditorHeightPolicy.ResolveHeight(300, 300, isEmpty: false),
            precision: 6);
    }

    [Fact]
    public void PreferredHeight_OverridesContentButStillRespectsAvailableSpace()
    {
        Assert.Equal(
            160,
            TodoTitleEditorHeightPolicy.ResolveHeight(
                measuredContentHeight: 200,
                availableHeight: 600,
                isEmpty: false,
                preferredHeight: 160));
        Assert.Equal(
            102,
            TodoTitleEditorHeightPolicy.ResolveHeight(
                measuredContentHeight: 200,
                availableHeight: 300,
                isEmpty: false,
                preferredHeight: 160),
            precision: 6);
    }
}
