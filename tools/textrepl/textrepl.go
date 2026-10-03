// Package textrepl 实现批量查找替换命令。
// 对应网页版：text/replace-tool.html（批量替换）
//
// 用法：
//
//	lyntoolbox textrepl foo bar -f input.txt
//	lyntoolbox textrepl -regex "(\d{4})-(\d{2})" "[$2/$1]" -f log.txt
//	echo "Foo foo" | lyntoolbox textrepl -i foo baz
package textrepl

import (
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const (
	Name  = "textrepl"
	Desc  = "批量查找替换，支持字面与正则模式、忽略大小写与替换计数"
	Usage = `用法: lyntoolbox textrepl [-regex] [-i] [-count] [-f 文件] [-o 输出] <pattern> <replacement>

参数:
  pattern     查找内容（-regex 时为 Go regexp 语法）
  replacement 替换内容（-regex 时支持 $1、${name} 引用分组，字面模式原样输出）
  -regex      按正则解释 pattern 与 replacement
  -i          忽略大小写（-regex 时等价于加 (?i) 前缀）
  -count      只输出替换次数，不输出替换后的文本
  -f          从文件读取待处理文本；省略时从 stdin 读取
  -o          结果写入文件（默认打印到终端）
示例:
  lyntoolbox textrepl foo bar -f input.txt
  lyntoolbox textrepl -regex "(\d{4})-(\d{2})" "[$2/$1]" -f log.txt
  echo "Foo foo" | lyntoolbox textrepl -i foo baz`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	regex := fs.Bool("regex", false, "正则模式")
	ignoreCase := fs.Bool("i", false, "忽略大小写")
	countOnly := fs.Bool("count", false, "只输出替换次数")
	file := fs.String("f", "", "输入文件")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "错误: 需要两个位置参数 <pattern> <replacement>")
		return 2
	}
	pattern, replacement := fs.Arg(0), fs.Arg(1)

	var data []byte
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取文件失败:", err)
			return 1
		}
		data = b
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 stdin 失败:", err)
			return 1
		}
		data = b
	}
	text := string(data)

	var count int
	var result string
	switch {
	case *regex:
		expr := pattern
		if *ignoreCase {
			expr = "(?i)" + expr
		}
		re, err := regexp.Compile(expr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "正则表达式无效:", err)
			return 1
		}
		count = len(re.FindAllStringIndex(text, -1))
		result = re.ReplaceAllString(text, replacement)
	case *ignoreCase:
		re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(pattern))
		if err != nil {
			fmt.Fprintln(os.Stderr, "内部错误:", err)
			return 1
		}
		count = len(re.FindAllStringIndex(text, -1))
		result = re.ReplaceAllLiteralString(text, replacement)
	default:
		count = strings.Count(text, pattern)
		result = strings.ReplaceAll(text, pattern, replacement)
	}

	if *countOnly {
		fmt.Println(count)
		return 0
	}

	if *out != "" {
		if err := os.WriteFile(*out, []byte(result), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "写入文件失败:", err)
			return 1
		}
		fmt.Printf("已写入 %s（替换 %d 处，%d 字节）\n", *out, count, len(result))
		return 0
	}
	fmt.Print(result)
	return 0
}
