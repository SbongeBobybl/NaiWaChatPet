// Copyright (c) DeskBox. All rights reserved.

using DeskBox.Platform;
using DeskBox.Services;
using DeskBox.Views;
using Xunit;

namespace DeskBox.Tests;

/// <summary>
/// Pins the integrity-asymmetry contract shared by the launch gate, the
/// elevation notice, the settings diagnostic, and the updater restart
/// (2026-10-14 batch, feedback elevation-notice / wechat newuser families):
/// only asymmetric shell/process relationships warn or act; same-level
/// sessions — including fully elevated company desktops — stay silent, and
/// the updater never hands an app down to Medium under an elevated shell
/// (that would manufacture the tray-dead, drag-dead ShellAboveApp state).
/// </summary>
public class ShellIntegrityAsymmetryContractTests
{
    [Theory]
    [InlineData(false, false, ShellIntegrityMismatch.None)]
    [InlineData(true, true, ShellIntegrityMismatch.None)]
    [InlineData(true, false, ShellIntegrityMismatch.AppAboveShell)]
    [InlineData(false, true, ShellIntegrityMismatch.ShellAboveApp)]
    public void AssessmentMismatch_DerivesAsymmetryFromTokenSnapshots(
        bool processElevated,
        bool explorerElevated,
        ShellIntegrityMismatch expected)
    {
        var assessment = new StartupElevationAssessment(
            IsCurrentProcessElevated: processElevated,
            IsExplorerElevated: explorerElevated,
            IsUacDisabled: false,
            CanRelaunchUnelevated: false);

        Assert.Equal(expected, assessment.Mismatch);
    }

    [Fact]
    public void NoticeVariant_AppAboveShellOffersOneClickExit()
    {
        // The interactive notice no longer relaunches the app itself (the
        // shell-parented child landed on a startup path whose raised-reveal
        // state starved drags); it exits the elevated instance and the user
        // restarts normally, which always lands at the shell's level.
        Assert.Equal(
            ElevationNoticeVariant.ExitElevatedInstance,
            ElevationNoticeWindow.SelectVariant(new StartupElevationAssessment(
                IsCurrentProcessElevated: true,
                IsExplorerElevated: false,
                IsUacDisabled: false,
                CanRelaunchUnelevated: true)));
        Assert.Equal(
            ElevationNoticeVariant.ExitElevatedInstance,
            ElevationNoticeWindow.SelectVariant(new StartupElevationAssessment(
                IsCurrentProcessElevated: true,
                IsExplorerElevated: false,
                IsUacDisabled: true,
                CanRelaunchUnelevated: false)));
    }

    [Fact]
    public void NoticeVariant_ShellAboveAppNeverOffersRestart()
    {
        // The app cannot elevate the shell; offering an "fix" action in the
        // ShellAboveApp direction would be a dead button.
        Assert.Equal(
            ElevationNoticeVariant.ExplainShellAboveApp,
            ElevationNoticeWindow.SelectVariant(new StartupElevationAssessment(
                IsCurrentProcessElevated: false,
                IsExplorerElevated: true,
                IsUacDisabled: false,
                CanRelaunchUnelevated: true)));
    }

    [Theory]
    [InlineData(true, false, true)]
    [InlineData(true, true, false)]
    [InlineData(true, null, false)]
    [InlineData(false, false, false)]
    [InlineData(false, true, false)]
    public void UpdaterRestartGuard_OnlyHandsDownToAStandardShell(
        bool updaterElevated,
        bool? shellElevated,
        bool expected)
    {
        Assert.Equal(
            expected,
            ShellUnelevatedLaunch.DecideRestartAtShellIntegrity(updaterElevated, shellElevated));
    }

    [Fact]
    public void DiagnoseIssue_ExtendsToShellAboveApp()
    {
        // The settings diagnostic models the reverse asymmetry as its own
        // issue; a distinct member (not a PermissionMismatch reuse) keeps
        // repair actions from offering a relaunch there.
        Assert.NotEqual(
            DragDropDiagnosticIssue.PermissionMismatch,
            DragDropDiagnosticIssue.ShellAboveApp);
        Assert.NotEqual(DragDropDiagnosticIssue.None, DragDropDiagnosticIssue.ShellAboveApp);
    }

    [Fact]
    public void DiagnoseIssue_KnowsShortcutRunAsAsDistinctIssue()
    {
        // A .lnk-level "run as administrator" elevates on every launch and
        // is invisible to the registry AppCompat scan, so it must surface as
        // its own issue with its own remediation text — never folded into
        // the startup/shortcut or appcompat buckets.
        Assert.NotEqual(
            DragDropDiagnosticIssue.StartupShortcutIssue,
            DragDropDiagnosticIssue.ShortcutRunAs);
        Assert.NotEqual(DragDropDiagnosticIssue.AppCompatIssue, DragDropDiagnosticIssue.ShortcutRunAs);
        Assert.NotEqual(DragDropDiagnosticIssue.None, DragDropDiagnosticIssue.ShortcutRunAs);
    }
}
