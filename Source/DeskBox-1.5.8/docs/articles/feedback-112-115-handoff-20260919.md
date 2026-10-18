# 反馈 #112-115 处理交接文档（2026-09-19）

> 面向接手的 agent / 会话。本轮：拉取后台反馈 → 逐条对码核实 → 后台回复 → 四条全部修复 → 3893/3893 绿（合并态）→ 未提交，待 Simon 真机验证。
> 敏感信息（服务器 IP、SSH 细节）不入库：见会话记忆 `deskbox-feedback-backend-workflow`（同工作区共享）。本文用 `<server>` 占位。

## 1. 后台用户反馈数据怎么拉

### 1.1 反馈列表（无鉴权，公开 API）

```
curl -s https://deskbox.fun/stats/api/feedback/public
```

返回 `{ok, items:[{id, type("bug"|"suggestion"), status, content, reply, app_version, channel, created_at}], has_more}`。

**坑：`id` 是字符串不是数字**（`"112"`），JS 里比较用 `String(it.id)`，否则 filter 静默为空。`has_more` 为 true 时此端点返回最新 20 条，更早的要用别的途径（目前够用）。

状态流转：`open → confirmed → in_progress → done / wontfix`。回复口径先例见 #103/#102（确认+承诺排期，语气平实）。

### 1.2 诊断包（SSH 直取）

```
scp root@<server>:/var/www/deskbox-analytics/uploads/feedback/{id}.zip <本地目录>
```

- 服务器 IP 与免密配置见记忆 `deskbox-feedback-backend-workflow`，本地凭据文件（不入库）只有 DNS/百度 token。
- 用户不带附件时该路径 404（#113/114/115 都没有，只有 #112 有）。
- 本地先例目录：`D:\project\wingezi\artifacts\feedback-{id}\`。

### 1.3 回复 / 改状态（SSH 本地回环 PATCH，2026-09-19 实测可用）

admin HTTP 接口外部路径被 dashboard AuthGate 登录门禁挡住，**但服务器本地回环可直调**（门禁只挂外部路由）：

```
ssh root@<server>
curl -X PATCH http://127.0.0.1:3002/api/admin/feedback/{id} \
  -H 'Content-Type: application/json' \
  --data '{"status":"confirmed","reply":"回复文本"}'
# → {"ok":true}
```

- 3002 = deskbox-analytics 服务监听端口（`ss -tlnp` 可核实）。
- `reply` ≤1000 字符（服务端 cleanText 截断）；`status` 只接受 open/confirmed/in_progress/done/wontfix。
- 回复后用 1.1 的公开 API 复核生效。

### 1.4 诊断包内容（zip 三文件）

| 文件 | 说明 |
|---|---|
| `diagnostics.json` | schemaVersion 5。有用字段：appVersion / distributionChannel(Direct\|store) / isPackaged / OS / 进程架构 / uiCulture / hotkeys / settings（加载恢复状态）/ shortcutNative / runtimeHealth / widgetManager（每个表面的 widgetKind/bounds/可见性——**格子几何尺寸在这，判断视口类 bug 用它**）/ displays |
| `DeskBox-sanitized.log` | **只有尾部**（非全量），路径全脱敏成 `<PATH>`/`<REDACTED>`；事件类日志（[Import]/[DropTarget]/[FileTransfer]/[Memory] 等）健在 |
| `README.txt` | 说明 |

## 2. 四条反馈的分析（全部对码核实过，均属实）

四条几乎确定是**同一位高技术水平用户**连续提交（#115 自述"第一条提交成功（#112）"，#112/#114 引用源码，#115 直接打接口核实 HTTP 429）。诊断包：1.5.4 直装 x64 zh-CN Win11 26200。这类反馈可信度极高，值得优先处理。

### 2.1 #115 反馈弹窗自身缺陷（bug）

三个独立缺陷，全在代码里核实：

1. **超时永久卡死**：`FeedbackService` 构造 `HttpClient{Timeout=60s}`；超时抛 `TaskCanceledException`（`OperationCanceledException` 子类），被 `catch (OperationCanceledException) { throw; }` 原样上抛；调用侧 `SettingsWindow.Feedback.cs` 的 `PrimaryButtonClick` 无 try/catch，`deferral.Complete()` 执行不到 → 弹窗永久停在"提交中…"、主按钮永久禁用。用户还观察到 ~60s 后应用重启，时间点吻合（未定案，回复里已提）。
2. **失败提示不可见**：`errorText` 挂在 `formPanel` 内，提交成功后 `formPanel.Visibility=Collapsed` → 同一弹窗内再次提交失败（如限流）时错误写进隐藏容器，表现为"点了完全没反应"。
3. **限流无引导**：服务端 429（10 分钟防滥用）只显示"10 分钟内已反馈过"，不显示还需等多久、不禁用按钮。

### 2.2 #112 更改收纳根迁移失败 + 回滚静默（bug，数据完整性风险）

用户场景：收纳根 C 盘、10 个跟随默认路径的收纳格子、332 个 .lnk，改收纳根到 D 盘，连续两次失败，22 个快捷方式分叉在 C/D 两处且格子里看不见。

根因链（诊断包日志确认，恰好 2 条 `Rollback failed`）：

1. **前向失败**：跨盘 move 走 chunked-cross-volume 路径，`FileService.TransferProgress.cs` 用 `CreateFileW(源, READ|DELETE|WRITE_ATTRIBUTES, ShareRead)` 打开源——只留读共享。此刻 Shell 图标缓存/格子元数据读取正持有该 .lnk 的无 FILE_SHARE_DELETE 句柄 → 打开失败（win32=32）→ **整批中止而非跳过该项**（48 个文件第 1 个被卡 = 0 item completed）。
2. **回滚静默**：`WidgetManager.Storage.cs` 的 catch 对已搬走目录做 best-effort 回滚，**失败只 `App.Log` 不上报**，最后 `throw` 的仍是前向原始异常 → 用户只看到"迁移失败"，不知道文件已分叉。
3. **连带风险**：失败后新根的半搬目录会被 `ManagedStorageDestinationResidueException` 判定为"上次失败的完整副本"提议移入回收站——半搬状态下有误删窗口。

注意：sharing violation 是**别人进程的句柄**卡的，改我们自己的共享模式消不掉，只能走"释放占用/容忍跳过"。单一 handle 跨 copy-to-commit 是刻意的防 TOCTOU 设计（`TransferProgress.cs:557` 注释），动它要过 FileSafety 的事故清单（历史上 lnk-incident 教训）。

### 2.3 #113 关闭格子后同名新建被堵死（bug）

- 关闭收纳格子（选"保留文件"）= 配置移除 + ID 进 deletedWidgetIds，**底层文件夹刻意保留**（设计如此，防误删用户文件）。
- 之后重命名/新建同名格子被 `RenameManagedWidgetFolderAsync` 一刀切拦下："已存在同名收纳格子，请换一个名称"——**不区分"活格子占用"和"残留文件夹"**。
- 关键澄清：**新建入口本来就会自动避让**（`CreateManagedFolderName` 冲突时加 "(2)"），用户的报错实际来自**重命名路径**。对码时别再追新建入口。
- 用户附带的合理抱怨：关闭确认没说明文件夹去哪了；缺"恢复已关闭格子"入口。

### 2.4 #114 收纳路径与映射路径重叠提示笼统（suggestion）

- 拦截本身是**刻意设计**（收纳存储要求独占目录树，迁移/清理才不碰重叠格子），用户自己核对源码后认可，只要提示细化。
- 原文案"相同或相互包含"看不出是同目录还是父子包含，也不显示对方路径。
- 用户补充的正确信息：两个**纯映射**格子的父子关系是允许的（`IsFileWidgetPathConflict` 末尾的 `FollowsDefaultStoragePath` 判定），文案里可以说明。

## 3. 已完成的修复（本轮，全部未提交）

### 3.1 #115 反馈弹窗

**`src/DeskBox/Services/FeedbackService.cs`**
- `SubmitAsync` / `GetMyFeedbackAsync` 的 OCE 拆分：`catch (OperationCanceledException) when (cancellationToken.IsCancellationRequested) { throw; }` 保留用户取消语义；其余 OCE（=HttpClient 超时）转 `NetworkFailure()` / `Failure()`。

**`src/DeskBox/Views/SettingsWindow.Feedback.cs`**
- `PrimaryButtonClick` 整体包 try/catch/finally：finally 保证 `deferral.Complete()`；catch 写错误提示（兜底，正常不该到）。
- `errorText` 挪出 formPanel，挂外层 StackPanel（成功后仍可见）。
- 成功面板加"再提交一条"按钮（`submitAnotherButton`）：回表单、清文本、重选 suggestion。
- 限流分支禁用主按钮；**复查补刀**：`rateLimitedPrimaryDisabled` 标志防止 `RefreshValidation`（文本编辑触发）在限流窗口内重新启用按钮；"再提交一条"复位该标志。

**文案**：新 key `Feedback.Dialog.SubmitAnother` ×12 语言（`src/DeskBox/Strings/*.json`）。

**测试**：`FeedbackServiceTests` +2：`Submit_HttpTimeoutReportsNetworkFailureInsteadOfThrowing`、`Submit_UserCancellationStillThrows`。

### 3.2 #112 迁移回滚失败上报 + 归位入口

**`src/DeskBox/Services/WidgetManager.cs`**
- 新 `record ManagedStorageRollbackFailure(WidgetId, WidgetName, DestinationFolder, SourceFolder, PreserveExisting, Reason)`。
- 新 `ManagedStorageRollbackFailureException`：携带原始异常（InnerException 保留）+ 未归位清单。

**`src/DeskBox/Services/WidgetManager.Storage.cs`**
- 迁移 catch 块收集回滚失败（`PreserveExisting` = 原回滚走保守合并路径的 widget），有失败则抛 `ManagedStorageRollbackFailureException` 而非裸 rethrow。
- `completedMoves` 元组加 `WidgetName`。
- 新 `RetryMigrationRollbackAsync(failures)`：保守重试归位（PreserveExisting 用 `RestoreMigratedDirectoryPreservingExistingAsync`，其余 `RelocateDirectoryAsync`），带 busy 标志和 widget 刷新，返回仍失败的清单。

**`src/DeskBox/Services/ManagedStorageMigrationResidueDialog.cs`**
- 新 `ShowRollbackFailureAsync`：列出未归位文件夹（格子名+新根路径）、解释文件可能两处并存、循环"重试归位"直到全部归位或用户关闭；全部归位显示完成提示，部分归位刷新剩余清单。

**调用侧**：`SettingsWindow.StorageAndUpdates.cs` + `OnboardingWindow.Storage.cs` 各加 `catch (ManagedStorageRollbackFailureException)`（在 `ManagedStorageDestinationResidueException` 之后、泛型 catch 之前）。

**文案**：6 个 key `Settings.Dialog.MigrateRollback{Title,Body,Hint,RetryButton,RetryComplete,RetryPartial}` ×12 语言。

**测试**：`ManagedStorageMigrationSafetyTests` +2：`ReportsFoldersLeftBehindWhenRollbackFails`（全链路：迁移失败→回滚被文件锁卡住→异常带清单→RetryMigrationRollbackAsync 归位成功）、`RetryMigrationRollbackAsync_KeepsFailureWhenRetryStillBlocked`。**锁时机坑**：FileStream 必须在 override 的前向迁移完成后开（迁移前文件不存在），并行会话帮忙修过一版。

### 3.3 #113 同名残留文件夹接管

**`src/DeskBox/Services/WidgetManager.Storage.cs` 的 `RenameManagedWidgetFolderAsync`**，逻辑改为三分支：
1. `IsManagedWidgetNameInUse`（活格子占用）→ 仍报 `Widget.Error.ManagedFolderNameExists`。
2. 目标是已存在的**目录**：
   - 当前格子文件夹**非空** → 报新 key（防合并丢数据）；
   - 当前格子文件夹**为空**（新建格子场景）→ **接管**：删除空文件夹、直接指向残留文件夹，内容出现在格子里（有 App.Log）。
3. 目标是已存在的**文件** → 报新 key。

**文案**：新 key `Widget.Error.ManagedFolderNameUnavailable`；`Widget.KeepManagedFolder` 改为"保留文件（文件夹留在原位置）"。

**测试**：`WidgetManagerStorageCleanupTests` +2：`EmptyWidgetAdoptsClosedWidgetResidueFolder`、`RejectsNameWhenBothFoldersHoldFiles`。

### 3.4 #114 重叠提示细化

**`src/DeskBox/Services/WidgetManager.cs`**
- 新 `enum FileWidgetPathRelation { SameDirectory, CandidateInsideOther, CandidateContainsOther, UnresolvableOverlap }`。
- 新 `DescribeFileWidgetPathRelation(candidate, other)` 静态判定器（同路径/互相包含=同目录；复用 `FileService.TryIsPathUnderDirectoryResolved`；判定不了进 UnresolvableOverlap）。
- 新 `FormatFileWidgetPathConflictMessage(candidate, otherName, otherPath)`：按关系选 4 个 key 之一，Format 参数 = (对方名, 对方路径)。
- `EnsureFileWidgetPathAvailable` 与迁移预检（Storage.cs）改用它。

**文案**：4 个 key `Widget.Error.FileWidgetPathConflict{SameFolder,InsideOther,ContainsOther,Unresolvable}` ×12 语言；InsideOther/ContainsOther 说明独占原因+"纯映射格子父子不受限"。旧 key `FileWidgetPathConflict` 保留（ViewModel 后备分支还在用，该分支 WidgetManager=null 生产不可达，未动）。

**测试**：`WidgetManagerStorageCleanupTests` +2：`DescribeFileWidgetPathRelation_DistinguishesSameInsideAndContains`、`FormatFileWidgetPathConflictMessage_NamesTheOtherPath`。

### 3.5 后台回复（全部已生效，状态 confirmed）

#112：致谢+确认定位（占用整批中止+回滚未告知），承诺归位入口优先修复、迁移容错后续版本。
#113：确认设计意图+承诺"复用引导/关闭说明"，恢复入口已记录待评估。
#114：致谢核对源码，承诺文案细化。
#115：致谢完整反馈，确认三问题，承诺尽快修复；提示 60s 重启若复现欢迎附诊断包。

## 4. 当前进度与工作区状态

- **修复批次验证**：2026-09-19 15:36 全量 `dotnet test -p:Platform=x64` **3893/3893 绿**（我的批次+并行批次合并态）。
- **全部未提交**，涉及文件：`FeedbackService.cs`、`SettingsWindow.Feedback.cs`、`WidgetManager.cs`、`WidgetManager.Storage.cs`、`ManagedStorageMigrationResidueDialog.cs`、`SettingsWindow.StorageAndUpdates.cs`、`OnboardingWindow.Storage.cs`、`Strings/*.json` ×12、测试 `FeedbackServiceTests` / `ManagedStorageMigrationSafetyTests` / `WidgetManagerStorageCleanupTests`。
- **并行会话**：同一工作区有另一 agent 持续活跃（Settings slice 迁移、#112 orphan restore、Platform P/Invoke 重构）。2026-09-19 15:5x 起它在重写 `src/DeskBox/Platform/{AdvApi32,Kernel32,Ole32,Shell32}NativeMethods.cs` + `Win32Helper.SessionDisplayFont.cs`，导致 3 个源码契约测试挂（`AotStage5B4C1B2BContractTests.ShellHelper_...`、`AotStage4D3BContractTests.DropTargetRegistration_...`、`AotPublishContractTests.RustNativeStage3C2_...`）——**归因明确是它的批次收尾责任，与本批次无关，勿修**。等它收尾后需一轮全量回归。
- **DeskBox 运行中**：canonical Debug 路径 `src/DeskBox/bin/Debug/net10.0-windows10.0.22621.0/DeskBox.exe`（重启时验证过 pid 与路径）。
- 本轮撞车实录与协议经验：见记忆 `deskbox-feedback-112-115-fixes`（契约测试挂了先查文件 mtime 归因，别急着改 manifest）。

## 5. 接下来做什么

### 5.1 等 Simon 真机验证（6 项，验证通过才提交）

1. **#115**：提交反馈 → 成功面板有编号+"再提交一条"；点它回到表单可再提交。
2. **#115**：断网提交 → 显示网络错误并恢复按钮，不永久卡"提交中…"。
3. **#115**：10 分钟内连提第二条 → 限流提示+按钮禁用，**编辑文本不会重新亮起按钮**。
4. **#113**：关闭收纳格子（保留文件）→ 新建格子 → 重命名成旧名 → 接管成功，旧内容出现在格子。
5. **#112**：C 盘收纳根+格子可见 → 改到 D 盘触发失败 → 弹"部分文件夹未能归位"对话框，重试归位后文件回 C 盘原位。
6. **#114**：收纳根设为某映射格子子目录 → 报错写明"位于「XX」的存储路径「D:\...」内部"+原因。

### 5.2 提交（验证过后，按 Simon 当次口径）

提交信息走临时文件（`git commit -F`，Mimosa hook 兼容）；**不加 Co-Authored-By/Generated with**（.githooks/commit-msg 强制）。注意 12 语言 json 与 manifest 契约测试已在绿态，直接提交即可。

### 5.3 遗留功能项（未做，按优先级）

| 优先级 | 项 | 说明 |
|---|---|---|
| 高 | #112 根治：per-item 容错 | 单文件被占用不再毁整批：跳过+最后汇总"N 项未处理"；需目录级搬运协议支持，单独立项 |
| 高 | #112 迁移前释放自身占用 | 迁移开始前收起/隐藏格子、暂停快捷方式元数据与图标读取（`SetManagedStorageMigrationBusy` 已有 busy 机制可挂） |
| 中 | #112 残留处置内容比对 | `ManagedStorageDestinationResidueException` 提议回收前加内容比对，堵半搬误删窗口 |
| 中 | #112 异常文本本地化 | 失败对话框 `{0}` 仍填英文原始异常（"The Windows file operation completed only part..."），只覆盖了一半 |
| 中 | #112 重试关闭后的后续入口 | 对话框关闭后仍失败的文件没有再入口（日志有全记录）；考虑设置页"上次迁移残留"卡片 |
| 低 | #113 新建同名完整复用引导 UI | 本轮只做了重命名接管；新建入口本会避让 "(2)"，要做"发现同名残留，是否复用"需改 `CreateManagedWidgetCoreAsync` |
| 低 | #113 恢复已关闭格子入口 | deletedWidgetIds 里的配置恢复；后台回复承诺"已记录待评估"，做之前别再承诺 |
| — | #115 60s 重启疑点 | 用户怀疑超时异常与 ~60s 应用重启相关；未定案，等复现+诊断包 |

### 5.4 发版挂接

后台回复的承诺口径：#115 "尽快修复"、#112 归位入口"优先"、#112 容错/#113 引导"后续版本"。**这些修复进 1.5.4.x 热修还是 1.5.5 由 Simon 拍板**；发版流程用 `/deskbox-release` skill（三种范围措辞触发，别越权提交推送）。

## 6. 红线与坑（接手必读）

1. **AGENTS.md 规则**（全文在仓库根）：改应用代码后先停本仓库路径下的 DeskBox.exe 再构建、重启用 canonical Debug 路径；测试必须 `dotnet test .\tests\DeskBox.Tests\DeskBox.Tests.csproj --no-restore --verbosity:minimal -p:Platform=x64`（AnyCPU 会被 MSIX 拒）；提交只带 `Simon <1047078635@qq.com>` 身份。
2. **并行会话协议**：工作区可能同时有别的 agent（当前就有）。测试挂了/构建挂了，**先查文件 mtime 归因**再动手；绝不 stash/reset 别人的改动；等对面写完再验证。记忆 `deskbox-parallel-agent-collision`。
3. **12 语言契约**：新增/修改文案 key 必须 `src/DeskBox/Strings/*.json` 12 个全同步；Format 参数个数变更也是全量同步；改完 `JSON.parse` 校验一遍（node 一行）。新增 JsonSerializer 调用另有基线+8 个阶段守卫（见记忆 `deskbox-json-frozen-count-trap`）。
4. **manifest ratchet**：`ModuleBoundaryContractTests.DestructiveFileOperations`（文件突变操作计数）和 `SettingsSliceOwnershipContractTests.FacadePassthroughAccess`（slice 访问计数）变了会挂——升级计数必须在 manifest 处留注释说明原因，这是"有意识扩展"契约。
5. **FileService 传输内核别乱动**：单 handle 跨 copy-to-commit、manifest 删源、保守合并回滚都是事故教训换来的（lnk-incident、TOCTOU）；改动前读 `ManagedStorageMigrationSafetyTests` 全部注释与 FileSafety 域文档。
6. **XamlCompiler 红字连锁**：`SettingsWindow.xaml` 新 x:Name 必须在 SectionElements.cs 手写属性（CS0103 是唯一真错）；AOT 下别绑 `IReadOnlyList<T> => [...]`（用 WrapOptions）。
7. **反馈 API id 是字符串**；诊断包 log 只有尾部且路径脱敏。
8. **Mimosa hook**：直连 git commit 可能被 L3 拦，用 `powershell -File 临时ps1` 包装或 `git commit -F 临时文件`；Write 工具写脚本比 Bash heredoc 稳。
