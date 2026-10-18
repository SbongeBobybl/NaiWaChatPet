# DeskBox SearchCore 原生 ABI v3

- 日期：2026-08-23
- 模块：`deskbox_search_core.dll`
- 目标：`x86_64-pc-windows-msvc`
- ABI：3
- 结构版本：1
- 产品状态：Direct x64 原生模块构建默认启用；运行期自动回退；Store 与 ARM64 不打包且默认关闭

## 1. v3 增量

ABI v3 保留 v2 的 DBIX v1 直载、构建、查询、复制、统计、取消和销毁操作，新增三个导出：

- `deskbox_search_core_mutate_batch_v1`：事务化 upsert、精确删除、目录树删除和按扫描代次清理；
- `deskbox_search_core_project_v1`：recent files 与 frequent folders 有界投影；
- `deskbox_search_core_save_dbix_v1`：把当前 live entries 原子保存为 DBIX v1。

当前 SearchCore 必需导出为 14 个。它仍是独立模块，不改变生产 `deskbox_native.dll` 的 ABI 2、
能力 511 和十个导出。

## 2. 增量 mutation 契约

一次调用最多接收 8,192 个 mutation。所有 UTF-16 数据由调用方拥有，只在调用期间借用。四种操作为：

1. `UPSERT`：以 ordinal-ignore-case 的完整路径替换现有 live entry，并写入目录、文件名、类型、
   UTC ticks、原始 `DateTime.ToBinary()` 和扫描代次；
2. `REMOVE_EXACT`：删除路径完全相同的 live entry；
3. `REMOVE_TREE`：删除目标本身及其路径分隔符边界内的所有后代；
4. `REMOVE_STALE_TREE`：只删除目标树内扫描代次不等于给定代次的 entry。

同一批内不允许两个大小写不敏感等价的 upsert。Rust 在改变 live/tombstone 状态前完成结构、范围、
时间、容量、目标集合和内存预留验证；失败时不公开部分 entry 变化。成功后返回 applied mutation 数、
live entry 数、tombstone 数和目录数。

更新采用追加新 entry、旧 entry 标记 tombstone 的方式，避免移动后续 entry ID。查询、复制和投影都
忽略 tombstone。持久化再清除 tombstone 和无人使用的目录，因此运行期稳定 ID 与落盘压缩可以同时满足。

## 3. 路径与时间语义

- 路径相等、目录复用和树边界均使用与现有托管索引对照过的 ordinal-ignore-case 规则；
- 树删除只接受相同路径或 `\\`、`/` 分隔符后的后代，不能把同前缀的相邻目录误删；
- upsert 的 UTC ticks 必须与 `modified_binary` 解码结果一致；
- Local/UTC 和既有 `DateTime.MinValue` sentinel 延续 ABI v2 语义；
- 托管层会先把非 MinValue 的 `DateTimeKind.Unspecified` 规范为 Local，再调用 `ToBinary()`，避免旧
  DBIX 升级时因缺少 kind 而进入无意义的重复 fallback。

## 4. 原生投影

产品桥把单次结果限制为 200：

- recent files 排除目录，按修改时间降序，再按稳定 entry ID 排序；
- frequent folders 只统计 live 文件，按目录内文件数降序、最新修改时间降序、目录 ID 升序排序；
- 第一次调用若 UTF-16 输出缓冲不足，Rust 返回精确所需字符数，调用方扩容后重试；
- Rust 不保留输出缓冲，也不把内部 arena 指针暴露给 C#。

投影是当前 resident snapshot 的只读结果。产品在查询、投影或保存前先提交已积累的扫描 mutation batch。

## 5. DBIX 保存与压缩

保存只序列化 live entries，并只写这些 entry 实际引用的目录。旧目录 ID 在写入时生成紧凑 remap，
文件内容保持现有 DBIX v1 格式。写入顺序为：

1. 写调用方指定的 `.tmp`；
2. flush 并 `sync_all`；
3. 再次检查取消 event；
4. 用 `MoveFileExW(REPLACE_EXISTING | WRITE_THROUGH)` 原子替换目标。

任何失败都会尝试删除临时文件并保留原目标。产品保存后若 tombstone 达到 `max(4096, live/8)`，会从
刚写出的 DBIX 重新打开一个紧凑句柄，再销毁旧句柄。这个过程只在同一写锁内交换 owner，不会并存
两份长期 resident 索引。

## 6. 所有权、并发与取消

- 句柄只由创建它的 DLL 销毁一次；
- C# `SearchIndexService` 串行化 load/mutate/query/project/save/destroy；
- query 的原子 cancel flag 与 load/save 的 Windows waitable event 继续保持各自边界；
- mutation 不保存调用方地址，save 不保存 event handle；
- panic policy 为 abort，Rust allocator 指针、集合、字符串和异常都不跨 ABI。

一次 DeskBox 会话只允许一个 resident owner：Rust 活跃时托管 `_index` 与目录池必须为空；Rust 加载
或 ABI 校验失败时先销毁任何原生句柄，再加载可用的托管 DBIX。失败不能被呈现为“后端成功但结果为空”。

## 7. 产品启用与发布边界

- Direct x64/ARM64 Debug/Release 和受审计 Native AOT 构建包含对应架构的 SearchCore DLL；
- `DESKBOX_SEARCH_CORE_DEFAULT` 在 Direct、SearchCore 模块随构建存在时对 x64/ARM64 定义；该构建中
  `SearchRustIndexerPreviewEnabled` 默认值为 `true`，Store 中为 `false`；
- 已经落盘的显式用户选择优先于新的构建默认值，因此升级不会覆盖用户保存的 `false`；
- DLL 缺失、架构、ABI、导出、DBIX 版本或时间语义失败会显示 managed fallback 原因；query、
  projection、save、idle unload、upsert、精确删除、树删除和扫描 reconciliation 的可恢复运行期
  异常会销毁 Rust owner、会话内隔离原生重试并恢复 managed snapshot；
- 只有用户显式重新配置搜索后端时才清除会话隔离并重试 Rust，避免故障循环；
- ARM64 已通过 GitHub 托管原生 ARM64 ABI、Unicode 查询和产品绑定门禁；Store 仍排除本模块，等待
  7C1 单独验证 MSIX 内容、框架依赖和升级策略；
- 普通 `deskbox_native.dll` 的选择策略与 ABI 完全不受 SearchCore 影响。

## 8. 阶段 6D 可靠性与默认决策

阶段 6D 已补齐：

- 运行期所有原生操作的确定性故障注入、managed owner 恢复和会话级隔离；
- 真实 `FileSystemWatcher` create/rename/tree-delete、模拟
  `InternalBufferOverflowException` 与 root reconciliation；
- 64 个 live 文件、65 轮更新、超过 4,096 个 tombstone、精确删除、重命名、树删除、保存压缩、
  owner 替换和 idle unload/reload 的 exact live-set soak；
- 真实 x64 Native AOT SearchCore 搜索结果、6 次筛选和 8 次排序控件转换；
- 208,021 条真实 DBIX、11 个启用 Widget、managed/Rust 各三次整进程测量。Rust 的 Private Bytes
  中位数为 235.08 MiB，对照为 269.73 MiB，下降 12.85%；Working Set 为 351.67 MiB，对照为
  388.00 MiB，下降 9.36%。

因此 Direct x64 默认启用门禁已经通过。ABI v3 本身不扩大到 WinUI、watcher 或业务状态机；这些继续
由 C# 管理，Rust 只持有适合紧凑化的 resident 搜索数据和粗粒度操作。

仍未封闭的是 ARM64 Rust target/PE/真实设备、Store MSIX 打包与升级验证，以及目标机器的长期产品
遥测。完整结果见 `rust-stage-6d-search-core-report.md`。

这些项目进入阶段 6D。ABI v3 当前允许显式预览，不等于已经满足默认启用或发布门槛。
