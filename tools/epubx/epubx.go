// Package epubx 实现 EPUB 文件处理命令。
// 对应网页版：document/epub-reader-tool.html（EPUB阅读器）
package epubx

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/net/html"
)

const (
	Name  = "epubx"
	Desc  = "EPUB 处理：解包/列表/按阅读顺序提取文本/元数据"
	Usage = `用法:
  lyntoolbox epubx unpack 书.epub [-d 目录]
  lyntoolbox epubx list 书.epub
  lyntoolbox epubx text 书.epub [-o 输出]
  lyntoolbox epubx meta 书.epub`
)

type epubDoc struct {
	zr       *zip.ReadCloser
	opfPath  string
	opfData  []byte
	manifest map[string]string // id -> href
	spine    []string          // idref 顺序
	title    string
	creator  string
	files    []*zip.File
}

// safeJoin 校验 zip 条目名后拼接到 dest 下。
// 条目名来自不可信的 EPUB 文件，必须拒绝绝对路径、盘符、.. 穿越段，
// 以及 Windows 会当作分隔符的反斜杠与 NTFS 冒号流。
func safeJoin(dest, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("空条目名")
	}
	if filepath.VolumeName(name) != "" {
		return "", fmt.Errorf("非法条目路径: %s", name)
	}
	s := filepath.ToSlash(name)
	if strings.HasPrefix(s, "/") {
		return "", fmt.Errorf("非法条目路径: %s", name)
	}
	for _, seg := range strings.Split(s, "/") {
		if seg == ".." || strings.Contains(seg, ":") {
			return "", fmt.Errorf("非法条目路径: %s", name)
		}
	}
	if path.Clean(s) == "." {
		return "", fmt.Errorf("非法条目路径: %s", name)
	}
	return filepath.Join(dest, filepath.FromSlash(path.Clean(s))), nil
}

func open(p string) (*epubDoc, error) {
	zr, err := zip.OpenReader(p)
	if err != nil {
		return nil, err
	}
	d := &epubDoc{zr: zr, manifest: map[string]string{}}
	// 解析中途出错时兜底关闭；成功后由调用方 d.zr.Close() 负责
	success := false
	defer func() {
		if !success {
			zr.Close()
		}
	}()
	for _, f := range zr.File {
		d.files = append(d.files, f)
	}
	// container.xml 找 OPF
	for _, f := range d.files {
		if f.Name == "META-INF/container.xml" {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			data, _ := io.ReadAll(rc)
			rc.Close()
			// 简单属性抓取
			s := string(data)
			rootfile := ""
			if i := strings.Index(s, "full-path=\""); i >= 0 {
				rest := s[i+len("full-path=\""):]
				if j := strings.Index(rest, "\""); j >= 0 {
					rootfile = rest[:j]
				}
			}
			d.opfPath = rootfile
			break
		}
	}
	if d.opfPath == "" {
		// 兜底：找第一个 .opf
		for _, f := range d.files {
			if strings.HasSuffix(strings.ToLower(f.Name), ".opf") {
				d.opfPath = f.Name
				break
			}
		}
	}
	if d.opfPath == "" {
		return nil, fmt.Errorf("未找到 OPF 文件，不是有效的 EPUB")
	}
	opfFile := d.find(d.opfPath)
	if opfFile == nil {
		return nil, fmt.Errorf("OPF 文件缺失: %s", d.opfPath)
	}
	rc, err := opfFile.Open()
	if err != nil {
		return nil, err
	}
	d.opfData, err = io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return nil, err
	}
	// 解析 OPF（宽松：字符串扫描 manifest 与 spine）
	s := string(d.opfData)
	// title / creator
	if i := strings.Index(s, "<dc:title"); i >= 0 {
		if j := strings.Index(s[i:], ">"); j >= 0 {
			rest := s[i+j+1:]
			if k := strings.Index(rest, "</dc:title"); k >= 0 {
				d.title = strings.TrimSpace(rest[:k])
			}
		}
	}
	if i := strings.Index(s, "<dc:creator"); i >= 0 {
		if j := strings.Index(s[i:], ">"); j >= 0 {
			rest := s[i+j+1:]
			if k := strings.Index(rest, "</dc:creator"); k >= 0 {
				d.creator = strings.TrimSpace(rest[:k])
			}
		}
	}
	// manifest item
	for _, seg := range strings.Split(s, "<item ") {
		id := attrOf(seg, "id=")
		href := attrOf(seg, "href=")
		if id != "" && href != "" {
			d.manifest[id] = href
		}
	}
	// spine itemref
	for _, seg := range strings.Split(s, "<itemref ") {
		if idref := attrOf(seg, "idref="); idref != "" {
			d.spine = append(d.spine, idref)
		}
	}
	success = true
	return d, nil
}

func (d *epubDoc) find(name string) *zip.File {
	name = strings.TrimPrefix(name, "./")
	for _, f := range d.files {
		if strings.TrimPrefix(f.Name, "./") == name {
			return f
		}
	}
	return nil
}

// resolve 相对 OPF 目录解析 href。
func (d *epubDoc) resolve(href string) *zip.File {
	full := path.Join(path.Dir(d.opfPath), href)
	return d.find(full)
}

func attrOf(seg, attr string) string {
	i := strings.Index(seg, attr)
	if i < 0 {
		return ""
	}
	rest := seg[i+len(attr):]
	if len(rest) == 0 || (rest[0] != '"' && rest[0] != '\'') {
		return ""
	}
	q := rest[0]
	rest = rest[1:]
	if j := strings.IndexByte(rest, q); j >= 0 {
		return rest[:j]
	}
	return ""
}

// extractText 提取 XHTML 纯文本，块级元素间换行。
func extractText(xhtml string) string {
	var b strings.Builder
	blk := map[string]bool{
		"p": true, "div": true, "h1": true, "h2": true, "h3": true,
		"h4": true, "h5": true, "h6": true, "li": true, "br": true,
		"tr": true, "blockquote": true, "section": true,
	}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			t := strings.TrimSpace(n.Data)
			if t != "" {
				b.WriteString(t + " ")
			}
		}
		if n.Type == html.ElementNode {
			if blk[n.Data] && b.Len() > 0 {
				s := strings.TrimRight(b.String(), " ")
				b.Reset()
				b.WriteString(s + "\n")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && blk[n.Data] {
			b.WriteString("\n")
		}
	}
	doc, err := html.Parse(strings.NewReader(xhtml))
	if err != nil {
		return ""
	}
	walk(doc)
	// 压缩多余空行
	lines := strings.Split(b.String(), "\n")
	var out []string
	blank := 0
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, Usage+"\n")
		return 2
	}
	mode := args[0]
	switch mode {
	case "unpack", "list", "text", "meta":
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s（unpack|list|text|meta）\n", mode)
		return 2
	}
	fs := flag.NewFlagSet(mode, flag.ContinueOnError)
	dest := fs.String("d", ".", "解包目标目录")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "用法: lyntoolbox epubx %s 书.epub\n", mode)
		return 2
	}
	d, err := open(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开 EPUB 失败:", err)
		return 1
	}
	defer d.zr.Close()

	switch mode {
	case "meta":
		fmt.Printf("书名:   %s\n", orDash(d.title))
		fmt.Printf("作者:   %s\n", orDash(d.creator))
		fmt.Printf("OPF:    %s\n", d.opfPath)
		fmt.Printf("章节:   %d（spine 顺序）\n", len(d.spine))
		fmt.Printf("文件数: %d\n", len(d.files))
		return 0
	case "list":
		for _, f := range d.files {
			fmt.Printf("%10d  %s\n", f.UncompressedSize64, f.Name)
		}
		fmt.Printf("\n共 %d 个文件\n", len(d.files))
		return 0
	case "unpack":
		count := 0
		for _, f := range d.files {
			target, err := safeJoin(*dest, f.Name)
			if err != nil {
				fmt.Fprintln(os.Stderr, "跳过:", err)
				return 1
			}
			if f.FileInfo().IsDir() {
				if err := os.MkdirAll(target, 0755); err != nil {
					fmt.Fprintln(os.Stderr, "创建目录失败:", err)
					return 1
				}
				continue
			}
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				fmt.Fprintln(os.Stderr, "创建目录失败:", err)
				return 1
			}
			rc, err := f.Open()
			if err != nil {
				fmt.Fprintln(os.Stderr, "读取失败:", err)
				return 1
			}
			dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				rc.Close()
				fmt.Fprintln(os.Stderr, "写出失败:", err)
				return 1
			}
			_, err = io.Copy(dst, rc)
			rc.Close()
			dst.Close()
			if err != nil {
				fmt.Fprintln(os.Stderr, "写出失败:", err)
				return 1
			}
			count++
		}
		fmt.Printf("已解包 %d 个文件到 %s\n", count, *dest)
		return 0
	case "text":
		var b strings.Builder
		if d.title != "" {
			b.WriteString("# " + d.title + "\n\n")
		}
		if d.creator != "" {
			b.WriteString(d.creator + "\n\n")
		}
		count := 0
		for _, idref := range d.spine {
			href := d.manifest[idref]
			if href == "" {
				continue
			}
			f := d.resolve(href)
			if f == nil {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				continue
			}
			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				continue
			}
			txt := strings.TrimSpace(extractText(string(data)))
			if txt == "" {
				continue
			}
			count++
			b.WriteString("\n\n<!-- 第 " + fmt.Sprint(count) + " 节: " + href + " -->\n\n" + txt + "\n")
		}
		result := strings.TrimRight(b.String(), "\n") + "\n"
		if *out != "" {
			if err := os.WriteFile(*out, []byte(result), 0644); err != nil {
				fmt.Fprintln(os.Stderr, "写入失败:", err)
				return 1
			}
			fmt.Fprintf(os.Stderr, "已写入 %s（%d 节）\n", *out, count)
			return 0
		}
		fmt.Print(result)
		fmt.Fprintf(os.Stderr, "\n（共 %d 节）\n", count)
		return 0
	}
	return 0
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
