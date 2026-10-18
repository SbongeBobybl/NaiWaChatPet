namespace DeskBox.Models;

public sealed record WidgetCompactTransitionVisualProfile(
    int DurationMilliseconds,    double BodyRevealStart,    double BodyHideEnd,    double IdentityTranslation,    bool IsAnimated,    int EasingPower)
{
    public static WidgetCompactTransitionVisualProfile Resolve(
        string? preset,        int customDurationMilliseconds,        bool animationsEnabled)
    {
        if (!animationsEnabled ||
            string.Equals(preset,"None",StringComparison.OrdinalIgnoreCase))
        {
            return new(0,0,0,0,false,1);
        }

        return preset?.Trim().ToLowerInvariant() switch
        {
            "slow" => new(360,0.34,0.58,4,true,3),            "snappy" => new(160,0.22,0.48,3,true,5),            "custom" => new(
                Math.Clamp(customDurationMilliseconds,120,400),                0.28,                0.54,                4,                true,                3),            _ => new(
                Math.Clamp(customDurationMilliseconds,120,400),                0.28,                0.54,                4,                true,                3)
        };
    }

    public double EaseProgress(double progress)
    {
        double value = Math.Clamp(progress,0,1);
        return 1 - Math.Pow(1 - value,Math.Max(1,EasingPower));
    }

    public (double CompactOpacity, double ExpandedOpacity) GetOpacity(
        bool collapsing,        double progress)
    {
        if (!IsAnimated)
        {
            return collapsing ? (1, 0) : (0, 1);
        }

        double value = Math.Clamp(progress,0,1);
        if (collapsing)
        {
            double handoff = Math.Clamp(BodyHideEnd,0.36,0.62);
            double expanded = 1 - SmoothStep(Math.Clamp(value / handoff,0,1));
            double compact = SmoothStep(Math.Clamp(
                (value - handoff) / (1 - handoff),                0,                1));
            return (compact, expanded);
        }

        double expandHandoff = Math.Clamp(BodyRevealStart,0.22,0.42);
        double compactOut = 1 - SmoothStep(Math.Clamp(
            value / expandHandoff,            0,            1));
        double expandedIn = SmoothStep(Math.Clamp(
            (value - expandHandoff) / (1 - expandHandoff),            0,            1));
        return (compactOut, expandedIn);
    }

    public double GetLiveContentOpacity(bool collapsing,double progress)
    {
        if (!collapsing)
        {
            return 1;
        }

        double value = Math.Clamp(progress,0,1);
        return 1 - SmoothStep(Math.Clamp((value - 0.46) / 0.22,0,1));
    }

    public double GetCompactSurfaceOpacity(bool collapsing,double progress)
    {
        double value = Math.Clamp(progress,0,1);
        if (collapsing)
        {
            // Keep the styled capsule background available throughout the handoff.
            // Identity and text use separate delayed stages below.
            return 1;
        }

        return GetOpacity(collapsing: false,value).CompactOpacity;
    }

    public double GetCompactIdentityOpacity(bool collapsing,double progress)
    {
        if (!collapsing)
        {
            return 1;
        }

        double value = Math.Clamp(progress,0,1);
        return SmoothStep(Math.Clamp((value - 0.60) / 0.16,0,1));
    }

    public double GetCompactTextOpacity(bool collapsing,double progress)
    {
        if (!collapsing)
        {
            return 1;
        }

        double value = Math.Clamp(progress,0,1);
        return SmoothStep(Math.Clamp((value - 0.72) / 0.24,0,1));
    }

    public double GetLiveContentTranslationY(bool collapsing,double progress) =>
        collapsing
            ? -4 * (1 - GetLiveContentOpacity(collapsing: true,progress))
            : 0;

    private static double SmoothStep(double value) =>
        value * value * (3 - (2 * value));
}
