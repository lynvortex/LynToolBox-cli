// Package placeholder 实现占位图生成命令。
// 对应网页版：work/placeholder-img-tool.html（占位图生成）
//
// 用法：
//
//	lyntoolbox placeholder [-size 800x600] [-bg #EEEEEE] [-fg #AAAAAA] [-text 文字] [-to png|jpg] [-o 输出]
package placeholder

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	Name  = "placeholder"
	Desc  = "生成占位图（Placeholder），可自定义尺寸/颜色/文字"
	Usage = `用法:
  lyntoolbox placeholder [-size 800x600] [-bg #EEEEEE] [-fg #AAAAAA] [-text 文字] [-to png|jpg] [-q 85] [-o 输出]

参数:
  -size    尺寸 宽x高（默认 800x600）
  -bg      背景色（默认 #EEEEEE）
  -fg      前景/文字颜色（默认 #AAAAAA）
  -text    文字（默认 "800 × 600"；内嵌字体仅支持 ASCII，× 会渲染为 x，其他非 ASCII 字符报错）
  -to      输出格式 png|jpg（默认 png）
  -q       JPG 质量 1-100（默认 85）
  -o       输出文件（默认 placeholder_800x600.png）`
)

// parseHexColor 解析 #RRGGBB / #RGB 颜色。
func parseHexColor(s string) (color.RGBA, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return color.RGBA{}, fmt.Errorf("无法解析颜色 %q（应为 #RRGGBB）", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("无法解析颜色 %q", s)
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, nil
}

// sanitizeASCII 将常见非 ASCII 字符替换为等价 ASCII，其余报错。
func sanitizeASCII(text string) (string, error) {
	repl := strings.NewReplacer(
		"×", "x", "✕", "x", "X", "X",
		"：", ":", "，", ",", "。", ".", "（）", "()",
	)
	out := repl.Replace(text)
	for _, r := range out {
		if r > 127 {
			return "", fmt.Errorf("内嵌字体仅支持 ASCII 字符，检测到非 ASCII 字符 %q", r)
		}
	}
	return out, nil
}

// renderText 用 basicfont 渲染文字为 1x 小图。
func renderText(text string, col color.RGBA) (*image.RGBA, error) {
	if text == "" {
		return nil, errors.New("文字为空")
	}
	face := basicfont.Face7x13
	runes := []rune(text)
	w := int(face.Width)*len(runes) + 2
	h := int(face.Height)
	tmp := image.NewRGBA(image.Rect(0, 0, w, h))
	d := &font.Drawer{
		Dst:  tmp,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(1, int(face.Ascent)),
	}
	d.DrawString(text)
	m := d.MeasureString(text)
	tw := int((m + 63) >> 6)
	if tw+1 > w {
		tw = w - 1
	}
	out := image.NewRGBA(image.Rect(0, 0, tw+1, h))
	draw.Draw(out, out.Bounds(), tmp, image.Point{}, draw.Src)
	return out, nil
}

// reorderArgs 将已知选项移到最前，使文件参数可以放在任意位置。
func reorderArgs(args []string, bools, known map[string]bool) []string {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		tok := args[i]
		name := strings.TrimLeft(tok, "-")
		if len(tok) > 1 && tok[0] == '-' && known[name] {
			flags = append(flags, tok)
			if !bools[name] && i+1 < len(args) && !known[strings.TrimLeft(args[i+1], "-")] {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		rest = append(rest, tok)
	}
	return append(flags, rest...)
}

var (
	knownFlags = map[string]bool{"size": true, "bg": true, "fg": true, "text": true, "to": true, "q": true, "o": true}
	boolFlags  = map[string]bool{}
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	size := fs.String("size", "800x600", "尺寸 宽x高")
	bgHex := fs.String("bg", "#EEEEEE", "背景色")
	fgHex := fs.String("fg", "#AAAAAA", "前景色")
	text := fs.String("text", "", "文字（默认 宽 × 高）")
	to := fs.String("to", "png", "输出格式 png|jpg")
	quality := fs.Int("q", 85, "JPG 质量 1-100")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	args = reorderArgs(args, boolFlags, knownFlags)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	// 解析尺寸
	parts := strings.SplitN(strings.ToLower(*size), "x", 2)
	if len(parts) != 2 {
		fmt.Fprintf(os.Stderr, "错误: -size 格式应为 宽x高，如 800x600\n")
		return 2
	}
	w, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	h, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || w < 1 || h < 1 || w > 16384 || h > 16384 {
		fmt.Fprintf(os.Stderr, "错误: 无效尺寸 %q\n", *size)
		return 2
	}
	bg, err := parseHexColor(*bgHex)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		return 2
	}
	fg, err := parseHexColor(*fgHex)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		return 2
	}
	if *quality < 1 || *quality > 100 {
		fmt.Fprintln(os.Stderr, "错误: -q 需在 1-100 之间")
		return 2
	}
	fmtBase := strings.ToLower(*to)
	if fmtBase != "png" && fmtBase != "jpg" && fmtBase != "jpeg" {
		fmt.Fprintln(os.Stderr, "错误: -to 仅支持 png|jpg")
		return 2
	}

	textStr := *text
	if textStr == "" {
		textStr = fmt.Sprintf("%d × %d", w, h)
	}
	textASCII, err := sanitizeASCII(textStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		return 2
	}

	// 画布
	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)

	// 文字：1x 渲染后按整数倍最近邻放大，尽量占画布宽度的 60%
	t1x, err := renderText(textASCII, fg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		return 1
	}
	tw := t1x.Bounds().Dx()
	th := t1x.Bounds().Dy()
	k := 1
	if tw > 0 {
		k = int(float64(w) * 0.6 / float64(tw))
	}
	kh := 1
	if th > 0 {
		kh = int(float64(h) * 0.4 / float64(th))
	}
	if kh < k {
		k = kh
	}
	if k < 1 {
		k = 1
	}
	if k > 32 {
		k = 32
	}
	scaled := image.NewRGBA(image.Rect(0, 0, tw*k, th*k))
	xdraw.NearestNeighbor.Scale(scaled, scaled.Bounds(), t1x, t1x.Bounds(), xdraw.Src, nil)
	ox := (w - tw*k) / 2
	oy := (h - th*k) / 2
	draw.Draw(canvas, image.Rect(ox, oy, ox+tw*k, oy+th*k), scaled, image.Point{}, draw.Over)

	// 输出
	dst := *out
	if dst == "" {
		ext := ".png"
		if fmtBase == "jpg" || fmtBase == "jpeg" {
			ext = ".jpg"
		}
		dst = fmt.Sprintf("placeholder_%dx%d%s", w, h, ext)
	}
	f, err := os.Create(dst)
	if err != nil {
		fmt.Fprintln(os.Stderr, "创建输出失败:", err)
		return 1
	}
	if fmtBase == "jpg" || fmtBase == "jpeg" {
		err = jpeg.Encode(f, canvas, &jpeg.Options{Quality: *quality})
	} else {
		err = png.Encode(f, canvas)
	}
	if err != nil {
		f.Close()
		os.Remove(dst)
		fmt.Fprintln(os.Stderr, "编码失败:", err)
		return 1
	}
	f.Close()
	fmt.Printf("已生成 %dx%d 占位图 → %s\n", w, h, dst)
	return 0
}
