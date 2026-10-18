# 拖到快捷方式上：用该程序打开

- 日期：2026-09-12（2026-09-11 初版评审后整体改版，方案 B 定稿；同日完成两轮拖拽体系审计与修复批次 0/1/2；同日按实机反馈把"拒绝→回退导入"改为"拒绝→消费+提示"（§10.1），再按第二轮实机把"委托 shell drop 处理器"换成"ShellExecuteEx 于 .lnk + 参数"（§10.2））
- 状态：**已实现，测试全绿；launch 机制已用脚本实测（Obsidian 被拉起），格子外拖入已由 Simon 真机确认可打开；格子内互拖为 2026-09-12 追加的 S2**
- 触发场景：把一张图片从资源管理器拖到格子里 Photoshop 的图标上 → 用 PS 打开这张图（Windows 桌面的原生语义）。现状是文件被**导入进格子目录**。

## 0. 定案摘要

**v1 直接采用方案 B：把 drop 委托给 shell 自己的 IDropTarget**（`SHCreateItemFromParsingName` → `BindToHandler(BHID_SFUIObject, IID_IDropTarget)` → `DragEnter`/`Drop`）。这就是 Explorer 拖到快捷方式上时跑的同一代码路径，语义零漂移：

| Explorer 语义 | 谁负责 |
|---|---|
| 目标是 exe → 启动并附带拖入文件为参数 | shell exe drop handler |
| 目标是普通文档 → 不接受拖放（DragEnter 返回 DROPEFFECT_NONE） | shell（无需自维护可执行清单） |
| 快捷方式自带 arguments → 拼接进最终命令行 | shell |
| 目标已移动 → 链接跟踪解析 | shell |
| 快捷方式的"起始位置"、提权标记、AUMID 打包应用走 file contract | shell |

被否决/降级的备选：
- **方案 A（解析目标 + 本地启动传参）**：必须解决 `.lnk` 传参不一致（社区长期报告）→ 落地为"解析目标直接启动"，丢失提权标记/链接跟踪/AUMID，且引入手写命令行构造（转义是本类功能最难查的缺陷源）。**已废弃，不写任何 `OpenWithAsync`**。委托绑定失败的兜底 = 维持今天的导入行为，零新增代码。
- **方案 C（Explorer 托管 ABI 加 `arguments`）**：环境继承有真实价值但只有这一点，且 AUMID/文档语义它不管。留作远期可选；`ELECTRON_RUN_AS_NODE` 险情在 v1 用环境清理覆盖（见 §4）。

**v1 明确不做**（同初版，红线不变）：

| 不做 | 理由 |
|---|---|
| 拖到**文件夹快捷方式**上时转移到其目标文件夹 | Explorer 的语义是移动/复制——会移动用户文件。执行方式：谓词挡掉、**永不委托**，维持导入 |
| `.url` / shell 命名空间类快捷方式 | 没有"用它打开文件"的语义 |
| 格子**内部互拖**也触发 | **已实现（2026-09-12 实机反馈后）**：拖到快捷方式格子=用该程序打开拖动的那些文件，条目留在格子里（不产生传输、不重排）。叠放弹层内的成员拖动仍排除（`IsStackPopoverMemberDrag`）——它的路径语义是叠放成员关系，不是"要打开的文件" |
| 叠放弹层内的快捷方式格子 | 弹层有独立 drop 清理逻辑，v1 只覆盖主网格；待定是否同规则 |

## 1. 现状与插入点（代码证据）

拖入是**双路径竞速**架构，两个入口都必须接线，缺一则该场景退回导入：

- **原生 OLE 路径**（外部拖入主力）：`Helpers/NativeDropTarget.cs` 经 `RegisterDragDrop` 注册在窗口与 `DesktopChildSiteBridge` 上。`OnDrop` 拿到原生 `IDataObject` 后释放并走 `DropIntentEvent` → `ContentWidgetWindow.NativeDragDrop.cs` 的 `ImportNativeFileDropAsync` → 导入。命中判定 `NormalizeNativeFileDropItemTarget`（`NativeDragDrop.cs:1299`）**只认叠放/文件夹**，快捷方式目前不可见。
- **XAML 路径**：`ItemSurface_DragOver/Drop`（`FileSurfaceContent.ItemVisuals.cs:159/268`）只认文件夹（`TryGetFolderDropTarget:856`），其余落到 `Root_Drop`（`xaml.cs:2237`）导入。两边靠 `_lastXamlFileDropUtc` 500ms 时间窗 + 120ms 延迟和解。
- **高亮机制现成**：`_folderDropTarget` + `ApplyNativeFolderDropTarget`（native 观察链 `ObserveNativeDragPointer`）+ `SetFolderDropTarget`/`ClearFolderDropTarget`（XAML 事件链）。
- **drop description 现成**：`CreateNativeFileDropDescription`（`NativeDragDrop.cs:245`）+ `NativeDropDescriptionWriter` 今天就在写"复制到 xxx"；"用 X 打开"是零新设施。
- **可复用判定件**：`WidgetItem.IsShortcut`/`TargetPath`、`ShortcutHelper.IsShellLinkPath`/`ReadStoredMetadata`（长度+mtime 缓存）、`ShortcutTargetProbe.Classify`。

## 2. 行为规则

共享判定件 **`ShortcutLaunchPolicy`**（纯函数，可单测；三个调用点共用：XAML 谓词、native 命中归一化、drop description）：

```
Evaluate(isShortcut, path, targetPath, targetIsDirectory) → Launch | None
  1. isShortcut && IsShellLinkPath(path)          // .url 挡在外面
  2. Classify(ExpandEnvironmentVariables(targetPath)) == LocalFileSystem
  3. !targetIsDirectory                            // 文件夹快捷方式红线，永不委托
```

- **不做**目标存在性短路：`File.Exists` 为假（存储目标缺失）不排除——链接跟踪可能救回（与 2026-09-12 点击打开行为一致：交给 Windows）。委托失败 shell 自己报错。
- **不做**可执行判定：悬停高亮按谓词给（体现"这是投放目标"），shell 在 `Drop` 时做权威裁决；shell 拒绝（`DragEnter` → DROPEFFECT_NONE）或启动失败 → **消费该次拖放**：OLE 层返回 `DROPEFFECT_NONE`（拖拽源因此也不删自己的文件）、不建传输会话、不导入，改为 Warning 提示 `Widget.DropOnShortcutCannotOpen`（zh-CN："{0}" 无法打开此文件）。**导入会把用户文件搬进格子目录，而手势意图是"用该程序打开"，两者不匹配**——2026-09-12 实机反馈后推翻 v1 的"回退导入"决定（原话：文件不支持该打开就走了移动，不该移动，应提示）。
- 保留的偏差：悬停高亮与 drop description 在 `Drop` 前无法预知目标是否接受（预知需在 DragOver 热路径上做 COM 探测，与 F3 的"热路径 stat"同类，已否决），所以"先高亮、松手才提示"。
- 仍未覆盖：**未被识别为 launch 目标的 `.lnk` 格子**（存储目标读不出/相对路径/shell 命名空间 URI）→ 谓词判 `ItemDropBehavior.None` → drop 落到根导入（移动）。与本次修的是两条不同路径，触发条件见 §10。- 命中后：高亮条目（独立的 `_launchDropTarget` 机制，与文件夹/叠放同用中性悬停面，视觉状态见 §11 发现 6）+ drop description 显示"用 X 打开"。
- Drop：`e.Handled = true`，**不启动任何传输**——不调 `BeginTrackedImport`、不建会话；成功给 `Widget.DropOnShortcutLaunched` 反馈。
- 多文件：全部交给 shell（与 Windows 一致；命令行长度上限与 Explorer 相同）。

## 3. 委托层

**`Helpers/ShellDropDelegator.cs`**：

- `SHCreateItemFromParsingName(lnkPath, NULL, **IID_IShellItem**, &item)`（`[LibraryImport]`，AOT 安全）→ `IShellItem::BindToHandler(NULL, BHID_SFUIObject, IID_IDropTarget, &target)`。**IID_IShellItem = `{43826d1e-e718-42ee-bc55-a1e261c37bfe}`**（该参数的语义是"要返回哪个接口"，不是 bhid —— 传错会得到 `E_NOINTERFACE`，见 §10.1 缺陷 1）；BHID_SFUIObject = `{3981e225-f559-11d3-8e3a-00c04f6837d5}`（早期草稿与本文件曾把值写错，见 §10.1 缺陷 2）；IID_IDropTarget = `{00000122-0000-0000-C000-000000000046}`。同项上 `BindToHandler(BHID_SFUIObject, IID_IShellLink)` 也成功，可用来确认拿到的是 .lnk 自身的 UI 对象。
- COM 接口用 **`[GeneratedComInterface]`**（AOT 安全，代码库已有 `ComInterfaceMarshaller` 先例）。`IShellItem` 只声明 `BindToHandler`（vtable 首位）；`IDropTarget` 四方法按 oleidl 顺序 `DragEnter/DragOver/DragLeave/Drop`，数据对象参数走 `nint` 原指针。
- `TryDelegateDrop(lnkPath, dataObjectPtr, keyState, screenX, screenY, allowedEffects, out effect)`：DragEnter → Drop，返回 shell 是否接受。**包一层 `ELECTRON_RUN_AS_NODE` 临时清理**（try/finally，同 `Win32Helper.OpenFileOrChooseApp`）——shell handler 在我们进程里 ShellExecuteEx，继承本进程环境。
- 线程约定：必须在使用该 `IDataObject` 的同一 STA 线程调用（native 路径 = OLE 回调线程；XAML/右键路径 = UI 线程构建后当场用）。

**`Helpers/ShellDataObjectBuilder.cs`**（XAML 路径与右键菜单用；native 路径用真实 `IDataObject` 不需要）：

- `SHCreateDataObject` 创建空 shell 数据对象 → `SetData` 一个 `CF_HDROP` HGLOBAL（`DROPFILES{pFiles=sizeof, fWide=1}` + 双 NUL 结尾宽字符路径表）。HDROP 是路径列表不是命令行——**不存在转义问题**。
- 初始用途：右键拖拽选"用 X 打开"（OLE 回调后菜单是异步的，真实对象已释放）、XAML 路径 Drop（路径来自 `GetSurfaceDropFilesAsync` 已物化的真实文件）、S2 内部互拖。

## 4. 双路径接线

| 站点 | 改动 |
|---|---|
| `ShortcutLaunchPolicy`（新） | §2 判定件；`targetIsDirectory` 由调用方 stat |
| `NativeDropTarget.OnDrop` | 释放前 `AddRef` 真实 `IDataObject`，`NativeDropIntentEventArgs` 增加 `RawDataObject`（init 属性，非位置参数）；事件返回后由消费者 `Release` |
| `ContentWidgetWindow.NativeDragDrop` | 命中归一化拆成两路：`_nativeFileDropItemTarget`（叠放/文件夹，导入语义不变）+ `_nativeFileDropLaunchTarget`（快捷方式）；`DropIntentEvent` 同步（OLE 线程）优先委托；成功 → 跳过 `ScheduleNativeFileDropFallback`、置 `_lastXamlFileDropUtc` 抑制任何迟到导入、清理临时物化文件改为延迟（进程可能懒读）；失败 → 原导入流程 |
| `CreateNativeFileDropDescription` | launch 目标命中 → "用 X 打开"（`%1` 占位，走既有 `ToShellDropDescriptionMessage`） |
| 右键拖拽 flyout | 命中 launch 目标时首项加"用 X 打开"；选中 → 用构建的 HDROP 数据对象委托 |
| `FileSurfaceContent.ItemVisuals` | `ItemSurface_DragOver/Drop` 增加 launch 分支（在文件夹谓词之后、任何导入之前）：高亮 + `e.Handled`；Drop 用构建的 HDROP 委托，成功后同样通知窗口置抑制时间戳 |
| 视觉状态 | 复用 `_folderDropTarget` 字段承载；若 `FileItemSurfaceVisualState` 加 `LaunchTarget` 状态廉价则加（与文件夹高亮区分），否则 v1 共用 DropTarget 视觉，由 drop description 承担语义区分 |

## 5. 需要新增的本地化键（12 语言，契约测试会强制补齐）

| 键 | 用途 | 占位符 |
|---|---|---|
| `Widget.DropOnShortcutLaunched` | 成功反馈（zh-CN：已用该程序打开） | 无 |
| `Widget.DropOnShortcutOpenWith` | drop description（zh-CN：用 "{0}" 打开） | `{0}` ×12 一致 |
| `Widget.DropOnShortcutCannotOpen` | 拒绝/失败反馈（zh-CN：`“{0}” 无法打开此文件`） | `{0}` ×12 一致 |

名字缺失的病态条目回落到既有 `Widget.OpenItemFailed`。拒绝与"委托不可用"用同一句提示；两者的区分只进日志（`ShellDropLaunchOutcome` + `[ShortcutLaunch]` 行），需要时再拆文案。

## 6. 测试

1. **ShortcutLaunchPolicy（纯函数）**：`.lnk`→exe 文件目标 = Launch；目标为目录 = None（红线）；`.url` = None；目标缺失（isDirectory=false）= Launch（链接跟踪交 shell）；环境变量目标展开。
2. **HDROP 构建器**：单/多文件、含空格、**含尾反斜杠目录路径**（HDROP 无转义，直接字节校验）、双 NUL 终止。
3. **不产生传输**：launch 命中后无新传输会话、导入忙标志未占用（XAML 分支在 `BeginTrackedImport` 之前返回）。
4. **回归**：拖到文件夹条目、空白处行为不变。
5. **AOT smoke**：`FileSurfaceContent.AotNativeDropSmoke` 若引用并行字段需同步；新增 `GeneratedComInterface` 走既有 AOT 护栏验证。

## 7. 真机验证矩阵（等价 spike 验收，Simon 手动）

| 用例 | 预期 |
|---|---|
| 拖图到 Photoshop 快捷方式格子 | PS 打开图片，格子无导入 |
| 拖图到记事本文档快捷方式格子（应用不接受该文件类型） | **Warning 提示"X"无法打开此文件；不导入、源文件留在原处**（2026-09-12 改） |
| 拖图到文件夹快捷方式格子 | 导入（红线不变） |
| 拖图到 `.url` | 导入 |
| 拖图到 UWP 应用快捷方式（AppsFolder 来的） | 应用被 file contract 激活 |
| 多选拖到 PS | PS 收到全部参数 |
| 拖图到空白处 / 文件夹格子 | 行为与今天一致 |
| 右键拖到 PS 格子 → 选"用 X 打开" | 启动，无传输 |
| 右键拖到 PS 格子 → 选"用 X 打开"但应用拒绝 | Warning 提示，不导入（不再是"回退复制"） |
| 拖拽悬停 | 高亮 + drop description "用 X 打开" |

## 8. 分片

| 片 | 内容 | 状态 |
|---|---|---|
| **S1** | 判定件 + 委托层 + 双路径接线 + §5 键 + §6 测试 | 实现中 |
| **S2** | 内部互拖触发（判定件复用：`ShortcutLaunchPolicy.EvaluateInternalDrag`；无传输、无重排） | **已实现** |
| **S3（可选）** | AUMID 激活实测打磨；Explorer 托管 ABI（方案 C）再评估 | 暂缓 |

## 9. 拖拽体系审计修复（2026-09-12，批次 0/1/2 已落地）

两轮只读审计后按批次修复（3396/3396 全绿）：

| 项 | 修复 |
|---|---|
| S1 越界读 | `ReadVirtualFileDescriptors` 现按 `GlobalSize` 钳制声明 count（`NativeDropTarget.cs`），+2 测试 |
| F1 缺 deferral | `TryHandleLaunchTargetDropAsync` 在物化 await 前取 `GetDeferral`，finally Complete |
| F2 双委托 | `_lastNativeLaunchConsumedAt` 闩锁：native 委托成功即在 UI 线程标记，XAML 分支 1s 窗口内识别为已消费并压制（含 `[DragProtocol] launch double-consumption suppressed` 检测日志） |
| F3 热 stat | `ShortcutLaunchPolicy.EvaluateItem` 先 `Classify`（纯字符串）后 stat，UNC/网络目标不再触发 UI 线程探测；**故意不缓存**目录判定——陈旧的"非目录"会击穿文件夹快捷方式红线 |
| 判定件收敛 | 新 `Services/ItemDropBehaviorPolicy`（None/FolderImport/StackImport/Launch），XAML 两谓词、native 命中归一化、`NormalizeNativeFileDropItemTarget` 全部改走它，+6 测试 |
| S3 MOTW | 新 `Helpers/ZoneIdentifierWriter`（写 `Zone.Identifier`，ZoneId=3+HostUrl）；接入浏览器 FileContents、bitmap/StorageFile、URL 物化四点；我们自己生成的 AppsFolder .lnk 不标 |
| S2 配额 | 每次拖拽 256MB 物化预算：COM 流限长拷贝（`NativeComStreamReader.CopyTo` 重载）、HGLOBAL 预检、web 下载改 `ResponseHeadersRead` 流式+中流熔断+半成品清理；QuickCapture 的 `SkippedCount` toast 搭现车，未新增本地化键 |
| 2-2 检测 | 双导入疑似（`NoteImportStartedForProtocol`，1.5s 窗口，只记不拦）+ 拖出对账超窗残留摘要（`[DragProtocol] drag-out watch expired`） |

**未修（有据不修）**：S4 UIPI 放宽（拖放应用标准取舍）、S5 内部 token 无认证（伪造者只影响自己的 drop）、C5 vtable 槽位 ABI（稳定契约）、N6 拖拽缓存无失效（传输层兜底）、N2 出向误删行（推断模型固有，已加检测）。
**后续**：批 3 按需（bridge 注册失败用户反馈、RDP/极速拖拽/SHCreateDataObject NULL 父目录真机项）。

## 10. 拒绝即消费（2026-09-12，实机反馈后）

**问题**：拖文件到应用图标上、应用不接受该文件类型时，原设计"回退导入"会按 `ManagedDropAction`（默认 Move）把文件**移动**进格子目录——手势意图是"用该程序打开"，与移动无关。Simon 反馈：不应该移动，应该提示应用无法打开。

| 改动 | 位置 |
|---|---|
| 结果四态化 `ShellDropLaunchOutcome`（NotAttempted/Launched/TargetRefused/LaunchFailed/**AlreadyResolved**）+ 共用判定件 `ShortcutDropOutcomePolicy`（`Consumed` / `ShouldContinueAsImport`） | `Helpers/ShellDropDelegator.cs` |
| 已消费的 drop 在 **OLE 层**返回 `DROPEFFECT_NONE`：不进 `DropIntentEvent`、不排导入，拖拽源也不删自己的文件 | `Helpers/NativeDropTarget.cs` |
| 原生路径：拒绝/失败 → 清目标、置 `_lastXamlFileDropUtc` 抑制迟到导入、清临时物化文件、Warning 提示 | `Views/ContentWidgetWindow.NativeDragDrop.cs`（`HandleNativeLaunchDrop`） |
| 右键"用 X 打开"失败：同样提示，不再"回退复制" | 同上（`ExecuteRightButtonLaunchChoice`） |
| XAML 路径：拒绝 → `e.Handled = true` + `AcceptedOperation = None` + 标记闩锁（防另一路径重复委托/重复提示）+ 提示 | `Controls/WidgetContents/FileSurfaceContent.ItemVisuals.cs` |
| 新增反馈方法 `ShowShortcutLaunchRefusedFeedback`（Warning 级，名字缺失回落 `Widget.OpenItemFailed`） | `Controls/WidgetContents/FileSurfaceContent.xaml.cs` |
| 新键 `Widget.DropOnShortcutCannotOpen` ×12 语言 | `Strings/*.json` |
| 测试：`ConsumedShortcutDrops_NeverFallBackToImport`（钉住"拒绝绝不回退导入"）+ `RefusedShortcutDrop_ReportsTheApplicationThatCouldNotOpenIt` | `tests/DeskBox.Tests/ItemDropBehaviorPolicyTests.cs` |

### 10.1 第二轮：实机复现后查出委托层三个缺陷（同日）

第一次实机验证失败（"没打开的时候还是走了移动"），日志与独立探针定位到三个**互相独立**的缺陷：

| # | 缺陷 | 证据 | 修法 |
|---|---|---|---|
| 1 | `SHCreateItemFromParsingName(path, 0, riid, out item)` 的 `riid` 传了 **BHID**，而该参数要的是**返回接口的 IID** | 日志 `hr=0x80004002`（E_NOINTERFACE）——**委托从未成功过**，每次 drop 都走"委托失败"分支 | 改传 `IID_IShellItem = {43826D1E-E718-42EE-BC55-A1E261C37BFE}`，之后 `BindToHandler(BHID_SFUIObject, IID_IDropTarget)` |
| 2 | `BHID_SFUIObject` 常量 GUID 后缀写错（`...8e3a-00C04FC9E26E`） | 独立探针：新值 `{3981e225-f559-11d3-8e3a-00c04f6837d5}` 绑定成功 `hr=0x0`；旧值 `hr=0x800401E5`（MK_E_NOOBJECT） | 常量改为公开值 |
| 3 | **第三个 drop 入口**：`WM_DROPFILES` 子类化路径（`DragAcceptFiles` 开启）无条件 `QueueNativeFileDropImport` | 日志 `stage=NativeDropFiles` 紧跟拒绝之后仍 `[Import] ... move=True`——即使 OLE 层已正确拒绝，文件还是被移动 | 入口改为先查"本次手势是否已被 launch 消费"（闩锁），命中即消费不导入 |
| 4 | `ShellDataObjectBuilder.DataObjectIid` 少了位数字：`0000000E-…`（正确 `0000010E-0000-0000-C000-000000000046`） | 临时探针：`SHCreateDataObject` 日志 `hr=0x80004002`；独立探针同调用传正确 IID 则 `hr=0x0`，`SetData(CF_HDROP)` `hr=0x0`，`QueryGetData` `hr=0x0`。**所有"合成数据对象"路径（XAML 路由 drop、右键 launch、内部互拖）一并失效** | GUID 改为 `0000010E…`（拖出方向 `NativeShellFileDragProvider` 一直用的是正确值） |
| 5 | 用**合成 CF_HDROP** 委托给快捷方式 drop 处理器，对 exe 目标会长时间阻塞 | 临时探针（自建 .lnk + 记事本目标 + 进程检测）：文档目标 13.8ms 干净拒绝（`E_NOTIMPL`，符合预期）；exe 目标 DragEnter 通过后 Drop 耗时 2.1s 且 `effect=DROPEFFECT_NONE`；换成记事本目标后**整通调用挂死 >2 分钟**，记事本进程从未出现（即未启动） | 见下方"决定"：合成对象路径不再委托 |

**探针证据（独立进程 + 仓库内临时测试，均已删除）**：修好后 `BindToHandler(IDropTarget)` 返回 `hr=0x0` 且指针非空；同一 `.lnk` 项上 `BindToHandler(IShellLink)` 也成功，**证明拿到的是 .lnk 自己的 drop 对象，而不是父文件夹的视图投放目标**（即不存在"把文件移进文件夹"的危险分支；该分支另外还有"文件夹快捷方式永不委托"的策略红线兜底）。

**决定（缺陷 5）**：合成数据对象（只有 CF_HDROP 一种格式）不足以让 shell 的快捷方式处理器真正启动程序，且会长时间阻塞——**在 UI 线程上做这件事等于引入挂死风险**。因此：

- **保留委托的只有原生 OLE 路径**——它持有拖拽源的真实 `IDataObject`（Explorer 给的多格式对象），这才是与 Explorer 语义等价的那条路。
- 我新增的 `WM_DROPFILES` 分支**收回为只做"闩锁抑制导入"**，不委托（原设计里的 XAML 路由 drop 分支仍保留委托，属既有实现，**但其阻塞风险由此得到证据，建议一并评估**）。

## 10.2 机制更替：不再委托 shell 的 drop 处理器（2026-09-12 实机）

修复 §10.1 的 1/2/4 之后委托**确实执行了**，但真机结果是"不能打开"：

```
[20:34:31.726] [DropTarget] NativeDrop received count=1
[20:34:35.647] [ShortcutLaunch] Delegated drop on 'E:\DeskBox\AI工具\Obsidiand.lnk' hr=0x00000000 effect=0 handled=False.
[20:34:35.650] [DropDiagnostic] stage=NativeDropFiles count=1
```

- `Drop` 耗时 **3.9 秒**后返回 `effect=0`（DROPEFFECT_NONE），即 shell 的快捷方式 drop 处理器**拒绝了这次 drop**；
- 随后 Windows 自己弹出 **"打开方式"选择器**（截图确认：该菜单不是我们的文案——我们的键是「用 “{0}” 打开」）；用户明确不要这个弹窗；
- 好消息：`stage=NativeDropFiles` 之后**没有** `[Import]` 行 → §10.1 的闩锁生效，**文件没有被移动**。

补充事实（探针 + 脚本实测）：该 `.lnk` 指向 `D:\Obsidian\Obsidian.exe`（无参数、目标存在）；被拖的是 `deskbox插件化复盘文章-雷军重写版.md`，其默认关联是 MarkText（Electron）。`ELECTRON_RUN_AS_NODE` 假说已排除（当前环境干净）。

**结论：原设计前提被证伪。** "`BindToHandler(BHID_SFUIObject, IID_IDropTarget)` 就是 Explorer 拖到快捷方式上跑的同一代码路径"不成立——Explorer 的"拖到快捷方式=用该程序打开"是它**视图层**的行为；通过 .lnk 的 `GetUIObjectOf(IDropTarget)` 拿到的处理器对这类 drop 会拒绝并触发系统"打开方式"UI，而这个 UI 我们无法从委托内部抑制。且在 UI 线程上调用它还有阻塞风险（§10.1 缺陷 5 实测 >2 分钟）。

**新机制**：`Helpers/ShortcutFileLauncher.TryLaunchWithFiles(lnk, droppedPaths)` —— 对 `.lnk` 执行 `ShellExecuteEx`，把拖入路径作为**参数**追加（`ProcessStartInfo(lnkPath, quotedPaths) { UseShellExecute = true, Verb = "open" }`），由 shell 解析链接：

| 保留的链接语义 | 由谁负责 |
|---|---|
| 快捷方式自带 arguments 前置拼接 | shell（`ShellExecuteEx` 于 .lnk） |
| 起始位置目录、提权标记、链接跟踪 | shell |
| 拖入路径的引号 | 我们（`BuildArgumentString`：**每个路径一律加引号**；Windows 路径不可能含 `"`，故这是全部转义逻辑，并已加单测钉死） |

**为什么不用 `ProcessStartInfo.ArgumentList`（免转义的参数列表）？** 实测（2026-09-12）：`UseShellExecute = true` 时 .NET **静默忽略** `ArgumentList`——进程照样启动、参数被丢掉、不抛异常（用 cmd 写标记文件验证，标记未生成）。而 `UseShellExecute = false` 时 `.lnk` 不是可执行映像、根本起不来。所以这条 ShellExecuteEx 路线只能传原始参数字符串，转义必须自己做。

**安全复查结论（Mimosa L2 报"命令注入"，判定为误报）**：
- 这不是 shell 命令行：没有任何 `cmd /c`，拖入路径走的是 `ShellExecuteEx` 的 `lpParameters` 通道，最终作为参数交给**快捷方式指向的那个应用**。
- 转义完备：每个路径**一律**包在双引号里，而 `"` 是 **Windows 文件名/文件夹名的保留字符**（[MS: Naming a File](https://learn.microsoft.com/en-us/windows/desktop/fileio/naming-a-file)），因此任何**能真实存在**的路径都无法提前结束引号；`&`、`|`、`>`、`^`、空格都因落在引号内而成为该参数的普通字符。为防止"路径从未接触过文件系统"的伪造值，`SanitizeDroppedPaths` 会**丢弃任何含 `"` 的路径**（该不变式由代码强制，并有单测）。
- 残留（如实记录，非本次引入）：若快捷方式的目标本身是命令解释器（cmd/bat），其参数会被解释器再解析一次，`%VAR%` 在引号内仍会展开。这与 **Explorer 直接把文件拖到此类快捷方式上**的行为一致，属于"按快捷方式启动"语义的固有面，除非放弃该语义否则无法消除。

实测（脚本，同一台机器、同一个 .lnk）：`lnk + 参数` → **Obsidian 被拉起并打开了该 .md**（进程 0→4）；文档目标的快捷方式 → 记事本打开该文档，**无"打开方式"弹窗**；两种都**不移动文件**。

调用点三处已全部切换（原生 OLE / XAML 路由 / 右键"用 X 打开"）；成功 → `Widget.DropOnShortcutLaunched`，失败 → `Widget.DropOnShortcutCannotOpen`，两条路径都**不导入**。原生路径成功时回给拖拽源的 effect 是 `DROPEFFECT_LINK`，因此移动型拖拽的源也不会删自己的文件。

**随之失效的代码（保留未删，待处置）**：`Helpers/ShellDropDelegator.cs`（`TryDelegateDrop`）与 `Helpers/ShellDataObjectBuilder.cs`（`TryCreateHdropDataObject`）现已无调用者。保留原因：它们记录了本次排查的全部结论、且 S3/AUMID 方向可能复用；`ShellDataObjectBuilderTests` 的字节级测试仍然通过。**要么删、要么留给后续路线，Simon 定。**

**已知取舍**：不再有"目标应用是否接受该文件"的预判（那正是刚才会弹"打开方式"的东西）。所以拖到**文档快捷方式**上会打开该文档（多出的参数被忽略），而不是旧矩阵写的"拒绝+回退导入"；拖到**文件夹快捷方式**上仍按红线走导入（策略层排除，不变）。若某天要恢复"文档目标拒绝"，可在启动前用 `DragEnter` 单独探一次（便宜、不弹 UI），但需重新验证。

**入口真相**：拖拽落地其实有**三个**入口——OLE `IDropTarget`（主力）、WinUI 路由的 XAML Drop、`WM_DROPFILES`。三处都必须做同一个 launch 判定，缺一即漏（此前文档写作"双路径"，是漏掉第三入口的根源）。三者用同一把闩锁互斥：谁先解决本次手势，谁负责提示，另两个只做"消费、不导入"。

**残留（更新）**：
1. **闩锁是 1 秒时间窗**（`MarkNativeLaunchConsumed` / `WasLaunchConsumedRecently`，`Stopwatch` tick + `Interlocked`，可从 OLE 线程写入）。作用是"同一次物理释放的重复投递去重"。副作用：1 秒内对同一格子的**第二次真实拖拽**会被静默跳过委托——但同文件同应用的结果是确定的，且这样能防"应用被启动两次"，取舍为接受。
2. **未被识别为 launch 目标的 `.lnk` 格子**（`ShortcutLaunchPolicy.EvaluateItem` 判 None：存储目标为空/相对路径/shell 命名空间 URI/UNC/网络盘）走 `ItemDropBehavior.None` → drop 落根导入（移动）。与本次修的是两条独立路径；`.url`、文件夹快捷方式、UNC 的导入语义是既有决定，不建议一并改。**判定方法**：复现后看日志有无 `[ShortcutLaunch] Shell refused drop on` 或 `[ShortcutLaunch] Delegated drop on` —— 有=launch 路径；都没有而文件被移动=本条残留。
3. 多文件时 shell 整体裁决（部分接受/部分拒绝不存在），与 Explorer 一致。
4. 悬停高亮与 drop description 仍无法预知目标是否接受（要预知需在 DragOver 热路径上做 COM 探测）。
5. `WM_DROPFILES` 分支只做闩锁抑制（不委托，见 §10.1 缺陷 5）。若某拖拽源只投递 `WM_DROPFILES` 而没有任何 DragOver，则该手势仍走导入（与今天一致）——因为该入口无法安全委托。
6. **"用该程序打开"这一半仍未在真机确认**：原生路径（左键拖拽）现在才第一次真正把 drop 交给 shell（此前因缺陷 1/2 必然失败），需 Simon 真机拖一次确认应用被启动。合成对象路径（右键菜单"用 X 打开"、XAML 路由）在修复缺陷 4 后能建出数据对象，但缺陷 5 表明其能否真正启动 exe 目标存疑；当前行为是"提示无法打开、不移动"（安全但可能误报）。

## 11. 格子内互拖（S2，2026-09-12 实机）

**需求**：格子里的文件拖到同一格子的应用快捷方式格子上，也要"用该程序打开"（Simon 反馈：格子外可以了，格子内不行）。

**第一个发现（判定层）**：`TryHandleLaunchTargetDragOver` 原本把 `IsInternalReorder` 一律放行给重排。内部的载荷已经带着真实路径（`FileItemDragPackage` 写入 `DeskBoxSourcePaths` + `DeskBoxInternalDragToken`），所以只需把判定放宽为 `ShortcutLaunchPolicy.EvaluateInternalDrag(isDeskBoxFileDrag, isStackPopoverMemberDrag, paths)`：内部拖拽+有路径=`Launch`，叠放弹层成员拖动仍排除。拖放命中后按文件夹瓦片同契约 `PersistSurfaceReorder()` 取消重排预览。

**第二个发现（投递层，实测）**：判定改对之后仍然"没反应、无提示"。加 5 个日志点实测（2026-09-12 21:24 / 21:39）得到：

```
[DragProtocol] item-surface dragover ... folderTarget=False launchTarget=True   ← 瓦片 DragOver 正常触发、判定为 Launch
[DragProtocol] launch-target dragover ... internalLaunch=True ...              ← 判定件取值为 Launch
（松手前 23ms 还有一次落在瓦片上的 DragOver）
[DragProtocol] stage=SourceCompleted ... dropResult=None                        ← 直接到源完成
（没有任何 item-surface drop-enter 行）
```

**WinUI 不把路由 `Drop` 投递给条目表面**——当拖拽源是**同一个 ListView** 时，`DragOver` 正常下发到子元素，`Drop` 却停在列表层。所以 Drop 侧那条分支（§10.2 的三处调用点之一）对内部拖拽是死代码。这不是我们代码的 bug，是框架行为，无法通过标记 `e.Handled`/`AcceptedOperation` 改变。

**修法**：改从可靠出口 **`DragItemsCompleted`** 兜底（它对内部拖拽必触发）：

| 判定 | 依据 |
|---|---|
| 手势确实落在快捷方式格子的图标上 | 最后一次接受 launch 的 DragOver 记录下瓦片与容器边框（`_internalLaunchHoverItem` / `_internalLaunchHoverBorder`，左键离开该瓦片即清空） |
| 表面没有接管这次拖拽 | `DropResult == None`（内部重排被接受时是 `Move`，跨格子落在同一表面是 `Link`） |
| 松手时指针仍在图标上 | **松手瞬间重读活几何**：经记录边框找到图标宿主（校验 `DataContext` 未被容器回收顶替），`GetCursorPos` 对图标矩形（±6px 余量）做包含判定。不再使用"DragOver 记录点 + 半径"——实时重排会回收容器、把瓦片挪到静止指针下方，记录点必然过期 |

判定收在纯函数 `ShortcutLaunchPolicy.ShouldLaunchFromCompletedInternalDrag(dropResult, hoveredLaunchTarget, cursorInsideLaunchTarget)`（可单测），命中后 `TryLaunchInternalDragOnShortcut` 打闩锁、给提示、**丢弃**待提交的重排预览（不重排、不移动）。

**第三个发现（接受操作那道门，同轮定位）**：判定放宽后仍然"没反应"，因为 `TryHandleLaunchTargetDragOver` 里选择接受操作的代码只认 `Link`/`Copy`：

```
e.AcceptedOperation = AllowedOperations.HasFlag(Link) ? Link : HasFlag(Copy) ? Copy : None;
if (e.AcceptedOperation == None) { ClearFolderDropTarget(); return; }   // ← 内部拖拽每次都命中
```

内部拖拽是 ListView 重排、拖拽源只授权 **Move**，所以每次都在这里提前返回——**记录没写、提示没出、松手不触发**。修法：内部 launch 用源实际允许的 `Move` 接受（外部仍 advertised Link 不变）。之所以敢用 Move：`ShouldCommitReleasedSurfaceReorder` 带 `!hasActiveChildDropTarget` 条件，瓦片成为子投放目标时松手**不提交重排**（`FileSurfaceContent.xaml.cs`）。

**第四个发现（假 DragLeave）**：中途曾把"离开瓦片就清记录"挂在 `ItemSurface_DragLeave` 上，结果实时重排预览**回收/重建瓦片容器**会在指针没动的情况下触发 `DragLeave`，记录被清掉。改成与文件夹分支同款守卫：`!IsPointerInsideDropElement(launchBorder, e)` 才清。

**第五个发现（拖自己）**：把快捷方式拖去排序、又拖回原位时，**它自己是 launch 目标**→ 松手"用该快捷方式打开它自己"，目标程序报"文件可能已损坏"（Simon 实机）。修法：`ShortcutLaunchPolicy.EvaluateInternalDrag` 多一个 `shortcutPath` 参数，首个条件之一为 `!ContainsSameFile(paths, shortcutPath)`（`Path.GetFullPath` + `OrdinalIgnoreCase`，与传输/重排判定同口径）。这样"拖回原位"退回重排语义（正常提交），而拖到**另一个**快捷方式上仍然启动。

**第六个发现（投放视觉统一，Simon 实机，纯视觉）**：快捷方式投放最初借 `SetFolderDropTarget`，用的是 `FileItemSurfaceVisualState.DropTarget` —— **强调色半透明底 + 1px 强调色边框**；叠放的 `ApplyStackSurfaceDropVisual` 自建 accent 画刷，文件夹同样走 `DropTarget`。三者都和格子里普通悬停/选中的中性色层（`GetNeutralStateLayer`，无边框）不是一套。Simon 要求全部统一 → **收敛到共享样式缓存**：

- `FileItemSurfaceStyleCache.Apply` 增加 `isDropTarget` 参数：为真时背景取 `_hoverSurfaceBrush`（选中时 `_selectedHoverSurfaceBrush`），**边框恒 0**；删除 `_dropTargetSurfaceBrush` / `_dropTargetBorderBrush` / `_pressedSurfaceBrush`。
- `ApplyItemSurfaceVisual` 传 `isDropTarget: state == FileItemSurfaceVisualState.DropTarget`，文件夹投放与快捷方式投放都命中这一条。
- 叠放投放改用 `SubtleFillColorSecondaryBrush` + 零边框（与叠放自身悬停同色），删除 `_stackDrop*` 三个字段。

**连带改动**：AOT 冒烟探针 `GetAotNativeFolderVisualState` 原先靠**边框粗细 + brush alpha** 判定 `DropTarget`，视觉不再画边框后会永远报 `Normal` → 改为 `IsActiveChildDropTarget(border)`（按 `_folderDropTarget` / `_stackMemberDropTarget` / `_launchDropTarget` 引用判定）；`AotStage5B4C1C2AContractTests` 里对应的 `thickness.Left >= 0.5` / `borderBrush.Color.A > 0` 断言同步替换。

**悬停提示**：内部拖拽没有 Shell drop description（那只有原生 OLE 拖拽才有），所以提示走 XAML 侧——命中时调 `ApplyDeskBoxFileDragFeedback(e, e.AcceptedOperation, Format("Widget.DropOnShortcutOpenWith", tile.Name))`，即"用 "X" 打开"（12 语言现成）。注意它必须在 `SuppressExternalDragOperationBadge`（会把 caption 关掉）**之后**调用。

**保留**：拖到**文件夹/叠放**瓦片的内部拖拽语义不变（谓词挡在前面）；叠放弹层成员拖动不变。`TryHandleLaunchTargetDropAsync` 的 launch 分支保留未删（若将来 WinUI 恢复投递 Drop，它是第一顺位）。

**代价与残留**：
1. 6px 图标余量：吸收 DragOver 采样间隔与手抖，远小于瓦片宽度，"瞄准文件名插入"不会被吃成打开。拖拽途中指针快速滑走再滑回同一图标仍会启动（最后一次 DragOver 会重新 armed，符合直觉）。
2. 该兜底只看"最后一次 DragOver armed 的瓦片"，所以**拖到格子外（如资源管理器）松手**不会被误判——那时指针早已离开瓦片，`DragLeave` 已清空记录。
3. armed 后松手若活几何不可知（记录边框被回收、`DataContext` 已顶替为其他条目）→ **不打开**（安全侧），排序也已被 armed 时的 `PersistSurfaceReorder` 丢弃，手势为无操作。该角落由 `[ShortcutLaunch] completion geometry rejected` verbose 日志观测，频率高再回头处理。

### 11.1 图标区命中 + 松手即时几何（2026-09-14，排序误开修正）

**问题**：格内拖动排序时，瓦片铺满整个网格区域（瓦片间距 <1.2px），排序的瞄准点必然落在某个瓦片上；快捷方式瓦片又整格是 launch 命中区且 launch 优先——排序经常被吃成"用该程序打开"。原 96→150px 松手余量还大于相邻瓦片中心距，进一步放大误判。

**修正两条（内外分离的依据是后果不对称）**：

| 改动 | 内容 |
|---|---|
| **命中区收窄（仅 DeskBox 来源的拖拽）** | `EvaluateInternalDrag` 判 Launch 后还须 `IsPointerOverLaunchIcon`（图标宿主矩形 + 6px 余量，`ShortcutLaunchPolicy.IsPointInsideRectWithSlack`）。落在标签/内边距 → 不处理该 DragOver，事件冒泡回根部，实时排序预照常推进（`HandleSurfaceRealTimeReorder` 对已取消的排序状态有完整的从载荷重臂逻辑，与 folder 瓦片今天的"armed 取消→离开重臂"同一条被日常使用的路径）。**外部拖入（Explorer）保持整格命中**：它落空的后果是掉入根导入=移动用户文件，与内部拖拽落空=无害重排不对称；native OLE 命中（`FindElementsInHostCoordinates`）只服务外部源，零改动 |
| **松手判定改活几何** | `IsCursorInsideLaunchIcon`：经 `_internalLaunchHoverBorder` 找回图标宿主（`DataContext` 路径校验防容器回收顶替）→ `GetCursorPos` 对图标矩形判定。删除"记录点 + 150px 半径"（`TryGetScreenPoint` / `IsCursorAtRecordedLaunchPoint` / `LaunchReleaseSlackPixels` 一并退役） |

**必须同步的三处（实现时验证过的遗漏）**：

1. **离开图标区由 launch 分支自清**：根部反馈入口（`ApplySurfaceDragOverFeedback`）与 `ClearStaleChildDropTargets` 只清文件夹/叠放目标，三类子目标的互斥只在"设置时"成立（`SetFolderDropTarget`/`SetStackMemberDropTarget` 清 launch），从 launch 滑走没有反向兜底 → 新增 `DisarmLaunchHover`（DragOver 与 routed Drop 的提前返回路径都调）。
2. **routed Drop 的内部分支不是死代码**：主网格自拖确实收不到 routed Drop，但**叠放弹层瓦片**（`StackPopoverFileIconTemplate` 等，挂同一对 handler）对主网格来源的拖拽是跨视图投放，Drop 会真投递 → `TryHandleLaunchTargetDropAsync` 的 internal-launch 分支同样加了 `IsPointerOverLaunchIcon` 门，且跨格子（`IsDeskBoxFileDrag` 但非本格 reorder）拖拽在 DragOver/Drop 两侧同口径收窄，避免"DragOver 承诺打开、Drop 却走导入"的分裂。
3. **顺手修现存残留 bug**：内部拖拽从快捷方式瓦片滑到根部空白区，launch 高亮会残留到手势结束（`ItemSurface_DragLeave` 的 launch 分支只清记录不清视觉；外部拖拽不受影响——native 链的 launch 高亮走 `SetFolderDropTarget` 通道，folder 陈旧检查顺带清掉）。修法：该分支真离开瓦片时补 `ClearLaunchDropTarget()`。

**图标元素暴露**：`FileItemSurface` 两个布局模板的图标宿主 Grid 加 `x:Name`（`IconItemIconHost` / `ListItemIconHost`），`CreateLayout` 用既有 `FindName` 模式一并取出，暴露 `IconHitTestElement`（按当前 Mode 取）。list 模式同样收窄到行首小图标，规则一致。

**测试**：`ShortcutLaunchPolicy.IsPointInsideRectWithSlack` 纯函数单测（余量带边界、空矩形拒绝）；契约测试 `ShortcutTiles_LaunchForInternalDragsToo` 的记录点钉换成 `IsPointerOverLaunchIcon(`/`DisarmLaunchHover(`/`_internalLaunchHoverBorder`/`IsCursorInsideLaunchIcon()` 五处新钉；`ShouldLaunchFromCompletedInternalDrag` 签名与语义不变，既有单测零改动。

