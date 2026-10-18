# DeskBox 音乐音量 Rust 原生边界与 AOT 收口报告

## 1. 结论

阶段 4C 选择在现有 `deskbox_native.dll` 中增加粗粒度 Rust Core Audio 服务，没有把
`IMMDeviceEnumerator`、`IAudioEndpointVolume`、`IAudioSessionManager2` 等接口逐个改写成
C# source-generated COM。普通 JIT 默认继续使用原有 C# `ComImport` 实现作为行为基准；
显式设置 `DESKBOX_MUSIC_VOLUME_BACKEND=rust` 时走 Rust，Native AOT 则在编译期只保留
Rust 路径，原有 coclass、接口和 `Marshal.ReleaseComObject` 代码全部排除。

这项选择的原因不是“COM 都应改成 Rust”，而是本功能天然适合单次调用、无长期对象跨界的
原生服务。C# 生成式 COM 需要同时建模 COM 激活、接口继承、字符串所有权和多组 Core Audio
接口；Rust `windows` crate 已提供相同系统契约。把四个产品操作收在一个 C ABI 后，托管侧
只传 UTF-16 身份、操作和音量，不接触 COM 指针，也不需要为 AOT 保留运行时 COM marshalling。

## 2. 已冻结的既有行为

本阶段先按旧 C# 服务冻结以下语义，再在 Rust 中逐项复现：

- 每次调用重新取得默认 `eRender` / `eMultimedia` endpoint，不缓存设备或 session；
- 系统音量和应用 session 音量范围均归一化到 `0.0..1.0`，非有限输入按 `0.0` 处理；
- snapshot 中系统音量读取失败仍返回 `0.0`，session 未找到时返回
  `HasSessionVolume=false`；setter 失败返回 `false`；
- 排除系统声音 session；
- 身份文本转小写并只保留字母和数字，匹配片段至少 3 个 UTF-16 code unit；
- 匹配顺序保持为 session identifier 对 AUMID、instance identifier 对 AUMID、显示名、
  进程名、AUMID 包含进程名、session identifier 对来源显示名；
- 没有显式匹配时，仅在恰好存在一个有进程 ID 的非系统 session 时回退；
- 进程名继续按不含 `.exe` 的文件名参与匹配；
- 当前服务原本没有 endpoint 通知、session 通知或热插拔回调，4C 不新增长期回调生命周期。

## 3. 产品后端策略

| 构建/运行方式 | 音量后端 | 失败回退 |
| --- | --- | --- |
| 普通 JIT，未设置环境变量 | C# 旧实现 | 不适用 |
| 普通 JIT，`DESKBOX_MUSIC_VOLUME_BACKEND=rust` | Rust | 不回退 C#，保留诊断 |
| Native AOT | Rust | 不允许回退已被编译排除的 C# COM |

`MusicVolumeService` 的四个公共异步方法和 ViewModel 调用方式没有变化。服务仍通过
`Task.Run` 在工作线程执行同步 Core Audio 操作；后端选择只发生在服务内部。AOT 路径加载
固定应用目录下的 `deskbox_native.dll`，复用既有安全加载标志、ABI 探针和能力探针。

## 4. ABI v1

模块整体 ABI 版本仍为 2。本批是向后兼容的能力扩展，新增能力位 `1 << 5`，完整模块能力
掩码由 31 变为 63，并新增导出：

```text
deskbox_music_volume_v1
```

新导出使用独立的 v1 请求和结果 envelope：

| 结构 | x64 固定尺寸 | 结构版本 |
| --- | ---: | ---: |
| `DeskBoxMusicVolumeRequestV1` | 88 字节 | 1 |
| `DeskBoxMusicVolumeResultV1` | 104 字节 | 1 |

请求支持四个操作：

| 值 | 操作 |
| ---: | --- |
| 1 | 同时读取系统音量和匹配到的 session 音量 |
| 2 | 读取系统主音量 |
| 3 | 设置系统主音量 |
| 4 | 设置匹配到的 session 音量 |

请求携带两个显式长度的只读 UTF-16 字符串和一个 `double` 音量。字符串最大 32,767 个
UTF-16 code unit，不允许嵌入 NUL；空字符串必须表示为空指针和零长度。所有 flags 与
reserved 字段必须为 0。结果记录状态、原始 operation HRESULT、COM/对象创建/设备/系统/
session HRESULT、尝试阶段、匹配种类、归一化音量和布尔结果。返回状态必须等于
`result.status`；托管侧还会校验 envelope、布尔值、reserved 字段和音量范围。

Rust 每次调用在当前线程执行 `CoInitializeEx(COINIT_MULTITHREADED)`。`S_OK` 与 `S_FALSE`
按对应次数调用 `CoUninitialize`；`RPC_E_CHANGED_MODE` 表示复用调用方已有 apartment，不错误
撤销其 COM 初始化。Core Audio 返回的 task-allocated 字符串在复制后由 `CoTaskMemFree`
释放，所有 COM 接口均在调用结束时由 Rust RAII 释放，任何 COM 指针或 Rust 所有权不会
跨越 C ABI。

## 5. 代码与审计门槛

实现涉及：

- `native/deskbox-native/src/music_volume.rs`：Core Audio 操作、session 匹配和资源释放；
- `native/deskbox-native/src/lib.rs` 与公开头文件：能力、结构、校验和导出；
- `MusicVolumeNativeBackend.cs`：固定路径加载、能力/导出检查、托管结构与结果校验；
- `MusicVolumeService.cs`：JIT oracle、显式 Rust 和 AOT 编译期路由；
- Rust 构建验证、AOT 审计摘要和契约测试：要求能力 63、七个导出和音乐音量
  `always-throw` 为 0。

审计配置 14 / schema 11 将剩余 `always-throw` 集合固定为空，并在摘要中分别记录 shortcut
和音乐音量集合。当前仍保留 12 类既有 AOT/XAML/普通编译警告；4C 只清除最后一个传统
COM coclass 硬阻断，不把其他 dynamic、COM marshalling、trimming 或 XAML 问题混入本批。

## 6. 验证边界与后续人工项

自动化覆盖 Rust 格式/Clippy、纯匹配和归一化单元测试、无效 envelope/操作/嵌入 NUL、
托管与原生结构尺寸、后端选择、实际 DLL 导出与只读 Core Audio ABI 探测、C# 产品调用链、
完整 x64 测试和隔离 AOT 产物审计。隔离 AOT 产物仍不启动，保持到阶段 5 的干净用户或
虚拟机运行矩阵。

自动化没有主动改变用户当前系统音量，也不能制造真实播放器 session。因此在把 4C 视为
交互验收完成前，仍应使用显式 Rust JIT 模式人工确认：系统音量读取/设置、存在播放器时的
session 读取/设置、无匹配 session 时的 UI 状态，以及切换默认输出设备后的再次读取。
这些人工结果应与构建/AOT 结构证据分开记录。

最终代码复盘另外核对并收紧了以下边界：snapshot 在读取 session 前仍会第二次获取默认
endpoint；两个 setter 都传入有效的零 GUID 事件上下文；原生操作分派不再保留可触发 panic
的不可达分支；托管侧会拒绝未知 phase、未知 match kind、非布尔值、非零 reserved 与越界
音量；六条旧匹配分支、优先级、系统声音排除、`.exe` 去除以及 BMP/CJK/非 BMP 过滤均有
单元测试。常规 AUMID、进程名和显示名路径已覆盖；若实际播放器身份依赖非常规 Unicode
大小写数据库差异，仍应在真实 session 人工项中记录原始身份并做差分确认。

## 7. 后续状态

4C 完成后没有直接启动 AOT 主程序。配置 14 日志先冻结了 4D-0 清单，随后 4D-1A 已完成
三个无需改变 COM 边界的类型化入口：`Win32Helper` 使用泛型静态尺寸，
`MarkdownDocumentView` 强类型访问 Markdig `TaskList.Checked`，`SearchPopupWindow` 使用
现有推荐 DTO 代替匿名对象反射。配置 15 / schema 12 确认三个目标文件警告为 0；Rust ABI、
能力和音乐音量后端策略均未变化。

4D-1B 随后已完成 Quick Capture 异常诊断和 `Localized` 的有限类型映射，配置 16 /
schema 13 确认目标文件为 0 告警，原生模块仍未变化。完成后引用审计确认原计划 4D-2 的
`FileOperationHelper` 没有调用者，实际文件操作由 `FileService` 的另一套实现承担；4D-2
现已删除这段死代码，配置 17 / schema 14 把对应 IL2050 清零，没有向 Rust DLL 增加导出。
OLE DropTarget 因包含 Shell 回调、数据对象和窗口生命周期，拆为 4D-3A 读取侧与 4D-3B
注册/回调侧。4D-3A 已用窄 vtable 边界清除 `IDataObject`/`IStream` 内置 RCW，配置 18 /
schema 15 确认新读取层零告警；4D-3B 继续优先源生成 COM。XAML `WMC1510` 仍留给 4E，
AOT 主程序运行留给阶段 5。最新细节以 `rust-native-aot-roadmap.md` 和
`aot-stage-4d-3a-report.md` 为准。

4D-3B 随后已按上述方向完成，没有扩展本音乐音量 ABI。配置 19 / schema 16 将 OLE
注册侧 IL2050 清零，Rust 仍为 ABI 2、能力 63、七个导出。最新细节以
`aot-stage-4d-3b-report.md` 和总路线为准。

4D-4A 随后在同一个 DLL 中新增独立的 Explorer 托管启动 v1 导出；本音乐音量请求、结果、
能力位和产品策略均未改变。完整模块当前为 ABI 2、能力 127、八个必需导出；配置 20 /
schema 17 继续确认音乐音量与完整 `always-throw` 均为 0。新边界详见
`explorer-shell-launch-native-abi-v1.md` 和 `aot-stage-4d-4a-report.md`。

4D-4B 又在同一个 DLL 中新增独立的 Quick Access v1 导出；本音乐音量 ABI、能力位和后端
策略仍未改变。完整模块当前为 ABI 2、能力 255、九个必需导出；配置 21 / schema 18 继续
确认音乐音量与完整 `always-throw` 均为 0。新边界见 `quick-access-native-abi-v1.md` 和
`aot-stage-4d-4b-report.md`。

阶段 5B-3A 已使用 profile 33 / schema 30 的最终 x64 Native AOT 产物实际运行本 ABI 的两个
只读操作。产品系统 getter、产品 snapshot、直接原生 snapshot 及前后系统 getter 均读取到
`0.370000004768372`，原生 snapshot status/HRESULT 为 0、attempted phases 为 `0x1F`，系统
音量前后不变。本机当时没有匹配 session，因此验证的是 `HasSessionVolume=false`、match kind 0
与 session `S_FALSE` 路径；匹配 session getter 和两个 setter 仍未完成实际运行门槛。完整证据见
`aot-stage-5b-3a-report.md`。

阶段 5B-3B 随后使用 profile 34 / schema 31 的最终 x64 Native AOT 产物实际验证系统主音量
setter。产品 `TrySetSystemMasterVolumeAsync` 把原值 `0.370000004768372` 临时改为约 `0.42`，
直接原生 getter 只负责复查；应用内主动异常由 App `finally` 恢复，强制终止则由独立新 AOT
进程读取持久恢复意图，先观察到 `0.420000016689301`，再恢复原值。最终恢复意图已删除，正式
数据指纹不变。匹配 session getter 与 session setter 仍未完成，留给带可控媒体 session 的
5B-3C。完整证据见 `aot-stage-5b-3b-report.md`。

阶段 5B-3C 使用 profile 35 / schema 32 的最终 x64 Native AOT 产物和测试专用 Rust 静音音频
夹具，补齐匹配 session getter 与 session setter。夹具固定使用
`deskbox-audio-session-fixture` 进程/display name，产品 getter 与直接 Rust snapshot 均确认
match kind 4 和 `0x3F` 健康阶段；session 音量只通过产品 `TrySetSessionVolumeAsync` 完成
`1.0 → 0.92 → 1.0`。应用内主动异常由 App `finally` 恢复，强制终止则由独立新 AOT 进程先
观察到约 `0.9200000167` 再恢复。session 消失会保留恢复意图，不视为成功；系统主音量全程保持
`0.370000004768372`。夹具循环播放全零 PCM、随父脚本退出且不进入产品 publish，也不控制用户
播放器。生产 ABI 仍为 2、能力 255、九个必需导出。完整证据见
`aot-stage-5b-3c-report.md`。
