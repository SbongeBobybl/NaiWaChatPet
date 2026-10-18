# DeskBox 回收站精确恢复 Rust 原生边界与 ABI v1

- 日期：2026-08-22
- 阶段：5B-4C1B1
- 产品删除入口：`FileItemMenuBuilder` → `FileSurfaceContent.DeleteItemsAsync` → `WidgetViewModel.DeleteItemsAsync` → `FileService.DeleteEntryAsync`
- 原生入口：`RecycleBinNativeBackend`
- 范围：按原父目录和项目名查询、只在唯一匹配时恢复；不负责产品删除、清空回收站、Properties、Shell 进度、Picker 或拖放

## 1. 实现选择

产品删除继续使用既有窄 `SHFileOperationW` P/Invoke。该路径数据量小、没有托管内存热点，也不需要为 AOT 重建 COM 接口，因此本阶段没有改写它。

跨进程验证和失败补偿必须准确找到本轮删除的项目。若在 C# 中实现，需要重新建立 `Shell.Application`、`Folder`、`FolderItems`、`FolderItem`、`FolderItem2`、`VARIANT` 和继承关系。本边界改为一个完整、同步、粗粒度 Rust 操作，使用 `windows` crate 的强类型 Shell 投影，并只把最终状态、阶段 HRESULT 和计数返回托管层。

该导出是内部恢复能力，不是通用回收站管理 API。它不解析私有 `$I`/`$R` 文件，不读取或写入 `$Recycle.Bin` 目录，不清空回收站，也不处理不匹配项目。

## 2. 精确身份与操作语义

输入身份由两部分组成：

1. `original_parent`：删除前项目的完整父目录；
2. `original_name`：删除前项目名。

Rust 打开 Shell namespace CSIDL 10，完整枚举回收站项目。名称通过 `FolderItem.Name` 按 ordinal ignore-case 比较；来源目录通过 `FolderItem2.ExtendedProperty("System.Recycle.DeletedFrom")` 获取，双方先经 `GetFullPathNameW` 规范化，再忽略大小写和末尾目录分隔符比较。

| 操作 | 行为 |
| --- | --- |
| `QUERY` | 完整枚举并返回 `matched_count`；0、1 或大于 1 均可作为成功查询结果 |
| `RESTORE` | 完整枚举结束后要求 `matched_count == 1`，才对保留的唯一 `FolderItem` 调用 `InvokeVerb("undelete")` |

恢复不会在看到第一个匹配项时提前执行。0 个匹配返回 file-not-found HRESULT；多个匹配返回 `E_UNEXPECTED`，两种情况都不调用恢复 verb。项目名称读取失败，或同名候选项的来源属性读取、类型转换或路径规范化失败，也会保守地让整个操作失败，不能退化为“0 个匹配”。

5B-4C1B1 runner 每轮生成 32 位小写十六进制 run ID，并把它写入三个 owned 项目名。调用恢复前仍先执行查询并要求恰好一个匹配；原生恢复操作自身再次完整枚举并执行同样的唯一性门禁。

## 3. COM 与资源生命周期

每次导出调用同步完成：

- 以 STA 请求初始化 COM；`S_OK`/`S_FALSE` 由本次调用配对 `CoUninitialize`；
- 遇到 `RPC_E_CHANGED_MODE` 时复用调用线程已有 apartment，不撤销调用方初始化；
- `IShellDispatch`、Folder、集合、项目、BSTR 和 VARIANT 全由 Rust RAII 管理；
- 不跨调用、线程或 ABI 保存 COM 指针。

## 4. 模块版本与能力

- 模块 ABI：`2`；
- 结构版本：`1`；
- 能力位：`DESKBOX_NATIVE_CAPABILITY_RECYCLE_BIN_V1 = 1 << 8`；
- 完整能力掩码：`511`；
- 新导出：`deskbox_recycle_bin_v1`；
- 当前发布必需导出：10 个。

```c
uint32_t deskbox_recycle_bin_v1(
    const DeskBoxRecycleBinRequestV1* request,
    DeskBoxRecycleBinResultV1* result);
```

托管加载器固定从 `AppContext.BaseDirectory/deskbox_native.dll` 获取现有受审计模块，先校验 ABI 和能力位，再动态解析操作导出。Native AOT 路径失败时不回退到传统 C# COM。

## 5. 输入与托管门禁

操作值为 1 `QUERY`、2 `RESTORE`。两个字符串均为显式长度 UTF-16 slice，非空、最多 32,767 个 code unit、不允许嵌入 NUL。flags 和所有 reserved 字段必须为 0。

托管层额外要求 `original_name` 不是完整路径且不含目录分隔符。AOT fixture 还要求精确场景、四个允许 phase 之一、32 位小写十六进制 run ID，以及位于隔离 preview 数据根内的 owned 目录。

## 6. x64 结构布局

请求固定为 80 字节：

| 偏移 | 字段 | 类型/大小 |
| ---: | --- | --- |
| 0 | `struct_size` | `uint32_t`，必须为 80 |
| 4 | `struct_version` | `uint32_t`，必须为 1 |
| 8 | `operation` | `uint32_t` |
| 12 | `flags` | `uint32_t`，必须为 0 |
| 16 | `original_parent` | UTF-16 slice，16 字节 |
| 32 | `original_name` | UTF-16 slice，16 字节 |
| 48 | `reserved[4]` | 32 字节，必须全为 0 |

结果固定为 104 字节：

| 偏移 | 字段 | 类型/大小 | 含义 |
| ---: | --- | --- | --- |
| 0 | `struct_size` | `uint32_t` | 必须保持 104 |
| 4 | `struct_version` | `uint32_t` | 必须保持 1 |
| 8 | `status` | `uint32_t` | 与函数返回值一致 |
| 12 | `operation_hresult` | `int32_t` | 最终 HRESULT |
| 16 | `attempted_phases` | `uint32_t` | 已进入阶段位集合 |
| 20 | `com_hresult` | `int32_t` | COM 初始化 |
| 24 | `create_hresult` | `int32_t` | Shell 对象创建 |
| 28 | `namespace_hresult` | `int32_t` | 回收站 namespace |
| 32 | `items_hresult` | `int32_t` | Items 集合 |
| 36 | `enumerate_hresult` | `int32_t` | Count/Item 完整枚举 |
| 40 | `item_name_hresult` | `int32_t` | 项目名读取 |
| 44 | `property_hresult` | `int32_t` | DeletedFrom/路径规范化 |
| 48 | `invoke_hresult` | `int32_t` | undelete verb |
| 52 | `matched_count` | `uint32_t` | 完整枚举后的精确匹配数 |
| 56 | `restored_count` | `uint32_t` | 只能为 0 或 1 |
| 60 | `operation_succeeded` | `uint32_t` | 只能为 0/1，并与成功状态一致 |
| 64 | `reserved0` | `uint32_t` | 必须为 0 |
| 68 | `reserved1` | `uint32_t` | 必须为 0 |
| 72 | `reserved[4]` | 32 字节 | 必须全为 0 |

Rust、C 头文件和托管结构均固定尺寸。托管层还拒绝未知阶段位、非法布尔值、reserved 非零、返回状态不一致、`restored_count > matched_count`、查询产生恢复计数，以及成功恢复不满足 1/1 的结果。

## 7. 阶段诊断

| 位 | 阶段 |
| ---: | --- |
| `1 << 0` | COM 初始化 |
| `1 << 1` | 创建 Shell 对象 |
| `1 << 2` | 打开回收站 namespace |
| `1 << 3` | 获取 Items 集合 |
| `1 << 4` | Count/Item 枚举 |
| `1 << 5` | 项目名读取 |
| `1 << 6` | DeletedFrom 与路径比较 |
| `1 << 7` | 调用 undelete |

## 8. 验证边界

静态契约覆盖 Rust/C/托管结构、能力、导出、输入与结果防御、完整枚举后唯一恢复、真实产品菜单调用链、延期范围和 runner 补偿。Rust 单元测试覆盖无效操作、空值/嵌入 NUL 和不兼容结果 envelope。

实际 AOT 矩阵只操作本轮生成的三个 owned 项目。它从真实 File Widget 单选和多选菜单进入产品删除，新进程查询三项各恰好一个匹配，Rust 逐项恢复，再由第三个进程确认原路径、长度、SHA-256 和匹配残留为 0。任何正常阶段失败都会尝试独立补偿；补偿失败时保留 preview 根和 run ID，禁止清空用户回收站。
