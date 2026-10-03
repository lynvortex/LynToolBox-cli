// Package xmlfmt 实现 XML 格式化/压缩/校验/转 JSON 命令。
// 对应网页版：text/xml-format-tool.html（XML格式化）
//
// 用法：
//
//	lyntoolbox xmlfmt -f doc.xml
//	lyntoolbox xmlfmt -c -f doc.xml
//	lyntoolbox xmlfmt -to json -f doc.xml
//	lyntoolbox xmlfmt -k -f doc.xml
package xmlfmt

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	Name  = "xmlfmt"
	Desc  = "XML 格式化、压缩、校验与转 JSON（属性 @ 前缀、文本 #text、同名兄弟合并数组）"
	Usage = `用法: lyntoolbox xmlfmt [-i 缩进数] [-c] [-k] [-to json] [-f 文件] [文本]

参数:
  -i        缩进空格数（默认 2）
  -c        压缩：去掉标签间空白，单行输出
  -k        只校验合法性
  -to json  转为 JSON：属性加 @ 前缀，文本放 #text，同名兄弟元素合并为数组
  -f        从文件读取
  文本      待处理内容；省略且无 -f 时从 stdin 读取

说明: 基于标准库 XML tokenizer 重缩进；解析错误时输出行列号。`
)

// lineCol 将字节偏移换算为 1 起始的行列号。
func lineCol(data []byte, offset int64) (int, int) {
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	line, col := 1, 1
	for i := 0; i < int(offset); i++ {
		if data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

func sameName(a, b xml.Name) bool { return a.Space == b.Space && a.Local == b.Local }

func renderName(n xml.Name) string {
	if n.Space != "" {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

func escapeText(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}

func escapeAttr(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func openTag(e *xml.StartElement) string {
	var b strings.Builder
	b.WriteString("<" + renderName(e.Name))
	for _, a := range e.Attr {
		b.WriteString(" " + renderName(a.Name) + `="` + escapeAttr(a.Value) + `"`)
	}
	b.WriteString(">")
	return b.String()
}

func selfCloseTag(e *xml.StartElement) string {
	return openTag(e)[:len(openTag(e))-1] + "/>"
}

func closeTag(n xml.Name) string { return "</" + renderName(n) + ">" }

// renderer 基于 tokenizer 的重缩进渲染器，可工作在格式化或压缩模式。
type renderer struct {
	b           strings.Builder
	compact     bool
	indent      string
	depth       int
	pending     *xml.StartElement
	pendingText *string
}

func (r *renderer) writeLine(s string) {
	if r.compact {
		r.b.WriteString(s)
		return
	}
	r.b.WriteString(strings.Repeat(r.indent, r.depth) + s + "\n")
}

// writeText 输出文本节点：格式化模式按行缩进，压缩模式折叠空白。
func (r *renderer) writeText(s string) {
	if r.compact {
		r.b.WriteString(escapeText(strings.Join(strings.Fields(s), " ")))
		return
	}
	for _, ln := range strings.Split(s, "\n") {
		if strings.TrimSpace(ln) != "" {
			r.writeLine(escapeText(strings.TrimSpace(ln)))
		}
	}
}

// flush 处理挂起的开始标签；next 非空且与之匹配时收尾为自闭合或行内文本。
func (r *renderer) flush(next *xml.EndElement) {
	if r.pending == nil {
		return
	}
	p := r.pending
	if next != nil && sameName(p.Name, next.Name) {
		if r.pendingText != nil {
			inline := openTag(p) + escapeText(strings.Join(strings.Fields(*r.pendingText), " ")) + closeTag(p.Name)
			r.writeLine(inline)
		} else {
			r.writeLine(selfCloseTag(p))
		}
		r.pending = nil
		r.pendingText = nil
		return
	}
	r.writeLine(openTag(p))
	r.depth++
	if r.pendingText != nil {
		r.writeText(*r.pendingText)
	}
	r.pending = nil
	r.pendingText = nil
}

func (r *renderer) render(dec *xml.Decoder, src []byte) error {
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			line, col := lineCol(src, dec.InputOffset())
			return fmt.Errorf("%s（第 %d 行第 %d 列附近）", err, line, col)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			r.flush(nil)
			e := t
			r.pending = &e
		case xml.EndElement:
			if r.pending != nil && sameName(r.pending.Name, t.Name) {
				r.flush(&t)
			} else {
				r.flush(nil)
				r.depth--
				r.writeLine(closeTag(t.Name))
			}
		case xml.CharData:
			if r.pending != nil {
				if strings.TrimSpace(string(t)) != "" {
					s := string(t)
					r.pendingText = &s
				}
			} else if strings.TrimSpace(string(t)) != "" {
				r.writeText(string(t))
			}
		case xml.Comment:
			r.flush(nil)
			r.writeLine("<!--" + string(t) + "-->")
		case xml.ProcInst:
			r.flush(nil)
			inst := strings.TrimSpace(string(t.Inst))
			if inst != "" {
				inst = " " + inst
			}
			r.writeLine("<?" + t.Target + inst + "?>")
		case xml.Directive:
			r.flush(nil)
			r.writeLine(string(t))
		}
	}
	r.flush(nil)
	return nil
}

// xnode 转换模式用的元素树节点。
type xnode struct {
	name     xml.Name
	attrs    []xml.Attr
	text     strings.Builder
	children []*xnode
}

func (n *xnode) toJSON() interface{} {
	m := map[string]interface{}{}
	for _, a := range n.attrs {
		m["@"+renderName(a.Name)] = a.Value
	}
	if t := strings.Join(strings.Fields(n.text.String()), " "); t != "" {
		m["#text"] = t
	}
	// 同名兄弟元素合并为数组
	order := []string{}
	grouped := map[string][]interface{}{}
	for _, c := range n.children {
		key := renderName(c.name)
		if _, seen := grouped[key]; !seen {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], c.toJSON())
	}
	for _, key := range order {
		vals := grouped[key]
		if len(vals) == 1 {
			m[key] = vals[0]
		} else {
			m[key] = vals
		}
	}
	return m
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	indent := fs.Int("i", 2, "缩进空格数")
	compact := fs.Bool("c", false, "压缩单行")
	checkOnly := fs.Bool("k", false, "只校验")
	to := fs.String("to", "", "目标格式 json")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	*to = strings.ToLower(*to)
	switch *to {
	case "", "json":
	default:
		fmt.Fprintf(os.Stderr, "不支持的 -to 目标: %s（可选 json）\n", *to)
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

	if *to == "json" {
		return convertToJSON(data)
	}

	dec := xml.NewDecoder(strings.NewReader(string(data)))
	r := &renderer{compact: *compact, indent: strings.Repeat(" ", *indent)}
	if err := r.render(dec, data); err != nil {
		fmt.Fprintln(os.Stderr, "XML 解析错误:", err)
		return 1
	}
	if *checkOnly {
		fmt.Println("XML 合法")
		return 0
	}
	out := r.b.String()
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	fmt.Print(out)
	return 0
}

// convertToJSON 把 XML 转为 JSON（元素→对象、属性 @、文本 #text、同名兄弟合并数组）。
func convertToJSON(data []byte) int {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	root := &xnode{name: xml.Name{Local: "#root"}}
	stack := []*xnode{root}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "XML 解析错误:", err)
			return 1
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &xnode{name: t.Name, attrs: t.Attr}
			top := stack[len(stack)-1]
			top.children = append(top.children, n)
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 1 {
				stack[len(stack)-1].text.Write(t)
			}
		}
	}
	if len(root.children) == 0 {
		fmt.Fprintln(os.Stderr, "XML 中没有根元素")
		return 1
	}
	if len(root.children) > 1 {
		fmt.Fprintln(os.Stderr, "XML 应有且仅有一个根元素")
		return 1
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(root.children[0].toJSON()); err != nil {
		fmt.Fprintln(os.Stderr, "转 JSON 失败:", err)
		return 1
	}
	return 0
}
