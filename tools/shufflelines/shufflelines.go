// Package shufflelines 实现文本打乱命令。
// 对应网页版：text/shuffle-tool.html（文本打乱）
//
// 用法：
//
//	lyntoolbox shufflelines [-mode line] [-u] [-n 10] [-seed 42] [-f 文件] [文本]
//	cat list.txt | lyntoolbox shufflelines -seed 7
package shufflelines

import (
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
)

const (
	Name  = "shufflelines"
	Desc  = "随机打乱文本行/单词/字符顺序，支持去重、抽样与固定种子"
	Usage = `用法: lyntoolbox shufflelines [-mode line|word|char] [-u] [-n N] [-seed 种子] [-f 文件] [文本]

参数:
  -mode  打乱粒度：line 按行（默认）、word 行内单词、char 行内字符
  -u     打乱前先按行去重（保留首次出现顺序）
  -n     打乱后抽取前 N 条（行模式下为行数）
  -seed  固定随机种子，结果可复现；省略则每次随机
  -f     从文件读取；省略且无参数时从 stdin 读取
  文本   多个参数按多行处理（每个参数一行）`
)

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
	args = reorderFlags(args, map[string]bool{"mode": true, "n": true, "seed": true, "f": true})
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	mode := fs.String("mode", "line", "打乱粒度 line|word|char")
	dedup := fs.Bool("u", false, "先去重")
	take := fs.Int("n", 0, "抽取前 N 条（0 表示全部）")
	seed := fs.Int64("seed", 0, "随机种子（0 表示随机）")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *mode != "line" && *mode != "word" && *mode != "char" {
		fmt.Fprintf(os.Stderr, "无效的 -mode: %s（可选 line|word|char）\n", *mode)
		return 2
	}
	if *take < 0 {
		fmt.Fprintln(os.Stderr, "-n 不能为负数")
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
		data = []byte(strings.Join(fs.Args(), "\n"))
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 stdin 失败:", err)
			return 1
		}
		data = b
	}

	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := splitLines(text)

	// 行内打乱模式：逐行打乱单词/字符，行顺序不变
	if *mode == "word" || *mode == "char" {
		rng := newRng(*seed)
		for i, line := range lines {
			if *mode == "word" {
				words := strings.Fields(line)
				rng.Shuffle(len(words), func(a, b int) { words[a], words[b] = words[b], words[a] })
				lines[i] = strings.Join(words, " ")
			} else {
				chars := strings.Split(line, "")
				rng.Shuffle(len(chars), func(a, b int) { chars[a], chars[b] = chars[b], chars[a] })
				lines[i] = strings.Join(chars, "")
			}
		}
		for _, l := range lines {
			fmt.Println(l)
		}
		return 0
	}

	// 行模式
	if *dedup {
		lines = dedupLines(lines)
	}
	rng := newRng(*seed)
	rng.Shuffle(len(lines), func(a, b int) { lines[a], lines[b] = lines[b], lines[a] })
	if *take > 0 && *take < len(lines) {
		lines = lines[:*take]
	}
	for _, l := range lines {
		fmt.Println(l)
	}
	return 0
}

// splitLines 按行切分，去掉末尾空行。
func splitLines(text string) []string {
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func dedupLines(lines []string) []string {
	seen := make(map[string]bool, len(lines))
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if seen[l] {
			continue
		}
		seen[l] = true
		out = append(out, l)
	}
	return out
}

// newRng 按种子创建随机数发生器；seed 为 0 时使用随机源。
func newRng(seed int64) *rand.Rand {
	if seed == 0 {
		return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	return rand.New(rand.NewPCG(uint64(seed), uint64(seed)+0x9E3779B97F4A7C15))
}
