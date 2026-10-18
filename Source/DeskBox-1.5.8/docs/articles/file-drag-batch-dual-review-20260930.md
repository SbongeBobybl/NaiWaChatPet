# 文件拖拽批次双审查交叉核对报告（2026-09-30）

> **用途**：本文档合并了两次独立审查的结论，供负责本功能模块的 agent 逐条核对与修复。
> 每条发现标注来源与核对状态：
> - **[双]** = 两次独立审查都发现（置信最高）
> - **[A]** = 仅审查 A（sess_37e5f3b0，稳定性/安全/兼容向）发现
> - **[B]** = 仅审查 B（本会话，拖拽场景四象限向）发现
> - **✅ 已亲证** = 主会话已直接读码确认；**◻ 待核对** = 单边主张，需核对裁定
>
> 行号均为 2026-09-30 工作区（未提交状态）行号。

## 审查背景

- 工作区：DeskBox `D:\project\wingezi`，基线 HEAD `9d5313da`，未提交批次 111 个跟踪文件（+36k/−43k）+ 20 余新文件。
- 四条主线：① 托管存储迁移改全量复制+双端校验（源永不删）；② 文件拖出保护方案 B（`StartDragAsync` + `AllowedOperations=Copy|Move` + `FileDragSourceGuardDataObject`，DeskBox 永不依据回执删源）；③ OnboardingWindow 拆分重构；④ 桌面自动整理策略化。
- 两次审查各自派 5 个并行 agent，合计 10 个审查视角；全量测试 **4578/4578 绿**（0 跳过，x64，1m25s）。
- 测试诚实性裁定（双方一致）：`ManagedStorageMigrationSafetyTests` −1040 行为**真搬家非缩水**，删除的旧断言全部对应有意退役的"移动式迁移"语义；未发现弱化/作弊模式。

## 两份总结论的分歧（需模块 agent 优先注意）

| 项 | 审查 A 结论 | 审查 B 结论 | 分歧说明 |
|---|---|---|---|
| 总体 | 无 P0/P1，可推进 | 无 P0，但有 6 个 P1 | 分歧全在下表 P1 区 |
| 原生 OLE 拖入丢弃 allowedEffects | 未发现 | **P1**（光标说复制实际删源） | B 独有，最重要的一条 |
| 空格子修饰键被忽略 | 未发现 | **P1**（Ctrl 拖入空格子会删源） | B 独有 |
| Onboarding 存储入口消失 | 未发现 | **P1**（与同批文档冲突，需产品裁定） | B 独有 |
| ExplorerLaunchCircuitBreaker | P2（有本地兜底不会封死启动） | P1（误杀后落入反馈#9 定案的挂起路径） | 同一发现，分级分歧 |
| ActiveDeskBoxDragRegistry 残留误吞 | P2 | P2 | 一致 |
| 诊断 schema 6 无测试钉 | P2 | 未发现 | A 独有，已亲证成立 |

---

## P1 清单（审查 B 口径；A 的对应分级已注明）

> **处理进度（2026-09-30 晚）**：P1-1 / P1-2 / P1-5 / P1-6 代码侧已修（见各条"已修"标注与文末修复记录）；P1-3 待 Simon 裁定未动。

### P1-1 [B]✅已修 原生 OLE 拖入路径丢弃源公告的 allowedEffects
- **证据**：`src/DeskBox/Helpers/NativeDropEffectPolicy.cs:81-98`（`ShouldCopyMappedTransfer` 调 `ResolveMappedTransfer` 不传 canCopy/canMove，默认 true/true）→ `NativeDropTarget.cs:422-426` → `ContentWidgetWindow.NativeDragDrop.cs:1159-1234`（`NativeDropIntentEventArgs` 不携带 allowedEffects）→ `FileSurfaceContent.xaml.cs:4014-4043`（导入侧再次默认 canMove=true 重解析）。
- **表现**：只公告 Copy 的源 + 跟随系统档 + 同卷：DragOver 光标显示"复制"（反馈侧用了真实 allowedEffects，`NativeDropTarget.cs:341`），实际却执行移动并删源。反向分歧（Move-only 源跨卷）同样成立。
- **对照组**：XAML 路径 `FileSurfaceContent.ShortcutDrop.cs:49-55` 本批已正确传 `canMove`——两路径行为分裂。
- **核对点**：确认 `NativeDropIntentEventArgs` 补字段并贯穿 `ScheduleNativeFileDropFallback → QueueNativeFileDropImport → ImportNativeDroppedFilesAsync`。

### P1-2 [B]✅已修 未映射（空）格子修饰键被整体忽略
- **证据**：`src/DeskBox/Helpers/FileDropIntentPolicy.cs:33-36`（`!hasMappedFolder` 在修饰键判断前 `return Reference` 短路）→ `FileSurfaceContent.xaml.cs:2809-2813`（未映射时 `moveWhenMapped=null`）→ `WidgetViewModel.LayoutAndSettings.cs:93-122`（`?? ShouldMoveManagedItems(...)` 只看设置档，默认 Move）。
- **表现**：Ctrl 拖入**空格子**（首次使用格子的典型时刻）→ 移动并删源，用户明确按了 Ctrl；Copy 档 + Shift → 复制而非移动。XAML 与原生路径同病。
- **核对点**：`Reference` 应只作无修饰默认，不应吞掉 Ctrl/Shift 分支；或把修饰键解析结果传入 `moveWhenMapped`。

### P1-3 [B]◻未处理（待 Simon 裁定）Onboarding 存储迁移入口被删，同批文档仍宣称覆盖首次引导
- **证据**：旧 `OnboardingWindow.Storage.cs`（HEAD，289 行）含系统盘警告 + `ManagedStorageMigrationDialog.RunAsync` 触发；新 Scene/Steps 零 storage 引用；全应用迁移入口只剩 `SettingsWindow.StorageAndUpdates.cs:60`。而 `docs/architecture/managed-storage-copy-migration-20260929.md:3` 写"范围为设置页**和首次引导**"。
- **核对点**：**需 Simon 裁定**——有意裁剪则改两处文档（迁移文档 + architecture-optimization-progress），误删则在新 Onboarding 恢复存储步骤。

### P1-4 [双]✓ ExplorerLaunchCircuitBreaker 阈值 1 + 永不自愈（A 标 P2，B 标 P1）
- **证据**：`src/DeskBox/Helpers/ExplorerLaunchCircuitBreaker.cs:21`（`RpcFailureThreshold = 1`）、`:97-127`（唯一复位条件 = shell PID 变化）。
- **表现**：单次瞬时 RPC 失败（登录高峰/UAC 模态即可）→ 进程生命周期内永久旁路 explorer-hosted 启动；降级到的本地 `ShellExecuteEx` 正是反馈 #9 定案过的挂起路径。托盘同类策略 `App.Tray.cs:1177` 取阈值 2。
- **核对点**：阈值提到 2-3，或加 half-open 探测 / RecordSuccess 冷却期解闸。

### P1-5 [B]✅已修 `.scratch/`（31MB）未跟踪且不在 .gitignore
- 含 fix1~8.py、约 30 张截图、bin/obj、exe/pdb。任何 `git add -A` 都会卷进仓库。**已亲证存在**。
- **处理**：.gitignore 加 `.scratch/` 或提交前删除。

### P1-6 [B]✅已修（代码侧）→ 真机证伪 → 已再修（Win10 钳单值 Move）
- **证据**：`FileItemDragPackage.cs:133`（`RequestedOperation=None`）+ `FileDragSourceGuardDataObject.cs:133-151`（对 Preferred 原样存储并回答，含 0 值，无进程区分）。WinUI 在 None 时是否写 `CFSTR_PREFERREDDROPEFFECT=0` 无法静态证实；Win10 Explorer/微信对"格式存在但值为 0"的解释是"Win10/11 同一路径"论证的唯一未验证变量。文档验收条件 1 自述未完成。
- **核对点**：真机矩阵专测一条（Win10 + FollowWindows + 拖出→桌面/Explorer/微信，确认不弹菜单、默认走卷规则）；或 guard 层把 0 值 preferred 降级为不回答（`DV_E_FORMATETC`）。
- **真机结论（2026-09-30 深夜，anim2 AOT 包，Win10 19045，诊断包 20260930-234554）**：证伪。guard 单值修复后（格式缺席）Win10 仍弹三态菜单——分歧点不在"0 值怎么解释"，而在 Win10 shell 对**无偏好的非 Shell 源**本来就要问。同包还暴露 AOT 桥白名单漏 4 个拖出绑定（设置下拉框空白+选项空）。两处均已修，全链排查与修复方案见 `docs/articles/win10-drag-out-incident-20260930.md`（§6 修复记录），Win10 专项清单已按钳制口径改写入 file-drag-source-safety-20260929.md。

---

## P2 清单（两次审查合并去重）

### 拖拽-入站
1. **[B]◻ WM_DROPFILES 与 OLE 回退导入 120ms 竞态**：`ContentWidgetWindow.NativeDragDrop.cs:717-753` 直接 `QueueNativeFileDropImport` 不查 `_lastXamlFileDropUtc`（该守卫只在 `ScheduleNativeFileDropFallback`:1462-1477）；慢盘/UNC 下首导入的 Task.Run 探测超 120ms 即双重导入（复制档产 "(2)" 副本）。
2. **[B]◻ 拖文件夹到待办留空孤儿项**：`TodoWidgetContent.DragDrop.cs:841-857` 先 `AddItemAsync` 建项再过滤，`addedCount==0` 时 `return false` 不回滚；对照组 QuickCapture（`QuickCaptureService.cs:536-544`）先过滤后建项，无此问题。
3. **[B]◻ Move-only 源 Alt 反馈误显"移动"**：`NativeDropEffectPolicy.cs:74-77` Shortcut 意图在 Link/Copy 未公告时降级显示 Move，实际建快捷方式。纯视觉。
4. **[B]◻ 原生反馈对空格子硬编码 `hasMappedFolder:true`**（`NativeDropEffectPolicy.cs:56`），与 XAML 路径（真实 MappedFolderPath）描述不一致。

### 拖拽-出站
5. **[双]✓ ActiveDeskBoxDragRegistry 残留误吞外来拖入**：`ActiveDeskBoxDragRegistry.cs:19`（TTL 10 分钟）+ `NativeDragDrop.cs:1182-1206/1064-1072` 命中即静默 return。丢 DropCompleted 的现实触发器：拖拽模态循环中 watcher 刷新致容器回收、`Items_DragStarting` 1214-1243 间异常。**另**：`Items_DragStarting:1070-1071` 开新会话只 `Complete` 了 FileDragSessionState，未 `End` 注册表旧条目（B 补充发现）。
6. **[B]◻ Esc 取消的拖出必走 Full 11 轮观察 + 看门狗告警**：`FileSurfaceContent.xaml.cs:1474-1481, 1585-1591`，取消是常见结局，每次产告警噪音稀释真信号。
7. **[B]◻ 多位 Preferred DropEffect 防线完全依赖 WASDK 行为约定**：guard 注释明言 No normalization（`FileDragSourceGuardDataObject.cs:44-48,133-147`）；旧 `PreferredDropEffectFilterDataObject` 的 CoGetCallerTID 隐藏防线已删。若 WASDK 升级后 StartDragAsync 从 AllowedOperations 派生多位 preferred，Win10 弹菜单回归且无报警。建议 guard 加钳制或把"Win10+FollowWindows 不弹菜单"钉为必测项。
8. **[B]◻ QuickCapture 仍走 ListViewBase 内置拖拽 + 多位 `RequestedOperation=Copy|Move`**：`QuickCaptureSurfaceContent.xaml:160`、`.xaml.cs:2231-2233,2256,2579`——正是本批消灭的病理（存量非回归，但与方案 B 哲学不一致，建议后续批统一）。
9. **[B]◻ 托管快捷方式专用 `Move|Link` 操作集被删**：跨卷拖回桌面从"还原"退化为"复制"（`FileItemDragPackage.cs` diff 删除了 `ManagedShortcutSupportedOperations`）。`isManagedShortcutDrag` 变量在 `xaml.cs:1250` 处已算出但只用于日志，有现成开关点可强制 preferred=Move。文档未记录此变化。

### 迁移与整理
10. **[A]✅ 延迟档切换不重排已入队条目**：`DesktopAutoOrganizationWatcher.cs` OnSettingsChanged（约 :126-153）只处理启停；入队时 `MarkDeferred(... GetDelay(...))`（约 :487-493）定死时间。12h 档切实时后已入队条目仍等 12h。**已亲证**。
11. **[A]✅ 迁移停止后改名组件 → 重选同目标卡死**：`ManagedStorageMigrationService.cs:123` 续传要求 journal OldRoot/NewRoot/Folders 与当前严格相等（`SequenceEqual`），改名后抛 SourceChanged；destination 未删时也无 Abandoned 出口。**已亲证**（"死循环"程度偏重，实际是"卡住直到删目标目录或恢复组件名"，但核心主张成立）。
12. **[A]✅ 迁移 .partial 残留无回收出口**：`:253` 每次重试新 GUID partial 从不清理（注释明言有意保留），孤立目录清理显式排除 `.deskbox-migration`；叠加空间预检只算文件总量不计残留（与 B 的 OBS 互证），磁盘近满时可被 DiskFull 锁死。**已亲证注释与排除逻辑**。
13. **[A]✅ 诊断包 schema 6 无测试钉住**：`App.DiagnosticsBundle.cs:36` 写 `SchemaVersion: 6`，tests/ 全量 grep 无任何断言诊断 schema 版本的测试（A 说"测试仍断言 5"细节存疑，但"改回 5 不会红"成立）。**已亲证**。
14. **[B]◻ 迁移源根也被强制 NTFS**：`ManagedStorageMigrationService.cs:94` 对 SourceFolder 也 `RequireSupportedVolume`，文档只对目标声明 NTFS；exFAT/FAT32 旧根用户被拒。方向安全，文档/文案需补。
15. **[B]◻ 诊断包内容缺口**：未收录豁免 ledger（`desktop-organization-suppressions.json` 在册 claim）、熔断器降级状态、活跃拖拽会话——恰是三类新故障的直接答案。

### 兼容与平台
16. **[B]◻ OneDrive 云占位使迁移拒绝且文案误导**：`FileService.StorageCopy.cs:44-47` 把 ReparsePoint 一律叫 "link"；OneDrive 占位实为 cloud filter tag，文案应区分并提示"设为始终保留在此设备"。
17. **[双]✓ `WritePerformedDropEffect` 用 AllocHGlobal 充当 HGLOBAL 介质**（A 标 P3，B 标 P2）：`NativeDropTarget.cs:576-620`；规范做法 `Win32Helper.TryCreateGlobalMemory`（GMEM_MOVEABLE）就在旁边。
18. **[B]◻ `CopyFileExW` 回调未区分 reason**：`StorageMigrationNativeMethods.cs:92`，空流的 STREAM_SWITCHED 也 flush，flush 失败中断整批。
19. **[B]◻ `AreSameVolume` 根字符串比较**：`FileDropIntentPolicy.cs:111-125`，mount point/subst 下光标误报"移动"（执行层正确，纯提示）。
20. **[A]◻ FileDragSafetyTests 封送盲区**：仅同单元、仅 HGLOBAL、未进真实 DoDragDrop 循环——与 B 的"验收条件 1 未完成"互证。
21. **[A]◻ 托盘交互门闩提前重武装 / 启动恢复循环单条损坏整体跳过**（A 的 P3，升列在此防漏）。

### 设置、文档与卫生
22. **[双]✓ ManagedDragOutAction 写单值 preferred 与文档口径不符**：safety 文档写"RequestedOperation=None 不给偏好"（绝对表述），实际三档设置会覆盖为 Move/Copy（`xaml.cs:1250-1252`，唯一消费点**无测试钉住**，删掉 3 行测试仍绿）。A 标 P3，B 标 P2 + 文档缺口。
23. **[B]◻ 文档日期异常**：三处未来日期 2026-10-12（`file-drag-source-safety-20260929.md:5`、`file-drag-explorer-move-reclaim-20260929.md:3`、`SettingsSliceOwnershipContractTests.cs:197` 注释），今天 09-30。
24. **[B]◻ 豁免指纹条目寿命文档失真**：代码 ≥13h（`DesktopAutoOrganizationSuppressionRegistry.cs:21,163-166`），文档写"+60min settle"。
25. **[B]◻ `json-source-generation-baseline.md` 计数失真**：文档 36/85/33 vs 测试冻结 37/87/33（漏回填豁免 ledger）。
26. **[B]◻ 12 语言死键**：`Settings.Dialog.MigrateResidue*` 等旧键零引用仍保留全部 12 语言；新增 4 个 SafeMigration 死键；`IManagedStorageSettings.cs:47-61` 注释仍描述已删的 residue/rollback 链。
27. **[B]◻ 9 语言迁移文案英文兜底**（文档已声明，发版前复查）。
28. **[B]◻ DragOut 三张设置卡未进设置搜索目录**：`SettingsSearchCatalog.cs:79-80`。
29. **[B]◻ `CommitRootPath`/`SetDefaultRootPath` 无生产调用方**（仅测试引用）。
30. **[B]◻ `GetArrivalNameCandidates` 点文件候选错误 + 无测试**：产出 `" (2).gitignore"`，Explorer 实际 `.gitignore (2)`。
31. **[B]◻ guard 的 `s_completionReceiptSequence` 自增后无读取方**（`FileDragSourceGuardDataObject.cs:220,335`），自称诊断实为死代码。

---

## 需真机验证清单（两次审查共同要求）

1. **Win10 21H2 + FollowWindows 拖出矩阵**（P1-6）：桌面/Explorer/微信，确认不弹操作菜单、卷规则生效。
2. **可控 OLE 接收端矩阵**（文档验收条件 1）：误报 Move/取消/拒收/延迟读取/退出/多选部分完成 × Win10/11 × 两种叠放模式；微信只用一次性测试文件，原路径+内容哈希必须保持。
3. **StackPopover 弹层外部拖入可达性**：弹层是独立顶层 HWND，全仓无 RegisterDragDrop 指向它，依赖 WinUI 自注册——Explorer 拖文件到打开的弹层实测一次。
4. 大文件拖出时序、跨进程封送（A 的 WinUI/Shell 语义假设项）。

## 已被驳回的主张（勿再追）

- **审查 A 的 agent 曾报 P1"桌面到达豁免 CreationTime 证据在慢速拖出失效"——复核不成立已降 P3**：登记发生在 DropCompleted 之后，目标文件已存在，走指纹分支（size+mtime+creation，`SuppressDraggedArrivals:190`），不查登记后创建时间。模块 agent 无需再查此条。

## 两次审查一致确认无恙的核心不变量（修复时不得破坏）

1. 完成回执（Performed/Logical Performed DropEffect、Paste Succeeded、TargetCLSID）永不抵达内层 Shell 对象（`FileDragSourceGuardDataObject.cs:210-241`，测试钉住）。
2. DeskBox 无任何由 DropResult/回执触发的删除/移动调用（全仓调用点逐一核对，双向独立确认）。
3. 内部路由反馈/完成永不返回 Move（`DeskBoxDragData.cs:157-163` 等 5 处赋值点 + `FileItemMultiDragTests:118-176`）。
4. 复制迁移源永不删除；失败/取消/磁盘满全程保源；`Committing` 中断有 journal 幂等恢复。
5. fRelease/介质所有权、x64/ARM64 结构布局正确；guard 包装失败回退 StorageItems，不空载荷出厂。
6. 删除类型（`NativeFileDragOut`/`PreferredDropEffectFilterDataObject`/`ResidueDialog` 等）零悬挂引用；已撤销的第三批 reclaim 无代码残留。
7. AOT 红线未触（新 COM 全 source-gen、无集合表达式绑定进封送）；冻结计数经独立复算一致。
8. 拖入路径解析（junction 逐段）、自包含循环防护、重名 "(2)"、IFileOperation 跨卷链路原样有效。

## 建议处理顺序

1. P1-1 / P1-2（两条入站行为 bug，本批质量核心）→ P1-6 真机项。
2. P1-3 待 Simon 裁定后执行；P1-5 我可直接处理。
3. P2 中 [双]/✅ 项优先（5、10-13、17、22），单边 ◻ 项先核对再修。
4. 文档勘误（14、16、22-26）可一批顺手做。

---

## 修复记录（2026-09-30 晚，按上述顺序执行）

**P1-1 已修**：`allowedEffects` 全链贯穿。`NativeDropIntentEventArgs` 新增 `AllowedEffects` 字段；`NativeDropTarget.OnDrop` 用 `canCopy/canMove`（含 sameVolume）算 `copyRequested`；`NativeFileDropTarget_DropIntentEvent` 的 forcedIntent 同样受约束门控；`ScheduleNativeFileDropFallback → QueueNativeFileDropImport → ImportNativeFileDropAsync → ImportNativeDroppedFilesAsync` 全链传 `uint? allowedEffects`（WM_DROPFILES 入口无 OLE 协商，保持 null=未知即双许可）；`ImportNativeDroppedFilesAsync` 的两处 `ResolveMappedTransfer` 重解析均传约束。右键菜单路径同样携带。反馈与传输决策现在共用同一约束。

**P1-2 已修**：`FileDropIntentPolicy.ResolveMappedTransfer` 对 `!hasMappedFolder` 不再先短路 `Reference`——修饰键照常解析（Ctrl→Copy、Shift→Move、Alt/Ctrl+Shift→Shortcut、forceCopy→Copy），仅无修饰默认回落 Reference（由设置档决定）。三处消费点改用共享 `ResolveMoveWhenMapped(mapped, intent)`（Root_Drop / 叠放面 / 原生导入），显式 Copy/Move 意图在未映射格子也生效；`ResolveShortcutIntentOverride` 保证未映射格子上的 Alt 拖入（无快捷方式目的地）回落到导入而非静默无操作。附带修复：未映射格子 + 临时/虚拟载荷此前会走 Reference→设置档（Move 档会移动临时文件），现在强制 Copy。

**P1-6 代码侧已修**：`FileDragSourceGuardDataObject` 只对单位值（Copy=1/Move=2/Link=4）的 Preferred DropEffect 存储并回答；`0`（DROPEFFECT_NONE）与多位掩码视为引擎的"无偏好"拼法，保持格式不存在（等价 Shell 原生源无偏好），且后续不可服务值会清掉先前的有效偏好。Win10 专项真机清单已落入 file-drag-source-safety-20260929.md 验收条件节（含 StackPopover 弹层外部拖入可达性一项）。

**P1-5 已修**：`.gitignore` 追加 `.scratch/`。

**测试**：新增/更新——`FileInteractionPolicyTests`（未映射修饰键矩阵 4 例 + 无修饰 Reference + 临时载荷 Copy + 无权修饰键拒斥 None）；`NativeDropEffectPolicyTests`（反馈与传输在源约束下一致、跨卷约束、修饰键不可越权 3 个新测试，原"反馈 Copy 传输 Move"的分歧钉测改为一致性断言）；`FileDragSafetyTests`（0/多位 preferred 永不回答 ×3、不可服务值清除先前偏好）；`AotStage5B4C1C2AContractTests` 锚点从 `bool? moveWhenMapped = mapped` 更新为 `ResolveMoveWhenMapped(`。全量 x64：**4581/4590 绿**，9 个失败全部归属并行会话（8 个设置冻结计数=动画线加成员未同步计数；1 个 WatcherDelay(300) 负载抖动，单独重跑 4/4 绿）。拖拽相关测试零失败。

**文档**：file-drag-source-safety-20260929.md 同步——偏好三档说明（FollowWindows=None 默认 / Move/Copy 单值偏好）、Preferred 单值才存储并回答的新规则、验证节测试清单、Win10 专项验收清单。

**P1-3 未动**：等产品裁定（恢复 Onboarding 存储步骤 vs 改两处文档口径）。

**2026-09-30 深夜补修（P1-6 真机证伪后，独立于上文修复轮）**：①`FileItemDragPackage.ResolveDragOutPreferredOperation` 增加可注入 `bool? isWindows11OrLater`——`FollowWindows`（及未知值）Win11 解析 `None`、Win10 解析单值 `Move`，显式 `Move`/`Copy` 两 OS 不变；调用点仍唯一（`FileSurfaceContent.xaml.cs`）。②`ManagedStorageSettingsViewModel.AotBindableProperties.cs` 白名单补 4 个漏加的 `nameof`（`DragOutAction`/`AvailableDragOutActionOptions`/`DragOutModifierTipEnabled`/`DragOutResultHintEnabled`——AOT 构建下拉框空白+选项空的根因）。③契约测试同步：`FileItemMultiDragTests` 双 OS 分支注入断言（10 用例）；`FileSettingsEditorTests` 补 4 条 XAML 绑定断言 + 4 个桥名断言。④文档同步：file-drag-source-safety-20260929.md（概述/实现/边界/Win10 清单四处按分档口径改写）。全量 x64 **4605/4605 绿**。完整排查链与验证清单=`docs/articles/win10-drag-out-incident-20260930.md`。
