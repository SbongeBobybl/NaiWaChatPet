# 安全文件转移候选模块

这是依据修复需求新写的独立 Go 模块，并非恢复出来的 V10 源码，也尚未与 V10 或桌面 GUI 集成。

调用接口：`Transfer(ctx, source, target) (Result, error)`。同盘、跨盘都强制走相同复制流程：以独占句柄打开源文件，以 `CREATE_NEW` 创建目标，复制并计算 SHA-256，`File.Sync()` 刷盘，再分别重读源和目标检查字节数及 SHA-256；只有全部成功才通过原源文件句柄标记删除并关闭它。该模块不创建目录，不覆盖已存在的文件。

Windows 句柄使用 share mode 0，避免复制时被其它进程写入、重命名或替换，并通过 `SetFileInformationByHandle` 删除同一源文件，避免按路径重新定位造成误删。源文件必须已被其它程序关闭。源、目标父目录必须存在；只接受固定本地磁盘上的普通文件；拒绝目录、符号链接、reparse point、设备路径、网络路径、备用数据流，以及有歧义的 Windows 文件名。模块通过源句柄枚举 NTFS streams，拒绝含命名流、EFS 加密或多个硬链接的源，避免默认流复制后丢失隐藏数据或改变不明关联。非 Windows 平台直接返回不支持错误，不使用较弱的 check-then-unlink 回退。

失败行为：删除源之前的复制、刷盘、验证或取消失败会保留源文件，并尝试移除本次创建的未验证目标。目标通过验证后若清除源失败，会保留两份完整文件，返回 `delete-source` 错误；调用方必须显示结果，不能当作已移动。`TransferError.Result` 和返回的 `Result` 记录目标是否创建、是否保留、是否验证、源是否删除，`CleanupError` 报告未验证目标清除失败。极少数句柄关闭错误发生在源已标记删除之后时，`SourceRemovalUncertain` 明确标记删除结果无法确认；不得据此自动再次操作源路径。

范围和局限：父目录检查并不是原子事务，因此父目录需要由可信调用方控制，不能被其它主体同时替换。模块没有 Windows 文件系统事务或崩溃恢复日志；目标文件 `Sync` 不等于保证断电时全部目录元数据顺序，不能宣称断电事务性。仅复制默认流内容；目标继承目标目录权限，不复制源 ACL、时间戳或其它元数据。SHA-256 是内容一致性验证，而不是恶意文件内容检查。读写错误、磁盘满、权限、杀毒软件拦截等会在相应阶段返回。目录、包、已打开文件、云占位文件及 reparse 文件不在当前支持范围内。

测试只操作 `testing.T.TempDir()` 临时文件，不涉及用户文件。覆盖空文件/大文件、内容哈希、已有目标冲突、并发争抢同一目标、取消、只读源清除失败保留两份、目录/设备/流路径拒绝、命名 NTFS stream 与多个硬链接拒绝，以及其它程序已打开源的拒绝。符号链接测试需要 Windows 权限，能力不足时明确跳过。所有测试走强制复制路径；尚未进行两块真实磁盘之间的测试，也未做 GUI 测试。

从模块根目录执行：`go test ./safestore`。2026-10-09 本轮 Windows 测试已通过；`TestRejectsSymlinkSourceAndParent` 因系统缺少符号链接创建权限明确跳过，不能算作已验证。命名 stream 和硬链接测试均真实执行并通过。


2026-10-10 ADS-compatible feed mode: when Windows reports that a regular file
contains alternate data streams such as `Zone.Identifier`, `Transfer` uses the
Windows native `CopyFileW` with fail-if-exists, then compares the stream names
and sizes and hashes of the default stream. The source file is retained so a
failed/incomplete metadata copy never destroys user data. `Result.SourceRetainedForStreams`
indicates this intentional copy-only path. Ordinary single-stream files still
use the original verified-copy-then-delete semantics. This path has been
Windows-cross-compiled but requires testing on an actual NTFS Windows system.
