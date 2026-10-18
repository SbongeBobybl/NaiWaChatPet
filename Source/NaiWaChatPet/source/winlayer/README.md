# Win32 layered renderer 候选适配模块

本模块是依据需求新写的 Go/Win32 重建组件，不是恢复的 V10 源码。它只管理 DIB 和提交像素；不创建、显示或销毁窗口，不实现应用主程序或消息循环，也不修复未知原视频。

API：

```go
surface, err := winlayer.New(width, height)
err = surface.Present(hwnd, x, y, premultipliedBGRA)
err = surface.Resize(width, height)
err = surface.Close()
```

调用方创建本进程拥有的 `WS_EX_LAYERED` 窗口并控制固定 canvas。`Present` 的 `[]byte` 必须包含完整的 `width*height*4` 字节，按自上而下的行顺序排列，每像素为预乘后的 B、G、R、A。输入由 `compositor.BGRA` 等上游完成一次预乘；本适配器不会再次预乘，且不会按人物边界改变窗口或画布尺寸。完整帧覆写 DIB，包括透明留白，可避免上帧像素残留。调用方在 `Present` 返回之前不能并发修改输入 buffer；构造后不能复制 `Surface` 值。

创建 DIB 时传 `biHeight=-height`、32 bpp、`BI_RGB` 和每行 `4*width` 字节；`UpdateLayeredWindow` 使用 `ULW_ALPHA`，`BLENDFUNCTION` 为 `AC_SRC_OVER`、常量 alpha 255、`AC_SRC_ALPHA`。负高度控制 top-down RGB DIB；每像素 alpha 输入需要预乘，常量 alpha 255 保留每像素透明度。[BITMAPINFOHEADER](https://learn.microsoft.com/en-us/windows/win32/api/wingdi/ns-wingdi-bitmapinfoheader)、[BLENDFUNCTION](https://learn.microsoft.com/en-us/windows/win32/api/wingdi/ns-wingdi-blendfunction)、[UpdateLayeredWindow](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-updatelayeredwindow)。

模块用互斥锁串行访问 GDI 对象，并在涉及 DC 的方法中临时锁定 OS 线程。`GetDC(0)` 与对应 `ReleaseDC(0, hdc)` 位于同一 OS 线程；内存 DC 使用 `CreateCompatibleDC(screenDC)`，而非线程生命周期绑定的 NULL 参数。提交前后调用 `GdiFlush`，避免跨线程接管时仍有 GDI batch。原始 DIB 内存写入也需要与 GDI 完成同步。[CreateDIBSection](https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-createdibsection)、[CreateCompatibleDC](https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-createcompatibledc)、[GdiFlush](https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-gdiflush)、[ReleaseDC](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-releasedc)。

`Close` 先通过 `SelectObject` 还原原有 bitmap，再 `DeleteObject` 删除自有 DIB，最后 `DeleteDC` 删除自建内存 DC；不会删除原 bitmap 或调用方窗口。如果还原选择失败，先删除 DC，再尝试释放已不被 DC 引用的自有 bitmap。未释放成功的资源保留在对象内，调用方可再次 `Close`；重复成功 `Close` 无副作用。Win32 失败返回包含 API 名称的错误；没有可靠 last-error 值时会明确写明，避免静默失败。[SelectObject](https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-selectobject)、[DeleteObject](https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-deleteobject)、[DeleteDC](https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-deletedc)。

`Resize` 只在显式调用时更换 canvas；分配失败保留当前可用 canvas。新 canvas 创建成功后若旧资源释放失败，新尺寸已经提交，错误文本会明确标注，旧资源保留至 `Close` 重试。极少数 `New` 失败时如果资源释放也失败，会返回非 nil 的已关闭 `Surface` 和错误；调用方应再次 `Close`。正常分配失败返回 nil 和错误。单个 canvas 上限为 256 MiB，resize 期间可能同时保留新旧两个 canvas。非 Windows API 统一返回 `ErrUnsupported`。

限制：premultiplied 检查只验证各颜色通道不超过 A，这是必要条件，不能从 bytes 证明输入只预乘了一次。模块不创建窗口，不处理 DPI、命中测试、显示配置变化、消息循环、生命周期或动画时间轴。调用方仍需处理窗口生命周期、固定 canvas 对齐及失败结果。若调用方先使用 `SetLayeredWindowAttributes`，后续 `UpdateLayeredWindow` 会失败，直到 layered style 被清除再设置；需要由窗口拥有者处理相应 style 生命周期，模块不偷偷调整窗口 style。[SetLayeredWindowAttributes](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setlayeredwindowattributes)。

2026-10-09 执行 `go test -v ./winlayer` 通过。测试没有创建或显示窗口：真实 GDI DIB 创建、32 bpp/stride/透明初始化、用 `GetPixel` 读取内存 DC 验证 top-down 行序、resize、Close 和 20 次 create→resize→close 后的 GDI 对象数检查均通过。当前 Windows 的 `GetObject(DIBSECTION)` 回读高度为正，因此朝向验证使用实际行像素而不是假定回读高度保留输入符号。[GetPixel](https://learn.microsoft.com/en-us/windows/win32/api/wingdi/nf-wingdi-getpixel)。

这些结果不代表真实窗口上的 `Present` 已获验证，也不代表最终 V10 EXE 已修复。尚未进行有窗口的视觉 QA、实际动画时长测试或 DPI/多显示器测试。
