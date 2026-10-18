# DeskBox 云同步协议契约（第 2C 刀前置）—— 契约文档，非实现

- 日期：2026-09-18
- 状态：**契约定稿，未立项实现**。后端选型（自建薄 REST + OSS / 其他）是本契约的填空项，不是前提——契约按"任何满足协议的后端"写。
- 上游：`module-boundary-roadmap-20260918.md` §4 第 2C 刀、§10 云备份立项（PR-1/2/3 已合并 #398/#399/#400）
- 边界：本文只钉**协议与客户端侧契约**（envelope/revision/cursor/三个接口）；服务端 API 形状、存储引擎、部署区域均不在本文射程。

## 0. 结论

1. **同步是投影，不是污染**：`Local 记录 → SyncEnvelope` 经 projection 层上行，云协议不进入本地 domain model。domain 记录只携带身份 + 轻量 provenance（`Id`/`DeviceId`/`UpdatedAt`/`IsDeleted`，已在 2B-1 预埋，有独立本地用途）；协议状态（`server_revision`/`base_revision`/`operation_id`/cursor/epoch）**永远不进 domain**，放 sync store 按 entity_id 关联。
2. **最小协议面**：`entity_id`/`server_revision`（服务端单调发放）/`base_revision`/`device_id`/`operation_id`/`deleted`——乐观并发检**真冲突**。UX 策略可选自动 LWW，但"发生了冲突"是已知事实，不是两台设备互猜时钟。
3. **KB 级数据不需要 CRDT**：revision 乐观并发 + 墓碑 + cursor/epoch 快照替换已足够；实时推送、协同编辑不在 v1。
4. **备份 ≠ 同步，两者不互相替代**：云备份（已落地）是单向快照+按需还原，域级整体覆盖；同步是逐实体双向合并。WebDAV 传输继续只做备份通道；官方云是 `ISyncTransport` 第一个实现。
5. **同步引擎是普通模块**：落地即 `Features/Sync`，独立队列、事件驱动，**绝不挂进 settings 防抖保存的热路径**（路线图 §9 硬约束）。

## 1. 与云备份的边界（已落地部分的复用与不复用）

| 维度 | 云备份（已合并） | 云同步（本契约） |
|---|---|---|
| 粒度 | 域级 ZIP 快照，整域覆盖 | 逐实体 envelope，按 revision 合并 |
| 方向 | 单向（上传/还原） | 双向（push/pull） |
| 触发 | 手动 + 定时间隔 | 事件驱动（本地变更入队） |
| 冲突 | 无概念（快照即真相） | revision 乐观并发 + 冲突事实 |
| 传输 | `ICloudBackupTransport`（WebDAV 实现已落地） | `ISyncTransport`（官方云首实现） |
| 凭证 | `ICredentialStore`（PasswordVault），provider 作用域 key | 同一 `ICredentialStore`，auth 作用域 key（§7） |
| 域划分 | `CloudBackupDomain` 三域 | **同一三域语义**，同步按 collection 细分（§3.2） |
| 远端身份 | 文件名 device8 后缀 | `device_id` 全程参与（归因+游标） |

**复用**：域白名单（路径规则）、样式投影白名单（`WidgetStyleBackupProjection` 的 key 集合即"样式域"定义，两端共用同一份——A-4 ratchet 是它的一致性保险）、`DeviceIdentity`、`ICredentialStore`、basename/路径安全约定。
**不复用**：ZIP 快照格式、远端文件布局、retention 语义、scoped restore 管线（含 todo remap——那是还原语义，同步有自己的 collection 模型，§3.2）。

## 2. 同步域与实体模型

### 2.1 三个可同步域（与备份域同边界）

| 域 | 内容 | collection 模型 |
|---|---|---|
| `todo-data` | `widgets/<id>/todo.json` 条目 + `attachments/` 引用 | **每 widget 一个 collection**，`collection_id` = widget id |
| `quick-capture-data` | `quick-capture/quick-capture.json` 条目 + `attachments/` 引用 | **单例 collection**：`collection_id` 固定 `"quick-capture"` |
| `widget-style` | `WidgetStyleBackupProjection` 白名单键（shell 全局 + 逐 widget 样式补丁） | **单例 collection**：`collection_id` 固定 `"widget-style"`；shell 键 = 实体 `"shell"`，逐 widget 补丁 = 实体 `<widget-id>` |

### 2.2 实体身份三元组

```
(domain, collection_id, entity_id)
```

- `entity_id` = domain 记录的 `Id`（GUID-N，已预埋）；widget-style 域为 `"shell"` 或 widget id。
- `collection_id` 是**同步作用域分组键**：同账户下同三元组的实体才互相合并。
- **v1 显式限制**：两台设备各自独立创建的待办格子 = 不同 collection，**不合并**（安全、可预期；防"两个 Work 清单被 GUID 字典序乱配"）。跨设备 collection 链接/绑定 UI 归 v2——那是产品决策不是协议缺陷，协议面已支持。
- `quick-capture` 与 `widget-style` 是单例 collection，跨设备天然合并——v1 的同步价值主要由它们 + 同 collection 的 todo（如恢复 remap 后同 id 延续）兑现。

### 2.3 domain 字段 vs sync store 字段（不可越界）

| domain 记录携带（已存在） | sync store 携带（协议态，data/sync/） |
|---|---|
| `Id`（entity_id） | `server_revision`（服务端单调发放） |
| `DeviceId`（最后写入者） | `base_revision`（本地上次确认的服务端 revision） |
| `UpdatedAt`（墙钟，仅供展示/LWW 参考） | `operation_id`（客户端幂等键） |
| `IsDeleted`（墓碑槽位，v1 软删启用点） | `cursor`（按域的拉取游标）、`epoch`、附件 blob 引用计数 |

**契约测试（立项即写）**：domain 模型反射枚举，除四个预埋字段外出现任何 sync 字段即红——防"图省事把 server_revision 塞进 TodoItem"。

## 3. 线协议：SyncEnvelope 与 revision 语义

### 3.1 SyncEnvelope（上行/下行同形）

```jsonc
{
  "domain": "todo-data",
  "collection_id": "<widget-id|quick-capture|widget-style>",
  "entity_id": "<guid-N|shell>",
  "schema_version": 1,
  "deleted": false,
  "device_id": "<写入设备>",
  "operation_id": "<客户端 UUID，幂等键>",
  "payload": { /* 域记录的序列化 JSON，见 §3.3 */ },
  "attachments": [
    { "name": "attachments/a.pdf", "blob_id": "<content-hash>", "size": 12345 }
  ]
}
```

- **envelope 是 wire 格式不是存储格式**：服务端存什么引擎是实现细节，但语义上等价于"按 (account, domain, collection_id, entity_id) 主键的最新 envelope + 单调 server_revision"。
- `schema_version` 逐域独立演进；低版本客户端收到高版本 envelope → **存 raw 跳过投影**（不解析未知字段，向前兼容）；高版本字段丢失风险记录在案。
- `operation_id`：同一 operation_id 重推必须幂等（网络重试不产生第二实体）；服务端按 (account, operation_id) 去重窗口 ≥ 7 天。

### 3.2 revision / 乐观并发

- `server_revision`：服务端**按账户单调递增**发放（不是按实体——游标语义需要全序）。任何实体写入/墓碑化消耗一个 revision。
- 上行：客户端携带 `base_revision`（它最后见到的该实体 server_revision；新实体 = 0）。
  - 服务端 `base_revision == 当前 server_revision` → 接受，发放新 revision。
  - 不等 → **409 Conflict**，响应带回当前服务端 envelope。冲突是事实事件：写入 `sync/conflicts` 日志供 UI 呈现，**不是异常吞掉**。
- 下行：按域游标拉取——`PullSince(domain, cursor) → envelopes[] + new_cursor`，envelope 按 server_revision 升序。
- **客户端落地顺序**：附件 blob 先取（§5），envelope 投影进 domain 后，sync store 更新 `server_revision`/`base_revision`/cursor——同一事务或 WAL 化，防"投影落了游标没进"的半态（FileSafety 的 #393 时序纪律在此复用）。

### 3.3 payload 投影规则

- Todo/QuickCapture：域记录 JSON 原样（`Id`/`DeviceId`/`UpdatedAt`/`IsDeleted` 随行）；`attachments` 在 envelope 层表达，payload 内文件路径**只存相对段**（`attachments/a.pdf`），不含本机绝对路径/widget 目录前缀——路径主语是 collection。
- widget-style：payload = `WidgetStyleBackupProjection` 白名单子集。shell 实体 = 全部 shell 键；`<widget-id>` 实体 = 该 widget 的样式补丁。布局/位置/拓扑/胶囊排序永远不在白名单（与备份共享同一份清单——**投影一致性由同一代码路径保证，不是两份清单对齐**）。

## 4. cursor / epoch / 墓碑 GC

- `cursor`：服务端按 (account, domain) 发放的 opaque 游标（实现可映射 server_revision 下界）。客户端逐域持久化。
- `epoch`：服务端按账户发放的纪元号。**任何纪元破坏事件（服务端重置、墓碑 GC 越过客户端游标、协议重大升级）→ epoch++**；客户端 PullSince 收到 epoch 不匹配 → 放弃增量，走 full snapshot replace（拉全量→本地 diff→按三元组覆盖/墓碑化本地多余实体）。
- **墓碑保留窗**：`deleted=true` 的 envelope 服务端保留 ≥ 30 天且直到"所有已注册设备的游标都越过它"；两者取大。超窗硬删 + epoch 评估。
- **防复活不变量**：设备离线超过墓碑窗 → epoch 机制强制全量替换，不无限 replay——已删实体不得因旧设备上线复活。
- 客户端侧同构：本地墓碑在"服务端确认 + 超窗"后才物理删除（v1 软删启用点=此规则上线时）。

## 5. 附件 blob

- `blob_id` = 内容寻址（SHA-256 of bytes）——去重、幂等、校验三合一。
- **`name` = 集合相对路径**，与 payload 中引用的路径逐字一致（`attachments/report.pdf` / `images/photo.png`），不是裸 basename——同一记录里同名 basename 的主图片与附件也能无歧义地还原到各自 payload 路径；下行按 name 即知 blob 落地位置。
- **顺序不变量**：上行先 PUT blob 再推引用它的 envelope（服务端可拒 dangling 引用）；下行先取 blob 再投影 envelope（附件未落地的实体不得对 UI 可见为"有附件"——按附件缺失降级显示）。
- 传输在 `ISyncTransport` 内是独立子面（`UploadBlob/DownloadBlob`），官方云实现走 OSS 预签名直传；payload 与 blob 分离使 envelope 永远 KB 级。
- GC：blob 仅被 live envelope 引用时存活；实体墓碑化→其 blob 进入 GC 候选（服务端窗同 §4）。

## 6. 三个接口契约（客户端侧）

### 6.1 `ISyncTransport`（换后端 = 换实现）

```csharp
Task<SyncEpoch> GetEpochAsync(CancellationToken ct);
Task<PushResult> PushAsync(IReadOnlyList<SyncEnvelope> envelopes, CancellationToken ct);
// PushResult: per-envelope { accepted | conflict(server envelope) | rejected(reason) }
Task<PullPage> PullSinceAsync(string domain, string cursor, int limit, CancellationToken ct);
// PullPage: envelopes[] + nextCursor + epoch + hasMore
Task UploadBlobAsync(string blobId, Stream content, CancellationToken ct);
Task DownloadBlobAsync(string blobId, Stream destination, CancellationToken ct);
Task<SyncSnapshot> GetSnapshotAsync(string domain, CancellationToken ct); // epoch 恢复用
```

- 语义要求：Push 批量内顺序无关（服务端逐 envelope 判 revision）；Pull 严格升序；接口不承诺实时性。
- WebDAV 理论上可实现一版（envelope 文件+索引），但**无服务端单调 revision 发放者**，退化不了乐观并发——v1 不做。WebDAV 传输维持备份职责。

### 6.2 `IAuthSession`

```csharp
Task<AuthTokens> SignInAsync(AuthCredential credential, CancellationToken ct);
Task<AuthTokens> RefreshAsync(CancellationToken ct);           // 静默续期
Task SignOutAsync(CancellationToken ct);                       // 吊销 + 本地清理
bool IsAuthenticated { get; }
string? AccountId { get; }                                     // 服务端 user_id
```

- token 对（access 短寿 + refresh 长寿）**只进 `ICredentialStore`**，key 形如 `deskblox-cloud:auth:<account_id>`；settings.json 只存"是否启用官方云 + account 显示名"，token 一个字符不落盘（沿用 §10 凭证纪律）。
- 401 → 自动 Refresh 一次再重放；refresh 也 401 → 会话失效事件给 UI（不静默循环——云备份 B-4 backlog 的同款教训）。

### 6.3 同步引擎（`Features/Sync`，普通模块）

- **outbox**：本地变更 → `data/sync/outbox` 持久化队列（设备域数据，不进备份不同步自身）→ 事件驱动推送。崩溃恢复 = 队列继续，不丢待推。
- 三域独立队列独立游标——一个域失败不阻塞其他域。
- 退避：网络/5xx 指数退避；401 走 §6.2；409 进冲突日志后按域策略处理（§7）。
- **绝不挂进 settings 防抖保存热路径**——store 写事件经独立队列异步消费，UI 写路径无网络等待。

## 7. 冲突策略（v1 默认值，可逐域演进）

- **检测**：仅 `base_revision` 不等判定——两台设备离线改同条 = 真冲突，无歧义。
- **默认解决（v1）**：auto-LWW——`UpdatedAt` 大者胜，并列取 `device_id` 字典序大者（确定性，不靠本机时钟当真相：UpdatedAt 只是排序键不是正确性依据，base_revision 才是）。
- **事实记录**：每次冲突写 `data/sync/conflicts`（device 域，不进备份）——v2 可做"冲突回看/手动选边"UI，数据已就位。
- widget-style 域整实体粒度合并（不逐键 merge——样式补丁是原子偏好集，逐键合会拼出四不像）。
- 附件引用冲突随实体冲突解决，blob 层无独立冲突概念。

## 8. 客户端存储布局（data/sync/，设备域，永不进备份/同步自身）

```
data/sync/
  outbox/                # 待推 envelope 队列（WAL 化）
  state.json             # 逐域 cursor + epoch + 账户摘要
  revisions.json         # entity_id → {server_revision, base_revision, operation_id}
  conflicts/             # 冲突事实记录（§7）
  blobs/                 # 下行 blob 暂存（投影前）
```

- 与恢复管线关系：scoped restore 覆盖三域数据文件但**不覆盖 data/sync/**——恢复后 sync store 的 revision 视图与新数据不符 → 下次同步按 base_revision 自然产生冲突或 epoch 全量替换收敛，语义诚实（恢复=用户明示的本地覆盖意图）。

## 9. v1 明确不做

- CRDT / 操作变换 / 字段级合并
- 实时推送（WebSocket/subscription）——v1 轮询+事件触发足够
- 通用偏好同步（语言/主题等非样式设置）、widget 布局/拓扑同步、collection 链接 UX
- 共享/协作（多用户同账户数据可见性由 auth 边界自然限定）
- 服务端实现细节（选型另文；推荐形态：薄 REST + PostgreSQL/SQLite + OSS 预签名 blob，香港起步）

## 10. 验收契约（立项即测试）

1. **冲突注入**：双客户端离线改同实体 → 第二推必收 409 + 当前服务端 envelope + 冲突日志落盘。
2. **epoch 重置**：服务端 epoch++ → 客户端放弃增量走 full replace；旧墓碑不复活。
3. **幂等**：同 operation_id 重推 → 单实体单 revision。
4. **outbox 持久化**：推送中途杀进程 → 重启队列继续，无丢推无重推副作用。
5. **投影纯度**：domain 模型反射枚举无第五 sync 字段；未知 schema_version envelope 存 raw 不解析不崩。
6. **半态恢复**：blob 到位/envelope 未投影/cursor 未进三阶段任意断电 → 重启后一致收敛。
7. **热路径隔离**：settings 防抖保存路径无 sync 代码引用（boundary 契约测试钉死）。

## 11. 与立项的衔接

- **绑定的前置**：2B 设备层迁移（widgets/groups/拓扑 → 设备域 store）——`data/sync/` 布局假设设备域边界已立；两者同立项执行（路线图 §9）。
- **PR 分解建议**（对齐云备份节奏）：PR-1 outbox+state store+projection（纯本地，无网络）；PR-2 ISyncTransport+IAuthSession+引擎循环；PR-3 设置 UI（账户/域开关/冲突呈现）；PR-4+ collection 链接/官方云付费层。
- **服务端最小实现**：两张表（`sync_records(account, domain, collection, entity, revision, envelope, updated)`、`blobs(account, blob_id, size, ref)`）+ epoch/序列发放 + OSS——立项时再定选型，本契约不假设。
