using System.Text.Json;
using System.Text.Json.Serialization;
using DeskBox.Models;

namespace DeskBox.Services;

// JSON source-generation contexts for the retired Todo and QuickCapture
// documents. They were declared next to TodoWidgetStore and
// QuickCaptureStore; those stores are deleted, but the backup service still
// deserializes the two historical documents while validating, counting and
// rebasing a restore, so the metadata-only contexts stay.

[JsonSourceGenerationOptions(
    GenerationMode = JsonSourceGenerationMode.Metadata,
    PropertyNamingPolicy = JsonKnownNamingPolicy.CamelCase,
    UseStringEnumConverter = true,
    WriteIndented = true)]
[JsonSerializable(
    typeof(TodoWidgetData),
    TypeInfoPropertyName = "StoreData")]
[JsonSerializable(
    typeof(TodoItem),
    TypeInfoPropertyName = "TodoItem")]
internal sealed partial class TodoJsonContext : JsonSerializerContext
{
}

[JsonSourceGenerationOptions(
    GenerationMode = JsonSourceGenerationMode.Metadata,
    PropertyNamingPolicy = JsonKnownNamingPolicy.CamelCase,
    UseStringEnumConverter = true,
    WriteIndented = true)]
[JsonSerializable(
    typeof(QuickCaptureStoreData),
    TypeInfoPropertyName = "StoreData")]
[JsonSerializable(
    typeof(QuickCaptureItem),
    TypeInfoPropertyName = "QuickCaptureItem")]
internal sealed partial class QuickCaptureJsonContext : JsonSerializerContext
{
}
