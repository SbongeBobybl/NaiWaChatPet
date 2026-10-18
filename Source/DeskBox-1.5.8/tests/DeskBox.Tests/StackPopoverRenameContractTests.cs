namespace DeskBox.Tests;

public sealed class StackPopoverRenameContractTests
{
    [Fact]
    public void StackMemberRename_UsesTheSameInlineEditorContractAsFileSurface()
    {
        string root = FindRepositoryRoot();
        string surface = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.xaml.cs"));
        string rename = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.StackPopoverRename.cs"));
        string popover = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.StackPopover.cs"));

        Assert.Contains(
            "await StartStackPopoverItemRenameAsync(item);",
            surface,
            StringComparison.Ordinal);
        Assert.DoesNotContain(
            "RenameStackPopoverItemAsync",
            surface,
            StringComparison.Ordinal);
        Assert.DoesNotContain("ContentDialog", surface, StringComparison.Ordinal);
        Assert.Contains(
            "WidgetInlineRenameTextBoxStyle",
            rename,
            StringComparison.Ordinal);
        Assert.Contains(
            "StackPopoverItemRenameEditor_KeyDown",
            rename,
            StringComparison.Ordinal);
        Assert.Contains(
            "StackPopoverItemRenameEditor_LostFocus",
            rename,
            StringComparison.Ordinal);
        Assert.Contains(
            "await ViewModel.RenameItemAsync(target, newName);",
            rename,
            StringComparison.Ordinal);
        Assert.Contains(
            "CreateStackPopoverItemRenameEditor(itemsHost);",
            popover,
            StringComparison.Ordinal);
        Assert.Contains(
            "IsStackPopoverItemRenameEditing",
            popover,
            StringComparison.Ordinal);
    }

    [Fact]
    public void StackMemberRename_PreservesPopoverLifecycleAndEscapeCancellation()
    {
        string root = FindRepositoryRoot();
        string rename = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.StackPopoverRename.cs"));
        string popover = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.StackPopover.cs"));
        string surface = File.ReadAllText(Path.Combine(
            root,
            "src/DeskBox/Controls/WidgetContents/FileSurfaceContent.xaml.cs"));

        Assert.Contains(
            "_stackPopoverCleanupPending = true;",
            popover,
            StringComparison.Ordinal);
        Assert.Contains(
            "CancelStackPopoverItemRename();",
            popover,
            StringComparison.Ordinal);
        Assert.Contains(
            "surface-stack-popover-item-rename-opened",
            rename,
            StringComparison.Ordinal);
        Assert.Contains(
            "surface-stack-popover-item-rename-closed",
            rename,
            StringComparison.Ordinal);
        Assert.Contains(
            "if (IsStackPopoverItemRenameEditing)",
            surface,
            StringComparison.Ordinal);
    }

    private static string FindRepositoryRoot()
    {
        DirectoryInfo? current = new(AppContext.BaseDirectory);
        while (current is not null)
        {
            if (File.Exists(Path.Combine(
                    current.FullName,
                    "src",
                    "DeskBox",
                    "DeskBox.csproj")))
            {
                return current.FullName;
            }

            current = current.Parent;
        }

        throw new DirectoryNotFoundException(
            "DeskBox repository root was not found.");
    }
}
