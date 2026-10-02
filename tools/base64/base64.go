// Package base64 实现 Base64 编解码命令。
// 对应网页版：text/base64-tool.html（Base64编解码）
//
// 用法：
//
//	lyntoolbox base64 [-d] [-url] [-file 路径] [文本]
//	echo abc | lyntoolbox base64
//	lyntoolbox base64 -d -file img.b64 -o out.png
package base64

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	Name  = "base64"
	Desc  = "文本/文件与 Base64 快速互转，支持 URL 安全字母表"
	Usage = `用法: lyntoolbox base64 [-d] [-url] [-file 路径] [-o 输出] [文本]

参数:
  -d        解码模式（默认编码）
  -url      使用 URL 安全字母表（- 和 _）
  -file     从文件读取（编码读原始字节，解码读文本）
  -o        输出到文件（默认打印到终端）
  文本      待处理的文本；省略且无 -file 时从 stdin 读取`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	decode := fs.Bool("d", false, "解码模式")
	urlSafe := fs.Bool("url", false, "URL 安全字母表")
	file := fs.String("file", "", "输入文件")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var data []byte
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取文件失败:", err)
			return 1
		}
		data = b
	} else if fs.NArg() > 0 {
		data = []byte(strings.Join(fs.Args(), " "))
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 stdin 失败:", err)
			return 1
		}
		data = b
	}

	enc := base64.StdEncoding
	if *urlSafe {
		enc = base64.URLEncoding
	}

	var result []byte
	if *decode {
		clean := strings.TrimSpace(string(data))
		clean = strings.NewReplacer("\r", "", "\n", "", "\t", "", " ", "").Replace(clean)
		dec, err := enc.DecodeString(clean)
		if err != nil {
			fmt.Fprintln(os.Stderr, "解码失败:", err)
			return 1
		}
		result = dec
	} else {
		result = []byte(enc.EncodeToString(data))
	}

	if *out != "" {
		if err := os.WriteFile(*out, result, 0644); err != nil {
			fmt.Fprintln(os.Stderr, "写入文件失败:", err)
			return 1
		}
		fmt.Printf("已写入 %s（%d 字节）\n", *out, len(result))
		return 0
	}
	os.Stdout.Write(result)
	if !*decode {
		fmt.Println()
	}
	return 0
}
