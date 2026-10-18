# CodexPet V12

本目录基于用户提供的 V11.1 完整源代码继续开发。保留的 `README.md` 是历史重构背景记录；当前使用与构建说明以源码包根目录 `USER_GUIDE.md`、`ARCHITECTURE.md` 和 `BUILD_WINDOWS.ps1` 为准。

Windows x64 原生 Win32/GDI 应用；Go 1.23+，无第三方 Go 依赖，角色资源完整内嵌。运行最终 EXE 不需要开发运行时。视频制作工具的 Python/FFmpeg 依赖仅用于开发。

用户后续要求已覆盖最初任务书的内存 Key 限制：现在 API Key 与成功文本问答使用当前 Windows 用户 DPAPI 加密持久化，截图与观察授权不恢复。详情见文档。
