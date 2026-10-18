# DeskBox UI 原生度全量审查（2026-10-02）

六路并行审查的汇总：设置窗口、格子外壳、格子内容控件、弹层/窗口/菜单、文案与本地化、全局设计 token。
第一性原则：能用原生组件就用原生（WinUI 3 系统控件 + ThemeResource + Segoe Fluent Icons + CommunityToolkit 官方控件），自绘只在原生真不满足时保留。
行号以 2026-10-02 磁盘状态为准（工作区含未提交的 post-155 与图标自定义批次，动工前需重新对行号）。

## 总评

原生度基础在同类 WinUI 3 项目中属上乘：ThemeResource 系统画刷 400+ 处、Segoe Fluent 字形 337 处、弹层几乎全走 ContentDialog/InfoBar/MenuFlyout/AppNotification、自制顶层窗口全部有书面理由（内存泄漏/IME/透明窗口发黑）、动效节奏 83/167/250 与官方一致、12 语言键集合由契约测试强制一致。

真正的债务集中在五处：
1. **设置页仍有手写卡片行**（钻入行、警告条、双份 token 字典）——SettingsCard/InfoBar 都是现成的；
2. **无障碍焦点视觉缺失**——文件瓦片容器模板剥掉了原生 FocusStates，键盘导航完全不可见；
3. **约 15 处漏网硬编码语义色**（警示黄/逾期橙/反馈 toast 四色），项目其余地方都在正确使用 SystemFillColor*；
4. **发光类效果不跟系统强调色**（Onboarding 蓝紫光晕、搜索胶囊 shimmer）；
5. **文案层**：en-US 约 60 处 Title Case 违反 sentence case、zh-CN 标点/术语漂移、一处插值后语法破损。

---

## A. 最值得做（换原生收益明显、低风险）

### A1. 设置页手写"钻入行"→ `SettingsCard IsClickEnabled="True"`
- `Views/SettingsWindow.xaml:2635-2697`（Maintenance 4 行纯钻入，无绑定冻结，最先做）
- `Views/SettingsSections/AppearanceSettingsSection.xaml:102-200`、`CapsuleModeSettingsSection.xaml:119-233`、`FileWidgetSettingsSection.xaml:20-130`
- 现状是 Border+DrillDownRowStyle+手排 E76C chevron 三件套，带内联 ComboBox 的行还靠 `IsHitTestVisible` hack 防误触。换 SettingsCard 后 hover/焦点/无障碍全免费。
- ⚠️ `AotStage4D1BContractTests` 冻结全仓 HeaderKey/DescriptionKey 计数（SettingsCard|HeaderKey=192 等，总 389），增删卡片必须同步改测试并重跑 `scripts/update-settings-search-catalog.ps1`。

### A2. 设置页手绘黄色警告条 → `InfoBar Severity="Warning"`
- `Views/SettingsWindow.xaml:1285-1300`（`ManagedStoragePathWarningBorder`，硬编码 `#66FFB900/#FFFFB900`）
- 同文件已有多处 InfoBar 正例；保留 x:Name 即不破 SectionElements 契约。

### A3. 标题栏搜索框手动叠图标 → `AutoSuggestBox QueryIcon="Find"`
- `Views/SettingsWindow.xaml:244-293`（叠加 FontIcon + 手工 Padding/CornerRadius 药丸）
- `DesktopOrganizationSettingsSection.xaml:109` 已用 QueryIcon 正例。

### A4. 文件瓦片补回键盘焦点视觉（无障碍 P1）
- `Controls/WidgetContents/FileSurfaceContent.xaml:38-85`（`SurfaceGridViewItemStyle/SurfaceListViewItemStyle` 模板只剩裸 ContentPresenter，FocusStates 全被剥掉；`KeyboardNavigation.cs:89` 在调用 `container.Focus(FocusState.Keyboard)` 但无任何可见指示）
- 最小修法：模板内加 FocusStates + `FocusStrokeColorOuterBrush` 双层焦点框。

### A5. 反馈 toast 四个严重级别色 → SystemFillColor* ThemeResource
- `Controls/WidgetFeedbackPresenter.xaml.cs:142-148`（Success/Warning/Error/Info 四个 ARGB 是浅色主题值，暗色亚克力上对比度不足）
- 换 `SystemFillColorSuccess/Caution/CriticalBrush` + `AccentFillColorDefaultBrush`；注意用元素树内解析（该控件同时服务 QuickCapture 的本地资源遮蔽）。已核实无测试冻结这四个值。

### A6. 零散语义色 → 系统 token（各一行）
- `Views/SettingsWindow.xaml:1288/1293` 警示黄 → `SystemFillColorAttentionBrush`
- `Controls/WidgetContents/TodoWidgetContent.xaml:404` 逾期橙 `#F7630C` → `SystemFillColorCautionBrush`
- `Controls/WidgetShell.xaml.cs:2523` warning 徽章 `0xE6D83B01` → `SystemFillColorCriticalBrush`；`:2927` attention → `SystemFillColorAttentionBrush`；`:5199-5211` overlay 按钮底/描边 → `SolidBackgroundFillColorBaseBrush`/`CardStrokeColorDefaultBrush`；`:2447` 缩略图描边 0x99FFFFFF → `CardStrokeColorDefaultBrush`

### A7. 两个自定义器 ToggleButton 对 → `toolkit:Segmented`
- `Controls/WidgetIconCustomizer.xaml:71-78`（Emoji/Image）、`WidgetBackgroundCustomizer.xaml:69-76`（填充/适应）
- 项目已引用 Segmented 且 Todo/QuickCapture/Weather 三处在用；无测试锁结构。

### A8. 发光效果跟随系统强调色
- `Views/OnboardingWindow.xaml:100-132` 蓝紫光晕（`#1691E8/#0B64BF/#58AAFE` 渐变）→ 基于 `SystemAccentColorLight1/2/3` 派生
- `Controls/WidgetShell.xaml:333-335` 搜索胶囊 shimmer（`#0063B1/#8B5CF6`，旧版 accent 蓝）→ 同上
- 建议 App.xaml 集中定义 `AccentGlowStartBrush/EndBrush`。

### A9. 死样式/死资源清理（零风险）
- `Controls/WidgetContents/MusicWidgetContent.xaml:109-197` `MusicPrimaryButtonStyle`（0 引用，含硬编码覆盖色）
- `App.xaml:345-349` `WidgetPivotSegmentedStyle`（0 引用）；`App.xaml:17-21,53-57,29-31,65-67,222-234` 一批死画刷/死样式（WidgetBackgroundBrush、WidgetTitleState* 三件、WidgetLayerCard* 两样式）
- `Views/SettingsWindow.xaml:44-54` PageTitleTextStyle/PageDescriptionTextStyle（0 引用）
- ⚠️ 删除前 grep 一遍；避开 `WidgetDragHandleThemeContractTests` 点名的键。`WidgetTitleState*` 二选一：删除，或接入 `WidgetTitleActionButtonStyle` 的 PointerOver/Pressed 让标题按钮 hover 用对 subtle 层级（现状偏实）。

### A10. 音乐只读进度条 → 原生 `ProgressBar`
- `MusicWidgetContent.xaml:943-962、1155-1174`（Record 两处纯展示自绘 Border 轨道）
- 交互式 ProgressHost（:600-636）二期再评估 Slider（拖拽手感/节流需重测）。

### A11. 搜索弹窗关闭钮字形 E70D → E8BB + 补 ToolTip
- `Views/SearchPopupWindow.xaml:376`（E70D 是 StatusError 圈叉，应为 ChromeClose 纯 X）

### A12. Onboarding emoji 图标 → Fluent glyph
- `Views/OnboardingWindow.xaml:171,186,272-285`（📁📄🗂️🗃️📂⚙️ → E8B7/E7C3/E8E5/E713 等），随主题、无 emoji 字体跨版本漂移；营销页要彩色感则保留（P3 可辩护）。

---

## B. 可做但有权衡（P2）

- **About 更新区裸 Expander → SettingsExpander + SettingsCard**（`SettingsWindow.xaml:3319-3468`，消除 40px 手工缩进/手绘分隔线/Header 双重绑定；同步 D1B 计数 + 搜索目录脚本）。
- **FileWidgetSettingsSection 整页手写卡片 → SettingsCard**：⚠️ `AotStage4E4ContractTests` 冻结该文件 `{x:Bind ` 恰 5 处、`{Binding }` 恰 0，`AotStage5B4B1ContractTests` 逐字冻结 4 条 x:Bind——迁移必须原样保留绑定写法。
- **设置导航图标 SVG(28×28) → FontIcon**（`SettingsWindow.xaml:320-409`，8 个 NavigationViewItem；品牌多色语言 vs 原生一致性的取舍，字形会立刻与全页 HeaderIcon 统一）。
- **WidgetShell 标题钮 5 组自绘 Path → FontIcon**（`WidgetShell.xaml:174-206`：Add=E710、More=E712、关闭=E74D 保留移除语义、锁=E72E 系；需同步 `WidgetShell.xaml.cs:768-774` 暴露的 icon 属性；PositionLock/SizeLock 双态语义可保留 Path）。
- **Stack 组切换器 hover/pressed 从代码快照迁回 VSM**（`WidgetGroupTitleSwitcher.xaml:21-38` 裸模板 + `Interaction.cs:953-1012` 手写 alpha 阶梯；给模板加 VisualState 引用真实 ThemeResource，先只迁 hover/pressed 两态）。
- **文件项 hover/选中 alpha 表对齐 Neutral 调色板**（`FileItemSurfaceStyleCache.cs:103-128`，hover 浓度比其它 neutral 面近一倍；未选中档改 `DeskBoxNeutralFillSecondary/Tertiary`，选中档对齐 SystemListLow 语义；需肉眼回归壁纸/材质组合）。
- **音乐 IconButton 模板 → 轻量样式覆写**（`MusicWidgetContent.xaml:16-95` 自绘 4 态 VSM；GlanceWidgetContent.xaml:28-31 已有 `ButtonBackgroundPointerOver=SubtleFillColorSecondaryBrush` 正例；内容透明度阶梯 0.92/1/0.78 若要保留则留模板）。
- **存储迁移对话框按钮搬进 ContentDialog 命令位**（`Services/ManagedStorageMigrationDialog.cs:354-399`；恢复 Enter→主按钮/标准度量/无障碍语义；三态状态机的按钮 IsEnabled 同步要小心）。
- **ReleaseNotes "在线查看" → HyperlinkButton**（`ReleaseNotesWindow.xaml:94-100`，参考 TaskView 的 AbandonRecoveryButton；⚠️ 别动被 `FrostedActionSurfaceContractTests` 冻结的 FooterAcrylicSurface）。
- **空状态三套样式收拢 `WidgetEmptyState*`**（FileSurfaceContent.xaml:734-807、QuickCaptureSurfaceContent.xaml:312-316 对齐 Todo/App.xaml 体系，顺带解决 14/11.5 硬编码字号与 TextScaling 断链）。
- **双份设计 token 字典合并**（`SettingsWindow.xaml:25-211` vs `Styles/SettingsOverviewResources.xaml:6-105` 完全重复十几个资源；Capsule Section 还有第三份变体）。
- **代码构建的设置行换 `new SettingsCard`**（`SettingsWindow.DataTools.cs:205-288`、`LocalizationAndWidgets.cs:223`；`Services/Localized.cs` 已有 SettingsCard 分支，基建现成）。
- **圆角离群 7(×17)/9(×4)/3(×21) 收敛到 4/6/8 token**：⚠️ 多处被字面量断言冻结（`FileWidgetFolderNavigationContractTests.cs:29` 断言含 `CornerRadius="7"`；`GlanceWidgetPhotoControlsTests.cs:23` 断言 `"9"`；`MarkdownAndSplitterContractTests.cs:541` **反向冻结** QuickCapture 不得含 `"8"`）——每改一处连测试一起改。
- **Glance 操作栏 acrylic 补 Light 变体或回落系统**（`GlanceWidgetContent.xaml:23-31` 只有暗色一组 TintColor/白色按钮覆写；若"永远深色玻璃"是产品决策则记 P3）。
- **SearchWidgetContent 搜索图标统一 FontIcon E721**（`SearchWidgetContent.xaml:96-101` 现走 WidgetTitleIcon Path，与 QuickCapture 的 FontIcon 不一致）。
- **MarkdownSourceEditor Flyout 菜单 → MenuFlyout**（省 70 行手排 Button）。
- **文件菜单排序**（`FileItemMenuBuilder.cs:150-158`：「在资源管理器中显示」上移到属性前，「更多系统操作」收到组尾——对齐 Explorer 惯例）。
- **整理完成双 Accent 修正**（`DesktopOrganizationTaskView.xaml:175-180` + Actions.cs:124-125：Done 与 RetryPublic 同现时双强调，Retry 去 Accent）。
- **RecoveryInfo 横排 StackPanel 不换行**（`DesktopOrganizationTaskView.xaml:120-126`：文本走 `InfoBar.Message`，链接走 ActionButton）。
- **Onboarding 补 ESC 出口**（按"完成引导"路径关窗）。
- **黑胶唱片两段 100% 重复的插画抽成一个控件**（`MusicWidgetContent.xaml:746-892` vs `1062-1101`，减 ~100 行）。
- **系统文本样式渐进采纳**：全项目 0 处 BodyStrong/Caption 等 ramp 引用、SemiBold×106 裸写；建议只动固定布局场景（设置页/ReleaseNotes/Onboarding），小组件侧字号绑定保留。

---

## C. 保持自绘/现状（有正当理由或被契约锁死——不要"优化"这些）

| 项 | 位置 | 理由 |
|---|---|---|
| 文字阴影双层 TextBlock | WidgetShell/FileItemSurface + WidgetTextShadow.cs | 壁纸可读性本体；`WidgetTextShadowContractTests` 禁止 GetAlphaMask/Composition 实现 |
| PinStateIcon 自绘 Path | PinStateIcon.xaml | 测试双向断言（含几何片段 `L8.4,15.5`、禁 E841） |
| MusicTransportIcon 26px 自绘 | MusicTransportIcon.xaml | `AotStage4E2` 冻结 7 个 x:Bind；`MusicWidgetContentLayoutTests` 冻结 15+3 计数；小尺寸光学对齐优于字形 |
| Glance 照片操作条 Path 图标/亚克力参数 | GlanceWidgetContent.xaml:390-462 | `GlanceWidgetPhotoControlsTests` 锁元素名/0.52/圆角 9 |
| Todo 原生 CheckBox + 中性色覆写 | TodoWidgetContent.xaml:19-43 | 教科书式 lightweight styling，测试锁结构 |
| 天气 SVG 三风格图标体系 | Assets/WeatherIcons + WeatherCodeMapper | 内容资产非 UI chrome；有独立 iconStyle 管线批次 |
| 搜索弹窗顶层窗口 + 恒不透明 | SearchPopupWindow | `BuildOpaquePopupSurfaceColor` 契约定案（透明顶层 XAML 发黑） |
| StackPopoverHostWindow 等四个自制顶层窗 | — | 书面理由：Popup hwnd 泄漏 / TSF-IME / click-through 预览 |
| Compact 提示气泡树内 Border | WidgetShell.xaml:503-523 | ToolTip 无法编程 dismiss；可顺手套 `WidgetTooltipCardStyle` 统一圆角 |
| 桌面图标式文字跑马灯、marquee | MusicWidgetContent | WinUI 无原生 marquee |
| 便签/QuickCapture 材质色板、黑胶插画、full-bleed 蒙版、`#01000000` 命中区 | 各处 | 材质即数据/设计语言本体 |
| DropDownButton+MenuFlyout 多选下拉 | 设置页多处 | WinUI 3 无 MultiSelectComboBox，已是官方等效解 |
| Accent 色板 swatch 按钮 | AppearanceSection | 无官方控件；旁边已有原生 ColorPickerButton |
| 页面 Visibility 硬切无转场动画 | SettingsWindow | 惰性创建架构刻意如此，动它风险高收益低 |
| `SettingsBoolToVisibilityConverter` | — | 与系统等价但被多条 Editor 测试逐字冻结，替换得不偿失 |

---

## D. 文案与本地化（Strings/*.json，12 语言 × 2746 条）

键集合/占位符由 `LocalizationResourceContractTests` 强制一致（改值必须 12 文件同步、占位符集合不变）；`TodoClipboardFormatterTests.cs:51` 冻结 en 值 `"No due date"`。

**P1：**
1. **en-US sentence case 违规约 60 处**：天气设置整节（"Weather Data Source"/"Show Forecast"…）、搜索设置整节、迁移对话框（"Move Back to Desktop?"…）、组件菜单（"Copy Path"/"Icon View"/"Lock Position"…）、"New Folder"→"New folder"。除专有名词外全部 sentence case——这直接决定"像不像系统应用"。
2. **`Widget.Empty.ManagedText` 插值后语法破损**（en+zh）：设置为「跟随 Windows 默认」时渲染出 "automatically Use Windows default them to X."／「自动跟随 Windows 默认到 X。」——FollowWindows 需单独成句。
3. **同页三名**：`Settings.Nav.Maintenance`=「诊断与维护」vs `Settings.Section.Maintenance`=「重置与更新」vs `Settings.Group.Maintenance.Title`=「诊断与恢复」，统一「诊断与维护」。
4. **zh-CN 新手引导 10 处半角逗号 + 1 处半角问号**（zh-CN.json:2728-2753）→ 全角。
5. **ja-JP「ピン留て」错别字 ×9**（:750-755、940、947、998、1001、1312）→「ピン留め」。
6. **句子粘连缺句号**：`Settings.Animation.Stagger.Description` en/zh 两句直接拼在一起。

**P2 术语收敛（zh）**：文件资源管理器勿缩短（6 处）；「使用管理员身份打开」→「以管理员身份运行」；Snooze「稍后提醒」→「暂停」；「打开所在位置」→「打开文件位置」；「切换样式/Tab 栏」→「标签样式/选项卡栏」（tab=选项卡）；restore 统一「还原」（本地备份用「恢复」造成分裂）；「透明效果」→「透明度效果」；「速记」→「随记」；「一键反馈/修复」→「发送反馈/修复」；「桌面原版右键菜单」→「系统右键菜单」；`Settings.Search.Description` zh 只剩「搜索」二字（信息丢失）。
**P2 体例（zh）**：引号 12 处「」→""（微软简中体例；zh-TW 反向统一「」）；设置描述句末句号 32 处去除；「均衡/平衡」统一「均衡」；「所有/全部」「风速/风力」「剪贴板记录/历史」统一。
**P2（en）**："..." 12 处 → "…"；"box" 指代 widget 5 处 → widget；叹号 3 处；Drag & Drop Check → Drag-and-drop diagnostics。
**硬编码漏网 3 处**：`PlaceholderWidgetContent.cs:64` "Content placeholder"；`OnboardingWindow.xaml:224` 键帽恒显 "F7"；`MarkdownSourceEditor.xaml:212` AutomationProperties.Name 硬编码英文（资源键已有）。

**术语表定稿建议**：widget=格子（「小组件」仅指 Windows 系统组件，3 处漏用需改回）、File Explorer=文件资源管理器、Snooze=暂停、clipboard history=剪贴板历史记录、tab=选项卡、restore=还原、Balanced=均衡。完整表见审查原文。

---

## E. 契约测试红线速查（动 XAML 前必读）

- `AotStage4D1BContractTests`：HeaderKey/DescriptionKey 全仓精确计数（192/160/…，总 389）+ `update-settings-search-catalog.ps1` 再生成。
- `AotStage4E4ContractTests`：FileWidgetSettingsSection `{x:Bind ` 恰 5、`{Binding ` 恰 0；App.xaml `{Binding ` 恰 2。
- `AotStage5B4B1ContractTests`：4 条 x:Bind 逐字冻结。
- `SettingsSectionElementAotContractTests`：SectionElements.cs 全部 name 字面量必须匹配 x:Name=。
- `AotStage4E1ContractTests`：DesktopOrganizationSection 的 ToolTip x:Bind；WMC1510 上限 864（新增编译绑定可能推高破 CI）。
- 圆角/几何字面量：`CornerRadius="7"`（FolderNavigation）、`"9"`（GlancePhoto）、`"6"`（GlanceCalendar）、QuickCapture **不得含** `"8"`（反向冻结）、PinStateIcon 几何片段、Glance 亚克力 0.52、SearchPopup 多组 `Padding="4,5" CornerRadius="4"`。
- 计数类：MusicTransportIcon 15+3、MusicSourceIcon 4、`FeatureSettingsExpander_Loaded` 各 5、Todo CheckBox 结构、FrostedActionSurface 系列。
- `LocalizationResourceContractTests`：12 语言键集合 + 占位符；`TodoClipboardFormatterTests`："No due date" 值。
- 各 Editor 测试逐字冻结绑定写法：**换控件可以，换绑定写法不行**。

---

## F. 建议批次划分

> **执行状态（2026-10-03 回填）**：批 1/2/3 已全部实施（未提交，随工作区既有批次），每批 4779 测试全绿并重启验证。实施范围与 F 节定义的差异与刻意保留决策如下：
>
> - **批 1 保留三处**（核实后判定，非漏做）：`WidgetShell.cs` 紧凑 Todo 进度橙（与 accent→绿渐变配套的数据可视化色，非 UI chrome 语义色）；缩略图白描边（相框设计）；overlay 不透明按钮底（已有明暗主题分支的刻意材质）。OnboardingWindow 警示黄为 obj 陈旧残留，源文件无。
> - **批 2 范围修正**：复合行（钻入按钮+内联 ComboBox/Toggle 并列）全部保留——`SettingsCard IsClickEnabled` 整卡点击与内联值控件互相冲突，换卡会丢失钻入子页入口。涉及 Appearance×4、Capsule×3、FileWidget×2、SettingsWindow Performance 行（审计曾误报为漏项，实为复合行）。Capsule Overrides 行保留：描述位是动态摘要文本（OverrideSummaryText），静态 DescriptionKey 无法承载。`WidgetTitleState*` 死键按 A9 的二选一取「删除」分支。
> - **批 3 修正**：双 Accent 为审计误报（Actions.cs 既有 Done 让位逻辑）。
> - **对抗审计（2026-10-02 深夜，四路）+ 修复轮（2026-10-03）**：发现并已修复——①`WidgetPivotSegmentedStyle` 误删（`WidgetSegmentedStyleHelper.cs` 运行时字符串键引用漏检，已恢复无 Binding 版并补守护测试 `PivotSegmentedStyleKey_RemainsDefinedForRuntimeLookup`；三处调用点的 MinHeight/FontSize 内联绑定本就存在，样式内 Binding setter 是冗余）；②批 1 两处 `Application.Current.Resources` 解析违反项目「禁止应用域主题查找」红线，已改 `NeutralInteractionBrush.ResolveThemedBrush(key, element)` 元素域解析；③python 写入误加的 BOM×3 已清理；④警告条 InfoBar 改 `IsOpen` 驱动（获得打开播报）；⑤焦点框补 `PointerFocused` 态与内白环（标准双环焦点）；⑥补 SettingsWindow 必须合并 SettingsOverviewResources 的守护断言。审计确认误报：InfoBar IsOpen 默认值问题（默认 true，实测正常）。
> - **发版前必办**：`publish-aot-audit.ps1` 双架构跑一次验证 WMC1510≤864（工作区含既有批次的大量 {Binding} 增量，本三批净增约 +1）；真机冒烟清单见审计报告（Pivot 标签样式、黑壁纸焦点框、新旧卡混排、About Details 长文案、收纳路径警告条）。
> - **勘误**：本文 A1 注脚所写 D1B 基线（192/160/389）与 HEAD 实测（185/156/376）不符，系成文时叠加了工作区中间态；改动后基线为 203/169/405（组合 5 种）。

1. **批 1（一行级 token/字形替换，几乎零风险）**：A5、A6、A11、Todo 橙、WidgetShell 徽章色 → 全部跑测试即可。
2. **批 2（设置页原生化）**：A1→A2→A3→About Expander→token 字典合并→死样式清理；同步 D1B 计数与搜索目录脚本。
3. **批 3（无障碍+交互控件）**：A4 焦点框、A7 Segmented、A10 ProgressBar、菜单排序、双 Accent、RecoveryInfo 布局。
4. **批 4（发光跟强调色 + 圆角收敛）**：A8、A12、圆角 7/9/3（连测试一起改）。
5. **批 5（文案批）**：en sentence case、破损插值、zh 标点/术语、ja 错字——12 文件同步 + 占位符不变。
6. 观察项（二期）：音乐按钮模板轻量化、迁移对话框命令位、Stack 切换器 VSM、hover alpha 对齐、系统文本 ramp 渐进采纳。
