// Package zhconv 实现中文转换命令：简繁互转与汉字注音。
// 对应网页版：text/chinese-convert-tool.html（中文简繁转换）、text/pinyin-tool.html（汉字转拼音）
//
// 用法：
//
//	lyntoolbox zhconv s2t 简体文本
//	lyntoolbox zhconv t2s -f trad.txt
//	lyntoolbox zhconv pinyin 汉字文本
package zhconv

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/longbridgeapp/opencc"
	"github.com/mozillazg/go-pinyin"
)

const (
	Name  = "zhconv"
	Desc  = "中文转换：简体↔繁体（OpenCC）与汉字注音（无声调/带声调/首字母）"
	Usage = `用法: lyntoolbox zhconv <子命令> [-t] [-f first] [-file 文件] [文本]

子命令:
  s2t     简体 → 繁体（OpenCC s2t.json 配置）
  t2s     繁体 → 简体（OpenCC t2s.json 配置）
  pinyin  汉字注音：默认无声调、空格分隔，非汉字原样保留

参数:
  -t      pinyin 子命令：拼音带声调
  -f      pinyin 子命令：-f first 启用首字母模式（每个汉字取拼音首字母）
  -file   从文件读取文本；省略文本与 -file 时从 stdin 读取
说明:
  s2t/t2s 忽略 -t/-f；pinyin 的非汉字内容原样保留，连续非汉字归为一段
示例:
  lyntoolbox zhconv s2t 简体中文
  lyntoolbox zhconv t2s -file trad.txt
  lyntoolbox zhconv pinyin 中国人 hello
  lyntoolbox zhconv pinyin -t -f first 春眠不觉晓`
)

// runPinyin 汉字注音：汉字逐字转拼音，连续非汉字字符保留为一段（首尾空白并入分隔）。
func runPinyin(text string, tone, firstLetter bool) {
	a := pinyin.NewArgs()
	if tone {
		a.Style = pinyin.Tone
	}
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if seg := strings.TrimSpace(cur.String()); seg != "" {
			tokens = append(tokens, seg)
		}
		cur.Reset()
	}
	for _, r := range text {
		if !unicode.Is(unicode.Han, r) {
			cur.WriteRune(r)
			continue
		}
		flush()
		py := pinyin.SinglePinyin(r, a)
		if len(py) == 0 || py[0] == "" {
			tokens = append(tokens, string(r))
			continue
		}
		if firstLetter {
			tokens = append(tokens, string([]rune(py[0])[0]))
		} else {
			tokens = append(tokens, py[0])
		}
	}
	flush()
	fmt.Println(strings.Join(tokens, " "))
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	tone := fs.Bool("t", false, "pinyin: 拼音带声调")
	first := fs.String("f", "", "pinyin: first 表示首字母模式")
	file := fs.String("file", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }

	// 子命令放在最前时先摘出，保证其后仍可写 flag（如 zhconv pinyin -t 文本）
	var sub string
	parseArgs := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub = args[0]
		parseArgs = args[1:]
	}
	if err := fs.Parse(parseArgs); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	rest := fs.Args()
	if sub == "" {
		if len(rest) == 0 {
			fmt.Fprintln(os.Stderr, "错误: 缺少子命令，可用值 s2t | t2s | pinyin")
			return 2
		}
		sub, rest = rest[0], rest[1:]
	}

	var data []byte
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取文件失败:", err)
			return 1
		}
		data = b
	} else if len(rest) > 0 {
		data = []byte(strings.Join(rest, " "))
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 stdin 失败:", err)
			return 1
		}
		data = b
	}
	text := string(data)

	switch sub {
	case "s2t", "t2s":
		cc, err := opencc.New(sub)
		if err != nil {
			fmt.Fprintln(os.Stderr, "初始化转换器失败:", err)
			return 1
		}
		out, err := cc.Convert(text)
		if err != nil {
			fmt.Fprintln(os.Stderr, "转换失败:", err)
			return 1
		}
		if strings.HasSuffix(out, "\n") {
			fmt.Print(out)
		} else {
			fmt.Println(out)
		}
		return 0
	case "pinyin":
		runPinyin(text, *tone, strings.EqualFold(strings.TrimSpace(*first), "first"))
		return 0
	default:
		fmt.Fprintf(os.Stderr, "错误: 未知子命令 %q，可用值 s2t | t2s | pinyin\n", sub)
		return 2
	}
}
