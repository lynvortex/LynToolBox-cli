//go:build ignore

// 暂存区：未集成/未完成实现的工具源码，不参与编译。

// Package htmlent 实现 HTML 实体编解码命令。
// 对应网页版：work/html-entity-tool.html（HTML实体转换）
//
// 用法：
//
//	lyntoolbox htmlent [文本]
//	lyntoolbox htmlent -d "&lt;p&gt;"
//	lyntoolbox htmlent -num "你好"
package htmlent

import (
	"flag"
	"fmt"
	"html"
	"io"
	"os"
	"strings"
)

const (
	Name  = "htmlent"
	Desc  = "HTML 实体编码与解码，支持非 ASCII 十六进制数字实体"
	Usage = `用法: lyntoolbox htmlent [-d] [-num] [文本]

参数:
  -d    解码模式（默认编码）
  -num  编码时非 ASCII 字符输出 &#xNN; 十六进制数字实体（默认仅转义 HTML 保留字符）
  文本  待处理的文本；省略时从 stdin 读取`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	decode := fs.Bool("d", false, "解码模式")
	num := fs.Bool("num", false, "非 ASCII 输出十六进制数字实体")
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
	if *decode {
		result = html.UnescapeString(string(data))
	} else {
		result = html.EscapeString(string(data))
		if *num {
			var b strings.Builder
			for _, r := range result {
				if r > 127 {
					fmt.Fprintf(&b, "&#x%X;", r)
				} else {
					b.WriteRune(r)
				}
			}
			result = b.String()
		}
	}
	fmt.Println(result)
	return 0
}
