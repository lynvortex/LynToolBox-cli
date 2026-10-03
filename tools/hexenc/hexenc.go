// Package hexenc 实现文本与十六进制互转命令。
// 对应网页版：text/16jinzhi-tool.html（文本/16进制转换）
//
// 用法：
//
//	lyntoolbox hexenc [文本]
//	lyntoolbox hexenc -d "e4 bd a0"
//	lyntoolbox hexenc -upper -sp "" [文本]
package hexenc

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	Name  = "hexenc"
	Desc  = "文本与十六进制字符串互转（按 UTF-8 字节）"
	Usage = `用法: lyntoolbox hexenc [-d] [-sp 分隔符] [-upper] [文本]

参数:
  -d      解码模式（默认编码）
  -sp     十六进制字节之间的分隔符（默认空格；传空字符串表示不分隔）
  -upper  编码输出大写字母（默认小写）
  文本    待处理的文本；省略时从 stdin 读取`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	decode := fs.Bool("d", false, "解码模式")
	sp := fs.String("sp", " ", "字节分隔符")
	upper := fs.Bool("upper", false, "输出大写")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
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

	if *decode {
		// 去掉分隔符与所有空白字符后再整体解析
		clean := strings.Map(func(r rune) rune {
			switch r {
			case ' ', '\t', '\r', '\n', ':', ',':
				return -1
			}
			return r
		}, string(data))
		b, err := hex.DecodeString(strings.TrimSpace(clean))
		if err != nil {
			fmt.Fprintln(os.Stderr, "解码失败:", err)
			return 1
		}
		os.Stdout.Write(b)
		fmt.Println()
		return 0
	}

	parts := make([]string, len(data))
	for i, b := range data {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	sep := *sp
	result := strings.Join(parts, sep)
	if *upper {
		result = strings.ToUpper(result)
	}
	fmt.Println(result)
	return 0
}
