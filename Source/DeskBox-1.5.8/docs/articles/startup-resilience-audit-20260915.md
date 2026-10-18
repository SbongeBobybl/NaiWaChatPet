# 启动韧性审计：单实例僵尸事故的同类缺口（2026-09-15）

> 只读审计，未改任何代码。工作区里已有的 3 个文件改动（`App.Tray.cs` / `App.xaml.cs` / `Helpers/Win32Helper.cs`）是当天事故的两处修复，本文把它们当作"已存在的补丁"一并审查。
> 六路并行：启动失败路径 / 单实例与多会话模型 / 窗口与 Shell API 时机危险 / 安装与更新链路 / 存活检测与自愈 / 日志与诊断保真度。行号基于当前工作区。

---

## 0. 摘要

事故链条（已定案）：登录早期 `CreateTrayIcon()` 中 `AppWindow.IsShownInSwitchers` 抛 `E_NOTIMPL` → `OnLaunched` 的 catch 只记日志 → 进程无 UI、无托盘、但持有单实例互斥锁 → 之后所有启动都被这个僵尸吞掉，安装程序也被同名进程拖住。

审计结论：这不是孤例，缺口分四类，按"离本次事故的距离"排序 ——

1. **补丁自身还有两个洞**，其中 #1 会让原症状原样复现（§1.1），#2 把"少个功能"升级成"完全打不开"（§1.2）。
2. **同一事故还有 3 个未堵的启动期入口**（就在已修的那行前后几十行内），另有 1 条路径会静默丢格子（§2）。
3. **单实例模型本身缺"健康判定"这一环**：无 ack、无探活、无逃生开关，任何"活着但没用"的实例都会吞掉一切（§3）。
4. **安装/更新链路有"报成功但没换文件"的弱点**，且因为安装器从不写日志，今天 1.5.2 为什么没装上都无法 100% 定案（§5、§6.4）。

---

## 1. 补丁自身的两个洞（最优先）

### 1.1【高 · 已确认】fatal 弹窗是模态对话框 → 无人点击时原症状原样复现

`src/DeskBox/App.xaml.cs:1147-1160` 的顺序是 `ShowFatalError(...)` → `DrainLogQueue()` → `Environment.Exit(1)`，而 `Win32Helper.ShowFatalError`（`Helpers/Win32Helper.cs:344-347`）是 `MessageBox(IntPtr.Zero, ...)`，**无 owner、无超时、阻塞直到被点击**。

在无人值守场景（登录自启失败——正是本次事故的场景，用户在别的房间或电脑前没看），进程会一直停在对话框上，**继续持有单实例互斥锁**（互斥锁只在 `ShutdownApplicationAsync` 释放，`App.xaml.cs:4349-4361`）。现象与原始事故一致：之后每次启动都是"点了没反应"，只是多了一个可能被其他窗口盖住、或不在当前虚拟桌面上的对话框。对话框文案里"请再试一次"在框未关闭前也是错的（此时重试必然无效）。

修复方向：致命路径**先保证必然退出**（后台看门狗线程 N 秒后无条件 `Environment.Exit`，弹窗只作附加提示），或先释放互斥锁再弹窗；对话框加超时（`MessageBoxTimeout` / TaskDialog 计时器）。

### 1.2【高 · 已确认（代码）/ 中（可达性）】致命化把"少个功能"升级成"完全打不开"，其中一条路径会永久砖化

新规则 = `OnLaunched` 那个大 try 内**任何**异常都会弹框退出。但那个 try 里混着大量可降级操作：

- **首启建格子失败会 rethrow**（`App.xaml.cs:2819-2835`）：
  ```csharp
  catch
  {
      // Directory creation can fail before a widget config exists. Keep
      // the setup pending in that case so a later interactive launch can
      // retry after the storage problem has been corrected.
      if (!InitialFileWidgetSetupPolicy.HasConfiguredFileWidget(settings))
      {
          settings.HasResolvedInitialFileWidgetSetup = false;
      }
      throw;
  }
  ```
  它的注释假设"下次启动还能重试"，但新规则下这次启动直接退出；而失败原因（存储不可写、OneDrive/企业策略重定向的桌面路径不可达）恰恰是用户**进不去 UI 就无法修复**的类型 → 每次启动都重复同一条路径，**永远打不开**。相比之下，修复前这里是"记日志继续跑，托盘还在，用户能进设置换存储路径自救"。
- **桌面自动整理 watcher 无论开关都会构造，构造体里有裸 IO**（`App.xaml.cs:1067-1072` → `Services/DesktopAutoOrganizationWatcher.cs:74-88` 的 `Directory.CreateDirectory(desktopPath)` + `new FileSystemWatcher(...)`）。
- **主题刷新被 await**（`App.xaml.cs:967` + `974`）：`Task.Run(themeService.RefreshAppearance)` 在线程池跑，而 `ThemeService._trackedWindows` 是普通 `List<Window>`、UI 线程会在 `TrackWindow`/`WindowClosed` 里改它（`ThemeService.cs:143-160`，`App.Tray.cs:181` 恰好在同一时刻 Track 托盘窗口）→ "集合已修改"这类异常以前只是记日志，现在会判死启动。
- 同类可降级项还有：Todo 提醒、原生通知注册、JumpList、自动备份快照、可移动盘的存储同步、托管存储桌面快捷方式、显示拓扑监听、Onboarding、诊断服务、空闲内存维护、更新检查、自启迁移（`App.xaml.cs:1048-1116` 一带）。

**建议的分界线**：应保持致命 = 单实例互斥/激活事件、`WidgetManager` 构造、托盘窗口 + 托盘图标创建（`ForceCreate` 后仍 `IsCreated == false`）、`UiDispatcherQueue` 取不到；其余一律"局部 try/catch + 记日志 + 用户可见的降级提示"。结构上把 `OnLaunched` 拆成"生命线阶段"与"功能阶段"，功能阶段失败只在末尾做一次自检（托盘未建 且 无任何可见窗口 才判致命）。

---

## 2. 同一事故还有 3 个未堵的启动期入口（+1 条静默丢格子）

已修的只有 `App.Tray.cs:130` 那一行。同一时间窗（登录后数秒）里还有：

### 2.1【高 · 已确认】`App.Tray.cs:137` —— `AppWindow.Resize` 裸调用
```csharp
        try
        {
            _trayWindow.AppWindow.IsShownInSwitchers = false;   // 130 已修
        }
        catch (Exception ex) { Log(...); }
        AppBranding.ApplyWindowIcon(_trayWindow.AppWindow);    // 136（恰好自带内部 try/catch）
        _trayWindow.AppWindow.Resize(new Windows.Graphics.SizeInt32(1, 1));  // 137 裸调用
```
同一个 `AppWindow` 上的**下一个**调用没有任何保护。`ApplyWindowIcon` 能活下来纯属巧合（它自己包了 try/catch），不是这条序列被设计成安全的。在 `OnLaunched` 的 try 内 → 新规则下就是"打不开"，异常点只是后移一行。

### 2.2【高 · 已确认】`App.Tray.cs:182` —— `_trayWindow.Activate()`
同样裸调用（`Window.Activate()` 内部走 `ShowWindow`/`SetForegroundWindow`，比 `Resize` 更贴近 shell 未就绪的触发面）。托盘图标由 `_trayIcon.ForceCreate`（`:184-189`）单独建立，不依赖该窗口可见，所以这里失败完全可以降级。

### 2.3【高 · 已确认】`Services/WidgetManager.cs:1871` —— 分组恢复分支不在 try 内
```csharp
            if (existingWindow is { Visible: true } && ...)
            {
                existingWindow.RestoreBoundsForCurrentTopology();   // 1871 裸调用
                ...
                continue;
            }
            try { await ShowGroupActiveWindowAsync(group); }        // 1879 只有这一支被保护
            catch (Exception ex) { App.Log(...); }
```
调用链全程无 catch：`RestoreBoundsForCurrentTopology`（`Views/WidgetWindowBase.Bounds.cs:579-582`）→ `ApplyWindowBounds`（`:425-491`）→ `AppWindow.MoveAndResize`（`:467/:472`），而 `ApplyWindowBounds` 用的是 **try/finally（不是 try/catch）**。这条路径**每次启动**（有可见分组时）都会走到——日志里 `[WidgetGroup] Kept visible group surface during restore` 那几行就是它。它不在 `StartupWidgetRestoreRunner` 的逐 widget catch 覆盖内。

### 2.4【中 · 已确认】`Views/WidgetWindowBase.Bounds.cs:60` + 三处弹窗宿主 —— 静默丢格子/半坏弹窗
`AppWindow.IsShownInSwitchers = false` 在 `ConfigureWindowCore()` 里，被每个 widget 窗口的构造函数调用：启动期被 `StartupWidgetRestoreRunner.RestoreOneAsync` 的逐 widget catch 兜住 → **不崩、不提示、静默少一个格子**（用户和客服都很难定位）。同类未保护的还有 `Views/StackPopoverHostWindow.cs:44`、`Views/StackPopoverInlineRenameWindow.cs:75`、`Views/WidgetDetachPlacementPreviewWindow.cs:99`（这三处在交互路径，异常被 `OnUnhandledException` 吞掉 → 弹窗半坏）。

修复方向（全部四类）：抽一个共享的降级辅助方法（例如 `TryApplyShellWindowState(AppWindow)`）把 `IsShownInSwitchers`/`Resize`/`Activate` 一次包住，失败降级为"1×1 裸窗口可能漏进 Alt+Tab"，绝不让窗口初始化中断。

---

## 3. 单实例模型：缺"健康判定"这一环

### 3.1【高 · 已确认】二级实例对"僵尸第一实例"没有任何判据
`App.xaml.cs:229-239`：`_activationEvent.Set()` 之后立即 `Environment.Exit(0)`——不等回执、不超时、不探测。而监听侧只在启动**末尾**注册（`App.xaml.cs:2151-2170` / `2202` `RegisterActivationListener()`）。于是"启动期就坏掉的第一实例"永远不注册监听 → 信号是单向黑洞。全仓库无 ping、无 ack、无 `IsHungAppWindow`、无对 peer 的 `SendMessageTimeout`、无 `--force`/`--new-instance` 逃生开关（命令行只认 `--startup*`、`--update-install-result` 和 JumpList 四个参数）。

UI 线程卡住（例如大文件夹布局几十秒）时同理：窗口"未响应"，点击无任何反馈，连点还会把激活流程排成队列串行跑。

### 3.2【高 · 已确认】开机自启静默退出 + 任务不重试
`App.xaml.cs:216-220`：`--startup` 启动遇到已有实例 → `Environment.Exit(0)`，**连 `_activationEvent.Set()` 都不走**，也不写日志（异步队列 + 硬退出，见 §6.2）。计划任务本身 `MultipleInstancesPolicy=IgnoreNew`、无 `RestartOnFailure`（`Services/DirectStartupTaskBackend.cs:388-405`）。合起来：开机时若存在任何"占位的坏实例"，用户看到的是"开机什么也没发生"，且系统层面显示任务成功。

### 3.3【高 · 已确认】开发机特有入口：Debug 与安装版共用同一把锁、同一个数据根
`Services/DeskBoxDataPathService.cs:26-28 / 34-35`：`InstanceScope` 只在设置了 `DESKBOX_DEV_DATA_ROOT` 时才派生自数据根，否则是固定常量 `7F3A9B2E`。直接双击运行的 Debug 版（不经过 `scripts/start-debug.ps1`）与 `C:\Program Files` 安装版**同名互斥、同写一份日志**。今天就是登录任务指向 Debug 路径先拿到锁（`ownsTask=True`），安装版只能当二级。对普通用户无影响，但对开发机是常态坑，也会让安装/卸载与调试实例互相纠缠。

### 3.4【中高 · 高度怀疑】商店版与直装版的 scope 与转发载荷
`Package.appxmanifest` 只有 `runFullTrust`、没有 `unvirtualizedResources` → 打包版写 `%LOCALAPPDATA%` 会被重定向到 `...\Packages\<PFN>\LocalCache\Local\DeskBox`；而内核对象名不含数据根。同会话里先跑直装版再点商店版：商店版成为二级、把通知信封/jumplist 参数写进**自己的**根，主实例读不到 → 参数丢失，只有一次空唤醒。

### 3.5【中 · 已确认】pending 参数无 TTL → 过期动作重放并抢走用户当次点击
jumplist 转发写纯文本、无时间戳（`App.xaml.cs:2057-2089`）；通知信封校验不含年龄检查（`Services/NativeNotificationActivationEnvelopeStore.cs:426-477`）。强杀/僵尸之后残留的参数会在**下一次**任意激活时优先执行，并且会 `continue` 跳过用户这一次真正的点击（细节见 `App.xaml.cs:2172-2204`）。

---

## 4. 吞异常总闸门：没有收口

`App.xaml.cs:4373-4377`：
```csharp
    private void OnUnhandledException(object sender, Microsoft.UI.Xaml.UnhandledExceptionEventArgs e)
    {
        Log($"Unhandled exception: {e.Exception}");
        e.Handled = true;
    }
```
无条件吞掉所有 UI 线程未处理异常（含 `async void` 事件处理器、`DispatcherQueue` 回调、`async void OnLaunched` 在 try 之外的部分）→ 进程带病存活、继续持锁。且全仓库没有 `TaskScheduler.UnobservedTaskException` 订阅；唯一的存活检测（Watchdog）**默认关闭**、只在 `OnLaunched` 末尾启动、只监测"UI 线程 >15s 不处理消息"、且**只写日志不恢复**（`Services/AppDiagnosticsService.cs:30-41 / 122-172`）——对"消息泵空闲的无 UI 僵尸"从原理上零检出。

值得注意的边界：`ExitApplication` / `ShutdownForRestartAsync` 是 `async void`、`ShutdownApplicationAsync`（`App.xaml.cs:4300-4371`）整链**无 try/catch**，而 `_trayIcon?.Dispose()`（`:4345`）在 `Exit()`（`:4370`）之前——中途任何一步抛异常被 `Handled = true` 吞掉，就会留下"托盘已经没了、进程还在"的形态（用户只能任务管理器）。

---

## 5. 安装 / 更新链路

### 5.1 今天 1.5.2 为什么没装上（事实 + 候选机制，未能 100% 定案）

**已确认的事实**：
- 安装树（`C:\Program Files\DeskBox`）326 个载荷文件的时间戳全是 1.5.1 构建产物（00:08–00:10），注册表 `DisplayVersion=1.5.1`、`Software\DeskBox\DirectInstall\InstallVersion=1.5.1`、`unins000.exe` 的 ProductVersion=1.5.1.0 → **1.5.2 的文件从未落盘，安装收尾阶段也从未以 1.5.2 身份执行**。
- `DeskBox_Setup_1.5.2_x64.exe` 自身版本信息正确（File 1.5.2 / Product 1.5.2.0），排除"包做错版本号"。
- 日志证明 1.5.2 安装器确实被运行过（10:05:51 拉起应用时的父进程名 `DeskBox_Setup_1.5.2_x64.tmp`），事件日志有 4 次 RestartManager 会话（10:05:19 / 10:05:51 / 10:07:24 / 10:08:08）。
- 10:07:23–10:07:36 有一次"以 1.5.1 身份"的收尾写（unins000.exe/dat + 开始菜单快捷方式 + 92 个目录 mtime 变化）。

**候选机制**（按可能性）：
1. **RM 关不掉被占用/被误判的应用**（`installer/DeskBox.iss:67-69` 只有 `CloseApplications=force` + 过滤 `DeskBox.exe`，没有 `AppMutex`）：RM 失败 → Inno 对锁定文件重试 4 次后弹错误框，用户可选 "Skip/Ignore"（`installer/Languages/English.isl:313-314`）→ 但注意：若走到"跳过文件后继续"，收尾会以 **1.5.2** 写 unins/注册表，与事实矛盾 → 所以更可能是**用户点了 Cancel/Abort 或流程中断**，什么都没变。
2. **10:07 那次是 1.5.1 安装包的一次完整运行**（可能是在 1.5.2 失败后重跑旧包"修一下"；卸载器/注册表/目录 mtime 全部与"1.5.1 安装成功"自洽）。
3. `installer/DeskBox.Installation.iss:275-285` 的"检测到 ≥2 个安装目录就拒绝运行"守卫（本机确实先后存在 `%LOCALAPPDATA%\Programs\DeskBox` 与 `C:\Program Files\DeskBox` 两套）——但该守卫在 RM 之前触发，不能解释 RM 会话。

**为什么不能定案**：安装器从不写日志（全仓无 `/LOG`，`%TEMP%` 无 Setup 日志残留，更新器日志今天无新条目）。这是 §5 里最该先补的诊断缺口。

### 5.2【高 · 已确认】"安装完成但文件没换"的弱点
- 安装脚本缺 `AppMutex` / `SetupMutex`，`[Files]` 也没用 `restartreplace`（`installer/DeskBox.iss:242-255`）→ 应用在跑时既不会被优雅拦住（"请先关闭 DeskBox"），也不会推迟到重启替换，只能走"重试→错误框→Skip/Ignore"这条会**报成功**的弱分支。
- 更新器只认退出码 + 注册表 `InstallLocation`/`InstallScope` 一致性，**不校验目标版本的文件是否真的换了**（`src/DeskBox.Updater/Program.cs:143-155 / 214-245`；应用侧 `Services/AppUpdateService.cs:145-253` 只校验下载 sha256）→ 静默更新失败可以表现为"成功"。
- 更新器 `WaitForParentExit` 90s 超时后**返回值未检查**、`WaitForExit()` 无超时（`Program.cs:81-101 / 141`）；静默安装参数没有 `/SUPPRESSMSGBOXES`（错误框仍会弹，卡住无人值守流程）。
- `DeskBox.InstallManifest.txt` 只有路径列表、无版本/哈希（`scripts/publish-aot-retail.ps1:352-371`），事后无法离线断言"文件到底换没换"。

### 5.3【中高 · 已确认】多安装目录守卫在静默模式下 = 什么都不做
`installer/DeskBox.Installation.iss:275-285` 检测到多个 DeskBox 安装目录即 `InitializeSetup` 返回 False；静默时消息被抑制（`:246-261` 只识别 `/VERYSILENT`、`/SUPPRESSMSGBOXES`）。从旧的 per-user 布局（`%LOCALAPPDATA%\Programs\DeskBox`）迁到 per-machine 的用户会遇到"双击安装包毫无反应/自动更新静默无效"。

### 5.4 商店版不受影响
商店版走 `StoreContext.RequestDownloadAndInstallStorePackageUpdatesAsync`（`Services/StoreAppUpdateService.cs:100-140`），关闭与重启由平台事务负责，不存在"跳过后报成功"的路径。

---

## 6. 日志与诊断保真度

### 6.1【中 · 已确认】日志行没有 PID、没有日期
`App.xaml.cs:666` 只有 `HH:mm:ss.fff`。加上"Debug 与安装版写同一个文件"（§3.3），今天那 13 个孤儿实例**无法归属到任何进程**。

### 6.2【已确认】"只写一行"的谜团已解释：异步日志队列 + 硬退出丢行
`Log()` 是入队 + 后台线程写盘（`App.xaml.cs:664-681 / 719-751`），而 `Environment.Exit`（`:219 / :239`）不等队列、**也不调用 `DrainLogQueue()`**（全仓库只有 worker 和新增的 fatal 路径会调用）。实测：`[Activation]` 与 `Another instance...` 两行是否落盘，取决于两次 `Log()` 的间隔（25–40 ms）与后台线程首次 drain 的赛跑——两行同时入队就被一次写盘带出，否则第二行永久丢失。丢失是"至少一行"，所以 10:02–10:08 的真实启动次数 ≥ 16 次（不是 13）。

### 6.3【中】其余保真度缺口
- `DrainLogQueue` 是**先出队后写**、异常全吞（`:728-751`）→ 任何 IOException（双写冲突、磁盘满）会静默丢掉整批已出队的行。
- 轮转只保留一代（`DeskBox.log.1`），且"先 Delete 再 Move"，Move 失败会连上一代一起丢（`:753-775`）。
- `MaxQueuedLogLines = 4096` 溢出静默丢弃，无计数无告警（`:668-676`）。
- 新增 fatal 路径里的 `DrainLogQueue()` 从 UI 线程调用，与 worker 存在双写风险（`File.AppendAllText` 共享冲突 → 被 catch 吞掉 → 丢批）。正常路径下通常无害（前面有模态框，队列多半已排空），但设计上应让 worker 成为唯一写者，或 `DrainLogQueue` 失败重入队。

### 6.4【中】诊断包在僵尸状态下不可达
导出入口只在设置页（`Views/SettingsWindow.Maintenance.cs:23-43`，需要 XAML + 文件夹选择器），无命令行开关；包内日志是 2 MB 尾部（`Services/DeskBoxDiagnosticsBundleService.cs:160 / 288-326`）。今天的僵尸既无托盘也无窗口，用户**无法导出任何诊断包**。

---

## 7. 明确"不是缺陷"的清单（避免误伤）

1. 二级实例不显示 UI、只转发后退出：刻意设计（避免多进程抢托盘/全局热键）。问题在"primary 不健康时没有退路"，不在"二级不起 UI"。
2. `--startup` 分支不 `Set()` 事件：刻意不打扰用户。缺的是健康检查，不是这个意图。
3. 命名 Mutex 在进程退出后由内核回收，DeskBox 从不 `WaitOne` 它 → **不存在残留锁/遗弃互斥问题**；真正会残留的是文件型 pending 状态（§3.5）。
4. 已做对的部分（审计确认良好）：显示拓扑变化（协调器 try/catch + 最多 8 次重试 + 二次校验 + 逐窗口 catch）、WTS 锁屏/解锁/RDP（只排队恢复、不在无 shell 状态动窗口）、Explorer 重启（三处监听 + try/catch）、`DwmSetWindowAttribute` 的 OS build 门控、`SHChangeNotify`/`RegisterHotKey`/`WTSRegisterSessionNotification` 均有保护、崩溃后的整理事务恢复（`RecoverPendingAsync`）与备份回滚链路完整。
5. `RestartAgent.exe` 是 Windows App SDK 运行时文件，**不是** DeskBox 的自愈机制（全仓唯一引用在旧清单里）；真正干活的重启器是 `DeskBox.Updater.exe --restart-only`。
6. `restore-staging` / `restore-rollback` 与更新无关（备份恢复用），本机为空目录、无 marker，不影响启动。
7. 开发机特有（非产品缺陷）：登录任务指向 Debug 构建、两套安装并存、今天多次手工跑测试包。

---

## 8. 建议优先级（仅排序，不含代码）

| 优先级 | 项 | 理由 |
| --- | --- | --- |
| P0 | §1.1 fatal 路径必须先保证退出 | 否则补丁只把"无提示的僵尸"换成"有提示的僵尸" |
| P0 | §1.2 收窄致命边界（首启建格子、watchdog 式 IO、主题刷新摘出致命通道） | 唯一能把"少个功能"永久升级成"打不开"的路径 |
| P0 | §2.1–2.3 三个启动期入口收敛到共享降级原语 | 与已修的那行同一时间窗、同一失败面 |
| P1 | §3.1 逃生出口（`--restart`/`--new-instance`，或"第二实例收不到 ack 时提示用户接管"） | 让"点了没反应"有出口，代价最低 |
| P1 | §4 `OnUnhandledException` 分层（生命线未建立时不得吞） | 吞异常总闸门，僵尸的另一条产线 |
| P1 | §5.2 安装后版本校验 + §5.3 静默模式守卫修好 | 影响更新可信度与老用户升级路径 |
| P2 | §3.5 TTL、§6.1 日志加 PID/日期、§6.2 Exit 前 flush、§5.1 给安装器加 `/LOG` | 下一次同类事故能不能 10 分钟定案，全看这些 |

---

## 9. 实现状态（2026-09-15，未提交）

复核阶段另派一路独立 agent 反查本计划，又补进 4 类缺口：**启动挂起（无异常）没有兜底**、**退出路径同样能留下"托盘没了还持锁"的进程**、**自检判据会把快速显示/慢启动误判成致命**、**裸调用收敛范围只有实际同类调用的一半**。以下实现已包含这些补强。

已实现（P0 全部 + §4）：
- **P0-1 致命路径**（新文件 `src/DeskBox/App.Startup.cs`）：`FailStartup` 带重入门（MessageBox 有自己的消息泵，会重入）＋专属线程 20s 兜底退出＋原生弹窗＋`DrainLogQueue()`＋`Environment.Exit(1)`。**不提前释放互斥锁**（复核结论：提前释放会让第二个实例与仍在写日志/设置的旧实例并存，交由内核在退出时回收）。
- **P0-2 收窄致命边界**：`OnLaunched` 拆成"生命线"（DispatcherQueue、`SettingsService.LoadAsync`、Theme/Localization 解析、`CreateTrayIcon`、`new WidgetManager`）与"功能"（其余 30+ 步全部走 `RunOptionalStartupStep(Async)` 逐条降级，失败只记 `[Startup] Optional step '<名称>' failed`）。首启建格子的 `throw;` 保留在方法内、由调用点降级（pending 标记仍留给下次启动重试）。末尾 `EnsureStartupProducedUsableSurface()`：托盘不可用且 `WidgetManager.LoadedWidgetCount == 0` 才判致命（用"已加载"而不是"可见"，避免误杀快速显示/隐藏格子）。
- **启动看门狗**（覆盖挂起）：`StartStartupWatchdog()` 在 `try` 之前武装，专属线程（不用线程池：启动期快照 zip/schtasks/WMI 会占满）90s 未见生命线即 `FailStartup`；生命线达成后线程自行退出。
- **P0-3 裸调用收敛**：`App.Tray.cs` 的 `Resize`/`Activate` 包降级；`WidgetManager.cs:1871` 的 `RestoreBoundsForCurrentTopology` 包 try/catch；新增 `Helpers/WindowShellState.cs`（`TryHideFromSwitchers` + `TryApplyBorderlessOverlappedPresenter`），5 处 `IsShownInSwitchers` 与 5 处 `SetPresenter`/presenter 配置全部收敛过来（契约测试断言"全仓只有这一个文件碰这两个 API"）；`WidgetWindowBase` 的 `DisplayArea.GetFromPoint` 加空值回退（回退到配置坐标而不是 NRE 掉整个格子）。
- **§4 分层与收口**：`OnUnhandledException` 在生命线未建立时走 `FailStartup`（建立后维持原来的吞异常策略）；`ShutdownApplicationAsync` 改 try/catch/finally，`Exit()` 无条件执行；`JumpList` 的 configure/activation、延迟桌面层初始化、外部激活处理、Shell 菜单预热全部改成可观测的 `SafeFireAndForget`/带 catch。

测试：新增 `tests/DeskBox.Tests/StartupResilienceContractTests.cs`（6 个契约测试：致命路径必退且不提前释放锁、看门狗专属线程且在 try 之前、功能步骤降级且失败不再短路、生命线用"已加载格子"判据、WindowShellState 唯一性、未处理异常与退出路径分层）；4 个既有契约测试更新到新形状（断言意图不变）。`dotnet test -p:Platform=x64` **3545/3545 绿**；Debug 构建启动验证通过（`OnLaunched completed successfully`，第二实例行为不变，90s 看门狗无误杀）。

未做（留待 Simon 拍板/下一批）：
- 安装/更新链路：`AppMutex`/`SetupMutex`/`restartreplace`、安装后版本校验、静默模式下多安装目录守卫、给 setup 传 `/LOG`（§5.1–5.3）。
- 登录任务的 `RestartOnFailure`（让致命退出能在开机时自动重试，§5 建议 5）。
- 逃生出口 / 第二实例接管（§3.1）：涉及"弹框询问是否结束无响应的实例"，属新 UI 机制，等拍板。
- pending 参数 TTL（§3.5）、日志加 PID/日期与 Exit 前 flush（§6.1–6.2）。
- 更广的裸调用收敛（`GetFromWindowId` 一族、`MoveAndResize`、popover 的停车式定位）与 `ThemeService` 跨线程 `List<Window>`（§2.5/§6 评测项）。
