namespace DeskBox.Services;

/// <summary>
/// Identifies which side of a grouping operation failed the chrome policy.
/// </summary>
public enum WidgetGroupChromeParticipant
{
    None,
    Source,
    Target,
    Group
}

/// <summary>
/// Machine-readable reasons why a widget cannot use grouped chrome.
/// </summary>
public enum WidgetGroupChromeRejectionReason
{
    None,
    EffectiveModeIsUnresolved,
    OverlayChromeCannotBeGrouped,
    HiddenChromeCannotBeGrouped,
    UnsupportedChromeMode
}

/// <summary>
/// Result of validating chrome for a group operation.
/// </summary>
public readonly record struct WidgetGroupChromeDecision(
    bool IsAllowed,    WidgetChromeMode? GroupMode,    WidgetGroupChromeParticipant RejectedParticipant,    WidgetGroupChromeRejectionReason RejectionReason,    WidgetChromeMode? RejectedMode);

/// <summary>
/// Defines the title-bar invariant for widget groups without depending on UI or
/// settings services. Callers must pass already-resolved effective member modes.
/// </summary>
public static class WidgetGroupChromePolicy
{
    /// <summary>
    /// Resolves two member modes to the visible chrome required by a group.
    /// The destination owns the resulting surface, so its Standard/Compact
    /// mode wins. Overlay/Hidden members are allowed to join and temporarily
    /// adopt the group's visible chrome; their standalone override is kept for
    /// use if they are detached later.
    /// </summary>
    public static WidgetGroupChromeDecision EvaluateMerge(
        WidgetChromeMode sourceEffectiveMode,        WidgetChromeMode targetEffectiveMode)
    {
        WidgetGroupChromeDecision sourceDecision = EvaluateMergeParticipant(
            sourceEffectiveMode,            WidgetGroupChromeParticipant.Source);
        if (!sourceDecision.IsAllowed)
        {
            return sourceDecision;
        }

        WidgetGroupChromeDecision targetDecision = EvaluateMergeParticipant(
            targetEffectiveMode,            WidgetGroupChromeParticipant.Target);
        if (!targetDecision.IsAllowed)
        {
            return targetDecision;
        }

        if (IsSupportedGroupMode(targetEffectiveMode))
        {
            return Allowed(targetEffectiveMode);
        }

        if (IsSupportedGroupMode(sourceEffectiveMode))
        {
            return Allowed(sourceEffectiveMode);
        }

        return Allowed(WidgetChromeMode.Standard);
    }

    /// <summary>
    /// Validates a requested shared group mode. Runtime callers should reject
    /// invalid requests instead of silently coercing them.
    /// </summary>
    public static WidgetGroupChromeDecision EvaluateGroupMode(
        WidgetChromeMode requestedMode)
    {
        WidgetGroupChromeDecision decision = EvaluateEffectiveMode(
            requestedMode,            WidgetGroupChromeParticipant.Group);
        return decision.IsAllowed
            ? Allowed(requestedMode)
            : decision;
    }

    public static bool IsSupportedGroupMode(WidgetChromeMode mode)
    {
        return mode is WidgetChromeMode.Standard or WidgetChromeMode.Compact;
    }

    /// <summary>
    /// Normalizes persisted group chrome. Legacy System, Overlay, and Hidden
    /// values (as well as unknown values) migrate to a visible Standard title.
    /// </summary>
    public static WidgetChromeMode NormalizePersistedMode(string? persistedValue)
    {
        if (!Enum.TryParse(
                persistedValue,                ignoreCase: true,                out WidgetChromeMode parsedMode) ||
            !Enum.IsDefined(parsedMode))
        {
            return WidgetChromeMode.Standard;
        }

        return NormalizePersistedMode(parsedMode);
    }

    public static WidgetChromeMode NormalizePersistedMode(
        WidgetChromeMode persistedMode)
    {
        return IsSupportedGroupMode(persistedMode)
            ? persistedMode
            : WidgetChromeMode.Standard;
    }

    public static string NormalizePersistedValue(string? persistedValue)
    {
        return WidgetChromeModeNames.ToSettingValue(
            NormalizePersistedMode(persistedValue));
    }

    private static WidgetGroupChromeDecision EvaluateEffectiveMode(
        WidgetChromeMode effectiveMode,        WidgetGroupChromeParticipant participant)
    {
        WidgetGroupChromeRejectionReason rejectionReason = effectiveMode switch
        {
            WidgetChromeMode.Standard or WidgetChromeMode.Compact =>
                WidgetGroupChromeRejectionReason.None,            WidgetChromeMode.System =>
                WidgetGroupChromeRejectionReason.EffectiveModeIsUnresolved,            WidgetChromeMode.Overlay =>
                WidgetGroupChromeRejectionReason.OverlayChromeCannotBeGrouped,            WidgetChromeMode.Hidden =>
                WidgetGroupChromeRejectionReason.HiddenChromeCannotBeGrouped,            _ => WidgetGroupChromeRejectionReason.UnsupportedChromeMode
        };

        return rejectionReason == WidgetGroupChromeRejectionReason.None
            ? Allowed(effectiveMode)
            : new WidgetGroupChromeDecision(
                IsAllowed: false,                GroupMode: null,                RejectedParticipant: participant,                RejectionReason: rejectionReason,                RejectedMode: effectiveMode);
    }

    private static WidgetGroupChromeDecision EvaluateMergeParticipant(
        WidgetChromeMode effectiveMode,        WidgetGroupChromeParticipant participant)
    {
        return effectiveMode switch
        {
            WidgetChromeMode.Standard or
            WidgetChromeMode.Compact or
            WidgetChromeMode.Overlay or
            WidgetChromeMode.Hidden => Allowed(effectiveMode),            WidgetChromeMode.System => new WidgetGroupChromeDecision(
                IsAllowed: false,                GroupMode: null,                RejectedParticipant: participant,                RejectionReason:
                    WidgetGroupChromeRejectionReason.EffectiveModeIsUnresolved,                RejectedMode: effectiveMode),            _ => new WidgetGroupChromeDecision(
                IsAllowed: false,                GroupMode: null,                RejectedParticipant: participant,                RejectionReason:
                    WidgetGroupChromeRejectionReason.UnsupportedChromeMode,                RejectedMode: effectiveMode)
        };
    }

    private static WidgetGroupChromeDecision Allowed(WidgetChromeMode mode)
    {
        return new WidgetGroupChromeDecision(
            IsAllowed: true,            GroupMode: mode,            RejectedParticipant: WidgetGroupChromeParticipant.None,            RejectionReason: WidgetGroupChromeRejectionReason.None,            RejectedMode: null);
    }
}
