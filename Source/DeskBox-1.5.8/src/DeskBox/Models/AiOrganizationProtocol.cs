using System.Text.Json.Serialization;

namespace DeskBox.Models;

public sealed record AiOrganizeItem(string ItemId,string DisplayName,string Extension,string Kind,bool IsProtected = false);
public sealed record AiOrganizeCategory(string Id,string DisplayName);
public sealed record AiCategorySuggestion(string ItemId,string CategoryId,double Confidence,string Reason);
public sealed record AiLayoutIntent(string CategoryId,string Position,bool Collapsed,bool Locked);
public sealed record AiOrganizeResult(AiCategorySuggestion[]? Items,AiLayoutIntent[]? Intents,AiOrganizeCategory[]? Categories = null);
public sealed record AiAgentDescriptor(int SchemaVersion,string PipeName,string Session,string Token,int Pid);
public sealed record AiCarryMove(string SourcePath,double DestinationX,double DestinationY,string DestinationName,string[]? SourcePaths = null);
public sealed record AiAgentRequest(
    int SchemaVersion,string RequestId,string Session,string Token,string Operation,bool Consent,    AiOrganizeItem[]? Items = null,AiOrganizeCategory[]? Categories = null,    string? Instruction = null,string? CancelRequestId = null,    AiCarryMove? Carry = null,string? CarryRequestId = null,bool Succeeded = false,bool ProposeCategories = false);
public sealed record AiAgentResponse(
    int SchemaVersion,string RequestId,string Session,string Status,    bool Configured,AiOrganizeResult? Result,string? Error);

[JsonSourceGenerationOptions(PropertyNamingPolicy = JsonKnownNamingPolicy.CamelCase,
    DefaultIgnoreCondition = JsonIgnoreCondition.WhenWritingNull,
    UnmappedMemberHandling = JsonUnmappedMemberHandling.Disallow)]
[JsonSerializable(typeof(AiAgentDescriptor))]
[JsonSerializable(typeof(AiAgentRequest))]
[JsonSerializable(typeof(AiAgentResponse))]
internal partial class AiOrganizationJsonContext : JsonSerializerContext { }
