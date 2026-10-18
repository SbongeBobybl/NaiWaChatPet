# 文件格子的原生右键菜单：宿主架构与修复批次

- 日期：2026-09-12（两轮用户反馈定位 + 备选方案调研定案后成文；同日追加菜单关闭修复，见 §4.1）
- 状态：**修复批次已实现（P0+P1），待真机矩阵验证**
- 功能：设置 → 文件格子 → "原生右键菜单"（`FileItemSystemContextMenuEnabled`，默认关）。开启后右键格子里/叠放弹层内的文件条目，弹 Windows 原生 Shell 右键菜单（含第三方处理器）。

## 0. 定案摘要

**保留"进程外宿主经典 IContextMenu"架构，改为常驻 server 进程，并修复四类缺陷。** 不迁移到任何其他方案：

| 备选 | 结论 | 理由 |
|---|---|---|
| 进程内宿主 HMENU | **拒绝**（既有契约测试禁止） | 第三方处理器 DLL 加载进主进程，崩了整个 App 陪葬（Files #17535：Locale Emulator/Dropbox/Box 一崩全崩） |
| Files 式转绘 XAML 菜单 | 拒绝（可作远期视觉融合备选） | 同样进程内加载处理器；子菜单懒加载/图标抠取工程量大且有"卡加载"系列 issue |
| 内嵌 IExplorerBrowser 真视图 | 拒绝 | 放弃整个格子 UI、HWND airspace、不支持暗色；且 Win11 上拿到的仍是经典菜单 |
| 注册表 {86ca1aa0} 强制经典菜单 | **红线，永不执行** | 用户级全局 hack，改的是整台机器 Explorer 行为，不是 API |
| 等待 Win11 新菜单宿主 API | 不可行（已验证缺席） | 新菜单是 twinui.pcshell 内部 CommandBarFlyout，微软只开放"提供方"（IExplorerCommand）方向；Files/SO 均撞墙（E_NOINTERFACE / REGDB_E_CLASSNOTREGISTERED） |

支撑证据：Directory Opus 13（2023-12）新增"尽可能进程外访问上下文菜单"选项，官方文档原话"上下文菜单扩展是最容易出问题的 shell 扩展"——与 DeskBox 架构趋同进化；微软 Win11 博客自认"大量命令在 Explorer 进程内运行造成性能与可靠性问题"。性能手段（预热/池化/懒加载/动词直调）见 §4。

## 1. 用户反馈与根因（2026-09-12 定位，均已复核代码证据）

反馈 A：开启后原生菜单"无法正确渲染"（格内自定义菜单正常）。
反馈 B：原生菜单"特别慢"，最终能显示，但点任何项都没反应。

| # | 根因 | 证据 | 归属症状 |
|---|---|---|---|
| R1 | 代理进程**零 DPI 声明**（无 manifest、无 API 调用），主程序 PerMonitorV2 传物理像素 → 缩放≠100% 时菜单位置错 + DWM 位图拉伸模糊/裁切；100% 缩放不复现 | `native/deskbox-thumbnail-proxy/` 全目录无 DPI 痕迹；`main.rs` TrackPopupMenuEx 直接消费 `screen_x/y` | A（渲染坏，最高嫌疑） |
| R2 | 菜单 owner 是**非置顶**隐藏窗口，叠放弹层宿主**永久 WS_EX_TOPMOST** 且菜单期间被刻意保活 → 菜单弹在弹层后面 | `StackPopoverHostWindow.cs:63-128`；`SelectionAndMenus.cs:494-501` | A（弹层路径确定性遮挡） |
| N1 | `InvokeCommand` 返回后**立即 process::exit**，无消息泵/宽限 → post-message 完成、以 owner 为父的异步 UI、STA 回调类动词被杀在半路 | `main.rs:536→854` | B（点击无反应，头号） |
| N2 | 裸 ANSI `CMINVOKECOMMANDINFO`（`fMask:0`，无 UNICODE/PTINVOKE/lpDirectory）→ 部分处理器退化或拒绝；失败只映射为小反馈气泡 | `main.rs:523-533` | B（次要） |
| S1 | **每次右键冷启动进程** + `CMF_NORMAL` 同步加载全部第三方处理器（OneDrive/杀软/TortoiseGit）后才打 ready；无任何计时日志 | `ShellContextMenuProxy.cs:52-89`；`main.rs:474-500` | B（慢，结构性） |
| S2 | stdout 只读一行不排空；无单飞守卫；父进程死亡后代进程无自超时滞留 | `ShellContextMenuProxy.cs:87-89` | 次生隐患 |

排除项（做对了的，别动）：`WM_INITMENUPOPUP/WM_MEASUREITEM/WM_DRAWITEM/WM_MENUCHAR` → `HandleMenuMsg2/HandleMenuMsg` 转发；`SetForegroundWindow`+`WM_NULL`；`lpVerb = MAKEINTRESOURCE(id - idCmdFirst)` 偏移；STA；进程隔离契约。

## 2. 本机状态审计结论（2026-09-12 全仓核查）

**本功能不可能改坏系统级右键菜单。** 全仓无 shell 扩展/上下文菜单处理器/图标 overlay 注册（MSIX 清单只有 startupTask + toast comServer，由部署栈管理）；第三方处理器只加载进一次性代理进程；无全局钩子残留（键盘/鼠标钩子属另外两个开关功能且有无条件 unhook）。上下文菜单"仅显示"路径零写入（无注册表/文件系统/剪贴板/MRU 变更）。会碰用户机器的只有预期内动作：用户点击的动词本身、剪切/复制写剪贴板、拖拽导入时对自己物化的临时文件写 Zone.Identifier（不涉本路径）、HKCU Run 自启动开关、快速访问固定自家目录。

## 3. 修复批次内容（本批已实现）

| 级别 | 修复 | 实现点 |
|---|---|---|
| P0 | 代理进程启动即声明 `PER_MONITOR_AWARE_V2`（上下文菜单/服务器路径，不动缩略图模式） | Rust `apply_per_monitor_dpi_awareness()` |
| P0 | `InvokeCommand` 换 `CMINVOKECOMMANDINFOEX` + `CMIC_MASK_UNICODE \| CMIC_MASK_PTINVOKE`（+Shift/Ctrl 按键状态位），`lpVerb/lpVerbW` 同偏移，`lpDirectory/lpDirectoryW` 填父目录（"Git Bash Here"类必需） | Rust `run_menu_on_owner` |
| P0 | Invoke 后进入**消息泵宽限期**（PeekMessage 循环 2s）再退出/应答，让 post-message 完成类动词跑完 | Rust `pump_messages()` |
| P1 | owner 窗口 `WS_EX_TOPMOST`，菜单进 TOPMOST 带，不再被叠放弹层盖住 | Rust `create_context_menu_window` |
| P1 | **常驻 server 模式**：一个 STA 公寓跨多次右键复用（处理器 DLL 只加载一次），50 次菜单后退役、连续 3 次失败退役；stdin EOF（父进程退出）自清理，根除滞留 | Rust `run_context_menu_server` |
| P1 | C# 侧改 server 管理：单飞守卫（菜单打开期间忽略后续右键）、spawn→ready / menu→shown / shown→result 三段预算（15s/15s/10min）、传输层失败自动重启重试一次（仅限菜单未显示阶段）、stderr 持续排空（防子进程管道堵塞）、每轮计时日志（**无条件**写入 `App.Log`，这是"菜单弹出但无效果"的唯一痕迹） | `ShellContextMenuProxy.cs` 重写 |
| P1 | **单发兜底**：server 轮在"菜单尚未显示"阶段失败时，自动回落一次自包含的单发轮（`--context-menu`），用户总能拿到菜单；两条路径共用同一原生构建代码与契约测试 | `TryShowOneShotAsync` |
| P1 | **菜单关闭**：`cancel` 命令 + 读取线程 post ESC（见 §4.1）；点击收口在格子窗口/叠放弹层窗口的 WndProc 子类；右键其他条目=关旧开新（`s_pendingRequest`）；server 模式 invoke 后立即回 `result` | Rust `dismiss_open_menu` + `CancelOpenMenu` |
| P1 | **子菜单不再为空**：去掉 `TPM_NONOTIFY`（见 §4.2） | Rust `track_flags` |
| P1 | **菜单跟随明暗主题**：代理进程通过 uxtheme 序号 opt-in（见 §4.3），DeskBox 每轮传主题 | Rust `apply_menu_theme` + C# `MenuThemeToken` |
| P1 | **预热**：启动后/开关开启时后台拉起 server 并对 `C:\` 做一次 throwaway QueryContextMenu（Files 同款）。实测有效：同一路径真实构建 2343ms（冷）→ 299ms（预热后） | Rust `warmup` 命令 + C# `Prewarm()` |
| — | 单发模式 `--context-menu <path> <x> <y>` 保留（同等修复；契约测试直接覆盖该 CLI） | 兼容 |

## 4. 协议规范（server 模式 v2）

- 启动：`DeskBox.ThumbnailProxy.exe --context-menu-server`，stdin/stdout 均 UTF-8 文本行协议；stderr 为诊断流（父端持续排空）。
- 子端就绪即输出 `ready`。父端命令：
  - `menu\t<x>\t<y>\t<path>`（路径可为任意含空格串；制表符分隔，路径取第 3 个制表符后的全部）
  - `warmup`（对 `C:\` 建 + 弃一次菜单，应答 `warm ok` / `warm err`）
  - `cancel`（关闭当前打开的菜单；见 §4.1）
- 子端应答：`shown`（QueryContextMenu 完成即将弹菜单）、`result <0|2|3>`（与单发退出码同义）、`bye`（即将退役退出）、未知行忽略（第三方处理器往 stdout printf 的噪音）。
- 预算：父端 spawn→ready ≤15s；menu→shown ≤15s（慢处理器超时即杀整个进程树）；shown→result ≤10min（菜单开着的时长，不设短超时）。
- 退役：50 次菜单 / 连续 3 次 result 3 → `bye` 后退出 0，父端下次请求时重启。
- 父进程退出：stdin 管道关闭 → 子端读到 EOF → 清理退出（孤儿自愈，无需守护）。

### 4.1 菜单关闭（`cancel` + 鼠标钩子）

**现象**：菜单弹出后关不掉——右键其他格子、点空白区域、点桌面、点其他软件都留着（只有左键点**同一个**格子能关）。

**根因（两层）**：
1. 宿主菜单主要靠 owner 的**激活变化/鼠标捕获**自行关闭，而 DeskBox 的格子窗口是**置顶且非激活**的窗口，点击它们既不改前台也不触发菜单的关闭逻辑；菜单 owner 又是**隐藏窗口**，永远收不到 `WM_ACTIVATE(WA_INACTIVE)`。
2. 已实测：在代理**确实拿到前台**、且点击落在普通可激活窗口上时，系统路径能关（探针 `src=menu`）；一旦环境不满足（真实交互里常见），就完全不关。**不能依赖前台**。

**方案（两条独立机制，互为兜底）**：

| 机制 | 覆盖 | 实现 |
|---|---|---|
| `cancel` 命令 → 读取线程 post ESC | DeskBox 自己的点击（格子窗口子类、叠放弹层子类） | 见下 |
| **`WH_MOUSE_LL` 鼠标钩子** → 任何外部鼠标按下即 post ESC | **所有情况**：非激活格子、桌面、其他软件 | **专职泵线程**上安装，菜单存活期内存在，菜单一关立即卸载 |

**钩子线程模型（第五轮修复，2026-09-12）**：低级鼠标钩子的回调由**安装它的线程的泵**送达。该线程一旦阻塞（`InvokeCommand` 里跑 1-2 秒的 Shell 动词、或慢处理器填子菜单），系统**停止向钩子递送鼠标事件、整条桌面输入管线卡顿**——用户侧即"点了菜单项后全系统鼠标卡 1-2s"。实验定案（注入 72 个鼠标事件、坐标走廊过滤排除真实鼠标）：钩子线程 Sleep 1.5s 期间**只有 2 个事件到达回调**，第一个 pending 调用等满 800ms 测量上限；泵着时中位 16ms。因此：

- 钩子安装在**专职线程**（`menu-mouse-hook`，只做 `GetMessageW` 泵，永不跑 Shell 工作）；guard `drop` 时 `PostThreadMessageW(WM_QUIT)` 通知其自卸载。
- `TrackPopupMenuEx` 返回（菜单关闭）后**立即 `drop(_mouse_hook)`**，在 `InvokeCommand` 之前——菜单关了钩子就没有职责了。

- ESC 是 `TrackPopupMenuEx` 模态循环的取消键（实测有效；`WM_CANCELMODE` 无效），返回 0 → `result 2`。
- **`cancel` 由 stdin 读取线程就地处理**：菜单开着时主线程阻塞在 `TrackPopupMenuEx` 里，只有别的线程 post 消息才能结束它。
- **钩子只对"菜单之外"的按下生效**：用 `WindowFromPoint` + 类名 `#32768`（Win32 菜单类）判定点击是否落在菜单自身，落在菜单内一律放行，不影响点选条目；只处理按键按下（不处理抬起/滚轮），所以打开菜单的那次手势不会误关。
- 钩子是低级别全局鼠标钩子，回调里只做一次 `WindowFromPoint` + `PostMessage`，且**仅在菜单存活期间存在于专职线程**（通常几秒）；安装失败也不致命（退化为仅系统路径 + `cancel` 命令），结果行会记录 `hook=0`。
- C# 侧 `CancelOpenMenu()` 在每次鼠标按下时调用（早退条件 `s_menuInFlight==0`）；收口点在**格子窗口 WndProc 子类**与**叠放弹层宿主窗口 WndProc 子类**。另外 `AllowSetForegroundWindow(proxyPid)` 在每轮前授予前台权限，让菜单行为更接近正常前台菜单（键盘导航等）。
- **右键另一条目 = 关旧开新**（Explorer 语义）：菜单开启期间的右键请求记入 `s_pendingRequest`（后到覆盖）并立即 `cancel`；本轮 `result` 到达后自动接着显示。为此 server 模式在 `InvokeCommand` 后**立即**回 `result`（2s 宽限泵只留给会退出的单发模式）。
- stdin 写入以 `_writeGate` 串行化，避免 `cancel` 与 `menu` 两行交错破坏协议。
- **自证格式**：`result <code> src=<outside-click|menu> fg=<0|1> hook=<0|1>`，C# 侧记入 `Menu timing ... native=…`，可直接从日志判定当次菜单是否拿到前台、钩子是否装上、由谁关闭。

### 4.2 子菜单为空（`TPM_NONOTIFY`）

**现象**：菜单顶层条目正常，但"发送到"/"打开方式"/压缩工具等子菜单展开后是**空的窄条**。

**根因**：这些子菜单是**延迟生成**的——在展开前条目数为 0，靠 `WM_INITMENUPOPUP` 填充（Raymond Chen part 5 讲的正是这个）。而 `TPM_NONOTIFY` 的文档语义是"不向 `hwnd` 发送通知消息"，实测与 Wine 实现都证实它**抑制 `WM_INITMENUPOPUP`**（`if (!(flags & TPM_NONOTIFY)) send_message(hwnd, WM_INITMENUPOPUP, ...)`），于是 handler 永远没机会填子菜单。参考实现（Chen 范例、Explorer++、Npp 插件、Files）**没有一个**同时用 `TPM_NONOTIFY` 与菜单消息转发。

**修复**：去掉该 flag。它本来就冗余——我们用 `TPM_RETURNCMD`，选择结果由返回值给出（文档："指定 TPM_RETURNCMD 时，函数返回用户选择项的标识"），不需要 `WM_COMMAND` 通知。

**验证**：代理在 `WM_INITMENUPOPUP` 时把子菜单条目数打到 stderr。修复前该消息根本不出现；修复后实测 `initmenupopup items=26`（延迟生成的条目已被填充）。

### 4.3 菜单跟随明暗主题（uxtheme 私有序号）

**现象**：Explorer 的原生菜单跟随明暗主题，DeskBox 里的原生菜单**永远是亮色**。

**根因**：宿主经典菜单的暗色渲染需要**拥有菜单的进程**主动 opt-in，而微软**没有公开 API**（WindowsAppSDK #2943/#5543/#95 都是这个诉求，微软明确回答"普通 Win32 窗口的系统菜单就是亮色的，这是 Win32 的行为"）。Explorer 自己走的是 uxtheme.dll 的**未文档化序号导出**。代理进程此前一次都没调用过，所以一直亮色。

**修复**（`native/deskbox-thumbnail-proxy/src/main.rs`，全部带空指针检查、失败即静默降级回亮色）：

| 序号 | 作用 |
|---|---|
| 135 `SetPreferredAppMode` | 进程级 opt-in（本机实测 `previous` 返回 `0`=`Default`，证明序号 135 确实指向真函数）；DeskBox 深色→`ForceDark`，浅色→`ForceLight`（用 `Force*` 而非 `AllowDark`，否则浅色系统上选择深色的用户仍会拿到亮菜单） |
| 104 `RefreshImmersiveColorPolicyState` + 136 `FlushMenuThemes` | 运行时切换主题时必须（顺序：先 refresh 再 set 再 flush），否则缓存的菜单主题不刷新 |
| 133 `AllowDarkModeForWindow` + `SetWindowTheme(owner, "DarkMode_Explorer"/"Explorer")` | owner 窗口层级的登记 |

- 进程级模式在**创建任何窗口之前**设置（server 启动时按 CLI 主题、单发模式按参数），每轮再按 DeskBox 当前主题幂等应用。
- 主题来源：C# `App.Current.ThemeService.CurrentTheme`（`System` 时取 `Application.Current.RequestedTheme`），随 `menu` 命令第四个字段传入；server/单发各自也带一个初始主题 token。
- **实测对照**（探针采样菜单区域平均亮度）：请求 `dark` → luma **61**（暗）；请求 `light` → luma **231**（亮）。五种调用组合（force/allow × 窗口主题 default/explorer/none）均得到暗色菜单，说明关键就是 135。
- 已知边界：菜单**栏**与 Alt+Space 系统菜单无法通过此路径变暗（平台限制，需 owner-draw）；第三方**自绘**菜单项保持其自身配色；若某天序号被移除，代码会退化为亮色菜单（不崩）。

## 5. 微软官方契约对照（宿主侧必须项，均已满足）

GetUIObjectOf(IID_IContextMenu) → QueryContextMenu（HMENU 归宿主，用毕 DestroyMenu）→ QI IContextMenu3/2 → 菜单存活期把 `WM_INITMENUPOPUP/WM_MEASUREITEM/WM_DRAWITEM/WM_MENUCHAR` 转发 `HandleMenuMsg2/HandleMenuMsg` → `TrackPopupMenuEx(TPM_RETURNCMD)` + 前台舞步（SetForegroundWindow → WM_NULL）→ `InvokeCommand(CMINVOKECOMMANDINFOEX, lpVerb=MAKEINTRESOURCE(id-idCmdFirst))`，全程单一 STA。权威范本：Raymond Chen "How to host an IContextMenu" 系列（2004，devblogs.microsoft.com/oldnewthing）。

已知平台边界（非缺陷，勿修）：Win11 宿主只能拿经典菜单；HMENU 菜单不跟随应用暗色模式（WindowsAppSDK #2943/#5543 仍开放）；Win11 新式注册项（纯 IExplorerCommand、无经典回退注册）在宿主菜单中缺失是全行业现象（Files 同样缺失）。

## 6. 待办（P2，未做）

- 菜单 UX 演进：自定义菜单常驻 + "更多系统选项"懒加载（Win11 Explorer 自身范式），消除感知延迟的最后一段。
- 常用动词直调快路：`ShellExecuteEx(SEE_MASK_INVOKEIDLIST)`（properties/delete/openas 文档化动词），常用操作不再建菜单。
- 坏扩展黑名单：server 崩溃时按最近一次菜单的处理器集合缩小嫌疑（Opus 13 的自动拉黑机制），先落日志后落策略。
- 观察项：微软 2026-08 Insider 菜单改版（提速 + 自定义页）若提供宿主 API 再评估。

## 7. 未坐实项与调查记录（2026-09-12 第二轮）

**已排除（每项都有对照实验）**：协议本身、BOM 之外的编码问题、预热污染公寓、非 ASCII 路径、E: 盘/网络挂起。用 PowerShell(pwsh 7)/.NET 10 复刻 C# 的真实管道路径，以下场景全部正常：

| 场景 | 结果 |
|---|---|
| 新建 server → 真实构建（`C:\Windows\notepad.exe`） | `shown` 2343ms（冷启动处理器加载） |
| 预热 → 真实构建 | `shown` 299ms |
| 预热 → 不存在路径 | `result 3` 即时 |
| 失败那条确切口径 `E:\DeskBox\AI工具\MiniMax Hub.lnk` | `shown` 693ms |
| 同目录其他 .lnk / 目录 | `shown` 648ms / 368ms |

**一处真实且已修的缺陷（但非本次现象）**：`StandardInputEncoding = Encoding.UTF8` 在**首次写入**子进程 stdin 时会写 UTF-8 BOM（EF BB BF）。已用文件喂入实验证实：带 BOM 的 `menu\t…` 行会被原生端 `strip_prefix("menu\t")` 判为噪音**静默丢弃**（无 `result`、无 stderr）。修复 = C# 改用 `UTF8Encoding(encoderShouldEmitUTF8Identifier: false)` + 原生端 `normalize_command_line` 容忍 BOM + 未知输入行上报 stderr（不再静默）。

**仍未坐实**：应用内曾出现 `Menu build timed out timeoutMs=15000 serverMs=38 error=`（`E:\DeskBox\AI工具\MiniMax Hub.lnk`，15s 后重试同样失败），且**未能在应用外复现**。已知该轮次原生端既无 stdout 也无 stderr 输出，即命令未被处理。

**下次复现要看的三行**（已加进代码，都是 `App.Log`）：
1. `Ignored unexpected native output generation=N line='…'`——协议行被当成噪音丢弃（能直接抓到 BOM 类污染）；
2. `Menu build timed out … generation=N exited=False error=…`——`exited=False` 且 stderr 为空 = 服务进程活着但没处理命令；`error` 非空则是原生构建失败（如 `SHParseDisplayName` 报错）；
3. `Menu timing result=… buildMs=…`——成功的轮次也会留下痕迹，用于判断"菜单出现但点击无效"是否真在发生。

**顺带确认的原生约束**：`SHParseDisplayName` 对**正斜杠**路径返回 `E_INVALIDARG (0x80070057)`，所以传给原生的路径必须是 `Path.GetFullPath` 得到的反斜杠形式（`NormalizePath` 已保证，勿改成透传用户输入）。
