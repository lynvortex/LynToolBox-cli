//go:build ignore

// 暂存区：未集成/未完成实现的工具源码，不参与编译。

// Package minify 实现 CSS/HTML/JS 压缩命令。
// 对应网页版：work/css-minify-tool.html（CSS压缩）、work/html-minify-tool.html（HTML压缩）
//
// 用法：
//
//	lyntoolbox minify -lang css -f style.css
//	lyntoolbox minify -lang html -f page.html
//	lyntoolbox minify -lang auto -f app.js
package minify

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	Name  = "minify"
	Desc  = "CSS/HTML/JS 压缩：去注释与多余空白（CSS/HTML 去注释，JS 不做变量改名）"
	Usage = `用法: lyntoolbox minify [-lang css|html|js|auto] [-f 文件] [文本]

参数:
  -lang     css|html|js|auto（默认 auto，按内容猜测）
  -f        从文件读取
  文本      待处理内容；省略且无 -f 时从 stdin 读取

说明:
  CSS: 去注释与空白、去声明尾分号；HTML: 去注释（保留条件注释）、折叠空白
  （pre/textarea/script/style 除外）；JS: 去 // 与注释、压缩空白（字符串/
  模板字面量/正则字面量保持原样）。不进行变量改名等混淆优化。`
)

// detectLang 按内容猜测语言。
func detectLang(src string) (string, bool) {
	t := strings.TrimSpace(src)
	low := strings.ToLower(t)
	if strings.HasPrefix(low, "<!doctype") || strings.HasPrefix(low, "<?") {
		return "html", true
	}
	if strings.Contains(low, "</") && strings.Contains(low, ">") {
		return "html", true
	}
	if i := strings.IndexByte(low, '<'); i >= 0 && i+1 < len(low) &&
		low[i+1] >= 'a' && low[i+1] <= 'z' {
		return "html", true
	}
	jsSignals := []string{"function", "=>", "var ", "let ", "const ", "return",
		"console.", "document.", "window.", "if (", "if("}
	isJS := false
	for _, s := range jsSignals {
		if strings.Contains(src, s) {
			isJS = true
			break
		}
	}
	isCSS := strings.Contains(src, "{") && strings.Contains(src, "}") &&
		strings.Contains(src, ";") && strings.Contains(src, ":")
	switch {
	case isJS && !isCSS:
		return "js", true
	case isCSS && !isJS:
		return "css", true
	case isCSS && isJS:
		if strings.Contains(src, "function") || strings.Contains(src, "=>") {
			return "js", true
		}
		return "css", true
	}
	return "", false
}

// skipQuoted 跳过带转义的引号字符串，返回结束位置。
func skipQuoted(src string, i int) int {
	q := src[i]
	i++
	for i < len(src) {
		if src[i] == '\\' {
			i += 2
			continue
		}
		if src[i] == q {
			return i + 1
		}
		i++
	}
	return i
}

// skipTemplate 跳过模板字面量。
func skipTemplate(src string, i int) int {
	i++
	for i < len(src) {
		if src[i] == '\\' {
			i += 2
			continue
		}
		if src[i] == '`' {
			return i + 1
		}
		i++
	}
	return i
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	lang := fs.String("lang", "auto", "语言 css|html|js|auto")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
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
	src := string(data)
	if strings.TrimSpace(src) == "" {
		fmt.Fprintln(os.Stderr, "输入为空")
		return 1
	}

	l := strings.ToLower(*lang)
	if l == "auto" {
		guess, ok := detectLang(src)
		if !ok {
			fmt.Fprintln(os.Stderr, "无法识别语言，请用 -lang 指定 css|html|js")
			return 1
		}
		l = guess
	}

	var out string
	switch l {
	case "css":
		out = minifyCSS(src)
	case "html":
		out = minifyHTML(src)
	case "js":
		out = minifyJS(src)
	default:
		fmt.Fprintf(os.Stderr, "不支持的语言: %s（可选 css|html|js|auto）\n", *lang)
		return 2
	}
	fmt.Println(out)
	return 0
}

// ---------- CSS ----------

func cssSpaceNeeded(prev byte, cur byte) bool {
	special := "{}();:,>+~"
	if strings.IndexByte(special, cur) >= 0 || prev == 0 {
		return false
	}
	if strings.IndexByte(special, prev) >= 0 {
		return false
	}
	return true
}

// minifyCSS 去注释、压缩空白与尾分号。
func minifyCSS(src string) string {
	var b strings.Builder
	last := byte(0)
	pendingSpace := false

	i := 0
	// 直接字符级处理；'}' 前的 ';' 通过回退处理
	for i < len(src) {
		c := src[i]
		switch {
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			j := strings.Index(src[i+2:], "*/")
			if j < 0 {
				i = len(src)
			} else {
				i += 2 + j + 2
			}
			pendingSpace = false // 注释按空白删除
		case c == '"' || c == '\'':
			j := skipQuoted(src, i)
			b.WriteString(src[i:j])
			last = src[j-1]
			pendingSpace = false
			i = j
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if last != 0 && cssSpaceNeeded(last, 'x') {
				pendingSpace = true
			}
			i++
		default:
			if c == '}' {
				// 去掉 '}' 前可能存在的分号
				s := b.String()
				b.Reset()
				b.WriteString(strings.TrimRight(s, ";"))
				last = ';'
			}
			if pendingSpace && cssSpaceNeeded(last, c) {
				b.WriteByte(' ')
			}
			pendingSpace = false
			b.WriteByte(c)
			last = c
			i++
		}
	}
	return b.String()
}

// minifyHTML 去注释（保留条件注释）、折叠空白。

var htmlRawElements = map[string]bool{
	"script": true, "style": true, "pre": true, "textarea": true,
}

// minifyHTML 去注释（保留条件注释）、折叠空白。
func minifyHTML(src string) string {
	var b strings.Builder
	i := 0
	rawUntil := "" // 位于 pre/textarea/script/style 内部时记录标签名

	writeRawText := func(end int) {
		// [i, end) 原样输出
		b.WriteString(src[i:end])
		i = end
	}

	for i < len(src) {
		if rawUntil != "" {
			close := "</" + rawUntil
			low := strings.ToLower(src[i:])
			p := strings.Index(low, close)
			if p < 0 {
				writeRawText(len(src))
				rawUntil = ""
				break
			}
			writeRawText(i + p)
			rawUntil = ""
			continue
		}
		if src[i] != '<' {
			j := strings.IndexByte(src[i:], '<')
			if j < 0 {
				j = len(src)
			} else {
				j += i
			}
			// 折叠空白文本
			text := src[i:j]
			collapsed := strings.Join(strings.Fields(text), " ")
			if collapsed != "" {
				b.WriteString(collapsed)
			}
			i = j
			continue
		}
		// 标签区
		if strings.HasPrefix(src[i:], "<!--") {
			j := strings.Index(src[i+4:], "-->")
			var end int
			if j < 0 {
				end = len(src)
			} else {
				end = i + 4 + j + 3
			}
			comment := src[i:end]
			// 保留条件注释
			if strings.Contains(comment, "[if") || strings.Contains(comment, "[endif") {
				b.WriteString(comment)
			}
			if end == len(src) {
				i = end
				break
			}
			i = end
			continue
		}
		if strings.HasPrefix(src[i:], "<!") || strings.HasPrefix(src[i:], "<?") {
			j := strings.IndexByte(src[i:], '>')
			if j < 0 {
				b.WriteString(src[i:])
				break
			}
			b.WriteString(src[i : i+j+1])
			i += j + 1
			continue
		}
		if strings.HasPrefix(src[i:], "</") {
			j := strings.IndexByte(src[i:], '>')
			if j < 0 {
				b.WriteString(src[i:])
				break
			}
			b.WriteString(src[i : i+j+1])
			i += j + 1
			continue
		}
		// 开始标签：原样输出，检查 raw 元素
		j := i + 1
		for j < len(src) && src[j] != '>' {
			if src[j] == '"' || src[j] == '\'' {
				j = skipQuoted(src, j)
				continue
			}
			j++
		}
		if j >= len(src) {
			b.WriteString(src[i:])
			break
		}
		tag := src[i : j+1]
		b.WriteString(tag)
		// 解析标签名
		k := i + 1
		for k < j && src[k] != ' ' && src[k] != '\t' && src[k] != '\n' && src[k] != '\r' && src[k] != '/' {
			k++
		}
		name := strings.ToLower(src[i+1 : k])
		selfClose := strings.HasSuffix(tag, "/>")
		if !selfClose && htmlRawElements[name] {
			rawUntil = name
		}
		i = j + 1
	}
	return b.String()
}

// ---------- JS ----------

func jsIdentChar(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' ||
		c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

// minifyJS 去注释与多余空白（不做变量改名）。
func minifyJS(src string) string {
	var b strings.Builder
	last := byte(0) // 上一个输出的有效字符
	lastWord := ""  // 上一个单词（关键字判断）
	pendingSpace := false
	pendingNewline := false // 空白中含换行（ASI 保护用）

	asiKeywords := map[string]bool{
		"return": true, "throw": true, "break": true, "continue": true,
	}
	regexAfter := map[string]bool{
		"return": true, "typeof": true, "instanceof": true, "in": true,
		"of": true, "new": true, "delete": true, "void": true, "do": true,
		"else": true, "case": true,
	}

	writeOut := func(s string) {
		b.WriteString(s)
		if n := len(s); n > 0 {
			last = s[n-1]
		}
	}

	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				i = len(src)
			} else {
				i += j
			}
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			j := strings.Index(src[i+2:], "*/")
			if j < 0 {
				i = len(src)
			} else {
				i += 2 + j + 2
			}
		case c == '/' && regexStart(last, lastWord, regexAfter):
			// 正则字面量原样保留
			j := i + 1
			inClass := false
			for j < len(src) {
				if src[j] == '\\' {
					j += 2
					continue
				}
				if src[j] == '[' {
					inClass = true
				} else if src[j] == ']' {
					inClass = false
				} else if src[j] == '/' && !inClass {
					break
				} else if src[j] == '\n' {
					break
				}
				j++
			}
			if j < len(src) && src[j] == '/' {
				j++
				for j < len(src) && jsIdentChar(src[j]) { // flags
					j++
				}
			}
			writeOut(src[i:j])
			lastWord = ""
			pendingSpace = false
			i = j
		case c == '\'' || c == '"':
			j := skipQuoted(src, i)
			writeOut(src[i:j])
			lastWord = ""
			pendingSpace = false
			i = j
		case c == '`':
			j := skipTemplate(src, i)
			writeOut(src[i:j])
			lastWord = ""
			pendingSpace = false
			i = j
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if c == '\n' {
				pendingNewline = true
			}
			pendingSpace = true
			i++
		default:
			if pendingSpace {
				prevIsIdent := jsIdentChar(last)
				curIsIdent := jsIdentChar(c)
				plusmin := func(ch byte) bool { return ch == '+' || ch == '-' }
				slashEdge := last == '/' || c == '/'
				if prevIsIdent && curIsIdent {
					// return x / var name —— 必须保留空格
					if pendingNewline && asiKeywords[lastWord] {
						writeOut("\n")
					} else {
						writeOut(" ")
					}
				} else if plusmin(last) && plusmin(c) {
					writeOut(" ") // 避免 ++ / -- 误合并
				} else if slashEdge {
					writeOut(" ")
				} else if pendingNewline && asiKeywords[lastWord] && !strings.ContainsRune("{}();,.", rune(c)) {
					writeOut("\n") // return 后换行需保留
				}
			}
			pendingSpace = false
			pendingNewline = false
			prev := last
			writeOut(string(c))
			if jsIdentChar(c) {
				if jsIdentChar(prev) {
					lastWord += string(c)
				} else {
					lastWord = string(c)
				}
			} else {
				lastWord = ""
			}
			i++
		}
	}
	return b.String()
}

// regexStart 判断当前位置的 '/' 是否应视为正则字面量开始（启发式）。
func regexStart(last byte, lastWord string, regexAfter map[string]bool) bool {
	if lastWord != "" && regexAfter[lastWord] {
		return true
	}
	switch last {
	case 0, '(', ',', '=', ':', '[', '!', '&', '|', '?', '{', '}', ';', '+', '-', '*', '%', '<', '>', '~', '^':
		return true
	}
	return false
}
