# 可选格子的接入缝：现状 → 目标形态

> 2026-09-11。评审用文档，不含代码改动。
> 背景：产品方向定为"小而美 + 可选功能做成自己的精品"（不做商店、不做第三方插件平台）。
> 那么"新增一个可选格子"的成本就直接决定了做精品的速度。本文只回答两件事：现在要碰多少地方，以及应该收敛成什么样。

## 一、现状（2026-09-11 实测，基线 main `be2a9cf`）

以 `WidgetKind.Todo` 为样本统计宿主内的接触点：

| 指标 | 数量 |
|---|---|
| 含 `case WidgetKind.` / `== WidgetKind.` 分支的文件 | **25** |
| 引用该 kind 的生产文件 | **约 30** |
| 其中属于 AOT 证据链的宿主 partial（`App.Aot*Smoke.cs`） | 6 |
| 单文件最多的 kind 分支数（`WidgetManager.FeatureWidgets.cs`） | **34** |

分支密度最高的文件：`WidgetManager.FeatureWidgets.cs` 34、`WidgetManager.cs` 11、`SettingsViewModel.FeatureOptions.cs` 10、`WidgetManager.Storage.cs` 7。

按角色分类：

| 角色 | 代表位置 | 为什么必须改 |
|---|---|---|
| 身份 | `Models/WidgetConfig.cs` | `WidgetKind` 枚举加成员 |
| 注册 | `Services/WidgetContentFactory.cs`（描述符表=kind 清单真源）、`WidgetRegistry.cs`（**有意分离**的窗口能力表）、`FeatureWidgetSettings.cs`（功能开关清单） | 前者是清单真源；后者原为硬编码副本，已改为派生 |
| 生命周期 | `WidgetManager.cs`、`WidgetManager.FeatureWidgets.cs`、`WidgetManager.Groups.cs`、`WidgetManager.Storage.cs` | 创建/移除/分组/存储各有一段 kind 分支 |
| 策略 | `WidgetCompactPrivacyPolicy.cs`、`WidgetCompactWarmupSchedulePolicy.cs`、`WidgetTitleIconMode.cs`、`WidgetSettingsMenuHelper.cs` | 每个策略表都要为新 kind 补一行 |
| 设置界面 | `SettingsViewModel.cs` + `FeatureOptions` + `FeatureCallbacks` + `SettingsSync` + `CapsuleOptions`（**5 个 partial**） | 每处加字段、回调、同步、胶囊选项 |
| 视图/窗口 | `Views/ContentWidgetWindow.xaml.cs`、`WidgetWindowBase.Collapse.cs`、`OnboardingWindow.*` | 窗口行为与引导流程的 kind 分支 |
| 集成 | `App.xaml.cs`、`Services/SearchEngineService.cs`、`SearchResultActionService.cs`、该功能自己的服务（如 `TodoReminderService`） | 启动接线与跨功能联动 |
| 内容实现 | `Controls/WidgetContents/<Kind>WidgetContentAdapter.cs` | 真正的功能实现 |
| AOT 证据 | 6 个 `App.Aot*Smoke.cs` | 宿主 partial，**物理上搬不走**（硬约束，非缺陷） |

## 二、现状暴露的四个问题

1. **注册点分散（2026-09-11 复核后收窄）**：原判断是"三处重复注册"，对码后发现只有**一处真正的重复**——`FeatureWidgetSettings` 里硬编码的功能开关清单（已改为从描述符表派生）。`WidgetContentFactory` 的 `DescriptorList` 本身就是完整的 kind 清单（含设置节/图标/可用性），是应该的单一真源；而 `WidgetRegistry` 的窗口能力表**是有意分离的**（`WidgetContentDescriptor` 的文档注释明确写着"不决定某个 kind 能否创建窗口"），把它并进描述符表属于设计倒退，不做。剩下的事实是：**新增 kind 仍要在 25 个文件里的分支处各加一行**，这与"注册"是两件事。
2. **kind 分支散落**：25 个文件、单个文件最多 34 处分支。新增 kind 靠"记得改哪里"，而不是靠编译器报错——这正是"加功能要改宿主十几处"的体感来源，也是当初关掉 #306（番茄时钟外部 PR）的实际理由。
3. **设置页成本最高**：5 个 `SettingsViewModel` partial 都要动，且国际化要同步 12 个文化表。
4. **AOT 证据链天生跨不过程序集**：6 个 smoke 是宿主 partial。这意味着"缝"只能落在宿主内——这条约束同时说明了为什么拆程序集/包格式解决不了这个问题。

## 三、目标形态：一处声明 + 三处实现

收敛目标是把一个 kind 的接入拆成"改宿主公共表 1 处、写自己的东西 3 处"：

1. **一处声明（唯一改宿主公共表的地方）**：一个 contribution 描述符，承载今天散落的全部 kind 知识——显示名/本地化键、标题图标、所属设置节、默认启用态、是否可多实例、紧凑与胶囊策略、存储根名、搜索/引导参与方式。
2. **内容实现**：一个 `IWidgetContent`（+ 其 provider）——真正的功能代码，天然属于功能自己。
3. **设置节**：一个 UserControl（宿主已有此模式：`Views/SettingsSections/*.xaml`），不再改 `SettingsViewModel` 的 5 个 partial。
4. **本地化键**：12 文化表加键。

**验收判据**：用一个真实新 kind（建议番茄时钟）走一遍——
- **注册点必须收敛到 1 处**。今天分散在 `WidgetRegistry` / `WidgetContentFactory` / `FeatureWidgetSettings` 三张表里，漏一处是**静默失效**（入口存在但创建不出来）。收敛后遗漏必须由编译器或契约测试报错。
- 新增文件 = 内容实现 + provider + 设置节，再加快捷的本地化键。
- 按 kind 逐条列策略的地方（紧凑隐私、预热计划、标题图标等）**仍会各加一行，这是有意保留的**——它们表达的是功能自己的策略，不是宿主的公共表，强行抽象反而会把策略知识挪进公共层。

## 四、分步改造（每步行为零变化、可独立验证）

| 步 | 内容 | 完成判据 |
|---|---|---|
| 1 | 把三处重复注册合并成一张 contribution 描述符表（纯搬运） | 契约测试钉住当前 6 个 kind 的清单，行为零变化，测试全绿 |
| 2 | `FeatureWidgets.cs` 的 34 处分支按**能力**归类（多实例/存储/提醒/胶囊/设置节），kind 判断换成描述符能力位 | 同一批行为测试全绿；新增 kind 不再需要碰该文件 |
| 3 | 设置页收敛为"描述符驱动通用开关 + 设置节 UserControl" | 新 kind 不再改 `SettingsViewModel` 的 5 个 partial |
| 4 | 用番茄时钟验收 | 注册点收敛到 1 处；遗漏报错而非靠记得；策略表各加一行属预期 |

顺序理由：1 和 2 是纯重构（可被现有测试完全覆盖，风险最低）；3 触及设置界面（有 UI 回归面，放中间）；4 是验收，不是改造。

### Step 1 已完成（2026-09-11）

`WidgetContentDescriptor` 新增显式 `IsFeatureWidget`（默认 false，6 个功能 kind 上显式置 true）；`FeatureWidgetSettings` 的功能清单改为从 `WidgetContentFactory.DescriptorList` 派生（派生顺序与原先硬编码的顺序逐项一致，所以设置归一化行为零变化）；新增 `FeatureWidgetKindContractTests`（4 个测试）把功能清单、非功能 kind、描述符去重、以及枚举覆盖钉住。全量 3370/3370 绿。

**顺带查出一个既有缺口**：`WidgetKind.Productivity` 在枚举里，但既没有内容描述符也没有 `WidgetRegistry` 条目——**没有任何东西能创建它，也没有任何东西枚举过枚举所以一直没被发现**。已作为 known gap 写进契约测试（这样第二个缺口出现时会报错）。需要你决定：给它补一条描述符（像 Tags 那样 Placeholder/Planned），还是把枚举成员删掉。

## 五、明确不做

- 不拆程序集、不做 C ABI、不做包格式/签名链/安装管线——理由见 2026-09-11 的产品决定（Store 主渠道下"按需下载代码"被政策阻断；三包内存实测 +13.2MB；进程内原生代码无故障隔离）。
- 不改动 6 个 `App.Aot*Smoke.cs` 的宿主 partial 形态——这是 NativeAOT 的硬约束，不是可优化项。
- 不为"可选"引入第二套运行时；可选功能与内置功能走同一条接入缝，用开关控制可见性（新装默认全关，已是现状）。
