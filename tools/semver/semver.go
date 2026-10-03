// Package semver 实现语义化版本（SemVer 2.0.0）解析、比较与范围检查命令。
// 对应网页版：text/semver-tool.html（语义化版本）
//
// 用法：
//
//	lyntoolbox semver parse <版本>
//	lyntoolbox semver compare <v1> <v2>
//	lyntoolbox semver check <版本> -range 表达式
//	lyntoolbox semver sort <版本>...
package semver

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	Name  = "semver"
	Desc  = "语义化版本（SemVer 2.0.0）解析、比较、范围检查与排序"
	Usage = `用法: lyntoolbox semver parse <版本>
      lyntoolbox semver compare <v1> <v2>
      lyntoolbox semver check <版本> -range 表达式
      lyntoolbox semver sort <版本>...

说明:
  严格遵循 SemVer 2.0.0（主.次.修订-预发布+构建），可省略版本前缀 v。
  范围表达式支持 ^ ~ > >= < <= = ，约束之间以空格或逗号分隔，全部满足（与运算）。
  ^1.2.3 → >=1.2.3 <2.0.0    ~1.2.3 → >=1.2.3 <1.3.0
示例:
  lyntoolbox semver parse v1.2.3-alpha.1+build
  lyntoolbox semver compare 1.0.0-alpha 1.0.0
  lyntoolbox semver check 1.5.0 -range "^1.0.0 <2.0.0"
  lyntoolbox semver sort 1.0.0 1.0.0-alpha 1.0.0-alpha.1 0.9.9`
)

// semverRe 严格 SemVer 2.0.0 官方正则。
var semverRe = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
	`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?` +
	`(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// version 解析后的语义化版本。
type version struct {
	major, minor, patch uint64
	pre                 []string // 预发布标识符（"." 拆分）
	hasPre              bool
	build               string
	raw                 string
}

// parseVersion 严格解析；允许可选的 v 前缀。
func parseVersion(s string) (*version, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(strings.ToLower(s), "v") {
		s = s[1:]
	}
	m := semverRe.FindStringSubmatch(s)
	if m == nil {
		return nil, fmt.Errorf("%q 不符合 SemVer 2.0.0（主.次.修订，各段无前导零，预发布与构建格式见规范）", s)
	}
	v := &version{raw: s, build: m[5], hasPre: m[4] != ""}
	cores := []*uint64{&v.major, &v.minor, &v.patch}
	for i, g := range []string{m[1], m[2], m[3]} {
		n, err := strconv.ParseUint(g, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("版本号段 %q 超出范围", g)
		}
		*cores[i] = n
	}
	if v.hasPre {
		v.pre = strings.Split(m[4], ".")
	}
	return v, nil
}

// compare 按 SemVer 2.0.0 第 11 节优先级规则比较，返回 -1/0/1。
func compare(a, b *version) int {
	if a.major != b.major {
		return signInt64(int64(a.major) - int64(b.major))
	}
	if a.minor != b.minor {
		return signInt64(int64(a.minor) - int64(b.minor))
	}
	if a.patch != b.patch {
		return signInt64(int64(a.patch) - int64(b.patch))
	}
	switch {
	case !a.hasPre && !b.hasPre:
		return 0
	case !a.hasPre:
		return 1 // 无预发布版本优先级更高
	case !b.hasPre:
		return -1
	}
	for i := 0; i < len(a.pre) && i < len(b.pre); i++ {
		if c := comparePreID(a.pre[i], b.pre[i]); c != 0 {
			return c
		}
	}
	return signInt64(int64(len(a.pre)) - int64(len(b.pre)))
}

// comparePreID 比较单个预发布标识符：数字按数值、数字小于字母串、字母串按 ASCII。
func comparePreID(x, y string) int {
	xn, xe := strconv.ParseUint(x, 10, 64)
	yn, ye := strconv.ParseUint(y, 10, 64)
	switch {
	case xe == nil && ye == nil:
		return signInt64(int64(xn) - int64(yn))
	case xe == nil:
		return -1
	case ye == nil:
		return 1
	}
	return strings.Compare(x, y)
}

func signInt64(d int64) int {
	switch {
	case d < 0:
		return -1
	case d > 0:
		return 1
	}
	return 0
}

// splitArgs 将本命令的已知旗标记号提前到参数列表最前端，
// 使旗标（如 check 的 -range）可以写在任意位置。
func splitArgs(args []string) ([]string, []string) {
	spec := map[string]bool{"range": true}
	var flags, pos []string
	i := 0
	for ; i < len(args); i++ {
		t := args[i]
		if t == "--" {
			i++
			break
		}
		if strings.HasPrefix(t, "-") && strings.TrimLeft(t, "-") != "" {
			name := strings.TrimLeft(t, "-")
			hasValue := false
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				name, hasValue = name[:eq], true
			}
			if takes, known := spec[name]; known {
				flags = append(flags, t)
				if !hasValue && takes && i+1 < len(args) {
					i++
					flags = append(flags, args[i])
				}
				continue
			}
			if name == "h" || name == "help" {
				flags = append(flags, t)
				continue
			}
		}
		pos = append(pos, t)
	}
	pos = append(pos, args[i:]...)
	return flags, pos
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	rangeExpr := fs.String("range", "", "范围表达式（check 子命令）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	flags, pos := splitArgs(args)
	if err := fs.Parse(flags); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	rest := pos
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "缺少子命令（parse|compare|check|sort）")
		return 2
	}
	switch strings.ToLower(rest[0]) {
	case "parse":
		if len(rest) != 2 {
			fmt.Fprintln(os.Stderr, "parse 需要恰好一个版本参数")
			return 2
		}
		return runParse(rest[1])
	case "compare":
		if len(rest) != 3 {
			fmt.Fprintln(os.Stderr, "compare 需要两个版本参数")
			return 2
		}
		return runCompare(rest[1], rest[2])
	case "check":
		if len(rest) != 2 {
			fmt.Fprintln(os.Stderr, "check 需要一个版本参数（范围用 -range 给出）")
			return 2
		}
		return runCheck(rest[1], *rangeExpr)
	case "sort":
		if len(rest) < 2 {
			fmt.Fprintln(os.Stderr, "sort 需要至少两个版本参数")
			return 2
		}
		return runSort(rest[1:])
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s（可选 parse|compare|check|sort）\n", rest[0])
		return 2
	}
}

// runParse 输出版本各组成与合法性。
func runParse(s string) int {
	v, err := parseVersion(s)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		fmt.Println("合法: 否")
		return 1
	}
	pre := "-"
	if v.hasPre {
		pre = strings.Join(v.pre, ".")
	}
	build := "-"
	if v.build != "" {
		build = v.build
	}
	fmt.Printf("major:      %d\n", v.major)
	fmt.Printf("minor:      %d\n", v.minor)
	fmt.Printf("patch:      %d\n", v.patch)
	fmt.Printf("prerelease: %s\n", pre)
	fmt.Printf("build:      %s\n", build)
	fmt.Println("合法:       是")
	return 0
}

// runCompare 输出比较结果 -1/0/1 与中文关系。
func runCompare(s1, s2 string) int {
	v1, err := parseVersion(s1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	v2, err := parseVersion(s2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	c := compare(v1, v2)
	fmt.Println(c)
	switch c {
	case -1:
		fmt.Printf("%s 小于 %s\n", v1.raw, v2.raw)
	case 0:
		fmt.Printf("%s 等于 %s\n", v1.raw, v2.raw)
	default:
		fmt.Printf("%s 大于 %s\n", v1.raw, v2.raw)
	}
	return 0
}

// runSort 升序输出多个版本。
func runSort(list []string) int {
	vers := make([]*version, 0, len(list))
	for _, s := range list {
		v, err := parseVersion(s)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
		vers = append(vers, v)
	}
	sort.SliceStable(vers, func(i, j int) bool { return compare(vers[i], vers[j]) < 0 })
	for _, v := range vers {
		fmt.Println(v.raw)
	}
	return 0
}

// looseRangeRe 范围约束中的宽松版本号（允许缺省次/修订段）。
var looseRangeRe = regexp.MustCompile(`^(>=|<=|==|=|>|<|\^|~)?v?(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

// constraint 一条范围约束。
type constraint struct {
	op       string // "", "=", "^", "~", ">", ">=", "<", "<="
	major    uint64
	minor    uint64
	patch    uint64
	pre      []string
	hasPre   bool
	minorSet bool
	patchSet bool
}

// parseConstraint 解析单条约束。
func parseConstraint(tok string) (*constraint, error) {
	m := looseRangeRe.FindStringSubmatch(tok)
	if m == nil {
		return nil, fmt.Errorf("无法解析范围约束 %q", tok)
	}
	c := &constraint{op: m[1]}
	if c.op == "==" {
		c.op = "="
	}
	var err error
	if c.major, err = strconv.ParseUint(m[2], 10, 64); err != nil {
		return nil, fmt.Errorf("版本号段超出范围: %q", tok)
	}
	if m[3] != "" {
		c.minorSet = true
		if c.minor, err = strconv.ParseUint(m[3], 10, 64); err != nil {
			return nil, fmt.Errorf("版本号段超出范围: %q", tok)
		}
	}
	if m[4] != "" {
		c.patchSet = true
		if c.patch, err = strconv.ParseUint(m[4], 10, 64); err != nil {
			return nil, fmt.Errorf("版本号段超出范围: %q", tok)
		}
	}
	if m[5] != "" {
		c.hasPre = true
		c.pre = strings.Split(m[5], ".")
	}
	return c, nil
}

// ver 组装一个用于比较的 version。
func ver(major, minor, patch uint64, pre []string, hasPre bool) *version {
	return &version{major: major, minor: minor, patch: patch, pre: pre, hasPre: hasPre}
}

// bounds 返回 ^ / ~ 约束对应的 [下限, 上限) 区间（npm 语义）。
func (c *constraint) bounds() (lo, hi *version) {
	lo = ver(c.major, c.minor, c.patch, c.pre, c.hasPre)
	switch {
	case !c.minorSet: // ^1、~1 → [1.0.0, 2.0.0)
		hi = ver(c.major+1, 0, 0, nil, false)
	case c.op == "~": // ~1.2[.3] → [1.2.0, 1.3.0)
		hi = ver(c.major, c.minor+1, 0, nil, false)
	case c.major > 0: // ^1.2.3 → [1.2.3, 2.0.0)
		hi = ver(c.major+1, 0, 0, nil, false)
	case !c.patchSet: // ^0.2 → [0.2.0, 0.3.0)
		hi = ver(0, c.minor+1, 0, nil, false)
	case c.minor > 0: // ^0.2.3 → [0.2.3, 0.3.0)
		hi = ver(0, c.minor+1, 0, nil, false)
	default: // ^0.0.3 → [0.0.3, 0.0.4)
		hi = ver(0, 0, c.patch+1, nil, false)
	}
	return lo, hi
}

// satisfied 判断版本 v 是否满足约束。
func (c *constraint) satisfied(v *version) bool {
	bound := ver(c.major, c.minor, c.patch, c.pre, c.hasPre)
	switch c.op {
	case "=", "":
		return compare(v, bound) == 0
	case "^", "~":
		lo, hi := c.bounds()
		return compare(v, lo) >= 0 && compare(v, hi) < 0
	case ">":
		return compare(v, bound) > 0
	case ">=":
		return compare(v, bound) >= 0
	case "<":
		return compare(v, bound) < 0
	case "<=":
		return compare(v, bound) <= 0
	}
	return false
}

// parseRange 拆分范围表达式：空格与逗号分隔，孤立运算符与后续版本合并。
func parseRange(expr string) ([]*constraint, error) {
	toks := strings.FieldsFunc(expr, func(r rune) bool {
		return r == ' ' || r == '\t' || r == ','
	})
	merged := make([]string, 0, len(toks))
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if isBareOp(t) && i+1 < len(toks) {
			merged = append(merged, t+toks[i+1])
			i++
			continue
		}
		merged = append(merged, t)
	}
	if len(merged) == 0 {
		return nil, fmt.Errorf("范围表达式为空")
	}
	cs := make([]*constraint, 0, len(merged))
	for _, t := range merged {
		c, err := parseConstraint(t)
		if err != nil {
			return nil, err
		}
		cs = append(cs, c)
	}
	return cs, nil
}

func isBareOp(t string) bool {
	switch t {
	case ">=", "<=", ">", "<", "=", "==", "^", "~":
		return true
	}
	return false
}

// runCheck 检查版本是否满足范围表达式。
func runCheck(vs, expr string) int {
	if strings.TrimSpace(expr) == "" {
		fmt.Fprintln(os.Stderr, "check 需要 -range 范围表达式")
		return 2
	}
	v, err := parseVersion(vs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	cs, err := parseRange(expr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 2
	}
	ok := true
	for _, c := range cs {
		if !c.satisfied(v) {
			ok = false
			break
		}
	}
	if ok {
		fmt.Printf("%s 满足 %s\n", v.raw, strings.TrimSpace(expr))
		return 0
	}
	fmt.Printf("%s 不满足 %s\n", v.raw, strings.TrimSpace(expr))
	return 1
}
