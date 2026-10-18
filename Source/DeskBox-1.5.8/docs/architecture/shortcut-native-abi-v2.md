# DeskBox Shortcut Native ABI v2 契约

## 1. 状态与范围

本文记录完成阶段 3C-3 后的 x64 C ABI、只读/Resolve/写入/Windows UI 实现与产品接入。公开定义以
`native/include/deskbox_native.h` 为准，Rust 结构布局由单元测试再次校验；
既有读取与 Resolve 结构尺寸相对 3B-0 没有变化；3C-1 追加独立的输入字符串、写入
请求和写入结果结构；3C-2 再追加独立的 UI Resolve 请求/结果结构，仍不修改已经
发布的结构布局；3C-3 不修改 ABI，只收口 AOT 编译、诊断与打包边界。

阶段 4C 在同一 DLL 和 ABI 2 上追加独立的音乐音量 v1 能力与导出，没有改变本文件冻结的
任何 shortcut 结构、状态或行为。当前完整模块能力掩码为 63，音乐音量新增位与结构见
[`music-volume-native-abi-v1.md`](./music-volume-native-abi-v1.md)；下表的 31 仍是阶段 3C-3
shortcut 收口时的历史值。

| 项目 | 阶段 3C-3 状态 |
| --- | --- |
| ABI 版本 | 2 |
| DLL | `deskbox_native.dll` |
| 能力位 | 31，即两个读取模式、`RESOLVE_NO_UI`、`WRITE` 与 `RESOLVE_WITH_UI` |
| shortcut read 导出 | 已实现真实 `.lnk` 只读 |
| no-UI Resolve 导出 | 已实现；不显示 UI，保留原始 HRESULT 后继续读元数据 |
| shortcut write 导出 | 已实现；完整设置五字段后覆盖保存 `.lnk` |
| Windows UI Resolve 导出 | 已实现；在调用方线程传递 owner HWND，保留更新、提示和删除语义 |
| DeskBox 产品接入 | JIT 默认 C#、显式 Rust opt-in；Native AOT 编译期只保留 Rust shortcut 路径 |
| 支持架构 | x64；ARM64 结构尺寸已预留为相同的 64 位指针布局，但尚未构建和验收 |

3C-2 在两套既有 `.lnk` 读取语义、无 UI Resolve 和完整写入之外，迁移损坏链接的
Windows 修复/删除 UI，并把文件组件的真实宿主 HWND 接到既有产品调用链。3C-3 保留
普通 JIT 的 C# oracle，但从 Native AOT 编译单元排除旧 `ComImport` 分支；原生缓存、
`.url` 解析和普通 JIT 默认后端没有改变。

## 2. 导出与能力协商

ABI 2 要求 DLL 同时导出：

```text
deskbox_native_abi_version
deskbox_native_capabilities
deskbox_shortcut_read_v2
deskbox_shortcut_resolve_no_ui_v2
deskbox_shortcut_write_v2
deskbox_shortcut_resolve_with_ui_v2
deskbox_music_volume_v1
```

调用方必须先安全加载 DLL 并解析版本探针、能力探针及四个操作导出；任何必需导出
缺失都拒绝模块。全部地址齐备后再调用探针，确认 ABI 等于 2，并在每类操作前检查
对应能力位。不能把“导出存在”视为“功能可用”。阶段 3C-3 的五个 shortcut 能力位固定为
31；阶段 4C 增加独立的 `1 << 5` 音乐音量能力后，完整模块掩码为 63。JIT 只有显式选择
对应 Rust 后端时才加载模块，默认仍为 C#；Native AOT
强制选择 Rust，加载失败时不得回退旧 `ComImport`。

能力位定义如下：

| 位 | 含义 | 当前状态 |
| --- | --- | --- |
| `1 << 0` | `STORED_RAW` 读取 | 已启用 |
| `1 << 1` | `EFFECTIVE_DIAGNOSTIC` 读取 | 已启用 |
| `1 << 2` | 无 UI Resolve 后读取 | 已启用 |
| `1 << 3` | 完整元数据写入并覆盖保存 | 已启用 |
| `1 << 4` | 带 owner HWND 的 Windows UI Resolve | 已启用 |
| `1 << 5` | 音乐系统/session 音量 v1 | 已启用；独立契约 |

缺少 DLL、ABI 不符、导出不全或能力位未启用都必须作为不同的可诊断状态保留。
AOT 模式不得因这些错误回退到不受支持的旧 `ComImport` 路径。

## 3. ABI 基本规则

- 调用约定为 C ABI；Windows 头文件明确为 `__cdecl`。
- 只跨边界传递固定宽度整数、指针和长度；结构体使用自然对齐的 `repr(C)`。
- 当前只支持 64 位进程。UTF-16 输出缓冲、UTF-16 输入字符串、读取请求、读取结果、
  Resolve 请求、写入请求、写入结果、UI Resolve 请求和 UI Resolve 结果的固定尺寸
  分别是 16、16、144、136、192、144、96、64、64 字节。
- 调用方必须把 `struct_size` 设为精确尺寸，把 `struct_version` 设为 2。
- 所有 `flags`、`reserved0` 和 `reserved[]` 在 ABI 2 中必须为 0。
- shortcut 路径为只读 UTF-16 指针和不含终止空字符的字符数。空指针、长度 0、
  长度超过 32767 或内容中嵌入空字符是 `INVALID_ARGUMENT`；原生实现自行复制并补
  终止空字符，不保存调用方指针。
- Rust 分配的内存、Rust 对象、COM 指针、异常和 panic 不得跨 ABI。Release 与
  Debug 都使用 `panic=abort`。
- 每次调用同步完成，不创建后台线程，不保留 COM 对象，也不建立原生缓存。

如果 `result` 为空，函数只返回 `INVALID_ARGUMENT`。如果结果结构的尺寸或版本不符，
函数返回 `INCOMPATIBLE_STRUCT` 且不修改结果内存。结果结构有效时，函数返回值必须
始终等于 `result.status`。

## 4. UTF-16 输出缓冲区

`DeskBoxNativeUtf16BufferV1` 由调用方拥有：

- `data == null && capacity_chars == 0` 表示只查询结果长度；
- `data != null && capacity_chars > 0` 表示可写缓冲区，容量包含终止空字符；
- 其余组合或非零 `reserved0` 为 `INVALID_ARGUMENT`；
- 单个字段容量不足时不写入该字段的部分字符串；其他容量足够的字段仍可写入；
- 对已成功读取的字段，`*_required_chars` 是精确的“内容长度 + 1”；成功的空字符串
  因而为 1；未尝试或读取失败的字段为 0；
- 任一成功字段容量不足时设置对应的 `caller_buffer_too_small_fields` 位，顶层状态为
  `BUFFER_TOO_SMALL`，顶层 HRESULT 为 `0x8007007A`；容量为 0 的长度查询也遵守此规则。

原生实现内部仍必须使用与现有 C# 行为一致的 Shell 缓冲区。微软的
[`IShellLinkW::GetPath`](https://learn.microsoft.com/en-us/windows/win32/api/shobjidl_core/nf-shobjidl_core-ishelllinkw-getpath)
契约规定返回上限为 `MAX_PATH`，且 `cch` 包含终止空字符。若兼容缓冲区最终占满到 `capacity - 1`，
`source_truncated_fields` 必须保守置位；这表示源读取可能已截断，与调用方输出缓冲区
不足是两个独立状态。

## 5. UTF-16 输入字符串

`DeskBoxNativeUtf16StringV1` 由调用方拥有，长度不含终止空字符：

- `data == null && length_chars == 0` 只表示允许为空的可选字段；
- 非空字段必须满足 `data != null && length_chars > 0`；shortcut 路径与目标路径不能为空；
- 单个输入最多 32767 个 UTF-16 code unit，内容中不得嵌入空字符；
- `reserved0` 必须为 0；
- 原生实现先按显式长度复制，再追加终止空字符。调用期间只读调用方内存，返回后不
  保留输入指针。

写入请求中的描述、参数、工作目录和图标路径允许为空；空值仍是一个明确的写入值，
用于覆盖已有快捷方式时清除旧字段，不能解释为“跳过此 setter”。

## 6. 两种读取模式

ABI 2 不合并当前两条 C# 路径。

| 规则 | `STORED_RAW` | `EFFECTIVE_DIAGNOSTIC` |
| --- | --- | --- |
| 当前对应代码 | `ShortcutHelper.ReadStoredMetadata` | `DragDropPermissionService.TryReadShortcut` |
| 是否调用 Resolve | 否 | 否 |
| `GetPath` flags | `SLGP_RAWPATH` | 0 |
| 目标内部容量 | 260 | 260 |
| 参数内部容量 | 260 | 512 |
| 其他字段 | 描述、工作目录、图标路径与索引，各 260 | 不尝试 |
| 后处理 | 五个字符串均不 Trim | 目标和参数分别 Trim |
| 成功条件 | Load 及五个 getter 均未失败；空字段保留 | Load、目标和参数 getter 均未失败，Trim 后目标非空白 |

`STORED_RAW` 的原始目标可能不存在，也可能含未展开的环境变量。图标索引使用有符号
32 位整数，必须保留负值。应用层的完整路径规范化、长度/时间戳指纹、最多 512 项
缓存和 `.url` 分派继续留在 C#；`.url` 必须在尝试加载 Rust DLL 之前完成分派。

## 7. 结果、HRESULT 与字段状态

固定状态码如下：

| 值 | 状态 | 含义 |
| ---: | --- | --- |
| 0 | `OK` | 操作按当前模式完成，调用方缓冲区充足 |
| 1 | `INVALID_ARGUMENT` | 空指针、非法模式、非法缓冲区、非零保留字段等 |
| 2 | `INCOMPATIBLE_STRUCT` | 结构尺寸或版本不符 |
| 3 | `BUFFER_TOO_SMALL` | 至少一个已成功读取的字段无法完整复制 |
| 4 | `COM_INITIALIZATION_FAILED` | COM 初始化失败且不能继续 |
| 5 | `OBJECT_CREATION_FAILED` | 创建 Shell Link COM 对象失败 |
| 6 | `LOAD_FAILED` | `IPersistFile::Load` 失败 |
| 7 | `OPERATION_FAILED` | getter 或要求成功的操作失败 |
| 8 | `INTERNAL_ERROR` | 无法映射到上述类别的原生内部错误 |
| 9 | `NOT_IMPLEMENTED` | ABI 已导出，但能力尚未实现 |

`operation_hresult` 保存顶层原始 HRESULT。读取结果中的 `com_hresult`、
`create_hresult`、`load_hresult`、`resolve_hresult` 和五个 getter HRESULT，写入结果中的
`com_hresult`、`create_hresult`、`save_hresult` 和五个 setter HRESULT，以及 UI Resolve
结果中的 COM、创建、Load 与 Resolve HRESULT，都保存各自原始值；
未尝试时写入 `0x8000000A`。不能只通过字符串是否为空推断成功：

- `attempted_fields` 表示调用过的 getter；
- `succeeded_fields` 对 HRESULT 成功值置位，包括 `S_FALSE`；
- `present_fields` 只对最终字符串非空置位；
- `GetPath` 返回 `S_FALSE` 时字段已尝试且成功、字符串为空、required 为 1；
- 字段 HRESULT 失败时 required 为 0，并保留失败值；
- `attempted_phases` 分别记录 COM 初始化、对象创建、Load、Resolve 和 Save 是否执行。

读取操作的顶层状态优先级固定为：结果结构校验、请求结构与参数校验、COM 初始化、
对象创建、Load、字段 getter、调用方缓冲区不足、成功。字段容量不足不会阻止后续
getter；如果后续 getter 失败，最终返回 `OPERATION_FAILED` 和该失败 HRESULT，而不是
用 `BUFFER_TOO_SMALL` 掩盖真实操作失败。`source_truncated_fields` 是独立诊断位，本身
不改变顶层成功状态。`EFFECTIVE_DIAGNOSTIC` 的目标在 Trim 后为空时返回
`OPERATION_FAILED/S_FALSE`。

Resolve 在 Load 后无条件记录 `RESOLVE` phase 与原始 `resolve_hresult`，再执行
`STORED_RAW` 五字段读取。`S_OK`、`S_FALSE` 和失败 HRESULT 都不会单独决定顶层状态；
只有 Load、字段 getter 或调用方缓冲区状态决定最终结果。

## 8. 无 UI Resolve

`deskbox_shortcut_resolve_no_ui_v2` 只接受嵌套读取模式 `STORED_RAW`，语义与
`ShortcutHelper.Resolve` 对齐：

1. Load `.lnk`；
2. 用空父窗口调用 `Resolve`；
3. 固定使用 `SLR_NO_UI | SLR_NOSEARCH`；
4. `timeout_ms == 0` 保留 Windows 默认的 3000 ms；1 至 65535 通过 flags 高 16 位
   传递；更大值是 `INVALID_ARGUMENT`；
5. 不增加 `SLR_NOTRACK`，因此关闭搜索启发式不等于关闭分布式链接跟踪；
6. Resolve 返回 `S_OK`、`S_FALSE` 或失败 HRESULT 都记录原值，随后仍读取已存储
   元数据。只有 Load 或后续读取失败才使整体操作失败。

该调用可能同步阻塞到指定超时。是否从 UI 线程调用由 C# 编排层决定，原生 DLL
不得暗中切线程，也不得显示 UI。超时高位、默认 3000 ms、`SLR_NOSEARCH` 与
`SLR_NOTRACK` 的区别以微软
[`IShellLinkW::Resolve`](https://learn.microsoft.com/en-us/windows/win32/api/shobjidl_core/nf-shobjidl_core-ishelllinkw-resolve)
契约为准。

## 9. 快捷方式写入

`deskbox_shortcut_write_v2` 创建一个新的 Shell Link 对象，不先 Load 旧文件，并按固定
顺序执行以下 setter：

1. `SetPath`；
2. `SetDescription`；
3. `SetArguments`；
4. `SetWorkingDirectory`；
5. `SetIconLocation`，图标索引按有符号 32 位整数原样传递；
6. `IPersistFile::Save(shortcut_path, TRUE)`。

五个 setter 都必须执行成功后才尝试 Save；首个失败 HRESULT 作为顶层
`OPERATION_FAILED` 返回，后续字段和 Save 保持未尝试。`attempted_fields` 和
`succeeded_fields` 记录实际 setter 进度，成功值包括 `S_FALSE`。Save 则严格只有
`S_OK` 表示写入成功；`S_FALSE` 或任何失败 HRESULT 都返回 `OPERATION_FAILED`，并在
`save_hresult` 中保留原值。这与微软
[`IPersistFile::Save`](https://learn.microsoft.com/en-us/windows/win32/api/objidl/nf-objidl-ipersistfile-save)
规定的返回语义一致。

覆盖已有路径依靠 Save 完成。由于每次都创建新对象且所有可选字段都会显式调用
setter，空描述、空参数、空工作目录和空图标路径会清除旧值，不会继承旧 `.lnk` 的
残留元数据。Rust 层不创建父目录、不做应用级路径规范化，也不管理缓存；文件夹快捷
方式的 `Path.GetFullPath`、两类写入的父目录创建以及成功后的已存储元数据缓存失效仍
由 C# 编排层负责。

`DeskBoxShortcutWriteRequestV2` 固定为 144 字节，包含 shortcut 路径、五个元数据字段、
图标索引以及保留位；`DeskBoxShortcutWriteResultV2` 固定为 96 字节，包含顶层状态、
COM/创建/Save HRESULT、字段掩码和五个 setter HRESULT。写入同步完成，不提供 `.url`
写入。Windows UI 修复使用下一节的独立结构和导出，不复用写入结果。

## 10. Windows UI Resolve

`deskbox_shortcut_resolve_with_ui_v2` 使用独立的 64 字节请求与 64 字节结果，不嵌套
读取缓冲区。请求包含 `.lnk` 路径和一个按 64 位位模式传递的 `owner_hwnd`；结果包含
顶层状态、phase、四段 HRESULT 与实际 Resolve flags。调用顺序固定为：

1. 在调用方线程初始化或复用 COM；
2. 创建 Shell Link 并取得 `IPersistFile`；
3. 以 `STGM_READ` Load `.lnk`；
4. 以传入的 HWND 为父窗口同步调用 `Resolve`；
5. 固定 flags 为 `SLR_UPDATE | SLR_NOSEARCH | SLR_OFFER_DELETE_WITHOUT_FILE`，即
   `0x0214`，明确不设置 `SLR_NO_UI`。

`SLR_UPDATE` 允许 Shell 在找到目标后更新已经通过 `IPersistFile` 加载的快捷方式；
`SLR_NOSEARCH` 保留旧产品关闭搜索启发式的行为，但不等于关闭分布式链接跟踪；
`SLR_OFFER_DELETE_WITHOUT_FILE` 保留无法修复时的删除提示。相关语义以微软
[`IShellLinkW::Resolve`](https://learn.microsoft.com/en-us/windows/win32/api/shobjidl_core/nf-shobjidl_core-ishelllinkw-resolve)
契约为准。

ABI 允许 `owner_hwnd == 0` 以保持 Win32 兼容性，但产品的文件组件激活路径必须传递
`FileSurfaceContent` 已保存的真实宿主 HWND。原生层不校验窗口归属，不保留句柄，也
不切换线程。Load 或 Resolve 失败会保留原始 HRESULT 并返回对应失败状态；显式 Rust
模式不回退 C#。C# 编排层在调用结束后使已存储元数据缓存失效，再按 `.lnk` 是否仍
存在映射为既有 `ResolvedOrKept` / `ShortcutDeleted`，因此用户在 Shell UI 中删除链接
时仍会触发 ViewModel 移除对应项目。

自动化测试只对有效快捷方式、损坏文件、固定 flags、结构布局、非零 HWND 产品调用
和缓存失效进行无交互验证。真正缺失目标时的 Windows 模态窗口、父子窗口关系、更新
目标与“删除快捷方式”按钮已经另外在 JIT 显式 Rust 实例中人工验证，未用有效链接
测试代替。

人工记录位于 `.artifacts/manual-shortcut-3c2/20260820-211241`：

1. 进程来自规范 Debug 输出，首次 `.lnk` 产品调用后确认实际加载同一目录中的
   `deskbox_native.dll`；
2. Widget HWND 为 `0x430DBE`，取消对话框 HWND 为 `0x380F62`，其 owner 是该 Widget；
   取消后 `.lnk` 与 Widget 项均保留；
3. 把目标在同卷移动后重新触发修复，Shell 跟踪把 `.lnk` 更新到新路径；再次打开能
   启动新目标；
4. 删除对话框 HWND 为 `0x3A0A0A`，owner 是该 Widget；确认删除后磁盘文件与 Widget
   项立即同时消失；
5. 验收进程结束后不再保留 `DESKBOX_SHORTCUT_BACKEND=rust` 进程级 opt-in，普通 JIT
   默认后端保持 C#。

Windows 版本可能改变对话框文案或布局，因此验收记录以动作和最终状态为准，不依赖
按钮的单一语言文本。

## 11. COM 初始化与线程模型

读取、两类 Resolve 与写入共用的实现按以下矩阵处理
[`CoInitializeEx`](https://learn.microsoft.com/en-us/windows/win32/api/combaseapi/nf-combaseapi-coinitializeex)：

| 返回值 | 后续动作 | 返回前是否 `CoUninitialize` |
| --- | --- | --- |
| `S_OK` | 在本调用线程继续 | 是 |
| `S_FALSE` | 在已初始化的同模型 apartment 继续 | 是 |
| `RPC_E_CHANGED_MODE` | 复用调用线程当前 apartment，继续尝试创建对象 | 否 |
| 其他失败 HRESULT | 返回 `COM_INITIALIZATION_FAILED` | 否 |

原生实现以 MTA 初始化请求作为默认尝试，从而覆盖未初始化的线程池线程；WinUI STA
线程出现 `RPC_E_CHANGED_MODE` 时不改变其 apartment。每个调用创建并释放自己的
Shell Link 对象，所有接口在原调用线程释放，绝不跨调用或跨线程保存。

## 12. 托管加载器与后端选择

产品加载器固定从 `AppContext.BaseDirectory/deskbox_native.dll` 加载，不使用当前目录、
`PATH` 或任意 DLL 搜索结果。Windows 加载标志固定为
`LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR | LOAD_LIBRARY_SEARCH_SYSTEM32`。基础 shortcut 加载先解析
六个 ABI/shortcut 导出，再调用探针验证 ABI 2，并在调用前验证所需能力位；音乐音量路径
在能力位存在时另外解析 `deskbox_music_volume_v1`。构建和发布验证要求当前模块七个导出
全部存在；托管调用使用静态非托管函数指针，不依赖动态 delegate 封送。

JIT 默认选择 C#。只有进程启动前设置 `DESKBOX_SHORTCUT_BACKEND=rust` 才显式选择
Rust；选择后任何模块、ABI、导出、能力或调用失败都记录诊断并返回失败，不回退 C#。
`PublishAot=true` 会定义 `DESKBOX_NATIVE_AOT`：后端策略在编译期固定为 Rust，两套
legacy helper、coclass、接口及其调用引用以 `#if !DESKBOX_NATIVE_AOT` 排除出 AOT
编译单元。`RuntimeFeature.IsDynamicCodeSupported == false` 的运行时判断继续作为第二层
保护。`.url` 扩展名在访问加载器前仍由 C# 解析。

受支持的 x64 AOT 审计脚本会同时显式设置 `DeskBoxRustNative=true`，因此其发布目录会
构建并复制唯一的 x64 原生模块。3C-3-R 新增项目目标
`ValidateDeskBoxNativeAotConfiguration`：任何 Native AOT 构建都必须同时满足
`Platform=x64`、`RuntimeIdentifier=win-x64` 和 `DeskBoxRustNative=true`，否则在编译前
失败；普通 JIT 默认走 C# 的行为不变。审计脚本也只允许 x64，`-Platform ARM64` 会在
解析工具、采集工作树和触碰产物目录前失败，直到阶段 7 完成 ARM64 Rust 模块与验证。

诊断快照不通过 `ShortcutNativeBackend.Default` 触发 `Lazy.Value`。它只在加载器此前已
由产品路径探测时读取缓存状态；始终只输出相对模块名、文件存在性、PE 架构和 SHA-256，
不输出绝对路径、未脱敏加载详情或 DLL 内容。

## 13. 3C-2、3C-3 与 3C-3-R 验收结果

已完成的自动化边界包括：

- Rust 格式化、Clippy、测试通过，结构尺寸测试通过；测试使用真实 `.lnk` 覆盖
  Unicode、原始/诊断语义、空字段、负图标索引、损坏文件、目标缺失、长度查询、
  无部分写入、260/512 源边界、写入覆盖清空、setter/Save 失败、UI Resolve 固定 flags、
  有效/损坏链接以及 STA/MTA；
- 构建脚本从实际 DLL 读取 ABI 2 和能力掩码 31，并验证六个必需导出；
- JIT C# oracle 与 Rust 覆盖普通/Unicode、相对路径、UNC、环境变量、目标缺失、
  源截断、损坏文件、PIDL、STA/MTA、并发、Resolve 后结果、文件夹/应用写入形状、
  覆盖清空、UI Resolve、真实宿主 HWND 路由和缓存失效；
- 安全加载器能区分模块缺失、导出缺失、ABI 不符、能力缺失和原生调用失败；
- AOT 审计摘要记录 ABI、能力位、必需导出和锁定的 Rust 依赖图；
- 不设置 opt-in 的规范 JIT Debug 实例仍运行 C# 后端且不加载 Rust DLL；显式 Rust
  模式不静默回退；
- 3C-2 人工验证实际 Rust DLL 路径、父窗口、取消、修复和删除；
- 3C-3 静态验证 AOT 编译期排除 legacy COM、所有产品激活均需 HWND、诊断无副作用、
  x64/ARM64 安装输入隔离，以及分离更新器不复制原生 DLL；
- 3C-3-R 真实执行普通 JIT、完整 x64 direct/audit AOT、缺失 Rust、ARM64 与 Platform/RID
  冲突组合，并验证 ARM64 审计在解析无效 `DotNetPath` 前即返回预期错误；
- 不运行本阶段生成的 AOT 主程序。

2026-08-20 最终结果：Rust 格式化与 Clippy `-D warnings` 通过，33/33 原生单元测试
通过；3C-3 的 shortcut/AOT 契约定向测试为 85/85，3C-3-R 的 AOT 发布契约测试为
19/19，显式 Rust 产品入口测试为 3/3，DeskBox x64 全量测试为 1970/1970；规范 Debug
构建为 0 错误、30 条既有警告。未设置 opt-in
的新实例精确运行自规范 Debug 输出，启动阶段加载的 `deskbox_native.dll` 数量为 0。
审计配置 11 / schema 9 的 x64 AOT 发布通过，实际 DLL 为
ABI 2、能力掩码 31，并包含六个必需导出；发布目录为 39 个文件、约 79.5 MiB，符号
目录为 3 个 PDB、约 164.9 MiB。隔离 staging 与 publish DLL 的 SHA-256 完全一致，具体值
由对应审计 `summary.json` 留档，不作为跨构建固定的 ABI 值；发布前后工作树指纹一致。
shortcut `always-throw` 为 0；在 3C-3-R 完成时，主程序还剩 FolderPicker 与音乐音量两条。

后续阶段 4A 没有修改 Rust ABI、能力或产品后端策略。FolderPicker 已迁移为 Windows App
SDK 现代异步 Picker；新增 FolderPicker 契约 4/4、与 AOT 发布契约合并执行 23/23、
DeskBox x64 全量测试 1974/1974 通过。审计配置 12 / schema 9 保持 39 个发布文件、3 个
分离 PDB、12 类既有警告、ABI 2、能力 31 和同次 staging/publish 哈希一致；主程序现在
只剩音乐音量 1 条 `always-throw`，shortcut 仍为 0。AOT 主程序没有启动；AOT shortcut
运行冒烟留到其他硬阻断清除并具备隔离测试环境后。规范 Debug 的真实 JumpList 转发探针
已确认现代 Picker 的 owner 等于托盘 HWND，关闭取消后主进程继续响应；完整 FolderPicker
人工入口矩阵随后已通过。4B-0 只冻结 JSON 现状和金样，没有修改 Rust ABI、能力、后端
策略或生产序列化；其 x64 全量测试 1981/1981 和配置 12 / schema 9 隔离 AOT 审计通过，
随后 4B-1 只迁移叶子资源、网络 DTO 与诊断数据的 8 处 JSON 调用，同样没有修改 Rust
ABI、能力或后端策略。4B-1 的 x64 全量测试 1983/1983 和配置 12 / schema 9 隔离 AOT
审计通过；JSON 直接相关警告 210→176，shortcut `always-throw` 仍为 0，Rust ABI 2、能力
31 与同次 staging/publish 哈希一致。4B-2 又只迁移 Settings、Quick Capture、Todo、
Glance preferences、Glance image catalog 和文件组件 metadata 的 22 处 JSON 调用，仍未
修改 Rust ABI、能力或后端策略。4B-2 的 x64 全量测试 1986/1986 和配置 12 / schema 9
隔离 AOT 审计通过；JSON 直接相关警告 176→80，shortcut `always-throw` 仍为 0，Rust
ABI 2、能力 31 与同次 staging/publish 哈希一致。当前只开放 4B-3 的跨域维护与搜索 JSON
迁移，AOT shortcut 运行冒烟仍保留到阶段 5。

## 14. 阶段 4C 后的模块状态

2026-08-21，阶段 4B-3A/3B/3C/4 已完成全部 JSON source generation 与默认反射关闭
审计；阶段 4C 随后在相同 DLL 中增加音乐音量 v1 边界。shortcut ABI 和五类行为没有变化，
完整模块改为 ABI 2、能力 63、七个发布必需导出。Rust 单元测试增至 42/42，DeskBox x64
全量测试增至 2006/2006；新增测试覆盖
音量归一化、session 匹配优先级、系统声音排除和 ABI 输入校验。

配置 14 / schema 11 的隔离 x64 AOT 审计继续产生 39 个发布文件和 3 个分离 PDB，保留
12 类既有警告；staging/publish Rust DLL 哈希一致，shortcut 与音乐音量
`always-throw` 均为 0，完整 `always-throw` 集合也为 0。AOT 主程序仍按阶段计划不启动；
shortcut 与音乐音量的真实 AOT 运行冒烟统一保留到阶段 5。

## 15. 阶段 4D-1A 后的模块状态

2026-08-21，4D-1A 只处理 DispatcherQueue 结构尺寸、Markdig task-list 和搜索推荐三个
托管类型化入口，没有修改 shortcut/music volume ABI、能力、导出或后端策略。配置 15 /
schema 12 的隔离 x64 AOT 审计继续确认 ABI 2、能力 63、七个必需导出、staging/publish
DLL 哈希一致以及完整 `always-throw=0`。AOT 主程序仍未启动；随后完成的 4D-1B 同样没有
修改原生模块，并确认原计划 4D-2 的 `FileOperationHelper` 没有调用者。4D-2 已完成死代码
删除，配置 17 / schema 14 将对应 IL2050 清零，现有 Rust DLL 仍未扩展。下一阶段 4D-3
中的 4D-3A 已用 C# 窄 vtable 边界迁移 OLE 数据读取侧，配置 18 / schema 15 通过；
4D-3B 再用源生成 COM 处理带反向回调的 `IDropTarget` 注册。后续 `Shell.Application`
dynamic 因属于生成器不支持的 `IDispatch`，再单独比较静态 Shell API 与完整 Rust 粗粒度边界。

4D-3B 随后已完成，配置 19 / schema 16 将 IL2050 清零，shortcut ABI、能力位和四类行为
均未变化。`Shell.Application` 后续又拆为 4D-4A Explorer 托管环境启动和 4D-4B 快速访问，
先分别评审完整 Rust 粗粒度边界，不一次扩展整个 Automation 对象模型。

4D-4A 现已在同一 DLL 中加入独立的 Explorer 托管启动 v1 操作；shortcut v2 的结构、五个
能力位和产品策略仍未改变。完整模块当前为 ABI 2、能力 127、八个必需导出；配置 20 /
schema 17 确认 staging/publish 哈希一致，Explorer 目标警告及完整 `always-throw` 均为 0。
新操作的独立契约见 `explorer-shell-launch-native-abi-v1.md`。

4D-4B 现已在同一 DLL 中加入独立的 Quick Access v1 操作；shortcut v2 的结构、五个能力位
和产品策略仍未改变。完整模块当前为 ABI 2、能力 255、九个必需导出；配置 21 / schema 18
确认 staging/publish 哈希一致，Quick Access 目标警告及完整 `always-throw` 均为 0。新操作的
独立契约见 `quick-access-native-abi-v1.md`。
