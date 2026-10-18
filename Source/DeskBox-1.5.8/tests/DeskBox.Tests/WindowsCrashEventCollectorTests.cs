using System.Text.Json;
using DeskBox.Services;

namespace DeskBox.Tests;

public sealed class WindowsCrashEventCollectorTests
{
    [Theory]
    [InlineData("C:\\Users\\someone\\AppData\\Local\\DeskBox\\DeskBox.exe")]
    [InlineData("C:\\Windows\\explorer.exe")]
    [InlineData("DeskBox.exe")]
    [InlineData("EXPLORER.EXE")]
    [InlineData("\"C:\\Program Files\\DeskBox\\DeskBox.exe\"")]
    public void FilterPolicy_IncludesMonitoredImages(string value)
    {
        Assert.True(WindowsCrashEventFilterPolicy.ShouldInclude(
            new Dictionary<string, string> { ["AppPath"] = value }));
    }

    [Theory]
    [InlineData("C:\\Games\\MyDeskBox.exe")]
    [InlineData("C:\\Windows\\System32\\notepad.exe")]
    [InlineData("explorer")]
    [InlineData("")]
    [InlineData("C:\\Users\\someone\\Desktop\\explorer.exe.bak")]
    public void FilterPolicy_ExcludesEverythingElse(string value)
    {
        Assert.False(WindowsCrashEventFilterPolicy.ShouldInclude(
            new Dictionary<string, string> { ["AppPath"] = value }));
    }

    [Fact]
    public void ParseEvents_ExtractsCrashMetadataAndFiltersUnrelatedApps()
    {
        string xml = """
            <Events>
              <Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
                <System>
                  <Provider Name="Application Error"/>
                  <EventID Qualifiers="0">1000</EventID>
                  <TimeCreated SystemTime="2026-10-13T13:51:30.500000000Z"/>
                </System>
                <EventData>
                  <Data Name="AppName">DeskBox.exe</Data>
                  <Data Name="AppPath">C:\Users\someone\AppData\Local\DeskBox\DeskBox.exe</Data>
                  <Data Name="ExceptionCode">0xc0000409</Data>
                  <Data Name="FaultingModule">deskbox_native.dll</Data>
                </EventData>
              </Event>
              <Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
                <System>
                  <Provider Name="Windows Error Reporting"/>
                  <EventID>1001</EventID>
                  <TimeCreated SystemTime="2026-10-13T13:52:10.000000000Z"/>
                </System>
                <EventData>
                  <Data Name="AppPath">C:\Windows\explorer.exe</Data>
                  <Data Name="FaultingModule">thirdparty.shell.dll</Data>
                </EventData>
              </Event>
              <Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
                <System>
                  <Provider Name="Application Error"/>
                  <EventID>1000</EventID>
                  <TimeCreated SystemTime="2026-10-13T13:53:00.000000000Z"/>
                </System>
                <EventData>
                  <Data Name="AppName">SomeOtherApp.exe</Data>
                </EventData>
              </Event>
            </Events>
            """;

        IReadOnlyList<WindowsCrashEventEntry> events = WindowsCrashEventCollector.ParseEvents(
            xml,
            DeskBoxDiagnosticsBundleService.SanitizeDiagnosticsValue);

        Assert.Equal(2, events.Count);
        WindowsCrashEventEntry deskBox = events[0];
        Assert.Equal("Application Error", deskBox.Provider);
        Assert.Equal(1000, deskBox.EventId);
        Assert.Equal(
            new DateTimeOffset(2026, 10, 13, 13, 51, 30, 500, TimeSpan.Zero),
            deskBox.TimeCreatedUtc);
        Assert.Equal("<PATH>\\DeskBox.exe", deskBox.Properties["AppPath"]);
        Assert.Equal("0xc0000409", deskBox.Properties["ExceptionCode"]);

        WindowsCrashEventEntry explorer = events[1];
        Assert.Equal("Windows Error Reporting", explorer.Provider);
        Assert.Equal("thirdparty.shell.dll", explorer.Properties["FaultingModule"]);
    }

    [Fact]
    public async Task CollectAsync_ReportsUnavailableWhenQueryFails()
    {
        var collector = new WindowsCrashEventCollector(
            () => throw new InvalidOperationException("wevtutil missing"));

        WindowsCrashEventReport report = await collector.CollectAsync(
            DeskBoxDiagnosticsBundleService.SanitizeDiagnosticsValue);

        Assert.Equal("unavailable", report.Status);
        Assert.Empty(report.Events);
    }

    [Fact]
    public async Task CollectAsync_CollectsThroughTestSeam()
    {
        string xml = """
            <Events>
              <Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
                <System>
                  <Provider Name="Application Error"/>
                  <EventID>1000</EventID>
                  <TimeCreated SystemTime="2026-10-13T21:51:32.000000000Z"/>
                </System>
                <EventData>
                  <Data Name="AppPath">C:\Users\someone\AppData\Local\DeskBox\DeskBox.exe</Data>
                  <Data Name="AppName">DeskBox.exe</Data>
                </EventData>
              </Event>
            </Events>
            """;
        var collector = new WindowsCrashEventCollector(() => Task.FromResult(xml));

        WindowsCrashEventReport report = await collector.CollectAsync(
            DeskBoxDiagnosticsBundleService.SanitizeDiagnosticsValue);

        Assert.Equal("ok", report.Status);
        WindowsCrashEventEntry entry = Assert.Single(report.Events);
        Assert.Equal("DeskBox.exe", entry.Properties["AppName"]);
    }

    [Fact]
    public void SerializeReport_UsesCamelCaseAndIndentedJson()
    {
        var report = new WindowsCrashEventReport(
            WindowsCrashEventCollector.SchemaVersion,
            "ok",
            DateTimeOffset.UtcNow,
            []);

        string json = WindowsCrashEventCollector.SerializeReport(report);

        Assert.Contains("\"schemaVersion\"", json);
        Assert.Contains("\"windowStartUtc\"", json);
        Assert.NotNull(JsonDocument.Parse(json));
    }
}
