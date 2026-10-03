// Package textdiff 实现文本对比（Diff）命令。
// 对应网页版：text/diff-tool.html（文本Diff）
//
// 用法：
//
//	lyntoolbox textdiff a.txt b.txt
//	lyntoolbox textdiff old.txt - -u 5 -color
//	lyntoolbox textdiff -a a.txt -b b.txt -u 0
package textdiff

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	Name  = "textdiff"
	Desc  = "对比两段文本差异，统一风格输出，支持上下文行数与彩色高亮"
	Usage = `用法: lyntoolbox textdiff [-u N] [-color] [-a 文件A] [-b 文件B] [文件A] [文件B]

参数:
  -a      文本 A 的文件路径
  -b      文本 B 的文件路径（-a 与 -b 需同时提供，优先于位置参数）
  -u      上下文行数，默认 3；-u 0 输出全量差异（不折叠上下文）
  -color  彩色输出（ANSI：绿色新增、红色删除）
  位置参数 文件A 文件B，"-" 表示从 stdin 读取
输出:
  公共行前缀空格，删除行前缀 -，新增行前缀 +，变更块以 @@ 头分隔；
  末尾输出统计（N 处变更）
退出码:
  0 无差异  1 有差异  2 参数错误
示例:
  lyntoolbox textdiff a.txt b.txt
  lyntoolbox textdiff old.txt - -u 5 -color < new.txt`
)

const ansiReset = "\x1b[0m"
const ansiRed = "\x1b[31m"
const ansiGreen = "\x1b[32m"
const ansiCyan = "\x1b[36m"

// diffOp 表示对齐后的一行操作：' ' 公共、'-' 删除、'+' 新增。
type diffOp struct {
	kind byte
	text string
}

func readSource(name string) ([]byte, error) {
	if name == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(name)
}

func splitLines(data []byte) []string {
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	if strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// diffOps 用行级 LCS 对齐 a、b，返回完整的操作序列。
func diffOps(a, b []string) []diffOp {
	n, m := len(a), len(b)
	// 先去掉公共前后缀，缩小动态规划规模
	p := 0
	for p < n && p < m && a[p] == b[p] {
		p++
	}
	s := 0
	for s < n-p && s < m-p && a[n-1-s] == b[m-1-s] {
		s++
	}
	ops := make([]diffOp, 0, p+(n-p)+(m-p)+s)
	for i := 0; i < p; i++ {
		ops = append(ops, diffOp{' ', a[i]})
	}

	ca, cb := a[p:n-s], b[p:m-s]
	ln, lm := len(ca), len(cb)
	if ln > 0 && lm > 0 && ln*lm <= 20_000_000 {
		// dp[i][j] = ca[i:] 与 cb[j:] 的 LCS 长度
		dp := make([][]int32, ln+1)
		for i := range dp {
			dp[i] = make([]int32, lm+1)
		}
		for i := ln - 1; i >= 0; i-- {
			for j := lm - 1; j >= 0; j-- {
				switch {
				case ca[i] == cb[j]:
					dp[i][j] = dp[i+1][j+1] + 1
				case dp[i+1][j] >= dp[i][j+1]:
					dp[i][j] = dp[i+1][j]
				default:
					dp[i][j] = dp[i][j+1]
				}
			}
		}
		i, j := 0, 0
		for i < ln && j < lm {
			switch {
			case ca[i] == cb[j]:
				ops = append(ops, diffOp{' ', ca[i]})
				i++
				j++
			case dp[i+1][j] >= dp[i][j+1]:
				ops = append(ops, diffOp{'-', ca[i]})
				i++
			default:
				ops = append(ops, diffOp{'+', cb[j]})
				j++
			}
		}
		for ; i < ln; i++ {
			ops = append(ops, diffOp{'-', ca[i]})
		}
		for ; j < lm; j++ {
			ops = append(ops, diffOp{'+', cb[j]})
		}
	} else {
		// 中段过大时退化为整块替换，避免内存爆炸
		for _, line := range ca {
			ops = append(ops, diffOp{'-', line})
		}
		for _, line := range cb {
			ops = append(ops, diffOp{'+', line})
		}
	}

	for i := 0; i < s; i++ {
		ops = append(ops, diffOp{' ', a[n-s+i]})
	}
	return ops
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	fileA := fs.String("a", "", "文本 A 文件")
	fileB := fs.String("b", "", "文本 B 文件")
	ctx := fs.Int("u", 3, "上下文行数，0 表示全量")
	color := fs.Bool("color", false, "彩色输出")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	var nameA, nameB string
	switch {
	case *fileA != "" && *fileB != "":
		nameA, nameB = *fileA, *fileB
	case *fileA != "" || *fileB != "":
		fmt.Fprintln(os.Stderr, "错误: -a 与 -b 必须同时提供")
		return 2
	case fs.NArg() == 2:
		nameA, nameB = fs.Arg(0), fs.Arg(1)
	default:
		fmt.Fprintln(os.Stderr, "错误: 需要两个位置参数（文件A 文件B，\"-\" 为 stdin），或同时提供 -a 与 -b")
		return 2
	}

	dataA, err := readSource(nameA)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取文本 A 失败:", err)
		return 1
	}
	dataB, err := readSource(nameB)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取文本 B 失败:", err)
		return 1
	}

	ops := diffOps(splitLines(dataA), splitLines(dataB))

	// 统计：变更块数与增删行数
	blocks, add, del := 0, 0, 0
	prev := byte(' ')
	for _, op := range ops {
		switch op.kind {
		case '+':
			add++
		case '-':
			del++
		}
		if op.kind != ' ' && prev == ' ' {
			blocks++
		}
		prev = op.kind
	}

	for _, h := range collectHunks(ops, *ctx) {
		aNo, bNo := 0, 0
		for j := 0; j < h.lo; j++ {
			if ops[j].kind != '+' {
				aNo++
			}
			if ops[j].kind != '-' {
				bNo++
			}
		}
		aCnt, bCnt := 0, 0
		for j := h.lo; j < h.hi; j++ {
			if ops[j].kind != '+' {
				aCnt++
			}
			if ops[j].kind != '-' {
				bCnt++
			}
		}
		aStart, bStart := aNo+1, bNo+1
		if aCnt == 0 {
			aStart = aNo
		}
		if bCnt == 0 {
			bStart = bNo
		}
		head := fmt.Sprintf("@@ %s +%s @@",
			formatRange(aStart, aCnt), formatRange(bStart, bCnt))
		if *color {
			head = ansiCyan + head + ansiReset
		}
		fmt.Println(head)
		for _, op := range ops[h.lo:h.hi] {
			line := string(op.kind) + op.text
			switch {
			case *color && op.kind == '+':
				line = ansiGreen + line + ansiReset
			case *color && op.kind == '-':
				line = ansiRed + line + ansiReset
			}
			fmt.Println(line)
		}
	}
	fmt.Printf("统计: %d 处变更（+%d 行 / -%d 行）\n", blocks, add, del)

	if blocks > 0 {
		return 1
	}
	return 0
}

type hunkRange struct{ lo, hi int }

// collectHunks 把操作序列折叠成带上下文的变更块；ctx<=0 时返回单个全量块。
func collectHunks(ops []diffOp, ctx int) []hunkRange {
	n := len(ops)
	if n == 0 {
		return nil
	}
	if ctx <= 0 {
		return []hunkRange{{0, n}}
	}
	var hs []hunkRange
	i := 0
	for i < n {
		if ops[i].kind == ' ' {
			i++
			continue
		}
		lo := i - ctx
		if lo < 0 {
			lo = 0
		}
		j, last := i, i
		for j < n {
			if ops[j].kind != ' ' {
				last = j
				j++
				continue
			}
			k := j
			for k < n && ops[k].kind == ' ' {
				k++
			}
			if k < n && k-last <= 2*ctx+1 {
				j = k
				continue
			}
			break
		}
		hi := last + 1 + ctx
		if hi > n {
			hi = n
		}
		hs = append(hs, hunkRange{lo, hi})
		i = hi
	}
	return hs
}

// formatRange 输出 unified diff 的行号范围：1 行省略数量，0 行取前一行的行号。
func formatRange(start, count int) string {
	switch {
	case count == 1:
		return fmt.Sprintf("%d", start)
	case count == 0:
		return fmt.Sprintf("%d,0", start)
	default:
		return fmt.Sprintf("%d,%d", start, count)
	}
}
