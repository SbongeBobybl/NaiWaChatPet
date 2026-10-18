# PR 合并窗口审查报告（2026-09-29）

审查对象：近 3 天（09-26 ~ 09-29）合并进 `main` 的全部 29 个 PR（#427–#473，另有缺号 #449–#459/#466 经查不存在）。审阅方：Devin 会话，10 路并行子代理按簇深审 + 主审对 P1/P2 关键发现在 `origin/main`（`027a0c0c`）逐条亲核。

前置输入：同目录 `architecture-optimization-review-20260924.md` 与 `pr-stack-review-20260925.md`（前两轮审计，覆盖批 1–22 与 PR 栈 #423–#426）。本报告覆盖的 PR 是前两轮审计的跟进执行（#427–#430）与后续收割期（#432–#473）。

## 1. 审查方法与编排

- **Per-PR 深审 ×10 组**：每个 PR 只看自己的增量（`merge^1..merge`），按簇分组：收尾修复、adapter 双 PR、设备层基建、协调器迁移 A/B、门面退役前半/外观+分组胶囊/文件栈+随记/待办+天气+备份、收官+AOT 修复。
- **棘轮单调性（主审）**：跨合并链追踪 AOT 冻结计数（SettingsViewModel ObservableProperty 75→75→73→53→14）、壳桥 nameof 计数（349→33）、FacadeAccessManifest 条目（净收 268）——全程只减不增。
- **主审回归**：P1/P2 发现在 `origin/main` 树逐条复核签字，排除子代理误报。
- 全程只读：未改文件、未构建、未跑测试；行号以各 merge 提交或 `origin/main` 为准。

## 2. 窗口全景

| 簇 | PR | 内容 | 规模 |
|---|---|---|---|
| 审计跟进 | #427–#430 | 剩余退出等待有界化、拆离对账修复、清理扫描 | ~675+ |
| 适配器 | #432/#433 | 内容适配器基座抽取 + 7 类 leaf 化；吸收 10 个 smoke-fix/harden 子分支 | ~4200+ |
| 设备层基建 | #438–#440 | 设备层收口核验、P/Invoke 主动全迁（硬零棘轮）、IFeatureRuntime 租约 | ~2900+ |
| 协调器 Track A | #441–#448 | 外观→胶囊→交互→文件显示→文件栈→分组导航→功能→收官，8 批 | ~6800+ |
| 门面退役 Track B | #460–#472 | 设置节 XAML 绑定逐节迁编辑器；General/About 裁决留壳；AppSettings 定性冻结线契约 | ~15000+ |
| 修复 | #473 | 设置转换器 partial 补 AOT ABI 双回归 | 18+/−16 |

**总评：方向正确、执行质量整体高、棘轮全程单调收紧、测试普遍为真行为测试；但收割期后段（Track B 复制批）跑赢了校验网，main 上现存两个用户可见的功能回归（P1）。**

## 3. P1 — 当前 main 上的实锤回归（主审亲核）

### P1-1 自定义强调色整卡失效（#463 引入，两缺陷叠加）

- `Features/Appearance/AppearanceSettingsViewModel.cs:381`：`CanEditCustomAccent => !Equals(AccentColorSource, "Custom")`——极性写反（旧门面为 `!UseSystemAccentColor`）。自定义模式下取色器与六色板整卡禁用，系统模式下反而可编辑；`AppearanceSettingsSection.xaml:64` `IsEnabled` 绑该属性。新增测试 `AppearanceSettingsEditorTests.cs:317` 把反转语义钉成了断言。
- `ViewModels/SettingsViewModel.AppearanceOptions.cs:87` `OnAppearanceAccentColorSourceUserChanged`：只调 `SetAccentMode`（静默保存不广播）**不更新 `_useSystemAccentColor`**；`PushAppearanceAccentPresentation`（:142）把陈旧值推回编辑器 → 下拉选择立即回弹，会话内持续失真至下一次无关广播。
- **合并效果：用户无法切换到自定义强调色；存量 custom 用户开设置页看到禁用卡。** 唯一仍正确的入口是色板点击路径（`SetCustomAccentColor` 会更新字段）。
- 修法：极性改回；handler 内同步 `_useSystemAccentColor`（或推送时读持久值）；更正测试断言。

### P1-2 Todo 字号提交事件断线（#468 引入）

- `Features/Todo/TodoSettingsViewModel.cs:138-141` 声明并在 `:508/:549` 触发 `ListTextSizeCommitted`/`ContentTextSizeCommitted`，但**壳零订阅**——对照 QuickCapture 同款事件在 `SettingsViewModel.cs:263-264` 有接线，Todo 至今（`027a0c0c`）未修。提交说明与进度文档均声称已接线，与实现不符。
- 后果：滑杆拖动中丢失逐格实时预览；键盘方向键等非指针修改只有 `scheduleSave:false` 的内存写，要等下一次无关保存才落盘——相对旧壳 `PersistOwnedTextSize→SaveAppearanceChange` 是真实回归。
- 修法：壳 ctor 补两行订阅 + Dispose 退订（对齐 QC 先例）。

## 4. P2 — 需跟进项

| # | 来源 | 位置 | 问题 | 修法方向 |
|---|---|---|---|---|
| 1 | #464 | `SettingsWindow.xaml:1042`、`:1103` | 两个 `Visibility` 仍绑已删门面名（`WidgetCompactHoverResponseCustomVisibility`、`CapsuleOverridesListVisibility`），绑定静默失败 → 折叠延迟卡/覆盖重置卡常显 | 改绑 `ShowHoverResponseCustom`/`ShowOverridesList`（bool，需 converter） |
| 2 | #441 | 仓库根 `push-a.ps1` | 误入库的开发脚本：硬编码 `D:\project\wingezi-final-a`，执行即 add+commit+push+建 PR | 删除 |
| 3 | #439 | progress 文档 @54166c22 | 分支内合并的冲突标记 `<<<<<<< HEAD` 被提交进 main（#440 已正确解决） | 流程教训：merge 提交需复核 |
| 4 | #439 | `Platform/DragDropPermissionNativeMethods.cs:108` | `StartupInfo` 丢 `CharSet.Unicode`（三 string 字段恒 null 故无实害，违背"逐字保留"承诺） | 补回 CharSet |
| 5 | #427 | `App.xaml.cs` 退出序列 | `settings-flush` 仍是唯一无界步骤，且其挂起路径不触发看门狗 | 改 Bounded 或加兜底 |
| 6 | #428 | `WidgetManager.Groups.cs` catch 臂 | `RegisterStandaloneUnifiedFileSessionIfNeeded` 在 catch 内无 try，窄概率逃逸截断剩余恢复链 | 补一层 try |
| 7 | #473 | `Controls/SettingsBindingConverters.cs` | partial 修复对症但未给自己补源码钉——回归能活 7 批正因唯一守卫是发布级审计 | 加入审计源清单/契约测试钉 `sealed partial` |

## 5. P3 簇（择要，按主题归并）

**复制批模板缺陷（被逐批继承）**
- 编辑器 `*UserChanged` 事件订阅零退订：壳 ctor 已堆 15+ 个 `+=`、`Dispose` 不退（#461 首创）。同寿命无实害，靠巧合正确——建议模式注释显式豁免或统一退订。
- 协调器 `Stop()`/`IsStopped` 家族级生产死码（仅 QC/Todo 的 StopAsync 真接线）。
- 已迁移模板仍标 `x:DataType="viewModels:SettingsViewModel"` 且 `ProcessBindings(壳VM)` 锁根——`{Binding}` 无害，但未来在迁移节写 `x:Bind` 会绑错（会大声失败）。

**门禁/棘轮缝隙**
- `appearanceWrite`/`interactionWrite` 正则只放行 `WidgetShell.` 前缀，字段实际横跨 `Core`/`FileWidget`/`Performance`——切片式直写（新规范写法）可在设置壳内无声绕过双棘轮。正确模板见 #444 `fileDisplayWrite`。
- `FacadePassthroughAccess` 正则误伤本地化键字符串，逼出 `"Settings.WidgetLayer" + "Mode."` 分片规避术（有测试钉，非直觉陷阱）。
- `PlatformInterop` 口径不覆盖 `ComImport`/`GeneratedComInterface`（9 个文件仍在 Platform 外）。

**双写主/双账本（过渡残留）**
- `IFeatureWidgetsSettings.SetManagedDropAction` 与 `IManagedStorageSettings` 双端口并存同一字段（#462），生产仅测试触达——收官需端口去重批。
- 壳 internal 工作态 vs 编辑器呈现态手工同步（`ManagedStorageRootPath`/`RootPath`）。

**潜伏/语义歧义**
- `IFeatureRuntime` 契约未立法 Dispose 后语义：Todo/QC 终态 vs Search 可循环并存（#440）。
- `SearchFeatureRuntime` 串行化靠 UI 线程纪律而非结构（#440）；`App.xaml.cs` 对 `StartAsync/DisposeAsync` 丢 Task 依赖同步实现。
- Platform 内 P/Invoke 重复声明分歧：`OpenProcess`×6（int/uint）、`GlobalFree` void/IntPtr、`SHCreateDataObject` nint/nint*（#439）；Platform→Helpers 反向依赖。
- `FileWidgetContentAdapter.WasLaunchConsumedRecently` 空 Surface 回退 `?? true` 无专测（#432）。
- TodoSettingsCoordinator 快照守卫族在协调器 API 下不对称项（`SetDefaultRootPath` 无等值跳过等，#448 已注明）。
- 遗留命名债：`CreateFileSurfaceContent` 等方法名仍称 Surface 实返 adapter（#433）。
- #469 天气三处在途搜索取消未跟随壳 CTS（自愈合漂移）；#467 QC 日志文案粒度略降。
- #448 残余写入者终表漏登 `SearchPopupViewModel.SetSaveSearchHistory`。
- `GroupByValues` 不含 `DateAdded` 的历史 quirk（#465，预存非回归）。

## 6. 系统性方向问题（比单点 bug 更重要）

1. **复制批缺两类机械化断言**——#464 残留绑定名、#468 断线事件、#463 反转属性是同一类盲点：跨文件接线无人钉。建议补两条通用契约断言：(a) 已删门面名在全部 XAML 零命中；(b) 编辑器公开事件在壳 ctor 有订阅。
2. **发布级 AOT 审计连红约 7 批无人清零**（批 43→50）——CsWinRT1028 回归与三处钉漂移共生至 #473 才修。建议"审计连续红即阻塞"写进批次纪律。
3. **merge 提交缺复核**：#439 把冲突标记带进 main（#440 顺手救回）；#432/#433 merge message 的 PR 号与分支互换（cosmetic）。
4. **提速与质量的张力客观存在**：Track A（#441-448）几乎无瑕，Track B 复制批（#463-470）两天内 1.5 万行且缺陷集中——复制批的安全网应从"逐点清单核对"升级为"机械化断言"。

## 7. 已验证无问题项（复核时可跳过）

- 写点收口完整性：Track A 各域设置页写点全走协调器，残余写入者（Onboarding/WidgetManager/迁移服务/SettingsService 内部）全部登记在册且棘轮覆盖。
- 语义等价性：归一化/默认值/等值跳过/SaveDebounced vs SaveAsync/静默保存/防抖/广播次序/快照守卫/0=继承字号——逐项复核等价或严格更稳。
- QuickCapture 三高危语义（字号继承、录制联动、裁剪静默期）与 Todo 提醒链、备份页端点代次/取消边界在编辑器化后逐字保全。
- AOT 面：`{Binding}` 全经 `GeneratedBindableCustomProperty` 具名桥、选项表真数组、无 `IReadOnlyList` 集合表达式、无新反射。
- #472"冻结磁盘线契约"定性如实：220 透传=纯磁盘映射、双向冻结测试、棘轮净收 268 逐数可验、残余写面全登记。
- adapter 迁移：生产 P/Invoke 零改动；COM vtable 补全（槽序/HRESULT/引用计数/所有权移交）教科书级正确；smoke-fix×6 与 harden×4 子分支全部修在根因。
- 序列完整性：merge 链线性、无绕 PR 直推、提交信息无工具署名（合规）。

## 8. 处置建议（按优先级）

1. **修 main 上两个 P1**：强调色双回归（极性 + `_useSystemAccentColor` 同步）+ Todo 字号事件订阅，并更正/补充测试断言。
2. #464 两行残留绑定改名（+converter）；删 `push-a.ps1`。
3. 补两类接线断言（删名零命中 + 事件订阅）与转换器 `sealed partial` 源码钉进契约测试；`StartupInfo` 补 `CharSet`。
4. 中项：`settings-flush` 有界化、门禁前缀补齐、协调器端口去重、Platform 签名归并、`IFeatureRuntime` Dispose 语义立法。

## 9. 方法论注记

- 本轮沿用"守恒校验+棘轮单调+主审签字"编排并新增一条经验：**复制批最危险的不是单点逻辑而是跨文件接线**（事件订阅、绑定名、DataContext 切换）——逐点清单核对挡不住，必须机械化断言。
- 子代理行号引用默认可信，但 #463 的 P1 与 #468 的 P1 均经主审在 `origin/main` 复核属实后才进结论。
