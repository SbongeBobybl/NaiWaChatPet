# 拖出到桌面/Explorer 的安全移动（第三批）

**状态：已撤销（2026-10-12）。** 本方案（复制+核验+回收站删除）已被 `StartDragAsync` + `AllowedOperations=Copy | Move` 的"方案 B"取代：移动由 Explorer 原生执行，DeskBox 源端永不删除。撤销原因：核验仅靠同名+大小+mtime 会误回收落点上已有的同名文件（取消/跳过/拖到应用图标都会删源）；`FOF_NOCONFIRMATION` 下过大文件、网络盘或关闭回收站会静默永久删除；Explorer 的 Ctrl+Z 撤销的是"复制"而非"移动"，可能双端尽失；AOT 下落点解析恒为空导致正式包行为与 Debug 不一致。以下正文仅留档，代码与对应测试已删除。

日期：2026-09-29。承接 [文件拖出原件保护](file-drag-source-safety-20260929.md)（第一批）与待实施的独立副本方案（第二批）。本文档定义第三批：恢复"拖到桌面/Explorer 即移动"的体验，且不重新引入源文件丢失风险。

## 统一判定模型

每一次拖出先回答三个问题：

1. **载荷里有什么？** 真实文件路径 / StorageItems / 纯文本 / 内部 token。
2. **接收方是谁？** DeskBox 内部 / 确认的 Explorer·桌面 / 未知外部。
3. **谁来执行最终变更？** 接收方经 OLE 协商 / DeskBox 文件服务 / DeskBox 核验回收。

总原则：**"移动"的权限永不交给 OLE；移动的执行者永远是 DeskBox。** 外部目标永远只看到 Copy。任何产生移动语义的路径都由 DeskBox 自行完成并逐项核验。

## 边界矩阵

| 拖拽源 → 目标 | 载荷 | 当前行为 | 第三批后 | 备注 |
|---|---|---|---|---|
| 格子 → 同格子排序 | token | 内部 Move，无文件操作 | 不变 | |
| 格子 → 另一格子 | 路径+token | Copy 传输 + FileService 执行移动 | 不变 | |
| 格子 → 文件夹/堆叠成员 | 路径+token | 内部处理 | 不变 | |
| 格子 → 待办/速记关联 | 路径+token | 目标端建立引用，不碰文件 | 不变 | |
| **格子 → 桌面/Explorer** | 真实路径 | Copy（第一批回归项） | **Copy + 核验 + 回收 = 净移动** | 本批核心 |
| 格子 → 微信等第三方 | 真实路径 | Copy | 不变 | 彻底隔离留给第二批（独立副本） |
| 格子 → 回收站 | — | 协议拒绝（Copy 无落点） | 不变 | 行为改变，实际是保护 |
| 格子 → 任务栏固定 | — | 可能失效（需 Link） | 不变 | 低频，文档化 |
| 搜索弹窗 → 外部 | StorageItems | Copy | 一致性收尾：readOnly + 会话日志 | |
| 待办/速记项 → 外部文本目标 | 文本+token | Copy\|Move | 建议收成 Copy | Move 对无文件路径载荷无害 |
| 组标题/标签重排 | token | 内部 Move | 不变 | |
| Explorer → 格子（进入方向） | 路径 | DeskBox 自行执行导入/移动 | 不变 | 对称：DeskBox 是唯一执行者 |

剪贴板剪切（Ctrl+X→粘贴到外部）仍携带 Move 语义，与拖拽同型但无落点识别手段。本批不处理，记为已知残留，后续可用同一核验框架统一收口。

## 核心设计：事后确认回收

不让 OLE 协商移动。Explorer 按 Copy 语义把文件落到真实目的地（它自行处理子文件夹图标、导航树等所有落点细节）；DeskBox 确认落点确为 Explorer/桌面、目的地文件逐项核验通过后，把源文件送回收站。**所有失败分支一律退化为"留下副本"，不产生丢失。**

### 流程

```
拖拽开始（载荷不变，仍 Copy-only）
  ├─ 落点追踪：拖拽期间 ~33ms 采样 GetCursorPos + LBUTTON 状态；
  │            按键释放时刻的坐标即物理落点（不依赖回执时机）；
  │            包装器收到完成回执时再捕获一次坐标作旁证。
  └─ 指纹锚点：打包时记录每个源文件 (size, mtimeUtc) ——
               核验的基准是"拖拽起始时刻的源"，不是当前源。
拖拽结束（DragItemsCompleted）
  └─ DropResult=Copy 或观察到完成回执 → 启动回收探针（异步，不阻塞）
       ├─ 落点判定：任一条件不满足 → 放弃回收（保留原件）
       ├─ 目的地等候：退避枚举目标目录，等待目的项出现
       ├─ 逐项核验：存在 + 大小匹配 + 指纹匹配 + 源未变化
       ├─ 通过的项：DeleteEntriesWithShellAsync(recycle:true)
       └─ HandleItemsMovedOutAsync 刷新格子条目
```

### 落点判定（任一失败 → 保留原件）

| 检查 | 方法 | 排除的误判 |
|---|---|---|
| 顶层窗口进程 | `WindowFromPoint` → 祖先链 → PID = explorer.exe | 微信、浏览器、DeskBox 自身、第三方文件管理器 |
| 命中子控件 | 必须是视图区（`SHELLDLL_DefView`/`DirectUIHWND`） | 导航树、地址栏、搜索框、标签栏（落点与实际目的地不符） |
| 目的地解析 | 桌面 → 用户桌面 + 公共桌面双候选；`CabinetWClass` → ShellWindows 按 HWND 匹配 → `LocationURL` 必须是 `file://` | 库/搜索结果/zip 视图/This PC 等伪路径 |

### 核验规则

- 锚点是**拖拽起始时刻**的指纹（size+mtimeUtc，可选哈希采样）。源文件在核验时仍需存在且指纹未变；变了 → 该项不回收。
- 目的项逐项核验；通过的回收、未通过的保留（部分通过是允许的）。
- Explorer "冲突→两个都保留"产生 `xxx - Copy` → 核验失败 → 保留（出现副本，安全）。
- "替换"同名不同内容 → 哈希不同 → 失败 → 保留。同名同内容 → 通过 → 净移动。
- 目的地即源目录 → 提前跳过回收（Explorer 会生成副本，用户意图就是复制）。
- 文件夹：枚举对比（条目数+总大小+抽样哈希）。
- 等候窗口按总字节放缩；超时 → 保留。
- 回收走回收站（与现有 managed-folder 删除同策略）；核验因此可用轻指纹而非全量哈希——误删可从回收站恢复。

## 与 OLE 方案的对比与取舍

| 维度 | 自有 DoDragDrop + IDropSourceNotify（旧设想） | 本方案（Copy + 核验回收） |
|---|---|---|
| 风险 | 之前实验卡死，拖拽生命周期不可控 | 失败单调退化为副本 |
| 目的地获取 | 仍需自行解析，且协议不给目的地路径 | Explorer 落点行为就是权威答案 |
| 同卷大文件 | 原生瞬时重命名 | 完整复制+核验+回收（双倍 IO） |
| 协议层面 | 需要放开 Move → 重新引入回执清理路径 | Copy-only 不变，第一批防线不动 |

已知代价：同卷拖到 Explorer 由瞬时改名变为"完整复制 + 回收"。对超大文件用户会感知等待与双倍占用。这是用 IO 成本换安全的有意取舍；原生瞬时移动的唯一替代是复活自有 `DoDragDrop`，风险不可接受，不做。

## 组件拆分

| 组件 | 职责 | 复用/新增 |
|---|---|---|
| `ExplorerDropTargetResolver`（`Services/`） | 落点判定 + 目的地解析，`Decide` 纯函数返回 Desktop / ExplorerFolder(path) / NotExplorer / Ambiguous | 链上溯用 `GetAncestor(GA_PARENT)`；非视图 chrome 类只查链头 3 层——Spike 实测 Win11 链路 `DirectUIHWND→SHELLDLL_DefView→…→ShellTabWindowClass→CabinetWClass`，中段容器不能误杀。ShellWindows 非 AOT 走 dynamic，AOT 暂返回空 → Ambiguous → 保持复制 |
| `FileSurfaceContent` 内嵌追踪（`StartExternalDragTracking`） | 拖拽期间 33ms 轮询指针位置与修饰键；按钮释放点=物理落点；Ctrl/Alt 按住=显式复制/链接意图→跳过回收 | 指纹存 `_activeDragFingerprints`，Completed 时快照 |
| `DragSourceFingerprint` + `DragOutReclaimEvaluator` + `DragOutReclaimRunner`（`Services/DragOutReclaimService.cs`） | 打包时记录源签名（文件：大小+mtime±2s 容差；目录：后代条目数+总字节）；退避轮询目的地逐项核验 | 核验锚点是"拖拽起始时刻"；源中途变化→不回收；核验不过→永远保留原件 |
| 回收执行 | `FileService.DeleteEntryAsync(path, recycle:true, ownerHandle:0)` 逐条 headless | Zero 句柄=静默无确认框；回收站放不下的按永久删除——此刻已有核验副本，语义仍等于移动 |
| `CopyOnlyFileDragDataObject` | 回执到达时打时间戳+捕获坐标（遥测，`CompletionReceiptSnapshot`） | 现文件，少量改动 |
| `DragOutReclaimPolicy.Enabled` | 代码级总开关，dogfood 期默认开 | 不入设置 schema（设置契约冻结，用户开关待生成器重跑+迁移） |

## 验证计划

1. **Spike-A（回执时机）**——待手动验证：现构建手动拖文件到 Explorer/桌面，读 `[DragProtocol]` 日志确认完成回执是否到达包装器 `SetData`（`stage=CompletionReceiptIgnored point=(x,y)`）、与按钮释放采样的落点是否一致。
2. **Spike-B（落点解析）**——已完成：探针枚举真实 Explorer 窗口成功，`CabinetWClass` HWND→ShellWindows→`file://` 路径映射可用；"此电脑"返回空 URL → Ambiguous；桌面走 Progman/WorkerW+DefView 链；实测 Win11 视图链路 `DirectUIHWND→SHELLDLL_DefView→CtrlNotifySink→DirectUIHWND→DUIViewWndClassName→ShellTabWindowClass→CabinetWClass`，确认 `SHELLDLL_DefView` 仍是正向锚点、中段容器类不能误杀。
3. 单元测试——已完成：`ExplorerDropTargetResolverTests`（13 例）与 `DragOutReclaimServiceTests`（10 例），全量 4575/4575 绿。
4. 真机矩阵——待手动：桌面、Explorer 视图、子文件夹图标、导航树、库、搜索结果、zip 视图、跨卷、大文件、多文件部分冲突、Esc 取消、Ctrl+拖动（显式复制，应不回收）、拖回 DeskBox 自身、Explorer 崩溃中。
5. 发布前补 Native AOT / ARM64 / Store 验证（AOT 下 ShellWindows 目录映射缺实现 → 自动退化为复制）。

## 验收标准

**假阳性回收率必须为 0**——宁可漏回收留下副本，不可错回收。任何"不确定"都等于"留下副本"。
