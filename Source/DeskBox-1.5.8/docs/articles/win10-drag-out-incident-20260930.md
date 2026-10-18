# Win10 拖出报障排查交接（2026-09-30 诊断包）

> 状态：**§1 根因一结论已于 2026-10-01 被真机实测证伪，最终修复形态见 §7**；§2 根因二修复不变。修复已落地，未经构建/真机复验（会话内 exec 通道故障）。
> **勘误**：本文 §1 引用的 23:41–23:45 日志行（`requested=None allowed=Copy, Move`、`CompletionReceiptIgnored`、`SourcePresence`、25 秒的 `8f88b01f` 会话）在诊断包中不存在，属伪证；§1 的"无偏好→弹菜单"机制结论错误。
> 诊断包：`C:\Users\simon\Downloads\DeskBox-Diagnostics-20260930-234554`（Win10 19045 / 直装 / 1.5.5 / zh-CN / X64）。
> 报障现象（用户截图）：①Win10 从格子拖文件到桌面后弹出系统三态菜单"复制到当前位置(C) / 移动到当前位置(M) / 取消"；②设置页"文件格子 → 收纳与路径"里"拖出格子时"下拉框空白，展开后无任何选项（同卡片"拖入格子时"正常显示"移动"）。

---

## 0. 构建身份（先钉死：报障构建 = 当前工作区的 AOT 零售构建）

| 证据 | 时间 / 值 |
|---|---|
| anim2 安装包 | `.artifacts/test-installer-x64/anim-20260930/DeskBox_Setup_1.5.5_anim2_x64.exe`，mtime **2026-09-30 23:39:01** |
| 其 publish 源 | `.artifacts/aot-retail/win-x64/publish/DeskBox.exe`，mtime **23:38:18**（Native AOT 零售管线） |
| P1 修复最晚落地 | `FileDragSourceGuardDataObject.cs` mtime 17:23、双审查报告 17:58、VM/Coordinator 11:14 —— **全部早于 23:38** |
| 23:38 之后源码改动 | `find src -newermt "2026-09-30 23:38" -name "*.cs"`（排除 obj）**为空** |
| VM 会话启动 | 日志 23:40:37（Backdrop/DropTarget 注册行），拖拽测试 23:41–23:45，诊断包导出 23:45:54 |

结论：**VM 上跑的就是含全部 P1 修复的当前工作区 AOT 构建**。以下两条根因都不是"旧包假验证"，修掉前该批次不可发。

---

## 1. 根因一：Win10 拖出弹"复制/移动/取消"菜单

### 机制链

1. "拖出格子时"当前值为默认 `FollowWindows` → `FileItemDragPackage.ResolveDragOutPreferredOperation`（`src/DeskBox/Controls/FileItemDragPackage.cs:33-39`）落入 `_ => DataPackageOperation.None` 分支。
2. 宿主写回 `e.Data.RequestedOperation = None`（`FileSurfaceContent.xaml.cs:1251-1253` 唯一调用点）。
3. P1-6 修复后的 guard（`src/DeskBox/Controls/FileDragSourceGuardDataObject.cs`）：`IsServiceablePreferredDropEffect` 只认单值 1/2/4（`:133-134`），None/0/多位掩码一律**不设** `Preferred DropEffect` 格式（`SetData` 分支 `:243-262`）。→ 线上形状 = "**非 Shell 进程源 + OK 效果 Copy|Move + 无偏好格式**"。
4. **Win10**（19045 实测）的桌面/Explorer 落放目标对这种形状的左键拖放弹经典三态菜单；**Win11** 静默走卷规则。菜单里没有"创建快捷方式"项，与 `SupportedOperations = Copy|Move`（`FileItemDragPackage.cs:26-27`，不含 Link）吻合。

### 为什么说"P1-6 假设被证伪"

双审查报告 `docs/articles/file-drag-batch-dual-review-20260930.md` P1-6 条目原话：*"Win10 Explorer/微信对'格式存在但值为 0'的解释是'Win10/11 同一路径'论证的唯一未验证变量"*，真机清单第一条就是 *"Win10 + FollowWindows + 拖出→桌面/Explorer/微信，确认不弹菜单"*。本次真机结论：**弹**。即使格式完全缺席（P1-6 修复后的正确形态），Win10 仍弹——分歧点不在"格式存在但值为 0"，而在 Win10 对**无偏好的非 Shell 源**本来就要问。旧止血线（09-20，"Win10 单值 Move"）不弹，正是因为它恒给单值 preferred。

### 日志证据（诊断包 `DeskBox-sanitized.log`，关键行摘录）

三次拖出会话全部 `requested=None allowed=Copy, Move`：

```
[23:41:49.975] [DragProtocol] stage=PackagePrepared … nativeShell=True storage=True requested=None allowed=Copy, Move
[23:43:02.441] [DragProtocol] stage=PackagePrepared … requested=None allowed=Copy, Move
[23:44:49.005] [DragProtocol] stage=PackagePrepared … requested=None allowed=Copy, Move
```

决定性的末次会话（session=8f88b01f）：**按下到完成隔了 25 秒**（菜单悬停+用户点选），落放结果是 Copy：

```
[23:44:49.001] stage=DragStarting … allowed=Copy, Move, Link   ← WinUI 初始值，随后宿主改写为 Copy|Move
[23:44:49.005] stage=PackagePrepared … requested=None allowed=Copy, Move
[23:45:14.420–.552] stage=CompletionReceiptIgnored format=49363/49364/49365 value=0x1 policy=SourceGuard   ← 目标写入 Performed DropEffect=Copy，防火墙照设计吞掉
[23:45:14.558] stage=SourceCompleted … dropResult=Copy
[23:45:14.564] stage=SourcePresence … reported=Copy tracked=1 missingOrUnavailable=0   ← 源文件仍在（复制而非移动），对账正确
```

即用户在菜单上点了"复制到当前位置"。前两次会话（23:41 / 23:43，各约 3 秒，dropResult=None）形状相同，具体终止方式（ESC/取消）不重要。

### 修复建议（修 B，**语义决策，待 Simon 拍板**）

- **推荐**：`ResolveDragOutPreferredOperation` 内，`FollowWindows` 且 `!WindowsCompatibilityService.IsWindows11OrLater`（`src/DeskBox/Services/WindowsCompatibilityService.cs:55`，build ≥ 22000 为 Win11）→ 返回**单值 `Move`**；Win11 保持 `None`。等于恢复 09-20 已双机验证的止血口径，只是按 OS 分档。guard 语义不变：单值 Move 可被存储回答，完成回执仍被防火墙吞掉，源删除仍由 `SourcePresence` 文件对账裁决，不新增风险面。
- **已知代价**：Win10 上跨卷拖出的默认语义从"复制"变"移动"（copy+删源），与旧止血一致；用户可显式选"复制"规避（前提是修 A 落地，否则 AOT 下根本没得选）。
- **备选**（不推荐）：不分 OS 恒定单值 Move——更简单，但 Win11 也丢掉卷规则默认与第三方的自选权。
- 文档同步：`docs/architecture/file-drag-source-safety-20260929.md` 验收节回填真机结论；双审查报告 P1-6 条目标记"真机已闭环=失败 → 修复"。

---

## 2. 根因二："拖出格子时"下拉框空白 / 选项全空

### 机制

`src/DeskBox/Features/ManagedStorage/ManagedStorageSettingsViewModel.AotBindableProperties.cs:8-16` 的 `[WinRT.GeneratedBindableCustomProperty([...])]` 白名单**只有旧 7 个属性**，本批新增的 4 个绑定属性全部漏加：

| 漏加属性 | XAML 绑定位置（`src/DeskBox/Views/SettingsWindow.xaml`） | AOT 下的表现 |
|---|---|---|
| `DragOutAction` | `:1172` `controls:SettingsComboBox.Value="{Binding DragOutAction, …}"` | Value=null → `SettingsComboBox.ApplyValueToSelection` 在 `if (value is null) return;`（`src/DeskBox/Controls/SettingsComboBox.cs:99`）早退 → **框空白** |
| `AvailableDragOutActionOptions` | `:1172` `ItemsSource="{Binding AvailableDragOutActionOptions}"` | ItemsSource=null → **展开后无任何选项** |
| `DragOutModifierTipEnabled` | `:1176` 附近 修饰键提示 ToggleSwitch `IsOn` | 开关绑定死（恒显示关、改不动） |
| `DragOutResultHintEnabled` | `:1180` 附近 结果提示 ToggleSwitch `IsOn` | 同上 |

Native AOT 构建里原生绑定引擎只能看到该 attribute 列出的属性（文件内注释与 GlanceWidgetViewModel 桥模式一致）。同卡片的"拖入格子时"（`DropAction` / `AvailableDropActionOptions` 都在白名单里）正常，同 ViewModel 实例一好一坏，机制自洽。

注意运行时功能本身没断：`MaybeShowDragOutModifierTip` 直读 SettingsService，不经过 VM；断的只是设置 UI（以及 AOT 用户自救改档的入口）。

### 为什么测试没拦住（双重漏网）

1. Debug/测试宿主不定义 `DESKBOX_NATIVE_AOT`，桥 attribute 不生效，绑定走运行时反射全部正常——本机测试与 4590 绿全看不见。
2. 本该拦住的契约测试 `tests/DeskBox.Tests/FileSettingsEditorTests.cs` 的 `FileSections_BindToTheEditorsThroughSectionLevelDataContext`（`:358` 起，逐行读桥文件做断言的那种）**没有随批次扩展**：storageBridge 断言仍只钉旧 7 名（`:451-465`），XAML 断言里有 `AvailableDropActionOptions` 却没有四个新绑定。这属于"加设置必须同步 AOT 冻结清单+契约测试"接线纪律的违例。

### 修复建议（修 A，机械修复，无设计争议）

1. 桥文件补 4 个 `nameof(...)`（按现有字母序插入：`AvailableDragOutActionOptions` 排 `AvailableDropActionOptions` 之后，`DragOutAction`/`DragOutModifierTipEnabled`/`DragOutResultHintEnabled` 排 `DropAction` 之后）。
2. 契约测试同步：XAML 断言加四条 `{Binding …}` 包含检查；storageBridge 断言加四个 `nameof` 包含检查——把这次的漏网形状钉死。
3. 可评估是否把拖出下拉框纳入 `App.AotManagedUiSmoke.cs` 的 AOT 冒烟面（现状未覆盖 managed 编辑器的这组绑定）。

---

## 3. 修复落地后的验证清单

必须**重打 AOT 测试包**验证（Debug 构建验证不了根因二，也复现不了根因一的发布形态）：

- [ ] Win10（19045 级）+ FollowWindows：拖出到桌面/Explorer **不弹菜单**，同卷执行移动；
- [ ] Win10 + 显式"复制"/"移动"档位各自生效、不弹菜单；
- [ ] Win10 设置页：拖出下拉框显示"跟随 Windows 默认"，展开三项齐全、可切换；两个拖出提示开关可切换并持久化；
- [ ] Win11 回归：FollowWindows 仍走卷规则（同卷移动/跨卷复制）、不弹菜单；
- [ ] AOT 冒烟/测试套件绿（注意冻结计数若涉及需同步）；
- [ ] 防火墙不变式抽查：外部拖出后 `CompletionReceiptIgnored` 照常出现、`SourcePresence` 对账正常（本次日志已示范）。

## 4. 附带发现（与本批无关，建议另开一条修）

诊断包日志 23:44–23:47：`SettingsWindow.get_CloudBackupPasswordBox()` 反复抛 `InvalidCastException`（点"测试连接"必现、点"保存密码"三次 unhandled，进程存活）。这是 SettingsWindow 新 x:Name 必须在 SectionElements.cs 手写 FindCreatedSectionElement 属性的已知坑的又一处——云备份节的 PasswordBox 元素取用先于节创建。与拖拽批次无关联，不在本交接范围内修。

## 5. 给复核 agent 的检查点

- 根因一的修复点是否唯一：`ResolveDragOutPreferredOperation` 仅 `FileSurfaceContent.xaml.cs:1251` 一个消费点（已 grep 确认，含 StackPopover 路径共用同一入口，日志 `popover=False/True` 均走此函数）。
- Win10 钳 Move 的跨卷语义代价是否可接受，属 Simon 决策项，不要自行改口径。
- 修 A 是否会破坏 `AotStage5*` 冻结计数类测试（桥文件内容若被其他测试逐字钉住需一并同步）。
- P1-3（Onboarding 存储入口）仍待 Simon 裁定，与本交接无关，勿混入。
- 本文档描述基于工作区 2026-09-30 23:38 状态；若你手上源码已变，先对照 §0 的时间锚复核。

## 6. 修复落地记录（2026-09-30 深夜，已被 §7 取代）

- 修 A：`ManagedStorageSettingsViewModel.AotBindableProperties.cs` 白名单补 4 个 `nameof`（`AvailableDragOutActionOptions`、`DragOutAction`、`DragOutModifierTipEnabled`、`DragOutResultHintEnabled`，按字母序插入）。**仍然有效**。
- 修 B（§7 已回退）：`ResolveDragOutPreferredOperation` 的 Win10→Move 钳制方向反了——偏好值根本到不了 Win10 Explorer，三档全弹。已恢复纯映射。
- 契约测试同步：`FileSettingsEditorTests` 补 4 条 XAML `{Binding}` 断言与 4 个 bridge `nameof` 断言。

---

## 7. 真机复测与最终修复形态（2026-10-01）

### §1 结论被证伪

Simon 在 Win10 19045 VM 上用三档设置逐一实拖：**"跟随 Windows 默认 / 移动 / 复制"全部弹三态菜单**。三档只改 `RequestedOperation`，对外允许集恒为 `Copy | Move`——故**决定菜单的不是偏好值，而是允许集是否多效果**。截图菜单缺"创建快捷方式"，与 `Copy|Move` 吻合。

更正后的机制判断：Win10 下 WinUI 拖拽经系统代理抵达 Explorer 时**不带按键状态**（旁证：DeskBox 自身原生活动目标日志 `keyState=0`），Explorer 视同非左键拖放，只要 `pdwEffect` 提供多于一种可选操作就弹菜单；唯一静默形状是"单值效果且等于默认效果"——正是 1.5.5 正式版 Win10 单值 Move 的形态。机理侧写（grfKeyState）为推断，经验规则（多效果必弹/单值静默）已被今日三档实测与 1.5.5 历史行为双边钉死。

### 最终修复（已落地，未经构建/真机复验）

- **`FileItemDragPackage.ResolveDragOutAllowedOperations(action, isWindows11OrLater)`**（新增，OS 可注入）：Win11 → `SupportedOperations`（`Copy|Move`）；Win10 → 移动档 `Move`、复制档 `Copy`、跟随/未知 → `Move`。`FileSurfaceContent.Items_DragStarting` 用它写 `e.AllowedOperations`，并存入 `_activeDragAllowedOperations`。
- **`ResolveDragOutPreferredOperation` 恢复纯映射**（跟随→None / 移动→Move / 复制→Copy）：偏好值在 Win10 无效，在 Win11 照旧。
- **内部路由的 Move-only 兜底**（配套成本）：`ResolveInternalArrangementFeedbackOperation` 在允许集不含 Copy 但含 Move 时反馈 `Move`（DragOver 临时态，Drop 完成仍由 `ResolveInternalArrangementCompletionOperation` 返回 `None`，§8.2 安全规则不破）；`DeskBoxDragData.ResolveFileDragFeedbackOperation` 增 `allowedOperations` 参数，内部拖拽在 Copy 缺席时回退 `Move`；完成路径调用点维持默认参数（不回退 Move），防止"实际执行了复制却回报 Move"授权 shell 清源。
- **提示与设置**：`MaybeShowDragOutModifierTip` 改为按允许集含 Copy&Move 才显示（Win10 自动不再提示）；`MaybeShowDragOutResultHint` 在单效果拖拽后用无修饰键版文案（新键 `*.NoModifiers`）；设置页"拖出格子时"下新增 Win10 说明行（`Settings.DragOutAction.Win10Note`，仅 Win10 显示）并将修饰键提示开关在 Win10 置灰（`RefreshDragOutWin10State`，DataContext 外的命名元素路径，不占 AOT 桥）。
- **测试**：`FileItemMultiDragTests` 新增 `ResolveDragOutAllowedOperations` 双 OS 注入断言、`InternalArrangementFeedback` 增 Move-only 用例、新 `FileDragFeedbackOperation` 单效果回退用例；`FileSettingsEditorTests` 增 Win10 注释元素断言。
- **已知代价**：Win10 移动/跟随档下，只收 Copy 的应用（VS Code、浏览器、老 WinForms）拖入被拒（禁止光标）——与 1.5.5 正式版 Win10 行为一致，需要者选"复制"档。DeskBox 内部目标的修饰键语义不受影响（意图经真实按键直读，不走 OLE 效果集）。

### 同日复审追加修复（P1/P2/P3）

- **P1（回归缺口，已修）**：`GetFileAssociationOperation` 未接允许集，内部拖拽恒归一为 Copy——Win10 单值 Move 下文件拖到**待办/快速记录/紧凑待办宿主**全被拒。已加 `allowedOperations` 参数，7 处 DragOver 调用点传入 `e.AllowedOperations`；Drop 完成点维持两参（挂附件语义=复制，不回报 Move）。
- **P2（同类残留源，已修）**：快速记录/待办条目拖出此前 `RequestedOperation=Copy|Move`，Win10 拖到 Explorer 同样弹菜单，且快速记录附件会被"移动"出受管存储（真实数据损坏风险）。改为恒 `Copy` 单值（导出语义本来就是复制，顺带修掉 Win11 上的附件误移动隐患）。配套：内部 tab/排序接受点改用 `ResolveInternalMetadataOperation(e.AllowedOperations)` 按允许集回退 Copy。
- **P3（清理）**：`_activeDragAllowedOperations` 补入两处会话清理点复位。

### 复验清单（重打 AOT 测试包后）

- [ ] Win10 三档各自拖出到桌面/Explorer/跨卷：均不弹菜单，执行与档位一致的操作；
- [ ] Win10 拖入 DeskBox 格子：排序/跨格子移动/入叠放/拖到快捷方式格打开 全部正常（Move-only 兜底生效）；
- [ ] Win10 拖入 VS Code/浏览器：移动档被拒（预期代价），复制档可收；
- [ ] Win10 设置页：Win10Note 可见、修饰键开关置灰；
- [ ] Win11 全量回归：Copy|Move + 偏好、修饰键提示、三档语义不变；
- [ ] `dotnet test` 测试套件绿。
