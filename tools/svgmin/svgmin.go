// Package svgmin 实现 SVG 压缩优化命令。
// 对应网页版：work/svg-minify-tool.html（SVG 压缩优化）
//
// 用法：
//
//	lyntoolbox svgmin [-prec 3] [-strip-title] [-o 输出] [-f 文件] [文本]
//	cat icon.svg | lyntoolbox svgmin -o icon.min.svg
package svgmin

import (
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	Name  = "svgmin"
	Desc  = "SVG 压缩优化：去注释/metadata、折叠空白、数值截断、颜色缩短"
	Usage = `用法: lyntoolbox svgmin [-prec 精度] [-strip-title] [-f 文件] [-o 输出] [文本]

参数:
  -prec         坐标/数值小数精度截断位数（默认 3）
  -strip-title  同时移除 <title>（默认保留以利无障碍）
  -f            输入文件；省略且无参数时从 stdin 读取
  -o            输出文件；省略时写到 stdout

优化策略:
  删除注释与 <metadata>/<desc>；折叠标签间与属性间空白；
  数值按 -prec 截断；十六进制颜色缩短（#AABBCC→#ABC）；删除空组 <g>。
  <text>/<tspan>/<style>/<script> 内容原样保留（避免破坏文本与 CSS）；
  处理结果经 XML 解析器校验，非法则报错退出。压缩统计输出到 stderr。`
)

var (
	reComments = regexp.MustCompile(`(?s)<!--.*?-->`)
	reMetadata = regexp.MustCompile(`(?is)<metadata\b[^>]*>.*?</metadata>|<metadata\b[^>]*/>`)
	reDesc     = regexp.MustCompile(`(?is)<desc\b[^>]*>.*?</desc>|<desc\b[^>]*/>`)
	reTitle    = regexp.MustCompile(`(?is)<title\b[^>]*>.*?</title>|<title\b[^>]*/>`)
	reTextEl   = regexp.MustCompile(`(?is)<text\b[^>]*>.*?</text>`)
	reTspanEl  = regexp.MustCompile(`(?is)<tspan\b[^>]*>.*?</tspan>`)
	reStyleEl  = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	reScriptEl = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	reCDATA    = regexp.MustCompile(`(?s)<!\[CDATA\[.*?\]\]>`)
	reTagGap   = regexp.MustCompile(`>\s+<`)
	reWS       = regexp.MustCompile(`[\t\n\r\f ]+`)
	reTag      = regexp.MustCompile(`<(?:[^>"']|"[^"]*"|'[^']*')*>`)
	reAttr     = regexp.MustCompile(`(\s[\w:.\-]+\s*=\s*)("[^"]*"|'[^']*')`)
	reEmptyG   = regexp.MustCompile(`(?is)<g\b[^>]*/>|<g\b[^>]*>\s*</g>`)
	reNumGuard = regexp.MustCompile(`(^|[^A-Za-z0-9_.])([+-]?(?:\d+\.\d+|\.\d+|\d+)(?:[eE][+-]?\d+)?)`)
	reNumPath  = regexp.MustCompile(`[+-]?(?:\d+\.\d+|\.\d+|\d+)(?:[eE][+-]?\d+)?`)
	reCSSHex   = regexp.MustCompile(`#([0-9a-fA-F]{6})([0-9a-fA-F]{2})?\b`)
)

// 不做数值处理的属性（值含语义字符串）
var skipNumAttrs = map[string]bool{
	"id": true, "class": true, "version": true, "href": true, "xlink:href": true,
	"xmlns": true, "in": true, "in2": true, "result": true, "standalone": true,
}

// 颜色类属性，可做十六进制颜色缩短
var colorAttrs = map[string]bool{
	"fill": true, "stroke": true, "color": true, "stop-color": true,
	"flood-color": true, "lighting-color": true, "solid-color": true,
}

type minifier struct {
	prec     int
	stripSub bool
	blocks   []string
}

// protect 抽出需要原样保留的内容块（text 先于 tspan，保证整体保护）。
func (m *minifier) protect(s string) string {
	for _, re := range []*regexp.Regexp{reTextEl, reTspanEl, reStyleEl, reScriptEl, reCDATA} {
		s = re.ReplaceAllStringFunc(s, func(match string) string {
			m.blocks = append(m.blocks, match)
			return fmt.Sprintf("\x00%d\x00", len(m.blocks)-1)
		})
	}
	return s
}

func (m *minifier) restore(s string) string {
	for i := range m.blocks {
		s = strings.ReplaceAll(s, fmt.Sprintf("\x00%d\x00", i), m.blocks[i])
	}
	return s
}

// roundNum 按精度截断数值。
func (m *minifier) roundNum(tok string) string {
	v, err := strconv.ParseFloat(tok, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return tok
	}
	p := float64(m.prec)
	rounded := math.Round(v*math.Pow(10, p)) / math.Pow(10, p)
	return strconv.FormatFloat(rounded, 'f', -1, 64)
}

// shrinkHex 缩短 6/8 位十六进制颜色（成对字符相同才可缩短）。
func shrinkHex(hex6, hex8 string) string {
	if hex8 != "" {
		if hex8[0] == hex8[1] && hex8[2] == hex8[3] && hex8[4] == hex8[5] && hex8[6] == hex8[7] {
			return "#" + string([]byte{hex8[0], hex8[2], hex8[4], hex8[6]})
		}
		return "#" + hex8
	}
	if hex6[0] == hex6[1] && hex6[2] == hex6[3] && hex6[4] == hex6[5] {
		return "#" + string([]byte{hex6[0], hex6[2], hex6[4]})
	}
	return "#" + hex6
}

// minifyAttrValue 对单个属性值做颜色缩短与数值截断。
func (m *minifier) minifyAttrValue(name, val string) string {
	quote := val[len(val)-1]
	inner := val[1 : len(val)-1]
	name = strings.ToLower(name)

	if name == "style" {
		inner = reCSSHex.ReplaceAllStringFunc(inner, func(h string) string {
			sub := reCSSHex.FindStringSubmatch(h)
			return shrinkHex(sub[1], sub[2])
		})
	} else if colorAttrs[name] {
		if strings.HasPrefix(inner, "#") {
			if n := len(inner); n == 7 {
				inner = shrinkHex(inner[1:], "")
			} else if n == 9 {
				inner = shrinkHex("", inner[1:])
			}
		}
	}

	if !skipNumAttrs[name] && !strings.HasPrefix(name, "xmlns") {
		if name == "d" || name == "points" || name == "transform" ||
			name == "gradientTransform" || name == "patternTransform" {
			// 路径/变换语法：数值可紧邻命令字母，用无边界版本
			inner = reNumPath.ReplaceAllStringFunc(inner, func(t string) string { return m.roundNum(t) })
		} else {
			inner = reNumGuard.ReplaceAllStringFunc(inner, func(match string) string {
				sub := reNumGuard.FindStringSubmatch(match)
				return sub[1] + m.roundNum(sub[2])
			})
		}
	}
	return string(quote) + inner + string(quote)
}

// reorderFlags 将 flag 参数挪到位置参数之前，使 "位置参数 -flag" 与 "-flag 位置参数" 两种顺序均可解析。
// valueFlags 为需要消费下一个参数的 flag 名集合。
func reorderFlags(args []string, valueFlags map[string]bool) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 1 && a[0] == '-' {
			if _, err := strconv.ParseFloat(a, 64); err == nil { // 负数视为位置参数
				pos = append(pos, a)
				continue
			}
			name := strings.TrimLeft(a, "-")
			if !strings.Contains(name, "=") && valueFlags[name] && i+1 < len(args) {
				flags = append(flags, a, args[i+1])
				i++
				continue
			}
			flags = append(flags, a)
			continue
		}
		pos = append(pos, a)
	}
	return append(flags, pos...)
} // Run 执行命令，返回退出码。
func Run(args []string) int {
	args = reorderFlags(args, map[string]bool{"prec": true, "f": true, "o": true})
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	prec := fs.Int("prec", 3, "数值小数精度")
	stripTitle := fs.Bool("strip-title", false, "移除 title 元素")
	file := fs.String("f", "", "输入文件")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *prec < 0 || *prec > 15 {
		fmt.Fprintln(os.Stderr, "-prec 必须在 0~15 之间")
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
	origSize := len(data)
	if origSize == 0 {
		fmt.Fprintln(os.Stderr, "输入为空")
		return 1
	}

	m := &minifier{prec: *prec, stripSub: *stripTitle}
	s := m.protect(string(data))
	s = reComments.ReplaceAllString(s, "")
	s = reMetadata.ReplaceAllString(s, "")
	s = reDesc.ReplaceAllString(s, "")
	if *stripTitle {
		s = reTitle.ReplaceAllString(s, "")
	}
	s = reTagGap.ReplaceAllString(s, "><")
	s = reWS.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)

	// 逐标签处理属性
	s = reTag.ReplaceAllStringFunc(s, func(tag string) string {
		if strings.HasPrefix(tag, "<?") || strings.HasPrefix(tag, "<!") {
			return tag
		}
		return reAttr.ReplaceAllStringFunc(tag, func(attr string) string {
			sub := reAttr.FindStringSubmatch(attr)
			// sub[1] 形如 " fill="，去掉首尾空白与等号得到属性名
			attrName := strings.ToLower(strings.Trim(sub[1], " =\t\r\n"))
			return sub[1] + m.minifyAttrValue(attrName, sub[2])
		})
	})

	// 迭代删除空组
	for i := 0; i < 20; i++ {
		next := reEmptyG.ReplaceAllString(s, "")
		if next == s {
			break
		}
		s = next
	}

	s = m.restore(s)

	// XML 合法性校验回读
	dec := xml.NewDecoder(strings.NewReader(s))
	dec.Strict = true
	for {
		_, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "压缩后 XML 校验失败: %v\n已放弃输出（可用原始文件）\n", err)
			return 1
		}
	}

	minSize := len(s)
	if *out != "" {
		if err := os.WriteFile(*out, []byte(s), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "写入文件失败:", err)
			return 1
		}
		fmt.Printf("已写入 %s（%d → %d 字节，压缩率 %.1f%%）\n", *out, origSize, minSize, ratio(origSize, minSize))
	} else {
		os.Stdout.WriteString(s)
		fmt.Fprintf(os.Stderr, "压缩统计: %d → %d 字节（压缩率 %.1f%%）\n", origSize, minSize, ratio(origSize, minSize))
	}
	return 0
}

func ratio(orig, min int) float64 {
	if orig == 0 {
		return 0
	}
	return float64(orig-min) / float64(orig) * 100
}
