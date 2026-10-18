# DeskBox 后续版本开发规划（最终版）

- 日期：2026-09-19
- 状态：**规划定稿**。不发版窗口期的主线排序与验收门；每个 PR 独立可验可回滚
- 上游文档：`module-boundary-roadmap-20260918.md`（刀序与边界）、`sync-protocol-contract-20260918.md`（2C 契约）、`widget-group-residency-roadmap-20260918.md`（P 系列）、`distribution_channel_workflow.md`（双通道）
- 本轮输入：① 对已落地工作的回归审查；② 同步协议与后端选型的外部方案验证搜索（ObjectBox/Realm/CouchDB 式 outbox 先例、阿里云 OSS 直传文档）

---

## 0. 结论

1. **已落地代码回归干净，两个残留口子已补**（`e6c80ed`）：手动备份路径缺 pending-restore 门、快照列表刷新缺凭证预检。主干健康，3841 测试全绿。
2. **协议契约方向经外部验证成立，四处需要修订**（§3）：时钟漂移下的 LWW 排序、outbox 合并语义、epoch 重置对未推条目的处理、push/pull 顺序。全部是契约级修订，不是推翻。
3. **下一版（1.5.5）不急着发**：已上 main 的云备份链需要实机观察期；发版触发条件见 §4。
4. **主线 = 云同步立项**（2B 设备层迁移 + 2C 引擎 + 服务端 v0），residency P0 归因可插队并行（零文件冲突）。
5. **后端维持原选型**：自建薄 REST + 阿里云 OSS 香港起步；本轮搜索找到一条新证据加固（内地 OSS 数据面 2025-03 起强制自定义域名，香港不受影响，§3.5）。

---

## 1. 现状快照

| 线 | 状态 | 证据 |
|---|---|---|
| 立法步 | ✅ 落地 | `ModuleBoundaryContractTests` 8 测试（含 global-using 守卫） |
| 第 1 刀启动管线 | ✅ 首阶段落地 | `StartupPipeline` + 关键步显式化；僵尸复测/计时待实机 |
| 第 2A 刀 Settings 切片 | ✅ 首阶段落地 | facade + 12 slice + 207 透传；调用点迁移走 ratchet |
| 第 2B 刀 | 2B-1 ✅ 2B-2 ✅ | sync 字段预埋 + FileSafety 首住户；**剩设备层迁移 ~330 处** |
| 云备份 | ✅ 三 PR 已合（#398-400） | WebDAV 传输 + 域 scoped 备份/恢复 + 设置页 |
| 备份 backlog | ✅ 清零 | A-4 ratchet、A-5 恢复预览计数、B-3/B-4 门、回归修复 |
| Platform interop | ✅ #401 已合 | Win32Helper/EverythingNativeMethods/图标叶 P/Invoke 归 Platform |
| 提交署名执法 | ✅ 三层 | `.githooks/commit-msg` + CI 检查 + AGENTS.md |
| 测试基线 | **3841 全绿** | x64 |
| 版本 | 1.5.4 已发（本地），main 领先未发 | |

## 2. 回归审查结果（本轮）

| 项 | 结论 |
|---|---|
| `03b6d21` 计数/门实现 | ✅ 路径假设与 `ValidateScopedRestoreData`/`RebaseManagedAttachmentPathsAsync` 同一遍历口径；`Items`/`RecentItems` 为独立集合不重复计数；墓碑过滤正确；解析失败贡献 0 不炸恢复 |
| ratchet 双层更新 | ✅ 基线计数（11→12 调用）与 8 个 AotStage 二阶字面量（73→74）同步修订，3841 绿 |
| **残留口子 1** | 手动 `RunBackupNowAsync` 缺 pending-restore 门（B-3 只挡了定时路径）→ 已补 `PendingRestore` 结果 + UI 提示（新增 `RestorePending` 键 ×12） |
| **残留口子 2** | 快照列表刷新在"配置了但没存密码"时仍打匿名 401 → 已加 `HasCredentialAsync` 预检，显示"密码未保存" |
| 已知小瑕疵（不修） | `AotStage5B4*` 测试名里的旧数字（"SeventyTwoCalls"）与实际计数漂移——测试名是历史快照，字面量断言才是法。不动 |
| lock file | ✅ 无污染（AOT 后已还原并纳入检查习惯） |

## 3. 方案验证：协议契约四处修订

> 验证方法：对照 ObjectBox Sync、Realm Device Sync、CouchDB 式 replicator、多个生产 outbox 实现逐条比对。**骨架成立**（revision 乐观并发 + epoch 全量替换 + 墓碑 GC 就是 Realm client-reset 的成熟形态；OSS 预签名直传是阿里云官方推荐形态），但四处需要写进契约修订。

### 3.1 时钟漂移下的 auto-LWW —— 需要服务器时钟域

- **问题**：§7 冲突解决用 `UpdatedAt`（客户端墙钟）排序。这是 LWW 教科书事故：快钟设备永远赢——检测没错（`base_revision` 负责），但**解决质量**受漂移支配。冲突虽落盘可查，自动选边可能系统性偏向某台设备。
- **修订（低成本，不改线协议）**：服务端在每个响应携带 `server_time`；客户端维护偏移估计（SkewClock 模式：`offset = server_ms - local_ms`，每次响应刷新，原子字段无锁）；**冲突排序用 corrected UpdatedAt（服务器时钟域）**，展示仍用本地墙钟。离线期间生成的 UpdatedAt 经偏移平移后仍保持真实先后（优于服务端盖到达戳）。
- **为什么不选 HLC**：加一个逻辑计数器分量才能表达因果关系，但本协议有 `base_revision` 兜正确性，HLC 的额外复杂度换不来增量保证。

### 3.2 Outbox 必须按实体 LWW 合并 —— 契约未钉

- **问题**：§6.3 只说"持久化队列"。同一实体推送前被编辑两次 → 第一条推完拿到新 `server_revision`，第二条带着旧 `base_revision` 必然 409——**自我冲突**噪音；delete 后仍有 pending edit 更糟（复活已删实体）。
- **修订**：outbox 以 `(domain, collection_id, entity_id)` 为键做 **last-write-wins 合并**——新写入覆盖同键 pending 条目（保留首个 operation_id 或重新生成均可，但要稳定）；**delete 丢弃该实体全部 pending edits**；`operation_id` 与行 id 分离（行 id 自增保 FIFO，operation_id 供服务端去重）。这是 bulwarkmail/smrt 等生产实现的标准语义。

### 3.3 Epoch 重置必须处理未推 outbox —— 契约未钉

- **问题**：§4 说 epoch 不匹配走 full snapshot replace，没说**本地未推条目**怎么办。Realm client-reset 的成熟答案是分档：recover-unsynced / discard / manual。
- **修订**：epoch 重置顺序 = 拉全量 → 应用 → **幸存 outbox 条目以新 operation_id 重放**（服务端按 §3.2 判冲突走 §7 正常路径，事实落 conflicts 日志）；替换前把将被覆盖的域文件快照进本地备份目录（机制已有，成本一次调用）。`discard-unsynced` 作为严重异常的兜底路径记录。

### 3.4 Push/Pull 周期顺序 —— 契约未钉

- **修订**：每周期固定 **push → pull**（先 drain outbox，再 `PullSince` 取新 cursor）。单实体遇 409 **停排该实体余下 op**（后续 op 假设同一 stale base，连发必连环 409），转入冲突处理后下个周期继续。

### 3.5 后端选型加固证据

- **阿里云 OSS 2025-03-20 新规**：新用户**内地地域 bucket** 的数据面 API 不再走默认外网域名，必须绑自定义域名（+ HTTPS 证书，内地域名 = ICP 备案）。**香港 bucket 不受影响**——"香港起步"从"延迟折中"升级为"合规成本最小"的正解。
- STS 预签名最长 12h、SDK 预签名最长 7 天——blob 直传按需签发即可，无阻塞。
- PIPL 注记：面向大陆用户即适用（与服务器位置无关），v0 只收集 email + 同步数据，隐私政策在上线前备齐即可，不构成立项阻塞。

## 4. 版本规划（不发版期）

### 4.1 1.5.5 候选内容池（已在 main 未发）

云备份三 PR、Platform interop 迁移、备份加固五连（A-4/A-5/B-3/B-4/回归修复）、启动管线首阶段、2A facade、2B-1/2B-2、commit 署名执法。**这批已经是一个完整的 minor 体量**——发版只是时机问题。

### 4.2 发版触发条件（满足即打 1.5.5）

1. WebDAV 云备份实机验证过至少两个真实提供商（Nextcloud + 坚果云）的上传/列表/恢复全流程；
2. 启动管线实机观察无回归（僵尸路径复测 + 启动计时无回退——第 1 刀遗留验收项）；
3. main 累计实跑无新增事故报告。

**不建议**：把 1.5.5 憋着等云同步 v1——账号+服务端+三域同步跨度过大，按节奏应拆成后续独立版本（1.6.x 线）。

### 4.3 版本线划分

| 版本 | 内容 | 状态 |
|---|---|---|
| 1.5.5 | §4.1 内容池（云备份+平台+管线） | 等触发条件 |
| 1.6.0 | 云同步 v1：设备层迁移 + 三域同步引擎 + 服务端上线 + 设置页账户 | 本规划主线 |
| 1.6.x | 同步冲突回看 UI、collection 链接、样式域同步实开 | 后续 |
| 穿插 | residency P 系列（P0 可提前插队） | 并行线 |

## 5. 主线工作分解（云同步立项）

> 排序沿用"事故驱动 > 假设驱动"。PR 分解对齐 sync 契约 §11。

### 5.1 PR-1：2B 设备层迁移（唯一立项理由是同步前置，现在立项成立）

- **范围**：widgets/groups/拓扑/设备态 → 设备域 store（~330 处触面，路线图已标注为最大一刀）；
- **硬约束**：磁盘迁移 fail-closed（沿用 2B-2 历史 store 的领养语义）；`data/sync/` 布局在迁移完成后才可写——契约假设设备域边界已立；
- **验收**：三域归属契约测试 + 迁移/回滚演练 + x64 全绿；
- **注意**：此 PR 可与 residency P1-c（File/QC adapter 化）并行——文件集基本不重叠，但都与 WidgetManager 大文件相邻，**建议排先后不并行**，防大范围合并冲突。

### 5.2 PR-2：sync store + outbox + projection（纯本地，无网络）

- `data/sync/` 布局落地：outbox（含 §3.2 合并语义）、state.json、revisions.json、conflicts/、blobs/；
- 三域投影器：Todo/QuickCapture → envelope；widget-style 复用 `WidgetStyleBackupProjection` 同一代码路径；
- **契约测试即写**：domain 模型反射枚举（除四预埋字段无 sync 字段）、outbox 合并语义、投影纯度（未知 schema_version 存 raw）；
- 零网络依赖，可与 PR-1 并行开发。

### 5.3 PR-3：服务端 v0 + ISyncTransport + IAuthSession + 引擎循环

- **服务端**（阿里云香港 ECS，ASP.NET Core，SQLite 起步）：
  - 表：`users`、`sync_records(account,domain,collection,entity,revision,envelope,updated)`、`blobs(account,blob_id,size,ref)`、`devices(account,device_id,cursor,last_seen)`；
  - API：auth(email+password+refresh)、envelope push/pull（per-envelope verdict）、blob 预签名直传、epoch/序列发放、每响应带 `server_time`（§3.1）；
  - 墓碑保留 ≥30 天且所有注册设备游标越过（§4 不变量）；
- **客户端**：`ISyncTransport` 官方云实现 + `IAuthSession`（401→refresh 一次→失效事件，B-4 教训复用）+ 引擎循环（push→pull、三域独立队列/游标、指数退避、409→冲突日志）；
- **验收契约**（sync 文档 §10 六条全部）：冲突注入、epoch 重置、幂等、outbox 断电续推、半态恢复、热路径隔离。

### 5.4 PR-4：设置 UI

账户登录/登出、域开关、同步状态/冲突呈现、`IFeatureRuntime` 租约（Sync 是持长生命周期资源的模块——disable=release 契约首次实装）。

### 5.5 明确不做（v1）

CRDT/实时推送/字段级合并/通用偏好同步/布局拓扑同步/collection 链接 UX/多用户协作——契约 §9 已定。

## 6. 并行线与触发线

| 线 | 何时做 | 冲突面 |
|---|---|---|
| residency **P0 归因** | **随时可插队**（纯测量，零行为变化） | 无；建议先做——其结果决定 P3 是否值得 |
| residency P1-b/c | PR-1 之后（避开 WidgetManager 冲突面） | 中（同批大文件） |
| residency P2/P3 | P0 归因 ≥10% 才全线 | — |
| 第 3 刀 contribution descriptor | 新格子立项时 | — |
| 图像出模型 | 边际斜率实测超预算或用户反馈 | — |
| AOT publish 后 lock file 还原 | 每次 AOT 后 | 流程纪律（已有教训） |

## 7. 验收门（每条线统一）

1. `dotnet test -p:Platform=x64` 全绿（当前基线 3841）；
2. Debug 构建 0 错误 + canonical 路径重启实例；
3. 碰序列化/边界时同步更新 ratchet 双层（基线计数 + AotStage 字面量）；
4. 碰 `DESKBOX_NATIVE_AOT` 相关文件 → `scripts/publish-aot-audit.ps1` 编译验证 + **还原 packages.lock.json**；
5. 提交零署名 trailer（hook+CI 已执法）；
6. 新协议/边界 → 契约测试先行（立法后搬家）。

## 8. 风险登记

| # | 风险 | 对策 |
|---|---|---|
| R1 | 服务端运维变成时间黑洞 | 单机 SQLite 起步；备份=OSS 每日快照；无多副本承诺 |
| R2 | 时钟漂移系统性错选冲突边 | §3.1 服务器时钟域偏移修正（响应带 server_time） |
| R3 | outbox 未合并 → 自我 409 风暴 | §3.2 契约级 LWW 合并 + delete 丢弃 pending |
| R4 | epoch 重置吞本地未推数据 | §3.3 recover-unsynced + 替换前本地快照 |
| R5 | 设备层迁移触面大、回归藏得深 | fail-closed 领养语义复用；迁移/回滚演练写进验收 |
| R6 | File adapter 化与设备层迁移撞文件 | §5.1 排先后不并行 |
| R7 | 中国合规（PIPL/备案） | HK 免备案；隐私政策上线前备齐；只收集 email+同步数据 |
| R8 | 同步引擎拖累 settings 热路径 | 契约已禁挂防抖路径；热路径隔离验收项 |
