// Package wordcount 实现字数统计命令。
// 对应网页版：text/word-count-tool.html（字数统计）
//
// 用法：
//
//	lyntoolbox wordcount [-json] [-f 文件] [文本]
//	echo 你好 world | lyntoolbox wordcount
package wordcount

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	Name  = "wordcount"
	Desc  = "统计字符/中文/单词/行数/段落等多种文本指标"
	Usage = `用法: lyntoolbox wordcount [-json] [-f 文件路径] [文本]

参数:
  -f     从文件读取文本（UTF-8）
  -json  以 JSON 格式输出统计结果
  文本   待统计文本；省略且无 -f 时从 stdin 读取

统计口径:
  总字符数   UTF-8 字符（rune）数
  非空白字符 不含空白（空格/制表/换行等）的字符数
  中文字符   CJK 统一表意文字区段（含扩展A/兼容区）
  英文单词   连续拉丁字母序列（[A-Za-z]+）个数
  行数       与 wc 一致：换行符个数，末行无换行时另计 1
  非空行数   去除首尾空白后仍有内容的行
  段落数     以空行分隔的非空文本块`
)

type stats struct {
	Chars      int `json:"chars"`      // 总字符数
	NonSpace   int `json:"non_space"`  // 不含空白字符数
	CJK        int `json:"cjk"`        // 中文字符数
	Words      int `json:"words"`      // 英文单词数
	Lines      int `json:"lines"`      // 行数
	NonEmpty   int `json:"non_empty"`  // 非空行数
	Paragraphs int `json:"paragraphs"` // 段落数
	Bytes      int `json:"bytes"`      // 字节大小
}

func isCJK(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF: // CJK 基本区
		return true
	case r >= 0x3400 && r <= 0x4DBF: // 扩展 A
		return true
	case r >= 0xF900 && r <= 0xFAFF: // 兼容表意文字
		return true
	case r >= 0x20000 && r <= 0x2EBEF: // 扩展 B~F
		return true
	}
	return false
}

func countStats(data []byte) stats {
	var s stats
	s.Bytes = len(data)
	text := string(data)
	s.Chars = utf8.RuneCountInString(text)

	for _, r := range text {
		if !isSpaceRune(r) {
			s.NonSpace++
		}
		if isCJK(r) {
			s.CJK++
		}
	}

	// 英文单词：连续拉丁字母序列
	inWord := false
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			if !inWord {
				s.Words++
				inWord = true
			}
		} else {
			inWord = false
		}
	}

	// 行数：换行符计数，末行无换行且非空补 1
	s.Lines = strings.Count(text, "\n")
	if len(data) > 0 && !strings.HasSuffix(text, "\n") {
		s.Lines++
	}

	// 非空行数与段落数
	inPara := false
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) != "" {
			s.NonEmpty++
			if !inPara {
				s.Paragraphs++
				inPara = true
			}
		} else {
			inPara = false
		}
	}
	return s
}

func isSpaceRune(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\v', '\f', 0x85, 0xA0:
		return true
	}
	return r > 0xFF && isUnicodeSpace(r)
}

func isUnicodeSpace(r rune) bool {
	// 常见 Unicode 空白
	switch r {
	case 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}

// reorderFlags 将 flag 参数挪到位置参数之前，使 "位置参数 -flag" 与 "-flag 位置参数" 两种顺序均可解析。
// valueFlags 为需要消费下一个参数的 flag 名集合。
func reorderFlags(args []string, valueFlags map[string]bool) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 1 && a[0] == '-' {
			if _, err := strconv.ParseFloat(a, 64); err == nil { // 负数视为位置参数
				pos = append(pos, a)
				continue
			}
			name := strings.TrimLeft(a, "-")
			if !strings.Contains(name, "=") && valueFlags[name] && i+1 < len(args) {
				flags = append(flags, a, args[i+1])
				i++
				continue
			}
			flags = append(flags, a)
			continue
		}
		pos = append(pos, a)
	}
	return append(flags, pos...)
} // Run 执行命令，返回退出码。
func Run(args []string) int {
	args = reorderFlags(args, map[string]bool{"f": true})
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "JSON 输出")
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

	s := countStats(data)

	if *jsonOut {
		b, err := json.Marshal(s)
		if err != nil {
			fmt.Fprintln(os.Stderr, "JSON 编码失败:", err)
			return 1
		}
		fmt.Println(string(b))
		return 0
	}

	fmt.Printf("总字符数:     %d\n", s.Chars)
	fmt.Printf("不含空白字符: %d\n", s.NonSpace)
	fmt.Printf("中文字符数:   %d\n", s.CJK)
	fmt.Printf("英文单词数:   %d\n", s.Words)
	fmt.Printf("行数:         %d\n", s.Lines)
	fmt.Printf("非空行数:     %d\n", s.NonEmpty)
	fmt.Printf("段落数:       %d\n", s.Paragraphs)
	fmt.Printf("字节大小:     %d\n", s.Bytes)
	return 0
}
