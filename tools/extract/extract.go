// Package extract 实现文本信息提取命令。
// 对应网页版：text/extract-tool.html（文本提取）
//
// 用法：
//
//	lyntoolbox extract -what url -f page.txt
//	echo "联系 13812345678" | lyntoolbox extract
//	lyntoolbox extract -what idcard "身份证 11010519491231002X"
package extract

import (
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
)

const (
	Name  = "extract"
	Desc  = "从文本提取 URL/邮箱/手机号/IPv4/IPv6/身份证，去重排序并统计"
	Usage = `用法: lyntoolbox extract [-what 类型] [-f 文件] [文本]

参数:
  -what   提取类型: url|email|phone|ipv4|ipv6|idcard|all（默认 all）
  -f      从文件读取文本；省略文本与 -f 时从 stdin 读取
规则说明:
  url     https?:// 开头，遇空白、引号、尖括号结束
  email   常规邮箱格式
  phone   中国大陆手机号 1[3-9] 开头的 11 位数字（边界断言，不匹配更长数字串）
  ipv4    各段 0-255 校验，前后不得紧邻数字或点
  ipv6    简易校验（十六进制分组 + :: 缩写，不含 IPv4 嵌套格式）
  idcard  18 位身份证（末位可为 X），做校验位验证，不合法的会被过滤
输出:
  全部结果合并去重排序输出，末尾统计各类数量
示例:
  lyntoolbox extract -what url -f page.txt
  echo "联系 13812345678" | lyntoolbox extract
  lyntoolbox extract -what idcard "身份证 11010519491231002X"`
)

var (
	urlRe    = regexp.MustCompile(`https?://[^\s"'<>]+`)
	emailRe  = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	digitRe  = regexp.MustCompile(`[0-9]+`)
	ipv4Re   = regexp.MustCompile(`[0-9]{1,3}(?:\.[0-9]{1,3}){3}`)
	hexrunRe = regexp.MustCompile(`[0-9A-Fa-f:]+`)
	idcardRe = regexp.MustCompile(`\b[0-9]{17}[0-9Xx]\b`)
)

const idcardCodes = "10X98765432"

var idcardWeights = [17]int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}

// validIDCard 校验 18 位身份证校验位。
func validIDCard(s string) bool {
	sum := 0
	for i := 0; i < 17; i++ {
		sum += int(s[i]-'0') * idcardWeights[i]
	}
	want := idcardCodes[sum%11]
	got := s[17]
	if got == 'x' {
		got = 'X'
	}
	return got == want
}

// validIPv4 校验点分四段：各段 0-255。
func validIPv4(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if p == "" || len(p) > 3 {
			return false
		}
		n := 0
		for i := 0; i < len(p); i++ {
			n = n*10 + int(p[i]-'0')
		}
		if n > 255 {
			return false
		}
	}
	return true
}

// validIPv6 简易校验：十六进制分组，至多一处 :: 缩写；不含 IPv4 嵌套格式。
func validIPv6(s string) bool {
	if len(s) < 2 || !strings.Contains(s, ":") {
		return false
	}
	if strings.Contains(s, ":::") {
		return false
	}
	if strings.Count(s, "::") > 1 {
		return false
	}
	countGroups := func(part string) (int, bool) {
		if part == "" {
			return 0, true
		}
		n := 0
		for _, g := range strings.Split(part, ":") {
			if g == "" || len(g) > 4 {
				return 0, false
			}
			for i := 0; i < len(g); i++ {
				if !isHexByte(g[i]) {
					return 0, false
				}
			}
			n++
		}
		return n, true
	}
	if strings.Contains(s, "::") {
		parts := strings.Split(s, "::")
		ln, ok1 := countGroups(parts[0])
		rn, ok2 := countGroups(parts[1])
		return ok1 && ok2 && ln+rn <= 7
	}
	n, ok := countGroups(s)
	return ok && n == 8
}

func isHexByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

// boundaryOK 检查匹配位置前后不紧邻 digitSet 中的字符。
func boundaryOK(text string, lo, hi int, digitSet, extra string) bool {
	if lo > 0 {
		c := text[lo-1]
		if strings.IndexByte(digitSet, c) >= 0 || strings.IndexByte(extra, c) >= 0 {
			return false
		}
	}
	if hi < len(text) {
		c := text[hi]
		if strings.IndexByte(digitSet, c) >= 0 || strings.IndexByte(extra, c) >= 0 {
			return false
		}
	}
	return true
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	what := fs.String("what", "all", "提取类型 url|email|phone|ipv4|ipv6|idcard|all")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	want := map[string]bool{}
	switch strings.ToLower(*what) {
	case "all":
		for _, t := range []string{"url", "email", "phone", "ipv4", "ipv6", "idcard"} {
			want[t] = true
		}
	case "url", "email", "phone", "ipv4", "ipv6", "idcard":
		want[strings.ToLower(*what)] = true
	default:
		fmt.Fprintf(os.Stderr, "错误: 未知类型 %q，可用值 url|email|phone|ipv4|ipv6|idcard|all\n", *what)
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
	text := string(data)

	counts := map[string]int{}
	results := map[string]bool{}
	add := func(typ, s string) {
		counts[typ]++
		results[s] = true
	}

	if want["url"] {
		for _, m := range urlRe.FindAllString(text, -1) {
			add("url", m)
		}
	}
	if want["email"] {
		for _, m := range emailRe.FindAllString(text, -1) {
			add("email", m)
		}
	}
	if want["phone"] {
		// 整段数字串恰好 11 位且 1[3-9] 开头，天然避免匹配更长数字
		for _, m := range digitRe.FindAllString(text, -1) {
			if len(m) == 11 && m[0] == '1' && m[1] >= '3' && m[1] <= '9' {
				add("phone", m)
			}
		}
	}
	if want["ipv4"] {
		for _, loc := range ipv4Re.FindAllStringIndex(text, -1) {
			m := text[loc[0]:loc[1]]
			if !boundaryOK(text, loc[0], loc[1], "0123456789", ".") {
				continue
			}
			if validIPv4(m) {
				add("ipv4", m)
			}
		}
	}
	if want["ipv6"] {
		for _, m := range hexrunRe.FindAllString(text, -1) {
			if validIPv6(m) {
				add("ipv6", m)
			}
		}
	}
	if want["idcard"] {
		for _, m := range idcardRe.FindAllString(text, -1) {
			if validIDCard(m) {
				add("idcard", strings.ToUpper(m))
			}
		}
	}

	found := make([]string, 0, len(results))
	for s := range results {
		found = append(found, s)
	}
	sort.Strings(found)
	for _, s := range found {
		fmt.Println(s)
	}
	fmt.Printf("统计: URL %d 个，邮箱 %d 个，手机号 %d 个，IPv4 %d 个，IPv6 %d 个，身份证 %d 个\n",
		counts["url"], counts["email"], counts["phone"], counts["ipv4"], counts["ipv6"], counts["idcard"])
	return 0
}
