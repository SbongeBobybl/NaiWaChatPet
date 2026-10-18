# DeskBox JSON source generation 迁移基线与后续设计

- 审计日期：2026-08-22（4A 人工验收完成于 2026-08-20）
- 阶段：4B-4 已完成；后续隔离 AOT 冒烟与 5B-4C3B2B1 产品 activation envelope 继续按独立 context 登记
- 范围：`src/DeskBox` 中生产代码的 `System.Text.Json` 调用、相关 DTO、持久化格式和现有回归测试
- 本阶段边界：只在主程序与更新器的 `DeskBoxAotAudit=true` 配置中关闭 System.Text.Json 默认反射，并让隔离审计脚本显式传入、记录该开关；普通 Debug/Release JIT 默认值保持未设置。AOT fixture 调用只写隔离预览根下的结构化证据或恢复意图；5B-4C3B2B1 新增的产品 envelope 使用独立 schema 1 spool，不改变既有用户数据 store 格式

## 1. 结论

1. 当前生产源码共有 65 处 `JsonSerializer.Serialize*` / `Deserialize*` 调用，分布在 29 个文件。4B 的 49 处产品调用仍全部显式绑定到原有 14 个分域 `JsonSerializerContext`；后续 AOT fixture 各使用独立 evidence context，5B-4C3B2B1 的产品 activation envelope store 增加 1 处写入和 1 处读取。当前共 27 个 context 所有者，反射型重载仍为 0。
2. 原有产品调用不能共用一个全局 options/context。现有产品格式至少包含 Web 默认、大小写不敏感读取、默认格式、camelCase 缩进格式、camelCase 紧凑格式、字符串枚举和数字枚举等不同契约；AOT evidence 文件分别使用独立的 camelCase、缩进、metadata context，不并入用户数据 store；activation envelope 也保持独立 schema/context。
3. 4B-0 基线中的非泛型 `JsonStringEnumConverter` 共 7 个。4B-1/4B-2/4B-3B 已用各分域 context 的 source-generation 字符串枚举支持全部替换，当前数量为 0。
4. `GlanceImageService` 的图片目录和 `SearchHistoryService` 的最近结果使用数字枚举。迁移时如果误用字符串枚举 context，会改变已落盘格式。
5. 旧规划只记录了两个泛型读取/校验 helper。实际共有 3 个泛型 JSON helper：两个读取/校验入口和一个原子写入入口。它们的实际类型集合均为有限白名单，不需要保留开放泛型反射路径。
6. 阶段 4B-4 已完成。主程序和更新器只有在 `DeskBoxAotAudit=true` 时将 `JsonSerializerIsReflectionEnabledByDefault` 求值为 `false`，普通构建求值为空；配置 13 / schema 10 的隔离 AOT 审计明确记录该值为 `false`，且 JSON 警告和反射回退错误均为 0。5B-4C3B2B1 的 profile 56 / schema 53 审计继续满足同一门禁，并编译当前全部 27 个 context 所有者。

## 2. 4A 放行记录

阶段 4A 的 8 个 FolderPicker 调用入口已经完成代码、自动化和 x64 AOT 产物审计。2026-08-20，用户完成人工入口矩阵并确认无问题，覆盖以下产品表面：

| 产品表面 | 调用数 | 人工核对范围 | 结果 |
| --- | ---: | --- | --- |
| 设置：维护 | 2 | 选择、取消、前台和设置窗口 owner | 通过 |
| 设置：存储与更新 | 1 | 选择、取消、前台和设置窗口 owner | 通过 |
| 首次引导 | 1 | 选择、取消、前台和引导窗口 owner | 通过 |
| Glance 设置 | 1 | 选择、取消、前台和组件设置窗口 owner | 通过 |
| 桌面整理 | 1 | 选择、取消、前台和任务窗口 owner | 通过 |
| 托盘 | 1 | 选择、取消、前台和稳定托盘宿主 owner | 通过 |
| JumpList | 1 | 选择、取消、前台和稳定托盘宿主 owner | 通过 |

该结论属于用户在真实界面完成的人工证据。自动化契约和 AOT 发布审计是独立证据，不能相互替代。4A 因此完成，4B-0 可以执行。

## 3. 生产调用清单

下表中的调用数按源代码中 `JsonSerializer.Serialize*` 和 `JsonSerializer.Deserialize*` 的直接出现次数计算。泛型 helper 只按 helper 内部的实际序列化调用计数，其运行时类型白名单另列于第 6 节。

| 批次 | 文件 | 调用数 | 根类型或文档 | 当前 options 与格式 |
| --- | --- | ---: | --- | --- |
| 5B-1 | `App.AotShortcutSmoke.cs` | 1 | `AotShortcutSmokeResult` | NativeAOT-only；独立 metadata context，camelCase、缩进，只写隔离预览根的 `result.json` |
| 5B-2A | `App.AotShellSmoke.cs` | 1 | `AotShellSmokeResult` | NativeAOT-only；独立 metadata context，camelCase、缩进，只写隔离预览根的 `result.json` |
| 5B-2B | `App.AotQuickAccessMutationSmoke.cs` | 1 | `AotQuickAccessMutationSmokeResult` | NativeAOT-only；独立 metadata context，camelCase、缩进，只写隔离预览根的正常/补偿证据 `result.json` |
| 5B-3A | `App.AotMusicVolumeReadSmoke.cs` | 1 | `AotMusicVolumeReadSmokeResult` | NativeAOT-only；独立 metadata context，camelCase、缩进，只写隔离预览根的系统/session 音量只读证据 `result.json` |
| 5B-3B | `App.AotMusicVolumeMutationSmoke.cs` | 2 | `AotMusicVolumeMutationSmokeResult`、`AotMusicVolumeRecoveryIntent` | NativeAOT-only；同一个独立 metadata context；泛型 helper 原子写入场景结果与持久恢复意图，并 source-generated 读取恢复意图；只访问隔离 preview 根 |
| 5B-3C | `App.AotMusicVolumeSessionMutationSmoke.cs` | 2 | `AotMusicVolumeSessionMutationSmokeResult`、`AotMusicVolumeSessionRecoveryIntent` | NativeAOT-only；同一个独立 metadata context；泛型 helper 原子写入场景结果与匹配 session 恢复意图，并 source-generated 读取恢复意图；只访问隔离 preview 根 |
| 5B-4A | `App.AotManagedUiSmoke.cs` | 1 | `AotManagedUiSmokeResult` | NativeAOT-only；独立 metadata context，camelCase、缩进；只写隔离预览根的托盘、Widget 恢复、语言、设置与搜索只读矩阵证据 |
| 5B-4C2A | `App.AotHotkeySmoke.cs` | 1 | `AotHotkeySmokeResult` | NativeAOT-only；独立 metadata context；只写隔离热键生命周期证据 |
| 5B-4C3A | `App.AotTodoRecurrenceReminderSmoke.cs` | 1 | `AotTodoRecurrenceReminderSmokeResult` | NativeAOT-only；独立 metadata context；只写固定时钟、owned store 的 recurrence/reminder 证据 |
| 5B-4C3B1 | `App.AotTodoNotificationLifecycleSmoke.cs` | 1 | `AotTodoNotificationLifecycleSmokeResult` | NativeAOT-only；独立 metadata context；只写通知展示、历史恢复与清理证据 |
| 5B-4C3B2A | `App.AotTodoNotificationActivationSmoke.cs` | 1 | `AotTodoNotificationActivationSmokeResult` | NativeAOT-only；独立 metadata context；只写受控 activation 路由证据 |
| 5B-4C3B2B1 | `App.AotTodoNotificationForwardingSmoke.cs` | 1 | `AotTodoNotificationForwardingSmokeResult` | NativeAOT-only；独立 metadata context；只写冷启动和真实第二实例转发证据 |
| 5B-4C3B2B1 | `NativeNotificationActivationEnvelopeStore.cs` | 2 | `NativeNotificationActivationEnvelope` | 产品 schema 1 spool；camelCase、缩进、metadata context；原子写入和 source-generated 读取，保存 arguments、`UserInput`、来源 PID 与时序信息 |
| 4B-1 | `AppUpdateService.cs` | 2 | `AppUpdateManifest`、`GitHubReleaseResponse` | 已迁移；`AppUpdateJsonContext` 保持 Web camelCase、大小写不敏感、字符串数字读取和 GitHub `JsonPropertyName` |
| 4B-1 | `CitySearchService.cs` | 1 | `List<PredefinedCity>` | 已迁移；复用 `WeatherJsonContext`，保持大小写不敏感和 `zh`、`country_zh` 等固定字段名 |
| 4B-3B | `DeskBoxAttachmentHealthService.cs` | 1 | 泛型 `T`，实际为 `QuickCaptureStoreData`、`TodoWidgetData` | 已迁移；复用 Quick Capture/Todo context 的兼容实例，保持大小写不敏感与字符串/数字枚举读取；只读健康扫描 |
| 4B-3B/3C | `DeskBoxDataBackupService.cs` | 11 | `DeskBoxBackupManifest`、`PendingRestoreMarker`、`AppSettings`、`QuickCaptureStoreData`、`TodoWidgetData` | 已全部迁移；用户数据复用 4B-2 兼容 context，控制文档由私有 `BackupJsonContext` 保持 camelCase、缩进、大小写敏感与 schema 1/2 格式 |
| 4B-1 | `DeskBoxDiagnosticsBundleService.cs` | 1 | `DeskBoxDiagnosticSnapshot` 完整对象图 | 已迁移；`DiagnosticsJsonContext` 保持 camelCase、缩进和字符串枚举；只写诊断包 |
| 4B-3A | `DesktopOrganizationRecoveryStore.cs` | 2 | `DesktopOrganizationRecoveryJournal` | 已迁移；`DesktopRecoveryJsonContext` 保持 camelCase、缩进；当前对象图无普通枚举；读写临时恢复日志 |
| 4B-2 | `GlanceImageService.cs` | 2 | `List<GlanceImageInfo>` | 已迁移；`GlanceImageCatalogJsonContext` 保持 camelCase、缩进和数字枚举；读写可丢弃缓存目录 |
| 4B-2 | `GlanceWidgetStore.cs` | 7 | `GlanceWidgetData` | 已迁移；`GlancePreferencesJsonContext` 保持 camelCase、缩进和字符串枚举；覆盖保存、加载、旧单文件迁移和深拷贝 |
| 4B-1 | `LocalizationService.cs` | 1 | `Dictionary<string,string>` | 已迁移；`LocalizationJsonContext` 使用默认 options，字典键和值原样读取 |
| 4B-2 | `QuickCaptureStore.cs` | 2 | `QuickCaptureStoreData` | 已迁移；`QuickCaptureJsonContext` 保持 camelCase、缩进和字符串枚举；弹性主文件/备份读取不变 |
| 4B-3A | `SearchHistoryService.cs` | 2 | 私有 `PersistedData` / `PersistedResult` | 已迁移；`SearchHistoryJsonContext` 保持 camelCase、缩进和数字 `SearchResultKind`；未版本化用户历史 |
| 4B-3A | `SearchIndexService.cs` | 3 | 私有旧 `PersistedIndex`、私有 `RootManifest` | 已迁移；`SearchIndexJsonContext` 保持 camelCase、紧凑；旧索引 JSON 只读，当前主索引为 DBIX 二进制；roots manifest 读写 |
| 4B-2 | `SettingsService.cs` | 2 | `AppSettings` | 已迁移；`SettingsJsonContext` 保持 camelCase、缩进和字符串枚举；`WidgetKind` 类型级容错 converter 不变 |
| 4B-2 | `TodoWidgetStore.cs` | 2 | `TodoWidgetData` | 已迁移；`TodoJsonContext` 保持 camelCase、缩进和字符串枚举；弹性主文件/备份读取不变 |
| 4B-1 | `WeatherService.cs` | 3 | `WeatherGeocodingResult`、`WeatherData`、`MsnWeatherResponse` | 已迁移；`WeatherJsonContext` 保持大小写不敏感，只读外部 API 响应 |
| 4B-2 | `WidgetFileStackSettings.cs` | 7 | `Dictionary<string,string>`、`Dictionary<string,List<string>>`、`List<string>` | 已迁移；`WidgetMetadataJsonContext` 使用默认 options，metadata 子文档继续保持紧凑、字典键和数组值原样 |
| 合计 | 29 个文件 | 65 |  |  |

批次数量为：4B-1 迁移 8 处，4B-2 迁移 22 处，4B-3A 迁移 7 处，4B-3B/3C 各计划迁移 6 处，总计 49 处。
当前状态为：4B-1/4B-2/4B-3A/4B-3B/4B-3C 的 49 处产品调用已全部迁移；后续隔离 AOT evidence 与 activation envelope store 均登记在精确固定清单中。65/65 均显式使用 source-generated `JsonTypeInfo`，反射型调用为 0。

## 4. 必须保持的格式行为

### 4.1 通用行为

- 所有现有 options 都没有设置 `UnmappedMemberHandling = Disallow`，因此未知 JSON 属性默认忽略。
- 缺失属性由 CLR 属性初始化值、构造函数默认值和各 store 的 Normalize/Migrate 逻辑共同处理。source generation 迁移不能绕过这些后处理步骤。
- 所有现有 options 都没有设置 `DefaultIgnoreCondition`，因此可序列化的 null 属性当前仍会写入。
- 当前没有 DictionaryKeyPolicy。字典键必须保持原样，不能随属性命名策略一起转换。
- `PropertyNamingPolicy = CamelCase` 与 `PropertyNameCaseInsensitive = true` 是两个独立行为。只有显式启用大小写不敏感的读取路径才能接受任意属性名大小写。
- 未知 JSON 属性与未知枚举值不是同一行为。普通 `JsonStringEnumConverter` 遇到未知枚举名称会抛出 `JsonException`；弹性 store 随后可能隔离主文件并恢复备份。

### 4.2 枚举格式

| 当前格式 | 所有者 | 写入 | 读取兼容性 |
| --- | --- | --- | --- |
| source-generated 字符串枚举兼容实例 | `DeskBoxAttachmentHealthService` | 只读 | 4B-3B 已替换非泛型 converter；枚举名称和整数均可读，属性名大小写不敏感 |
| source-generated 字符串枚举兼容实例 | `DeskBoxDataBackupService` 用户数据路径 | 名称字符串 | 4B-3B 已替换非泛型 converter；名称字符串和整数均可读，属性名大小写不敏感 |
| source-generated 字符串枚举 | `DeskBoxDiagnosticsBundleService` | 名称字符串 | 4B-1 已替换非泛型 converter；只写路径 |
| source-generated 字符串枚举 | `GlanceWidgetStore` | 名称字符串 | 名称字符串和整数均可读 |
| source-generated 字符串枚举 | `QuickCaptureStore` | 名称字符串 | 名称字符串和整数均可读 |
| source-generated 字符串枚举 | `SettingsService` | 名称字符串 | 名称字符串和整数均可读；`WidgetKindJsonConverter` 将未知值降级为 `File` |
| source-generated 字符串枚举 | `TodoWidgetStore` | 名称字符串 | 名称字符串和整数均可读 |
| 数字枚举 | `GlanceImageService` | `onlineCategory`、`onlineProvider` 写整数 | 现有目录的整数必须继续可读；写入也保持整数 |
| 数字枚举 | `SearchHistoryService` | `PersistedResult.kind` 写整数 | 现有历史的整数必须继续可读；写入也保持整数 |

阶段 4B-3B 已消除基线中的全部 7 个非泛型 converter，当前为 0。各域仍不能改用一个全局字符串枚举策略；字符串枚举域使用 source-generation 的字符串枚举支持，数字枚举域不注册字符串 converter。

## 5. 版本与兼容性基线

| 数据域 | 当前版本/形态 | 已有兼容路径 | 4B 迁移要求 |
| --- | --- | --- | --- |
| Settings | `AppSettings.SchemaVersion = 1`，另有字段级迁移版本 | 缺文件、主文件损坏恢复、旧功能字段、未知 `WidgetKind` 等迁移 | 保持字段默认值、类型级 converter、Normalize/Migrate 顺序和备份恢复 |
| Quick Capture | store version 4 | 旧内联图片、超长正文、旧 image path、损坏主文件恢复 | 保持字符串写入、整数枚举读取、缺集合补空和版本归一化 |
| Todo | store version 3 | 缺字段归一化、损坏主文件隔离、备份恢复 | 保持字段命名、null、集合和弹性存储行为 |
| Glance preferences | version 8 | 旧单文件迁移到按组件文件，Normalize 修正非法值 | 保持字符串枚举、旧单文件迁移和克隆语义 |
| Glance image catalog | 未版本化、可丢弃缓存 | 缺失/损坏返回空目录 | 保持数字枚举和 camelCase；未知字段忽略 |
| Backup/restore | 当前 schema 2，兼容 schema 1 | 完整性 manifest、路径穿越防护、未来应用版本拒绝、缺 settings 拒绝 | 私有 record 构造参数、schema 1/2 和 pending marker 格式全部保持 |
| Search index | 当前 DBIX 二进制；旧 JSON 只读 | 旧 JSON 加载后保存为紧凑二进制 | 不能删除 `PersistedIndex` source metadata；roots manifest 继续紧凑 camelCase |
| Search history | 未版本化 | 缺集合补空、读取失败保留空默认 | 保持数字 `SearchResultKind`、未知字段忽略和缺字段默认 |
| Desktop recovery | 未版本化临时事务日志 | 缺文件返回 null；成功后清理 | 保持 camelCase、原子替换、未知字段忽略和属性初始化值 |
| Widget metadata JSON | 未版本化子文档 | 无效 metadata 被规范化或丢弃 | 保持默认命名、字典键和数组顺序 |
| 网络/资源 DTO | 外部或嵌入只读 | 缺必填业务字段由上层验证；未知字段忽略 | 保持大小写策略、`JsonPropertyName` 和 Web 数字读取行为 |
| Diagnostics | snapshot 自带 schema 字段、只写 | 无反向读取 | 保持 camelCase、缩进和字符串枚举，不能扩大导出内容 |

## 6. JSON helper 实际白名单与收口

| helper | 文件 | 实际类型 | 4B-3 目标形式 |
| --- | --- | --- | --- |
| `ReadJson<T>` | `DeskBoxAttachmentHealthService.cs` | `QuickCaptureStoreData`、`TodoWidgetData` | 4B-3B 已增加必需的 `JsonTypeInfo<T>` 参数 |
| `ValidateJsonFileIfPresent<T>` | `DeskBoxDataBackupService.cs` | `AppSettings`、`QuickCaptureStoreData`、`TodoWidgetData` | 4B-3B 已增加必需的 `JsonTypeInfo<T>` 参数；不做运行时反射类型发现 |
| `WritePendingRestoreMarkerAtomicallyAsync` | `DeskBoxDataBackupService.cs` | 仅 `PendingRestoreMarker` | 4B-3C 已改为非泛型 marker 专用入口并显式使用 source-generated `JsonTypeInfo` |

这 3 个 helper 没有开放插件类型需求。两个读取/校验 helper 继续使用带显式类型信息的有限泛型入口，pending marker 写入 helper 已收口为非泛型专用入口；这比 `DynamicallyAccessedMembers`、宽泛 trimming root 或继续使用反射泛型更符合当前实际调用面。

## 7. 4B-0 冻结契约与 4B-1 至 4B-4 收口契约

`JsonSerializationBaselineContractTests` 新增以下自动化边界：

1. 精确冻结 29 个生产文件和每个文件的调用数，总计 65；同时精确冻结当前 27 个 context 所有者。新增清单包括 Hotkey、Todo recurrence/reminder、通知生命周期、activation 路由和转发 evidence，以及产品 activation envelope store 的两处读写。
2. 确认生产代码中的非泛型与泛型 `JsonStringEnumConverter` 均为 0；字符串枚举能力由各分域 source-generated context 提供。
3. 精确冻结两个读取/校验 helper 的类型白名单并要求显式 `JsonTypeInfo<T>`；原反射型原子写入 helper 已收口为非泛型 pending marker 专用入口。
4. 搜索历史金样确认 camelCase、数字枚举、缺字段默认和未知字段忽略。
5. 桌面整理恢复日志金样确认 camelCase、缩进、缺字段初始化和未知字段忽略。
6. Quick Capture/Glance 金样确认字符串枚举写入；Quick Capture 继续接受旧整数枚举。
7. Glance 图片目录金样确认数字枚举、缺字段和未知字段读取。
8. `GlanceCalendarAndImageServiceTests` 在真实目录写入路径上确认 `onlineCategory` 和 `onlineProvider` 仍写为整数。
9. 4B-1 契约精确确认 8 个调用各引用唯一的 source-generated `JsonTypeInfo`，并冻结 AppUpdate、Weather、Localization 和 Diagnostics 四类 options 配置。
10. 更新 manifest 测试同时覆盖大小写不敏感、Web 数字字符串和未知字段；天气测试覆盖 geocoding、Open-Meteo、MSN 三类嵌套响应的大小写与未知字段；诊断测试确认枚举仍写为名称字符串。
11. 4B-2 契约精确确认 22 个调用分别引用 6 个 context 的 8 个 `JsonTypeInfo` 属性，并冻结四类字符串枚举 store、Glance 数字枚举目录及默认格式 metadata 的 options 配置。
12. Settings、Quick Capture 和 Glance preferences 金样确认旧整数枚举仍可读且新写入为名称字符串；Glance image catalog 继续写数字枚举；Widget metadata 继续紧凑写入并保留大小写敏感的字典键。
13. 4B-3B 契约精确确认附件健康和备份用户数据路径复用 3 个既有 context 类型的兼容实例，所有读取/校验 helper 均显式接收有限白名单的 `JsonTypeInfo<T>`。
14. 附件健康与备份恢复金样同时覆盖混合大小写属性、字符串枚举、旧数字枚举、未知字段忽略和 managed attachment 路径重定位。
15. 4B-3C 契约精确确认 6 个控制文档调用分别绑定 `BackupManifest` 与 `PendingRestoreMarker` 类型信息，并冻结私有 metadata context 的 camelCase、缩进和大小写敏感配置。
16. 控制文档金样覆盖 manifest/file manifest 的规范字段与未知字段、pending marker 规范写入与未知字段读取，以及 PascalCase manifest 继续被拒绝。
17. 4B-4 契约确认主程序与更新器各自只有一个 `JsonSerializerIsReflectionEnabledByDefault=false`，且都位于 `DeskBoxAotAudit=true` 条件组；审计脚本在 restore/publish 两处显式传入该开关，并在 schema 10 摘要中记录实际审计值。

已有测试继续承担以下兼容范围：

- `AppUpdateServiceTests`：manifest、GitHub fallback、架构字段，以及 Web 默认的大小写和字符串数字行为。
- `CitySearchServiceTests`：嵌入城市资源实际加载。
- `WeatherServiceTests`：三类 source-generated 外部响应对象图及转换逻辑。
- `DeskBoxDiagnosticsBundleServiceTests`：诊断 JSON、字符串枚举、脱敏和导出边界。
- `DeskBoxAttachmentHealthServiceTests`：Quick Capture/Todo 实际读取及损坏文件隔离。
- `DeskBoxDataBackupServiceTests`：schema 1/2、缺 settings、无效 JSON、未来版本、篡改和 pending restore。
- `SettingsServiceTests`：缺文件、损坏恢复、旧字段迁移和未知 `WidgetKind`。
- `QuickCaptureServiceTests`、`TodoWidgetStoreTests`、`GlanceWidgetStoreTests`：版本迁移、规范化、损坏恢复和保存格式。
- `SearchIndexServiceTests.SaveIndex_MigratesLegacyJsonToCompactBinary_AndPreservesResults`：旧 JSON 到 DBIX 的兼容读取。
- `WidgetFileStackSettingsTests`：metadata 子文档往返和顺序保持。
- `LocalizationResourceContractTests`：12 份语言资源键、值和占位符一致。

## 8. 分域 context 设计

第一目标是移除 AOT 不安全的反射入口并保持格式，不以本批追求最快序列化。4B-1/4B-2 的各 context 均采用 metadata generation；后续可单独评估适合 fast-path 的只写 DTO。

### 8.1 4B-1：叶子资源、网络 DTO 和诊断

| context | 类型 | options 约束 |
| --- | --- | --- |
| `AppUpdateJsonContext` | `AppUpdateManifest`、私有 GitHub release/asset DTO | 已实现为 partial service 内的私有 context；Web 默认、大小写不敏感、字符串数字读取和 `JsonPropertyName` 均保持 |
| `WeatherJsonContext` | `PredefinedCity`、`List<PredefinedCity>`、`WeatherGeocodingResult`、`WeatherData`、`MsnWeatherResponse` 及对象图 | 已实现并由城市/天气共享；只启用大小写不敏感，不增加命名策略或枚举 converter |
| `LocalizationJsonContext` | `Dictionary<string,string>` | 已实现；默认 options，不设置 DictionaryKeyPolicy |
| `DiagnosticsJsonContext` | `DeskBoxDiagnosticSnapshot` 及对象图 | 已实现；camelCase、缩进、source-generated 字符串枚举，已替换该文件的非泛型 converter |

AppUpdate 的 context 已放在对应 partial service 内部，因此 GitHub DTO 保持 private；本批没有为 source generation 扩大网络 DTO 的公开可见性。

4B-1 只修改上述 5 个文件中的 8 处调用。它不修改任何用户持久化文件，不触碰 Rust ABI、Rust 后端选择、FolderPicker、音乐 COM 或 XAML binding。

### 8.2 4B-2：用户持久化 store

- `SettingsJsonContext`：已实现；`AppSettings` 对象图使用 camelCase、缩进和字符串枚举，并保留 `WidgetKindJsonConverter`。
- `QuickCaptureJsonContext`：已实现；`QuickCaptureStoreData` 对象图使用 camelCase、缩进和字符串枚举。
- `TodoJsonContext`：已实现；`TodoWidgetData` 对象图使用 camelCase、缩进和字符串枚举。
- `GlancePreferencesJsonContext`：已实现；`GlanceWidgetData` 使用 camelCase、缩进和字符串枚举。
- `GlanceImageCatalogJsonContext`：已实现；`List<GlanceImageInfo>` 使用 camelCase、缩进，明确不使用字符串枚举。
- `WidgetMetadataJsonContext`：已实现；现有三类 Dictionary/List 子文档使用默认 options。

Glance preferences 与 Glance image catalog 必须使用不同 context/options，不能因类型同属 Glance 而合并枚举策略。

4B-2 只修改上述 6 个生产文件中的 22 处调用。弹性 store 的加载、Normalize/Migrate、备份恢复和 Glance 旧单文件迁移顺序均保持不变；Rust ABI、后端选择、FolderPicker、音乐 COM、搜索和 XAML 均未修改。

### 8.3 4B-3：跨域维护和搜索

- **4B-3A（已完成）** 迁移搜索历史、搜索索引和桌面恢复 3 个文件中的 7 处调用；三类数据分别使用独立 metadata context，未修改搜索 DBIX 二进制格式。
- **4B-3B（已完成）** 迁移附件健康和备份服务中的用户数据读取、校验与附件路径重定位 6 处调用，并移除最后 2 个非泛型 `JsonStringEnumConverter` 所有者。
- **4B-3C（已完成）** 迁移备份 manifest、file manifest 和 pending marker 等控制文档 6 处调用。
- 备份与附件健康已复用 4B-2 的 Settings/Quick Capture/Todo context 类型，但没有直接使用 `*.Default`：每个 context 使用独立兼容 options 实例构造，保持 camelCase、缩进、source-generated 字符串枚举和 `PropertyNameCaseInsensitive=true`。PascalCase/混合大小写旧数据金样已证明兼容性，无需新增超大维护 context。
- `BackupJsonContext` 已嵌套在 partial backup service 内，仅拥有私有 manifest、file manifest 和 pending marker；没有扩大控制 DTO 可见性。
- `DesktopRecoveryJsonContext` 负责桌面恢复日志。
- `SearchHistoryJsonContext` 嵌套在 history service 内，保持数字 `SearchResultKind`。
- `SearchIndexJsonContext` 嵌套在 index service 内，保留私有旧 JSON index 和 roots manifest；不改变 DBIX 二进制实现。
- 读取与校验两个泛型 helper 已改为显式 `JsonTypeInfo<T>`；pending marker 原子写入 helper 已收口为非泛型专用入口。

### 8.4 4B-4：关闭默认反射序列化（已完成）

主程序与更新器的 AOT 审计条件组现已设置 `JsonSerializerIsReflectionEnabledByDefault=false`；隔离脚本还在两个项目的 restore 和主程序 publish 中以全局 MSBuild 属性显式传入同一值。普通构建不设置该属性，经实际 MSBuild 求值确认仍为空，因此本批没有改变普通 JIT 默认行为。

49 处生产调用全部显式绑定 source-generated `JsonTypeInfo` 后，默认反射关闭的 x64 AOT publish 继续成功，摘要明确记录 `jsonSerializer.reflectionEnabledByDefault=false`。当前不把开关扩大到普通 Debug/Release：4B 的目标是建立 AOT fail-fast 门槛并证明调用面完整，正式 JIT 行为继续作为稳定基线。

本批只要求 `System.Text.Json` 和非泛型 `JsonStringEnumConverter` 来源的 IL2026/IL3050 归零，不要求其他反射、COM 或 XAML 警告同时归零。

## 9. 4B-2 至 4B-4 完成状态

4B-2 以 4B-1 的 176 条 JSON 直接相关警告和已验证格式金样为输入基线，完成结果如下：

- 仅迁移计划中的 6 个生产文件、22 处调用；每处调用显式使用 source-generated `JsonTypeInfo`，生产直接调用总数仍为 49。
- 新增 6 个 metadata context，当前共 10 个；已迁移调用从 8 处增至 30 处，反射型调用从 41 处降至 19 处。
- Settings、Quick Capture、Todo 和 Glance preferences 的 4 个非泛型 converter 已移除；当前只剩附件健康和备份服务 2 个所有者。
- 用户数据兼容金样确认字符串枚举的新写入与旧整数读取、未知字段忽略、Glance 图片数字枚举和 Widget metadata 紧凑格式/字典键均保持不变；原有 Normalize/Migrate 与弹性恢复测试继续通过。
- x64 全量测试和隔离 AOT 审计通过。JSON 直接相关 IL2026/IL3050 从 176 条降到 80 条，其中反射型 `JsonSerializer` 从 164 条降到 76 条，非泛型枚举 converter 从 12 条降到 4 条；本轮 6 个迁移文件均为 0 条；警告类别没有增加。
- AOT 主程序仍未启动；真实 AOT 运行冒烟保持在阶段 5。

4B-3A 以 4B-2 的 80 条 JSON 直接相关警告为输入基线，完成结果如下：

- 只迁移搜索历史、搜索索引和桌面恢复 3 个生产文件、7 处调用；增加 3 个 metadata context。当前 context 所有者共 13 个，已迁移调用从 30 处增至 37 处，反射型调用从 19 处降至 12 处。
- 搜索历史继续写数字 `SearchResultKind`；搜索索引继续读取旧 camelCase JSON，并保持当前 DBIX 二进制格式及紧凑 roots manifest；桌面恢复继续写 camelCase 缩进日志。
- 原有搜索历史和桌面恢复金样继续覆盖未知/缺失字段；搜索索引旧 JSON → DBIX 测试增加根级与条目级未知字段，确认 forward compatibility。
- 4B-3A 定向测试 55/55、x64 全量测试 1987/1987、规范 Debug 构建和隔离 x64 AOT 审计通过。JSON 直接相关 IL2026/IL3050 从 80 条降到 52 条；本轮 3 个迁移文件均为 0 条，剩余记录只来自附件健康 6 条和备份服务 46 条。
- AOT 审计仍为配置 12 / schema 9、39 个发布文件、3 个分离 PDB、12 类既有警告、1 条音乐音量 `always-throw` 和 0 条 shortcut `always-throw`；工作树前后指纹一致，Rust ABI 2、能力 31、staging/publish 哈希一致。AOT 主程序未启动。

4B-3B 以 4B-3A 的 52 条 JSON 直接相关警告为输入基线，完成结果如下：

- 只迁移附件健康和备份服务中的用户数据读取、校验及附件路径重定位 6 处调用；没有新增 context 所有者。已迁移调用从 37 处增至 43 处，反射型调用从 12 处降至 6 处。
- Settings、Quick Capture 和 Todo 的既有 metadata context 分别使用独立兼容 options 实例，明确保留 camelCase、缩进、属性名大小写不敏感、字符串枚举写入和旧数字枚举读取。
- `ReadJson<T>` 与 `ValidateJsonFileIfPresent<T>` 均要求调用方显式传入有限白名单的 `JsonTypeInfo<T>`；生产代码中的非泛型 `JsonStringEnumConverter` 从 2 个降为 0。pending marker 的原子写入 helper 未提前修改。
- 迁移前新增附件健康与备份恢复兼容金样；旧反射实现先通过，迁移后继续通过。金样覆盖混合大小写属性、字符串/旧数字枚举、未知字段以及 managed attachment 重定位。
- 4B-3B 定向测试 210/210、x64 全量测试 1990/1990、隔离 x64 AOT 审计通过。JSON 直接相关 IL2026/IL3050 从 52 条降到 24 条；附件健康服务为 0，剩余 24 条全部来自备份服务中留给 4B-3C 的 6 个控制文档调用。
- AOT 审计仍为配置 12 / schema 9、39 个发布文件、3 个分离 PDB、12 类既有警告、1 条音乐音量 `always-throw` 和 0 条 shortcut `always-throw`；工作树前后指纹一致，Rust ABI 2、能力 31、staging/publish 哈希一致。AOT 主程序未启动。

4B-3C 以 4B-3B 的 24 条 JSON 直接相关警告为输入基线，完成结果如下：

- 只迁移 `DeskBoxDataBackupService` 的 manifest、file manifest 和 pending marker 6 处控制文档调用；新增 1 个私有 metadata context。context 所有者从 13 个增至 14 个，已迁移调用从 43 处增至 49 处，反射型调用从 6 处降至 0。
- `BackupJsonContext` 嵌套在 partial service 内，私有 DTO 无需扩大可见性；保持 camelCase、缩进、默认大小写敏感和未知字段忽略，不注册字符串枚举 converter。
- 原 `WriteJsonAtomicallyAsync<T>` 仅有 `PendingRestoreMarker` 一个实际类型，现已改为非泛型 `WritePendingRestoreMarkerAtomicallyAsync` 并显式使用对应 `JsonTypeInfo`。两个读取/校验泛型 helper 继续要求显式有限白名单类型信息。
- 迁移前新增并先由旧反射实现通过控制文档金样；迁移后继续通过。金样覆盖规范 manifest/file manifest 字段、缩进、unknown fields、pending marker 读取/清理，以及控制 manifest 的大小写敏感行为。
- 4B-3C 定向测试 31/31、x64 全量测试 1993/1993、隔离 x64 AOT 审计通过。与 `JsonSerializer` / `JsonStringEnumConverter` 直接相关的 IL2026/IL3050 从 24 条降到 0，备份服务本身的 IL2026/IL3050 也为 0。
- AOT 审计仍为配置 12 / schema 9、39 个发布文件、3 个分离 PDB、12 类既有警告、1 条音乐音量 `always-throw` 和 0 条 shortcut `always-throw`；工作树前后指纹一致，Rust ABI 2、能力 31、staging/publish 哈希一致。AOT 主程序未启动。

4B-4 以 49/49 处 source-generated 调用和 0 条 JSON 直接相关警告为输入基线，完成结果如下：

- 主程序与更新器各自在 `DeskBoxAotAudit=true` 条件组设置 `JsonSerializerIsReflectionEnabledByDefault=false`；普通构建实际求值为空，审计构建实际求值为 `false`。
- 审计脚本在 restore/publish 参数中显式传入该开关，审计配置从 12 升级为 13；摘要增加 `jsonSerializer.reflectionEnabledByDefault`，schema 从 9 升级为 10。
- 新契约先在旧实现上以“属性集合为空”按预期失败，实施后转为通过。AOT/JSON 定向契约 32/32、x64 全量测试 1994/1994 通过。
- 隔离 x64 AOT publish 继续产出 39 个发布文件和 3 个 PDB；摘要记录反射默认值为 `false`，JSON IL2026/IL3050 为 0，未出现反射序列化关闭或缺少 `TypeInfoResolver` 的回退错误。
- 12 类既有警告、1 条音乐音量 `always-throw`、0 条 shortcut `always-throw` 均未变化；unexpected warning 与 unexpected always-throw 为 0，工作树前后指纹一致，Rust ABI 2、能力 31、staging/publish 哈希一致。
- 本批没有修改任何生产序列化调用、DTO 或持久化格式，也没有启动 AOT 主程序。4B JSON 阶段到此收口，后续剩余 AOT 工作按 4C/4D/4E 分域推进。

## 10. 验证记录

| 证据 | 命令或来源 | 结果 |
| --- | --- | --- |
| 4A 人工矩阵 | 用户在 2026-08-20 的实际界面验收 | 通过 |
| 4B-2 定向兼容测试 | 6 个迁移域、JSON 基线契约及相关恢复/格式测试过滤执行 | 287/287 通过 |
| 4B-3A 定向兼容测试 | JSON 基线、搜索索引/历史、桌面恢复和相关行为测试过滤执行 | 55/55 通过 |
| 4B-3B 定向兼容测试 | JSON 基线、附件健康、备份恢复及 Settings/Quick Capture/Todo 相关行为测试过滤执行 | 210/210 通过 |
| 4B-3C 定向兼容测试 | JSON 基线与备份控制文档格式、读取、恢复和清理测试过滤执行 | 31/31 通过 |
| 4B-4 定向契约 | AOT 发布契约与全部 JSON 基线契约过滤执行 | 32/32 通过 |
| Debug 构建 | `dotnet build .\src\DeskBox\DeskBox.csproj --no-restore --verbosity:minimal` | 0 错误；本次增量构建报告 24 条既有 C# 警告，未重新发出此前 6 条 XAML WMC1506；source generator 无诊断 |
| x64 全量测试 | `dotnet test .\tests\DeskBox.Tests\DeskBox.Tests.csproj --no-restore --verbosity:minimal -p:Platform=x64` | 1994/1994 通过 |
| x64 AOT 审计 | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\publish-aot-audit.ps1 -Platform x64` | 配置 13 / schema 10；`reflectionEnabledByDefault=false`；39 个发布文件、3 个 PDB、12 类既有警告；JSON 直接相关警告 210→176→80→52→24→0，反射回退错误为 0；源码稳定；1 条音乐音量 `always-throw`，shortcut 为 0；Rust ABI 2、能力 31、同次哈希一致 |
| 规范 Debug 进程 | 重新启动后核对仓库内 `DeskBox.exe` | PID 27008；仓库内实例 1 个；路径精确匹配 `src/DeskBox/bin/Debug/net10.0-windows10.0.22621.0/DeskBox.exe`；启动时 Rust 模块数为 0（尚未触发按需加载） |

上述自动化与 AOT 门槛全部通过，4B-4 与整个 4B JSON source generation 阶段完成。4B 完成时的固定基线是 16 个文件、49/49 处产品调用和 14 个 context 所有者；后续 AOT evidence 与 5B-4C3B2B1 产品 activation envelope 均以独立 context 增量登记。当前固定基线为 29 个文件、65/65 处 source-generated 调用、27 个 context 所有者、0 个非泛型 converter、审计构建默认反射关闭和 0 条 JSON 直接相关警告。

## 2026-09-29 收纳迁移恢复记录登记

新增 ManagedStorageMigrationService 的 2 处 JSON 读写，均显式使用 ManagedStorageMigrationModels 内的 StorageMigrationJsonContext。恢复记录使用独立 schema 1、camelCase 紧凑格式，不向 AppSettings 磁盘格式添加字段。本次精确清单更新为 36 个调用文件、85 处调用、33 个 context 所有者；此前阶段的统计保留为历史记录。