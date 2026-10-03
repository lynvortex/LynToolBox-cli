// Package codefmt 实现 HTML/CSS/JS 基础美化命令。
// 对应网页版：work/code-format-tool.html（代码格式化）
//
// 用法：
//
//	lyntoolbox codefmt -lang html -f page.html
//	lyntoolbox codefmt -lang css -i 4 -f style.css
//	lyntoolbox codefmt -lang auto -f app.js
package codefmt

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	Name  = "codefmt"
	Desc  = "HTML/CSS/JS 基础美化：标签与语句缩进、声明换行（按内容自动识别语言）"
	Usage = `用法: lyntoolbox codefmt [-lang html|css|js|auto] [-i 缩进数] [-f 文件] [文本]

参数:
  -lang     html|css|js|auto（默认 auto，按内容猜测）
  -i        缩进空格数（默认 2）
  -f        从文件读取
  文本      待处理内容；省略且无 -f 时从 stdin 读取

说明（基础美化，非完整编译器级处理）:
  CSS: 规范化空白、每条声明一行、花括号缩进；HTML: 标签缩进，void 元素
  与 pre/textarea/script/style 内容原样保留；JS: 语句级缩进，字符串/模板
  字面量保持原样，支持 switch-case 缩进；正则字面量未特殊处理。`
)

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

var rawTextElements = map[string]bool{
	"script": true, "style": true, "pre": true, "textarea": true,
}

var inlineElements = map[string]bool{
	"a": true, "abbr": true, "b": true, "bdi": true, "bdo": true, "cite": true,
	"code": true, "data": true, "dfn": true, "em": true, "i": true, "kbd": true,
	"mark": true, "q": true, "rp": true, "rt": true, "ruby": true, "s": true,
	"samp": true, "small": true, "span": true, "strong": true, "sub": true,
	"sup": true, "time": true, "u": true, "var": true,
}

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
	// CSS 信号: 花括号内存在 "属性: 值;" 声明
	isCSS := strings.Contains(src, "{") && strings.Contains(src, "}") &&
		strings.Contains(src, ";") && strings.Contains(src, ":")
	switch {
	case isJS && !isCSS:
		return "js", true
	case isCSS && !isJS:
		return "css", true
	case isCSS && isJS:
		// JS 里也常有 {}; 出现 function 等强信号时判 JS
		if strings.Contains(src, "function") || strings.Contains(src, "=>") {
			return "js", true
		}
		return "css", true
	}
	return "", false
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	lang := fs.String("lang", "auto", "语言 html|css|js|auto")
	indent := fs.Int("i", 2, "缩进空格数")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *indent < 0 {
		fmt.Fprintln(os.Stderr, "缩进空格数不能为负")
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
			fmt.Fprintln(os.Stderr, "无法识别语言，请用 -lang 指定 html|css|js")
			return 1
		}
		l = guess
	}
	ind := strings.Repeat(" ", *indent)

	var out string
	switch l {
	case "css":
		out = formatCSS(src, ind)
	case "html":
		out = formatHTML(src, ind)
	case "js":
		out = formatJS(src, ind)
	default:
		fmt.Fprintf(os.Stderr, "不支持的语言: %s（可选 html|css|js|auto）\n", *lang)
		return 2
	}
	fmt.Print(out)
	return 0
}

// ---------- CSS ----------

// normalizeDecl 规范化声明 "prop:value" 为 "prop: value"。
func normalizeDecl(s string) string {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 {
				return strings.TrimSpace(s[:i]) + ": " + strings.TrimSpace(s[i+1:])
			}
		}
	}
	return s
}

// formatCSS 规范化空白、声明分行、花括号缩进。
func formatCSS(src string, ind string) string {
	var b strings.Builder
	depth := 0
	var buf strings.Builder
	bufSpace := false
	parenDepth := 0

	writeBufSpace := func() {
		if buf.Len() > 0 && !bufSpace {
			buf.WriteByte(' ')
			bufSpace = true
		}
	}
	emit := func(s string) {
		b.WriteString(strings.Repeat(ind, depth) + s + "\n")
	}
	flushDecl := func(withSemi bool) {
		d := strings.TrimSpace(buf.String())
		buf.Reset()
		bufSpace = false
		if d != "" {
			if withSemi {
				emit(normalizeDecl(d) + ";")
			} else {
				emit(normalizeDecl(d))
			}
		}
	}

	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			j := strings.Index(src[i+2:], "*/")
			flushDecl(false)
			var text string
			if j < 0 {
				text = src[i:]
				i = len(src)
			} else {
				text = src[i : i+2+j+2]
				i += 2 + j + 2
			}
			emit(strings.Join(strings.Fields(text), " "))
		case c == '"' || c == '\'':
			j := skipQuoted(src, i)
			buf.WriteString(src[i:j])
			bufSpace = false
			i = j
		case c == '(':
			parenDepth++
			buf.WriteByte(c)
			bufSpace = false
			i++
		case c == ')':
			if parenDepth > 0 {
				parenDepth--
			}
			buf.WriteByte(c)
			bufSpace = false
			i++
		case c == '{':
			sel := strings.TrimSpace(buf.String())
			buf.Reset()
			bufSpace = false
			if depth == 0 && b.Len() > 0 {
				b.WriteString("\n")
			}
			emit(sel + " {")
			depth++
		case c == '}':
			flushDecl(false)
			if depth > 0 {
				depth--
			}
			emit("}")
		case c == ';' && parenDepth == 0:
			flushDecl(true)
			i++
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			writeBufSpace()
			i++
		default:
			buf.WriteByte(c)
			bufSpace = false
			i++
		}
	}
	flushDecl(false)
	return b.String()
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

// ---------- HTML ----------

type htmlNode struct {
	name        string
	attrs       string
	selfClosing bool
	text        string // 文本节点内容
	raw         bool   // 原样文本（script/style/pre/textarea 内部）
	children    []*htmlNode
}

// formatHTML 标签缩进的基础美化。
func formatHTML(src string, ind string) string {
	root := buildHTMLTree(src)
	var b strings.Builder
	renderHTML(&b, root, 0, ind)
	return b.String()
}

func renderHTML(b *strings.Builder, n *htmlNode, depth int, ind string) {
	indent := strings.Repeat(ind, depth)
	for _, c := range n.children {
		if c.text != "" && c.name == "" {
			// 文本节点
			if c.raw {
				for _, ln := range strings.Split(strings.Trim(c.text, "\r\n"), "\n") {
					b.WriteString(ln + "\n")
				}
				continue
			}
			t := strings.Join(strings.Fields(c.text), " ")
			if t != "" {
				b.WriteString(indent + t + "\n")
			}
			continue
		}
		open := "<" + c.name
		if attrs := strings.Join(strings.Fields(c.attrs), " "); attrs != "" {
			open += " " + attrs
		}
		if c.selfClosing || voidElements[c.name] {
			if c.selfClosing {
				open += "/>"
			} else {
				open += ">"
			}
			b.WriteString(indent + open + "\n")
			continue
		}
		open += ">"
		if rawTextElements[c.name] {
			// 原样内容元素
			b.WriteString(indent + open + "\n")
			renderHTML(b, c, depth+1, ind)
			b.WriteString(indent + "</" + c.name + ">\n")
			continue
		}
		if htmlCanInline(c) {
			b.WriteString(indent + open + htmlInlineText(c) + "</" + c.name + ">\n")
			continue
		}
		b.WriteString(indent + open + "\n")
		renderHTML(b, c, depth+1, ind)
		b.WriteString(indent + "</" + c.name + ">\n")
	}
}

// htmlCanInline 判断元素是否可以保持单行（仅文本，或仅由行内元素组成）。
func htmlCanInline(n *htmlNode) bool {
	hasText := false
	for _, c := range n.children {
		if c.text != "" && c.name == "" {
			if c.raw {
				return false
			}
			hasText = true
			continue
		}
		if !inlineElements[c.name] || !htmlCanInline(c) {
			return false
		}
	}
	return hasText || len(n.children) > 0
}

// htmlInlineText 输出行内内容。
func htmlInlineText(n *htmlNode) string {
	var b strings.Builder
	for _, c := range n.children {
		if c.text != "" && c.name == "" {
			b.WriteString(strings.Join(strings.Fields(c.text), " "))
			continue
		}
		open := "<" + c.name
		if attrs := strings.Join(strings.Fields(c.attrs), " "); attrs != "" {
			open += " " + attrs
		}
		open += ">"
		if c.selfClosing || voidElements[c.name] {
			if c.selfClosing {
				open = open[:len(open)-1] + "/>"
			}
			b.WriteString(open)
			continue
		}
		b.WriteString(open + htmlInlineText(c) + "</" + c.name + ">")
	}
	return b.String()
}

// buildHTMLTree 将 HTML 解析为树。
func buildHTMLTree(src string) *htmlNode {
	root := &htmlNode{name: "#root"}
	stack := []*htmlNode{root}
	i := 0

	appendChild := func(n *htmlNode) {
		top := stack[len(stack)-1]
		top.children = append(top.children, n)
	}
	appendText := func(s string, raw bool) {
		top := stack[len(stack)-1]
		top.children = append(top.children, &htmlNode{text: s, raw: raw})
	}

	for i < len(src) {
		if src[i] != '<' {
			j := strings.IndexByte(src[i:], '<')
			if j < 0 {
				appendText(src[i:], false)
				break
			}
			appendText(src[i:i+j], false)
			i += j
			continue
		}
		// 标签或特殊结构
		if strings.HasPrefix(src[i:], "<!--") {
			j := strings.Index(src[i+4:], "-->")
			if j < 0 {
				appendText(src[i:], false)
				break
			}
			end := i + 4 + j + 3
			appendText(src[i:end], false) // 注释按文本节点原样输出
			i = end
			continue
		}
		if strings.HasPrefix(src[i:], "<!") || strings.HasPrefix(src[i:], "<?") {
			j := strings.IndexByte(src[i:], '>')
			if j < 0 {
				appendText(src[i:], false)
				break
			}
			appendText(src[i:i+j+1], false)
			i += j + 1
			continue
		}
		if strings.HasPrefix(src[i:], "</") {
			j := strings.IndexByte(src[i:], '>')
			if j < 0 {
				appendText(src[i:], false)
				break
			}
			name := strings.ToLower(strings.TrimSpace(src[i+2 : i+j]))
			// 弹栈直到匹配
			for k := len(stack) - 1; k >= 1; k-- {
				if stack[k].name == name {
					stack = stack[:k]
					break
				}
			}
			i += j + 1
			continue
		}
		// 开始标签
		j := i + 1
		for j < len(src) && src[j] != '>' && src[j] != ' ' && src[j] != '\t' && src[j] != '\n' && src[j] != '/' {
			j++
		}
		name := strings.ToLower(src[i+1 : j])
		// 属性区
		k := j
		selfClose := false
		for k < len(src) && src[k] != '>' {
			if src[k] == '/' && k+1 < len(src) && src[k+1] == '>' {
				selfClose = true
				break
			}
			k++
		}
		if k >= len(src) {
			attrs := src[j:]
			appendChild(&htmlNode{name: name, attrs: attrs, selfClosing: selfClose})
			break
		}
		attrs := src[j:k]
		end := k
		if !selfClose {
			end = k + 1
		}
		node := &htmlNode{name: name, attrs: attrs, selfClosing: selfClose}
		appendChild(node)
		i = end
		if selfClose || voidElements[name] {
			continue
		}
		if rawTextElements[name] {
			// 原样捕获内容直到对应结束标签
			close := "</" + name
			rest := src[i:]
			low := strings.ToLower(rest)
			p := strings.Index(low, close)
			if p < 0 {
				node.children = append(node.children, &htmlNode{text: rest, raw: true})
				i = len(src)
			} else {
				node.children = append(node.children, &htmlNode{text: rest[:p], raw: true})
				gt := strings.IndexByte(rest[p:], '>')
				if gt < 0 {
					i = len(src)
				} else {
					i += p + gt + 1
				}
			}
			continue
		}
		stack = append(stack, node)
	}
	return root
}

// ---------- JS ----------

// formatJS 语句级缩进的基础美化。
func formatJS(src string, ind string) string {
	var b strings.Builder
	depth := 0
	caseBoost := false
	var line strings.Builder
	lineSpace := false
	parenDepth := 0

	flushLine := func() {
		s := strings.TrimSpace(line.String())
		line.Reset()
		lineSpace = false
		if s == "" {
			return
		}
		lv := depth
		if isCaseLabel(s) {
			lv = depth
			caseBoost = true
		} else if caseBoost {
			lv = depth + 1
		}
		b.WriteString(strings.Repeat(ind, lv) + s + "\n")
	}
	writeSpace := func() {
		if line.Len() > 0 && !lineSpace {
			line.WriteByte(' ')
			lineSpace = true
		}
	}

	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				j = len(src)
			} else {
				j += i
			}
			if line.Len() > 0 {
				line.WriteString(" ")
			}
			line.WriteString(strings.TrimRight(src[i:j], "\r"))
			flushLine()
			i = j
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			j := strings.Index(src[i+2:], "*/")
			var text string
			if j < 0 {
				text = src[i:]
				i = len(src)
			} else {
				text = src[i : i+2+j+2]
				i += 2 + j + 2
			}
			if strings.Contains(text, "\n") {
				flushLine()
				for _, ln := range strings.Split(strings.TrimRight(text, "\r\n"), "\n") {
					b.WriteString(strings.Repeat(ind, depth) + strings.TrimSpace(ln) + "\n")
				}
			} else {
				writeSpace()
				line.WriteString(text)
				lineSpace = false
			}
		case c == '\'' || c == '"':
			j := skipQuoted(src, i)
			line.WriteString(src[i:j])
			lineSpace = false
			i = j
		case c == '`':
			j := skipTemplate(src, i)
			line.WriteString(src[i:j])
			lineSpace = false
			i = j
		case c == '(':
			parenDepth++
			line.WriteByte(c)
			lineSpace = false
			i++
		case c == ')':
			if parenDepth > 0 {
				parenDepth--
			}
			line.WriteByte(c)
			lineSpace = false
			i++
		case c == '{':
			s := strings.TrimRight(line.String(), " \t")
			line.Reset()
			if s != "" {
				lv := depth
				if caseBoost {
					lv = depth + 1
				}
				b.WriteString(strings.Repeat(ind, lv) + s + " {\n")
			} else {
				flushLine()
				b.WriteString(strings.Repeat(ind, depth) + "{\n")
			}
			lineSpace = false
			depth++
			caseBoost = false
			i++
		case c == '}':
			flushLine()
			if depth > 0 {
				depth--
			}
			caseBoost = false
			// } else { / } catch { / } while (...);
			j := i + 1
			for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == '\r') {
				j++
			}
			word, wend := nextWord(src, j)
			if word == "else" || word == "catch" || word == "finally" || word == "while" {
				b.WriteString(strings.Repeat(ind, depth) + "} " + word)
				lineSpace = false
				i = wend
			} else {
				b.WriteString(strings.Repeat(ind, depth) + "}\n")
				i++
			}
		case c == ';' && parenDepth == 0:
			line.WriteString(";")
			flushLine()
			i++
		case c == '\n':
			flushLine() // 保留原换行（保守处理无分号语句）
			i++
		case c == ' ' || c == '\t' || c == '\r':
			writeSpace()
			i++
		default:
			line.WriteByte(c)
			lineSpace = false
			i++
		}
	}
	flushLine()
	return b.String()
}

func isCaseLabel(s string) bool {
	return s == "default:" || strings.HasPrefix(s, "default:") ||
		strings.HasPrefix(s, "case ") || strings.HasPrefix(s, "case:") ||
		s == "case"
}

func isIdentChar(c byte) bool {
	return c == '_' || c == '$' || c >= '0' && c <= '9' ||
		c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func nextWord(src string, i int) (string, int) {
	j := i
	for j < len(src) && (isIdentChar(src[j])) {
		j++
	}
	return src[i:j], j
}

// skipTemplate 跳过模板字面量（忽略 ${} 内部细节）。
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
