# DeskBox Explorer 托管启动 Rust 原生边界与 ABI v1

- 日期：2026-08-21
- 阶段：4D-4A
- 产品入口：`Win32Helper.OpenFileOrChooseApp` → `ExplorerShellLaunchService.TryOpen`
- 范围：仅迁移 Explorer 托管环境中的 `ShellExecute`；不处理快速访问固定状态、pin/unpin、
  Shell 上下文菜单或通用进程启动

## 1. 目标与行为边界

DeskBox 优先让正在运行的 Explorer 桌面进程执行 `ShellExecute`。这样启动的应用继承当前
Explorer 用户环境，避免 DeskBox 自身启动时保存的环境变量传给 Electron 等关联程序。
这一语义不能退化为单纯的 `Process.Start(UseShellExecute=true)`。

4D-4A 在既有 `deskbox_native.dll` 中增加一个完整、同步、无状态的 Rust 操作。Rust 使用
`windows` crate 生成的强类型 Shell 接口完成下列链路：

```text
Shell / IShellDispatch
  → Windows / IShellWindows
  → FindWindowSW(SWC_DESKTOP, SWFO_NEEDDISPATCH)
  → IWebBrowser.Document
  → IShellFolderViewDual.Application
  → IShellDispatch2.ShellExecute
```

该边界只把路径、工作目录和 verb 传入 Rust。COM 接口、`BSTR`、`VARIANT` 和所有权均不跨
C ABI，也不在调用间缓存。没有在 C# 中重建完整 `IDispatch::Invoke`、DISPID 查询和
VARIANT 生命周期。

## 2. 后端选择与失败策略

| 构建/运行方式 | Explorer 启动后端 | 原生调用失败后的产品行为 |
| --- | --- | --- |
| 普通 JIT，未设置环境变量 | 原 C# dynamic 实现 | 继续既有本地 ShellExecute 回退 |
| 普通 JIT，`DESKBOX_EXPLORER_SHELL_BACKEND=rust` | Rust | 不静默回退 C#；由产品层走既有本地回退 |
| Native AOT | Rust | C# dynamic/RCW 实现不进入编译单元；由产品层走既有本地回退 |

原生边界自身不调用 C# oracle。`ExplorerShellLaunchService.TryOpen` 返回失败后，
`Win32Helper.OpenFileOrChooseApp` 仍按原顺序尝试
`Process.Start(UseShellExecute=true)`；遇到 Win32 1155 时再显示带 owner HWND 的
`SHOpenWithDialog`。因此 4D-4A 没有改变关联程序、Open With 或最终布尔返回契约。

## 3. 模块版本与能力

模块整体 ABI 版本保持为 `2`。本批是向后兼容的能力扩展：

- 新能力位：`DESKBOX_NATIVE_CAPABILITY_EXPLORER_SHELL_LAUNCH_V1 = 1 << 6`；
- 完整能力掩码：`127`；
- 新导出：`deskbox_explorer_shell_launch_v1`；
- 当前发布模块必需导出总数：8。

导出函数使用 C calling convention：

```c
uint32_t deskbox_explorer_shell_launch_v1(
    const DeskBoxExplorerShellLaunchRequestV1* request,
    DeskBoxExplorerShellLaunchResultV1* result);
```

托管加载器仍固定从 `AppContext.BaseDirectory/deskbox_native.dll` 加载，先校验 ABI 2 和基础
shortcut 导出；本操作还会校验能力位和自身导出。任何模块、ABI、能力、导出、输入或结果
异常都会返回可诊断失败，不回退到同进程 C# dynamic 实现。

## 4. x64 结构布局

请求结构版本为 1，x64 固定为 96 字节：

| 偏移 | 字段 | 类型/大小 | 规则 |
| ---: | --- | --- | --- |
| 0 | `struct_size` | `uint32_t` | 必须为 96 |
| 4 | `struct_version` | `uint32_t` | 必须为 1 |
| 8 | `flags` | `uint32_t` | 必须为 0 |
| 12 | `reserved0` | `uint32_t` | 必须为 0 |
| 16 | `path` | UTF-16 slice，16 字节 | 必填 |
| 32 | `working_directory` | UTF-16 slice，16 字节 | 可为空 |
| 48 | `verb` | UTF-16 slice，16 字节 | 必填，当前产品传 `open` |
| 64 | `reserved[4]` | 32 字节 | 必须全为 0 |

结果结构版本为 1，x64 固定为 88 字节：

| 偏移 | 字段 | 类型/大小 | 含义 |
| ---: | --- | --- | --- |
| 0 | `struct_size` | `uint32_t` | 必须保持 88 |
| 4 | `struct_version` | `uint32_t` | 必须保持 1 |
| 8 | `status` | `uint32_t` | 与函数返回值一致 |
| 12 | `operation_hresult` | `int32_t` | 最终成功或失败 HRESULT |
| 16 | `attempted_phases` | `uint32_t` | 已进入阶段的位集合 |
| 20..44 | 7 个阶段 HRESULT | `int32_t × 7` | 未进入时为 `0x8000000A` |
| 48 | `operation_succeeded` | `uint32_t` | 只能为 0 或 1，并与成功状态一致 |
| 52 | `reserved0` | `uint32_t` | 必须为 0 |
| 56 | `reserved[4]` | 32 字节 | 必须全为 0 |

三个 UTF-16 输入均使用显式字符数，不以结尾 NUL 决定长度。每项最多 32,767 个 UTF-16
code unit，不允许嵌入 NUL；空工作目录必须使用空指针和零长度。路径和 verb 不允许为空。

## 5. 阶段诊断与 COM 生命周期

`attempted_phases` 依次记录：

| 位 | 阶段 |
| ---: | --- |
| `1 << 0` | `CoInitializeEx` |
| `1 << 1` | 创建本地 `Shell.Application` 对象 |
| `1 << 2` | 获取并转换 `IShellWindows` |
| `1 << 3` | 定位 Explorer 桌面窗口 |
| `1 << 4` | 获取桌面 document |
| `1 << 5` | 获取 Explorer-hosted Application |
| `1 << 6` | 执行 `ShellExecute` |

调用先请求 STA。`S_OK` 和 `S_FALSE` 会按本次初始化次数配对 `CoUninitialize`；
`RPC_E_CHANGED_MODE` 表示复用调用线程现有 apartment，不对调用方的 COM 初始化做撤销；其他
初始化失败直接返回。所有生成的强类型 COM 接口依靠 Rust RAII 在同一线程、函数返回前释放。

## 6. 自动化与人工验证边界

自动化当前覆盖：

- Rust 结构尺寸、偏移、能力、输入 envelope、空必填项、嵌入 NUL 和不兼容结果 envelope；
- `cargo fmt`、Clippy `-D warnings` 和 45/45 Rust 测试；
- C# 后端选择、AOT 编译期排除 dynamic oracle、托管结构布局、真实 DLL 能力与导出；
- 构建脚本要求能力 127 和八个导出；
- x64 Native AOT 配置 20 / schema 17 发布审计，4D-4A 两个目标文件告警为 0，
  Explorer 启动 `always-throw` 为 0。

自动化不会主动打开用户文件、URL 或系统对话框。进入 4D-4B 前，仍需在显式 Rust JIT 实例
中人工确认：已有文件、文件夹、URL、未知扩展名/Open With、缺失目标/本地回退，以及从
Explorer 环境启动的关联程序不继承 DeskBox 特有环境变量。AOT 应用本阶段仍不启动；完整
AOT 运行矩阵保留到阶段 5。

## 7. 4D-4B 后的模块状态

4D-4B 随后在同一 DLL 中新增独立 Quick Access v1 导出，没有修改本 Explorer 启动请求、
结果、能力位或产品回退。完整模块当前为 ABI 2、能力 255、九个必需导出；配置 21 /
schema 18 继续确认 Explorer 启动与完整 `always-throw` 为 0。新边界见
`quick-access-native-abi-v1.md` 和 `aot-stage-4d-4b-report.md`。
