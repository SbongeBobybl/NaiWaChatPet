using DeskBox.Models;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class NaiWaOrganizationTests
{
    [Fact]
    public void ReferenceCandidatesIncludeFoldersLargeAndPublicWithoutEnablingSystemMoves()
    {
        var reasons = new[] { DesktopOrganizationExclusionReason.Folder, DesktopOrganizationExclusionReason.SlowItem,
            DesktopOrganizationExclusionReason.BatchLimit, DesktopOrganizationExclusionReason.SourceNotSelected,
            DesktopOrganizationExclusionReason.HiddenOrSystem };
        var items = reasons.Select((r,i) => new DesktopOrganizationFileSnapshot("C:/fixture/item"+i, "item"+i, "",
            200_000_000, DateTime.UtcNow, "Other", null, r, i == 0)).ToList();
        var source = new DesktopOrganizationPlan { SourceItems = items, ExcludedItems = items };
        var result = AiOrganizePlanner.IncludeReferenceCandidates(source);
        Assert.Equal(4, result.EligibleItemCount);
        Assert.Equal(DesktopOrganizationExclusionReason.HiddenOrSystem, Assert.Single(result.ExcludedItems).ExclusionReason);
        Assert.True(AiOrganizePlanner.IsDependencyProtected(result.Targets[0].Items[0]));
        Assert.Equal(4, AiOrganizePlanner.IncludeReferenceCandidates(result).EligibleItemCount);
    }

    [Theory]
    [InlineData(false, false)]
    [InlineData(true, false)]
    [InlineData(false, true)]
    [InlineData(true, true)]
    public async Task MoveWaitsForPetAndRevalidatesAfterWalking(bool changeWhileWalking, bool publicScope)
    {
        string root = Path.Combine(Path.GetTempPath(), "DeskBox.NaiWa.Tests", Guid.NewGuid().ToString("N"));
        string desktop = Directory.CreateDirectory(Path.Combine(root, "desktop")).FullName;
        string storage = Directory.CreateDirectory(Path.Combine(root, "storage")).FullName;
        string source = Path.Combine(desktop, "notes.txt"); await File.WriteAllTextAsync(source, "original content");
        var scan = await new DesktopOrganizationScanner(new DesktopOrganizationClassifier(),
            () => publicScope ? Directory.CreateDirectory(Path.Combine(root,"personal")).FullName : desktop,
            () => publicScope ? desktop : string.Empty).ScanAsync();
        var baseline = new DesktopOrganizationPlanner(new DesktopOrganizationRuleResolver()).CreatePlan(scan, storage, [], [], includePublicDesktop: publicScope);
        var plan = AiOrganizePlanner.Build(baseline, new Dictionary<string,string>(), AiOrganizePlanner.DefaultCategories, AiOrganizationMode.Move);
        var carry = new ReceiptCarry(source, changeWhileWalking);
        var publicTransfer = publicScope ? new PublicFixtureTransfer() : null;
        var settings = new SettingsService(Path.Combine(root, "settings"));
        var transaction = new DesktopOrganizationTransaction(settings, new FileService(),
            new DesktopOrganizationRecoveryStore(Path.Combine(root, "recovery.json")), transfer: publicTransfer) { Carry = carry };
        var result = await transaction.ExecuteAsync(plan);
        Assert.True(carry.Walked);
        Assert.True(carry.Completed == !changeWhileWalking,
            string.Join("; ",result.RetainedItems.Select(item => item.ToString())));
        if (changeWhileWalking)
        {
            Assert.Empty(result.CompletedItems); Assert.Equal("changed while walking", await File.ReadAllTextAsync(source));
            Assert.Single(result.RetainedItems);
        }
        else
        {
            var receipt = Assert.Single(result.CompletedItems);
            Assert.False(File.Exists(source)); Assert.Equal("original content", await File.ReadAllTextAsync(receipt.DestinationPath));
            await transaction.UndoAsync(result.History.Id, ownerWindowHandle: publicScope ? new IntPtr(1) : IntPtr.Zero,
                allowPublicDesktopRestore: publicScope);
            Assert.Equal("original content",await File.ReadAllTextAsync(source));
            Assert.Equal(desktop,Path.GetDirectoryName(Assert.Single(result.History.Items).RestoredPath));
            if (publicTransfer is not null) Assert.Equal(2, publicTransfer.Calls);
        }
        Assert.False(File.Exists(Path.Combine(root, "recovery.json")));
    }

    // Test the journal and original-directory restore with real temporary files,
    // replacing only the interactive Shell boundary (the test runner has no HWND).
    private sealed class PublicFixtureTransfer : IDesktopOrganizationTransfer
    {
        public int Calls { get; private set; }
        public async Task<IReadOnlyList<FileService.FileTransferResult>> MoveAsync(
            IReadOnlyList<FileService.FileTransferPlan> plans, bool publicDesktop, IntPtr ownerWindowHandle,
            Action<FileService.FileTransferResult> itemCompleted, CancellationToken cancellationToken)
        {
            Assert.True(publicDesktop);
            Calls++;
            return await new FileService().ExecuteTransferPlanAsync(plans, move: true,
                itemCompleted: itemCompleted, cancellationToken: cancellationToken);
        }
    }

    [Fact]
    public void UndoCannotRedirectToPublicOrSiblingDirectory()
    {
        string root = Path.Combine(Path.GetTempPath(),"NaiWa.Restore",Guid.NewGuid().ToString("N"));
        string source = Path.Combine(root,"personal","item.txt");
        DesktopOrganizationTransaction.ValidateRestoreDirectory(source,Path.Combine(root,"personal","item (2).txt"));
        Assert.Throws<IOException>(() => DesktopOrganizationTransaction.ValidateRestoreDirectory(source,Path.Combine(root,"public","item.txt")));
        Assert.Throws<IOException>(() => DesktopOrganizationTransaction.ValidateRestoreDirectory(source,Path.Combine(root,"personal","..","public","item.txt")));
    }

    [Fact]
    public void AiReclassificationMovesPublicItemsOnlyWhenSelected()
    {
        var item = new DesktopOrganizationFileSnapshot("C:/fixture/shared.lnk","shared.lnk",".lnk",1,DateTime.UtcNow,
            DesktopOrganizationCategoryIds.Shortcuts,null,DesktopOrganizationExclusionReason.None) { SourceScope = DesktopOrganizationSourceScope.Public };
        var source = new DesktopOrganizationPlan { StorageRootPath = "C:/fixture/storage", Targets =
            [new() { TargetWidgetId="old",CreatesWidget=false,Items=[item],SuggestedDisplayName="old category" }] };
        var mapping = new Dictionary<string,string> { [item.SourcePath] = "development" };
        var references = AiOrganizePlanner.Build(source,mapping,AiOrganizePlanner.DefaultCategories);
        Assert.Equal("development",Assert.Single(references.Targets).CategoryId);
        var move = AiOrganizePlanner.Build(source,mapping,AiOrganizePlanner.DefaultCategories,AiOrganizationMode.Move);
        Assert.Empty(move.Targets);
        Assert.Single(move.ExcludedItems);
        var selected = new DesktopOrganizationPlan { StorageRootPath = source.StorageRootPath,
            Targets = source.Targets, IncludePublicDesktop = true };
        var publicMove = AiOrganizePlanner.Build(selected,mapping,AiOrganizePlanner.DefaultCategories,AiOrganizationMode.Move);
        Assert.Single(publicMove.Targets);
        Assert.Empty(publicMove.ExcludedItems);
    }

    [Fact]
    public void SemanticPlannerKeepsEightCategoriesAndProtectsDependencies()
    {
        string root = Path.Combine(Path.GetTempPath(), "DeskBox.NaiWa.Tests", Guid.NewGuid().ToString("N"));
        var categories = AiOrganizePlanner.DefaultCategories;
        var files = categories.Select((c,i) => new DesktopOrganizationFileSnapshot(Path.Combine(root,$"item{i}.txt"),$"item{i}.txt",".txt",1,
            DateTime.UtcNow,DesktopOrganizationCategoryIds.Documents,null,DesktopOrganizationExclusionReason.None)).ToList();
        files.Add(new(Path.Combine(root,"portable.exe"),"portable.exe",".exe",1,DateTime.UtcNow,
            DesktopOrganizationCategoryIds.Other,null,DesktopOrganizationExclusionReason.None));
        var source = new DesktopOrganizationPlan { StorageRootPath = Path.Combine(root,"storage"), Targets =
            [new() { SourceBucketId="base",TargetWidgetId="base",CreatesWidget=true,Items=files }] };
        var mapping = files.Take(8).Select((f,i)=>(f.SourcePath,categories[i].Id)).ToDictionary(x=>x.SourcePath,x=>x.Id);
        var move = AiOrganizePlanner.Build(source,mapping,categories,AiOrganizationMode.Move);
        Assert.Equal(8,move.Targets.Count);Assert.Equal(8,move.EligibleItemCount);Assert.Single(move.ExcludedItems);
        var views = AiOrganizePlanner.Build(source,mapping,categories,AiOrganizationMode.ViewOnly);
        Assert.Equal(9,views.EligibleItemCount);
    }

    [Theory]
    [InlineData(false)]
    [InlineData(true)]
    public async Task SameCategoryUsesOneTripAndRevalidatesWholeBatch(bool mutate)
    {
        string root = Path.Combine(Path.GetTempPath(), "DeskBox.NaiWa.Tests", Guid.NewGuid().ToString("N"));
        string desktop = Directory.CreateDirectory(Path.Combine(root, "desktop")).FullName;
        string storage = Directory.CreateDirectory(Path.Combine(root, "storage")).FullName;
        string[] paths = [Path.Combine(desktop, "a.txt"), Path.Combine(desktop, "b.txt")];
        foreach (var path in paths) await File.WriteAllTextAsync(path, "original");
        var scan = await new DesktopOrganizationScanner(new DesktopOrganizationClassifier(), () => desktop, () => string.Empty).ScanAsync();
        var baseline = new DesktopOrganizationPlanner(new DesktopOrganizationRuleResolver()).CreatePlan(scan, storage, [], []);
        var plan = AiOrganizePlanner.Build(baseline, new Dictionary<string,string>(), AiOrganizePlanner.DefaultCategories, AiOrganizationMode.Move);
        var carry = new BatchReceiptCarry(mutate);
        var transaction = new DesktopOrganizationTransaction(new SettingsService(Path.Combine(root, "settings")), new FileService(),
            new DesktopOrganizationRecoveryStore(Path.Combine(root, "recovery.json"))) { Carry = carry };
        var result = await transaction.ExecuteAsync(plan);
        Assert.Equal(1, carry.Trips);
        Assert.Equal(!mutate, carry.Completed);
        Assert.Equal(mutate ? 0 : 2, result.CompletedItems.Count);
        Assert.All(paths, path => Assert.Equal(mutate, File.Exists(path)));
    }
    private sealed class BatchReceiptCarry(bool mutate) : IDesktopOrganizationBatchCarry
    {
        public int Trips { get; private set; }
        public bool Completed { get; private set; }
        public Task<string> BeforeMoveAsync(DesktopOrganizationFileSnapshot item, DesktopOrganizationTargetPlan target, CancellationToken ct)
            => throw new InvalidOperationException("Single item route must not be used for a batch");
        public async Task<string> BeforeBatchAsync(IReadOnlyList<DesktopOrganizationFileSnapshot> items, DesktopOrganizationTargetPlan target, CancellationToken ct)
        {
            Trips++; Assert.Equal(2, items.Count);
            Assert.All(items, i => Assert.True(File.Exists(i.SourcePath)));
            if (mutate) await File.WriteAllTextAsync(items[1].SourcePath, "changed during walking", ct);
            return "batch-arrived";
        }
        public Task AfterMoveAsync(string id, bool completed) { Assert.Equal("batch-arrived", id); Completed = completed; return Task.CompletedTask; }
    }

    private sealed class ReceiptCarry(string source, bool mutate) : IDesktopOrganizationCarry
    {
        public bool Walked {get;private set;}
        public bool Completed {get;private set;}
        public async Task<string> BeforeMoveAsync(DesktopOrganizationFileSnapshot item, DesktopOrganizationTargetPlan target, CancellationToken cancellationToken)
        {
            Assert.Equal("original content",await File.ReadAllTextAsync(source,cancellationToken));
            Walked=true;if(mutate)await File.WriteAllTextAsync(source,"changed while walking",cancellationToken);
            return "fixture-arrived";
        }
        public Task AfterMoveAsync(string id,bool completed){Assert.True(Walked);Assert.Equal("fixture-arrived",id);Completed=completed;return Task.CompletedTask;}
    }

    [Fact]
    public void LayoutIntents_MoveAnchoredCategoriesToTheirCornerWithoutOverlap()
    {
        AiLayoutScreen[] screens = [new(new(0, 0, 1200, 800), 1)];
        AiLayoutBox[] occupied = [new(0, 0, 254, 194)];
        AiLayoutRequest[] requests =
        [
            new(AiLayoutAnchor.Auto),
            new(AiLayoutAnchor.BottomRight),
            new(AiLayoutAnchor.BottomRight)
        ];
        var placed = AiLayoutGeometry.Place(requests, screens, occupied);

        Assert.Equal(3, placed.Count);
        // The two bottom-right requests occupy the lowest row, right to left,
        // and the automatic request stays in the first free slot above them.
        Assert.Equal(placed[1].Y, placed[2].Y);
        Assert.True(placed[1].X > placed[2].X);
        Assert.True(placed[1].Y > placed[0].Y);
        Assert.True(placed[1].X + placed[1].Width <= 1200 - 14);
        Assert.True(placed[1].Y + placed[1].Height <= 800 - 14);
        for (int i = 0; i < placed.Count; i++)
            for (int j = i + 1; j < placed.Count; j++)
                Assert.False(placed[i].Overlaps(placed[j]));
        Assert.DoesNotContain(placed, box => box.Overlaps(occupied[0]));
        // The legacy overload keeps its canonical order and determinism.
        Assert.Equal(AiLayoutGeometry.Place(3, screens, occupied), AiLayoutGeometry.Place(3, screens, occupied));
    }

    [Fact]
    public void LayoutIntents_RejectImpossibleAnchorInsteadOfOverlapping()
    {
        // One slot only: the second request cannot be honoured and must fail
        // the whole preview rather than producing an overlapping layout.
        AiLayoutScreen[] screens = [new(new(0, 0, 300, 220), 1)];
        Assert.Throws<InvalidOperationException>(() => AiLayoutGeometry.Place(
            [new(AiLayoutAnchor.BottomRight), new(AiLayoutAnchor.BottomRight)], screens, []));
    }

    [Fact]
    public void PlannerAppliesAcceptedIntentsToCategoryTargets()
    {
        string root = Path.Combine(Path.GetTempPath(), "DeskBox.NaiWa.Tests", Guid.NewGuid().ToString("N"));
        var categories = AiOrganizePlanner.DefaultCategories;
        var files = categories.Select((c, i) => new DesktopOrganizationFileSnapshot(Path.Combine(root, $"item{i}.txt"),
            $"item{i}.txt", ".txt", 1, DateTime.UtcNow, DesktopOrganizationCategoryIds.Documents, null,
            DesktopOrganizationExclusionReason.None)).ToList();
        var source = new DesktopOrganizationPlan { StorageRootPath = Path.Combine(root, "storage"), Targets =
            [new() { SourceBucketId = "base", TargetWidgetId = "base", CreatesWidget = true, Items = files }] };
        var mapping = files.Select((f, i) => (f.SourcePath, categories[i].Id)).ToDictionary(x => x.SourcePath, x => x.Id);
        var intents = new Dictionary<string, AiLayoutIntent>(StringComparer.Ordinal)
        {
            [categories[2].Id] = new(categories[2].Id, "bottom_right", true, true),
            [categories[4].Id] = new(categories[4].Id, "keep", false, false)
        };

        var plan = AiOrganizePlanner.Build(source, mapping, categories, AiOrganizationMode.ViewOnly, intents);
        var study = plan.Targets.Single(t => t.CategoryId == categories[2].Id);
        Assert.Equal("bottom_right", study.PositionPreference);
        Assert.True(study.Collapsed);
        Assert.True(study.Locked);
        var references = plan.Targets.Single(t => t.CategoryId == categories[4].Id);
        Assert.Equal("auto", references.PositionPreference);
        Assert.False(references.Collapsed);
        Assert.False(references.Locked);
        // With no explicit layout, new categories go to the right side.
        var plain = AiOrganizePlanner.Build(source, mapping, categories, AiOrganizationMode.ViewOnly);
        Assert.All(plain.Targets, t => Assert.Equal("top_right", t.PositionPreference));
        Assert.Equal("auto", AiOrganizePlanner.NormalizePosition("unknown"));
        Assert.Equal("top_left", AiOrganizePlanner.NormalizePosition("top_left"));
    }

    [Fact]
    public void CategoryTemplate_RoundTripsAndFallsBackWhenCorrupt()
    {
        var categories = AiOrganizePlanner.DefaultCategories;
        var stored = AiOrganizePlanner.FormatTemplate(categories);
        Assert.Equal(categories, AiOrganizePlanner.ParseTemplate(stored));
        // Renaming keeps the id so remembered rules keep routing.
        var renamed = AiOrganizePlanner.ParseTemplate(["study=科研", "development=开发工具"]);
        Assert.Equal("study", renamed[0].Id);
        Assert.Equal("科研", renamed[0].DisplayName);
        Assert.Empty(AiOrganizePlanner.ParseTemplate(["missing-separator"]));
        Assert.Empty(AiOrganizePlanner.ParseTemplate(["bad id=名称"]));
        Assert.Empty(AiOrganizePlanner.ParseTemplate(["a=1", "a=2"]));
        Assert.Empty(AiOrganizePlanner.ParseTemplate([]));
    }

    [Fact]
    public async Task CategoryTemplate_PersistsThroughSettings()
    {
        string root = Path.Combine(Path.GetTempPath(), "DeskBox.NaiWa.Tests", Guid.NewGuid().ToString("N"));
        var settings = new SettingsService(Path.Combine(root, "settings"));
        await settings.LoadAsync();
        Assert.Empty(settings.Settings.NaiWaCategoryTemplate);
        settings.Settings.NaiWaCategoryTemplate = ["study=学习", "development=开发工具"];
        Assert.True(await settings.SaveCheckedAsync());

        var reloaded = new SettingsService(Path.Combine(root, "settings"));
        await reloaded.LoadAsync();
        Assert.Equal(["study=学习", "development=开发工具"], reloaded.Settings.NaiWaCategoryTemplate);
    }

    [Fact]
    public void RememberedRules_RouteOnlyWidgetCreatingTargets()
    {
        var rules = new List<DesktopOrganizationRule>();
        var items = new List<DesktopOrganizationFileSnapshot>
        {
            new("C:/desktop/a.pdf", "a.pdf", ".pdf", 1, DateTime.UtcNow,
                DesktopOrganizationCategoryIds.Documents, null, DesktopOrganizationExclusionReason.None)
        };
        DesktopOrganizationRuleFactory.AddForNewTargets(
        [
            new() { TargetWidgetId = "widget-1", CategoryId = "study", CreatesWidget = true, Items = items },
            new() { TargetWidgetId = "widget-2", CategoryId = "work", CreatesWidget = false, Items = items }
        ], rules);

        var rule = Assert.Single(rules);
        Assert.Equal("widget-1", rule.TargetWidgetId);
        Assert.Equal([DesktopOrganizationCategoryIds.Documents], rule.CategoryIds);
        Assert.True(rule.IsEnabled);
    }
}
