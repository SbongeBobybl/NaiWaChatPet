# CodexPet V13

本目录来自用户提供的 V12 完整源码 ZIP，并在该基线上升级。历史 README 和 README_V12 保留用于追溯；当前说明以包根目录 USER_GUIDE.md、TEST_REPORT.md、ANIMATION_AUDIT.md 和 BUILD_WINDOWS.ps1 为准。

Go 1.23+，原生 Windows x64 Win32/GDI，无第三方 Go 依赖。运行包包含内嵌原动画的 EXE 和经校验的 animations 目录，无需开发运行时。

源码包包含原视频、RGBA clip、制作工具、固定的 200 条情绪拟定场景、待人工审核表与机器结果。用户已暂缓人工验收，不能把模板回归成绩作为真实人工泛化准确率。

从原视频重新制作：设置 FFMPEG 路径，运行 tools/audit_videos.py，再编译 tools/matte.go 为制作工具并处理原始帧，运行 tools/prepare_emotions.py；该脚本默认不标为 reviewed。完成逐帧审核后，显式使用 --approve-reviewed 生成可激活清单。预置运行帧已经完成本次资源合法性和 Win32 加载检查。
