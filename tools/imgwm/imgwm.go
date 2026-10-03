// Package imgwm 实现图片水印命令（文字水印/图片水印）。
// 对应网页版：image/shuiyin-tool.html（图片水印）
//
// 用法：
//
//	lyntoolbox imgwm 图片... -text "文本" [-size 24] [-color #FFFFFF] [-opacity 100] [-pos mc|tile] [-margin 10]
//	lyntoolbox imgwm 图片... -wm 水印.png [-scale 25] [-opacity 80] [-pos br] [-margin 10]
//	lyntoolbox imgwm -dir 目录 ... [-outdir 目录]
package imgwm

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const (
	Name  = "imgwm"
	Desc  = "图片批量加水印，支持文字（9 宫格/平铺）与图片水印"
	Usage = `用法:
  lyntoolbox imgwm 图片... -text 文字 [-size 24] [-color #000000] [-opacity 100] [-pos mc] [-margin 10] [-outdir 目录]
  lyntoolbox imgwm 图片... -wm 水印图.png [-scale 25] [-opacity 100] [-pos br] [-margin 10] [-outdir 目录]
  lyntoolbox imgwm -dir 目录 [同上参数]

参数:
  -text     文字水印内容（内嵌字体仅支持 ASCII，中文请改用 -wm 图片水印）
  -size     文字水印字号（默认 24，按整数倍放大内嵌 7x13 点阵字体）
  -color    文字颜色十六进制（默认 #000000）
  -wm       图片水印文件
  -scale    图片水印宽度占原图宽度百分比（默认 25）
  -opacity  不透明度 0-100（默认 100）
  -pos      位置 tl|tc|tr|ml|mc|mr|bl|bc|br（默认 br 右下角）或 tile 平铺
  -margin   边距像素（默认 10）
  -dir      输入目录（批量）
  -o        单文件输出路径（仅单个输入时可用）
  -outdir   批量输出目录（默认输出到原目录，文件名加 .wm 后缀）`
)

var imgExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".bmp": true, ".tif": true, ".tiff": true, ".webp": true,
}

func isImageFile(p string) bool { return imgExts[strings.ToLower(filepath.Ext(p))] }

func loadImage(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	return image.Decode(f)
}

func encodeImage(w interface{ Write(p []byte) (int, error) }, img image.Image, ext string) error {
	switch strings.TrimPrefix(strings.ToLower(ext), ".") {
	case "png":
		return png.Encode(w, img)
	case "jpg", "jpeg":
		return jpeg.Encode(w, img, &jpeg.Options{Quality: 90})
	case "gif":
		return gif.Encode(w, img, nil)
	case "bmp":
		return bmp.Encode(w, img)
	case "tif", "tiff":
		return tiff.Encode(w, img, nil)
	case "webp":
		return errors.New("暂不支持编码 WebP 输出，请改用 png/jpg/gif/bmp/tiff")
	}
	return fmt.Errorf("不支持的输出格式: %s", ext)
}

func normalizeExt(ext string) string {
	e := strings.ToLower(strings.TrimPrefix(ext, "."))
	switch e {
	case "jpg", "jpeg":
		return ".jpg"
	case "png", "gif", "bmp":
		return "." + e
	case "tif", "tiff":
		return ".tiff"
	}
	return ""
}

func outPath(src, suffix, ext, outdir string) string {
	base := filepath.Base(src)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	name := stem + suffix + ext
	if outdir != "" {
		return filepath.Join(outdir, name)
	}
	return filepath.Join(filepath.Dir(src), name)
}

// parseHexColor 解析 #RRGGBB / #RRGGBBAA / #RGB 颜色。
func parseHexColor(s string) (color.RGBA, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	switch len(s) {
	case 3:
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	case 8:
		v, err := parseHexByte(s[6:]) // alpha
		if err != nil {
			return color.RGBA{}, fmt.Errorf("无法解析颜色 %q", s)
		}
		c, err := parseRGB(s[:6])
		c.A = v
		return c, err
	case 6:
		return parseRGB(s)
	}
	return color.RGBA{}, fmt.Errorf("无法解析颜色 %q（应为 #RRGGBB）", s)
}

func parseRGB(s string) (color.RGBA, error) {
	r, err := parseHexByte(s[:2])
	if err != nil {
		return color.RGBA{}, err
	}
	g, err := parseHexByte(s[2:4])
	if err != nil {
		return color.RGBA{}, err
	}
	b, err := parseHexByte(s[4:6])
	if err != nil {
		return color.RGBA{}, err
	}
	return color.RGBA{R: r, G: g, B: b, A: 255}, nil
}

func parseHexByte(s string) (byte, error) {
	var v byte
	for i := 0; i < 2; i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			v = v*16 + c - '0'
		case c >= 'a' && c <= 'f':
			v = v*16 + c - 'a' + 10
		case c >= 'A' && c <= 'F':
			v = v*16 + c - 'A' + 10
		default:
			return 0, fmt.Errorf("非法十六进制字符 %q", c)
		}
	}
	return v, nil
}

// toNRGBA 将任意图像拷贝为 NRGBA，便于逐像素处理。
func toNRGBA(src image.Image) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

// renderText 用 basicfont 渲染文字为小图（1x），仅支持 ASCII。
func renderText(text string, col color.RGBA) (*image.RGBA, error) {
	for _, r := range text {
		if r > 127 {
			return nil, fmt.Errorf("内嵌字体仅支持 ASCII 字符，检测到非 ASCII 字符 %q；中文水印请改用图片水印（-wm 水印图.png）", r)
		}
	}
	if text == "" {
		return nil, errors.New("水印文字为空")
	}
	face := basicfont.Face7x13
	w := int(face.Width) * len([]rune(text))
	h := int(face.Height)
	tmp := image.NewRGBA(image.Rect(0, 0, w+2, h))
	d := &font.Drawer{
		Dst:  tmp,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(1, int(face.Ascent)),
	}
	d.DrawString(text)
	// 裁剪到实际宽度
	m := d.MeasureString(text)
	tw := int((m + 63) >> 6)
	if tw > w {
		tw = w
	}
	out := image.NewRGBA(image.Rect(0, 0, tw+1, h))
	draw.Draw(out, out.Bounds(), tmp, image.Point{X: 0, Y: 0}, draw.Src)
	return out, nil
}

// scaleText 将 1x 文字图按整数倍最近邻放大，保持点阵清晰。
func scaleText(src *image.RGBA, k int) *image.RGBA {
	if k <= 1 {
		return src
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx()*k, b.Dy()*k))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := src.At(b.Min.X+x, b.Min.Y+y)
			for dy := 0; dy < k; dy++ {
				for dx := 0; dx < k; dx++ {
					dst.Set(x*k+dx, y*k+dy, c)
				}
			}
		}
	}
	return dst
}

// applyOpacity 按 0-100 不透明度整体缩放图像 alpha 通道。
func applyOpacity(img *image.RGBA, opacity int) {
	if opacity >= 100 {
		return
	}
	f := float64(opacity) / 100
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := img.PixOffset(x, y)
			img.Pix[i+3] = uint8(math.Round(float64(img.Pix[i+3]) * f))
		}
	}
}

// wmPositions 计算 9 宫格位置坐标。
func wmPositions(pos string, sw, sh, ww, wh, margin int) []image.Point {
	pad := margin
	innerW := sw - ww - 2*pad
	innerH := sh - wh - 2*pad
	if innerW < 0 {
		innerW = 0
	}
	if innerH < 0 {
		innerH = 0
	}
	var x, y int
	switch pos {
	case "tl", "ml", "bl":
		x = pad
	case "tc", "mc", "bc":
		x = pad + innerW/2
	default: // tr/mr/br
		x = pad + innerW
	}
	switch pos {
	case "tl", "tc", "tr":
		y = pad
	case "ml", "mc", "mr":
		y = pad + innerH/2
	default: // bl/bc/br
		y = pad + innerH
	}
	return []image.Point{{X: x, Y: y}}
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
	knownFlags = map[string]bool{"text": true, "size": true, "color": true, "wm": true, "scale": true, "opacity": true, "pos": true, "margin": true, "dir": true, "o": true, "outdir": true}
	boolFlags  = map[string]bool{}
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	text := fs.String("text", "", "文字水印内容")
	fontSize := fs.Int("size", 24, "文字字号")
	colorHex := fs.String("color", "#000000", "文字颜色 #RRGGBB")
	wmFile := fs.String("wm", "", "图片水印文件")
	wmScale := fs.Int("scale", 25, "图片水印宽度占原图宽度百分比")
	opacity := fs.Int("opacity", 100, "不透明度 0-100")
	pos := fs.String("pos", "br", "位置 tl|tc|tr|ml|mc|mr|bl|bc|br 或 tile")
	margin := fs.Int("margin", 10, "边距像素")
	dir := fs.String("dir", "", "输入目录（批量）")
	out := fs.String("o", "", "单文件输出路径")
	outdir := fs.String("outdir", "", "批量输出目录")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	args = reorderArgs(args, boolFlags, knownFlags)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *text == "" && *wmFile == "" {
		fmt.Fprintln(os.Stderr, "错误: 需要指定 -text 文字水印或 -wm 图片水印")
		return 2
	}
	if *opacity < 0 || *opacity > 100 {
		fmt.Fprintln(os.Stderr, "错误: -opacity 需在 0-100 之间")
		return 2
	}
	if *wmScale <= 0 {
		fmt.Fprintln(os.Stderr, "错误: -scale 需为正数")
		return 2
	}
	if *margin < 0 {
		fmt.Fprintln(os.Stderr, "错误: -margin 不能为负")
		return 2
	}

	col, err := parseHexColor(*colorHex)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		return 2
	}

	// 预生成水印层（文字或图片）
	var wmImg *image.RGBA
	if *text != "" {
		t1x, err := renderText(*text, col)
		if err != nil {
			fmt.Fprintln(os.Stderr, "错误:", err)
			return 2
		}
		k := *fontSize / int(basicfont.Face7x13.Height)
		if k < 1 {
			k = 1
		}
		wmImg = scaleText(t1x, k)
		applyOpacity(wmImg, *opacity)
	} else {
		src, _, err := loadImage(*wmFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取水印图失败:", err)
			return 1
		}
		b := src.Bounds()
		// 先渲染到统一画布方便调透明度
		tmp := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(tmp, tmp.Bounds(), src, b.Min, draw.Src)
		applyOpacity(tmp, *opacity)
		wmImg = tmp
	}

	var inputs []string
	if *dir != "" {
		err := filepath.Walk(*dir, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() && isImageFile(p) {
				inputs = append(inputs, p)
			}
			return nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "遍历目录失败:", err)
			return 1
		}
	}
	for _, a := range fs.Args() {
		if isImageFile(a) {
			inputs = append(inputs, a)
		}
	}
	if len(inputs) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 未指定输入图片（可传文件参数或用 -dir）")
		return 2
	}
	if *out != "" && len(inputs) > 1 {
		fmt.Fprintln(os.Stderr, "错误: 多个输入时应使用 -outdir 而非 -o")
		return 2
	}
	if *outdir != "" {
		if err := os.MkdirAll(*outdir, 0755); err != nil {
			fmt.Fprintln(os.Stderr, "创建输出目录失败:", err)
			return 1
		}
	}

	failed := 0
	for _, src := range inputs {
		if err := watermark(src, wmImg, *wmFile != "", *wmScale, *pos, *margin, *out, *outdir); err != nil {
			fmt.Fprintf(os.Stderr, "处理 %s 失败: %v\n", src, err)
			failed++
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func watermark(src string, wm *image.RGBA, isImageWM bool, wmScale int, pos string, margin int, out, outdir string) error {
	img, format, err := loadImage(src)
	if err != nil {
		return fmt.Errorf("解码失败 %v", err)
	}
	nrgba := toNRGBA(img)
	sw, sh := nrgba.Bounds().Dx(), nrgba.Bounds().Dy()

	// 图片水印按原图宽度百分比缩放
	layer := wm
	if isImageWM {
		ww := sw * wmScale / 100
		if ww < 1 {
			ww = 1
		}
		wRatio := float64(wm.Bounds().Dx()) / float64(wm.Bounds().Dy())
		wh := int(math.Round(float64(ww) / wRatio))
		if wh < 1 {
			wh = 1
		}
		dst := image.NewRGBA(image.Rect(0, 0, ww, wh))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), wm, wm.Bounds(), draw.Src, nil)
		layer = dst
	}

	lw, lh := layer.Bounds().Dx(), layer.Bounds().Dy()
	if lw > sw || lh > sh {
		return fmt.Errorf("水印尺寸 %dx%d 大于原图 %dx%d", lw, lh, sw, sh)
	}

	var pts []image.Point
	if pos == "tile" {
		stepX := lw + margin
		stepY := lh + margin
		if stepX < 1 {
			stepX = 1
		}
		if stepY < 1 {
			stepY = 1
		}
		for y := margin; y+lh <= sh; y += stepY {
			for x := margin; x+lw <= sw; x += stepX {
				pts = append(pts, image.Point{X: x, Y: y})
			}
		}
		if len(pts) == 0 {
			return errors.New("原图太小无法平铺水印")
		}
	} else {
		switch pos {
		case "tl", "tc", "tr", "ml", "mc", "mr", "bl", "bc", "br":
		default:
			return fmt.Errorf("无效位置 %q（可选 tl|tc|tr|ml|mc|mr|bl|bc|br|tile）", pos)
		}
		pts = wmPositions(pos, sw, sh, lw, lh, margin)
	}

	for _, p := range pts {
		draw.Draw(nrgba, image.Rect(p.X, p.Y, p.X+lw, p.Y+lh), layer, image.Point{}, draw.Over)
	}

	outExt := normalizeExt(format)
	if outExt == "" { // webp 等无法编码的格式回退为 png
		outExt = ".png"
	}
	dst := out
	if dst == "" {
		dst = outPath(src, ".wm", outExt, outdir)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if err := encodeImage(f, nrgba, outExt); err != nil {
		f.Close()
		os.Remove(dst)
		return err
	}
	f.Close()
	fmt.Printf("%s: 已加水印（%s %dx%d）→ %s\n", src, map[bool]string{true: "图片水印", false: "文字水印"}[isImageWM], lw, lh, dst)
	return nil
}
