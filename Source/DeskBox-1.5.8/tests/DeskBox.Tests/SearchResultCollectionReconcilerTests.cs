using System.Collections.ObjectModel;
using System.Collections.Specialized;
using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class SearchResultCollectionReconcilerTests
{
    [Fact]
    public void SameIdentitySequence_IsRecognizedAcrossProviderInstances()
    {
        var existing = new[] { CreateFile("one.txt"), CreateFile("two.txt") };
        var incoming = new[] { CreateFile("one.txt"), CreateFile("two.txt") };

        Assert.True(SearchResultCollectionReconciler.HasSameIdentitySequence(existing, incoming));
    }

    [Fact]
    public void ReuseExistingInstances_PreservesResolvedIconMetadata()
    {
        SearchResultItem existing = CreateFile("one.txt");
        existing.IconResolved = true;
        existing.SizeDisplay = "128 KB";
        var incoming = new[] { CreateFile("one.txt") };

        List<SearchResultItem> merged = SearchResultCollectionReconciler.ReuseExistingInstances(
            [existing],
            incoming);

        SearchResultItem result = Assert.Single(merged);
        Assert.Same(existing, result);
        Assert.True(result.IconResolved);
        Assert.Equal("128 KB", result.SizeDisplay);
    }

    [Fact]
    public void Reconcile_UsesGranularChangesAndKeepsMatchingInstances()
    {
        SearchResultItem one = CreateFile("one.txt");
        SearchResultItem two = CreateFile("two.txt");
        SearchResultItem three = CreateFile("three.txt");
        var current = new ObservableCollection<SearchResultItem> { one, two };
        var actions = new List<NotifyCollectionChangedAction>();
        current.CollectionChanged += (_, args) => actions.Add(args.Action);

        bool changed = SearchResultCollectionReconciler.Reconcile(current, [two, one, three]);

        Assert.True(changed);
        Assert.Equal([two, one, three], current);
        Assert.DoesNotContain(NotifyCollectionChangedAction.Reset, actions);
        Assert.Contains(NotifyCollectionChangedAction.Move, actions);
        Assert.Contains(NotifyCollectionChangedAction.Add, actions);
    }

    private static SearchResultItem CreateFile(string name) => new()
    {
        Kind = SearchResultKind.File,
        Title = name,
        DetailPath = $@"C:\\SearchTests\\{name}",
        RelevanceScore = 10
    };
}
