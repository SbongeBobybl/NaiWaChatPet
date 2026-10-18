namespace DeskBox.Tests;

public sealed class TodoCompletionContainerReuseContractTests
{
    [Fact]
    public void ListCompletionHandler_SynchronizesTheContainerOwnedByTheChangedItem()
    {
        string source = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetContents/TodoWidgetContent.DragDrop.cs"));
        string interactionSource = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetContents/TodoWidgetContent.ListInteraction.cs"));

        int handlerStart = source.IndexOf(
            "private async void ItemCompletionCheckBox_Click",
            StringComparison.Ordinal);
        int nextHandlerStart = source.IndexOf(
            "private async void ImportantItemButton_Click",
            handlerStart,
            StringComparison.Ordinal);
        string handler = source[handlerStart..nextHandlerStart];

        Assert.Contains(
            "bool updated = await SetCompletedWithFeedbackAsync",
            handler,
            StringComparison.Ordinal);
        Assert.Contains(
            "SynchronizeTodoCompletionCheckBox(item)",
            handler,
            StringComparison.Ordinal);
        Assert.Contains(
            "else if (ReferenceEquals(checkBox.DataContext, item))",
            handler,
            StringComparison.Ordinal);
        Assert.Contains("TodoListView.ContainerFromItem(item)", interactionSource, StringComparison.Ordinal);
        Assert.Contains(
            "FindVisualChild<CheckBox>(container, \"TodoCompletionCheckBox\")",
            interactionSource,
            StringComparison.Ordinal);
    }
}
