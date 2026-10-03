// Package cloc 实现代码行数统计命令。
// 对应网页版：work/code-stats-tool.html（代码统计）
//
// 用法：
//
//	lyntoolbox cloc ./src
//	lyntoolbox cloc main.go util.go
//	lyntoolbox cloc ./project -exclude generated,testdata
package cloc

import (
	"flag"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const (
	Name  = "cloc"
	Desc  = "统计代码行数：按语言输出文件数/代码/注释/空行，支持目录递归与排除"
	Usage = `用法: lyntoolbox cloc <文件或目录>... [-exclude 子串列表] [-f 文件]

参数:
  路径      一个或多个文件/目录（目录递归统计）
  -exclude  按路径子串排除（逗号分隔；默认 node_modules,.git,vendor,dist）

说明:
  内置常见扩展名→语言与注释规则（//、/* */、#、<!-- -->、-- 等）；
  简化处理：字符串字面量按"去除引号内容后判断注释"识别，跨行字符串与
  某些边界情况可能误判；无扩展名或未知类型计入"其他"。`
)

// langRules 语言注释规则。
type langRules struct {
	name   string
	line   []string    // 行注释起始
	block  [][2]string // 块注释 [开, 闭]
	quotes []byte      // 需要屏蔽的字符串引号
}

var extLang = map[string]string{
	"go": "Go", "js": "JavaScript", "mjs": "JavaScript", "cjs": "JavaScript",
	"jsx": "JavaScript", "ts": "TypeScript", "tsx": "TypeScript",
	"py": "Python", "java": "Java", "c": "C", "h": "C/C++头文件",
	"cpp": "C++", "cc": "C++", "cxx": "C++", "hpp": "C/C++头文件",
	"cs": "C#", "php": "PHP", "rb": "Ruby", "rs": "Rust", "sql": "SQL",
	"html": "HTML", "htm": "HTML", "css": "CSS", "scss": "SCSS",
	"xml": "XML", "svg": "XML", "json": "JSON", "yaml": "YAML",
	"yml": "YAML", "md": "Markdown", "sh": "Shell", "bash": "Shell",
	"ps1": "PowerShell", "bat": "Batch", "cmd": "Batch", "vue": "Vue",
}

func rulesFor(lang string) langRules {
	cLike := langRules{line: []string{"//"}, block: [][2]string{{"/*", "*/"}}, quotes: []byte{'"', '\'', '`'}}
	hashLike := langRules{line: []string{"#"}, quotes: []byte{'"', '\''}}
	switch lang {
	case "Go":
		return cLike
	case "JavaScript", "TypeScript":
		return cLike
	case "C", "C++", "C/C++头文件", "C#", "Java", "Rust", "PHP", "SCSS":
		return cLike
	case "Python", "Ruby", "YAML", "Shell":
		return hashLike
	case "PowerShell":
		return langRules{line: []string{"#"}, block: [][2]string{{"<#", "#>"}}, quotes: []byte{'"', '\''}}
	case "Batch":
		return langRules{line: []string{"rem", "::"}, quotes: []byte{'"'}}
	case "SQL":
		return langRules{line: []string{"--"}, block: [][2]string{{"/*", "*/"}}, quotes: []byte{'\''}}
	case "HTML", "XML", "Vue", "Markdown":
		return langRules{line: nil, block: [][2]string{{"<!--", "-->"}}, quotes: []byte{}}
	default: // JSON / 其他：无注释
		return langRules{quotes: []byte{'"', '\''}}
	}
}

// stats 单语言统计。
type stats struct {
	files   int
	code    int
	comment int
	blank   int
}

// stripStrings 用空格替换引号包裹内容（保持长度不变，便于按位定位注释）。
func stripStrings(line string, quotes []byte) string {
	b := []byte(line)
	i := 0
	for i < len(b) {
		c := b[i]
		q := byte(0)
		for _, cand := range quotes {
			if c == cand {
				q = cand
				break
			}
		}
		if q == 0 {
			i++
			continue
		}
		j := i + 1
		for j < len(b) {
			if b[j] == '\\' {
				b[j] = ' '
				if j+1 < len(b) {
					b[j+1] = ' '
				}
				j += 2
				continue
			}
			if b[j] == q {
				break
			}
			b[j] = ' '
			j++
		}
		if j < len(b) {
			b[j] = ' ' // 引号本身也屏蔽
		}
		i = j + 1
		if i < 0 {
			i = len(b)
		}
	}
	return string(b)
}

// countLines 统计一段文本的代码/注释/空行（简化规则，每物理行只计入一类）。
func countLines(text string, r langRules) (code, comment, blank int) {
	inBlock := false
	blockEnd := ""
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, "\r")
		if inBlock {
			if idx := strings.Index(line, blockEnd); idx < 0 {
				comment++
				continue
			} else {
				comment++
				line = line[idx+len(blockEnd):]
				inBlock = false
			}
		}
		trimmed := strings.TrimSpace(stripStrings(line, r.quotes))
		if trimmed == "" {
			if strings.TrimSpace(line) == "" {
				blank++
			} else {
				code++ // 行内只有字符串字面量
			}
			continue
		}
		isLine := false
		for _, m := range r.line {
			if strings.HasPrefix(trimmed, m) ||
				strings.HasPrefix(strings.ToLower(trimmed), strings.ToLower(m)) {
				isLine = true
				break
			}
		}
		if isLine {
			comment++
			continue
		}
		// 行首块注释（可连续多段）
		for {
			var pair [2]string
			found := false
			for _, p := range r.block {
				if strings.HasPrefix(trimmed, p[0]) {
					pair = p
					found = true
					break
				}
			}
			if !found {
				break
			}
			after := trimmed[len(pair[0]):]
			if end := strings.Index(after, pair[1]); end >= 0 {
				trimmed = strings.TrimSpace(after[end+len(pair[1]):])
				if trimmed == "" {
					break
				}
				continue
			}
			inBlock = true
			blockEnd = pair[1]
			trimmed = ""
			break
		}
		if trimmed == "" {
			comment++
			continue
		}
		code++
	}
	return
}

// displayWidth 计算字符串显示宽度（CJK 记 2）。
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hangul, r) ||
			unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) ||
			(r >= 0xFF00 && r <= 0xFF60) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

func pad(s string, width int) string {
	d := width - displayWidth(s)
	if d <= 0 {
		return s
	}
	return s + strings.Repeat(" ", d)
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	exclude := fs.String("exclude", "node_modules,.git,vendor,dist", "排除的路径子串（逗号分隔）")
	stdinMode := fs.String("f", "", "从文件统计单个文本")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *stdinMode != "" {
		data, err := os.ReadFile(*stdinMode)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取文件失败:", err)
			return 1
		}
		lang := "其他"
		if ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(*stdinMode)), "."); ext != "" {
			if l, ok := extLang[ext]; ok {
				lang = l
			}
		}
		r := rulesFor(lang)
		code, comment, blank := countLines(string(data), r)
		printTable(map[string]*stats{lang: {files: 1, code: code, comment: comment, blank: blank}})
		return 0
	}

	paths := fs.Args()
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "缺少统计路径（文件或目录）")
		return 2
	}
	excludes := strings.Split(*exclude, ",")

	result := map[string]*stats{}
	skipBinary := []string{}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "访问路径失败:", err)
			return 1
		}
		if !info.IsDir() {
			addFile(result, &skipBinary, p, excludes, false)
			continue
		}
		err = filepath.WalkDir(p, func(path string, d iofs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != p {
					for _, ex := range excludes {
						ex = strings.TrimSpace(ex)
						if ex != "" && strings.Contains(path, ex) {
							return filepath.SkipDir
						}
					}
				}
				return nil
			}
			for _, ex := range excludes {
				ex = strings.TrimSpace(ex)
				if ex != "" && strings.Contains(path, ex) {
					return nil
				}
			}
			addFile(result, &skipBinary, path, excludes, false)
			return nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "遍历目录失败:", err)
			return 1
		}
	}
	for _, name := range skipBinary {
		fmt.Fprintf(os.Stderr, "跳过二进制文件: %s\n", name)
	}
	printTable(result)
	return 0
}

func addFile(result map[string]*stats, skipBinary *[]string, path string, excludes []string, _ bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: 读取失败 %s: %v\n", path, err)
		return
	}
	if containsNUL(data) { // 简单二进制判定
		*skipBinary = append(*skipBinary, path)
		return
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	lang, ok := extLang[ext]
	if !ok {
		lang = "其他"
	}
	r := rulesFor(lang)
	code, comment, blank := countLines(string(data), r)
	st := result[lang]
	if st == nil {
		st = &stats{}
		result[lang] = st
	}
	st.files++
	st.code += code
	st.comment += comment
	st.blank += blank
}

func containsNUL(data []byte) bool {
	// 检查前 8KB 是否含 NUL，足以判定常见二进制
	n := len(data)
	if n > 8192 {
		n = 8192
	}
	for i := 0; i < n; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

func printTable(result map[string]*stats) {
	type row struct {
		lang  string
		files int
		code  int
		com   int
		blank int
	}
	var rows []row
	for lang, st := range result {
		rows = append(rows, row{lang, st.files, st.code, st.comment, st.blank})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].code != rows[j].code {
			return rows[i].code > rows[j].code
		}
		return rows[i].lang < rows[j].lang
	})

	headers := []string{"语言", "文件数", "代码", "注释", "空行"}
	widths := make([]int, 5)
	for i, h := range headers {
		widths[i] = displayWidth(h)
	}
	rowsFmt := make([][]string, 0, len(rows))
	tot := row{lang: "合计"}
	for _, r := range rows {
		strs := []string{r.lang, fmt.Sprint(r.files), fmt.Sprint(r.code), fmt.Sprint(r.com), fmt.Sprint(r.blank)}
		rowsFmt = append(rowsFmt, strs)
		for i, s := range strs {
			if w := displayWidth(s); w > widths[i] {
				widths[i] = w
			}
		}
		tot.files += r.files
		tot.code += r.code
		tot.com += r.com
		tot.blank += r.blank
	}
	totalStrs := []string{tot.lang, fmt.Sprint(tot.files), fmt.Sprint(tot.code), fmt.Sprint(tot.com), fmt.Sprint(tot.blank)}

	line := func(cells []string) string {
		var b strings.Builder
		for i, c := range cells {
			b.WriteString(pad(c, widths[i]))
			b.WriteString("  ")
		}
		return strings.TrimRight(b.String(), " ")
	}
	sep := func() string {
		var b strings.Builder
		for i := range widths {
			b.WriteString(strings.Repeat("-", widths[i]+2))
		}
		return b.String()
	}

	fmt.Println(line(headers))
	fmt.Println(sep())
	for _, strs := range rowsFmt {
		fmt.Println(line(strs))
	}
	if len(rowsFmt) > 0 {
		fmt.Println(sep())
	}
	fmt.Println(line(totalStrs))
}
