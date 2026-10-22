using System.Buffers.Binary;
using System.IO.Pipes;
using System.Security.Principal;
using System.Text.Json;
using DeskBox.Models;
using DeskBox.Platform;
using Microsoft.Win32.SafeHandles;

namespace DeskBox.Services;

public interface IAiOrganizeClient
{
    Task<AiAgentResponse> HealthAsync(CancellationToken cancellationToken);
    Task<AiOrganizeResult> ClassifyAsync(AiOrganizeItem[] items,AiOrganizeCategory[] categories,        bool consent,CancellationToken cancellationToken);
}

public sealed class NaiWaCarryRejectedException(string status,string? detail) : IOException(
    detail is null ? $"奶蛙未到达目标位置（{status}），此文件不会移动。" : $"奶蛙未到达目标位置（{status}：{detail}），此文件不会移动。")
{
    public string Status { get; } = status;
}

// The descriptor carries only an ephemeral local handshake, never model credentials.
public sealed partial class AiOrganizeClient : IAiOrganizeClient
{
    private const int MaxMessage = 64 * 1024;
    private readonly string _descriptorPath;
    public AiOrganizeClient(string? descriptorPath = null) => _descriptorPath = descriptorPath ?? Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData),        "NaiWaChatPet","versions","2.1.0","deskbox-agent","agent.json");

    public Task<AiAgentResponse> HealthAsync(CancellationToken cancellationToken) =>
        SendAsync("health",false,null,null,null,cancellationToken);

    public async Task<AiOrganizeResult> ClassifyAsync(AiOrganizeItem[] items,AiOrganizeCategory[] categories,        bool consent,CancellationToken cancellationToken)
    {
        if (!consent) throw new InvalidOperationException("尚未同意本次文件名分析。");
        if (items.Length > 40 || items.Any(x => x.IsProtected)) throw new ArgumentException("候选超限或含保护项。");
        var response = await SendAsync("classify",consent,items,categories,null,cancellationToken);
        var result = RequireResult(response);
        ValidateResult(result,items,categories,false);
        return result;
    }

    public async Task<AiOrganizeResult> DiscoverCategoriesAsync(AiOrganizeItem[] items,        CancellationToken cancellationToken)
    {
        if (items.Length is < 1 or > 40 || items.Any(x => x.IsProtected))
            throw new ArgumentException("请选择 1–40 个可上传名称。");
        var result = RequireResult(await SendAsync("classify",true,items,            [new("pending","待推断")],null,cancellationToken,proposeCategories: true));
        var categories = result.Categories ?? [];
        if (categories.Length is < 6 or > 10 || categories.Any(c => string.IsNullOrWhiteSpace(c.Id) || c.Id.Length > 64
                || c.Id.Any(ch => !char.IsAsciiLetterOrDigit(ch) && ch is not ('_' or '-'))
                || string.IsNullOrWhiteSpace(c.DisplayName) || c.DisplayName.Length > 40
                || c.DisplayName.IndexOfAny(Path.GetInvalidFileNameChars()) >= 0 || c.DisplayName is "." or "..")
            || categories.Select(c => c.Id).Distinct(StringComparer.Ordinal).Count() != categories.Length
            || categories.Select(c => c.DisplayName).Distinct(StringComparer.OrdinalIgnoreCase).Count() != categories.Length)
            throw new InvalidDataException("AI 必须提出 6–10 个不重复的分类。");
        ValidateResult(result,items,categories,false);
        if ((result.Items?.Length ?? 0) != items.Length)
            throw new InvalidDataException("AI 分类结果不完整，请重试。");
        return result;
    }

    private static AiOrganizeResult RequireResult(AiAgentResponse response) =>
        response.Status == "ok" && response.Result is not null ? response.Result :
        throw new IOException(response.Status switch
        {
            "invalid_result" => "模型返回格式或分类数量不符合要求（6–10 类），请重试。",
            "cancelled" => "模型分析超时或已取消，请重试。",
            "busy" => "奶蛙正在执行另一个请求，请稍后重试。",
            _ => "模型请求未成功，请检查奶蛙已有的 AI 配置和网络；本次没有应用 AI 分类。"
        });

    public async Task<string> CarryAsync(AiCarryMove move,CancellationToken cancellationToken)
    {
        var response = await SendAsync("carry",true,null,null,null,cancellationToken,move);
        if (response.Status != "arrived")
        {
            App.Log($"[NaiWa] Carry rejected status={response.Status} error={response.Error ?? "none"} " +
                $"dest={move.DestinationX:F0},{move.DestinationY:F0} name={move.DestinationName}");
            throw new NaiWaCarryRejectedException(response.Status,response.Error);
        }
        return response.RequestId;
    }

    public async Task CompleteCarryAsync(string id,bool succeeded,CancellationToken cancellationToken)
    {
        var response = await SendAsync("complete_carry",false,null,null,null,cancellationToken,            carryRequestId: id,succeeded: succeeded);
        if (response.Status != "ok") throw new IOException("文件事务已返回，但奶蛙未确认动画回执。");
    }

    public static void ValidateResult(AiOrganizeResult result,AiOrganizeItem[] items,        AiOrganizeCategory[] categories,bool intentOnly)
    {
        var ids = items.Where(x => !x.IsProtected).Select(x => x.ItemId).ToHashSet(StringComparer.Ordinal);
        var categoryIds = categories.Select(x => x.Id).ToHashSet(StringComparer.Ordinal);
        var seen = new HashSet<string>(StringComparer.Ordinal);
        foreach (var item in result.Items ?? [])
            if (intentOnly || !ids.Contains(item.ItemId) || !seen.Add(item.ItemId) || !categoryIds.Contains(item.CategoryId)
                || !double.IsFinite(item.Confidence) || item.Confidence is < 0 or > 1
                || string.IsNullOrWhiteSpace(item.Reason) || item.Reason.Length > 320)
                throw new InvalidDataException("AI 返回了无效的分类建议。");
        seen.Clear();
        foreach (var intent in result.Intents ?? [])
            if (!intentOnly || !categoryIds.Contains(intent.CategoryId) || !seen.Add(intent.CategoryId)
                || intent.Position is not ("keep" or "top_left" or "top_right" or "bottom_left" or "bottom_right"))
                throw new InvalidDataException("AI 返回了无效的布局意图。");
    }

    private async Task<AiAgentResponse> SendAsync(string operation,bool consent,AiOrganizeItem[]? items,        AiOrganizeCategory[]? categories,string? instruction,CancellationToken cancellationToken,        AiCarryMove? carry = null,string? carryRequestId = null,bool succeeded = false,bool proposeCategories = false)
    {
        cancellationToken.ThrowIfCancellationRequested();
        var file = new FileInfo(_descriptorPath);
        if (!file.Exists || file.Length > 4096) throw new IOException("奶蛙尚未连接。");
        var descriptor = JsonSerializer.Deserialize(await File.ReadAllBytesAsync(_descriptorPath,cancellationToken),            AiOrganizationJsonContext.Default.AiAgentDescriptor) ?? throw new InvalidDataException("无效的代理描述。");
        if (descriptor.SchemaVersion != 1 || !descriptor.PipeName.StartsWith("NaiWa.DeskBox.v1.",StringComparison.Ordinal)
            || descriptor.PipeName.Length > 100 || descriptor.PipeName.Any(c => !char.IsAsciiLetterOrDigit(c) && c != '.')
            || descriptor.Session.Length != 64 || descriptor.Token.Length != 64 || descriptor.Pid <= 0)
            throw new InvalidDataException("代理协议不兼容。");
        string id = Guid.NewGuid().ToString("N");
        var request = new AiAgentRequest(1,id,descriptor.Session,descriptor.Token,operation,consent,            items,categories,instruction,Carry: carry,CarryRequestId: carryRequestId,Succeeded: succeeded,ProposeCategories: proposeCategories);
        using var timeout = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        timeout.CancelAfter(TimeSpan.FromSeconds(operation == "carry" ? 180 : 90));
        try { return await ExchangeAsync(descriptor,request,timeout.Token); }
        catch (OperationCanceledException)
        {
            if (operation is "classify" or "parse_intent" or "carry")
            {
                using var cancelTimeout = new CancellationTokenSource(TimeSpan.FromSeconds(2));
                try { await ExchangeAsync(descriptor,new(1,Guid.NewGuid().ToString("N"),descriptor.Session,                    descriptor.Token,"cancel",false,CancelRequestId: id),cancelTimeout.Token); }
                catch (Exception ex) when (ex is IOException or OperationCanceledException or UnauthorizedAccessException) { }
            }
            throw;
        }
    }

    private static async Task<AiAgentResponse> ExchangeAsync(AiAgentDescriptor descriptor,        AiAgentRequest request,CancellationToken cancellationToken)
    {
        using var pipe = new NamedPipeClientStream(".",descriptor.PipeName,PipeDirection.InOut,            PipeOptions.Asynchronous,TokenImpersonationLevel.Identification);
        await pipe.ConnectAsync(3000,cancellationToken);
        VerifyServer(pipe.SafePipeHandle,descriptor.Pid);
        byte[] body = JsonSerializer.SerializeToUtf8Bytes(request,AiOrganizationJsonContext.Default.AiAgentRequest);
        if (body.Length > MaxMessage) throw new InvalidDataException("请求过大。");
        byte[] header = new byte[4]; BinaryPrimitives.WriteInt32LittleEndian(header,body.Length);
        await pipe.WriteAsync(header,cancellationToken); await pipe.WriteAsync(body,cancellationToken);
        await pipe.FlushAsync(cancellationToken);
        await pipe.ReadExactlyAsync(header,cancellationToken);
        int size = BinaryPrimitives.ReadInt32LittleEndian(header);
        if (size is <= 0 or > MaxMessage) throw new InvalidDataException("响应过大。");
        byte[] responseBytes = new byte[size]; await pipe.ReadExactlyAsync(responseBytes,cancellationToken);
        var response = JsonSerializer.Deserialize(responseBytes,AiOrganizationJsonContext.Default.AiAgentResponse)
            ?? throw new InvalidDataException("空响应。");
        if (response.SchemaVersion != 1 || response.RequestId != request.RequestId || response.Session != descriptor.Session)
            throw new InvalidDataException("响应与本次请求不匹配。");
        return response;
    }

    private static void VerifyServer(SafePipeHandle pipe,int expectedPid)
    {
        if (!AgentProcessNativeMethods.GetNamedPipeServerProcessId(pipe,out uint pid) || pid != expectedPid
            || !AgentProcessNativeMethods.ProcessIdToSessionId(pid,out uint session)
            || !AgentProcessNativeMethods.ProcessIdToSessionId((uint)Environment.ProcessId,out uint ownSession)
            || session != ownSession) throw new UnauthorizedAccessException("代理会话不匹配。");
        using var process = AgentProcessNativeMethods.OpenProcess(0x1000,false,pid);
        if (process.IsInvalid || !AgentProcessNativeMethods.OpenProcessToken(process,8,out var token))
            throw new UnauthorizedAccessException("无法验证代理身份。");
        using (token)
        using (var identity = new WindowsIdentity(token.DangerousGetHandle()))
        using (var current = WindowsIdentity.GetCurrent())
            if (identity.User != current.User) throw new UnauthorizedAccessException("代理不属于当前用户。");
    }
}
