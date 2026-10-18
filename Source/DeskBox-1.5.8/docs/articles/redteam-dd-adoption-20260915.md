# 红队 DD 报告采纳对照（2026-09-15）

> 对象：Simon 转来的《红队代码尽调》（投资人视角技术 DD，审查对象为 GitHub main / 1.5.2 公开源码 + CI/release pipeline）。
> 方法：四条事实性主张全部亲自对码核验；与本仓未提交的四批改动（启动韧性、render window、文件安全 Round 1/2）和既定 Round 3 计划逐项对照。
> 本文只做采纳裁定与计划调整，不改代码。

## 一、可信度评估

**总评：事实性主张 4/4 成立，引用与本地代码一字不差，可信度高。** 主要盲区恰是我们已补的领域（见 §三）。

| 主张 | 对码结果 | 证据 |
| --- | --- | --- |
| P1 Settings migration fail-open | **成立（实锤）** | `SettingsMigrationService.cs:56-74`：`FromVersion >= version` 条件 + catch 后继续循环 + `:74` 无条件 `SchemaVersion = CurrentSchemaVersion`。报告推演（4→5 失败→5→6 照跑→最终标 9）逐行吻合。**加重因素（报告未提）**：`anyApplied` 在部分成功时仍返回 true → `SettingsService.cs:693` `changed |= ...` → 触发保存，把半迁移状态+SchemaVersion=9 一起持久化；下次启动 `:48` 的 `>=` 检查直接跳过 → **被跳过的迁移永久不再执行**。 |
| P1 更新只有 SHA-256 无发布者身份 | **成立** | `AppUpdateService.cs IsManifestUsable`：仅查 `SchemaVersion==1 && Version 非空 && URL 绝对`，不强制 HTTPS、不限域名、无签名校验。SHA-256 防损坏不防投递的分析正确。定级（consumer P2 / enterprise P1）属观点，事实部分准确。 |
| P1 ARM64/Distribution gate 非 main 门禁 | **成立** | `arm64-runtime.yml` 与 `distribution-audit.yml` 触发均为 `workflow_dispatch` + `codex/stage7b-arm64-actions` 分支；main/PR 只有 `ci.yml`（x64）。 |
| P2 Settings 返回 mutable object graph | **成立** | `SettingsService.cs:414-417` 锁只保护引用交接；"当前没爆是因为约定俗成 UI 线程改"的判断与代码现状一致。 |
| P2 跨盘迁移 size+timestamp 判源 | **对 main 成立，本地已部分修复** | 本地工作树（未提交）Round 1 已加 `FileTransferSourceIdentity`（NTFS FileKey）：单文件 move 删源前复验已上 FileKey；目录 manifest（`DeleteSourceTreeByManifest`）仍是 Length+mtime——正是既定 Round 3 的统一项。报告建议的 NTFS File ID 方向与我们已实施/计划的完全一致。 |
| 正面评价（fail-closed 方向、ARM64 gate 能力、Rust ABI、release engineering） | 与既有记忆/审计一致，无美化 | — |

**报告的盲区（我们已补、它没找到的）**：红队结论"没有找到 copy 完就 delete source 的灾难性实现、文件迁移给它加分"——实际上 main 上存在我们三路核对实锤的三处：通用 copy 回滚的 `Directory.Delete(recursive:true)`（**用户取消即触发**）、无进度 `CopyEntryAsync` catch 里无条件删目标、`MoveFileAsync` fallback 删 dest 无归属验证（本地 Round 1/2 已全部修复）。说明其文件操作深挖深度不及专项审计；但其广度（migration/update/CI/Rust）覆盖了我们没审的域。互补关系明确。

## 二、逐项采纳裁定与计划调整

### 已覆盖（无需调整）
- **P2 size+timestamp** → Round 1 已修单文件；目录 manifest FileKey 化维持 Round 3。
- **§12/§19 故障注入与 invariant 测试**（Term Sheet 条件四："任何阶段失败不得同时失去 source 和 valid destination"）→ 与 Round 3 计划完全吻合，**升格为 Round 3 主体**（原"身份统一层"并入为其中一项）。报告把它的战略优先级抬得有理：migration fail-open 恰是 3500+ 绿测试下的漏网之鱼，证明 regression 测试锁不住 invariant。

### 新增采纳（计划调整）
1. **P1 migration fail-open → 新增 Round 2.5 小批次（最优先，20 行级修复）**：
   - 条件改 `migration.FromVersion == version`（链式：只有前一步成功才走下一步）；
   - 任何一步失败：立即停止、**不写** `SchemaVersion`、`anyApplied` 如实反映、向上抛出（或至少返回失败让调用方不保存半迁移状态——需要与 `SettingsService.LoadAsync` 的恢复语义（ResilientJsonStore .bak 回退）对齐设计）；
   - 配故障注入测试：每一步 migration 人为抛异常，断言后续步骤不执行、SchemaVersion 不前进（报告 Term Sheet 条件一的验收形态）。
2. **P1 更新真实性 → 拆成"便宜立即项 + 决策项"**：
   - 立即项（代码级，建议并入 Round 2.5）：`IsManifestUsable` 强制 HTTPS + 域名白名单（deskbox.fun / github.com）——把报告 §3 指出的缺口先堵一半；
   - 决策项（Simon 拍板，涉及成本与流程）：Authenticode 签名 + updater 固定 publisher 校验 + signed manifest（minisign/TUF 风格）+ SBOM/provenance。企业版前为硬门槛，消费级可分期。
3. **P1 release gate 治理 → 决策项**：把 distribution-audit（x64+ARM64+安装包审计）挂到 release tag 事件并设为不可绕过（required check）。机制已存在，只差组织强制——报告"能力已具备、未制度化"的判断准确。
4. **P2 Settings mutable graph → 长期架构项**（mutation gateway / snapshot API），排在身份统一层之后，商业化前完成。当前"UI 线程约定"仍成立，不紧急。
5. **P2 CI AppSec → 渐进清单**（CodeQL、cargo audit、Action pin 到 SHA、SBOM），随 release 治理一起排。
6. **P2 Rust lib.rs 拆分 / panic=abort**：记录为技术债方向，与 pluginization 存档结论一致，不单独立项。

### 修正后的路线图
- **Round 2.5（新增，尽快）**：migration fail-open 修复 + 每步故障注入测试 + manifest HTTPS/域名白名单。小、急、独立可验收。
- **Round 3（调整后）**：故障注入矩阵为主体（`sourceExists || destinationExists` 永恒断言 + 迁移/回滚/取消各注入点）+ FileIdentitySnapshot 统一层（Watcher/FileService 两套 ByHandleFileInformation 合一 + 目录 manifest 上 FileKey + Manual org 统一身份）。
- **决策项清单（Simon）**：签名链方案与预算、distribution gate 挂 tag、CI AppSec 渐进项、Settings mutation gateway 排期。
- **前置提醒**：工作树已压四批未提交改动 + 并行会话批次，建议先验收提交（Round 2.5 很小，可搭车同一批）。

## 三、对报告评分与投资结论的态度（供 Simon 参考）

评分维度与结论（PASS WITH CONDITIONS、7.2/10、"文件完整性是最高产品 invariant 的信号"）与我们三次专项审计的体感一致，无夸大。其"创始人从高产开发者进化为工程治理负责人"的命题，与本仓近两周的实际轨迹（审计→核对→分轮修复→故障注入计划）方向吻合。报告自我声明的审查边界（静态红队、未做动态攻击）诚实，应予采信。
