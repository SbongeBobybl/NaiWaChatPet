namespace DeskBox.Services;

public sealed partial class WidgetManager
{
    internal void CloseUncommittedOrganizationWindows(IEnumerable<string> ids)
    {
        foreach (string id in ids)
        {
            _suppressClosedVisibilityPersistence.Add(id);
            if (_fileWidgets.TryGetValue(id,out var session))
                _fileSessionRegistration.UnregisterIfMatch(id,session);
            if (!_contentWidgets.TryGetValue(id,out var window)) continue;
            _contentWindowRegistration.UnregisterIfMatch(id,window);
            try { window.Close(); }
            catch (Exception ex) { App.Log($"[NaiWa] Closing uncommitted target failed: {ex.Message}"); }
        }
    }
}
