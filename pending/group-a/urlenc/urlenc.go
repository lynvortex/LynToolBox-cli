//go:build ignore

// 暂存区：未集成/未完成实现的工具源码，不参与编译。

// Package urlenc 实现 URL 编解码命令。
// 对应网页版：network/url-encode-tool.html（URL编解码）
//
// 用法：
//
//	lyntoolbox urlenc [文本]
//	lyntoolbox urlenc -d "%E4%BD%A0%E5%A5%BD"
//	lyntoolbox urlenc -path "a/b c"
package urlenc

import (
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
)

const (
	Name  = "urlenc"
	Desc  = "URL 百分号编码与解码，支持查询串与路径两种模式"
	Usage = `用法: lyntoolbox urlenc [-d] [-path] [-strict] [文本]

参数:
  -d       解码模式（默认编码）
  -path    使用路径编码规则（/ 不转义；默认按查询串规则，空格转义为 +）
  -strict  解码时遇到非法转义序列直接报错（默认保留原文继续输出）
  文本     待处理的文本；省略时从 stdin 读取`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	decode := fs.Bool("d", false, "解码模式")
	pathMode := fs.Bool("path", false, "路径编码规则")
	strict := fs.Bool("strict", false, "解码失败时报错")
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
		s := string(data)
		var (
			out string
			err error
		)
		if *pathMode {
			out, err = url.PathUnescape(s)
		} else {
			out, err = url.QueryUnescape(s)
		}
		if err != nil {
			if *strict {
				fmt.Fprintln(os.Stderr, "解码失败:", err)
				return 1
			}
			out = s // 非严格模式：无法解码的部分保留原文
		}
		result = out
	} else {
		if *pathMode {
			result = url.PathEscape(string(data))
		} else {
			result = url.QueryEscape(string(data))
		}
	}
	fmt.Println(result)
	return 0
}
