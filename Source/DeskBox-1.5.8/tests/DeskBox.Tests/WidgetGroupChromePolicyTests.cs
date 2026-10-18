using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WidgetGroupChromePolicyTests
{
    [Theory]
    [InlineData(WidgetChromeMode.Standard, WidgetChromeMode.Standard)]
    [InlineData(WidgetChromeMode.Standard, WidgetChromeMode.Compact)]
    [InlineData(WidgetChromeMode.Compact, WidgetChromeMode.Standard)]
    [InlineData(WidgetChromeMode.Compact, WidgetChromeMode.Compact)]
    public void EvaluateMerge_PrefersConcreteVisibleTargetMode(
        WidgetChromeMode sourceMode,
        WidgetChromeMode targetMode)
    {
        WidgetGroupChromeDecision decision =
            WidgetGroupChromePolicy.EvaluateMerge(sourceMode, targetMode);

        Assert.True(decision.IsAllowed);
        Assert.Equal(targetMode, decision.GroupMode);
        Assert.Equal(
            WidgetGroupChromeParticipant.None,
            decision.RejectedParticipant);
        Assert.Equal(
            WidgetGroupChromeRejectionReason.None,
            decision.RejectionReason);
        Assert.Null(decision.RejectedMode);
    }

    [Theory]
    [InlineData(
        WidgetChromeMode.System,
        WidgetGroupChromeRejectionReason.EffectiveModeIsUnresolved)]
    [InlineData(
        (WidgetChromeMode)999,
        WidgetGroupChromeRejectionReason.UnsupportedChromeMode)]
    public void EvaluateMerge_RejectsInvalidSourceWithStructuredReason(
        WidgetChromeMode sourceMode,
        WidgetGroupChromeRejectionReason expectedReason)
    {
        WidgetGroupChromeDecision decision =
            WidgetGroupChromePolicy.EvaluateMerge(
                sourceMode,
                WidgetChromeMode.Standard);

        Assert.False(decision.IsAllowed);
        Assert.Null(decision.GroupMode);
        Assert.Equal(
            WidgetGroupChromeParticipant.Source,
            decision.RejectedParticipant);
        Assert.Equal(expectedReason, decision.RejectionReason);
        Assert.Equal(sourceMode, decision.RejectedMode);
    }

    [Theory]
    [InlineData(
        WidgetChromeMode.System,
        WidgetGroupChromeRejectionReason.EffectiveModeIsUnresolved)]
    [InlineData(
        (WidgetChromeMode)999,
        WidgetGroupChromeRejectionReason.UnsupportedChromeMode)]
    public void EvaluateMerge_RejectsInvalidTargetWithStructuredReason(
        WidgetChromeMode targetMode,
        WidgetGroupChromeRejectionReason expectedReason)
    {
        WidgetGroupChromeDecision decision =
            WidgetGroupChromePolicy.EvaluateMerge(
                WidgetChromeMode.Compact,
                targetMode);

        Assert.False(decision.IsAllowed);
        Assert.Null(decision.GroupMode);
        Assert.Equal(
            WidgetGroupChromeParticipant.Target,
            decision.RejectedParticipant);
        Assert.Equal(expectedReason, decision.RejectionReason);
        Assert.Equal(targetMode, decision.RejectedMode);
    }

    [Theory]
    [InlineData(WidgetChromeMode.Overlay)]
    [InlineData(WidgetChromeMode.Hidden)]
    public void EvaluateMerge_AllowsFloatingOrHiddenSourceToAdoptTargetGroupMode(
        WidgetChromeMode sourceMode)
    {
        WidgetGroupChromeDecision decision =
            WidgetGroupChromePolicy.EvaluateMerge(
                sourceMode,
                WidgetChromeMode.Compact);

        Assert.True(decision.IsAllowed);
        Assert.Equal(WidgetChromeMode.Compact, decision.GroupMode);
    }

    [Theory]
    [InlineData(WidgetChromeMode.Overlay)]
    [InlineData(WidgetChromeMode.Hidden)]
    public void EvaluateMerge_UsesVisibleSourceModeWhenTargetHasOverlayChrome(
        WidgetChromeMode targetMode)
    {
        WidgetGroupChromeDecision decision =
            WidgetGroupChromePolicy.EvaluateMerge(
                WidgetChromeMode.Compact,
                targetMode);

        Assert.True(decision.IsAllowed);
        Assert.Equal(WidgetChromeMode.Compact, decision.GroupMode);
    }

    [Theory]
    [InlineData(WidgetChromeMode.Overlay, WidgetChromeMode.Overlay)]
    [InlineData(WidgetChromeMode.Overlay, WidgetChromeMode.Hidden)]
    [InlineData(WidgetChromeMode.Hidden, WidgetChromeMode.Overlay)]
    [InlineData(WidgetChromeMode.Hidden, WidgetChromeMode.Hidden)]
    public void EvaluateMerge_FallsBackToStandardWhenNeitherMemberHasVisibleChrome(
        WidgetChromeMode sourceMode,
        WidgetChromeMode targetMode)
    {
        WidgetGroupChromeDecision decision =
            WidgetGroupChromePolicy.EvaluateMerge(sourceMode, targetMode);

        Assert.True(decision.IsAllowed);
        Assert.Equal(WidgetChromeMode.Standard, decision.GroupMode);
    }

    [Theory]
    [InlineData(WidgetChromeMode.Standard)]
    [InlineData(WidgetChromeMode.Compact)]
    public void EvaluateGroupMode_AllowsOnlyConcreteVisibleModes(
        WidgetChromeMode mode)
    {
        WidgetGroupChromeDecision decision =
            WidgetGroupChromePolicy.EvaluateGroupMode(mode);

        Assert.True(decision.IsAllowed);
        Assert.Equal(mode, decision.GroupMode);
    }

    [Theory]
    [InlineData(
        WidgetChromeMode.System,
        WidgetGroupChromeRejectionReason.EffectiveModeIsUnresolved)]
    [InlineData(
        WidgetChromeMode.Overlay,
        WidgetGroupChromeRejectionReason.OverlayChromeCannotBeGrouped)]
    [InlineData(
        WidgetChromeMode.Hidden,
        WidgetGroupChromeRejectionReason.HiddenChromeCannotBeGrouped)]
    public void EvaluateGroupMode_RejectsNonConcreteMode(
        WidgetChromeMode mode,
        WidgetGroupChromeRejectionReason expectedReason)
    {
        WidgetGroupChromeDecision decision =
            WidgetGroupChromePolicy.EvaluateGroupMode(mode);

        Assert.False(decision.IsAllowed);
        Assert.Equal(
            WidgetGroupChromeParticipant.Group,
            decision.RejectedParticipant);
        Assert.Equal(expectedReason, decision.RejectionReason);
        Assert.Equal(mode, decision.RejectedMode);
    }

    [Theory]
    [InlineData("Standard", WidgetChromeMode.Standard)]
    [InlineData("standard", WidgetChromeMode.Standard)]
    [InlineData("Compact", WidgetChromeMode.Compact)]
    [InlineData("compact", WidgetChromeMode.Compact)]
    public void NormalizePersistedMode_PreservesSupportedModes(
        string value,
        WidgetChromeMode expected)
    {
        Assert.Equal(
            expected,
            WidgetGroupChromePolicy.NormalizePersistedMode(value));
    }

    [Theory]
    [InlineData(null)]
    [InlineData("")]
    [InlineData("System")]
    [InlineData("Overlay")]
    [InlineData("Hidden")]
    [InlineData("FutureMode")]
    [InlineData("999")]
    public void NormalizePersistedMode_MigratesLegacyOrUnknownModeToStandard(
        string? value)
    {
        Assert.Equal(
            WidgetChromeMode.Standard,
            WidgetGroupChromePolicy.NormalizePersistedMode(value));
        Assert.Equal(
            WidgetChromeModeNames.Standard,
            WidgetGroupChromePolicy.NormalizePersistedValue(value));
    }
}
