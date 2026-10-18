# 01 · 代码审查报告（v1.5.5 → 9d5313da + 未提交工作区）

> 方法：对抗性审查——每条发现附 file:line 证据与置信度；每个"已修复"主张对码验证而非采信提交信息；每个可疑点要么定罪要么证伪。承重结论经主会话二次复核。

---

## 一、发现清单（P0 → P2）

### P0：未发现

文件安全不变量全部在位（证据见 §二.15-17）。这是本轮最重要的阴性结论。

### P1（用户大概率触发的功能缺陷）

**P1-1 云备份密码框 getter 在 AOT 下抛 InvalidCastException — ✅ 2026-10-01 已修（终局证明待 AOT 零售包）**
- 原 symptom：`SettingsWindow.SectionElements.cs` 的 `CloudBackupPasswordBox` getter 强制非空 + 消费点直接解引用，AOT 零售包上点"测试连接/保存密码"必炸（真机日志实锤）。
- **根因（10-01 定案）**：PasswordBox 是全应用唯一零 IL 引用的延迟节控件（无 x:Bind/handler/从未 new）→ NativeAOT 裁掉其投影类型反射根 → CsWinRT 按运行时类名解析时走 trim fallback 把 RCW 包装成基类并按 COM 身份缓存 → `is T` 与强转永远失败。JIT 靠完整反射不发病；**09-22 的 FromAbi 重包装修法被证实结构性无效**（换不回身份缓存的错型 RCW）。
- 修法：`[DynamicDependency(All, typeof(PasswordBox))]` 显式保根 + getter 可空 + 两处消费守卫（App.Log 可见）+ AotDeepSmoke 路由后断言 + 4 条结构契约测试。

### P2（边界/潜伏）

**P2-1 拖出结果提示恒用"无修饰键"文案 — ✅ 2026-10-01 已修**
- 原缺陷：`FileSurfaceContent.xaml.cs` 在 `CompleteDragItemsSession` 内先把 `_activeDragAllowedOperations = None` 复位，之后才调 `MaybeShowDragOutResultHint`（读该字段算 `modifiersCouldFlip`）→ 恒 false → Win11 上"按 Ctrl 可复制"教学变体永不出现。
- 修法：复位前捕获允许集传参 + 提取 `GetDragOutResultHintKey` 纯函数 + 13 例矩阵与 Win10 组合测试（Win10 单比特行为不变有钉）。

**P2-2 `WritePerformedDropEffect` HGLOBAL 分配方式与全库约定不一致**
- `src/DeskBox/Helpers/NativeDropTarget.cs:600` 用 `AllocHGlobal`（LMEM_FIXED）构造 medium；库内其它 HGLOBAL 生产点（`Win32Helper.cs:688-713`）用 `GlobalAlloc(GMEM_MOVEABLE)`。现代 Windows 两堆合一大概率无害，潜伏 ABI 摩擦，记录备查。置信度：低-中。

**P2-3 `QuickCaptureClipboardService.Dispose` 无排水 cancel-then-dispose**
- `src/DeskBox/Services/QuickCaptureClipboardService.cs:88-94`：同步 `Stop()` 后立即 Dispose token，不等待在飞读排空——与 `9d5313da` 修掉的 BackupSettingsViewModel 同类。仅关停路径可触达，异常大概率被读循环吞掉。置信度：中；影响：低。

**P2-4 `settings-flush` 关停步骤无 deadline**
- `src/DeskBox/App.xaml.cs:4675-4679` 用裸 `new ShutdownStep`（非 `Bounded`），`FlushPendingSaveAsync` 挂起则关停链挂起。设计上有意（写一半被杀=settings 撕裂），但与 f9305dab 的主张不完全一致。置信度：低。

**P2-5（纪律观察）自动整理间隔下拉绕过协调器**
- `src/DeskBox/Views/SettingsSections/DesktopOrganizationSettingsSection.xaml.cs:200-217` 直接写 `service.Settings.DesktopOrganization.<字段>` + `SaveAsync()`，本批其它新设置全走协调器。无功能问题。

**P2-6 拖出设置三卡不在设置搜索目录** ★已二次复核实锤
- `src/DeskBox/Services/SettingsSearchCatalog.cs` grep "DragOut" = 0。设置搜索搜不到"拖出"。需重跑 `scripts/update-settings-search-catalog.ps1` 或手工补条目（注意目录键名字符串会撞门面访问棘轮正则，清单要 +2）。

### 过程性风险（非代码缺陷）

- **Win10 拖出终态（含单比特公告）未构建 AOT 包、未真机复验**：`win10-drag-out-incident-20260930.md:137` 自述"已落地，未经构建/真机复验"，且 Debug 构建验证不了 AOT 桥形态（历史两次实证）。发版前 §7 六项清单必须跑完。
- **CHANGELOG.md Unreleased 段重复两次**（第 3 行与第 18 行逐字相同）★实锤，且下版内容大量缺录（见 03 文档 A2）。

---

## 二、已核实为真修复（逐条对码，非采信提交信息）

1. **Win10 单比特公告在位**：`FileItemDragPackage.cs:54-70`（Win11→Copy|Move；Win10→单值 Move/Copy）、`ResolveDragOutPreferredOperation` 纯映射（`:35-41`）、宿主 `FileSurfaceContent.xaml.cs:1254-1262` 分写 AllowedOperations/RequestedOperation——与事件文档 §7 一致。
2. **证伪方案删除干净**：`NativeFileDragOut` / `PreferredDropEffectFilterDataObject` 全库零引用。替代物 `FileDragSourceGuardDataObject`：单值 1/2/4 才应答偏好（`:133-134`），完成回执四格式全吞（`:113-123`），CF_HDROP 自检（`NativeShellFileDragProvider.cs:52-65`）。
3. **自拖识别迁移完整**：`ActiveDeskBoxDragRegistry`（10 分钟兜底寿命 + cancel/complete 双点 End）在 `ContentWidgetWindow.NativeDragDrop.cs` 三处决策全部接通，无第三方文件误分类窗口。
4. **允许集贯通到传输决策**：`NativeDropTarget.OnDrop` 的 sourceCanCopy/Move（`:427-436`）、forced intent 受约束、`GetFileAssociationOperation` 7 个 DragOver 调用点全传参（15 个调用点逐一核对）。内部完成操作仍永不返回 Move。
5. **Link-only 源不再当可移动**（`FileSurfaceContent.ShortcutDrop.cs:48-60`）+ `ResolveMoveWhenBlocked` 保住显式修饰键语义。
6. **随记/待办拖出恒单值 Copy**：`TodoWidgetContent.DragDrop.cs:480,502`、`QuickCaptureSurfaceContent.xaml.cs:2235,2434`——关掉附件被"移动"出受管存储的隐患。
7. **AOT 桥白名单全补**：ManagedStorage +4、Appearance +2（Stagger）、壳 VM +SilentStartup；契约测试同步。
8. **修饰键提示门控**：要求 Copy&Move 双位（`FileSurfaceContent.xaml.cs:2556-2564`）；12 语言 9 个 DragOutTip 键 + Win10Note 齐全；Win10 置灰走命名元素不占桥面。
9. **跨卷移动仍走 IFileOperation**：`FileService.cs:1484-1534` 路由未动，AOT 分支划分保留。
10. **2b6b2e0d 三处均为真修复**：① accent 极性 `== Custom`（`AppearanceSettingsViewModel.cs:381`）+ 壳先同步再刷预览防回弹（`SettingsViewModel.AppearanceOptions.cs:92`）；② Todo 字号壳订阅在 `SettingsViewModel.cs:272-273`；③ 胶囊两卡重绑到编辑器活属性（桥白名单 `:47/:51`）。**同类扫描**：全库 6 个 `*Committed` 事件全有订阅，无孤儿。
11. **9d5313da 为真修复非掩盖**：`RetireCancellationScope` 排水后才 Dispose（`BackupSettingsViewModel.cs:120-136`）；`TrackRead` 覆盖全部 4 个发射点；附回归测试。
12. **关停可靠性双提交达标**：f2ae1802 watchdog 路径不释放互斥体（`App.xaml.cs:4538-4554`）；f9305dab `ShutdownStep.Bounded` 15s 宽限 + 有意不设限处写明理由（`:4617-4621`）。
13. **动画批无"白名单洗掉"复发**：新效果三处集合一致（编辑器选项 `AppearanceSettingsViewModel.cs:91-100`、持久化白名单 `SettingsService.cs:1587-1598`、双归一化器）；`UsesSlideDirection` 在 Contracts；`SlideFade` 名不副实=已知名义债务且加载时迁移自洽。
14. **CsWinRT IVectorView 危害规避在位**：新代码唯一 WinRT 向量枚举（`WidgetSlideBoundaryPolicy.cs:80-85`）用索引循环并附注释；托盘动画文件 foreach 均为托管集合。
15. **文件安全不变量**：取消=保留已完成项+拒绝回滚删除（`FileService.TransferProgress.cs:192-214`）；删除源验证句柄+目录非递归（`FileService.cs:2781-2793`）；迁移=复制+`originalsRetained=true`+journal 断电恢复（`WidgetManager.StorageMigration.cs:88,115-157`）；拖出对账纯取证绝不删源。
16. **Schema/门面一致**：schemaVersion 仍 9；6 个新字段全为加法+默认+归一化；冻结计数全部同批有意更新。
17. **XAML 拖拽机制切换自洽**：容器级 CanDrag + 先退订防复用双订阅（`FileSurfaceContent.xaml.cs:828-847`）；旧 Items_DragItemsStarting 已彻底删除无死代码；部分集即取消不广播残缺选择。

---

## 三、证伪 / 无需担心

1. `NativeShellFileDragProvider.SetDataObject` 的 `vtable[4]` 裸调用——v1.5.5 基线原样（git show 确认），真机日志证明现网 ABI 有效，已有自检+回退防线。已接受债务。
2. 删除物悬挂引用（NativeFileDragOut、ResidueDialog、8 个旧 Onboarding partial 等）——全部零引用。
3. foreach-over-IVectorView 旧存量（`e.AddedItems` 等）——v1.5.5 已随正式版发行，非新风险。
4. 意外内容扫描：~155 文件 diff 零新增 TODO/FIXME、零注释掉的代码、零调试残留。
5. "拖出触发 2000× SaveDebounced"——现路径无按 tick 保存，保存只在提交时发生。维持已修复判断。
6. 关停顺序与互斥体释放——f2ae1802 两目标都达成。
7. 托盘菜单双通道单发门闩 + 原生 PopupMenu 终态逃生口——在位（`TrayContextMenuResilienceTests`）。

---

## 四、未提交批次的遗留裁定项（来自双审查，多数未修，发版前过一遍）

- **P1-3**：Onboarding 重构裁掉存储迁移步骤，迁移文档仍宣称覆盖首次引导 → 待产品裁定。
- **P1-4**：`ExplorerLaunchCircuitBreaker` 阈值=1 + 仅 shell PID 变化复位（永不自愈）。
- **P2 ×21**：见 `docs/articles/file-drag-batch-dual-review-20260930.md` P2 清单（120ms 竞态、待办拖文件夹孤儿项、Esc 取消观察噪音、QuickCapture 仍用 ListViewBase 内置拖拽、迁移 .partial 残留、OneDrive 占位文案等）。
- **迁移准入文案**：源盘 exFAT/FAT32 同样被拒但文案只提目标盘。

---

## 五、变更地图（摘要）

**已提交（用户可见）**：提权热键说明条 ×2（#474/#475）；AOT 转换器 partial 双回归修复（#473）；三处 UI 回归修复（2b6b2e0d：accent 极性/Todo 字号/胶囊死绑定）。其余 40+ PR 为内部架构（门面退役 29-51、内容适配器常驻、P/Invoke 全迁、组表面事务化、关停所有权），**设置项零增删移、schemaVersion 9 不变**，对用户无感。

**未提交（将随下版发布）**：
| 批次 | 用户可见变化 | 测试 | 文档 |
|---|---|---|---|
| 拖出重做（最大） | 方案 B 原生 Shell 拖出 + Win10 单比特公告 + 拖出三档设置 + 修饰键/结果提示 + 桌面整理到达豁免 | ✅ 充分 | ✅ 4 份 |
| 收纳迁移复制化 | 复制+SHA-256+永不删源，重写确认/进度对话框，诊断 schema 6 | ✅ 强 | ✅ |
| 动画批次 | 3 新效果+Spring+错峰+邻屏钳制 | 部分（缺口见 04） | ❌ 缺 |
| Onboarding 重构 | 5 步+场景画布（存储步骤被裁） | 重钉 | ❌ 缺 |
| 静默启动 | 常规段新开关 | ✅ | ❌ 缺 |
| 自动整理档位 | 10s~12h 六档 | ✅ | ❌ 缺 |
| 韧性杂项 | 托盘降级、Explorer 熔断、#459 打开分派、空格子帮助 | ✅ | ❌ 缺 |

新增设置字段 6 个（全部加默认值、无迁移）：`SilentStartup`(false)、`ManagedDragOutAction`("FollowWindows")、`DragOutModifierTipEnabled`(true)、`DragOutResultHintEnabled`(true)、`WidgetAnimationStaggerEnabled`(false)、`DesktopAutoOrganizationDelaySeconds`(10)。动画枚举扩容：Effect +EdgeScale/Tilt/Wipe、Easing +Spring。
