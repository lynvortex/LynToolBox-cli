//go:build ignore

// 暂存区：未集成/未完成实现的工具源码，不参与编译。

// Package mdconv 实现 Markdown 与 HTML 双向转换命令。
// 对应网页版：text/markdown-tool.html（Markdown转HTML）、text/html-to-md-tool.html（HTML转Markdown）
//
// 用法：
//
//	lyntoolbox mdconv -f doc.md
//	lyntoolbox mdconv -x -f doc.md
//	lyntoolbox mdconv -to md -f page.html
package mdconv

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
	xhtml "golang.org/x/net/html"
)

const (
	Name  = "mdconv"
	Desc  = "Markdown 与 HTML 双向转换（Markdown 渲染为 HTML，HTML 还原为 Markdown）"
	Usage = `用法: lyntoolbox mdconv [-to html|md] [-x] [-f 文件] [文本]

参数:
  -to html  Markdown → HTML（默认）
  -to md    HTML → Markdown
  -x        输出完整 HTML 文档（含 <html> 骨架），仅 -to html 时有效
  -f        从文件读取
  文本      待处理内容；省略且无 -f 时从 stdin 读取

说明:
  MD→HTML 使用 CommonMark 扩展（表格/围栏代码/删除线等）；
  HTML→MD 支持 h1-h6/p/strong/em/a/img/ul/ol/li/blockquote/pre>code/br/hr/
  table/code/del 等常用标签，未知标签取其纯文本。`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	to := fs.String("to", "html", "目标格式 html|md")
	fullDoc := fs.Bool("x", false, "输出完整 HTML 文档")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	*to = strings.ToLower(*to)
	switch *to {
	case "html", "md":
	default:
		fmt.Fprintf(os.Stderr, "不支持的 -to 目标: %s（可选 html|md）\n", *to)
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
	if len(strings.TrimSpace(string(data))) == 0 {
		fmt.Fprintln(os.Stderr, "输入为空")
		return 1
	}

	if *to == "md" {
		out, err := htmlToMarkdown(string(data))
		if err != nil {
			fmt.Fprintln(os.Stderr, "HTML 解析失败:", err)
			return 1
		}
		fmt.Print(out)
		return 0
	}

	extensions := parser.CommonExtensions
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(data)
	opts := html.RendererOptions{Flags: html.CommonFlags}
	if *fullDoc {
		opts.Flags |= html.CompletePage
	}
	renderer := html.NewRenderer(opts)
	os.Stdout.Write(markdown.Render(doc, renderer))
	return 0
}

// ---------- HTML → Markdown ----------

// htmlToMarkdown 把 HTML 文本转换为 Markdown。
func htmlToMarkdown(src string) (string, error) {
	nodes, err := xhtml.Parse(strings.NewReader(src))
	if err != nil {
		return "", err
	}
	// 定位 body
	var body *xhtml.Node
	var find func(n *xhtml.Node)
	find = func(n *xhtml.Node) {
		if body != nil {
			return
		}
		if n.Type == xhtml.ElementNode && n.Data == "body" {
			body = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(nodes)
	root := nodes
	if body != nil {
		root = body
	}

	skipTags := map[string]bool{"script": true, "style": true, "head": true,
		"meta": true, "title": true, "link": true, "noscript": true}
	md := renderBlock(root, skipTags)
	out := strings.Trim(md, "\n")
	// 压缩 3 个以上连续空行
	var b strings.Builder
	blanks := 0
	for _, ln := range strings.Split(out, "\n") {
		if strings.TrimSpace(ln) == "" {
			blanks++
			if blanks > 2 {
				continue
			}
		} else {
			blanks = 0
		}
		b.WriteString(ln + "\n")
	}
	return b.String(), nil
}

// renderBlock 渲染块级内容，块之间以空行分隔。
func renderBlock(n *xhtml.Node, skip map[string]bool) string {
	var parts []string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != xhtml.ElementNode {
			if c.Type == xhtml.TextNode {
				if t := strings.TrimSpace(c.Data); t != "" {
					parts = append(parts, t)
				}
			}
			continue
		}
		if skip[c.Data] {
			continue
		}
		switch c.Data {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			level := int(c.Data[1] - '0')
			parts = append(parts, strings.Repeat("#", level)+" "+inline(c, skip))
		case "p":
			parts = append(parts, inline(c, skip))
		case "br":
			parts = append(parts, "")
		case "hr":
			parts = append(parts, "---")
		case "blockquote":
			inner := strings.TrimRight(renderBlock(c, skip), "\n")
			var q strings.Builder
			for _, ln := range strings.Split(inner, "\n") {
				if strings.TrimSpace(ln) == "" {
					q.WriteString(">\n")
				} else {
					q.WriteString("> " + ln + "\n")
				}
			}
			parts = append(parts, strings.TrimRight(q.String(), "\n"))
		case "pre":
			parts = append(parts, renderCodeBlock(c, skip))
		case "ul", "ol":
			parts = append(parts, renderList(c, skip, 0))
		case "table":
			if md := renderTable(c, skip); md != "" {
				parts = append(parts, md)
			}
		default:
			// 未知块级标签：递归取内容
			inner := renderBlock(c, skip)
			if strings.TrimSpace(inner) != "" {
				parts = append(parts, strings.Trim(inner, "\n"))
			}
		}
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// renderCodeBlock 输出 ``` 围栏代码块。
func renderCodeBlock(pre *xhtml.Node, skip map[string]bool) string {
	var text strings.Builder
	var lang string
	var walk func(n *xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "code" {
			for _, a := range n.Attr {
				if a.Key == "class" {
					for _, cls := range strings.Fields(a.Val) {
						if strings.HasPrefix(cls, "language-") {
							lang = strings.TrimPrefix(cls, "language-")
						}
					}
				}
			}
		}
		if n.Type == xhtml.TextNode {
			text.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(pre)
	code := strings.Trim(text.String(), "\n")
	return "```" + lang + "\n" + code + "\n```"
}

// renderList 渲染 ul/ol，depth 为嵌套层级。
func renderList(list *xhtml.Node, skip map[string]bool, depth int) string {
	var b strings.Builder
	idx := 0
	for li := list.FirstChild; li != nil; li = li.NextSibling {
		if li.Type != xhtml.ElementNode || li.Data != "li" {
			continue
		}
		idx++
		indent := strings.Repeat("  ", depth)
		marker := "- "
		if list.Data == "ol" {
			marker = fmt.Sprintf("%d. ", idx)
		}
		// li 内容：行内部分 + 嵌套列表
		var inlinePart strings.Builder
		var nested []string
		for c := li.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == xhtml.ElementNode {
				switch c.Data {
				case "ul", "ol":
					nested = append(nested, renderList(c, skip, depth+1))
					continue
				case "p":
					inlinePart.WriteString(inline(c, skip))
					continue
				}
			}
			inlinePart.WriteString(inlineNode(c, skip))
		}
		b.WriteString(indent + marker + strings.TrimSpace(inlinePart.String()) + "\n")
		for _, n := range nested {
			b.WriteString(n)
		}
	}
	return b.String()
}

// renderTable 输出 Markdown 表格。
func renderTable(table *xhtml.Node, skip map[string]bool) string {
	var header []string
	var rows [][]string
	var collectRow func(tr *xhtml.Node, isHeader bool)
	collectRow = func(tr *xhtml.Node, isHeader bool) {
		var cells []string
		for c := tr.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == xhtml.ElementNode && (c.Data == "td" || c.Data == "th") {
				cells = append(cells, strings.TrimSpace(inline(c, skip)))
			}
		}
		if len(cells) == 0 {
			return
		}
		if isHeader {
			header = cells
		} else {
			rows = append(rows, cells)
		}
	}
	var walk func(n *xhtml.Node)
	walk = func(n *xhtml.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != xhtml.ElementNode {
				walk(c)
				continue
			}
			switch c.Data {
			case "thead":
				for tr := c.FirstChild; tr != nil; tr = tr.NextSibling {
					if tr.Type == xhtml.ElementNode && tr.Data == "tr" {
						collectRow(tr, true)
					}
				}
			case "tbody", "tfoot":
				for tr := c.FirstChild; tr != nil; tr = tr.NextSibling {
					if tr.Type == xhtml.ElementNode && tr.Data == "tr" {
						collectRow(tr, false)
					}
				}
			case "tr":
				collectRow(c, header == nil)
			default:
				walk(c)
			}
		}
	}
	walk(table)
	if len(header) == 0 {
		return ""
	}
	cols := len(header)
	// 单元格中的竖线转义
	escape := func(s string) string { return strings.ReplaceAll(s, "|", `\|`) }
	var b strings.Builder
	b.WriteString("|")
	for _, h := range header {
		b.WriteString(" " + escape(h) + " |")
	}
	b.WriteString("\n|")
	for i := 0; i < cols; i++ {
		b.WriteString(" --- |")
	}
	b.WriteString("\n")
	for _, r := range rows {
		b.WriteString("|")
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(r) {
				cell = r[i]
			}
			b.WriteString(" " + escape(cell) + " |")
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// inline 渲染元素内的行内内容为 Markdown。
func inline(n *xhtml.Node, skip map[string]bool) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(inlineNode(c, skip))
	}
	return b.String()
}

// inlineNode 渲染单个行内节点。
func inlineNode(n *xhtml.Node, skip map[string]bool) string {
	switch n.Type {
	case xhtml.TextNode:
		// 折叠空白（HTML 源码换行缩进不是内容）
		return strings.Join(strings.Fields(n.Data), " ")
	case xhtml.ElementNode:
		if skip[n.Data] {
			return ""
		}
		inner := inline(n, skip)
		switch n.Data {
		case "strong", "b":
			return "**" + inner + "**"
		case "em", "i":
			return "*" + inner + "*"
		case "del", "s", "strike":
			return "~~" + inner + "~~"
		case "code":
			return "`" + inner + "`"
		case "a":
			href := ""
			title := ""
			for _, a := range n.Attr {
				switch a.Key {
				case "href":
					href = a.Val
				case "title":
					title = a.Val
				}
			}
			if title != "" {
				return "[" + inner + "](" + href + " \"" + title + "\")"
			}
			return "[" + inner + "](" + href + ")"
		case "img":
			alt, src := "", ""
			for _, a := range n.Attr {
				switch a.Key {
				case "alt":
					alt = a.Val
				case "src":
					src = a.Val
				}
			}
			return "![" + alt + "](" + src + ")"
		case "br":
			return "\n"
		default:
			return inner
		}
	}
	return ""
}
