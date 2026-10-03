//go:build windows

// Package deps 的平台限定锁定项：
// windows 包只在 GOOS=windows 下参与构建，放这里避免破坏 linux/darwin 编译。
package deps

import (
	_ "golang.org/x/sys/windows"
)
