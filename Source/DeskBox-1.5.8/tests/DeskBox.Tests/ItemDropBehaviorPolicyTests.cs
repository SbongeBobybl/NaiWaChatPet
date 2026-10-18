using DeskBox.Helpers;
using DeskBox.Models;
using DeskBox.Services;
using DeskBox.ViewModels;

namespace DeskBox.Tests;

public sealed class ItemDropBehaviorPolicyTests
{
    [Fact]
    public void StackItems_ImportAsStacks()
    {
        var stack = new WidgetStackItem
        {
            Path = @"C:\grid\stack",
            Category = WidgetStackCategory.Documents,
            StackKey = "Kind:Documents"
        };

        Assert.Equal(
            ItemDropBehavior.StackImport,
            ItemDropBehaviorPolicy.Resolve(stack));
    }

    [Fact]
    public void FolderItems_ImportIntoTheFolder()
    {
        var folder = new WidgetItem
        {
            Path = @"C:\grid\folder",
            IsFolder = true,
            IsShortcut = false
        };

        Assert.Equal(
            ItemDropBehavior.FolderImport,
            ItemDropBehaviorPolicy.Resolve(folder));
    }

    [Fact]
    public void PlainFiles_DoNothing()
    {
        var file = new WidgetItem
        {
            Path = @"C:\grid\readme.txt",
            IsFolder = false,
            IsShortcut = false
        };

        Assert.Equal(
            ItemDropBehavior.None,
            ItemDropBehaviorPolicy.Resolve(file));
    }

    [Fact]
    public void ShortcutsToApplications_Launch()
    {
        var shortcut = new WidgetItem
        {
            Path = @"C:\grid\photoshop.lnk",
            TargetPath = @"C:\Tools\Photoshop.exe",
            IsFolder = false,
            IsShortcut = true
        };

        Assert.Equal(
            ItemDropBehavior.Launch,
            ItemDropBehaviorPolicy.Resolve(shortcut));
    }

    [Fact]
    public void ShortcutsToFolders_StayFolderImportExcluded()
    {
        // The launch path must never delegate to a folder shortcut: the shell
        // would move user files into the target folder. The Windows directory
        // is a directory that exists on every test machine.
        var shortcut = new WidgetItem
        {
            Path = @"C:\grid\windows.lnk",
            TargetPath = Environment.GetFolderPath(
                Environment.SpecialFolder.Windows),
            IsFolder = false,
            IsShortcut = true
        };

        Assert.Equal(
            ItemDropBehavior.None,
            ItemDropBehaviorPolicy.Resolve(shortcut));
    }

    [Fact]
    public void ShortcutsToUncTargets_AreExcludedWithoutNetworkIo()
    {
        var shortcut = new WidgetItem
        {
            Path = @"C:\grid\netapp.lnk",
            TargetPath = @"\\server\share\app.exe",
            IsFolder = false,
            IsShortcut = true
        };

        Assert.Equal(
            ItemDropBehavior.None,
            ItemDropBehaviorPolicy.Resolve(shortcut));
    }

    [Fact]
    public void ConsumedShortcutDrops_NeverFallBackToImport()
    {
        // Importing relocates the user's files, so a shortcut drop that did not
        // launch must be consumed at the OLE level instead. Only a drop that no
        // shortcut owned keeps its ordinary import semantics.
        Assert.False(ShortcutDropOutcomePolicy.IsConsumed(
            ShellDropLaunchOutcome.NotAttempted));
        Assert.True(ShortcutDropOutcomePolicy.ShouldContinueAsImport(
            ShellDropLaunchOutcome.NotAttempted));

        Assert.False(ShortcutDropOutcomePolicy.IsConsumed(
            ShellDropLaunchOutcome.Launched));
        Assert.False(ShortcutDropOutcomePolicy.ShouldContinueAsImport(
            ShellDropLaunchOutcome.Launched));

        Assert.True(ShortcutDropOutcomePolicy.IsConsumed(
            ShellDropLaunchOutcome.TargetRefused));
        Assert.False(ShortcutDropOutcomePolicy.ShouldContinueAsImport(
            ShellDropLaunchOutcome.TargetRefused));

        Assert.True(ShortcutDropOutcomePolicy.IsConsumed(
            ShellDropLaunchOutcome.LaunchFailed));
        Assert.False(ShortcutDropOutcomePolicy.ShouldContinueAsImport(
            ShellDropLaunchOutcome.LaunchFailed));

        // A gesture another entry point already resolved stays consumed: the
        // legacy WM_DROPFILES / routed paths must not turn it into an import.
        Assert.True(ShortcutDropOutcomePolicy.IsConsumed(
            ShellDropLaunchOutcome.AlreadyResolved));
        Assert.False(ShortcutDropOutcomePolicy.ShouldContinueAsImport(
            ShellDropLaunchOutcome.AlreadyResolved));
    }

    [Fact]
    public void RefusedShortcutDrop_ReportsTheApplicationThatCouldNotOpenIt()
    {
        ShellDropLaunchResult refused =
            ShellDropLaunchResult.Refused(NativeDropEffectPolicy.None, -2147467259);
        ShellDropLaunchResult failed = ShellDropLaunchResult.Failed(-2147467259);

        Assert.True(refused.Consumed);
        Assert.False(refused.Handled);
        Assert.Equal(ShellDropLaunchOutcome.TargetRefused, refused.Outcome);
        Assert.True(failed.Consumed);
        Assert.False(failed.Handled);
        Assert.Equal(ShellDropLaunchOutcome.LaunchFailed, failed.Outcome);
        Assert.False(ShellDropLaunchResult.NotAttempted.Consumed);
    }
}
