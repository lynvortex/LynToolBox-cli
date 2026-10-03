// Package vsrc 实现网页源码查看命令。
// 对应网页版：network/source-view-tool.html（网站源码查看器）
package vsrc

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	Name  = "vsrc"
	Desc  = "获取网页源码（原始 / meta 摘要 / 重缩进）"
	Usage = `用法: lyntoolbox vsrc URL [-raw|-meta|-fmt] [-o 文件]

参数:
  -raw   原始源码（默认，超 2MB 截断）
  -meta  摘要模式：标题/描述/关键词/OG 标签/服务器信息
  -fmt   标签重缩进美化（script/style 内容原样）
  -o     保存到文件`
)

const maxBytes = 2 * 1024 * 1024

func fetch(target string) (string, *http.Response, error) {
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "https://" + target
	}
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", "LynToolBox/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return "", resp, err
	}
	body := string(b)
	if len(b) > maxBytes {
		body += "\n<!-- 内容超过 2MB，已截断 -->"
	}
	return body, resp, nil
}

func metaValue(src, attr, name string) string {
	// 抓取 <meta name/property="x" content="y">（简单正则级处理，属性顺序无关）
	low := strings.ToLower(src)
	needle := attr + `="` + strings.ToLower(name) + `"`
	idx := 0
	for {
		i := strings.Index(low[idx:], "<meta")
		if i < 0 {
			return ""
		}
		i += idx
		end := strings.Index(low[i:], ">")
		if end < 0 {
			return ""
		}
		tag := src[i : i+end]
		tagLow := low[i : i+end]
		if strings.Contains(tagLow, needle) {
			if j := strings.Index(tagLow, `content="`); j >= 0 {
				rest := tag[j+len(`content="`):]
				if k := strings.Index(rest, `"`); k >= 0 {
					return rest[:k]
				}
			}
		}
		idx = i + end
	}
}

func titleValue(src string) string {
	low := strings.ToLower(src)
	i := strings.Index(low, "<title")
	if i < 0 {
		return ""
	}
	start := strings.Index(low[i:], ">")
	if start < 0 {
		return ""
	}
	rest := src[i+start+1:]
	if end := strings.Index(strings.ToLower(rest), "</title>"); end >= 0 {
		return strings.TrimSpace(rest[:end])
	}
	return ""
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	meta := fs.Bool("meta", false, "摘要模式")
	format := fs.Bool("fmt", false, "重缩进")
	out := fs.String("o", "", "保存文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox vsrc URL")
		return 2
	}

	body, resp, err := fetch(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "获取失败:", err)
		return 1
	}

	var result string
	switch {
	case *meta:
		fmt.Printf("URL:          %s\n", resp.Request.URL)
		fmt.Printf("状态码:       %s\n", resp.Status)
		fmt.Printf("Content-Type: %s\n", resp.Header.Get("Content-Type"))
		fmt.Printf("Server:       %s\n", orDash(resp.Header.Get("Server")))
		fmt.Printf("页面标题:     %s\n", orDash(titleValue(body)))
		fmt.Printf("description:  %s\n", orDash(metaValue(body, "name", "description")))
		fmt.Printf("keywords:     %s\n", orDash(metaValue(body, "name", "keywords")))
		fmt.Printf("og:title:     %s\n", orDash(metaValue(body, "property", "og:title")))
		fmt.Printf("og:image:     %s\n", orDash(metaValue(body, "property", "og:image")))
		return 0
	case *format:
		result = reindent(body)
	default:
		result = body
	}

	if *out != "" {
		if err := os.WriteFile(*out, []byte(result), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "写入失败:", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "已写入 %s（%d 字节）\n", *out, len(result))
		return 0
	}
	fmt.Print(result)
	if !strings.HasSuffix(result, "\n") {
		fmt.Println()
	}
	return 0
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// reindent 按标签重缩进；script/style/pre 内容原样保留。
func reindent(src string) string {
	var b strings.Builder
	depth := 0
	verbatim := ""
	low := strings.ToLower(src)
	i := 0
	newline := func() {
		b.WriteString("\n")
		b.WriteString(strings.Repeat("  ", max(depth, 0)))
	}
	for i < len(src) {
		if verbatim != "" {
			end := strings.Index(low[i:], "</"+verbatim)
			if end < 0 {
				b.WriteString(src[i:])
				break
			}
			b.WriteString(src[i : i+end])
			i += end
			verbatim = ""
			continue
		}
		if src[i] != '<' {
			j := strings.IndexByte(src[i:], '<')
			if j < 0 {
				text := strings.TrimSpace(src[i:])
				if text != "" {
					newline()
					b.WriteString(text)
				}
				break
			}
			text := strings.TrimSpace(src[i : i+j])
			if text != "" {
				newline()
				b.WriteString(text)
			}
			i += j
			continue
		}
		// 标签
		end := strings.IndexByte(src[i:], '>')
		if end < 0 {
			b.WriteString(src[i:])
			break
		}
		tag := src[i : i+end+1]
		tagLow := low[i : i+end+1]
		name := tagName(tagLow)
		closing := strings.HasPrefix(tagLow, "</")
		selfClose := strings.HasSuffix(strings.TrimSpace(tag), "/>") ||
			name == "br" || name == "hr" || name == "img" || name == "input" ||
			name == "meta" || name == "link" || name == "!doctype"
		if closing {
			depth--
			newline()
			b.WriteString(strings.TrimSpace(tag))
		} else {
			newline()
			b.WriteString(strings.TrimSpace(tag))
			if !selfClose {
				depth++
			}
			if name == "script" || name == "style" || name == "pre" || name == "textarea" {
				verbatim = name
			}
		}
		i += end + 1
	}
	return strings.TrimSpace(b.String()) + "\n"
}

func tagName(tagLower string) string {
	s := strings.TrimPrefix(tagLower, "</")
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimLeft(s, " \t\n")
	if i := strings.IndexAny(s, " \t\n>/"); i >= 0 {
		return s[:i]
	}
	return s
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
