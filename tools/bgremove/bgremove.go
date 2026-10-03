// Package bgremove 实现背景移除/换底色命令（颜色容差算法）。
// 对应网页版：image/bg-remove-tool.html（抠图/背景移除）、image/idphoto-tool.html（证件照换底色，算法向）
//
// 用法：
//
//	lyntoolbox bgremove 图片... [-key #RRGGBB] [-tol 18] [-feather 1] [-bg #RRGGBB] [-outdir 目录]
//	lyntoolbox bgremove -dir 目录 [...同上]
package bgremove

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
	Name  = "bgremove"
	Desc  = "背景移除/换底色（四角取色 + 颜色容差 + 边缘羽化），支持批量"
	Usage = `用法:
  lyntoolbox bgremove 图片... [-key #RRGGBB] [-tol 18] [-feather 1] [-bg #RRGGBB] [-outdir 目录] [-o 输出]
  lyntoolbox bgremove -dir 目录 [同上参数]

参数:
  -key      背景色（省略则自动取样四角颜色中位数）
  -tol      颜色容差 0-100（默认 18，RGB 欧氏距离归一化；越大移除越多）
  -feather  边缘羽化宽度，单位与 -tol 相同（默认 1；0 为硬边）
  -bg       新背景色 #RRGGBB（换底色模式；省略则输出透明 PNG）
  -o        单文件输出路径（仅单个输入时可用）
  -outdir   批量输出目录（默认输出到原目录，文件名加 .nobg 后缀）`
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

// cornerMedian 取四角 3x3 区域均值，再逐通道取中位数作为背景色。
func cornerMedian(img image.Image) color.RGBA {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	corners := [4][2]int{{0, 0}, {w - 3, 0}, {0, h - 3}, {w - 3, h - 3}}
	var means [4][3]float64
	for ci, c := range corners {
		x0, y0 := c[0], c[1]
		if x0 < 0 {
			x0 = 0
		}
		if y0 < 0 {
			y0 = 0
		}
		if x0+3 > w {
			x0 = w - 3
		}
		if y0+3 > h {
			y0 = h - 3
		}
		var sum [3]int64
		n := int64(0)
		for y := y0; y < y0+3 && y < h; y++ {
			for x := x0; x < x0+3 && x < w; x++ {
				r, g, bb, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
				sum[0] += int64(r >> 8)
				sum[1] += int64(g >> 8)
				sum[2] += int64(bb >> 8)
				n++
			}
		}
		if n == 0 {
			n = 1
		}
		means[ci][0] = float64(sum[0]) / float64(n)
		means[ci][1] = float64(sum[1]) / float64(n)
		means[ci][2] = float64(sum[2]) / float64(n)
	}
	var med [3]uint8
	for ch := 0; ch < 3; ch++ {
		vals := []float64{means[0][ch], means[1][ch], means[2][ch], means[3][ch]}
		for i := 1; i < len(vals); i++ {
			for j := i; j > 0 && vals[j] < vals[j-1]; j-- {
				vals[j], vals[j-1] = vals[j-1], vals[j]
			}
		}
		med[ch] = uint8(math.Round((vals[1] + vals[2]) / 2))
	}
	return color.RGBA{R: med[0], G: med[1], B: med[2], A: 255}
}

// removeBackground 返回处理后的 NRGBA 图像与被移除像素比例。
func removeBackground(img image.Image, key color.RGBA, tol, feather float64, newBG *color.RGBA) (*image.NRGBA, float64) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	src := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(src, src.Bounds(), img, b.Min, draw.Src)

	kr := float64(key.R)
	kg := float64(key.G)
	kb := float64(key.B)
	norm := 255.0 * math.Sqrt(3)

	// 依据与背景色的归一化欧氏距离计算 alpha（0 完全移除 → 255 保留）
	alphas := make([]float64, w*h)
	removed := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := src.PixOffset(x, y)
			dr := float64(src.Pix[i]) - kr
			dg := float64(src.Pix[i+1]) - kg
			db := float64(src.Pix[i+2]) - kb
			dist := math.Sqrt(dr*dr+dg*dg+db*db) / norm * 100
			a := 255.0
			if dist <= tol {
				a = 0
			} else if feather > 0 && dist < tol+feather {
				a = (dist - tol) / feather * 255
			}
			if a < 0 {
				a = 0
			}
			if a > 255 {
				a = 255
			}
			alphas[y*w+x] = a
			if a == 0 {
				removed++
			}
		}
	}
	ratio := 0.0
	if w*h > 0 {
		ratio = float64(removed) / float64(w*h) * 100
	}

	// 换底色模式：与背景色合成（含羽化半透明）
	if newBG != nil {
		bgR := float64(newBG.R)
		bgG := float64(newBG.G)
		bgB := float64(newBG.B)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				a := alphas[y*w+x] / 255
				i := src.PixOffset(x, y)
				src.Pix[i] = uint8(math.Round(float64(src.Pix[i])*a + bgR*(1-a)))
				src.Pix[i+1] = uint8(math.Round(float64(src.Pix[i+1])*a + bgG*(1-a)))
				src.Pix[i+2] = uint8(math.Round(float64(src.Pix[i+2])*a + bgB*(1-a)))
				src.Pix[i+3] = 255
			}
		}
		return src, ratio
	}

	// 透明模式
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := src.PixOffset(x, y)
			src.Pix[i+3] = uint8(math.Round(alphas[y*w+x]))
		}
	}
	return src, ratio
}

func encodeOut(w interface{ Write(p []byte) (int, error) }, img image.Image, ext string) error {
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
	knownFlags = map[string]bool{"key": true, "tol": true, "feather": true, "bg": true, "dir": true, "o": true, "outdir": true}
	boolFlags  = map[string]bool{}
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	keyHex := fs.String("key", "", "背景色 #RRGGBB（省略则四角自动取样）")
	tol := fs.Float64("tol", 18, "颜色容差 0-100")
	feather := fs.Float64("feather", 1, "边缘羽化宽度（0 为硬边）")
	bgHex := fs.String("bg", "", "新背景色 #RRGGBB（换底色模式）")
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
	if *tol < 0 || *tol > 100 {
		fmt.Fprintln(os.Stderr, "错误: -tol 需在 0-100 之间")
		return 2
	}
	if *feather < 0 || *feather > 100 {
		fmt.Fprintln(os.Stderr, "错误: -feather 需在 0-100 之间")
		return 2
	}
	var key *color.RGBA
	if *keyHex != "" {
		c, err := parseHexColor(*keyHex)
		if err != nil {
			fmt.Fprintln(os.Stderr, "错误:", err)
			return 2
		}
		key = &c
	}
	var newBG *color.RGBA
	if *bgHex != "" {
		c, err := parseHexColor(*bgHex)
		if err != nil {
			fmt.Fprintln(os.Stderr, "错误:", err)
			return 2
		}
		newBG = &c
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
		img, format, err := loadImage(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "处理 %s 失败: 解码失败 %v\n", src, err)
			failed++
			continue
		}
		bgKey := key
		autoSampled := false
		if bgKey == nil {
			c := cornerMedian(img)
			bgKey = &c
			autoSampled = true
		}
		result, ratio := removeBackground(img, *bgKey, *tol, *feather, newBG)

		// 输出格式：换底色或非 png 模式可保留原格式；透明模式必须是 png
		ext := normalizeExt(format)
		if ext == "" || (newBG == nil && ext != ".png") {
			ext = ".png"
		}
		dst := *out
		if dst == "" {
			dst = outPath(src, ".nobg", ext, *outdir)
		}
		f, err := os.Create(dst)
		if err != nil {
			fmt.Fprintf(os.Stderr, "处理 %s 失败: %v\n", src, err)
			failed++
			continue
		}
		if err := encodeOut(f, result, ext); err != nil {
			f.Close()
			os.Remove(dst)
			fmt.Fprintf(os.Stderr, "处理 %s 失败: %v\n", src, err)
			failed++
			continue
		}
		f.Close()
		mode := "透明"
		if newBG != nil {
			mode = fmt.Sprintf("#%02X%02X%02X", newBG.R, newBG.G, newBG.B)
		}
		keyStr := "自动取样"
		if !autoSampled {
			keyStr = fmt.Sprintf("#%02X%02X%02X", bgKey.R, bgKey.G, bgKey.B)
		}
		fmt.Printf("%s: 背景 %s → %s，移除像素 %.1f%% → %s\n", src, keyStr, mode, ratio, dst)
	}
	if failed > 0 {
		return 1
	}
	return 0
}
