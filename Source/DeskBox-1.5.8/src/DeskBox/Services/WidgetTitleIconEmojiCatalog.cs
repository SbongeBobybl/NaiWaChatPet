namespace DeskBox.Services;

public sealed record WidgetEmojiCategory(string Id,string[] Emojis);

/// <summary>
/// Curated emoji palette for the title-icon customizer. Entries must render
/// as color emoji through the Segoe UI Emoji font on Windows 11; default
/// text-presentation glyphs carry an explicit variation selector.
/// </summary>
public static class WidgetTitleIconEmojiCatalog
{
    public const string SmileysCategory = "Smileys";
    public const string PeopleCategory = "People";
    public const string AnimalsCategory = "Animals";
    public const string FoodCategory = "Food";
    public const string TravelCategory = "Travel";
    public const string ObjectsCategory = "Objects";
    public const string SymbolsCategory = "Symbols";

    public static readonly WidgetEmojiCategory[] Categories =
    [
        new(SmileysCategory,        [
            "😀","😁","😂","🤣","😊","😇","😌","😍","🥰","😋",            "😎","🤩","🥳","🤔","🤨","😐","😴","🤤","😅","🥺",            "😭","😱","🤯","😠"
        ]),        new(PeopleCategory,        [
            "👍","👎","👌","✌️","🤞","👏","🙌","🙏","💪","👋",            "🤝","✍️","👀","🧠","🏃","🚶","💃","🧘","🏆","🎯",            "⚽","🏀","🎮","🎸"
        ]),        new(AnimalsCategory,        [
            "🐱","🐶","🐭","🐹","🐰","🦊","🐻","🐼","🐨","🐯",            "🦁","🐮","🐷","🐸","🐵","🐔","🐧","🦄","🐝","🦋",            "🐢","🐙","🐬","🐳"
        ]),        new(FoodCategory,        [
            "🍎","🍌","🍇","🍓","🍒","🍑","🍍","🥝","🍅","🥕",            "🌽","🍞","🧀","🍔","🍟","🍕","🌭","🍜","🍣","🍩",            "🍪","🎂","☕","🍵"
        ]),        new(TravelCategory,        [
            "🚗","🚕","🚌","🚲","🚂","✈️","🚀","🛸","⛵","🏔️",            "🌋","🏝️","🏙️","🗼","🗽","🏰","🎡","🗺️","🧭","🏕️",            "🌅","🌙","🌈","❄️"
        ]),        new(ObjectsCategory,        [
            "💻","🖥️","⌨️","🖱️","📱","💾","📷","🎥","🔋","💡",            "🔒","🔑","🛠️","🧲","📎","✂️","📌","🗂️","📁","🗑️",            "⏰","💰","🎁","🔔"
        ]),        new(SymbolsCategory,        [
            "❤️","🧡","💛","💚","💙","💜","🖤","⭐","🌟","✨",            "⚡","🔥","💧","♻️","✅","❌","❓","❗","💯","💤",            "🎵","🎉","🏁","🎨"
        ])
    ];

    public static string[] GetAllEmojis() =>
        Categories.SelectMany(category => category.Emojis).ToArray();
}
