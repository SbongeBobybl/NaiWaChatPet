using DeskBox.Controls;
using DeskBox.Models;
using DeskBox.ViewModels;

namespace DeskBox.Tests;

public sealed class TodoManualReorderTests
{
    [Fact]
    public void ResolveManualDropTargetIndex_AccountsForRemovalBeforeInsertion()
    {
        TodoItemViewModel first = CreateItem("first");
        TodoItemViewModel second = CreateItem("second");
        TodoItemViewModel third = CreateItem("third");
        TodoItemViewModel fourth = CreateItem("fourth");
        TodoItemViewModel[] items = [first, second, third, fourth];

        Assert.Equal(
            2,
            TodoDragPackage.ResolveManualDropTargetIndex(
                items,
                second.Id,
                fourth.Id,
                insertAfter: false));
        Assert.Equal(
            3,
            TodoDragPackage.ResolveManualDropTargetIndex(
                items,
                second.Id,
                fourth.Id,
                insertAfter: true));
        Assert.Equal(
            1,
            TodoDragPackage.ResolveManualDropTargetIndex(
                items,
                fourth.Id,
                second.Id,
                insertAfter: false));
        Assert.Equal(
            1,
            TodoDragPackage.ResolveManualDropTargetIndex(
                items,
                second.Id,
                second.Id,
                insertAfter: true));
        Assert.Equal(
            -1,
            TodoDragPackage.ResolveManualDropTargetIndex(
                items,
                "missing",
                second.Id,
                insertAfter: false));
        Assert.Equal(
            -1,
            TodoDragPackage.ResolveManualDropTargetIndex(
                items,
                second.Id,
                null,
                insertAfter: false));
    }

    [Fact]
    public void TodoSurface_UsesManualRowDropWithoutMutatingTheAotProjection()
    {
        string xaml = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetContents/TodoWidgetContent.xaml"));
        string dragDrop = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetContents/TodoWidgetContent.DragDrop.cs"));
        string listInteraction = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetContents/TodoWidgetContent.ListInteraction.cs"));
        string viewModel = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/ViewModels/TodoWidgetViewModel.cs"));

        Assert.Contains("CanReorderItems=\"False\"", xaml, StringComparison.Ordinal);
        Assert.Contains(
            "TodoDragPackage.ResolveManualDropTargetIndex",
            dragDrop,
            StringComparison.Ordinal);
        Assert.Contains(
            "await ViewModel.MoveItemAsync(",
            dragDrop,
            StringComparison.Ordinal);
        Assert.Contains(
            "TodoListView.CanReorderItems = false",
            dragDrop,
            StringComparison.Ordinal);
        Assert.DoesNotContain(
            "CanReorderItems = true",
            dragDrop + listInteraction,
            StringComparison.Ordinal);
        Assert.Contains(
            "public object[] VisibleItemsSource",
            viewModel,
            StringComparison.Ordinal);
    }

    [Fact]
    public void TodoColorFilterChips_UseOnlyManualDragInitiation()
    {
        string[] chipLines = File.ReadAllLines(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetContents/TodoWidgetContent.xaml"))
            .Where(line => line.Contains("ColorFilterButton", StringComparison.Ordinal))
            .ToArray();
        string dragDrop = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetContents/TodoWidgetContent.DragDrop.cs"));

        // native CanDrag 手势初始化与 PointerMoved 里的手工 StartDragAsync 叠加
        // 会在拖拽管线内部抛 0x8000FFFF 杀进程（#528/#565）：色点只准手工路径。
        Assert.NotEmpty(chipLines);
        Assert.DoesNotContain(
            "CanDrag=\"True\"",
            string.Join(Environment.NewLine, chipLines),
            StringComparison.Ordinal);
        Assert.Contains("StartDragAsync", dragDrop, StringComparison.Ordinal);
    }

    [Fact]
    public void TodoColorMarkerDrag_UsesInProcessMarkerWithDeferredSave()
    {
        string dragDrop = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Controls/WidgetContents/TodoWidgetContent.DragDrop.cs"));
        string dragData = File.ReadAllText(TestPaths.FromRepository(
            "src/DeskBox/Services/DeskBoxDragData.cs"));

        // 颜色是进程内状态：走 _draggedColorMarker 字段同步取用，不再经 OLE
        // 自定义格式异步读（Drop 处理器里无 deferral 的 await 曾致丢/挂，
        // 拖过桌面还会被 shell 逐格式探测）。
        Assert.Contains("_draggedColorMarker", dragDrop, StringComparison.Ordinal);
        Assert.DoesNotContain(
            "TodoColorMarkerFormat",
            dragDrop + dragData,
            StringComparison.Ordinal);
        Assert.DoesNotContain(
            "SetTodoColorMarker",
            dragDrop + dragData,
            StringComparison.Ordinal);
        // Drop 里的存盘 await 必须持 deferral，且存盘异常不得逃出 async void。
        Assert.Contains("await ViewModel.SetColorMarkerAsync(", dragDrop, StringComparison.Ordinal);
        Assert.Contains("var deferral = e.GetDeferral();", dragDrop, StringComparison.Ordinal);
        Assert.Contains("deferral.Complete();", dragDrop, StringComparison.Ordinal);
    }

    private static TodoItemViewModel CreateItem(string id)
    {
        return new TodoItemViewModel(new TodoItem
        {
            Id = id,
            Text = id
        });
    }
}
