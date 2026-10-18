# 来源、修改与许可证

本软件是 NaiWaChatPet 与 DeskBox 的集成修改版本，不代表 DeskBox 上游官方发行。

- DeskBox 基线：v1.5.8，commit `6bbbb7d6aff6368316f6e25b0c1914b96dd68b70`。源代码、GPL-3.0 全文和原版权声明位于 `Source/DeskBox-1.5.8`。
- 修改日期：2026-10-17；本次版本：2.0.1 / v2.0.1。修改内容见 `CHANGELOG.md`。
- 集成增加奶蛙主入口、AI 分类、引用/快捷方式视图、文件事务与撤销、宠物搬运动画协议及专用界面，并移除不需要的上游功能模块。
- 对应源码：与安装包同版本的 `NaiWaChatPet-Source-v2.0.1.zip`，包含两个源项目、资源、构建脚本和安装定义。分发安装包时请一同提供对应源码和本声明。
- DeskBox 衍生部分按 GPL-3.0 分发；保留全部原版权声明。角色素材及其他独立资源的权利不因本说明发生转让。
- Go 依赖：`go-winio`（MIT）及 `golang.org/x/sys`（BSD），许可证保存在 `Source/NaiWaChatPet/third_party`，运行发行目录另附 `licenses`。
- Everything SDK 和其他依赖继续遵循各自许可。整理引擎的 `ThirdParty` 文件和 NuGet/Cargo 锁文件随源码保留；运行负载携带其原有第三方声明。
- 安装负载包含私有 Windows App Runtime；不依赖安装时下载运行库。

本文不移除或替换任何文件原有的许可证和版权声明。
