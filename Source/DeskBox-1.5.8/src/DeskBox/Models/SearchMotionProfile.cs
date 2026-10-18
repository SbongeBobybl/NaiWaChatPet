namespace DeskBox.Models;

public enum SearchAppMotionKind
{
    FadeScale,
    Rise,
    Wave,
    SoftScale
}

public sealed record SearchMotionProfile(
    SearchAppMotionKind Kind,    int DurationMilliseconds,    int MaximumStaggerMilliseconds,    double TranslationY,    double InitialScale,    bool IsAnimated)
{
    public static SearchMotionProfile Resolve(
        int persistedStyle,        bool animationsEnabled)
    {
        if (!animationsEnabled)
        {
            return new(
                SearchAppMotionKind.FadeScale,                0,                0,                0,                1,                false);
        }

        return persistedStyle switch
        {
            1 => new(SearchAppMotionKind.Rise,167,0,4,1,true),            2 => new(SearchAppMotionKind.Wave,167,120,4,1,true),            3 => new(SearchAppMotionKind.SoftScale,200,0,0,0.98,true),            _ => new(SearchAppMotionKind.FadeScale,167,0,0,0.98,true)
        };
    }
}
