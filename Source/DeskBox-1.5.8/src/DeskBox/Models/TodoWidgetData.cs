namespace DeskBox.Models;

public sealed class TodoWidgetData
{
    public int Version { get; set; } = 3;

    public List<TodoItem> Items { get; set; } = [];
}
