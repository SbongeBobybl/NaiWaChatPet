# 03 · 历史文档筛查与优化跟进（docs/ 全量，2026-10-01）

> 范围：docs/ 全树 + 根目录 md，共 **151 份**（tracked 143 + 未提交新档 5 + 根 8）。基线 HEAD `9d5313da` + 未提交工作区。判定口径：对码核对文档的具体主张（类名/计数/路径/流程），失实=需更新；被取代=应归档；不可变历史=有效。

---

## 一、三件最高优先（两处结构事故 + 缺口）★均已主会话复核实锤

### A1.【结构事故】architecture-optimization-progress-20260922.md 整文件双份拼接
- 第 1–1338 行是活副本（批 1–51 全量）；**第 1340 行又出现第二个同名 H1**，重复刊载批 1–28 旧快照（更新时间停在 2026-09-23），共 1919 行。
- 未提交的"2026-09-29 文件安全专项"段落被**追加到了陈旧副本尾部**（第 1918 行）而非活副本。
- **动作**：删第 1340 行起的重复块，把 09-29 专项段并入活副本。优先级：**发版前必须**（这是 agent 入口文档之一，会直接误导新会话）。

### A2.【CHANGELOG 事故+缺口】Unreleased 段重复两次 + 下版内容大量缺失
- `CHANGELOG.md` 第 3 行与第 18 行是**两个逐字相同的 `## Unreleased`**（各只含热键说明条一条，已进 main）。
- Unreleased 完全没有覆盖：已修的三处 UI 回归（2b6b2e0d，用户可见）；未提交的收纳迁移复制化（重大行为变化：交互式移动→复制+校验+保留原件）、拖出三档设置+Win10 语义变化、静默启动、自动整理档位、3 新动画效果+邻屏钳制、Onboarding 重构。
- **动作**：合并为一个 Unreleased；按上清单补条目（dragfix2 的 Win10 语义变化须写明：Move 档跨卷也变移动、copy-only 接收端拒收）。**发版前必须**。

### A3.【缺失文档】三处用户可见变更零文档覆盖
| 缺失 | 证据 | 动作 |
|---|---|---|
| 静默启动 | docs 全树 grep "静默启动/SilentStartup" 0 命中 | `startup-policy.md` 加一节或新文档 + CHANGELOG |
| 自动整理档位 | `docs/requirements/desktop-auto-organization.md` 停在 2026-07-31，无档位内容；该文档自我约束"实现偏差必须先更新本文"——已属违约 | 增补"处理延迟档位"小节（默认 10s、六档、归一化、到达豁免 60min 余量） |
| 新动画效果+邻屏钳制 | `adaptive-widget-animation.md` 只覆盖帧调度；`WidgetSlideBoundaryPolicy` 无记载 | 扩写效果清单与边界语义；合入后校正 `[重要勿删]widget_zorder_lifecycle.md` 的唤起序列 |

---

## 二、分组明细

### 根目录（8 份）
| 文档 | 判定 | 动作 |
|---|---|---|
| CHANGELOG.md | **需更新**（A2） | 发版前必须 |
| README.md / README.zh-CN.md | 有效（钉在 1.5.5 与已发布状态一致） | 下次发版随版本升 |
| AGENTS.md / CONTRIBUTING / CODE_OF_CONDUCT / SECURITY / SUPPORT | 有效 | 无 |

### docs/architecture（29 份）
| 文档 | 判定 | 动作 | 优先级 |
|---|---|---|---|
| architecture-optimization-progress-20260922.md | **结构事故**（A1） | 去重+段落归位 | 发版前 |
| [重要勿删]file_drag_stack_contract.md | 有效（工作树版已按方案 B 重写与代码一致；§8.1.1b"已归档勿复活"自洽） | 随提交入库；§12 可补 `FileSurfaceContent.DragSafety.cs` 索引行 | 随提交 |
| widget-group-residency-roadmap-20260918.md | **需更新**：头部仍写"未立项实施"，实际 P0 已结案跳过 Cold、P1 适配器已落地（#432/#433） | 头部加终态指针，防重开已裁决线 | 高 |
| current_architecture.md | 有效（09-29 快照与代码对齐） | 无 | — |
| json-source-generation-baseline.md | 有效（工作树版含迁移恢复记录登记，与新代码一致） | 随提交入库 | 随提交 |
| startup-policy.md | 基本有效（未覆盖静默启动，主题不同） | 互链或加一节（A3） | 中 |
| native_context_menu_hosting.md | **需确认**：状态停在 09-12"待真机矩阵"，功能已随 1.5.4/1.5.5 出货但矩阵无完成证据 | 向 Simon 确认后改状态行 | 中 |
| development-plan-20260919.md | **应归档/加注**：版本表仍写 1.5.5 未发；1.6.0 云同步未启动（实际走了架构线） | 文首加历史声明或移 stage-reports | 中 |
| widget_contribution_seam.md | 已完成待归档（已由适配器基座落地） | 加落地注或移 stage-reports | 低 |
| cloud-backup-audit / sync-protocol-contract / distribution_channel_workflow / release_submission_scope / store_msix_build_notes / residency-p0-attribution / drop_on_shortcut_open | 有效 | 无 | — |
| rust-native-aot-roadmap.md | 有效但**测试钉住**（9 个 AotStage 测试逐字断言） | 更新须同步 9 个测试 | 钉住 |
| 6 份 native ABI 契约 | 有效（其中 recycle-bin-v1、shortcut-v2 被测试钉住） | 不动 | 钉住×2 |
| [重要勿删]widget_zorder_lifecycle.md | **需更新两处**：引用不存在的 `widget_layer_workspace_plan.md`（死链）；唤起序列描述在动画批合入后会漂移 | 删死链；动画批合入时校正序列 | 中 |
| file-drag-source-safety-20260929.md（未提交） | 有效（拖出主文档；Win10 清单待复测勾选） | 随提交入库 | 随提交 |
| managed-storage-copy-migration-20260929.md（未提交） | 有效（结尾如实登记"Debug 重启验证未完成"） | 补做后更新验证节 | 随提交 |
| file-drag-explorer-move-reclaim-20260929.md（未提交） | 已撤销方案存档，但撤销日期写成 **2026-10-12（未来日期笔误）** | 修日期后归档 | 低 |

### docs/architecture/stage-reports（57 份）
- 47 份 AOT/Rust 阶段报告 = 不可变历史，其中 **9 份被契约测试逐字断言**。
- `pr-merge-window-review-20260929.md` **需修引用**：引用的两份前置文档（`architecture-optimization-review-20260924.md`、`pr-stack-review-20260925.md`）仓库与磁盘均不存在 → 改为"未入库"或补归档。

### docs/articles（27 份）
- **00–14 号功能文系统性过刊**（全部早于 1.5.5，缺两版内容）：最痛的四篇 = `13-backup`（**0 次提及云备份/WebDAV**——1.5.5 旗舰功能）、`05-global-wake`（无拖出三档）、`09-capsule`（无展开方向）、`12-appearance`（无阴影/修剪）。中优先级，建议排一次过刊更新。
- `15-getting-started.md`：有效但**被 OnboardingExperienceTests 断言文本钉住**（"默认拖入行为是移动"），改动须同步测试。
- `16-troubleshooting.md`：可补"Win10 拖出被 copy-only 应用拒收→切换复制档"条目（随拖出批）。
- 9 篇带日期审计/事故文 = 不可变历史，全部有效；两篇未提交（win10-drag-out-incident、file-drag-batch-dual-review）随提交入库。

### docs/requirements（4 份）
- `desktop-auto-organization.md` **需更新**（A3，自我约束已违约）；`adaptive-widget-animation.md` **需更新**（A3）；另两份有效。

### docs/releases（21 份）/ press-kit / support / images / baselines
- releases 全部有效不可变；`docs/support/crash-dumps.md` **被 LocalDumpsSupportContractTests 钉住**；images README 截图为 1.5.x 时期（下次大版本可换）；`docs/architecture/evidence/` 约 20 个文件被 3 个 AotStage 测试引用不可动。

---

## 三、悬空引用汇总（追链失败点）
1. architecture-optimization-progress ×6 处引用 `architecture-final-candidate-review-20260924.md` —— 不存在。
2. pr-merge-window-review 引用两份 09-24/25 审计 —— 不存在。
3. widget_zorder_lifecycle 引用 `widget_layer_workspace_plan.md` —— 不存在。
- 推测原在已被 gitignore 且现已不存在的 `docs/architecture/archive/` —— **归档约定已断链**：要么恢复 archive/，要么把引用改"未入库"。

## 四、测试钉住文档完整清单（动前必查）
| 文档 | 钉住测试 |
|---|---|
| rust-native-aot-roadmap.md | AotStage5B4C1A/C1B1/C1B2A/C1B2B/C1C1/C3B2A/C3B2B1/C3B2B2A、AotStage7A（9 个） |
| stage-reports/aot-stage-5b-4c1a 等 8 份 | 对应 AotStage5B4*ContractTests |
| stage-reports/rust-stage-7a-arm64-static-report.md | AotStage7AContractTests |
| shortcut-native-abi-v2.md | AotPublishContractTests |
| recycle-bin-native-abi-v1.md | AotStage5B4C1B1ContractTests |
| docs/support/crash-dumps.md | LocalDumpsSupportContractTests |
| articles/15-getting-started.md | OnboardingExperienceTests |
| architecture/evidence/ 二进制 | 3 个 AotStage 测试 |

## 五、计数汇总
有效（含不可变历史）约 112 份；**需更新 9 份**（progress 去重、CHANGELOG、residency 头部、auto-organization 需求、animation 需求、articles×4 过刊 + development-plan + widget_zorder + native_context_menu ≈ 11–12 项动作）；**应归档 3 份**；**应删除 0 份**（progress 是删块非删文件）；**测试钉住 14 份**；未提交新档 5 份覆盖拖出+迁移两主线。

**最容易误导新会话的三份**：① progress-tracker（双份内容+新旧"下一批"并存）；② residency-roadmap 头部"未立项"（可能重开已裁决线）；③ development-plan 版本表（1.5.5"未发"）。
