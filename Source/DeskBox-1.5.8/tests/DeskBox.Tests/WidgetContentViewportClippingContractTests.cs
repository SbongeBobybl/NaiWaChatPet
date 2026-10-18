namespace DeskBox.Tests;

public sealed class WidgetContentViewportClippingContractTests
{
    [Fact]
    public void GroupContentTransition_ClipsCompositionVisualsToTheBodyViewport()
    {
        string root = FindRepositoryRoot();
        string xaml = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Controls/WidgetShell.xaml"));
        string code = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Controls/WidgetShell.xaml.cs"));
        string fileSurfaceCode = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.xaml.cs"));

        int viewport = xaml.IndexOf(
            "x:Name=\"ContentTransitionViewport\"",
            StringComparison.Ordinal);
        int incomingPresenter = xaml.IndexOf(
            "x:Name=\"ShellContentPresenter\"",
            viewport,
            StringComparison.Ordinal);
        Assert.True(viewport >= 0);
        Assert.True(incomingPresenter > viewport);
        Assert.Contains(
            "Grid.Row=\"1\"",
            xaml[viewport..incomingPresenter],
            StringComparison.Ordinal);
        Assert.Contains(
            "Background=\"Transparent\"",
            xaml[viewport..incomingPresenter],
            StringComparison.Ordinal);

        Assert.Contains(
            "contentTransitionVisual.Compositor.CreateInsetClip()",
            code,
            StringComparison.Ordinal);
        Assert.Contains(
            "contentTransitionVisual.Clip = _contentTransitionClip",
            code,
            StringComparison.Ordinal);

        int beginTransition = code.IndexOf(
            "public void BeginContentTransition(",
            StringComparison.Ordinal);
        int ensureBeforeReparent = code.IndexOf(
            "EnsureContentTransitionViewportClip();",
            beginTransition,
            StringComparison.Ordinal);
        int reparentOutgoing = code.IndexOf(
            "OutgoingContentPresenter.Content = outgoingContent.View;",
            beginTransition,
            StringComparison.Ordinal);
        Assert.InRange(
            ensureBeforeReparent,
            beginTransition + 1,
            reparentOutgoing - 1);

        int animateTransition = code.IndexOf(
            "public Task AnimateContentTransitionAsync(",
            StringComparison.Ordinal);
        int ensureBeforeAnimation = code.IndexOf(
            "EnsureContentTransitionViewportClip();",
            animateTransition,
            StringComparison.Ordinal);
        int animationGuard = code.IndexOf(
            "if (OutgoingContentPresenter.Content is null)",
            animateTransition,
            StringComparison.Ordinal);
        Assert.InRange(
            ensureBeforeAnimation,
            animateTransition + 1,
            animationGuard - 1);

        int suspendLiftedItemTransitions = code.IndexOf(
            "SuspendFileSurfaceItemTransitions(outgoingContent, incomingContent);",
            beginTransition,
            StringComparison.Ordinal);
        Assert.InRange(
            suspendLiftedItemTransitions,
            beginTransition + 1,
            reparentOutgoing - 1);

        int completeTransition = code.IndexOf(
            "public void CompleteContentTransition()",
            StringComparison.Ordinal);
        int resumeLiftedItemTransitions = code.IndexOf(
            "ResumeFileSurfaceItemTransitions();",
            completeTransition,
            StringComparison.Ordinal);
        Assert.True(resumeLiftedItemTransitions > completeTransition);

        Assert.Contains(
            "ItemsGrid.ItemContainerTransitions = null;",
            fileSurfaceCode,
            StringComparison.Ordinal);
        Assert.Contains(
            "ItemsList.ItemContainerTransitions = null;",
            fileSurfaceCode,
            StringComparison.Ordinal);
        Assert.Contains(
            "ItemsGrid.ItemContainerTransitions =\n            _suspendedGridItemContainerTransitions;",
            fileSurfaceCode.Replace("\r\n", "\n", StringComparison.Ordinal),
            StringComparison.Ordinal);
        Assert.Contains(
            "ItemsList.ItemContainerTransitions =\n            _suspendedListItemContainerTransitions;",
            fileSurfaceCode.Replace("\r\n", "\n", StringComparison.Ordinal),
            StringComparison.Ordinal);
    }

    private static string FindRepositoryRoot()
    {
        DirectoryInfo? current = new(AppContext.BaseDirectory);
        while (current is not null)
        {
            if (Directory.Exists(Path.Combine(
                    current.FullName,
                    "src",
                    "DeskBox")))
            {
                return current.FullName;
            }

            current = current.Parent;
        }

        throw new DirectoryNotFoundException(
            "Could not locate repository root.");
    }
}
