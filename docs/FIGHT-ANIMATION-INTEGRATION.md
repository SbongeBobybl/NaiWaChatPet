# 用户骨骼动画接入记录

资源来源：本地 `naiwa-skills-skeletal.tar.gz`。

归档 SHA256：`39cbbf6ec5ca7cc3c4d43e8839dc8c077b03228282377b34f540cf7d02fe1cc7`。

原归档及解压后的原始资源均保留。`source/rig/skilldata` 中的骨骼 JSON、atlas、图片、验证基准由原文件逐字节复制；原始关键帧、骨骼层级和贴图没有重绘或重新生成。

## 适配范围

原资源为 Spine 4.2.43 JSON，包含 region/point attachments、层级骨骼、translate/rotate/scale 时间线、RGBA/attachment 时间线、线性/stepped/Bezier 曲线。

`rig.SkillAnimations` 为本项目独立读取器，沿用 `rig.Affine` 与画布接口，不引入 Spine 运行时库。它支持该资源实际使用的 region/point 子集；遇到不支持的骨骼轨道、附件类型、版本、缺失骨骼或图片会给出具体错误。旧的 Stand/Stance `rig.Puppet` 与用户骨骼拓扑不同，因此采用并行读取适配和渲染混合，未修改旧骨架。

| 动作 | 原始动画 | 时长 | 逻辑事件 |
|---|---|---|---|
| 飞扇 | skill_fan_throw | 65/60 秒 | 原始 fan_release=0.25s，按 hand_near 锚点生成弹体 |
| 翻滚 | skill_roll | 1 秒 | 原始 roll_start≈0.166667s，roll_land≈0.833333s；期间攻击有效 |
| 强化 | skill_power_up | 65/60 秒 | 原资源无生效标记，独立配置默认第 20 tick 生效 |
| 强化循环 | skill_power_up_loop | 1 秒 | 原始循环可由读取器播放；永久强化状态由战斗核心保存 |
| 闪避 | 尚未提供 | 逻辑默认 0.30 秒 | 无敌第 0–10 tick，冷却为 0；正式骨骼视觉待接入 |

飞扇资源内 `fan_explode` 是演示用事件，不直接结算游戏伤害。脱手后使用独立弹体，只有实际碰撞才生成程序爆炸。原始 JSON 中演示弹体、预设爆炸槽位仍保持原样，仅在游戏渲染时屏蔽这些表现槽位，避免画面预演与实际命中重复。

翻滚根节点原始 X 轨迹从 0 到 -110。逻辑层逐 tick 读取相邻根位移，转换为朝向相关的世界移动；渲染时去除已应用的根平移，防止位置重复移动。边界与对手阻挡仍由战斗核心约束。

## 接口

第二版只在翻滚紧缩阶段将显示摄像变换放大到 1.30 倍，起手与结束平滑还原为 1.00 倍。玩家和对手使用同一变换；原始骨骼、关键帧、根运动和碰撞不变。

`LoadSkillAnimations`、`RegisterSkillAnimations`、`PlaySkillAnimation`、`StopSkillAnimation`、`GetAnimationDuration`、`GetAnimationProgress`、`GetAnimationEvents`、`GetBoneAnchor`、`RootTranslation`、`BlendToExistingPose`。

外部加载示例：

```powershell
.\NaiWaChatPet.exe -skill-assets .\user-skills\skeletal -combat-rules .\combat.json
```

目录中需保留 `naiwa.json` 和相对的 `images/*.png`。atlas 原件保留供审阅，读取器使用原始单张 region 图片，无需重新打包图集。三项正式技能使用上表的动画名。

未来提供 `skill_dodge` 时，读取器会自动注册该动作。若资源包含 `invulnerable_start` / `invulnerable_end`，将优先使用其时间标记；否则使用独立配置。自定义资产应先运行集成测试并同步服务器使用的逻辑时间配置。

## 验证

自动化测试检查载入、时长、完整帧区间、事件只触发一次、停止后事件清理、手部锚点、根位移、透明像素、缩放后恢复原 rig，以及原资源附带的 spine-core 4.2.43 验证数据。

738 个参考骨骼变换通过：最大矩阵分量差约 0.00102，最大位置差约 0.1363 个原始画布像素。差异来自有限精度采样；原始曲线未改动。关键姿态接触图已导出到测试报告目录。

已接入 U/I/O 的正式资源；L 闪避的正式动作资产缺失。真实 GUI 下的人工逐帧视觉、左右朝向、各种显示器与 DPI、音效主观体验需要继续人工验收，不能由离屏渲染或运行探针替代。

## 第五版一技能显示校准

`play/combat.json` 的 fan 定义增加 `displayScale: 1.12`。校准围绕相同地面锚点进行，默认 `displayOffsetY: 0`，玩家和对手使用相同显示参数；服务端按相同比例转换手部发射点。旧配置省略字段时也应用该修复。用户原始 JSON、关键帧、贴图和骨骼层级未修改，原始 `GetBoneAnchor` 仍返回作者坐标。
