// Package imggeom 实现图片几何操作命令：裁剪/旋转/翻转/拼接/宫格切图。
// 对应网页版：image/crop-tool.html（图片裁剪）、image/rotate-flip-tool.html（图片旋转翻转）、
// image/img-merge-tool.html（图片拼接）、image/grid-cut-tool.html（九宫格切图）
//
// 用法：
//
//	lyntoolbox imggeom crop 图片... [-x 0 -y 0 -w 100 -h 100 | -ratio 16:9] [-outdir 目录]
//	lyntoolbox imggeom rotate 图片... [-deg 90|180|270|任意角度] [-bg #FFFFFF|none]
//	lyntoolbox imggeom flip 图片... [-h] [-v]
//	lyntoolbox imggeom concat 图片... [-mode h|v] [-gap 0] [-color #FFFFFF|none] [-o 输出]
//	lyntoolbox imggeom grid 图片... [-m 3|4] [-outdir 目录]
package imggeom

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
	"strconv"
	"strings"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const (
	Name  = "imggeom"
	Desc  = "图片几何操作：裁剪/旋转/翻转/拼接/九宫格十六宫格切图，支持批量"
	Usage = `用法:
  lyntoolbox imggeom crop 图片... [-x N -y N -w N -h N | -ratio 16:9] [-outdir 目录] [-o 输出]
  lyntoolbox imggeom rotate 图片... [-deg 90] [-bg none|#RRGGBB] [-outdir 目录] [-o 输出]
  lyntoolbox imggeom flip 图片... [-h] [-v] [-outdir 目录] [-o 输出]
  lyntoolbox imggeom concat 图片... [-mode h] [-gap 0] [-color #FFFFFF|none] [-o 输出]
  lyntoolbox imggeom grid 图片... [-m 3] [-outdir 目录]

子命令:
  crop     裁剪：-x/-y/-w/-h 指定区域；或 -ratio "16:9" 居中裁剪到指定比例
  rotate   旋转：-deg 90/180/270 直角无损旋转，其他角度画布扩大并填充 -bg（默认 none 透明）
  flip     翻转：-h 水平翻转，-v 垂直翻转（两者可同用）
  concat   拼接：-mode h 横向 / v 纵向，-gap 间距像素，-color 填充色（none 为透明）
  grid     切图：-m 3 九宫格 / 4 十六宫格，输出 m*m 个文件（加 .grid.N 后缀）`
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
		return jpeg.Encode(w, img, &jpeg.Options{Quality: 92})
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

func parseHexColor(s string) (color2, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return 0, fmt.Errorf("无法解析颜色 %q（应为 #RRGGBB）", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("无法解析颜色 %q", s)
	}
	return color2(v), nil
}

// color2 用 uint32 表示 0xRRGGBB，避免与 image/color 名字冲突。
type color2 uint32

func (c color2) rgba() (r, g, b uint8) {
	return uint8(c >> 16 & 0xFF), uint8(c >> 8 & 0xFF), uint8(c & 0xFF)
}

// saveImage 编码保存，ext 决定格式。
func saveImage(dst string, img image.Image, ext string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if err := encodeImage(f, img, ext); err != nil {
		f.Close()
		os.Remove(dst)
		return err
	}
	return f.Close()
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

// 各子命令的选项集（bool 为无值开关）。
var (
	cropFlags   = map[string]bool{"x": true, "y": true, "w": true, "h": true, "ratio": true, "outdir": true, "o": true}
	rotateFlags = map[string]bool{"deg": true, "bg": true, "outdir": true, "o": true}
	flipFlags   = map[string]bool{"h": true, "v": true, "outdir": true, "o": true}
	flipBools   = map[string]bool{"h": true, "v": true}
	concatFlags = map[string]bool{"mode": true, "gap": true, "color": true, "outdir": true, "o": true}
	gridFlags   = map[string]bool{"m": true, "outdir": true}
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, "错误: 缺少子命令 crop|rotate|flip|concat|grid\n\n")
		fmt.Fprint(os.Stderr, Usage+"\n")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "crop":
		return runCrop(rest)
	case "rotate":
		return runRotate(rest)
	case "flip":
		return runFlip(rest)
	case "concat":
		return runConcat(rest)
	case "grid":
		return runGrid(rest)
	default:
		fmt.Fprintf(os.Stderr, "错误: 未知子命令 %q（可用: crop, rotate, flip, concat, grid）\n\n", sub)
		fmt.Fprint(os.Stderr, Usage+"\n")
		return 2
	}
}

// collect 收集输入文件。
func collect(args []string) []string {
	var list []string
	for _, a := range args {
		if info, err := os.Stat(a); err == nil && info.IsDir() {
			_ = filepath.Walk(a, func(p string, fi os.FileInfo, err error) error {
				if err == nil && fi.Mode().IsRegular() && isImageFile(p) {
					list = append(list, p)
				}
				return nil
			})
		} else if isImageFile(a) {
			list = append(list, a)
		}
	}
	return list
}

// commonFlags 解析批处理工具共有的 -outdir/-o 参数。
func commonFlags(fs *flag.FlagSet, outdir, out *string) {
	fs.StringVar(outdir, "outdir", "", "批量输出目录")
	fs.StringVar(out, "o", "", "单文件输出路径")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
}

func checkOutFlags(outdir, out string, n int) int {
	if out != "" && n > 1 {
		fmt.Fprintln(os.Stderr, "错误: 多个输入时应使用 -outdir 而非 -o")
		return 2
	}
	if outdir != "" {
		if err := os.MkdirAll(outdir, 0755); err != nil {
			fmt.Fprintln(os.Stderr, "创建输出目录失败:", err)
			return 1
		}
	}
	return 0
}

func batchLoop(inputs []string, fn func(src string) (string, error)) int {
	failed := 0
	for _, src := range inputs {
		dst, err := fn(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "处理 %s 失败: %v\n", src, err)
			failed++
			continue
		}
		fmt.Printf("%s → %s\n", src, dst)
	}
	if failed > 0 {
		return 1
	}
	return 0
}

// resolveOutExt 决定输出扩展名；webp 源回退 png。
func resolveOutExt(format string) string {
	ext := normalizeExt(format)
	if ext == "" {
		ext = ".png"
	}
	return ext
}

func runCrop(args []string) int {
	fs := flag.NewFlagSet("crop", flag.ContinueOnError)
	args = reorderArgs(args, nil, cropFlags)
	x := fs.Int("x", 0, "起点 X")
	y := fs.Int("y", 0, "起点 Y")
	w := fs.Int("w", 0, "裁剪宽度")
	h := fs.Int("h", 0, "裁剪高度")
	ratio := fs.String("ratio", "", "目标比例，如 16:9（居中裁剪）")
	var outdir, out string
	commonFlags(fs, &outdir, &out)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	inputs := collect(fs.Args())
	if len(inputs) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 未指定输入图片")
		return 2
	}
	if *w < 0 || *h < 0 || *x < 0 || *y < 0 {
		fmt.Fprintln(os.Stderr, "错误: -x/-y/-w/-h 不能为负")
		return 2
	}
	if *ratio == "" && (*w == 0 || *h == 0) {
		fmt.Fprintln(os.Stderr, "错误: 需要 -x/-y/-w/-h 或 -ratio 比例")
		return 2
	}
	var rw, rh int
	if *ratio != "" {
		parts := strings.SplitN(*ratio, ":", 2)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "错误: -ratio 格式应为 宽:高，如 16:9\n")
			return 2
		}
		a, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		b, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err1 != nil || err2 != nil || a <= 0 || b <= 0 {
			fmt.Fprintf(os.Stderr, "错误: 无效比例 %q\n", *ratio)
			return 2
		}
		rw, rh = a, b
	}
	if code := checkOutFlags(outdir, out, len(inputs)); code != 0 {
		return code
	}
	return batchLoop(inputs, func(src string) (string, error) {
		img, format, err := loadImage(src)
		if err != nil {
			return "", fmt.Errorf("解码失败 %v", err)
		}
		ext := resolveOutExt(format)
		b := img.Bounds()
		sw, sh := b.Dx(), b.Dy()
		var rect image.Rectangle
		if *ratio != "" {
			// 居中裁剪到 rw:rh
			cw, ch := sw, sh
			if float64(sw)/float64(sh) > float64(rw)/float64(rh) {
				cw = int(math.Round(float64(sh) * float64(rw) / float64(rh)))
			} else {
				ch = int(math.Round(float64(sw) * float64(rh) / float64(rw)))
			}
			if cw > sw {
				cw = sw
			}
			if ch > sh {
				ch = sh
			}
			x0 := b.Min.X + (sw-cw)/2
			y0 := b.Min.Y + (sh-ch)/2
			rect = image.Rect(x0, y0, x0+cw, y0+ch)
		} else {
			cw, ch := *w, *h
			if cw > sw {
				cw = sw
			}
			if ch > sh {
				ch = sh
			}
			if *x+cw > sw {
				cw = sw - *x
			}
			if *y+ch > sh {
				ch = sh - *y
			}
			if cw <= 0 || ch <= 0 {
				return "", fmt.Errorf("裁剪区域超出图片范围（原图 %dx%d）", sw, sh)
			}
			rect = image.Rect(b.Min.X+*x, b.Min.Y+*y, b.Min.X+*x+cw, b.Min.Y+*y+ch)
		}
		dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
		draw.Draw(dst, dst.Bounds(), img, rect.Min, draw.Src)
		out := out
		if out == "" {
			out = outPath(src, ".crop", ext, outdir)
		}
		if err := saveImage(out, dst, ext); err != nil {
			return "", err
		}
		fmt.Printf("  %s: %dx%d → %dx%d\n", filepath.Base(src), sw, sh, rect.Dx(), rect.Dy())
		return out, nil
	})
}

func runRotate(args []string) int {
	fs := flag.NewFlagSet("rotate", flag.ContinueOnError)
	args = reorderArgs(args, nil, rotateFlags)
	deg := fs.Int("deg", 90, "旋转角度（90/180/270 或任意角度）")
	bg := fs.String("bg", "none", "任意角度时的填充色 none|#RRGGBB（none 为透明）")
	var outdir, out string
	commonFlags(fs, &outdir, &out)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	inputs := collect(fs.Args())
	if len(inputs) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 未指定输入图片")
		return 2
	}
	var bgColor color2
	hasBG := false
	if *bg != "none" && *bg != "" {
		c, err := parseHexColor(*bg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "错误:", err)
			return 2
		}
		bgColor = c
		hasBG = true
	}
	if code := checkOutFlags(outdir, out, len(inputs)); code != 0 {
		return code
	}
	d := *deg % 360
	if d < 0 {
		d += 360
	}
	return batchLoop(inputs, func(src string) (string, error) {
		img, format, err := loadImage(src)
		if err != nil {
			return "", fmt.Errorf("解码失败 %v", err)
		}
		ext := resolveOutExt(format)
		var rotated image.Image
		switch d {
		case 0:
			rotated = img
		case 90, 180, 270:
			rotated = rotateExact(img, d)
		default:
			rotated = rotateAny(img, float64(d), bgColor, hasBG)
		}
		o := out
		if o == "" {
			o = outPath(src, fmt.Sprintf(".rot%d", *deg), ext, outdir)
		}
		if err := saveImage(o, rotated, ext); err != nil {
			return "", err
		}
		fmt.Printf("  %s: %dx%d → %dx%d（旋转 %d°）\n", filepath.Base(src),
			img.Bounds().Dx(), img.Bounds().Dy(), rotated.Bounds().Dx(), rotated.Bounds().Dy(), *deg)
		return o, nil
	})
}

// rotateExact 90/180/270 度精确旋转。
func rotateExact(img image.Image, deg int) image.Image {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	var dw, dh int
	switch deg {
	case 90, 270:
		dw, dh = sh, sw
	default:
		dw, dh = sw, sh
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var sx, sy int
			switch deg {
			case 90: // 顺时针 90°
				sx, sy = y, sh-1-x
			case 180:
				sx, sy = sw-1-x, sh-1-y
			case 270: // 逆时针 90°
				sx, sy = sw-1-y, x
			}
			dst.Set(x, y, img.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	return dst
}

// rotateAny 任意角度旋转：画布扩大，中心旋转，背景填充。
func rotateAny(img image.Image, deg float64, bgColor color2, hasBG bool) image.Image {
	b := img.Bounds()
	sw, sh := float64(b.Dx()), float64(b.Dy())
	rad := deg * math.Pi / 180
	sin, cos := math.Sin(rad), math.Cos(rad)
	dw := int(math.Ceil(math.Abs(sw*cos) + math.Abs(sh*sin)))
	dh := int(math.Ceil(math.Abs(sw*sin) + math.Abs(sh*cos)))
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	if hasBG {
		r, g, bb := bgColor.rgba()
		fill := image.NewUniform(color.NRGBA{R: r, G: g, B: bb, A: 255})
		draw.Draw(dst, dst.Bounds(), fill, image.Point{}, draw.Src)
	}
	// 逆变换：目的坐标 → 源坐标
	cx, cy := float64(dw)/2, float64(dh)/2
	scx, scy := sw/2, sh/2
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			dx := float64(x) + 0.5 - cx
			dy := float64(y) + 0.5 - cy
			// 旋转 -deg
			sx := dx*cos + dy*sin + scx
			sy := -dx*sin + dy*cos + scy
			if sx < 0 || sy < 0 || sx >= sw || sy >= sh {
				continue
			}
			dst.Set(x, y, img.At(b.Min.X+int(sx), b.Min.Y+int(sy)))
		}
	}
	return dst
}

func runFlip(args []string) int {
	fs := flag.NewFlagSet("flip", flag.ContinueOnError)
	args = reorderArgs(args, flipBools, flipFlags)
	horiz := fs.Bool("h", false, "水平翻转（左右镜像）")
	vert := fs.Bool("v", false, "垂直翻转（上下镜像）")
	var outdir, out string
	commonFlags(fs, &outdir, &out)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if !*horiz && !*vert {
		fmt.Fprintln(os.Stderr, "错误: 需要 -h（水平翻转）或 -v（垂直翻转）")
		return 2
	}
	inputs := collect(fs.Args())
	if len(inputs) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 未指定输入图片")
		return 2
	}
	if code := checkOutFlags(outdir, out, len(inputs)); code != 0 {
		return code
	}
	suffix := ".flip"
	if *horiz && *vert {
		suffix = ".fliphv"
	} else if *horiz {
		suffix = ".fliph"
	} else if *vert {
		suffix = ".flipv"
	}
	return batchLoop(inputs, func(src string) (string, error) {
		img, format, err := loadImage(src)
		if err != nil {
			return "", fmt.Errorf("解码失败 %v", err)
		}
		ext := resolveOutExt(format)
		b := img.Bounds()
		sw, sh := b.Dx(), b.Dy()
		dst := image.NewRGBA(image.Rect(0, 0, sw, sh))
		for y := 0; y < sh; y++ {
			for x := 0; x < sw; x++ {
				sx, sy := x, y
				if *horiz {
					sx = sw - 1 - x
				}
				if *vert {
					sy = sh - 1 - y
				}
				dst.Set(x, y, img.At(b.Min.X+sx, b.Min.Y+sy))
			}
		}
		o := out
		if o == "" {
			o = outPath(src, suffix, ext, outdir)
		}
		if err := saveImage(o, dst, ext); err != nil {
			return "", err
		}
		return o, nil
	})
}

func runConcat(args []string) int {
	fs := flag.NewFlagSet("concat", flag.ContinueOnError)
	args = reorderArgs(args, nil, concatFlags)
	mode := fs.String("mode", "h", "拼接方向 h 横向 / v 纵向")
	gap := fs.Int("gap", 0, "图片间距像素")
	colorStr := fs.String("color", "#FFFFFF", "间距填充色 #RRGGBB 或 none（透明）")
	var outdir, out string
	commonFlags(fs, &outdir, &out)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *mode != "h" && *mode != "v" {
		fmt.Fprintln(os.Stderr, "错误: -mode 只能是 h（横向）或 v（纵向）")
		return 2
	}
	if *gap < 0 {
		fmt.Fprintln(os.Stderr, "错误: -gap 不能为负")
		return 2
	}
	inputs := collect(fs.Args())
	if len(inputs) < 2 {
		fmt.Fprintln(os.Stderr, "错误: 拼接至少需要 2 张图片")
		return 2
	}
	transparent := false
	var fillColor color2
	if strings.EqualFold(*colorStr, "none") {
		transparent = true
	} else {
		c, err := parseHexColor(*colorStr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "错误:", err)
			return 2
		}
		fillColor = c
	}
	outExt := ".png"
	if out != "" {
		if e := normalizeExt(filepath.Ext(out)); e != "" {
			outExt = e
		}
	}
	if out == "" {
		out = "concat" + outExt
	}

	type loaded struct {
		img image.Image
		w   int
		h   int
	}
	var imgs []loaded
	for _, src := range inputs {
		img, _, err := loadImage(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取 %s 失败: %v\n", src, err)
			return 1
		}
		b := img.Bounds()
		imgs = append(imgs, loaded{img, b.Dx(), b.Dy()})
	}

	var tw, th int
	if *mode == "h" {
		th = 0
		for _, m := range imgs {
			tw += m.w
			if m.h > th {
				th = m.h
			}
		}
		tw += *gap * (len(imgs) - 1)
	} else {
		tw = 0
		for _, m := range imgs {
			th += m.h
			if m.w > tw {
				tw = m.w
			}
		}
		th += *gap * (len(imgs) - 1)
	}

	canvas := image.NewNRGBA(image.Rect(0, 0, tw, th))
	if !transparent {
		r, g, b := fillColor.rgba()
		draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.NRGBA{R: r, G: g, B: b, A: 255}), image.Point{}, draw.Src)
	}
	offset := 0
	for _, m := range imgs {
		if *mode == "h" {
			draw.Draw(canvas, image.Rect(offset, 0, offset+m.w, m.h), m.img, m.img.Bounds().Min, draw.Src)
			offset += m.w + *gap
		} else {
			draw.Draw(canvas, image.Rect(0, offset, m.w, offset+m.h), m.img, m.img.Bounds().Min, draw.Src)
			offset += m.h + *gap
		}
	}
	if outdir != "" {
		out = filepath.Join(outdir, filepath.Base(out))
	}
	if err := saveImage(out, canvas, outExt); err != nil {
		fmt.Fprintln(os.Stderr, "写入失败:", err)
		return 1
	}
	fmt.Printf("已拼接 %d 张图 → %s（%dx%d）\n", len(imgs), out, tw, th)
	return 0
}

func runGrid(args []string) int {
	fs := flag.NewFlagSet("grid", flag.ContinueOnError)
	args = reorderArgs(args, nil, gridFlags)
	m := fs.Int("m", 3, "每边切分数：3 九宫格 / 4 十六宫格")
	outdir := fs.String("outdir", "", "输出目录（默认与原文件同目录）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *m != 3 && *m != 4 {
		fmt.Fprintln(os.Stderr, "错误: -m 只支持 3（九宫格）或 4（十六宫格）")
		return 2
	}
	inputs := collect(fs.Args())
	if len(inputs) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 未指定输入图片")
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
		img, format, err := loadImage(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "处理 %s 失败: 解码失败 %v\n", src, err)
			failed++
			continue
		}
		ext := resolveOutExt(format)
		b := img.Bounds()
		sw, sh := b.Dx(), b.Dy()
		cw, ch := sw/(*m), sh/(*m)
		if cw < 1 || ch < 1 {
			fmt.Fprintf(os.Stderr, "处理 %s 失败: 图片太小无法切 %d 宫格\n", src, *m**m)
			failed++
			continue
		}
		stem := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
		idx := 0
		for r := 0; r < *m; r++ {
			for c := 0; c < *m; c++ {
				idx++
				rect := image.Rect(b.Min.X+c*cw, b.Min.Y+r*ch, b.Min.X+(c+1)*cw, b.Min.Y+(r+1)*ch)
				cell := image.NewRGBA(image.Rect(0, 0, cw, ch))
				draw.Draw(cell, cell.Bounds(), img, rect.Min, draw.Src)
				name := fmt.Sprintf("%s.grid.%02d%s", stem, idx, ext)
				dst := name
				if *outdir != "" {
					dst = filepath.Join(*outdir, name)
				} else {
					dst = filepath.Join(filepath.Dir(src), name)
				}
				if err := saveImage(dst, cell, ext); err != nil {
					fmt.Fprintf(os.Stderr, "写入 %s 失败: %v\n", dst, err)
					failed++
				}
			}
		}
		fmt.Printf("%s: 切为 %d 宫格（每块 %dx%d）\n", src, *m**m, cw, ch)
	}
	if failed > 0 {
		return 1
	}
	return 0
}
