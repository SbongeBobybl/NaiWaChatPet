# 04 · 测试项目有效性梳理（tests/DeskBox.Tests，2026-10-01）

> 问题意识：仓库主人被"代理为全绿弱化测试"坑过。本审计用四视角对抗性审查：作弊/弱化扫描、冻结计数合理性、行为覆盖缺口、结构健康。基线：424 个测试文件、3074 个 `[Fact]` + 374 个 `[Theory]`/1457 条 `InlineData`，展开 = 4616（与当前全绿数吻合）。

## 总体结论

**4616 绿是有意义的，不是"为测试而测试"。**
- 作弊扫描**干净**：无 `Assert.True(true)`、无自等断言、无 `Skip=`、无空测试体、无被注释的断言、无 `async void`；仅 2 个空 catch 是 Dispose 清理惯例。
- v1.5.5..HEAD 测试提交 52 个（+14,282/−267）：删除的 267 行逐条核对全是棘轮计数更新与重命名跟随，无整块删除。
- 工作区每处冻结计数变化都能对上真实产品变更（见 §二）。
- 用户最痛的行为域（拖拽不删原件、迁移不丢文件、动画设置不漂移、整理节奏、关机清理）都有真实行为测试，多处测试注释直接对应历史用户可见 bug。

代价：~10%（50 个 Aot 文件、458 facts）是高摩擦的形式化棘轮；另有 5 处 nameof 计数锁步钉和 11 个"元钉"（测试钉测试文本）属维护性隐患；墙钟断言是未来 flaky/弱化的最大诱因。

---

## 一、测试家族地图（判定：有效 / 形式化 / 有害）

| 家族 | 规模 | 判定 | 依据 |
|---|---|---|---|
| 纯函数 Policy（NativeDropEffectPolicy、DesktopAutoOrganizationPolicy、FileDropIntent、Win10FramePacing…） | 50 文件/267 facts | **有效（最佳）** | 输入-输出矩阵直测决策逻辑（如"Shift 不能把只读源强制成移动"） |
| Coordinator 写路径（16 个） | 132 facts | **有效** | 真实磁盘 round-trip（如 SilentStartup 写入→重载） |
| Service 级真实 I/O（FileService 84、SettingsService 70、CloudBackup 61…） | 26 文件 | **有效** | temp 目录真实磁盘/JSON round-trip |
| SettingsSlice/门面契约（3 文件） | ~10 facts | **有效（强）** | 反射透传集↔实际 JSON 序列化成员集**集合相等**，非纯计数 |
| 状态机（5 文件） | — | **有效** | 如"初等待不烧瞬时重试预算"（新增） |
| 迁移 copy 流程（22 facts） | 1 文件 | **有效（强）** | 全失败矩阵：中途取消双保留/提交失败双保留/目标损坏保源/逐文件取消永不删原件 |
| Rust/C# 差分（31） | 1 文件 | **有效** | 真实 native DLL 与 C# 逐位对比 |
| 本地化契约（3） | — | **有效** | 12 语言键/占位符强一致（但对死键/缺失键不设防，见 05 文档） |
| AotStage\* 契约 | 50 文件/458 facts | **形式化偏多** | 文本扫描+计数棘轮，保护真实吃过亏的域，但对运行时语义盲 |
| ↳ 元钉子族（测试钉其他测试文件的断言文本） | 11 个 stage 测试 | **有害（低危）** | 改一次 baseline 要同步 12 处 |
| ↳ MigrationPattern nameof 钉 | 5 处 | **形式化+锁步隐患** | `Assert.Equal(34, nameof( 计数)` 同一数字散布 5 文件无单一事实源（本轮静默启动就踩到：6 处必须同步 +1） |
| XAML 结构 parity（~10 文件） | — | **形式化** | 防"绑定被删"不防"绑到错的东西" |
| 诊断探针（CoGetCallerTidProbeTests） | 1 | **形式化（边际）** | 钉 Win32 API 行为非产品回归 |

## 二、工作区计数变更逐项核对（全部对得上，附注释）

- facade 220→226 ↔ AppSettings 恰好 +6 透传（silentStartup、widgetAnimationStaggerEnabled、managedDragOutAction、两个 drag-out tip、desktopAutoOrganizationDelaySeconds）；
- SettingsCard 182→185 / sum 370→376 ↔ XAML 恰好 +3 卡（整理时机、错峰、静默启动）；
- SettingsViewModel 14→15 ↔ 桥白名单 +`nameof(SilentStartup)`（nameof 33→34 同源）；
- JSON 基线 35→37 / 83→87 ↔ +2 序列化文件各 +2 调用；
- WidgetGroupCloseConfirmation 3→4 ↔ 前景色拾取器第 4 次 handoff（有注释）。

**唯一大删除判定为合理退役**：`ManagedStorageMigrationSafetyTests` −23 facts + `ManagedStorageMigrationDialogTests` 整删（4 facts）——被测 API 已随 move→copy 重构删除（无定义无调用），新流程由 `ManagedStorageCopyMigrationTests` 22 个失败路径测试接替，覆盖面对等甚至更强。**但两个残留风险**：① `UpdateDefaultManagedStorageRootAsync` 本体还在（`WidgetManager.StorageMigration.cs:13`），部分失败语义只剩间接覆盖；② 旧测试保护的"Retry 永不粘滞"用户可见策略随功能消失，新 `ConfirmAsync/RunAsync` 无对应策略测试。

## 三、行为覆盖缺口（按优先级）

1. **动画错峰（新功能，最大缺口）**：唯一运行时消费点 `GetTrayStaggerIntervalMs`（`WidgetManager.TrayAnimation.cs:380`）**零行为测试**；`WidgetTrayBatchAnimationDriver` 无专属测试文件。未保护场景：开关关了仍交错、多窗口间隔算错。
2. **静默启动**：策略+持久化已测；缺 (a) `WidgetManager.cs:904` 消费点的"启动真的隐藏"集成测试；(b) 隐藏后**托盘切换把格子显示出来**的行为（TrayToggleDecisionPolicy 可见性矩阵不含 silent-startup 起始态）。
3. **拖出**：覆盖良好，但 Win10 单比特公告只剩 `NativeDropEffectPolicy` 纯函数层；XAML 侧 `DragOutWin10Note` 只有字符串钉。
4. **elevated-hotkey notice**：文案有保护；两处重复 markup 无 parity 测试（低风险）。
5. **Onboarding 重构**：步骤导航/完成态机无行为测试（现以 XAML/字符串扫描为主）。
6. 已确认覆盖良好：自动整理初延迟不烧预算✓、假时钟 watcher 延迟✓、间隔下限 clamp✓、动画效果持久化 round-trip✓、分组表面 6 阶段回滚✓、关机 deadline 5 项✓、backup CTS 退休✓（含 `Assert.Fail` 防回归逃逸）。

## 四、结构建议（本轮不动代码，仅列行动项）

1. **合并 nameof=34 锁步钉**（AotStage5B4B1 + 4 个 editor MigrationPattern）为单一事实源。
2. **废除元钉**：11 个 stage 测试断言 baseline 测试文件的断言文本。
3. **消灭墙钟断言**（flaky 种子=未来弱化的最大诱因）：`FolderEnumerationPerfTests.cs:34`（<800ms）、`BoundedBackgroundWorkSchedulerTests.cs:84`（<2s）；真睡等待：FileDragSafety 150ms、ElevatedFileLauncher 200ms、DeskBoxDataBackup 750ms×2、WatcherDelay 300ms×3。
4. **拆超大文件**：FileServiceTests 2682 行、FileSurfaceParity 2117、CloudBackupScoped 1895、SettingsServiceTests 1868 等 10 个 >1000 行。
5. MigrationPattern 模板去重（4–8 个 editor 文件尾部 60–90 行同构钉）。
6. 为 23 个退役迁移测试补一份"旧断言→新测试"退役映射说明。
7. 契约测试补强建议（与 05 文档合流）：占位符改"有序+计次"比对；加同文件重复键检测；考虑加死键/引用存在性双向检查（当前两者均不设防）。

## 五、Top-5 行动清单

① 补 `GetTrayStaggerIntervalMs`/batch driver 行为测试；② 补静默启动"隐藏→托盘恢复"链路测试；③ nameof/计数钉合并单一事实源；④ 消灭墙钟断言与真睡等待；⑤ 为新迁移对话框决策逻辑补策略测试并记录退役映射。
