# 奶蛙桌宠 · NaiWaChatPet

**V2.0.1（2.0.1） / v2.0.1 · Windows x64**

让奶蛙陪你聊天、玩耍，也帮你收拾桌面。桌面整理是奶蛙的一项功能：从奶蛙启动，在预览中决定分类和文件处理方式，再看它收集文件、跑到分类格子旁放下。

[开始使用](#开始使用) · [整理方式](#整理方式) · [AI 分类](#ai-分类) · [外观与动画](#外观与动画) · [隐私](#隐私与发布数据) · [构建](#从源码构建) · [常见问题](#常见问题) · [更新记录](CHANGELOG.md) · [测试报告](docs/TEST-REPORT.md)

## 开始使用

1. 从本仓库 **Releases** 下载 `NaiWaChatPet-Setup-v2.0.1-x64.exe`。源码 ZIP 不能直接运行。
2. 运行安装程序，安装后启动 **奶蛙桌宠**。也可使用完整运行目录中的 `NaiWaChatPet.exe`。
3. 右键奶蛙 → **整理桌面**，检查扫描结果、整理范围与存放位置。
4. 选择离线分类或 AI 分类，修改分类名称、归属、排除项。
5. 确认“原文件如何处理”，再执行。先出现分类格子，奶蛙随后搬运。

> 始终启动 `NaiWaChatPet.exe`。`Organizer/DeskBox.exe` 是随奶蛙附带的整理引擎，不是日常使用入口。完整运行目录必须保留 `Organizer` 子目录。

支持 Windows 10 2004（19041）及更新版本、Windows 11，当前发布 x64 安装包。安装包自带整理引擎所需运行库；AI 请求需要网络和模型服务配置。首次源码构建需要联网下载工具链和依赖。未提供数字签名，系统可能显示未知发布者。

<details>
<summary><strong>升级、卸载与校验</strong>（点击展开）</summary>

- 升级前退出旧版奶蛙，运行新版安装程序。不要同时运行多个版本。
- 安装版默认位于 `%LOCALAPPDATA%\Programs\NaiWaChatPet`，开始菜单入口指向奶蛙。
- V2.0.1 使用全新的独立配置目录，首次启动不读取旧版 API Key、记忆或游戏进度；需要重新填写自己的模型配置。旧版数据不会被删除。
- 卸载程序不是“撤销整理”。已移动的文件需先在整理界面撤销；不要直接删除存放目录。
- 下载后在 PowerShell 运行：

```powershell
Get-FileHash .\NaiWaChatPet-Setup-v2.0.1-x64.exe -Algorithm SHA256
```

与同一 Release 的 `SHA256SUMS.txt` 比较。运行目录内的 `checksums.json` 用于逐文件核对。

</details>

## 整理方式

| 方式 | 源文件 | 格子内容 | 撤销结果 |
| --- | --- | --- | --- |
| 原位置引用 | 留在原目录 | 指向源文件的引用 | 移除本次分类视图，原文件不变 |
| 创建快捷方式 | 留在原目录 | 单独建立的快捷方式 | 清理本程序创建且未被修改的链接；用户修改或添加的文件保留 |
| 移动源文件 | 移到选定的分类目录 | 分类目录内的真实文件 | 按历史记录逐项返回各自原目录 |

**个人桌面和公用桌面是两个独立来源。** 公用桌面需纳入整理范围后才能移动；需要权限时由 Windows 文件操作流程处理。撤销不会把个人桌面文件改送到公用桌面。遇到同名文件不会覆盖原有文件，恢复位置与冲突结果以界面提示为准。

文件夹、程序依赖、系统/隐藏文件、重解析点和云端占位符等有不同保护规则。“全部纳入”不会绕过文件安全检查。引用视图不能锁定源文件：如果你在资源管理器中改名、删除或移动源文件，需要重新扫描。

<details>
<summary><strong>推荐操作流程与中途失败处理</strong></summary>

1. 先用“原位置引用”检查分类是否合适。
2. 使用分类卡片的选择、改名和拖拽功能调整预览；需要新增类别时使用新增类别入口。
3. 确实想移动时再切换“移动源文件”，检查存放目录并确认。
4. 同一次整理只由整理引擎执行真实文件操作。奶蛙动画负责展示，动画落点不是文件路径。
5. 搬运等待结束后还会重新检查源文件，期间变动的文件不会按旧快照盲目移动。
6. 出错时保留已完成记录，不要手工删除历史或把存放目录拖回桌面。先处理界面中的恢复提示，再使用撤销。

大批量内容按类别分批搬运；完整动画每批最多 40 项。取消可能留下已经完成的分类或移动，这些仍需按记录撤销。不要在整理过程中同时运行其他桌面移动工具。

</details>

## AI 分类

AI 沿用奶蛙已有模型服务设置与 API Key，无需为整理引擎重复配置。首次使用奶蛙的新用户需要在奶蛙设置中配置自己的服务。

1. 在奶蛙设置中配置自己的模型服务；整理界面复用同一配置。
2. 勾选“允许 AI 细化”，授权发送本次纳入整理项目的名称与类型信息；已排除项目不发送。
3. 点击“智能细分”。未勾选时使用离线分类，勾选后尝试 AI 按名称细分，目标为 **6–10 类**，实际受项目数量和有效结果限制。
4. 检查预览，拖拽调整归属，勾选或排除项目，选择处理方式与动画档位。
5. 点击底部确认按钮执行。分类请求本身不会移动源文件。

默认支持离线规则。结果区会标明本次是“AI 语义分类”还是“本地规则分类（未使用 AI）”；未配置模型或奶蛙未连接时不会发起 AI 请求。遇到服务失败、超时或返回无效类别，先查看提示、模型连接与名称发送授权。文件内容、完整路径、聊天记忆和 API Key 不作为分类条目发送；API Key 只用于所选模型服务的鉴权。



## 外观与动画

奶蛙主界面保留聊天、情绪、游戏及桌面交互。整理工作台使用浅色偏黄风格；分类格子的菜单可选择浅色/深色、透明/不透明。透明模式让内容区融入桌面，标题栏保留操作入口。文件格子保留拖拽、圆角、布局、叠放和胶囊等基础能力。

| 动画选项 | 行为 |
| --- | --- |
| 完整动画 | 按类别收集、携带、走到目标格子、放下 |
| 简易动画 | 整次整理展示一趟代表性搬运 |
| 无动画 | 直接执行整理，不等待奶蛙表演 |

V2.0.1 修复了引用模式被额外 10 秒限制截断的问题；动画使用实际分类格子位置，并保留统一通信超时、阶段看门狗和用户取消机制。快捷方式简易动画也会正确结束等待。

## 隐私与发布数据

**这个仓库和安装包不包含开发者的 API Key、聊天记录、记忆、武器存档或个人桌面布局。** 发布使用源码和编译产物白名单，构建前后进行隐私检查。

| 本地运行数据 | 是否进入发行包 |
| --- | --- |
| `companion-preferences.json` / 模型设置与凭据 | 否 |
| `companion-memory.dpapi` 及备份 | 否 |
| `play.json` / 武器持有、装备、游戏成绩 | 否 |
| `settings.json` / 格子、路径、整理历史 | 否 |
| IPC 描述文件、日志、请求记录、恢复记录 | 否 |
| 内置动画、角色图片、武器定义和游戏代码 | 是，属于功能资源，不是个人存档 |

V2.0.1 的奶蛙配置位于 `%APPDATA%\NaiWaChatPet\versions\2.0.1`，不自动导入旧版 `13.4.0` 的配置、记忆、聊天或武器存档。旧数据保留在原目录。整理引擎继续保留本机文件整理历史，以便撤销先前移动；这些历史不进入发布包。不要把 AppData、日志或存档提交到仓库。

<details>
<summary><strong>发布前自检命令</strong></summary>

```powershell
python scripts/test_release_privacy.py
python scripts/release_privacy.py Release/NaiWaChatPet-v2.0.1
python scripts/release_privacy.py Release/NaiWaChatPet-Source-v2.0.1.zip
```

检查失败时只会报告文件名，不会打印密钥值。扫描会拒绝运行存档、常见密钥形式、非空 JSON 密钥字段、重复 ZIP 条目和越界路径。自动扫描不能识别所有可能形式的秘密，提交自定义文件前仍应检查其内容。

</details>

## 从源码构建

源码包解压后的根目录可以直接作为 Git 仓库，不依赖原开发机器的 `C:\pet` 或 `D:` 路径。

```text
README.md                 使用说明（当前文件）
CHANGELOG.md              V2.0.1 更新说明
LICENSE / NOTICE.md       GPL 与第三方声明
Source/NaiWaChatPet/       奶蛙 Go 源码、资源、随附依赖
Source/DeskBox-1.5.8/      修改后的整理引擎完整源码、Rust 模块
scripts/                  构建、打包、隐私检查
installer/                Inno Setup 安装定义
docs/                     测试报告、发布检查表
```

### 工具链

- Windows x64，PowerShell，Python 3.10+。
- Go 1.27；本地随附 `go-winio` 和 `x/sys`，见 `go.mod` 的 replace。
- .NET SDK 10.0.402（由整理引擎 `global.json` 指定，可用后续补丁）。
- Visual Studio Build Tools：C++ 桌面开发工具与匹配 Windows SDK，用于 Native AOT。
- Rust/rustup；引擎锁定 Rust 1.96.0 和 MSVC 目标。
- Inno Setup 6，用于安装包。

```powershell
# 在源码包根目录；将 ISCC 路径替换为本机安装位置
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1 `
  -IsccPath 'C:\Program Files (x86)\Inno Setup 6\ISCC.exe'
```

该命令从源码构建奶蛙 GUI 程序和自带运行库的 Native AOT 整理引擎，再生成运行目录、对应源码包、安装包与 SHA256 清单。已有同名发布目录时拒绝覆盖；请先归档旧产物或更新版本标签。

<details>
<summary><strong>分步构建与必要测试</strong></summary>

```powershell
# 奶蛙逻辑与搬运流程
Push-Location Source/NaiWaChatPet/source
go test ./cmd/codexpet ./aiorganization ./tidy ./organizer ./companion
go build -trimpath -ldflags '-H=windowsgui' -o ../../../build/NaiWaChatPet.exe ./cmd/codexpet
Pop-Location

# 整理引擎：完整 Native AOT 构建，不复用外部旧二进制
powershell -NoProfile -ExecutionPolicy Bypass -File Source/DeskBox-1.5.8/scripts/publish-aot-retail.ps1 -Platform x64

# 核心回归（显式指定 x64，避免 AnyCPU/MSIX 错误）
Push-Location Source/DeskBox-1.5.8
dotnet test tests/DeskBox.Tests/DeskBox.Tests.csproj -p:Platform=x64 -p:RuntimeIdentifier=win-x64 -p:NaiWaOrganizerCoreTests=true -p:WindowsAppSDKSelfContained=true
Pop-Location
python scripts/test_release_privacy.py
```

上游已退出产品的功能仍有历史测试文件，当前验收使用 `NaiWaOrganizerCoreTests=true`，不声称完整上游测试套件通过。详细实测范围和未覆盖场景见 [测试报告](docs/TEST-REPORT.md)。

普通 Debug 调试入口为 `Source/DeskBox-1.5.8/src/DeskBox/bin/Debug/net10.0-windows10.0.22621.0/DeskBox.exe`。先关闭同名进程再编译；以新 DLL 时间戳确认写入，避免误用旧版。正式使用仍从奶蛙入口启动。

</details>

## 常见问题

<details>
<summary>为什么引用整理后，文件还在桌面？</summary>

引用方式不会移动原文件。希望真实收纳时，在预览选择移动源文件，并指定存放目录；先检查分类结果。

</details>

<details>
<summary>为什么奶蛙不搬运，或者只搬一次？</summary>

先检查动画选项。“无动画”不搬运，“简易动画”只展示一趟。完整动画若中断，查看整理提示；先退出旧版本，确认当前启动目录含 V2.0.1 的 `Organizer`。宠物忙碌时会重试，连接失效或超时会报告跳过情况。

</details>

<details>
<summary>安装新版后怎么判断没有启动旧引擎？</summary>

退出旧奶蛙后再启动新版，任务管理器中找到 `DeskBox.exe` 并打开文件所在位置。它应在当前奶蛙目录下的 `Organizer` 内；不能指向旧解压目录。运行目录的 `release.json` 和 `checksums.json` 可核对版本及二进制哈希。

</details>

<details>
<summary>AI 没有改变分类怎么办？</summary>

确认模型服务连接正常、已经同意发送名称并勾选项目，而且执行了智能分组并应用到预览。仅选择“AI”并不会自动授权上传或执行文件移动。可以加入明确用途描述，检查历史本地规则和模型返回提示。

</details>

<details>
<summary>撤销遇到权限或同名冲突怎么办？</summary>

保留原存放目录及整理历史，依据界面提示授权或重试。公用桌面可能需要 Windows 权限；不要以清空配置代替撤销。恢复必须以记录中的每项原路径为准。

</details>

## 上传 GitHub

把源码 ZIP 解压后的内容提交到仓库根目录，README 会自动显示目录跳转、可折叠说明和操作表格。安装包、源码 ZIP 与 `SHA256SUMS.txt` 放在同一 **GitHub Release**，不要把二进制包提交进 Git。发布步骤见 [发布检查表](docs/RELEASE-CHECKLIST.md)。

本交付只准备本地文件，不会自动替你创建仓库、上传或发布。

## 许可证与来源

整理引擎基于 DeskBox v1.5.8，保留 [GPL-3.0 许可证全文](LICENSE) 与源代码中的版权声明。本仓库是奶蛙集成修改版，不是 DeskBox 官方发行版。分发二进制时应同时提供本版本对应源码、构建脚本和许可证。第三方依赖保留各自许可证，见 [NOTICE.md](NOTICE.md)。
