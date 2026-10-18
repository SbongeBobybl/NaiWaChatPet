# 设置窗口主题不跟随系统：根因分析与修复方案

- 日期：2026-09-20
- 症状：DeskBox 设置为跟随系统主题时，系统明暗切换后格子正常切换，设置页面停留在深色
- 结论：**设置窗口的"关闭即隐藏复用"与 `ThemeService` 的"Closed 即注销"语义冲突**。窗口被用户关闭（实际隐藏）后即被移出主题跟踪列表，`RequestedTheme` 冻结在最后一次应用的值。属 `0ea2ddbe`（2026-09-01，perf: fix window/material leaks）引入隐藏复用后的回归。
- 状态：写本文时只做了分析、未动代码；§4 的 P0–P3 方案**已随后续批次全部落地**（`SettingsWindow` 迁 `AppWindow.Closing`+Cancel 门闸、`WindowTrackingRegistry` 弱引用登记等）。下文行号锚点基于实现前的代码，阅读时以当前源码为准。

---

## 1. 现象与复现路径

精确表述：**设置窗口经历过一次"关闭"（实为隐藏）后，主题冻结在关闭前的值**。

典型复现：
1. 系统深色期间打开过设置窗口，关闭它（实际只是隐藏）；
2. 系统切到浅色 → 格子变浅，托盘图标刷新；
3. 打开设置 → 仍是深色（窗口与标题栏按钮颜色一致地卡在深色）。

重启应用后第一次打开设置是正常的（构造时 `TrackWindow` 会按当前系统主题应用），这也解释了"始终深色"的观感——只要上次关闭时系统是深色，之后无论系统怎么切，打开都是深色。

## 2. 根因链路（代码证据）

### 2.1 冲突的两端

**ThemeService 端**：`TrackWindow` 订阅 `window.Closed += OnTrackedWindowClosed`（[ThemeService.cs:158](../../src/DeskBox/Services/ThemeService.cs#L158)）。回调**不检查 `args.Handled`**，一进 Closed 就把窗口从 `_trackedWindows` 移除并退订（[ThemeService.cs:161-170](../../src/DeskBox/Services/ThemeService.cs#L161)）。该语义自 `c32265eb` 起如此——当时所有窗口都是真关闭，没有问题。

**SettingsWindow 端**：`0ea2ddbe` 为修设置树泄漏，把关闭改成"取消关闭 + 隐藏复用"——用户点 X 时 `SettingsWindow_Closed` 设 `args.Handled = true` 然后 `_appWindow.Hide()`，实例保留整进程生命周期（[SettingsWindow.xaml.cs:305-320](../../src/DeskBox/Views/SettingsWindow.xaml.cs#L305)）。此改动未同步 ThemeService 的注销语义。

### 2.2 触发时序

`Window.Closed` 是多播事件，按订阅顺序调用。构造函数里 `TrackWindow(this)`（197 行）先订阅、`Closed += SettingsWindow_Closed`（220 行）后订阅：

1. 用户点关闭 → `OnTrackedWindowClosed` 先执行（无条件移除出跟踪列表，此刻 `args.Handled` 还是 false）；
2. `SettingsWindow_Closed` 后执行（设 `Handled = true`，隐藏窗口）。

无论订阅顺序谁先谁后移除都会发生，因为 ThemeService 根本不看 Handled 标志。

### 2.3 之后的一切

- 系统切换 → `UISettings.ColorValuesChanged` → 防抖 200ms → `RefreshAppearance()` → `ApplyToAllWindows()` 遍历 `_trackedWindows`（[ThemeService.cs:213-218](../../src/DeskBox/Services/ThemeService.cs#L213)）——列表已无设置窗口，`SettingsRoot.RequestedTheme` 不再被赋值；
- 重新打开 → `ShowWindow → RefreshOnReopen` 只刷新功能格子列表/页面数据/响应式布局（[SettingsWindow.xaml.cs:268-279](../../src/DeskBox/Views/SettingsWindow.xaml.cs#L268)），既不重新 Track 也不 `ApplyToWindow`；`App.OpenSettings`（[App.xaml.cs:2885-2890](../../src/DeskBox/App.xaml.cs#L2885)）同样不做主题重应用；
- 隐藏期间 `AppearanceChanged` 订阅仍存活（只在真关闭解除），但 `OnAppearanceChanged` 只刷标题栏按钮颜色和格子列表，不应用主题；且 `IsEffectiveSettingsThemeDark()` 读的 `SettingsRoot.ActualTheme` 是冻结值——连标题栏也内外一致地卡深色。

### 2.4 为什么格子正常

格子窗口（`WidgetWindowBase`，由 `WidgetManager` Track，[WidgetManager.cs:2636](../../src/DeskBox/Services/WidgetManager.cs#L2636)）常驻桌面、从不走"用户关闭→隐藏"路径，一直在跟踪列表里，每次系统切换都被重新赋值 `RequestedTheme = EffectiveTheme`；其内容控件靠 `ActualThemeChanged` 联动刷新。

### 2.5 排除项

- `SettingsWindow.xaml` / `App.xaml` 均无 `RequestedTheme` 硬编码（全仓库 XAML grep 为空）；
- `MicaBackdrop` 未设固定 `Theme` 属性，跟随 root 的 `ActualTheme`，不是成因；
- backdrop、标题栏、内容三者颜色统一卡深色，正是因为源头都是那个没被更新的 `RequestedTheme`；
- 其他 tracked 窗口不受影响：`DesktopOrganizationWindow`、`ReleaseNotesWindow`、Tray、Onboarding 都是真关闭，注销行为正确；`SearchPopupWindow` 等未走 TrackWindow，自管主题。

## 3. 平台层核实（2026-09-20 网络调研）

| # | 事实 | 对本问题的意义 |
|---|---|---|
| 1 | 官方取消关闭的契约是 **`Window.Closing`（`WindowClosingEventArgs.Cancel`，WinAppSDK 1.4+ 加入）或更底层的 `AppWindow.Closing`（`AppWindowClosingEventArgs.Cancel`）**。Uno 文档、microsoft-ui-xaml issue、Microsoft Q&A 一致指向此模式 | DeskBox 现用的 `Window.Closed + args.Handled = true` 是**未承诺行为**：`WindowEventArgs.Handled` 文档只写 "Gets or sets whether a Window event was handled"，未承诺在 Closed 上取消销毁。实践中在 packaged + WinAppSDK 1.x+ 生效（DeskBox 复用确实在工作），但存在确认场景下崩溃的 issue 报告 |
| 2 | `UISettings.ColorValuesChanged` 有**已知的不触发 bug**（microsoft-ui-xaml #9372，2024-02），社区 workaround 为 `WM_SETTINGCHANGE` 消息钩子 | DeskBox 只靠这一个信号驱动系统主题跟随 → "显示时自愈"（P2）有真实兜底价值，不是纯防御 |
| 3 | `ColorValuesChanged` 在**后台线程**触发，必须 marshal 回 UI 线程 | DeskBox 已正确处理（`App.UiDispatcherQueue.TryEnqueue`，[ThemeService.cs:32](../../src/DeskBox/Services/ThemeService.cs#L32)），无需改动 |
| 4 | WinUI Gallery 官方 `ThemeService` 模式 = root element `RequestedTheme` + `ActualThemeChanged` + 持久化 | DeskBox 架构方向与官方一致（且更强：多窗口 Track + `EffectiveTheme` 解析），问题只在注销时机 |
| 5 | 已知平台 bug：运行时切主题且窗口无 backdrop 时可能表现异常（microsoft-ui-xaml issue，编号未核实到原文） | 风险备注：DeskBox 格子多为 Solid 无 backdrop，但实测切换正常，暂不行动 |

仓库内先例：`DesktopOrganizationWindow` 已经在用 `AppWindow.Closing + args.Cancel + _allowClose` 门闸模式（[DesktopOrganizationWindow.xaml.cs:67,146-161](../../src/DeskBox/Views/DesktopOrganizationWindow.xaml.cs#L146)）——迁移有成熟的同仓样板。项目使用 WinAppSDK 2.4.0，`Window.Closing` 可用。

> 可信度标注：#1/#3/#4 来自官方文档与多个独立来源交叉；#2 来自 GitHub issue 标题与摘要；#5 两次抓取原文超时，仅凭搜索摘要，行动前需复核。

## 4. 修复方案（分层，按优先级）

### P0 核心修复：把"假关闭"从 `Closed` 语义中移出去

**SettingsWindow 的隐藏复用迁移到 `AppWindow.Closing + args.Cancel = true`**（照抄 DesktopOrganizationWindow 先例）：

```csharp
// 构造函数
_appWindow.Closing += SettingsWindow_AppWindowClosing;

private void SettingsWindow_AppWindowClosing(AppWindow sender, AppWindowClosingEventArgs args)
{
    if (_allowRealClose)
    {
        return; // 真关闭（CloseForShutdown）放行
    }

    args.Cancel = true;              // 官方契约：取消销毁
    _appWindow.Hide();
    App.Current.WidgetManager?.ReleaseRaisedBandGuest(_hWnd, "settings-hidden");
    // App 侧原 ClosedForApp 的隐藏分支逻辑（如 ScheduleBackgroundMemoryCleanup）迁至此处
}
```

- `Window.Closed` 回归纯"真关闭"语义 → **ThemeService 的"Closed 即注销"现有语义自动正确，零适配改动**；
- 同时消除 `Closed + Handled` 未承诺行为自身的崩溃风险（§3 #1）；
- 这是比"让 ThemeService 适配假关闭"更本质的修法：不是服务迁就 bug，而是让假关闭不再伪装成真关闭。

### P1 防御与锁定（与 P0 同批提交）

**弱引用簿记**：`List<Window>` → `List<WeakReference<Window>>`，`ApplyToAllWindows` 遍历时 `TryGetTarget` 失败即顺手清扫；读写默认由 UI 线程执行，越线只记日志不阻断（实现见 `ThemeService.EnsureUiThread`）。P0 之后注销路径已正确，这层是结构性保险——**ThemeService 永不依赖“逻辑写对了”才不泄漏，而是引用根本拖不住任何窗口**。

**契约测试**：把簿记抽成纯逻辑类 `WindowTrackingRegistry`（Track/Untrack/OnWindowClosed/EnumerateAlive 含清扫），锁定：
- Track 幂等（重复登记不增条目）；
- **本次回归用例**：假关闭（Cancel 路径，Closed 不触发）后窗口仍在册、仍被 `RefreshAppearance` 覆盖；
- 真关闭后移除；`UntrackWindow` 幂等；
- 弱引用自愈（丢强引用 + 强制 GC 后不再枚举出）。

### P2 自愈（独立小改，随时可做）

- **显示时校准**：`ShowWindow`/`RefreshOnReopen` 补一次 `ThemeService.ApplyToWindow(this)`。兜住 `ColorValuesChanged` 不触发的已知 bug（§3 #2）及其他任何刷新链路断裂——用户可见态永远正确；
- **派生外观锚定真值**：`SettingsRoot.ActualThemeChanged` 驱动 `ApplyTitleBarButtonColors` 等派生颜色（照抄 DesktopOrganizationWindow 的 `AppTitleBar_ActualThemeChanged` 模式）。现状 `OnAppearanceChanged` 触发时 `ActualTheme` 未必已更新，是又一个隐藏的时序依赖，顺手一并消除。

### P3 可观测与可选兜底

- Track/Untrack/RefreshAppearance 各打一行 verbose 日志（窗口类型 + 动作 + 计数）——本次排查若有日志五分钟定位；
- `OnColorValuesChanged` 里 `App.UiDispatcherQueue` 为 null 时静默丢弃，补日志；
- 可选：`WM_SETTINGCHANGE` 兜底监听（仅当真机复现 #9372；DeskBox 已有 subclass/消息泵基建，不急做）。

### 刻意不做

- **不引入 `IThemeAwareWindow` 之类窗口接口**：Track+Apply 模型对几十个窗口工作良好，问题只在注销时机，接口化收益低、改动面大；
- **不改弱事件模式**：弱引用簿记已拿到同等安全收益；
- **不做 `TrackWindow` lifetime 枚举**（原备选 A2）：P0 落地后"复用型窗口"不再需要向 ThemeService 声明特殊生命周期；若 P0 被否决，A2 是最佳备选（见 §5）。

## 5. 备选方案对比

| 方案 | 内容 | 优点 | 缺点 | 结论 |
|---|---|---|---|---|
| **P0（推荐）** | 隐藏复用迁移 `Closing + Cancel` | 官方契约；ThemeService 零适配；顺带消除未承诺行为的崩溃风险 | 动关闭链路 3 处 + App 侧 1 处，ripple 面最大但都明确 | ✅ 采用 |
| A1 | `OnTrackedWindowClosed` 检查 `Handled` + 调整订阅顺序 | 一两行 | 依赖未承诺行为；引入"复用窗口必须先订阅自己的 Closed"隐式契约；ThemeService 的 handler 先跑时 `Handled` 恒为 false，写错即静默失效 | ❌ 脆弱 |
| A2 | `TrackWindow` 加 `TrackedWindowLifetime` 枚举 + 显式 `UntrackWindow` | 显式契约，编译器可见 | 仍是给未承诺行为做适配；每个复用窗口多一条义务 | 备选（P0 被否决时） |
| B | `RefreshOnReopen`/`ShowWindow` 补 `ApplyToWindow` | 最小正确性修复 | 不修根因；隐藏期间 `ActualTheme` 滞后，标题栏颜色联动慢一拍 | ❌ 仅作 P2 的组成部分 |

## 6. 内存安全性论证（P0 + P1 之后）

- **实例数不增长**：隐藏复用窗口全进程单例，`App._settingsWindow` 强持有；`OpenSettings` 的 `?? CreateSettingsWindow()` 只创建一次；`TrackWindow` 有 `Contains` 短路，不重复登记、不重复订阅。
- **刷新幂等无累积**：`ApplyToWindow` 只做 `RequestedTheme` 赋值（无分配）、`AccentResourceScope.Apply`（固定覆盖同组 6 个资源 key，已存在时只改 `brush.Color`，[AccentResourceScope.cs:13-42](../../src/DeskBox/Helpers/AccentResourceScope.cs#L13)）、`SetWindowTheme`（DWM 属性）。多次系统切换内存平稳——格子窗口现状即此跑法。
- **与 `0ea2ddbe` 防泄漏目标不冲突**：那次防的是"真关闭窗口的 XAML 树被原生引用拖住、每次开关泄漏一棵树"。P0 之后保留在列表里的是从未真关闭的复用窗口（树本来就活着）；真关闭路径仍正常注销。
- **结构性保险**：弱引用簿记下，即使未来任何注销路径出 bug，ThemeService 也拖不住窗口——内存安全不依赖逻辑正确性。

## 7. 验证矩阵

| 场景 | 预期 |
|---|---|
| 设置窗开着，系统明暗切换 | 窗口与格子同步切换（含标题栏按钮颜色） |
| 设置窗关闭(隐藏)后系统切换，再打开 | 打开即正确主题 |
| 手动切 Light/Dark/System 三态 | 设置窗即时正确（三条路径都收敛到 RefreshAppearance） |
| 系统切换 N 次往返 | 内存平稳 |
| 设置窗开关 N 次 | 单实例、无树泄漏（对照 `0ea2ddbe` 的验收曲线） |
| 应用退出（CloseForShutdown） | Closing 门闸放行、Closed 清理完整执行、注销日志出现 |
| 模拟 ColorValuesChanged 丢失（如断掉事件）后打开设置 | 显示时校准兜底，主题正确 |
| 测试套件 | 新增契约测试全绿；按 AGENTS.md 用 `-p:Platform=x64` 跑 |

## 8. P0 迁移动手清单（ripple 面）

1. `SettingsWindow` 构造函数：`_appWindow.Closing += SettingsWindow_AppWindowClosing`（新增）；
2. 新增 Closing handler：`_allowRealClose` 门闸 + `args.Cancel = true` + `Hide()` + `ReleaseRaisedBandGuest`（逻辑搬自现 `SettingsWindow_Closed` 隐藏分支）；
3. `SettingsWindow_Closed`：删隐藏分支，只留 `_isClosed = true` 起的完整清理（真关闭时才执行）；
4. App 侧 `SettingsWindow_ClosedForApp`（[App.xaml.cs:2899-2907](../../src/DeskBox/App.xaml.cs#L2899)）：`args.Handled` 检查分支随迁移失效，`ScheduleBackgroundMemoryCleanup("settings-hidden")` 移入 Closing 路径；真关闭分支保留；
5. 检查 `ShowWindow` 的 `_isClosed` guard、`IsVisibleToUser`、subclass `WM_NcDestroy` 路径不受影响（均为真关闭语义，无假关闭依赖）。

## 参考资料

- [Window.Closed Event (Microsoft.UI.Xaml) — Microsoft Learn](https://learn.microsoft.com/en-us/windows/windows-app-sdk/api/winrt/microsoft.ui.xaml.window.closed)
- [WindowEventArgs.Handled Property — Microsoft Learn](https://learn.microsoft.com/en-us/windows/windows-app-sdk/api/winrt/microsoft.ui.xaml.windoweventargs.handled)
- [UISettings.ColorValuesChanged not fired — microsoft-ui-xaml #9372](https://github.com/microsoft/microsoft-ui-xaml/issues/9372)
- [Preventing Window Closing — Uno Platform docs](https://platform.uno/docs/)
- [Window.Closing support tracking — microsoft-ui-xaml issues](https://github.com/microsoft/microsoft-ui-xaml/issues?q=is%3Aissue+Window.Closing)（早期无 `Window.Closing`，1.4+ 补齐）
- [FrameworkElement.RequestedTheme — Microsoft Learn](https://learn.microsoft.com/en-us/windows/windows-app-sdk/api/winrt/microsoft.ui.xaml.frameworkelement.requestedtheme)
- WinUI Gallery `ThemeService` 模式（经 Context7 摘要转述）；albertakhmetov.com 主题切换文章（原文 404，仅搜索摘要）
- 仓库内先例：`DesktopOrganizationWindow` 的 `AppWindow.Closing` 门闸（`src/DeskBox/Views/DesktopOrganizationWindow.xaml.cs:146`）
