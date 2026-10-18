# DeskBox 崩溃转储支持工具

这组脚本用于在明确需要排查崩溃时，临时启用 Windows Error Reporting（WER）的 LocalDumps。它不是 DeskBox 的常驻功能，也不会随应用或安装包自动运行。

## 安全边界

- 只写入当前用户的 `HKCU` 注册表，不需要管理员权限，也不修改 `HKLM`。
- 只允许配置 `DeskBox.exe` 和 `DeskBox.Updater.exe`；默认只配置 `DeskBox.exe`。
- 默认保存到 `%LOCALAPPDATA%\DeskBox\CrashDumps`，也可通过显式参数指定其他绝对路径。
- 默认生成 Mini dump，最多保留 5 份。Full dump 仅应在 Mini dump 信息不足时临时启用。
- 脚本不会自动上传、打包或发送转储，也不会把转储加入现有诊断包。
- 启用脚本不会覆盖已有的、并非由本工具管理的 per-exe LocalDumps 配置。
- 禁用脚本只处理带有本工具所有权记录的指定可执行文件。若某个值在启用后被外部修改，该值会保留并显示警告。
- 禁用不会删除已经生成的 `.dmp` 文件。完成支持排查后，应由用户确认并手动删除。

转储可能包含文件路径、窗口内容片段、内存中的文本或其他敏感数据。分享前应确认接收方和传输渠道；不再需要时应及时删除。Full dump 的敏感程度和磁盘占用通常显著高于 Mini dump。

## 启用

在仓库根目录运行：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\Enable-DeskBoxLocalDumps.ps1
```

指定保存位置和保留数量：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\Enable-DeskBoxLocalDumps.ps1 `
  -DumpFolder 'D:\DeskBoxSupport\CrashDumps' `
  -DumpCount 3
```

仅在支持人员明确要求时生成 Full dump：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\Enable-DeskBoxLocalDumps.ps1 -DumpType Full
```

同时诊断主程序和更新器：

```powershell
& .\scripts\Enable-DeskBoxLocalDumps.ps1 `
  -ExecutableName @('DeskBox.exe', 'DeskBox.Updater.exe')
```

重复执行启用命令会更新本工具已经管理的同一 per-exe 配置。如果检测到用户或其他工具已有配置，脚本会停止且不接管这些设置。

## 禁用

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\Disable-DeskBoxLocalDumps.ps1
```

如果启用时同时选择了更新器，禁用时也明确列出它：

```powershell
& .\scripts\Disable-DeskBoxLocalDumps.ps1 `
  -ExecutableName @('DeskBox.exe', 'DeskBox.Updater.exe')
```

禁用完成后，检查并手动清理转储目录。脚本有 `-WhatIf` 支持，可在实际修改前预览目标。

## 获取转储

启用后正常运行 DeskBox，并复现一次真实崩溃。WER 会将 `.dmp` 写入配置目录。仅将与复现时间对应的文件交给可信的支持人员，并同时说明 DeskBox 版本、Windows 版本、复现步骤以及使用的是 Mini 还是 Full dump。
