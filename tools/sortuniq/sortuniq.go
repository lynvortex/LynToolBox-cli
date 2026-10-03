// Package sortuniq 实现行排序去重命令。
// 对应网页版：text/sort-uniq-tool.html（排序去重）
//
// 用法：
//
//	lyntoolbox sortuniq -u -n -f numbers.txt
//	cat words.txt | lyntoolbox sortuniq -i -len
//	lyntoolbox sortuniq -r -f list.txt
package sortuniq

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	Name  = "sortuniq"
	Desc  = "行排序去重，支持数值排序、忽略大小写、按长度排序与反序"
	Usage = `用法: lyntoolbox sortuniq [-u] [-n] [-r] [-i] [-len] [-f 文件] [文本]

参数:
  -u    去重（排序后去重；-i 时按忽略大小写判定相同）
  -n    按行首数值排序（无数字的行按 0 处理）
  -r    反序输出
  -i    比较与去重时忽略大小写
  -len  按行长度（字符数）排序，可与其他键叠加作为次级键
  -f    从文件读取文本；省略文本与 -f 时从 stdin 读取
说明:
  多键排序顺序为：数值(-n) → 长度(-len) → 文本（-i 忽略大小写）
示例:
  lyntoolbox sortuniq -u -n -f numbers.txt
  cat words.txt | lyntoolbox sortuniq -i -len
  lyntoolbox sortuniq -r "b
  a
  c"`
)

// leadingNumber 提取行首的数值（跳过前导空白），无数字时返回 0。
func leadingNumber(line string) float64 {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	j := i
	if j < len(line) && (line[j] == '+' || line[j] == '-') {
		j++
	}
	start := j
	for j < len(line) && line[j] >= '0' && line[j] <= '9' {
		j++
	}
	if j < len(line) && line[j] == '.' {
		j++
		for j < len(line) && line[j] >= '0' && line[j] <= '9' {
			j++
		}
	}
	if j == start {
		return 0
	}
	v, err := strconv.ParseFloat(line[i:j], 64)
	if err != nil {
		return 0
	}
	return v
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	uniq := fs.Bool("u", false, "去重")
	numeric := fs.Bool("n", false, "按行首数值排序")
	reverse := fs.Bool("r", false, "反序输出")
	ignoreCase := fs.Bool("i", false, "忽略大小写")
	byLen := fs.Bool("len", false, "按行长度排序")
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

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	// 去掉末尾因文件换行产生的空元素，保留中间的空行
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}

	key := func(line string) string {
		if *ignoreCase {
			return strings.ToLower(line)
		}
		return line
	}

	less := func(a, b string) bool {
		if *numeric {
			na, nb := leadingNumber(a), leadingNumber(b)
			if na != nb {
				return na < nb
			}
		}
		if *byLen {
			la, lb := utf8.RuneCountInString(a), utf8.RuneCountInString(b)
			if la != lb {
				return la < lb
			}
		}
		ka, kb := key(a), key(b)
		if ka != kb {
			return ka < kb
		}
		return false
	}

	sort.SliceStable(lines, func(x, y int) bool {
		if *reverse {
			return less(lines[y], lines[x])
		}
		return less(lines[x], lines[y])
	})

	prev := ""
	first := true
	for _, line := range lines {
		if *uniq && !first && key(line) == key(prev) {
			continue
		}
		fmt.Println(line)
		prev = line
		first = false
	}
	return 0
}
