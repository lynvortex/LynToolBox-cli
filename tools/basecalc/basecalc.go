// Package basecalc 实现任意进制转换与位运算命令。
// 对应网页版：text/base-convert-tool.html（进制转换）、work/bitwise-tool.html（位运算）
//
// 用法：
//
//	lyntoolbox basecalc [-from 进制] [-to 进制] [数值]
//	lyntoolbox basecalc <and|or|xor|shl|shr> <a> <b> [-inbase 进制] [-to 进制]
//	lyntoolbox basecalc not <a> [-inbase 进制] [-to 进制]
package basecalc

import (
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"
)

const (
	Name  = "basecalc"
	Desc  = "2-36 进制互转与任意精度位运算（and/or/xor/not/shl/shr），基于 math/big"
	Usage = `用法: lyntoolbox basecalc [-from 进制] [-to 进制] [数值]
      lyntoolbox basecalc <and|or|xor|shl|shr> <a> <b> [-inbase 进制] [-to 进制]
      lyntoolbox basecalc not <a> [-inbase 进制] [-to 进制]

参数:
  -from    输入进制（2-36，默认 10；输入带 0x/0b/0o 前缀时自动识别）
  -to      输出进制（2-36，默认 10）
  -inbase  位运算操作数输入进制（2-36，默认 10，同样支持前缀自动识别）
  数值     待转换的数值；位运算模式下第一个位置参数为运算名
示例:
  lyntoolbox basecalc 255 -to 16          → FF
  lyntoolbox basecalc 0xFF                → 255
  lyntoolbox basecalc and 0b1100 10 -inbase 2 -to 2`
)

// ops 位运算名集合。
var ops = map[string]bool{"and": true, "or": true, "xor": true, "shl": true, "shr": true, "not": true}

// splitArgs 将本命令的已知旗标记号提前到参数列表最前端，
// 使旗标可以写在任意位置，同时保证以 - 开头的数值
// （如 -0b101、-5）不会被 flag 包误认为旗标。
// spec 映射旗标名 → 是否需要取值。
func splitArgs(spec map[string]bool, args []string) ([]string, []string) {
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

// parseBig 按指定进制解析整数，支持 0x/0b/0o 前缀自动识别与负数。
func parseBig(s string, base int) (*big.Int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("空数值")
	}
	b := base
	neg := ""
	body := s
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		neg = string(s[0])
		body = s[1:]
	}
	lower := strings.ToLower(body)
	switch {
	case strings.HasPrefix(lower, "0x"):
		b, body = 16, body[2:]
	case strings.HasPrefix(lower, "0b"):
		b, body = 2, body[2:]
	case strings.HasPrefix(lower, "0o"):
		b, body = 8, body[2:]
	}
	if body == "" {
		return nil, fmt.Errorf("数值 %q 缺少有效数字", s)
	}
	v, ok := new(big.Int).SetString(neg+body, b)
	if !ok {
		return nil, fmt.Errorf("数值 %q 不是合法的 %d 进制整数", s, b)
	}
	return v, nil
}

// formatBig 以指定进制输出（字母统一大写，负号保留）。
func formatBig(v *big.Int, base int) string {
	return strings.ToUpper(v.Text(base))
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	from := fs.Int("from", 10, "输入进制 2-36")
	to := fs.Int("to", 10, "输出进制 2-36")
	inbase := fs.Int("inbase", 10, "位运算输入进制 2-36")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	spec := map[string]bool{"from": true, "to": true, "inbase": true}
	flags, pos := splitArgs(spec, args)
	if err := fs.Parse(flags); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	rest := pos
	if *from < 2 || *from > 36 || *to < 2 || *to > 36 || *inbase < 2 || *inbase > 36 {
		fmt.Fprintln(os.Stderr, "进制必须在 2-36 之间")
		return 2
	}
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "缺少数值（用法见 -h）")
		return 2
	}

	if ops[strings.ToLower(rest[0])] {
		op := strings.ToLower(rest[0])
		operands := rest[1:]
		want := 2
		if op == "not" {
			want = 1
		}
		if len(operands) != want {
			fmt.Fprintf(os.Stderr, "%s 需要 %d 个操作数\n", op, want)
			return 2
		}
		vals := make([]*big.Int, 0, len(operands))
		for _, o := range operands {
			v, err := parseBig(o, *inbase)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			vals = append(vals, v)
		}
		var r *big.Int
		switch op {
		case "and":
			r = new(big.Int).And(vals[0], vals[1])
		case "or":
			r = new(big.Int).Or(vals[0], vals[1])
		case "xor":
			r = new(big.Int).Xor(vals[0], vals[1])
		case "not":
			// 按无符号语义对 -v-1 取反（二进制补码意义上翻转全部位）。
			r = new(big.Int).Not(vals[0])
		case "shl":
			n, err := shiftCount(vals[1])
			if err != "" {
				fmt.Fprintf(os.Stderr, "shl 位移量%s\n", err)
				return 1
			}
			r = new(big.Int).Lsh(vals[0], n)
		case "shr":
			n, err := shiftCount(vals[1])
			if err != "" {
				fmt.Fprintf(os.Stderr, "shr 位移量%s\n", err)
				return 1
			}
			r = new(big.Int).Rsh(vals[0], n)
		}
		fmt.Println(formatBig(r, *to))
		return 0
	}

	// 进制转换模式：全部剩余参数拼接为一个数值（允许中间夹空格）。
	v, err := parseBig(strings.Join(rest, ""), *from)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(formatBig(v, *to))
	return 0
}

// shiftCount 校验位移量为非负且不致内存失控。
func shiftCount(v *big.Int) (uint, string) {
	if v.Sign() < 0 {
		return 0, "不能为负数"
	}
	if !v.IsInt64() || v.Int64() > 1<<20 {
		return 0, "过大（上限 1048576）"
	}
	return uint(v.Int64()), ""
}
