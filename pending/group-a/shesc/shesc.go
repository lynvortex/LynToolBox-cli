//go:build ignore

// 暂存区：未集成/未完成实现的工具源码，不参与编译。

// Package shesc 实现 Shell 命令行转义命令。
// 对应网页版：work/shell-escape-tool.html（Shell转义）
//
// 用法：
//
//	lyntoolbox shesc -mode posix "it's ok"
//	lyntoolbox shesc -mode cmd 'a & b'
//	lyntoolbox shesc -mode powershell "don't"
package shesc

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	Name  = "shesc"
	Desc  = "按 POSIX Shell / cmd / PowerShell 规则转义字符串"
	Usage = `用法: lyntoolbox shesc [-mode posix|cmd|powershell] [文本]

参数:
  -mode  目标 Shell，取值 posix（默认）、cmd、powershell
  文本   待转义的文本；省略时从 stdin 读取`
)

// escapePosix 用单引号包裹，内部单引号按 '\” 处理。
func escapePosix(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// escapeCmd 用双引号包裹，内部 ^ % ! 前加 ^，双引号写成两个。
func escapeCmd(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '^', '%', '!':
			b.WriteByte('^')
			b.WriteRune(r)
		case '"':
			b.WriteString(`""`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// escapePowerShell 用单引号包裹，内部单引号双写。
func escapePowerShell(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	mode := fs.String("mode", "posix", "目标 Shell: posix|cmd|powershell")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var data []byte
	if fs.NArg() > 0 {
		data = []byte(strings.Join(fs.Args(), " "))
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 stdin 失败:", err)
			return 1
		}
		data = b
	}

	var result string
	switch strings.ToLower(*mode) {
	case "posix":
		result = escapePosix(string(data))
	case "cmd":
		result = escapeCmd(string(data))
	case "powershell":
		result = escapePowerShell(string(data))
	default:
		fmt.Fprintf(os.Stderr, "不支持的模式: %s（可选 posix|cmd|powershell）\n", *mode)
		return 2
	}
	fmt.Println(result)
	return 0
}
