// Package regext 实现正则表达式测试命令。
// 对应网页版：text/regex-tool.html（正则测试）、work/regex-cheat-tool.html（正则速查）
//
// 用法：
//
//	lyntoolbox regext -p "正则" [文本]
//	lyntoolbox regext -p "正则" -f 文件
//	lyntoolbox regext -p "正则" -replace "[$1]" [文本]
//	lyntoolbox regext -cheat
package regext

import (
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	Name  = "regext"
	Desc  = "正则表达式测试：匹配位置与分组、批量替换、内置 Go(RE2) 中文速查表"
	Usage = `用法: lyntoolbox regext -p 正则 [-flag imsU] [-replace 替换串] [-only-count] [-f 文件] [文本]
      lyntoolbox regext -cheat

参数:
  -p          正则表达式（必需）
  -flag       附加标志，可组合: i 忽略大小写 / m 多行 / s 点号匹配换行 / U 非贪婪
  -replace    执行全局替换并输出结果，支持 $1、${name} 引用分组
  -only-count 只输出匹配数量
  -f          从文件读取文本；省略文本与 -f 时从 stdin 读取
  -cheat      输出正则中文速查表（Go regexp/RE2）后退出
示例:
  lyntoolbox regext -p "(\d{4})-(\d{2})" "2024-01-02 2024-03-04"
  lyntoolbox regext -p "(?P<user>\w+)@\w+" -replace "${user}#*" "a@b.com"
  echo hello | lyntoolbox regext -p "l+" -only-count`
)

const cheatSheet = `正则速查表（Go regexp / RE2）

【字符类】
  [abc]    a、b 或 c                [^abc]   除 a、b、c 外任意字符
  [a-z]    a 到 z                   [a-zA-Z0-9] 区间可组合
  .        任意字符（默认不含换行，加 s 标志后包含）

【预定义字符类】
  \d   数字 [0-9]                   \D   非数字
  \w   单词字符 [0-9A-Za-z_]        \W   非单词字符
  \s   空白符 [\t\n\f\r ]           \S   非空白符
  \p{Han}   Unicode 汉字（\p{L} 任意字母、\p{N} 任意数字）

【量词】
  *        0 次或多次（贪婪）       *?       0 次或多次（非贪婪）
  +        1 次或多次（贪婪）       +?       1 次或多次（非贪婪）
  ?        0 次或 1 次              ??       非贪婪
  {n}      恰好 n 次                {n,}     至少 n 次
  {n,m}    n 到 m 次                {n,m}?   非贪婪

【分组与引用】
  (re)          捕获分组，替换串中用 $1 引用
  (?P<name>re)  命名捕获分组，替换串中用 ${name} 引用
  (?:re)        非捕获分组
  (?i)re        就地设置标志（如 (?i)abc 忽略大小写）

【边界断言】
  ^   文本开头（m 标志下为每行开头）    $   文本结尾（m 标志下为每行结尾）
  \A  文本开头（不受 m 影响）           \z  文本结尾（不受 m 影响）
  \b  ASCII 单词边界                    \B  非单词边界

【RE2 标志】
  i  忽略大小写     m  多行模式（^ $ 匹配行首行尾）
  s  让 . 匹配换行  U  非贪婪（交换 x* 与 x*? 的含义）

【RE2 不支持】
  反向引用（\1）、环视断言（lookahead/lookbehind）、回溯、递归。
  Go regexp 保证线性时间，语法错误的正则会在编译期报错。`

// splitArgs 将本命令的已知旗标记号提前到参数列表最前端，
// 使旗标可以写在任意位置，同时保证以 - 开头的文本参数
// 不会被 flag 包误认为旗标。
func splitArgs(args []string) ([]string, []string) {
	spec := map[string]bool{"p": true, "flag": true, "replace": true, "only-count": false, "f": true, "cheat": false}
	var flags, pos []string
	i := 0
	for ; i < len(args); i++ {
		t := args[i]
		if t == "--" {
			i++
			break
		}
		if strings.HasPrefix(t, "-") && strings.TrimLeft(t, "-") != "" {
			name := strings.TrimLeft(t, "-")
			hasValue := false
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				name, hasValue = name[:eq], true
			}
			if takes, known := spec[name]; known {
				flags = append(flags, t)
				if !hasValue && takes && i+1 < len(args) {
					i++
					flags = append(flags, args[i])
				}
				continue
			}
			if name == "h" || name == "help" {
				flags = append(flags, t)
				continue
			}
		}
		pos = append(pos, t)
	}
	pos = append(pos, args[i:]...)
	return flags, pos
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	pattern := fs.String("p", "", "正则表达式")
	flagStr := fs.String("flag", "", "附加标志（i/m/s/U）")
	replace := fs.String("replace", "", "替换串")
	onlyCount := fs.Bool("only-count", false, "只输出匹配数量")
	file := fs.String("f", "", "输入文件")
	cheat := fs.Bool("cheat", false, "输出速查表")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	flags, pos := splitArgs(args)
	if err := fs.Parse(flags); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *cheat {
		fmt.Println(cheatSheet)
		return 0
	}
	if strings.TrimSpace(*pattern) == "" {
		fmt.Fprintln(os.Stderr, "缺少 -p 正则表达式")
		return 2
	}

	expr := *pattern
	if *flagStr != "" {
		var b strings.Builder
		b.WriteString("(?")
		for _, c := range *flagStr {
			switch c {
			case 'i', 'm', 's', 'U':
				b.WriteRune(c)
			default:
				fmt.Fprintf(os.Stderr, "不支持的标志: %c（可选 i/m/s/U）\n", c)
				return 2
			}
		}
		b.WriteString(")")
		expr = b.String() + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "正则编译失败: %v\n", err)
		return 1
	}

	var text string
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取文件失败:", err)
			return 1
		}
		text = string(b)
	} else if len(pos) > 0 {
		text = strings.Join(pos, "\n")
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 stdin 失败:", err)
			return 1
		}
		text = string(b)
	}

	matches := re.FindAllStringSubmatchIndex(text, -1)
	if *onlyCount {
		fmt.Println(len(matches))
		return 0
	}
	names := re.SubexpNames()
	for i, m := range matches {
		start, end := m[0], m[1]
		sl, sc := lineCol(text, start)
		el, ec := lineCol(text, end)
		fmt.Printf("#%d [行%d:列%d - 行%d:列%d] %q\n", i+1, sl, sc, el, ec, text[start:end])
		for g := 1; g*2 < len(m); g++ {
			label := fmt.Sprintf("$%d", g)
			if names[g] != "" {
				label += "(" + names[g] + ")"
			}
			if m[2*g] < 0 {
				fmt.Printf("    %s = 未参与\n", label)
				continue
			}
			fmt.Printf("    %s = %q\n", label, text[m[2*g]:m[2*g+1]])
		}
	}
	fmt.Printf("共 %d 处匹配\n", len(matches))

	// -replace 显式给出（含空替换串）时执行全局替换并输出结果。
	replaceSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "replace" {
			replaceSet = true
		}
	})
	if replaceSet {
		fmt.Println("替换结果:")
		fmt.Println(re.ReplaceAllString(text, *replace))
	}
	return 0
}

// lineCol 返回字节偏移对应的 1 起始行号与列号（列按字符计）。
func lineCol(text string, off int) (int, int) {
	if off > len(text) {
		off = len(text)
	}
	line, start := 1, 0
	for i := 0; i < off; i++ {
		if text[i] == '\n' {
			line++
			start = i + 1
		}
	}
	col := utf8.RuneCountInString(text[start:off]) + 1
	return line, col
}
