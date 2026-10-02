//go:build !windows

package main

// initConsole 非 Windows 平台默认 UTF-8，无需处理。
func initConsole() {}
