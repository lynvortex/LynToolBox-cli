// Package calc 实现科学计算器命令。
// 对应网页版：text/calc-tool.html（科学计算器）
//
// 用法：
//
//	lyntoolbox calc "表达式"
//	lyntoolbox calc -deg "sin(30)"
//	lyntoolbox calc -p 6 "sqrt(2)"
package calc

import (
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

const (
	Name  = "calc"
	Desc  = "科学计算器：四则运算、幂、括号、三角/双曲/对数等函数与常量"
	Usage = `用法: lyntoolbox calc [-deg] [-p 位] "表达式"

运算:
  + - * / %（取余）  ^（幂，右结合）  括号
函数:
  sin cos tan asin acos atan sinh cosh tanh
  ln log log2 log10 sqrt cbrt abs floor ceil round exp
常量:
  pi  e      字面量: 十进制 1.5e3  十六进制 0x1F  二进制 0b1010  八进制 0o17
参数:
  -deg    角度制：三角函数入参、反三角函数出参按度处理（默认弧度制）
  -p      输出有效数字位数（1-17，默认 10）
示例:
  lyntoolbox calc "sin(pi/2)"     → 1
  lyntoolbox calc "0x10 + 0b101"  → 21
  lyntoolbox calc -deg "cos(60)"  → 0.5`
)

// funcs 一元函数表。degAffected 表示受 -deg 角度制影响。
var funcs = map[string]struct {
	fn          func(float64) float64
	degAffected bool
}{
	"sin":   {math.Sin, true},
	"cos":   {math.Cos, true},
	"tan":   {math.Tan, true},
	"asin":  {math.Asin, true},
	"acos":  {math.Acos, true},
	"atan":  {math.Atan, true},
	"sinh":  {math.Sinh, false},
	"cosh":  {math.Cosh, false},
	"tanh":  {math.Tanh, false},
	"ln":    {math.Log, false},
	"log":   {math.Log10, false},
	"log2":  {math.Log2, false},
	"log10": {math.Log10, false},
	"sqrt":  {math.Sqrt, false},
	"cbrt":  {math.Cbrt, false},
	"abs":   {math.Abs, false},
	"floor": {math.Floor, false},
	"ceil":  {math.Ceil, false},
	"round": {math.Round, false},
	"exp":   {math.Exp, false},
}

// evaluator float64 递归下降求值器。
type evaluator struct {
	src string
	pos int
	deg bool
}

// splitArgs 将本命令的已知旗标记号提前到参数列表最前端，
// 使旗标可以写在任意位置，同时保证以 - 开头的表达式
// （如 "-2^2"、负数字面量）不会被 flag 包误认为旗标。
func splitArgs(args []string) ([]string, []string) {
	spec := map[string]bool{"deg": false, "p": true}
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
	deg := fs.Bool("deg", false, "角度制")
	prec := fs.Int("p", 10, "输出有效数字位数")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	flags, pos := splitArgs(args)
	if err := fs.Parse(flags); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *prec < 1 || *prec > 17 {
		fmt.Fprintln(os.Stderr, "-p 必须在 1-17 之间")
		return 2
	}
	expr := strings.Join(pos, " ")
	if strings.TrimSpace(expr) == "" {
		fmt.Fprintln(os.Stderr, "缺少表达式（用法见 -h）")
		return 2
	}

	e := &evaluator{src: expr, deg: *deg}
	v, err := e.evalExpression()
	if err == nil && len(e.skipSpace()) > 0 {
		err = fmt.Errorf("表达式存在多余内容: %q", e.src[e.pos:])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "表达式错误: %v\n", err)
		return 1
	}
	fmt.Println(strconv.FormatFloat(v, 'g', *prec, 64))
	return 0
}

// skipSpace 跳过空白并返回剩余串。
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
func (e *evaluator) evalExpression() (float64, error) {
	left, err := e.evalTerm()
	if err != nil {
		return 0, err
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
			return 0, err
		}
		if op == '+' {
			left += right
		} else {
			left -= right
		}
	}
}

// evalTerm term := unary (('*'|'/'|'%') unary)*
func (e *evaluator) evalTerm() (float64, error) {
	left, err := e.evalUnary()
	if err != nil {
		return 0, err
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
			return 0, err
		}
		switch op {
		case '*':
			left *= right
		case '/':
			if right == 0 {
				return 0, fmt.Errorf("除数为 0")
			}
			left /= right
		case '%':
			if right == 0 {
				return 0, fmt.Errorf("取余模数为 0")
			}
			left = math.Mod(left, right)
		}
	}
}

// evalUnary unary := ('+'|'-') unary | power（因此 -2^2 = -4）
func (e *evaluator) evalUnary() (float64, error) {
	rest := e.skipSpace()
	if rest != "" && (rest[0] == '+' || rest[0] == '-') {
		op := rest[0]
		e.pos++
		v, err := e.evalUnary()
		if err != nil {
			return 0, err
		}
		if op == '-' {
			v = -v
		}
		return v, nil
	}
	return e.evalPower()
}

// evalPower power := primary ('^' unary)?
func (e *evaluator) evalPower() (float64, error) {
	base, err := e.evalPrimary()
	if err != nil {
		return 0, err
	}
	rest := e.skipSpace()
	if rest != "" && rest[0] == '^' {
		e.pos++
		exp, err := e.evalUnary()
		if err != nil {
			return 0, err
		}
		return math.Pow(base, exp), nil
	}
	return base, nil
}

// evalPrimary primary := 数值 | 常量 | 函数 '(' expr ')' | '(' expr ')'
func (e *evaluator) evalPrimary() (float64, error) {
	rest := e.skipSpace()
	if rest == "" {
		return 0, fmt.Errorf("表达式意外结束")
	}
	if rest[0] == '(' {
		e.pos++
		v, err := e.evalExpression()
		if err != nil {
			return 0, err
		}
		rest = e.skipSpace()
		if rest == "" || rest[0] != ')' {
			return 0, fmt.Errorf("缺少右括号 )")
		}
		e.pos++
		return v, nil
	}
	if isDigit(rest[0]) || rest[0] == '.' {
		return e.parseNumber()
	}
	if isLetter(rest[0]) {
		return e.parseIdent()
	}
	return 0, fmt.Errorf("无法识别的字符 %q", string(rest[0]))
}

// parseIdent 解析常量或函数调用。
func (e *evaluator) parseIdent() (float64, error) {
	rest := e.skipSpace()
	i := 0
	for i < len(rest) && (isLetter(rest[i]) || isDigit(rest[i]) || rest[i] == '_') {
		i++
	}
	name := strings.ToLower(rest[:i])
	e.pos += i
	switch name {
	case "pi":
		return math.Pi, nil
	case "e":
		return math.E, nil
	}
	f, ok := funcs[name]
	if !ok {
		return 0, fmt.Errorf("未知函数或常量 %q", name)
	}
	rest = e.skipSpace()
	if rest == "" || rest[0] != '(' {
		return 0, fmt.Errorf("函数 %s 后缺少括号", name)
	}
	e.pos++
	v, err := e.evalExpression()
	if err != nil {
		return 0, err
	}
	rest = e.skipSpace()
	if rest == "" || rest[0] != ')' {
		return 0, fmt.Errorf("函数 %s 缺少右括号 )", name)
	}
	e.pos++
	if f.degAffected && e.deg {
		if name[0] == 'a' { // asin/acos/atan：出参转度
			return f.fn(v) * 180 / math.Pi, nil
		}
		return f.fn(v * math.Pi / 180), nil // sin/cos/tan：入参转弧度
	}
	return f.fn(v), nil
}

// parseNumber 解析十进制（含 e 指数）或 0x/0b/0o 字面量。
func (e *evaluator) parseNumber() (float64, error) {
	rest := e.skipSpace()
	lower := strings.ToLower(rest)
	switch {
	case strings.HasPrefix(lower, "0x"), strings.HasPrefix(lower, "0b"), strings.HasPrefix(lower, "0o"):
		base := 8
		digits := "01234567"
		switch lower[1] {
		case 'x':
			base, digits = 16, "0123456789abcdef"
		case 'b':
			base, digits = 2, "01"
		}
		i := 2
		for i < len(rest) && strings.IndexByte(digits, lower[i]) >= 0 {
			i++
		}
		if i == 2 {
			return 0, fmt.Errorf("%q 缺少有效数字", rest)
		}
		u, err := strconv.ParseUint(rest[2:i], base, 64)
		if err != nil {
			return 0, fmt.Errorf("无法解析字面量 %q: %v", rest[:i], err)
		}
		e.pos += i
		return float64(u), nil
	}

	i := 0
	for i < len(rest) && isDigit(rest[i]) {
		i++
	}
	if i < len(rest) && rest[i] == '.' {
		i++
		for i < len(rest) && isDigit(rest[i]) {
			i++
		}
	}
	if i == 0 || (i == 1 && rest[0] == '.') {
		return 0, fmt.Errorf("无法解析数值: %q", rest)
	}
	if i < len(rest) && (rest[i] == 'e' || rest[i] == 'E') {
		j := i + 1
		if j < len(rest) && (rest[j] == '+' || rest[j] == '-') {
			j++
		}
		d := 0
		for j < len(rest) && isDigit(rest[j]) {
			j++
			d++
		}
		if d > 0 {
			i = j
		}
	}
	v, err := strconv.ParseFloat(rest[:i], 64)
	if err != nil {
		return 0, fmt.Errorf("无法解析数值 %q", rest[:i])
	}
	e.pos += i
	return v, nil
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
