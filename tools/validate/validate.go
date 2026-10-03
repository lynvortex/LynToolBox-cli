// Package validate 实现通用格式校验命令。
// 对应网页版：work/validate-tool.html（格式校验）、daily/idcard-tool.html（身份证解析）
//
// 用法：
//
//	lyntoolbox validate [-t email|ipv4|ipv6|phone|idcard|creditcard|url|uuid] [-info] [文本]
//	cat lines.txt | lyntoolbox validate
//	lyntoolbox validate -t idcard -info 110101199003077774
package validate

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	Name  = "validate"
	Desc  = "通用格式校验：邮箱/IP/手机号/身份证/银行卡/URL/UUID（身份证可解析详情）"
	Usage = `用法: lyntoolbox validate [-t 类型] [-info] [-f 文件] [文本...]

参数:
  -t     指定校验类型：email|ipv4|ipv6|phone|idcard|creditcard|url|uuid
         省略 -t 时自动识别每行输入的类型并校验
  -info  idcard 专用：解析省份/生日/性别/年龄详情
  -f     从文件读取（每行一个值）；省略时用参数，参数为空则读 stdin

判定口径:
  email       常规邮箱格式（本地部分@域名，域名含点分后缀）
  ipv4        严格点分四段 0-255，不允许前导零
  ipv6        标准冒号十六进制格式
  phone       中国手机号 1[3-9] 开头共 11 位
  idcard      18 位居民身份证，验证出生日期与 GB 11643 加权校验位
  creditcard  13-19 位数字 + Luhn 校验，识别 Visa/MasterCard/银联/Amex
  url         http/https URL 且主机名非空
  uuid        8-4-4-4-12 十六进制标准格式`
)

type checker struct {
	name  string
	check func(string) (bool, string) // 合法？、失败原因
}

var (
	reEmail = regexp.MustCompile(`^[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}$`)
	rePhone = regexp.MustCompile(`^1[3-9]\d{9}$`)
	reUUID  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	reDigit = regexp.MustCompile(`^\d+$`)
)

var checkers = map[string]checker{
	"email": {"email", checkEmail},
	"ipv4":  {"ipv4", checkIPv4},
	"ipv6":  {"ipv6", checkIPv6},
	"phone": {"phone", checkPhone},
	"idcard": {"idcard", func(s string) (bool, string) {
		_, err := parseIDCard(s)
		if err != nil {
			return false, err.Error()
		}
		return true, ""
	}},
	"creditcard": {"creditcard", func(s string) (bool, string) {
		_, err := parseCreditCard(s)
		if err != nil {
			return false, err.Error()
		}
		return true, ""
	}},
	"url": {"url", checkURL},
	"uuid": {"uuid", func(s string) (bool, string) {
		if reUUID.MatchString(s) {
			return true, ""
		}
		return false, "UUID 格式应为 8-4-4-4-12 位十六进制"
	}},
}

func checkEmail(s string) (bool, string) {
	if reEmail.MatchString(s) {
		return true, ""
	}
	return false, "邮箱格式不正确（应为 local@domain.tld）"
}

func checkIPv4(s string) (bool, string) {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false, "应为 4 段点分十进制"
	}
	for _, p := range parts {
		if !reDigit.MatchString(p) {
			return false, "含非数字字符"
		}
		if len(p) > 1 && p[0] == '0' {
			return false, "不允许前导零"
		}
		n, err := strconv.Atoi(p)
		if err != nil || n > 255 {
			return false, "每段需在 0~255 之间"
		}
	}
	return true, ""
}

func checkIPv6(s string) (bool, string) {
	ip := net.ParseIP(s)
	if ip == nil || !strings.Contains(s, ":") {
		return false, "不是合法的 IPv6 地址"
	}
	return true, ""
}

func checkPhone(s string) (bool, string) {
	if rePhone.MatchString(s) {
		return true, ""
	}
	return false, "手机号应为 1[3-9] 开头的 11 位数字"
}

// autoDetect 依优先级自动识别类型。
func autoDetect(s string) (checker, bool) {
	order := []string{"uuid", "email", "ipv4", "ipv6", "url", "idcard", "creditcard", "phone"}
	for _, name := range order {
		c := checkers[name]
		if ok, _ := c.check(s); ok {
			return c, true
		}
	}
	return checker{}, false
}

// ---------- 身份证 ----------

var provinceCodes = map[string]string{
	"11": "北京市", "12": "天津市", "13": "河北省", "14": "山西省", "15": "内蒙古自治区",
	"21": "辽宁省", "22": "吉林省", "23": "黑龙江省",
	"31": "上海市", "32": "江苏省", "33": "浙江省", "34": "安徽省", "35": "福建省",
	"36": "江西省", "37": "山东省",
	"41": "河南省", "42": "湖北省", "43": "湖南省", "44": "广东省", "45": "广西壮族自治区", "46": "海南省",
	"50": "重庆市", "51": "四川省", "52": "贵州省", "53": "云南省", "54": "西藏自治区",
	"61": "陕西省", "62": "甘肃省", "63": "青海省", "64": "宁夏回族自治区", "65": "新疆维吾尔自治区",
	"71": "台湾省", "81": "香港特别行政区", "82": "澳门特别行政区",
}

var idWeights = [17]int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
var idCheckCodes = [11]string{"1", "0", "X", "9", "8", "7", "6", "5", "4", "3", "2"}

// parseIDCard 校验并解析 18 位身份证。
func parseIDCard(s string) (map[string]string, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) != 18 {
		return nil, fmt.Errorf("长度应为 18 位（当前 %d）", len(s))
	}
	for i := 0; i < 17; i++ {
		if s[i] < '0' || s[i] > '9' {
			return nil, fmt.Errorf("前 17 位含非数字字符")
		}
	}
	if s[17] != 'X' && (s[17] < '0' || s[17] > '9') {
		return nil, fmt.Errorf("校验位应为数字或 X")
	}
	if _, ok := provinceCodes[s[:2]]; !ok {
		return nil, fmt.Errorf("省份代码 %s 无效", s[:2])
	}
	birth := s[6:14]
	bday, err := time.ParseInLocation("20060102", birth, time.Local)
	if err != nil || bday.Format("20060102") != birth {
		return nil, fmt.Errorf("出生日期 %s 无效", birth)
	}
	sum := 0
	for i := 0; i < 17; i++ {
		n, _ := strconv.Atoi(s[i : i+1])
		sum += n * idWeights[i]
	}
	expect := idCheckCodes[sum%11]
	if string(s[17]) != expect {
		return nil, fmt.Errorf("校验位不符（应为 %s）", expect)
	}

	info := map[string]string{
		"province": provinceCodes[s[:2]],
		"birthday": bday.Format("2006-01-02"),
		"gender":   "女",
	}
	if n, _ := strconv.Atoi(s[16:17]); n%2 == 1 {
		info["gender"] = "男"
	}
	age := time.Now().Year() - bday.Year()
	if time.Now().YearDay() < bday.YearDay() {
		age--
	}
	info["age"] = strconv.Itoa(age)
	info["seq"] = s[14:17]
	info["check"] = s[17:18]
	return info, nil
}

// ---------- 银行卡 ----------

// parseCreditCard 做 Luhn 校验并识别品牌。
func parseCreditCard(s string) (map[string]string, error) {
	s = strings.ReplaceAll(s, " ", "")
	if !reDigit.MatchString(s) {
		return nil, fmt.Errorf("含非数字字符")
	}
	if len(s) < 13 || len(s) > 19 {
		return nil, fmt.Errorf("位数应为 13~19 位（当前 %d）", len(s))
	}
	sum := 0
	double := false
	for i := len(s) - 1; i >= 0; i-- {
		d := int(s[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	if sum%10 != 0 {
		return nil, fmt.Errorf("Luhn 校验失败")
	}
	brand := "未知品牌"
	two := s[:2]
	switch {
	case s[0] == '4':
		brand = "Visa"
	case two == "34" || two == "37":
		brand = "American Express"
	case len(s) >= 4 && s[0] == '5' && s[1] >= '1' && s[1] <= '5',
		len(s) >= 4 && s[:4] >= "2221" && s[:4] <= "2720":
		brand = "MasterCard"
	case two == "62":
		brand = "银联 UnionPay"
	}
	return map[string]string{"brand": brand}, nil
}

func checkURL(s string) (bool, string) {
	u, err := url.Parse(s)
	if err != nil {
		return false, "URL 解析失败"
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false, "仅支持 http/https 协议"
	}
	if u.Host == "" {
		return false, "缺少主机名"
	}
	return true, ""
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
	args = reorderFlags(args, map[string]bool{"t": true, "f": true})
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	typ := fs.String("t", "", "校验类型")
	info := fs.Bool("info", false, "idcard 解析详情")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *info && *typ != "idcard" && *typ != "" {
		fmt.Fprintln(os.Stderr, "-info 仅支持 idcard 类型")
		return 2
	}

	var values []string
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取文件失败:", err)
			return 1
		}
		values = toLines(string(b))
	} else if fs.NArg() > 0 {
		if *typ == "idcard" && *info && fs.NArg() == 1 {
			values = []string{fs.Arg(0)}
		} else {
			// 多参数时每个参数可能本身多行
			values = toLines(strings.Join(fs.Args(), "\n"))
		}
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 stdin 失败:", err)
			return 1
		}
		values = toLines(string(b))
	}
	if len(values) == 0 {
		fmt.Fprintln(os.Stderr, "没有待校验的输入")
		return 2
	}

	exitCode := 0
	for _, v := range values {
		if *info && *typ == "idcard" {
			data, err := parseIDCard(v)
			if err != nil {
				fmt.Printf("%s → 解析失败（%v）\n", v, err)
				exitCode = 1
				continue
			}
			fmt.Printf("%s → 合法 | 省份: %s | 生日: %s | 性别: %s | 年龄: %s | 顺序码: %s | 校验位: %s\n",
				v, data["province"], data["birthday"], data["gender"], data["age"], data["seq"], data["check"])
			continue
		}

		if *typ != "" {
			c, ok := checkers[*typ]
			if !ok {
				fmt.Fprintf(os.Stderr, "不支持的类型: %s\n", *typ)
				return 2
			}
			if ok, reason := c.check(v); ok {
				fmt.Printf("%s → 合法 (%s)\n", v, c.name)
			} else {
				fmt.Printf("%s → 非法（%s）\n", v, reason)
				exitCode = 1
			}
			continue
		}

		// 自动识别
		c, ok := autoDetect(v)
		if !ok {
			fmt.Printf("%s → 非法（无法识别类型）\n", v)
			exitCode = 1
			continue
		}
		ok2, reason := c.check(v)
		if ok2 {
			fmt.Printf("%s → 合法 (%s)\n", v, c.name)
		} else {
			fmt.Printf("%s → 非法（%s）\n", v, reason)
			exitCode = 1
		}
	}
	return exitCode
}

func toLines(s string) []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
