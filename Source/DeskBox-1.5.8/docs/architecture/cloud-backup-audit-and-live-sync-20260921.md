# 云备份实机审计 + 免重启同步路线分析

- 日期：2026-09-21
- 状态：审计结论与方案分析，非实现
- 上游：`sync-protocol-contract-20260918.md`（2C 同步契约）、`module-boundary-roadmap-20260918.md` §10
- 触发：坚果云 WebDAV 实机联调中发现并修复 3 个 Native AOT 缺陷后，对整条链路做了一次完整复查；并就"随记/待办免重启自动同步"给出路线判断。

## 0. 结论

1. **备份通道本身已可靠**：上传/列取/下载/还原/retention 在坚果云实测全通；本审计未发现新的传输级 bug。
2. **已修复的三个 AOT 缺陷是本功能的全部"系统兼容"坑**（详见 §3），与 Win10/Win11 无关——两者 API 面无差异。
3. **"免重启自动同步"可行，但有两条路**：
   - 路线 A（轻量，复用 WebDAV）：快照拉取 + 域内 item 级合并 + 热重载。不引入新协议面，坚果云即可用；冲突语义弱、未知字段有丢失风险。
   - 路线 B（完整，即既有 2C 契约）：`ISyncTransport` + revision 乐观并发 + outbox。语义正确，但要服务端；WebDAV 只能做备份通道做不了它的 cursor/revision 发放者。
   - **建议**：路线 A 作为"备份增强"可独立立项且工作量小；路线 B 仍是终态，二者不冲突——A 的合并器与热重载入口在 B 落地后原样复用。

## 1. 审计：已验证健康的部分

| 面 | 结论 | 位置 |
|---|---|---|
| 传输 | PROPFIND Depth:0/1、MKCOL 逐级（405 视为已存在）、PUT 走 `FileStream` 有 Content-Length（坚果云不接受 chunked）、DELETE 容忍 404 | `WebDavBackupTransport.cs` |
| multistatus 解析 | 仅取 200 propstat；集合自引用按解码后路径对比跳过（兼容坚果云"目录 href 不带尾斜杠"）；文件名只取 href 末段，不信任 displayname；DTD 禁止 | `ParseMultistatus` |
| 凭据 | PasswordVault（非打包可用）；key = `provider:user@scheme://host[:port]/basepath`，换端点/账号不会串密；保存时清理同 provider 陈旧 key | `PasswordVaultCredentialStore.cs`、`CloudBackupSettingsPolicy.CredentialKey` |
| 并发 | `_gate` 串行化；定时路径 `WaitAsync(0)` 跳过重入；pending-restore marker 存在时禁上传；endpoint generation 丢弃过期异步结果 | `CloudBackupService.cs` |
| 命名 | `…-<utc秒>-<device8>.zip` 双设备不互覆；旧格式解析兼容 | `BuildRemoteSnapshotName`/`ParseSnapshotTimestamp` |
| 还原安全 | 下载只接受安全 basename；暂存区校验条目必须落在 manifest 声明域内；pre-restore 快照兜底；重启后 apply 失败保留 marker 下次重试 | `DeskBoxDataBackupService.cs` |

## 2. 审计：遗留问题清单

| 级别 | 问题 | 说明 |
|---|---|---|
| P1 | **数据变化不触发上传** | 上传只由 1 分钟 tick + `OnBackupSettingsChanged` 驱动（App.xaml.cs `RunCloudBackupIfDueAsync`）。改完待办就关机 → 本次编辑不上传，下次开机还要等 interval（默认 1440min）。备份语义下可接受，同步语义下是硬伤。 |
| P1 | **恢复必须重启，且默认整域文件替换** | 内存 owner 各持缓存：`TodoWidgetViewModel.Items`（`InitializeAsync` 一次加载）、`QuickCaptureService._data`（`_gate`+`Changed` 事件）。直接换文件会被下一次 `SaveAsync` 用旧内存覆盖——这是"不能简单 hot-swap 文件"的根因。2026-09-21：确认对话框新增还原方式选择——增量合并（按条目 Id 并集、UpdatedAt 新者胜、远端墓碑不覆盖本机存活条目，绝不删除本机数据）或完全恢复（原快照忠实替换）；选择写入 pending marker 的 `ReplaceItemData` 字段供重启后 apply 使用（极性故意倒置：true=完全恢复替换，false/缺省=增量合并；marker 在用户确认前落盘，缺省必须落在非破坏的合并模式上）。 |
| P2 | **Todo 的 widget-id 归属** | `data/widgets/<widgetId>/todo.json`，widget id 是本机 GUID、两机不同。还原侧已修（2026-09-21）：导出只打包活 widget 的 todo.json；还原按 `widget-style.json` 的源端 widget 清单剔除源机孤儿目录，孤儿与空闲本机 widget 各自按 Ordinal 排序后逐一配对，配对剩余的孤儿合并进同一目标 store（首个配对目标，否则清单内首个本机存活 widget），目标内按 item `Id` 去重追加，无目标时落盘并报 unmapped。同步语义仍需跨设备稳定身份（见 §4.3）。 |
| P2 | **附件不同步、引用悬空** | attachments 被刻意排除（无大小上限）。另一台机器上条目文本正常、`FilePath` 悬空。设计意图如此，需用户可感知。 |
| P2 | **版本门对同步不友好** | 还原通道已降级为警告放行（2026-09-21）：仅 `manifest.SchemaVersion` 是硬门，app 版本偏新不再拒绝——`IsFromNewerAppVersion` 只让两个还原确认对话框警告"更新版本产生的备份可能含本版不识别的数据"。同步通道仍应按 schema 版本容忍（见 §4.4）。 |
| P3 | retention 全局共享 | `RetentionCount` 对目录内所有设备快照合计计数；两机各传时实际保留量减半。 |
| P3 | `EnsureDirectoryAsync` 每次上传逐级 MKCOL | 浪费少量请求，无害。 |
| P3 | 远端列表此前只在手动点刷新时拉取 | 已修：进入云备份页自动刷新一次（2026-09-21）。 |

## 3. Win10/11 兼容性结论

- `TargetPlatformMinVersion=10.0.19044`（Win10 21H2），本功能用到的全部 API 均满足：HttpClient/WebDAV、LINQ-to-XML、source-gen JSON、PasswordVault（双 API 分区，非打包进程可用——已在非打包 AOT 产物上实测通过）。
- 无 Win11 专属 API；两台系统行为无差异。
- **Native AOT 才是真实风险面**，本次联调共修掉三处，均有契约测试守护：
  1. 延迟模板内 `PasswordBox` 等控件经 `FindName` 返回无类型 IInspectable，CsWinRT 反射查型失败 → `InvalidCastException`。修复：`FindCreatedSectionElement<T>` 泛型 + `MarshalInspectable<T>.FromAbi` 静态类型重包装（`SettingsWindow.SectionElements.cs`）。
  2. `{Binding}` 到 `ObservableCollection<T>` 需要 `IObservableVector` CCW 封送，AOT 下静默变空列表。修复：`ItemsSource` 改 code-behind 赋 `object[]`（与 `BackupSnapshotsList` 同模式），item 模板改 `x:Bind`。
  3. item 类型的 `Title`/`Details` 经典绑定缺 `GeneratedBindableCustomProperty` 元数据 → 行渲染空白。修复：属性已加（对 `x:Bind` 不再必需，保留作防御）。

## 4. 免重启同步：方案分析

### 4.1 模型层已预埋

`TodoItem`/`QuickCaptureItem` 均已有 `Id`/`DeviceId`/`UpdatedAt`/`IsDeleted`（墓碑）——注释明示为 sync 预留。item 级 LWW 合并的所有输入都齐了。

### 4.2 路线 A：基于 WebDAV 的轻量热同步

**通道布局**：与备份快照分离，每设备一份稳定文档：
```
DeskBox/sync/<device8>/todo-data.<collectionKey>.json
DeskBox/sync/<device8>/quick-capture.json
DeskBox/sync/<device8>/widget-style.json
```
不覆盖、不竞争；拉取时合并所有 `<device8>` 目录下文档（跳过本机）。

**推（变更驱动）**：`TodoWidgetStore.SaveAsync`/`QuickCaptureService` 保存点 → "domain dirty" 信号 → 防抖 15–30s → 上传本机文档。进程退出时若 dirty 且快速可完成则推一次。

**拉**：复用 1 分钟 tick 降频到每 5–15 分钟 + 启动时 + 窗口前台时；按远端 `getlastmodified`/内容哈希跳过未变更文档。

**合并**：按 `Id` LWW（`UpdatedAt` 大者胜，相等取 `DeviceId` 字典序大者保证确定性）；墓碑参与合并并保留窗口期（如 30 天）再清理，防离线设备复活已删条目。

**热应用（免重启的关键）**：
- `QuickCaptureService`：加 `MergeRemoteDataAsync`——`_gate` 内合并 → `_data` 更新 → `SaveAsync` → 触发既有 `Changed` 事件，UI 自刷。
- `TodoWidgetViewModel`：加 `ApplyMergedDataAsync`——合并后重建 `Items`（复用 `InitializeAsync` 的重建段）。
- **回环抑制**：合并写入须打 sync-applied 标记或比对内容哈希，避免"拉→脏→推→拉"乒乓。
- **Widget 样式**：写的是 `settings.json`/`widget-layout.json` 两个文件，运行时真源在 `SettingsService` 内存。热应用需新增 SettingsService "patch style slice" API + 全量广播。建议 v1 样式仍走重启路径（现有 marker 机制原样保留），只做 todo/随记热同步。

**局限**（与契约 §0-4 一致）：WebDAV 无 revision 发放者，冲突只有 LWW 没有"冲突事实"；无服务端游标，拉取是全文档比对。对个人级 KB 数据够用。

### 4.3 路线 A 的待解设计点

1. **Todo collection 身份**：widget id 跨设备不稳定。两解：
   - (a) 每个 todo widget 增加持久化 `SyncKey`（创建时 GUID），远端文档以 SyncKey 组织，本机存 syncKey→widgetId 映射 —— 正确解；
   - (b) v1 收敛为"单一共享待办列表"：全部合并进本机默认 todo widget —— quick win，多列表语义放弃。
   契约 §3.2 的 `collection_id` 模型与 (a) 同构，选 (a) 则路线 A 与路线 B 的实体模型直接兼容。
2. **未知字段保留**：source-gen JSON 反序列化→合并→再序列化会丢弃高版本新增字段。若允许跨版本同步，合并层须按 `JsonNode`/`JsonObject` 字段级合并，或模型加 `[JsonExtensionData]`。否则高版本设备编辑过的条目被低版本 merge 后丢扩展字段。
3. **版本门**：同步通道只看 `manifest.SchemaVersion`/文档 schema 容忍，不走 `IsBackupFromNewerApp`；还原通道已是警告放行——仅 schema 版本硬门，app 版本偏新经 `IsFromNewerAppVersion` 在还原确认对话框提示，不拒绝。
4. **附件**：v1 继续不同步（悬空引用已知）；v2 可加大小白名单选择性同步。
5. **与 pending-restore 的关系**：手动 scoped restore marker 存在时，同步推/拉都应暂停（避免合并流冲掉待还原状态）。

### 4.4 路线 B：契约既定方案（备忘）

`sync-protocol-contract-20260918.md` 已定稿：envelope/revision/cursor + outbox + 服务端单调 revision。路线 A 不替代它——A 的合并器（LWW+墓碑）、热重载入口、dirty 推送队列在 B 落地时原样复用；A 只是把"备份管道"向前推一步到"够用就好"的同步。

## 5. 建议的立项拆解（若走路线 A）

| PR | 内容 | 依赖 |
|---|---|---|
| A-1 | sync 文档布局 + 推侧 dirty 防抖上传 + 拉侧变更检测（skip-unchanged） | 无；纯 CloudBackupService 扩展 |
| A-2 | QuickCapture 合并器 + `MergeRemoteDataAsync` + Changed 热刷 | A-1；单例 collection 最简单先落地 |
| A-3 | Todo `SyncKey` 身份 + 合并器 + VM 热重载 | A-1；需决策 §4.3-1 |
| A-4 | 未知字段保留策略（JsonNode 合并或 JsonExtensionData）+ 契约测试 | 与 A-2/A-3 并行可 |
| A-5 | （可选）样式域热应用：SettingsService patch API | 可延后/不做，保留重启语义 |

> 验收要点：双设备互传实测（坚果云即可）、断网恢复、合并幂等（重复拉取不产生重复项）、墓碑窗口期、sync-applied 不回环、pending-restore 暂停同步。
