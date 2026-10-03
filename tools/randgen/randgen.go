// Package randgen 实现随机数据生成命令：UUID、密码、令牌、随机数。
// 合并对应网页版：work/uuid-tool.html、work/token-gen-tool.html、
// text/password-tool.html、daily/random-tool.html
//
// 用法：
//
//	lyntoolbox randgen uuid -n 5
//	lyntoolbox randgen password -len 20 -no-ambiguous
//	lyntoolbox randgen token -len 32 -hex
//	lyntoolbox randgen number -min 1 -max 100 -n 10 -unique
package randgen

import (
	"crypto/rand"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"
)

const (
	Name  = "randgen"
	Desc  = "随机数据生成：UUID v4、密码、令牌、随机数（crypto/rand）"
	Usage = `用法: lyntoolbox randgen <子命令> [参数]

子命令:
  uuid      生成 UUID v4
  password  生成随机密码（保证启用的每个字符类别至少出现一次）
  token     生成随机令牌（默认字母数字，-hex 使用十六进制）
  number    生成随机数（默认整数，-dec 指定小数位）

通用参数:
  -n        生成数量（默认 1），结果每行一个输出到 stdout
  -len      长度：password/token 用（默认 16/32，按子命令取默认）
  -min      number 最小值（默认 0，含）
  -max      number 最大值（默认 100，含）
  -dec      number 小数位（默认 0 整数，0-15）
  -unique   number 结果不重复
  -hex      token 使用十六进制字符集
  -no-upper / -no-lower / -no-digit / -no-symbol
            password 去除对应字符集
  -no-ambiguous
            password 去除易混淆字符 0O1lI

说明: 全部基于 crypto/rand 密码学安全随机源。`
)

const (
	upperChars   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	lowerChars   = "abcdefghijklmnopqrstuvwxyz"
	digitChars   = "0123456789"
	symbolChars  = "!@#$%^&*()-_=+[]{}<>,.;:~"
	ambiguousSet = "0O1lI"
	alnumChars   = upperChars + lowerChars + digitChars
	hexChars     = "0123456789abcdef"
)

// randInt 返回 [0, n) 内的密码学安全随机整数。
func randInt(n int64) (int64, error) {
	if n <= 0 {
		return 0, fmt.Errorf("随机范围无效")
	}
	v, err := rand.Int(rand.Reader, big.NewInt(n))
	if err != nil {
		return 0, err
	}
	return v.Int64(), nil
}

// randString 从字符集均匀抽取 length 个字符（拒绝采样避免模偏差）。
func randString(pool string, length int) (string, error) {
	limit := int64(256 - 256%len(pool))
	out := make([]byte, length)
	for i := 0; i < length; i++ {
		for {
			b := make([]byte, 1)
			if _, err := rand.Read(b); err != nil {
				return "", err
			}
			if int64(b[0]) < limit {
				out[i] = pool[int(b[0])%len(pool)]
				break
			}
		}
	}
	return string(out), nil
}

// ---------- uuid ----------

func runUUID(count int) int {
	for i := 0; i < count; i++ {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			fmt.Fprintln(os.Stderr, "生成 UUID 失败:", err)
			return 1
		}
		b[6] = (b[6] & 0x0f) | 0x40 // 版本 4
		b[8] = (b[8] & 0x3f) | 0x80 // RFC 4122 变体
		fmt.Printf("%x-%x-%x-%x-%x\n", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	}
	return 0
}

// ---------- password ----------

func runPassword(o *options) int {
	strip := func(set string) string {
		if !o.noAmbiguous {
			return set
		}
		return strings.Map(func(r rune) rune {
			if strings.ContainsRune(ambiguousSet, r) {
				return -1
			}
			return r
		}, set)
	}

	var classes []string
	if !o.noUpper {
		classes = append(classes, strip(upperChars))
	}
	if !o.noLower {
		classes = append(classes, strip(lowerChars))
	}
	if !o.noDigit {
		classes = append(classes, strip(digitChars))
	}
	if !o.noSymbol {
		classes = append(classes, strip(symbolChars))
	}
	if len(classes) == 0 {
		fmt.Fprintln(os.Stderr, "所有字符集都被排除，无法生成密码")
		return 2
	}
	if o.length < len(classes) {
		fmt.Fprintf(os.Stderr, "密码长度 %d 小于启用的字符类别数 %d\n", o.length, len(classes))
		return 2
	}
	pool := strings.Join(classes, "")

	for i := 0; i < o.count; i++ {
		pwd := make([]byte, 0, o.length)
		// 每个启用类别至少保证一位
		for _, c := range classes {
			s, err := randString(c, 1)
			if err != nil {
				fmt.Fprintln(os.Stderr, "生成密码失败:", err)
				return 1
			}
			pwd = append(pwd, s[0])
		}
		s, err := randString(pool, o.length-len(pwd))
		if err != nil {
			fmt.Fprintln(os.Stderr, "生成密码失败:", err)
			return 1
		}
		pwd = append(pwd, s...)
		// Fisher-Yates 洗牌
		for j := len(pwd) - 1; j > 0; j-- {
			k, err := randInt(int64(j + 1))
			if err != nil {
				fmt.Fprintln(os.Stderr, "生成密码失败:", err)
				return 1
			}
			pwd[j], pwd[k] = pwd[k], pwd[j]
		}
		fmt.Println(string(pwd))
	}
	return 0
}

// ---------- token ----------

func runToken(o *options) int {
	pool := alnumChars
	if o.hex {
		pool = hexChars
	}
	for i := 0; i < o.count; i++ {
		s, err := randString(pool, o.length)
		if err != nil {
			fmt.Fprintln(os.Stderr, "生成令牌失败:", err)
			return 1
		}
		fmt.Println(s)
	}
	return 0
}

// ---------- number ----------

func runNumber(o *options) int {
	if o.dec < 0 || o.dec > 15 {
		fmt.Fprintln(os.Stderr, "-dec 必须在 0-15 之间")
		return 2
	}
	if o.min > o.max {
		fmt.Fprintln(os.Stderr, "-min 不能大于 -max")
		return 2
	}

	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(o.dec)), nil)
	scaledMin := new(big.Int).Mul(big.NewInt(int64(o.min)), scale)
	scaledMax := new(big.Int).Mul(big.NewInt(int64(o.max)), scale)
	span := new(big.Int).Sub(scaledMax, scaledMin)
	span.Add(span, big.NewInt(1)) // 闭区间 [min, max]

	if o.unique && span.Cmp(big.NewInt(int64(o.count))) < 0 {
		fmt.Fprintf(os.Stderr, "范围大小 %s 不足生成 %d 个不重复结果\n", span.String(), o.count)
		return 2
	}

	seen := make(map[string]bool, o.count)
	for i := 0; i < o.count; i++ {
		var out string
		for attempt := 0; ; attempt++ {
			if attempt > 10000 {
				fmt.Fprintln(os.Stderr, "生成失败：范围过小，无法满足不重复要求")
				return 1
			}
			k, err := rand.Int(rand.Reader, span)
			if err != nil {
				fmt.Fprintln(os.Stderr, "生成随机数失败:", err)
				return 1
			}
			out = formatScaled(new(big.Int).Add(scaledMin, k), o.dec)
			if !o.unique || !seen[out] {
				break
			}
		}
		seen[out] = true
		fmt.Println(out)
	}
	return 0
}

// formatScaled 将按 10^dec 缩放的整数格式化为小数字符串。
func formatScaled(v *big.Int, dec int) string {
	if dec == 0 {
		return v.String()
	}
	neg := v.Sign() < 0
	s := new(big.Int).Abs(v).String()
	for len(s) <= dec {
		s = "0" + s
	}
	s = s[:len(s)-dec] + "." + s[len(s)-dec:]
	if neg {
		s = "-" + s
	}
	return s
}

// options 汇总全部旗标。
type options struct {
	count       int
	length      int
	min         int
	max         int
	dec         int
	unique      bool
	hex         bool
	noUpper     bool
	noLower     bool
	noDigit     bool
	noSymbol    bool
	noAmbiguous bool
}

// valueFlags 记录需要取值的旗标名（用于在解析前定位位置参数）。
var valueFlags = map[string]bool{
	"n": true, "len": true, "min": true, "max": true, "dec": true,
}

// splitArgs 从参数中提取前 n 个位置参数（自动跳过旗标及其取值），
// 返回提取出的位置参数与剩余参数。用于子命令式工具：
// Go 的 flag 包遇到首个非旗标参数即停止解析，因此需先剥离子命令。
func splitArgs(args []string, n int) (pos []string, rest []string) {
	rest = []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" { // 其后的参数全部按位置参数处理
			for j := i + 1; j < len(args); j++ {
				if len(pos) < n {
					pos = append(pos, args[j])
				} else {
					rest = append(rest, args[j])
				}
			}
			return pos, rest
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			name := strings.ToLower(strings.TrimLeft(a, "-"))
			if !strings.Contains(name, "=") && valueFlags[name] && i+1 < len(args) {
				rest = append(rest, a, args[i+1])
				i++
				continue
			}
			rest = append(rest, a)
			continue
		}
		if len(pos) < n {
			pos = append(pos, a)
			continue
		}
		rest = append(rest, a)
	}
	return pos, rest
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	o := &options{}
	fs.IntVar(&o.count, "n", 1, "生成数量")
	fs.IntVar(&o.length, "len", 16, "长度")
	fs.IntVar(&o.min, "min", 0, "最小值")
	fs.IntVar(&o.max, "max", 100, "最大值")
	fs.IntVar(&o.dec, "dec", 0, "小数位")
	fs.BoolVar(&o.unique, "unique", false, "随机数不重复")
	fs.BoolVar(&o.hex, "hex", false, "令牌使用十六进制字符集")
	fs.BoolVar(&o.noUpper, "no-upper", false, "密码去除大写字母")
	fs.BoolVar(&o.noLower, "no-lower", false, "密码去除小写字母")
	fs.BoolVar(&o.noDigit, "no-digit", false, "密码去除数字")
	fs.BoolVar(&o.noSymbol, "no-symbol", false, "密码去除符号")
	fs.BoolVar(&o.noAmbiguous, "no-ambiguous", false, "密码去除易混淆字符 0O1lI")
	fs.Usage = func() { fmt.Print(Usage + "\n") }

	subPos, rest := splitArgs(args, 1)
	sub := ""
	if len(subPos) > 0 {
		sub = subPos[0]
	}
	if err := fs.Parse(rest); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if o.count <= 0 {
		fmt.Fprintln(os.Stderr, "-n 必须为正整数")
		return 2
	}

	switch strings.ToLower(sub) {
	case "uuid":
		return runUUID(o.count)
	case "password":
		if o.length < 1 || o.length > 4096 {
			fmt.Fprintln(os.Stderr, "-len 必须在 1-4096 之间")
			return 2
		}
		return runPassword(o)
	case "token":
		if o.length < 1 || o.length > 4096 {
			fmt.Fprintln(os.Stderr, "-len 必须在 1-4096 之间")
			return 2
		}
		return runToken(o)
	case "number":
		return runNumber(o)
	default:
		if sub == "" {
			fmt.Fprintln(os.Stderr, "缺少子命令（uuid|password|token|number）")
		} else {
			fmt.Fprintf(os.Stderr, "未知子命令: %s（可选 uuid|password|token|number）\n", sub)
		}
		return 2
	}
}
