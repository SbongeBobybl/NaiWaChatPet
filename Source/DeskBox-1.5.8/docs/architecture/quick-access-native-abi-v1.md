# DeskBox Quick Access Rust 原生边界与 ABI v1

- 日期：2026-08-21
- 阶段：4D-4B
- 产品入口：`ExplorerQuickAccessHelper`
- 范围：固定状态查询、`pintohome`、`unpinfromhome`；不处理 Explorer 启动、上下文菜单、
  文件操作或托盘图标

## 1. 目标与实现选择

原实现通过 `Shell.Application`、`dynamic` 和 ProgID 激活访问 Quick Access。该链路是同步、
单向、无长期 COM 状态的粗粒度操作，`windows` crate 已提供 `IShellDispatch`、`Folder`、
`FolderItems`、`FolderItem` 和 `FolderItem2` 的强类型投影。因此 4D-4B 使用独立的完整 Rust
边界，没有在 C# 中重建通用 `IDispatch::Invoke`、DISPID、VARIANT 参数栈或接口继承。

本边界不与 4D-4A Explorer 托管启动共用操作导出。两者只共享同一个版本化 DLL、ABI 探针
和加载器。

## 2. 后端选择与线程边界

| 构建/运行方式 | Quick Access 后端 | C# oracle 是否进入编译单元 |
| --- | --- | --- |
| 普通 JIT，未设置环境变量 | C# | 是 |
| 普通 JIT，`DESKBOX_QUICK_ACCESS_BACKEND=rust` | Rust | 是，但失败时不静默回退 |
| Native AOT | Rust | 否 |

公开同步和异步 API 均保持不变。异步 API 继续创建后台 STA 线程，使用
`TaskCreationOptions.RunContinuationsAsynchronously` 返回结果，并保留既有耗时日志。Rust
导出本身同步执行；`S_OK`/`S_FALSE` 的 COM 初始化按本次调用配对撤销，
`RPC_E_CHANGED_MODE` 复用调用方 apartment，不撤销调用方初始化。

## 3. 产品行为冻结

### 3.1 查询

1. C# 先规范化路径；空白或非法路径返回 `Unknown`，不存在目录返回 `NotPinned`；
2. Rust 打开 Quick Access namespace，枚举项目并读取项目 `Path`；单个项目路径读取或规范化
   失败时跳过该项目；
3. 路径按忽略大小写、忽略末尾目录分隔符的规则比较；
4. 匹配项通过 `FolderItem2::ExtendedProperty("System.IsPinnedToNameSpaceTree")` 读取状态；
5. 仅接受 `VT_BOOL`、`VT_I4` 和可由 `bool.TryParse` 等价规则识别的 `VT_BSTR`；属性读取、
   接口转换或类型识别失败返回成功的 `Unknown` 查询，不把它改成 `NotPinned`；
6. 完成枚举但没有匹配项返回 `NotPinned`。

### 3.2 固定

固定前仍由 C# 规范化并创建目录，随后计算父路径与目录名。Rust 依次执行
`NameSpace(parent)`、`ParseName(folderName)` 和 `InvokeVerb("pintohome")`。目录创建、路径
验证和错误文本映射不进入原生 ABI。

### 3.3 取消固定

公开入口先执行状态查询；`NotPinned` 直接成功，保持重复取消固定的幂等行为。其他状态继续
调用取消固定操作。Rust 重新创建 Shell 对象并枚举 Quick Access：匹配时直接执行
`unpinfromhome`；未匹配，或 namespace 以空对象形式不可用时，回退到父目录
`ParseName` 后执行同一 verb。匹配项执行 verb 失败时不改走回退。

## 4. 模块版本与能力

- 模块 ABI：`2`；
- 结构版本：`1`；
- 能力位：`DESKBOX_NATIVE_CAPABILITY_QUICK_ACCESS_V1 = 1 << 7`；
- 完整能力掩码：`255`；
- 新导出：`deskbox_quick_access_v1`；
- 当前发布必需导出：9 个。

```c
uint32_t deskbox_quick_access_v1(
    const DeskBoxQuickAccessRequestV1* request,
    DeskBoxQuickAccessResultV1* result);
```

加载器固定从 `AppContext.BaseDirectory/deskbox_native.dll` 加载。调用前校验模块 ABI、能力位、
操作导出、输入和结果一致性；显式 Rust 或 AOT 路径失败时不调用 C# oracle。

## 5. 操作和输入

| 值 | 操作 | 输入 |
| ---: | --- | --- |
| 1 | `QUERY_PIN_STATE` | `folder_path` 必填；父路径和目录名必须为空 |
| 2 | `PIN` | 三项均必填 |
| 3 | `UNPIN` | 三项均必填 |

所有字符串均为显式长度的 UTF-16 slice，最多 32,767 个 code unit，不允许嵌入 NUL。空字符串
必须使用空指针和零长度。flags 与 reserved 字段必须为 0。

## 6. x64 结构布局

请求固定为 96 字节：

| 偏移 | 字段 | 类型/大小 |
| ---: | --- | --- |
| 0 | `struct_size` | `uint32_t`，必须为 96 |
| 4 | `struct_version` | `uint32_t`，必须为 1 |
| 8 | `operation` | `uint32_t` |
| 12 | `flags` | `uint32_t`，必须为 0 |
| 16 | `folder_path` | UTF-16 slice，16 字节 |
| 32 | `parent_path` | UTF-16 slice，16 字节 |
| 48 | `folder_name` | UTF-16 slice，16 字节 |
| 64 | `reserved[4]` | 32 字节，必须全为 0 |

结果固定为 112 字节：

| 偏移 | 字段 | 类型/大小 | 含义 |
| ---: | --- | --- | --- |
| 0 | `struct_size` | `uint32_t` | 必须保持 112 |
| 4 | `struct_version` | `uint32_t` | 必须保持 1 |
| 8 | `status` | `uint32_t` | 与函数返回值一致 |
| 12 | `operation_hresult` | `int32_t` | 最终 HRESULT |
| 16 | `attempted_phases` | `uint32_t` | 已进入阶段位集合 |
| 20..56 | 10 个阶段 HRESULT | `int32_t × 10` | 未进入为 `0x8000000A` |
| 60 | `pin_state` | `uint32_t` | 0 Unknown、1 NotPinned、2 Pinned |
| 64 | `operation_succeeded` | `uint32_t` | 只能为 0/1，并与成功状态一致 |
| 68 | `matched_item` | `uint32_t` | 是否命中枚举项 |
| 72 | `fallback_used` | `uint32_t` | unpin 是否使用父目录回退 |
| 76 | `reserved0` | `uint32_t` | 必须为 0 |
| 80 | `reserved[4]` | 32 字节 | 必须全为 0 |

托管和 Rust 双侧均有尺寸、偏移和 reserved 契约。托管还拒绝未知阶段位、非法布尔值、未知
pin state、返回状态不一致，以及成功状态与 `operation_succeeded` 不一致的结果。

## 7. 阶段诊断

| 位 | 阶段 |
| ---: | --- |
| `1 << 0` | COM 初始化 |
| `1 << 1` | 创建 `Shell.Application` |
| `1 << 2` | Quick Access namespace |
| `1 << 3` | `Items` 集合 |
| `1 << 4` | Count/Item 枚举 |
| `1 << 5` | 项目路径读取与规范化 |
| `1 << 6` | 固定属性读取 |
| `1 << 7` | 父目录 namespace |
| `1 << 8` | `ParseName` |
| `1 << 9` | `InvokeVerb` |

所有 COM 接口、集合、项目、`BSTR` 和 `VARIANT` 都由 Rust RAII 在一次调用返回前释放，不跨
线程、调用或 ABI 保存。

## 8. 自动化与人工边界

自动化覆盖后端策略、AOT 条件编译、公开 API/STA 契约、双侧 ABI 布局、非法操作、输入空值、
嵌入 NUL、不兼容结果、属性三种类型、路径比较、真实 DLL 能力与导出。当前系统还完成了一次
只读真实查询：返回 status 0、成功 1、命中 1、pin state 2、阶段 `0x7F`；没有调用 pin 或
unpin。

配置 21 / schema 18 的 x64 AOT 审计确认两个目标文件告警为 0，Quick Access 和完整
`always-throw` 均为 0，staging/publish DLL 哈希一致。真实固定、重复固定、取消固定、重复
取消固定、Explorer 重启后的刷新及失败恢复仍属于人工交互矩阵；自动化不会修改用户的
Quick Access 状态。AOT 主程序启动仍保留到阶段 5。
