// Package maskdata 实现敏感信息打码命令。
// 对应网页版：work/mask-tool.html（敏感信息脱敏）、text/sponge-tool.html（文本 sponge）
//
// 用法：
//
//	lyntoolbox maskdata -f user.txt
//	lyntoolbox maskdata -types phone,idcard -mode full -f user.txt
//	echo "手机 13812345678" | lyntoolbox maskdata
package maskdata

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
	Name  = "maskdata"
	Desc  = "敏感信息打码：手机号/身份证/邮箱/银行卡/IPv4/车牌，支持多种打码模式"
	Usage = `用法: lyntoolbox maskdata [-types 类型] [-mode 模式] [-f 文件] [文本]

参数:
  -types  逗号分隔的类型列表，默认全部:
          phone(手机号) idcard(身份证) email(邮箱)
          bankcard(银行卡 13-19 位 Luhn 校验) ipv4 plate(车牌 普通+新能源)
  -mode   打码模式:
          partial 默认，手机 138****5678、身份证前3后4、邮箱用户名保留首字符、
                  银行卡后4、车牌保留省份简称+末1、IPv4 掩去末段
          full    整段全部替换为 *
          keep1   保留第一个字符，其余替换为 *
  -f      从文件读取文本；省略文本与 -f 时从 stdin 读取
输出:
  打码后的文本，末尾统计各类型命中数
示例:
  lyntoolbox maskdata -f user.txt
  lyntoolbox maskdata -types phone,idcard -mode full -f user.txt
  echo "手机 13812345678" | lyntoolbox maskdata`
)

type hit struct {
	start, end int
	typ        string
}

// 类型优先级：同位置重叠时先保留更具体的类型
var typeOrder = []string{"email", "idcard", "plate", "bankcard", "phone", "ipv4"}

var (
	emailRe = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	idRe    = regexp.MustCompile(`\b[0-9]{17}[0-9Xx]\b`)
	// 省份简称 + 发牌机关字母 + 5 位（普通）或 6 位（新能源），兼容挂学警港澳
	plateRe = regexp.MustCompile(
		`[京津沪渝冀豫云辽黑湘皖鲁新苏浙赣鄂桂甘晋蒙陕吉闽贵粤青藏川宁琼使领]` +
			`[A-HJ-NP-Z][A-HJ-NP-Z0-9]{4}(?:[A-HJ-NP-Z0-9]{2}|[A-HJ-NP-Z0-9挂学警港澳])`)
	digitRe   = regexp.MustCompile(`[0-9]+`)
	ipv4Re    = regexp.MustCompile(`[0-9]{1,3}(?:\.[0-9]{1,3}){3}`)
	idWeights = [17]int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
)

const idcardCodes = "10X98765432"

func validIDCard(s string) bool {
	sum := 0
	for i := 0; i < 17; i++ {
		sum += int(s[i]-'0') * idWeights[i]
	}
	want := idcardCodes[sum%11]
	got := s[17]
	if got == 'x' {
		got = 'X'
	}
	return got == want
}

// luhn 校验银行卡号。
func luhn(s string) bool {
	sum, alt := 0, false
	for i := len(s) - 1; i >= 0; i-- {
		d := int(s[i] - '0')
		if alt {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		alt = !alt
	}
	return sum%10 == 0
}

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

func boundaryOK(text string, lo, hi int, digits, extra string) bool {
	if lo > 0 && (strings.IndexByte(digits, text[lo-1]) >= 0 || strings.IndexByte(extra, text[lo-1]) >= 0) {
		return false
	}
	if hi < len(text) && (strings.IndexByte(digits, text[hi]) >= 0 || strings.IndexByte(extra, text[hi]) >= 0) {
		return false
	}
	return true
}

// maskSpan 按 mode 对命中片段打码。
func maskSpan(s, typ, mode string) string {
	r := []rune(s)
	n := len(r)
	stars := func(k int) string { return strings.Repeat("*", k) }
	switch mode {
	case "full":
		return stars(n)
	case "keep1":
		return string(r[0]) + stars(n-1)
	default: // partial
		switch typ {
		case "phone":
			return s[:3] + "****" + s[7:]
		case "idcard":
			return s[:3] + stars(n-7) + s[n-4:]
		case "email":
			return string(r[0]) + "***" + s[strings.Index(s, "@"):]
		case "bankcard":
			return stars(n-4) + s[n-4:]
		case "plate":
			return string(r[0]) + stars(n-2) + string(r[n-1])
		case "ipv4":
			return s[:strings.LastIndex(s, ".")] + ".*"
		}
	}
	return s
}

// collectHits 收集启用类型的全部命中（未去重）。
func collectHits(text string, want map[string]bool) []hit {
	var hits []hit
	if want["email"] {
		for _, loc := range emailRe.FindAllStringIndex(text, -1) {
			hits = append(hits, hit{loc[0], loc[1], "email"})
		}
	}
	if want["idcard"] {
		for _, loc := range idRe.FindAllStringIndex(text, -1) {
			if validIDCard(text[loc[0]:loc[1]]) {
				hits = append(hits, hit{loc[0], loc[1], "idcard"})
			}
		}
	}
	if want["plate"] {
		for _, loc := range plateRe.FindAllStringIndex(text, -1) {
			hits = append(hits, hit{loc[0], loc[1], "plate"})
		}
	}
	if want["bankcard"] || want["phone"] {
		for _, loc := range digitRe.FindAllStringIndex(text, -1) {
			n := loc[1] - loc[0]
			switch {
			case want["phone"] && n == 11 && text[loc[0]] == '1' && text[loc[0]+1] >= '3' && text[loc[0]+1] <= '9':
				hits = append(hits, hit{loc[0], loc[1], "phone"})
			case want["bankcard"] && n >= 13 && n <= 19 && luhn(text[loc[0]:loc[1]]):
				hits = append(hits, hit{loc[0], loc[1], "bankcard"})
			}
		}
	}
	if want["ipv4"] {
		for _, loc := range ipv4Re.FindAllStringIndex(text, -1) {
			if boundaryOK(text, loc[0], loc[1], "0123456789", ".") && validIPv4(text[loc[0]:loc[1]]) {
				hits = append(hits, hit{loc[0], loc[1], "ipv4"})
			}
		}
	}
	return hits
}

// resolveHits 排序并去掉重叠命中：先开始的优先，同位置更长的优先，再按类型优先级。
func resolveHits(hits []hit) []hit {
	rank := map[string]int{}
	for i, t := range typeOrder {
		rank[t] = i
	}
	sort.Slice(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.start != b.start {
			return a.start < b.start
		}
		la, lb := a.end-a.start, b.end-b.start
		if la != lb {
			return la > lb
		}
		return rank[a.typ] < rank[b.typ]
	})
	var out []hit
	lastEnd := -1
	for _, h := range hits {
		if h.start < lastEnd {
			continue
		}
		out = append(out, h)
		lastEnd = h.end
	}
	return out
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	types := fs.String("types", "", "逗号分隔的类型列表，默认全部")
	mode := fs.String("mode", "partial", "打码模式 partial|full|keep1")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	validModes := map[string]bool{"partial": true, "full": true, "keep1": true}
	if !validModes[*mode] {
		fmt.Fprintf(os.Stderr, "错误: 未知模式 %q，可用值 partial|full|keep1\n", *mode)
		return 2
	}

	aliases := map[string]string{
		"phone": "phone", "手机": "phone", "手机号": "phone",
		"idcard": "idcard", "身份证": "idcard", "身份证号": "idcard",
		"email": "email", "邮箱": "email", "邮件": "email",
		"bankcard": "bankcard", "银行卡": "bankcard", "卡号": "bankcard",
		"ipv4": "ipv4", "ip": "ipv4",
		"plate": "plate", "车牌": "plate", "车牌号": "plate",
	}
	want := map[string]bool{}
	if strings.TrimSpace(*types) == "" {
		for _, t := range typeOrder {
			want[t] = true
		}
	} else {
		for _, raw := range strings.Split(*types, ",") {
			key := strings.TrimSpace(raw)
			t, ok := aliases[strings.ToLower(key)]
			if !ok {
				t, ok = aliases[key]
			}
			if !ok {
				fmt.Fprintf(os.Stderr, "错误: 未知类型 %q，可用值 phone|idcard|email|bankcard|ipv4|plate（可用中文）\n", key)
				return 2
			}
			want[t] = true
		}
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

	hits := resolveHits(collectHits(text, want))
	var b strings.Builder
	last := 0
	counts := map[string]int{}
	for _, h := range hits {
		b.WriteString(text[last:h.start])
		b.WriteString(maskSpan(text[h.start:h.end], h.typ, *mode))
		last = h.end
		counts[h.typ]++
	}
	b.WriteString(text[last:])
	fmt.Println(b.String())
	fmt.Printf("统计: 手机号 %d 个，身份证 %d 个，邮箱 %d 个，银行卡 %d 个，IPv4 %d 个，车牌 %d 个\n",
		counts["phone"], counts["idcard"], counts["email"], counts["bankcard"], counts["ipv4"], counts["plate"])
	return 0
}
