// Package colorcalc 实现颜色计算命令。
// 对应网页版：daily/contrast-check-tool.html（对比度检查）、image/color-mix-tool.html（颜色混合）
//
// 用法：
//
//	lyntoolbox colorcalc conv #3498db
//	lyntoolbox colorcalc contrast #ffffff #000000
//	lyntoolbox colorcalc mix #ff0000 #0000ff -r 0.5
//	lyntoolbox colorcalc shades #3498db -n 10
//	lyntoolbox colorcalc lighten #3498db -p 20
//	lyntoolbox colorcalc darken #3498db -p 20
package colorcalc

import (
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

const (
	Name  = "colorcalc"
	Desc  = "颜色格式转换/对比度/混合/明暗阶梯计算"
	Usage = `用法: lyntoolbox colorcalc <子命令> [参数]

子命令:
  conv <颜色>                    HEX↔RGB↔HSL↔HSV 全格式输出
  contrast <颜色1> <颜色2>       WCAG 对比度（1~21）与 AA/AAA 判定（正文/大字）
  mix <颜色1> <颜色2> [-r 0.5]   线性混合，-r 为第二色占比（0~1）
  shades <颜色> [-n 10]          明暗阶梯：保持色相/饱和度，亮度从 5% 到 95% 均分
  lighten <颜色> [-p 20]         HSL 亮度提升百分比
  darken <颜色> [-p 20]          HSL 亮度降低百分比

颜色格式: #RGB / #RRGGBB / rgb(r,g,b) / "r,g,b"`
)

type rgb struct{ r, g, b uint8 }
type hsl struct{ h, s, l float64 } // h 0~360, s/l 0~1
type hsv struct{ h, s, v float64 }

// parseColor 解析 HEX / rgb() / 逗号分隔颜色。
func parseColor(s string) (rgb, error) {
	s = strings.TrimSpace(s)
	low := strings.ToLower(s)
	if strings.HasPrefix(low, "rgb(") && strings.HasSuffix(s, ")") {
		inner := strings.TrimSuffix(strings.TrimPrefix(low, "rgb("), ")")
		return parseChannels(inner)
	}
	if strings.HasPrefix(s, "#") {
		s = s[1:]
	} else if strings.Contains(s, ",") {
		return parseChannels(s)
	}
	switch len(s) {
	case 3:
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	case 6:
	default:
		return rgb{}, fmt.Errorf("无法解析颜色 %q（支持 #RGB/#RRGGBB/rgb(r,g,b)）", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return rgb{}, fmt.Errorf("无法解析颜色 %q", s)
	}
	return rgb{uint8(v >> 16), uint8(v >> 8), uint8(v)}, nil
}

func parseChannels(s string) (rgb, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 3 {
		return rgb{}, fmt.Errorf("rgb 需要 3 个通道")
	}
	var ch [3]uint8
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 || n > 255 {
			return rgb{}, fmt.Errorf("通道值无效: %q（0~255）", strings.TrimSpace(p))
		}
		ch[i] = uint8(n)
	}
	return rgb{ch[0], ch[1], ch[2]}, nil
}

func (c rgb) hex() string { return fmt.Sprintf("#%02X%02X%02X", c.r, c.g, c.b) }

func rgbToHSL(c rgb) hsl {
	r, g, b := float64(c.r)/255, float64(c.g)/255, float64(c.b)/255
	max, min := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l := (max + min) / 2
	if max == min {
		return hsl{0, 0, l}
	}
	d := max - min
	s := d / (2 - max - min)
	if l <= 0.5 {
		s = d / (max + min)
	}
	var h float64
	switch max {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return hsl{h, s, l}
}

func hslToRGB(c hsl) rgb {
	h := math.Mod(math.Mod(c.h, 360)+360, 360) / 360
	s, l := c.s, c.l
	if s == 0 {
		v := uint8(math.Round(l * 255))
		return rgb{v, v, v}
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	return rgb{
		hueToByte(p, q, h+1.0/3),
		hueToByte(p, q, h),
		hueToByte(p, q, h-1.0/3),
	}
}

func hueToByte(p, q, t float64) uint8 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6:
		return uint8(math.Round((p + (q-p)*6*t) * 255))
	case t < 0.5:
		return uint8(math.Round(q * 255))
	case t < 2.0/3:
		return uint8(math.Round((p + (q-p)*(2.0/3-t)*6) * 255))
	}
	return uint8(math.Round(p * 255))
}

func rgbToHSV(c rgb) hsv {
	r, g, b := float64(c.r)/255, float64(c.g)/255, float64(c.b)/255
	max, min := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	d := max - min
	v := max
	s := 0.0
	if max > 0 {
		s = d / max
	}
	if d == 0 {
		return hsv{0, s, v}
	}
	var h float64
	switch max {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return hsv{h, s, v}
}

// wcagLuminance WCAG 相对亮度。
func wcagLuminance(c rgb) float64 {
	lin := func(x float64) float64 {
		x /= 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(float64(c.r)) + 0.7152*lin(float64(c.g)) + 0.0722*lin(float64(c.b))
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, "缺少子命令\n"+Usage+"\n")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "conv":
		return runConv(rest)
	case "contrast":
		return runContrast(rest)
	case "mix":
		return runMix(rest)
	case "shades":
		return runShades(rest)
	case "lighten", "darken":
		return runShift(sub, rest)
	case "-h", "-help", "help":
		fmt.Print(Usage + "\n")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n可用: conv contrast mix shades lighten darken\n", sub)
		return 2
	}
}

func runConv(args []string) int {
	fs := flag.NewFlagSet(Name+" conv", flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox colorcalc conv <颜色>")
		return 2
	}
	c, err := parseColor(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	h, v := rgbToHSL(c), rgbToHSV(c)
	fmt.Printf("HEX: %s\n", c.hex())
	fmt.Printf("RGB: rgb(%d, %d, %d)\n", c.r, c.g, c.b)
	fmt.Printf("HSL: hsl(%.1f, %.1f%%, %.1f%%)\n", h.h, h.s*100, h.l*100)
	fmt.Printf("HSV: hsv(%.1f, %.1f%%, %.1f%%)\n", v.h, v.s*100, v.v*100)
	return 0
}

func runContrast(args []string) int {
	fs := flag.NewFlagSet(Name+" contrast", flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox colorcalc contrast <颜色1> <颜色2>")
		return 2
	}
	c1, err := parseColor(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	c2, err := parseColor(fs.Arg(1))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	l1, l2 := wcagLuminance(c1), wcagLuminance(c2)
	hi, lo := math.Max(l1, l2), math.Min(l1, l2)
	ratio := (hi + 0.05) / (lo + 0.05)

	fmt.Printf("对比度: %.2f:1\n", ratio)
	fmt.Printf("正文 AA(≥4.5):  %s\n", pass(ratio >= 4.5))
	fmt.Printf("正文 AAA(≥7):   %s\n", pass(ratio >= 7))
	fmt.Printf("大字 AA(≥3):    %s\n", pass(ratio >= 3))
	fmt.Printf("大字 AAA(≥4.5): %s\n", pass(ratio >= 4.5))
	return 0
}

func pass(ok bool) string {
	if ok {
		return "通过"
	}
	return "未通过"
}

func runMix(args []string) int {
	args = reorderFlags(args, map[string]bool{"r": true})
	fs := flag.NewFlagSet(Name+" mix", flag.ContinueOnError)
	ratio := fs.Float64("r", 0.5, "第二色占比 0~1")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox colorcalc mix <颜色1> <颜色2> [-r 0.5]")
		return 2
	}
	if *ratio < 0 || *ratio > 1 {
		fmt.Fprintln(os.Stderr, "-r 必须在 0~1 之间")
		return 2
	}
	c1, err := parseColor(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	c2, err := parseColor(fs.Arg(1))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	t := *ratio
	out := rgb{
		uint8(math.Round(float64(c1.r)*(1-t) + float64(c2.r)*t)),
		uint8(math.Round(float64(c1.g)*(1-t) + float64(c2.g)*t)),
		uint8(math.Round(float64(c1.b)*(1-t) + float64(c2.b)*t)),
	}
	fmt.Printf("混合结果: %s rgb(%d, %d, %d)\n", out.hex(), out.r, out.g, out.b)
	return 0
}

func runShades(args []string) int {
	args = reorderFlags(args, map[string]bool{"n": true})
	fs := flag.NewFlagSet(Name+" shades", flag.ContinueOnError)
	n := fs.Int("n", 10, "阶梯级数")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox colorcalc shades <颜色> [-n 10]")
		return 2
	}
	if *n < 2 || *n > 100 {
		fmt.Fprintln(os.Stderr, "-n 必须在 2~100 之间")
		return 2
	}
	c, err := parseColor(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	h := rgbToHSL(c)
	for i := 0; i < *n; i++ {
		l := 0.05 + 0.9*float64(i)/float64(*n-1)
		sc := hslToRGB(hsl{h.h, h.s, l})
		fmt.Printf("%s  (L %.0f%%)\n", sc.hex(), l*100)
	}
	return 0
}

func runShift(sub string, args []string) int {
	args = reorderFlags(args, map[string]bool{"p": true})
	fs := flag.NewFlagSet(Name+" "+sub, flag.ContinueOnError)
	p := fs.Float64("p", 10, "亮度调整百分比")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "用法: lyntoolbox colorcalc %s <颜色> [-p 百分比]\n", sub)
		return 2
	}
	if *p < 0 || *p > 100 {
		fmt.Fprintln(os.Stderr, "-p 必须在 0~100 之间")
		return 2
	}
	c, err := parseColor(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	h := rgbToHSL(c)
	delta := *p / 100
	if sub == "darken" {
		delta = -delta
	}
	out := hslToRGB(hsl{h.h, h.s, clamp(h.l+delta, 0, 1)})
	fmt.Printf("%s: %s rgb(%d, %d, %d)\n", sub, out.hex(), out.r, out.g, out.b)
	return 0
}
