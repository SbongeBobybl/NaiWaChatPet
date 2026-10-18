# DeskBox 格子层级（Z-Order）生命周期与排查手册

> 文档性质：技术实现手册 + 故障复盘指南。
> 适用场景：F7 / 托盘唤起格子后出现的层级类问题（压屏、不回落、闪烁、不收起等）。
> 关联文档：`docs/architecture/widget_layer_workspace_plan.md`（产品规则口径）、`docs/architecture/current_architecture.md`（整体架构）。
> 最后更新：2026-08-12（按现行代码校正回落策略、组恢复与空闲排序规则）。

---

## 1. 系统目标与两种层级模式

格子（widget）本质上是一组**无边框 Win32 窗口**，需要在两个状态之间切换：

| 状态 | 含义 | Z-order 位置 |
|---|---|---|
| 桌面静置（DesktopResting） | 常态逻辑状态；物理位置由本次回落目标决定 | 外部前台窗口之后，或桌面图标附近 / 普通层级带底部 |
| 唤起（RaisedSession） | F7/托盘唤起，临时浮到最前 | **普通层级带顶部（非持久 TopMost）** |
| 交互中（InteractionActive） | 用户正在拖拽/重命名/开菜单 | 同唤起，且阻止自动回落 |
| 隐藏（Hidden） | 不可见 | — |

层级模式（`Settings.WidgetLayerMode`）：

1. **动态层级（默认）**：本文档主要描述的模式。F7 唤起时浮起，交互结束/点击外部后回落。
2. **桌面固定层（DesktopPinned，实验）**：格子 attach 到 WorkerW 桌面容器，所有"置顶/回落"操作都改为桌面图标层内的兄弟排序。**注意：几乎所有 Z-order 入口函数都有 `UsesDesktopPinnedMode()` 分支，修改任何一条路径时必须两种模式都过一遍。**

现行代码还把“逻辑状态”和“物理落点”分开处理。`DesktopResting` 只表示格子已经退出临时唤起/交互状态，不再等同于“绝对底层”。动态模式回落时由 `RelativeLayerRestorePolicy` 选择物理落点：

| 回落判定 | 物理落点 |
|---|---|
| 外部应用获得前台 | 整组格子紧随该前台窗口之后，保留格子内部顺序 |
| 前台是 DeskBox 自身 | 保持当前全局层级，只整理格子内部顺序 |
| 前台缺失或为桌面壳 | 回到桌面层 / 普通层级带底部 |
| DesktopPinned 模式 | 回到 Explorer 桌面 Owner 内，仅做桌面兄弟排序 |

格子窗口有两种宿主类型，**每条唤起/回落路径都有两份平行实现**，改动时必须同步：

| 宿主类 | 文件 | 用于 |
|---|---|---|
| `QuickCaptureWidgetWindow` | `src/DeskBox/Views/QuickCaptureWidgetWindow.*.cs` | 随记格子 |
| `ContentWidgetWindow` | `src/DeskBox/Views/ContentWidgetWindow.*.cs` | 文件、Todo、音乐、天气、搜索等统一内容格子 |

---

## 2. 核心机制：唤起不靠"持久置顶"

**这是理解整个系统的钥匙。** 唤起时格子**不是**持久 TopMost 窗口，而是通过一个 Win32 技巧浮到普通层级带顶部：

```
SetWindowPos(hwnd, HWND_TOPMOST,   ..., SWP_NOACTIVATE | SWP_SHOWWINDOW);
SetWindowPos(hwnd, HWND_NOTOPMOST, ..., SWP_NOACTIVATE | SWP_SHOWWINDOW);
```

先设为 TOPMOST 再立刻取消，窗口会停留在**普通（非 TopMost）层级带的最顶部**。效果上"浮在所有普通窗口之上"，但不占用 TopMost 属性——这样其他窗口被激活时可以正常盖过它。

实现位置：`src/DeskBox/Helpers/Win32Helper.cs` 的 `BringWindowTemporarilyToFront()`（约 556 行）。

> **推论**：格子压屏问题几乎都不是"置顶没清除"，而是"**回落（restore）没有被触发**"或"**回落了但没有视觉效果**"。排查时不要先找谁设了 TopMost，先找回落信号为什么没响。

### 持久置顶只作为瞬态存在

`WidgetLayerService.BringGroupTemporarilyToFront()`（`src/DeskBox/Services/WidgetLayerService.cs:137`）在批量唤起时会**短暂**把所有格子设为持久 TopMost，随后在同一函数内逐个 `ClearWindowTopMost` 清除，最后把活动窗口 `BringWindowToFront` + `SetForegroundWindow`。整个序列同步执行，正常结束后无残留。

---

## 3. F7 唤起全流程

### 3.1 热键入口

`src/DeskBox/Services/GlobalHotkeyService.cs`

- 主路径：`RegisterHotKey` + 窗口子类化（`SetWindowSubclass`）接 `WM_HOTKEY`。
- 兜底路径：`WH_KEYBOARD_LL` 低级键盘钩子（`KeyboardHookProc`）。**当前台窗口是提权进程时，UIPI 会拦截 WM_HOTKEY 投递，只有钩子路径能收到 F7**——这就是"热键时而灵时而不灵"的来源，不是注册失败。
- 两条路径都去重（`_hookGestureIsDown` / `_isInvoking`），最终调 `App.Tray.cs` 的 `ToggleTrayWidgetsAsync()`。

### 3.2 Toggle 决策（三态）

`src/DeskBox/Services/WidgetManager.cs` 的 `ShouldHideWidgetsForTrayToggle()`（231 行）：

| 条件 | 决策 | 日志标记 |
|---|---|---|
| `_widgetsRaisedFromTray == true`（唤起态中） | **hide** | `reason=raised-session` |
| 无可见格子 | **raise** | `reason=no-visible-windows` |
| 前台是 DeskBox / 桌面壳（Progman/WorkerW）/ 任务栏 | **hide** | `reason=foreground-local` |
| 格子可见但被埋 + 前台是外部窗口 | **raise** | `reason=visible-widgets-behind` |

> 注意最后一行是**有意设计**（2026-07-24 用户确认保留）：F7 可以把被其他窗口埋住的格子重新捞上来。所以"唤起 → 点外部回落 → 再按 F7"是重新浮起而不是隐藏，属预期行为。

### 3.3 唤起执行序列

`src/DeskBox/Services/WidgetManager.TrayAnimation.cs` 的 `RaiseWidgetsFromTrayAsync`（约 40-130 行），顺序固定、相互依赖，**调整顺序前务必读完整个函数**：

1. `_isTogglingWidgetsDesktopLayer = true`（finally 复位，防重入）。
2. 逐格子 `PrepareWidgetForBatchShowAsync`（异步，可能首次创建窗口）。
3. 隐藏中的格子 `ShowPreparedRaisedFromTray()`；已可见的格子 `EnsureRaisedFromTrayTopMost()`。
   - 注意 `EnsureRaisedFromTrayTopMost` 有 `_isAtDesktopLayer` 短路（统一内容宿主与随记宿主均有）：**上一轮已被 restore 的格子会被跳过**，它们的物理浮起实际靠第 6 步的 group 操作兜底。
4. 记录 `_foregroundAtRaiseTime = GetForegroundWindow()`；设置 `_suppressTrayLayerRestoreUntilUtc = now + 160ms`（防止唤起动画期间的瞬时事件误触发回落）。
5. `SetWidgetsRaisedFromTray(true)` 进入唤起态。
6. `QueueTrayRaiseTopMostConfirmation` → `BringGroupTemporarilyToFront`（瞬态置顶再清除，见 §2）。
7. **`StartTrayLayerRestoreMonitor`：启动 200ms 恢复监视器 + 50ms 鼠标边沿采样器（见 §4）。**
8. `ActivateLastRaisedWindow` → 两个宿主类的 `ActivateRaisedFromTrayBatch()`：`base.Activate()` + `SetForegroundWindow(hwnd)`。
   - **`SetForegroundWindow` 经常失败**（Windows 前台锁：只有"收到最后一次输入事件"的进程才能抢前台；热键经异步队列 + 窗口准备耗时后，输入事件归属可能已不是 DeskBox；前台是提权进程时 UIPI 也会拒绝）。**返回值必须检查并记日志**（2026-07-24 起已加，日志前缀 `[ZOrder] ... SetForegroundWindow FAILED`）。失败是合法的，系统必须能在"DeskBox 从未获得前台"的情况下正确回落。

---

## 4. 回落（Restore）信号体系

**唤起态期间，单窗口的所有自救路径都被显式禁用**（`WidgetWindowBase.Interaction.cs`、`ContentWidgetWindow.WindowInteraction.cs` 与随记宿主的 Deactivated/安全定时器路径都检查 `WidgetsRaisedFromTray: true` 后跳过）。唯一生效的回落路径是管理器侧的 **200ms 恢复监视器**：

`src/DeskBox/Services/WidgetManager.ZOrder.cs` 的 `TrayLayerRestoreTimer_Tick` → `TryRestoreRaisedWidgetsAfterInteraction`（约 93-152 行）。

### 4.1 监视器的四道闸（任一不满足则跳过本 tick）

1. `_isTogglingWidgetsDesktopLayer`：toggle 进行中。
2. `IsWidgetInteractionActive`：交互深度 > 0（拖拽/重命名/菜单/对话框）。**泄漏会永久堵死回落，见 §6 坑 #4。**
3. `_suppressTrayLayerRestoreUntilUtc`：唤起后 160ms 抑制窗。
4. 前台判断：前台是 DeskBox → 保持唤起并标记 `_hasDeskBoxForegroundSinceRaise`；前台是任务栏 → 保持唤起。

### 4.2 触发回落的三条信号

| 信号 | 可靠性 | 说明 |
|---|---|---|
| **DeskBox 曾拿前台，后离开** | 高 | 激活成功时的主路径。点任何外部窗口即触发。 |
| **前台窗口发生变化**（≠ `_foregroundAtRaiseTime`） | 高 | 激活失败时的主路径。要求用户点了**不同的**窗口。 |
| **鼠标按下边沿**（50ms 采样器） | 高（2026-07-24 修复后） | 激活失败 + 用户点回**同一个**已激活窗口时的兜底。 |

### 4.3 鼠标边沿采样器（方案 B，2026-07-24 引入）

`WidgetManager.ZOrder.cs` 的 `TrayMouseSamplerTimer_Tick`：

- 50ms 轮询 `Win32Helper.IsAnyMouseButtonDown()`（`GetAsyncKeyState` **高位**，全局物理状态，与目标进程是否提权无关）。
- 检测 up→down 跳变，**在按下瞬间**判断光标不在 DeskBox/任务栏上 → 置 `_outsideMousePressObserved = true`。
- 200ms 监视器消费该标志触发回落。
- 启动时预充当前按键状态（`_lastMouseButtonsDown = IsAnyMouseButtonDown()`），防止用户按住触发热键的那次点击被误判为新按下。

> **历史教训（坑 #1）**：旧实现用 `GetAsyncKeyState & 0x0001` 低位（"自上次查询以来是否按下"）。低位只对**派发到本线程消息队列**的输入可靠，点击其他进程窗口时经常不置位——这就是"点同一窗口 1 次不回落"的根因。代码注释里早就写了 *"GetAsyncKeyState (which only sees presses posted to our own thread)"*，但仍被用作唯一兜底信号。**检测跨进程点击，只能用高位轮询 + 自己记边沿，或 WH_MOUSE_LL 钩子。**

### 4.4 回落执行（现行代码）

`WidgetManager.RestoreRaisedWidgetsToDesktopLayer` 先让各宿主执行 `ForceRestoreDesktopLayerFromManager()`，清理临时置顶、交互计时器及逻辑状态；随后调用 `WidgetLayerService.RestoreGroupPreservingForeground()`，把所有可见格子作为一个连续 Z-order 组恢复。

组恢复只读取一次当前前台根窗口，并按 `RelativeLayerRestorePolicy` 选择落点：外部页面对应 `BehindForeground`，DeskBox 前台对应 `PreservePeerOrder`，桌面壳或无有效前台对应 `DesktopBottom`。外部页面分支以该页面为固定边界，使用 `BeginDeferWindowPos` / `DeferWindowPos` 一次排列整个格子组；失败时再以相同边界逐格子 `SetWindowPos` 兜底。

`NormalizeIdleWidgetZOrder()` 只能整理格子之间的顺序。它以当前最高格子的上一窗口作为全局边界，不得调用 `MoveToDesktopBottom()`、`SetWindowToBottom()` 或重新绑定 Owner。否则前面刚建立的“前台页面 > 格子组 > 更早页面”会在恢复末尾或 120ms 延迟回调中被覆盖成“所有页面 > 格子组”。

> **关键约束**：回落必须同时完成“退出临时唤起状态”和“建立新的全局 Z-order 边界”。单窗口可以负责清理自身状态；多个格子的全局位置只能由管理器按组确定。

---

## 5. 涉及文件速查表

| 文件 | 职责 |
|---|---|
| `src/DeskBox/Services/GlobalHotkeyService.cs` | F7 注册（RegisterHotKey + WH_KEYBOARD_LL 双路径）、去重、触发 |
| `src/DeskBox/App.Tray.cs` | `ToggleTrayWidgetsAsync`（551 行）：toggle 总入口 |
| `src/DeskBox/Services/WidgetManager.cs` | `ShouldHideWidgetsForTrayToggle`（231）、`RestoreRaisedWidgetsToDesktopLayer`（1141）、`IsWidgetInteractionActive`（112） |
| `src/DeskBox/Services/WidgetManager.TrayAnimation.cs` | `RaiseWidgetsFromTrayAsync` 唤起序列、`_foregroundAtRaiseTime`、抑制窗、`ActivateLastRaisedWindow` |
| `src/DeskBox/Services/WidgetManager.ZOrder.cs` | **恢复监视器（200ms）+ 鼠标采样器（50ms）+ 交互泄漏看门狗**，前台/任务栏/桌面壳判定 |
| `src/DeskBox/Services/WidgetLayerService.cs` | Z-order 原语：`BringWindowTemporarilyToFront`、`BringGroupTemporarilyToFront`、`RestoreGroupPreservingForeground`、相对前台组排序、DesktopPinned attach/detach |
| `src/DeskBox/Services/WidgetSessionManager.cs` | 会话状态机 + 交互深度计数（`BeginInteraction`/`EndInteraction`/`ForceResetInteractions`） |
| `src/DeskBox/Helpers/Win32Helper.cs` | `BringWindowTemporarilyToFront`（556）、`SetWindowTopMost`（575）、`ClearWindowTopMost`（590）、`IsAnyMouseButtonDown`（约 344）、`GetAsyncKeyState` 封装 |
| `src/DeskBox/Views/WidgetWindowBase.Interaction.cs` | 基类版本同上 + `ShouldDeferDesktopLayerRestore`（94） |
| `src/DeskBox/Views/WidgetWindowBase.Collapse.cs` | `RaiseForExpandedState`（1811）：胶囊展开时的层级处理（含"物理浮起但状态已回落"的兼容分支） |
| `src/DeskBox/Views/ContentWidgetWindow.xaml.cs` | 文件与功能格子的统一宿主；实现唤起/回落/激活和内容切换 |
| `src/DeskBox/Views/QuickCaptureWidgetWindow.xaml.cs` | 随记宿主的平行唤起/回落/激活实现 |
| `src/DeskBox/App.xaml.cs` | `IsDeskBoxWindow`（562）：按 PID + 已知窗口根判定，**范围宽（本进程所有窗口）** |

---

## 6. 踩坑清单（按危害排序）

### 坑 #1：用 `GetAsyncKeyState` 低位检测跨进程点击 —— 不可靠
- 低位（`& 0x0001`）语义是"自**本线程**上次查询以来是否按下"，对其他进程窗口收到的点击经常不置位。
- **正确做法**：高位（`& 0x8000`）全局物理状态 + 50ms 轮询 + 自记 up→down 边沿（已实现，见 §4.3）；或 `WH_MOUSE_LL` 全局钩子（兜底升级路径，暂未启用）。
- 采样间隔必须小于典型点击按下时长（50-150ms），200ms 轮询会漏快速点击。

### 坑 #2：把状态回落等同于 `HWND_BOTTOM` —— 会破坏页面相对层级
- 点击外部页面后，Windows 会把该页面提升到普通层级带顶部；格子此时自然应成为紧随其后的连续组。
- 如果恢复末尾或延迟空闲排序再调用 `MoveToDesktopBottom()`，格子会从“新前台页面之后”直接掉到所有普通页面之后。
- `HWND_BOTTOM` 只允许用于桌面壳/无有效前台等明确的桌面回落场景。外部页面场景必须使用该前台根窗口作为 `hWndInsertAfter` 边界。

### 坑 #3：`SetForegroundWindow` 静默失败 —— 必须检查返回值
- Windows 前台锁（foreground lock）规则：只有"收到最后一次输入事件"的进程等少数情况能抢前台。热键 → 异步队列 → 窗口准备/动画耗时后，输入归属可能已丢失；前台是提权进程时 UIPI 直接拒绝。
- 失败时格子仍浮起但永远拿不到前台，`_hasDeskBoxForegroundSinceRaise` 永不成立，回落完全依赖"前台变化"或"鼠标边沿"两条信号。
- 三个 `ActivateRaisedFromTrayBatch` 已实现返回值日志（`[ZOrder] ... SetForegroundWindow FAILED`），复盘先看这条。

### 坑 #4：`BeginInteractionLayer`/`ReleaseInteractionLayer` 配对泄漏 —— 会永久堵死回落
- 交互深度计数在 `WidgetSessionManager._interactionDepth`，> 0 时监视器每 tick 跳过。
- 清零只发生在 `MarkDesktopResting`/`MarkHidden`——而这俩又依赖回落先发生，**泄漏即死锁**。
- 已加看门狗（`WidgetManager.ZOrder.cs` 的 `RunInteractionLeakWatchdog`）：深度 > 0 且 DeskBox 无前台持续 10s → 判定泄漏，强制 reset。真实交互必有 DeskBox 前台，不误伤。
- **新增任何 Begin 调用点时**：确认所有退出路径（异常、取消、窗口中途隐藏、flyout 轻 dismiss）都有配对 End。

### 坑 #5：修改一条路径，忘了另一类宿主 / 另一种层级模式
- 两种宿主（QuickCaptureWidgetWindow / ContentWidgetWindow）有平行实现；`UsesDesktopPinnedMode()` 分支遍布所有 Z-order 原语。改动后四象限（2 宿主 × 2 模式）都要过。

### 坑 #6：瞬态置顶函数被误当持久置顶用
- `BringWindowTemporarilyToFront`（先 TOPMOST 再 NOTOPMOST）只保证"浮到普通带顶部"，调用返回后窗口**不是** TopMost。`StartTopMostSafetyTimer` 一进来就查 `IsWindowTopMost`，不是持久置顶会直接退出——安全网对瞬态浮起无效，别指望它兜底。
- `BringGroupTemporarilyToFront` 内部的"全员 TopMost → 逐个清除"序列必须保持原子（同步执行），往中间插入 await 会留下持久置顶残留。

### 坑 #7：监视器的抑制窗/代际（generation）被意外延长或失效
- `_suppressTrayLayerRestoreUntilUtc` 目前只有唤起时 +160ms 一处赋值。新增赋值点要克制——它直接推迟回落。
- `_trayRaiseBatchGeneration` 用于让过期异步回调失效（每次唤起/确认/回落都自增）。写新的延迟回调时记得捕获当前代际并在回调里比对，参考 `ConfirmTrayRaiseTopMost`。

### 坑 #8：`IsDeskBoxWindow` 按进程判定，范围很宽
- 本进程**所有**窗口（搜索弹窗、设置、托盘隐藏窗口）都算 DeskBox 窗口。前台判断时，DeskBox 自家任何窗口拿到前台都会被视为"用户还在用格子"而保持唤起。新增顶级窗口类型时意识到这一点。

### 坑 #9：死代码假象
- `RequestRestoreRaisedWidgetsToDesktopLayer` 目前只记日志不调度任何检查（"held until=next-toggle"），配套的 `QueueRequestedLayerRestoreCheck` 定义了但无人调用。以为"请求一下就会回落"会落空——真正的回落永远走监视器。

---

## 7. 排查手册：遇到层级问题怎么看

### 7.1 先分类症状

| 症状 | 大概率原因 | 首查 |
|---|---|---|
| 唤起后点外部窗口，格子**从不**回落 | 回落信号全失效（坑 #1/#4）或监视器没启动 | 日志搜 `[TrayBatch] RaisedStateMonitor started` 是否出现 |
| 点**不同**窗口能回落，点**同一**窗口不回落 | 激活失败 + 鼠标检测失效（坑 #1/#3） | 搜 `SetForegroundWindow FAILED` |
| 点击外部页面后格子直接掉到所有页面后面 | 回落后又被空闲排序绝对置底（坑 #2） | 搜 `Group restore ... disposition=BehindForeground` 后是否又出现 `SetWindowToBottom` |
| 交互过一次格子后永远压屏 | 交互深度泄漏（坑 #4） | 搜 `Interaction watchdog` |
| 只有前台是提权应用时出问题 | UIPI：热键走钩子兜底、激活必失败 | 同第 2 行 |
| F7 时灵时不灵 | 同上（钩子路径在干活，主路径被 UIPI 拦） | `GlobalHotkeyService` 日志 `source=hook/registered` |

### 7.2 关键日志标记（App.Log / App.LogVerbose）

```
[GlobalHotkey] Triggered source=registered|hook     热键触发及路径
[TrayBatch] Raise requested / completed             唤起开始与结束
[TrayBatch] RaisedStateMonitor started/stopped      监视器生命周期
[TrayBatch] RaisedState released reason=...         回落触发及依据
        -foreground-changed      前台变化（可靠）
        -outside-click           鼠标边沿采样（50ms 采样器）
        -deskbox-leave           DeskBox 曾有前台后离开（可靠）
[TrayBatch] ToggleDecision=hide|raise reason=...    F7 决策依据
[ZOrder] ... SetForegroundWindow FAILED             激活失败（坑 #3）
[ZOrder] Group restore ... disposition=...          整组回落使用的前台锚点和策略
[ZOrder] Window order applied reason=...             组排序边界和执行结果
[TrayBatch] Interaction watchdog ...                交互泄漏看门狗（坑 #4）
[WidgetSession] changed/kept ...                    会话状态机迁移
```

### 7.3 标准复现路径（回归用）

1. 在窗口 X 中按 F7 → 格子浮起。
2. 点击**另一个**窗口 Y → 格子应立即被 Y 盖住（前台变化路径）。
3. 再按 F7 → 格子重新浮起（`visible-widgets-behind` 特性，预期行为）。
4. 点击**同一个**已激活窗口 1 次 → 格子应被盖住（鼠标边沿路径；修复前这里会卡住）。
5. 再按 F7 → 格子正常收起。
6. 提权应用（如管理员终端）在前台时重复 1-5，行为应一致（高位采样对提权进程同样有效）。
7. 依次打开页面 A、B、C；在 C 上按 F7，再点击 C：预期顺序为 `C > 格子组 > B > A`，格子不得落到 A 之后。
8. 重复第 7 步并点击 B：预期顺序为 `B > 格子组 > C > A`；再等待超过 120ms，顺序不得被空闲整理改变。

---

## 8. 本次修复（2026-07-24）变更清单

> 本节保留历史记录。2026-08-12 的现行实现已用 `RelativeLayerRestorePolicy` 与 `RestoreGroupPreservingForeground` 替代旧的“无条件 `BringWindowToFront(foreground)`”回落方式，实际排查应以 §1、§4.4 和当前代码为准。

| 方案 | 文件 | 变更 |
|---|---|---|
| A 恢复要看得见 | `Services/WidgetLayerService.cs` | `ClearTopMostPreservingForeground` 去掉 `wasTopMost` 门控，无条件 `BringWindowToFront(foreground)` |
| B 检测可靠化 | `Helpers/Win32Helper.cs` | 新增 `IsAnyMouseButtonDown()`（高位物理状态） |
| B | `Services/WidgetManager.ZOrder.cs` | 新增 50ms 采样定时器（边沿检测 + 按下瞬间位置过滤）；200ms 监视器改消费 `_outsideMousePressObserved`；`StopTrayLayerRestoreMonitor` 同步停采样器 |
| B | `Services/WidgetManager.TrayAnimation.cs` | 删除为旧低位机制预充的 `HasMouseButtonActivity()` 调用 |
| D 可观测 | 两个宿主的 `ActivateRaisedFromTrayBatch` | 检查 `SetForegroundWindow` 返回值并记日志 |
| D 看门狗 | `Services/WidgetSessionManager.cs` | 新增 `ForceResetInteractions` |
| D 看门狗 | `Services/WidgetManager.ZOrder.cs` | `RunInteractionLeakWatchdog`：泄漏 >10s 且无 DeskBox 前台 → 强制清零 |
| C toggle 语义 | — | **不改**：`visible-widgets-behind → raise` 特性经用户确认保留 |

---

## 9. 2026-08-12 相对页面层级修复

| 文件 | 变更 |
|---|---|
| `Services/WidgetLayerService.cs` | 新增 `RestoreGroupPreservingForeground`；用同一个前台根窗口作为整组边界，并复用批量 peer 排序原语 |
| `Services/WidgetManager.cs` | F7 回落先统一清理宿主状态，再以单次组操作确定全局 Z-order；取消宿主排队的过期空闲整理 |
| `Services/WidgetManager.ZOrder.cs` | `NormalizeIdleWidgetZOrder` 改为 peer-only，不再对每个格子调用 `MoveToDesktopBottom` |
| `tests/DeskBox.Tests/WidgetZOrderRestoreContractTests.cs` | 锁定“外部页面用组恢复”和“空闲整理不得绝对置底”两项契约 |

预期层级示例：初始页面为 `C > B > A`，F7 后为 `格子组 > C > B > A`；用户点击 B 后必须变为 `B > 格子组 > C > A`，后续空闲排序只能改变格子组内部顺序。
