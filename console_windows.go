//go:build windows

package main

import "golang.org/x/sys/windows"

// initConsole 将 Windows 控制台切换到 UTF-8 代码页，
// 避免默认 GBK 代码页下中文输出乱码。
func initConsole() {
	_ = windows.SetConsoleOutputCP(65001)
	_ = windows.SetConsoleCP(65001)
}
