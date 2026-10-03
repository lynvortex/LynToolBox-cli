// Package bignum 实现大数高精度四则运算命令。
// 对应网页版：work/bignum-tool.html（大数计算器）
//
// 用法：
//
//	lyntoolbox bignum eval "表达式"
//	lyntoolbox bignum "0.1+0.2"
//	lyntoolbox bignum -prec 30 "(1/3)*3"
package bignum

import (
	"flag"
	"fmt"
	"math"
	"math/big"
	"os"
	"strings"
)

const (
	Name  = "bignum"
	Desc  = "大数高精度计算：+ - * / % ^ 与括号，基于 math/big 精确求值"
	Usage = `用法: lyntoolbox bignum [-prec 位] [-raw] eval "表达式"
      lyntoolbox bignum [-prec 位] [-raw] "表达式"

说明:
  支持 + - * / % ^（幂，指数必须为整数）与括号，一元正负号，科学计数法 1.2e30。
  内部基于 math/big.Rat 精确计算；/ 除法结果按 -prec 指定的小数位四舍五入输出。
  默认输出带千分位分隔符，-raw 关闭。
示例:
  lyntoolbox bignum "0.1+0.2"             → 0.3
  lyntoolbox bignum "2^100"               → 1267650600228229401496703205376
  lyntoolbox bignum -prec 10 "1/3"        → 0.3333333333`
)

// evaluator 基于 big.Rat 的递归下降求值器。
type evaluator struct {
	src string
	pos int
}

// splitArgs 将本命令的已知旗标记号提前到参数列表最前端，
// 使旗标可以写在任意位置，同时保证以 - 开头的表达式
// （如 "-7%3"、"-2^2"）不会被 flag 包误认为旗标。
func splitArgs(args []string) ([]string, []string) {
	spec := map[string]bool{"prec": true, "raw": false}
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
	prec := fs.Int("prec", 20, "除法输出小数位数（0-1000）")
	raw := fs.Bool("raw", false, "输出不加千分位分隔符")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	flags, pos := splitArgs(args)
	if err := fs.Parse(flags); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *prec < 0 || *prec > 1000 {
		fmt.Fprintln(os.Stderr, "-prec 必须在 0-1000 之间")
		return 2
	}
	exprArgs := pos
	if len(exprArgs) > 0 && strings.EqualFold(exprArgs[0], "eval") {
		exprArgs = exprArgs[1:]
	}
	expr := strings.Join(exprArgs, " ")
	if strings.TrimSpace(expr) == "" {
		fmt.Fprintln(os.Stderr, "缺少表达式（用法见 -h）")
		return 2
	}

	e := &evaluator{src: expr}
	r, err := e.evalExpression()
	if err == nil && len(e.skipSpace()) > 0 {
		err = fmt.Errorf("表达式存在多余内容: %q", e.src[e.pos:])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "表达式错误: %v\n", err)
		return 1
	}
	if r == nil {
		fmt.Fprintln(os.Stderr, "表达式错误: 结果缺失")
		return 1
	}
	if r.Sign() == 0 {
		fmt.Println("0")
		return 0
	}
	fmt.Println(formatRat(r, *prec, !*raw))
	return 0
}

// skipSpace 返回跳过空白后的剩余串。
func (e *evaluator) skipSpace() string {
	for e.pos < len(e.src) {
		c := e.src[e.pos]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			e.pos++
			continue
		}
		break
	}
	return e.src[e.pos:]
}

// evalExpression expr := term (('+'|'-') term)*
func (e *evaluator) evalExpression() (*big.Rat, error) {
	left, err := e.evalTerm()
	if err != nil {
		return nil, err
	}
	for {
		rest := e.skipSpace()
		if rest == "" || (rest[0] != '+' && rest[0] != '-') {
			return left, nil
		}
		op := rest[0]
		e.pos++
		right, err := e.evalTerm()
		if err != nil {
			return nil, err
		}
		if op == '+' {
			left.Add(left, right)
		} else {
			left.Sub(left, right)
		}
	}
}

// evalTerm term := unary (('*'|'/'|'%') unary)*
func (e *evaluator) evalTerm() (*big.Rat, error) {
	left, err := e.evalUnary()
	if err != nil {
		return nil, err
	}
	for {
		rest := e.skipSpace()
		if rest == "" || (rest[0] != '*' && rest[0] != '/' && rest[0] != '%') {
			return left, nil
		}
		op := rest[0]
		e.pos++
		right, err := e.evalUnary()
		if err != nil {
			return nil, err
		}
		switch op {
		case '*':
			left.Mul(left, right)
		case '/':
			if right.Sign() == 0 {
				return nil, fmt.Errorf("除数为 0")
			}
			left.Quo(left, right)
		case '%':
			if right.Sign() == 0 {
				return nil, fmt.Errorf("取模模数为 0")
			}
			// 截断取模：a % b = a - b*trunc(a/b)，符号跟随被除数。
			q := new(big.Rat).Quo(left, right)
			qi := new(big.Int).Quo(q.Num(), q.Denom())
			left.Sub(left, new(big.Rat).Mul(right, new(big.Rat).SetInt(qi)))
		}
	}
}

// evalUnary unary := ('+'|'-') unary | power
func (e *evaluator) evalUnary() (*big.Rat, error) {
	rest := e.skipSpace()
	if rest != "" && (rest[0] == '+' || rest[0] == '-') {
		op := rest[0]
		e.pos++
		v, err := e.evalUnary()
		if err != nil {
			return nil, err
		}
		if op == '-' {
			v.Neg(v)
		}
		return v, nil
	}
	return e.evalPower()
}

// evalPower power := primary ('^' unary)?（右结合，指数可带符号）
func (e *evaluator) evalPower() (*big.Rat, error) {
	base, err := e.evalPrimary()
	if err != nil {
		return nil, err
	}
	rest := e.skipSpace()
	if rest != "" && rest[0] == '^' {
		e.pos++
		exp, err := e.evalUnary()
		if err != nil {
			return nil, err
		}
		if !exp.IsInt() {
			return nil, fmt.Errorf("幂指数必须为整数")
		}
		n := exp.Num()
		if n.BitLen() > 20 {
			return nil, fmt.Errorf("幂指数过大（绝对值上限 1048576）")
		}
		neg := n.Sign() < 0
		e := new(big.Int).Abs(n)
		num := new(big.Int).Exp(base.Num(), e, nil)
		den := new(big.Int).Exp(base.Denom(), e, nil)
		if neg {
			if num.Sign() == 0 {
				return nil, fmt.Errorf("0 不能取负次幂")
			}
			num, den = den, num
		}
		return new(big.Rat).SetFrac(num, den), nil
	}
	return base, nil
}

// evalPrimary primary := 数值 | '(' expr ')'
func (e *evaluator) evalPrimary() (*big.Rat, error) {
	rest := e.skipSpace()
	if rest == "" {
		return nil, fmt.Errorf("表达式意外结束")
	}
	if rest[0] == '(' {
		e.pos++
		v, err := e.evalExpression()
		if err != nil {
			return nil, err
		}
		rest = e.skipSpace()
		if rest == "" || rest[0] != ')' {
			return nil, fmt.Errorf("缺少右括号 )")
		}
		e.pos++
		return v, nil
	}
	return e.parseNumber()
}

// parseNumber 解析十进制数值（可含小数与 e 指数），交给 big.Rat 精确表示。
func (e *evaluator) parseNumber() (*big.Rat, error) {
	rest := e.skipSpace()
	i := 0
	for i < len(rest) && (rest[i] >= '0' && rest[i] <= '9') {
		i++
	}
	if i == 0 {
		return nil, fmt.Errorf("在 %q 处无法解析数值", rest)
	}
	if i < len(rest) && rest[i] == '.' {
		i++
		d := 0
		for i < len(rest) && (rest[i] >= '0' && rest[i] <= '9') {
			i++
			d++
		}
		if d == 0 {
			return nil, fmt.Errorf("小数点后缺少数字: %q", rest)
		}
	}
	if i < len(rest) && (rest[i] == 'e' || rest[i] == 'E') {
		j := i + 1
		if j < len(rest) && (rest[j] == '+' || rest[j] == '-') {
			j++
		}
		d := 0
		for j < len(rest) && (rest[j] >= '0' && rest[j] <= '9') {
			j++
			d++
		}
		if d > 0 {
			i = j
		}
	}
	tok := rest[:i]
	r, ok := new(big.Rat).SetString(tok)
	if !ok {
		return nil, fmt.Errorf("无法解析数值 %q", tok)
	}
	e.pos += i
	return r, nil
}

// formatRat 按 prec 位小数输出，去尾零；thou 为真时对整数部分加千分位。
func formatRat(r *big.Rat, prec int, thou bool) string {
	// 以足够高的二进制精度承载有理数：整数部分位数 + prec 位小数所需位数 + 余量，
	// 避免默认精度（约 64 位）在第 prec 位小数处产生末位噪声。
	bits := r.Num().BitLen() + r.Denom().BitLen() + int(float64(prec)/math.Log10(2)) + 16
	s := new(big.Float).SetPrec(uint(bits)).SetRat(r).Text('f', prec)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	intPart, fracPart := s, ""
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		intPart, fracPart = s[:dot], s[dot+1:]
	}
	fracPart = strings.TrimRight(fracPart, "0")
	if thou && len(intPart) > 3 {
		var b strings.Builder
		lead := len(intPart) % 3
		if lead > 0 {
			b.WriteString(intPart[:lead])
		}
		for k := lead; k < len(intPart); k += 3 {
			if b.Len() > 0 {
				b.WriteByte(',')
			}
			b.WriteString(intPart[k : k+3])
		}
		intPart = b.String()
	}
	out := intPart
	if fracPart != "" {
		out += "." + fracPart
	}
	if neg && (intPart != "0" || fracPart != "") {
		out = "-" + out
	}
	return out
}
