// Package tabspace 实现 Tab 与空格互转命令。
// 对应网页版：text/tab-space-tool.html（Tab空格转换）
//
// 用法：
//
//	lyntoolbox tabspace -f code.go
//	lyntoolbox tabspace -to tabs -w 4 -f code.go
//	lyntoolbox tabspace -all < messy.txt
package tabspace

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	Name  = "tabspace"
	Desc  = "Tab 与空格互转，支持制表宽度、行首缩进或全文转换"
	Usage = `用法: lyntoolbox tabspace [-to spaces|tabs] [-w 宽度] [-all] [-f 文件] [文本]

参数:
  -to   转换目标: spaces（默认，Tab→空格，按列对齐展开）| tabs（空格→Tab）
  -w    制表宽度，默认 4（1-16）
  -all  转换全文所有 Tab/空格（默认仅转换行首缩进）
说明:
  Tab→空格时每个 Tab 展开到下一个制表位（至少 1 个空格）；
  空格→Tab 时仅当空格从制表位起、能凑满整数个宽度才合并，余量保留空格
示例:
  lyntoolbox tabspace -f code.go
  lyntoolbox tabspace -to tabs -w 4 -f code.go
  lyntoolbox tabspace -all < messy.txt`
)

// expandLine Tab→空格：all 为真时处理整行，否则仅处理行首缩进。
func expandLine(line string, w int, all bool) string {
	runes := []rune(line)
	var b strings.Builder
	col := 0
	i := 0
	// 行首空白（空格与 Tab 都按列对齐展开）
	for i < len(runes) && (runes[i] == '\t' || runes[i] == ' ') {
		if runes[i] == '\t' {
			n := w - col%w
			b.WriteString(strings.Repeat(" ", n))
			col += n
		} else {
			b.WriteByte(' ')
			col++
		}
		i++
	}
	if all {
		for ; i < len(runes); i++ {
			if runes[i] == '\t' {
				n := w - col%w
				b.WriteString(strings.Repeat(" ", n))
				col += n
			} else {
				b.WriteRune(runes[i])
				col++
			}
		}
	} else {
		b.WriteString(string(runes[i:]))
	}
	return b.String()
}

// unexpandLine 空格→Tab：仅当空格段从制表位开始且能凑满整数个宽度时合并。
func unexpandLine(line string, w int, all bool) string {
	runes := []rune(line)
	var b strings.Builder
	col := 0
	i := 0
	convert := func() {
		// 连续空格段：从当前列起，只合并能对齐的完整宽度块
		j := i
		for j < len(runes) && runes[j] == ' ' {
			j++
		}
		if j > i && col%w == 0 {
			tabs := (j - i) / w
			rest := (j - i) % w
			b.WriteString(strings.Repeat("\t", tabs))
			b.WriteString(strings.Repeat(" ", rest))
			col += j - i
		} else {
			b.WriteString(strings.Repeat(" ", j-i))
			col += j - i
		}
		i = j
	}
	if all {
		for i < len(runes) {
			switch {
			case runes[i] == ' ':
				convert()
			case runes[i] == '\t':
				b.WriteByte('\t')
				col += w - col%w
				i++
			default:
				b.WriteRune(runes[i])
				col++
				i++
			}
		}
		return b.String()
	}
	// 默认仅行首缩进
	for i < len(runes) && (runes[i] == '\t' || runes[i] == ' ') {
		if runes[i] == '\t' {
			b.WriteByte('\t')
			col += w - col%w
			i++
		} else {
			convert()
		}
	}
	b.WriteString(string(runes[i:]))
	return b.String()
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	to := fs.String("to", "spaces", "转换目标 spaces|tabs")
	width := fs.Int("w", 4, "制表宽度")
	all := fs.Bool("all", false, "转换全文（默认仅行首缩进）")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *width < 1 || *width > 16 {
		fmt.Fprintln(os.Stderr, "错误: -w 宽度需在 1-16 之间")
		return 2
	}
	if *to != "spaces" && *to != "tabs" {
		fmt.Fprintf(os.Stderr, "错误: 未知目标 %q，可用值 spaces|tabs\n", *to)
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

	lines := strings.Split(string(data), "\n")
	convert := expandLine
	if *to == "tabs" {
		convert = unexpandLine
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = convert(line, *width, *all)
	}
	fmt.Print(strings.Join(out, "\n"))
	return 0
}
