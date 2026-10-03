// Package slug 实现 URL Slug 生成命令。
// 对应网页版：text/slug-tool.html（Slug生成）
//
// 用法：
//
//	lyntoolbox slug "Hello 世界 123"
//	lyntoolbox slug -sep _ -max 20 "重构：用户登录模块"
//	lyntoolbox slug -t "Spring 部署指南"
package slug

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/mozillazg/go-pinyin"
)

const (
	Name  = "slug"
	Desc  = "生成 URL Slug：中文转拼音，保留英文数字，非法字符转分隔符"
	Usage = `用法: lyntoolbox slug [-sep 符号] [-max N] [-t] [-f 文件] [文本]

参数:
  -sep  单词分隔符，默认 "-"
  -max  最大长度（按字符截断，0 不限制），截断后去掉尾部分隔符
  -t    拼音带声调（默认无声调）
  -f    从文件读取文本；省略文本与 -f 时从 stdin 读取
规则:
  汉字逐字转拼音（相邻汉字间插入分隔符），英文字母小写、数字保留，
  其余字符视为分隔；连续分隔合并为一个，去掉首尾分隔
示例:
  lyntoolbox slug "Hello 世界 123"
  lyntoolbox slug -sep _ -max 20 "重构：用户登录模块"
  lyntoolbox slug -t "Spring 部署指南"`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	sep := fs.String("sep", "-", "单词分隔符")
	maxLen := fs.Int("max", 0, "最大长度，0 不限制")
	tone := fs.Bool("t", false, "拼音带声调")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
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
	text := strings.TrimSpace(string(data))

	a := pinyin.NewArgs()
	if *tone {
		a.Style = pinyin.Tone
	}

	var pieces []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			pieces = append(pieces, cur.String())
			cur.Reset()
		}
	}
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r):
			flush()
			py := pinyin.SinglePinyin(r, a)
			if len(py) > 0 && py[0] != "" {
				pieces = append(pieces, py[0])
			} else {
				pieces = append(pieces, string(r))
			}
		case r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9':
			cur.WriteRune(unicode.ToLower(r))
		default:
			flush()
		}
	}
	flush()
	result := strings.Join(pieces, *sep)

	if *maxLen > 0 {
		rs := []rune(result)
		if len(rs) > *maxLen {
			result = string(rs[:*maxLen])
		}
		if *sep != "" {
			for strings.HasPrefix(result, *sep) {
				result = result[len(*sep):]
			}
			for strings.HasSuffix(result, *sep) {
				result = result[:len(result)-len(*sep)]
			}
		}
	}
	fmt.Println(result)
	return 0
}
