// Package docx2md 实现 Word(.docx) 内容提取命令。
// 对应网页版：document/word-view-tool.html（Word文档预览）
package docx2md

import (
	"archive/zip"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	Name  = "docx2md"
	Desc  = "提取 Word(.docx) 文本并转 Markdown（标题/加粗/列表/表格）"
	Usage = `用法: lyntoolbox docx2md 文档.docx [-plain] [-o 输出]

参数:
  -plain  纯文本模式（仅段落文本，不转 Markdown 语法）
  -o      输出文件（默认打印）`
)

// ---------- OOXML 结构 ----------

type document struct {
	Body body `xml:"body"`
}

type body struct {
	Nodes []node `xml:",any"`
}

// node 表示 body 下的段落或表格（用 XMLName 区分）。
type node struct {
	XMLName xml.Name
	Par     *paragraph `xml:"-"`
	Tbl     *table     `xml:"-"`
}

type paragraph struct {
	Props *pPr    `xml:"pPr"`
	Runs  []run   `xml:"r"`
	Hyper []hlink `xml:"hyperlink"`
}

type pPr struct {
	Style *styleRef `xml:"pStyle"`
	NumPr *numPr    `xml:"numPr"`
}

type styleRef struct {
	Val string `xml:"val,attr"`
}

type numPr struct{}

type run struct {
	Props *rPr  `xml:"rPr"`
	Ts    []wT  `xml:"t"`
	Tabs  []tab `xml:"tab"`
	Br    []br  `xml:"br"`
}

type rPr struct {
	Bold   *onOff `xml:"b"`
	Italic *onOff `xml:"i"`
}

type onOff struct {
	Val string `xml:"val,attr"`
}

type wT struct {
	Text  string `xml:",chardata"`
	Space string `xml:"space,attr"`
}

type tab struct{}
type br struct{}

type hlink struct {
	Runs []run  `xml:"r"`
	Rel  string `xml:"id,attr"`
}

type table struct {
	Rows []trow `xml:"tr"`
}

type trow struct {
	Cells []tcell `xml:"tc"`
}

type tcell struct {
	Paras []paragraph `xml:"p"`
}

func (p *paragraph) isHeading() (int, bool) {
	if p.Props == nil || p.Props.Style == nil {
		return 0, false
	}
	s := strings.ToLower(p.Props.Style.Val)
	for i := 1; i <= 6; i++ {
		if s == fmt.Sprintf("heading%d", i) || s == fmt.Sprintf("%d", i) {
			return i, true
		}
	}
	return 0, false
}

func (p *paragraph) isList() bool {
	return p.Props != nil && p.Props.NumPr != nil
}

func (r *run) isBold() bool {
	return r.Props != nil && r.Props.Bold != nil && r.Props.Bold.Val != "0" && r.Props.Bold.Val != "false"
}
func (r *run) isItalic() bool {
	return r.Props != nil && r.Props.Italic != nil && r.Props.Italic.Val != "0" && r.Props.Italic.Val != "false"
}

func (r *run) text() string {
	var b strings.Builder
	for _, t := range r.Ts {
		b.WriteString(t.Text)
	}
	for range r.Tabs {
		b.WriteString("\t")
	}
	for range r.Br {
		b.WriteString("\n")
	}
	return b.String()
}

// extractText 递归提取段落文本（不加工）。
func paraText(p *paragraph) string {
	var b strings.Builder
	for _, r := range p.Runs {
		b.WriteString(r.text())
	}
	for _, h := range p.Hyper {
		for _, r := range h.Runs {
			b.WriteString(r.text())
		}
	}
	return b.String()
}

// decode 解析 document.xml。
func decode(data []byte) (*document, error) {
	// 直接用 token 流解析，避免严格结构体映射的坑
	var doc document
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	dec.Strict = false
	depth := 0
	_ = depth
	var curP *paragraph
	var curT *table
	var curRow *trow
	var curCell *tcell
	var runStack []run
	var cellParas []paragraph
	var textBuf strings.Builder
	inCell := false
	_ = inCell
	cellCount := 0

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.CharData:
			// <w:t> 内的字符数据（其他位置没有裸文本）
			textBuf.Write(t)
		case xml.StartElement:
			name := t.Name.Local
			switch name {
			case "p":
				if inCell {
					// 单元格内段落：收集到 cellParas
					cellParas = append(cellParas, paragraph{})
					curP = &cellParas[len(cellParas)-1]
				} else {
					curP = &paragraph{}
				}
				depth++
			case "tbl":
				curT = &table{}
			case "tr":
				curRow = &trow{}
			case "tc":
				curCell = &tcell{}
				cellParas = nil
				inCell = true
				cellCount++
			case "pStyle":
				if curP != nil {
					for _, a := range t.Attr {
						if a.Name.Local == "val" {
							curP.Props = &pPr{Style: &styleRef{Val: a.Value}}
						}
					}
				}
			case "numPr":
				if curP != nil {
					if curP.Props == nil {
						curP.Props = &pPr{}
					}
					curP.Props.NumPr = &numPr{}
				}
			case "r":
				runStack = append(runStack, run{})
			case "b":
				if len(runStack) > 0 {
					// 只设置 Bold 字段，避免覆盖同一 run 里已有的 Italic 等属性
					if runStack[len(runStack)-1].Props == nil {
						runStack[len(runStack)-1].Props = &rPr{}
					}
					runStack[len(runStack)-1].Props.Bold = &onOff{Val: attrVal(t, "val")}
				}
			case "i":
				if len(runStack) > 0 {
					if runStack[len(runStack)-1].Props == nil {
						runStack[len(runStack)-1].Props = &rPr{}
					}
					runStack[len(runStack)-1].Props.Italic = &onOff{Val: attrVal(t, "val")}
				}
			case "t":
				// 文本已在 CharData 分支累积，此处不处理
			case "tab":
				textBuf.WriteString("\t")
			case "br":
				textBuf.WriteString("\n")
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "t":
				if len(runStack) > 0 {
					runStack[len(runStack)-1].Ts = append(runStack[len(runStack)-1].Ts, wT{Text: textBuf.String()})
				}
				textBuf.Reset()
			case "r":
				if curP != nil && len(runStack) > 0 {
					curP.Runs = append(curP.Runs, runStack[len(runStack)-1])
					runStack = runStack[:len(runStack)-1]
				}
			case "p":
				if inCell && curP != nil {
					// 已追加在 cellParas
					curP = nil
				} else if curP != nil {
					doc.Body.Nodes = append(doc.Body.Nodes, node{XMLName: xml.Name{Local: "p"}, Par: curP})
					curP = nil
				}
			case "tc":
				if curCell != nil {
					curCell.Paras = cellParas
					curRow.Cells = append(curRow.Cells, *curCell)
					curCell = nil
					inCell = false
				}
			case "tr":
				if curT != nil && curRow != nil {
					curT.Rows = append(curT.Rows, *curRow)
					curRow = nil
				}
			case "tbl":
				if curT != nil {
					doc.Body.Nodes = append(doc.Body.Nodes, node{XMLName: xml.Name{Local: "tbl"}, Tbl: curT})
					curT = nil
				}
			}
		}
	}
	_ = cellCount
	return &doc, nil
}

func attrVal(t xml.StartElement, local string) string {
	for _, a := range t.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	plain := fs.Bool("plain", false, "纯文本模式")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox docx2md 文档.docx")
		return 2
	}

	zr, err := zip.OpenReader(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开 docx 失败（应为 .docx 压缩包）:", err)
		return 1
	}
	defer zr.Close()
	var xmlData []byte
	for _, zf := range zr.File {
		if zf.Name == "word/document.xml" {
			rc, err := zf.Open()
			if err != nil {
				fmt.Fprintln(os.Stderr, "读取 document.xml 失败:", err)
				return 1
			}
			xmlData, err = io.ReadAll(rc)
			rc.Close()
			break
		}
	}
	if xmlData == nil {
		fmt.Fprintln(os.Stderr, "不是有效的 docx（缺少 word/document.xml）")
		return 1
	}

	doc, err := decode(xmlData)
	if err != nil {
		fmt.Fprintln(os.Stderr, "解析 document.xml 失败:", err)
		return 1
	}

	var b strings.Builder
	for _, n := range doc.Body.Nodes {
		switch {
		case n.Par != nil:
			p := n.Par
			text := strings.TrimSpace(paraText(p))
			if text == "" {
				continue
			}
			if *plain {
				b.WriteString(text + "\n\n")
				continue
			}
			if lv, ok := p.isHeading(); ok {
				b.WriteString(strings.Repeat("#", lv) + " " + text + "\n\n")
				continue
			}
			prefix := ""
			if p.isList() {
				prefix = "- "
			}
			b.WriteString(prefix + renderRunsMD(p) + "\n\n")
		case n.Tbl != nil:
			if *plain {
				for _, row := range n.Tbl.Rows {
					cells := make([]string, 0, len(row.Cells))
					for _, c := range row.Cells {
						t := ""
						for i := range c.Paras {
							t += paraText(&c.Paras[i])
						}
						cells = append(cells, strings.TrimSpace(t))
					}
					b.WriteString(strings.Join(cells, "\t") + "\n")
				}
				b.WriteString("\n")
				continue
			}
			rows := n.Tbl.Rows
			for ri, row := range rows {
				cells := make([]string, 0, len(row.Cells))
				for _, c := range row.Cells {
					t := ""
					for i := range c.Paras {
						t += paraText(&c.Paras[i])
					}
					cells = append(cells, strings.ReplaceAll(strings.TrimSpace(t), "|", "\\|"))
				}
				b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
				if ri == 0 {
					b.WriteString("|" + strings.Repeat(" --- |", len(cells)) + "\n")
				}
			}
			b.WriteString("\n")
		}
	}

	result := strings.TrimRight(b.String(), "\n") + "\n"
	if *out != "" {
		if err := os.WriteFile(*out, []byte(result), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "写入失败:", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "已写入 %s\n", *out)
		return 0
	}
	fmt.Print(result)
	return 0
}

// renderRunsMD 输出段落文本，加粗/斜体按 run 包裹标记。
func renderRunsMD(p *paragraph) string {
	var b strings.Builder
	write := func(r run) {
		t := r.text()
		if t == "" {
			return
		}
		if r.isBold() {
			t = "**" + strings.TrimSpace(t) + "**"
		}
		if r.isItalic() {
			t = "*" + strings.Trim(t, "*") + "*"
		}
		b.WriteString(t)
	}
	for _, r := range p.Runs {
		write(r)
	}
	for _, h := range p.Hyper {
		for _, r := range h.Runs {
			write(r)
		}
	}
	// 清理相邻标记产生的 ****
	out := b.String()
	for strings.Contains(out, "****") {
		out = strings.ReplaceAll(out, "****", "")
	}
	return strings.TrimSpace(out)
}
