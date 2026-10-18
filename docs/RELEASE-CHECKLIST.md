# GitHub 发布检查表

[返回 README](../README.md)

## 本地发布

- [ ] 更新版本、标签、更新记录与本页检查状态。
- [ ] 使用本版本源码重新编译两个程序，不从旧版发行目录复制 exe。
- [ ] 运行核心回归与发布隐私检查，记录真实结果。
- [ ] 检查完整运行目录包含 `NaiWaChatPet.exe` 和 `Organizer/DeskBox.exe`。
- [ ] 校验 `release.json` 中的引擎和奶蛙哈希；对照 `checksums.json`。
- [ ] 在干净测试用户下验证启动、引用完整搬运与撤销，不能用真实桌面文件做破坏性测试。
- [ ] 确认没有模型凭据、记忆、日志、武器进度、个人路径和历史配置。
- [ ] 同时准备安装包、完整对应源码 ZIP、SHA256 清单、许可证及更新说明。

## 上传

- [ ] 源码包解压后的根目录提交至 GitHub 仓库，保留 README、LICENSE、NOTICE、Source、scripts、installer、docs。
- [ ] 二进制和源码 ZIP 上传 GitHub Releases，不加入 Git 仓库历史。
- [ ] Release 标签使用 `v2.0.1`，正文采用 CHANGELOG 对应章节。
- [ ] 附上测试范围；未实测 Windows 10、多屏、多 DPI 时不要声称全环境通过。
- [ ] 上传后重新下载并比对 SHA256，确认源码与安装包属于同一版本。

已有开发机配置不应通过“打包前删除 AppData”处理。发布器只收集源码与全新构建输出，遇到运行存档或可疑凭据立即中止。
