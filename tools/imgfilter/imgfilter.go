// Package imgfilter 实现图片滤镜套件命令，多滤镜可按序组合。
// 对应网页版：image/filter-tool.html（图片滤镜）、image/mosaic-tool.html（马赛克）、
// image/sketch-tool.html（素描）、image/duotone-tool.html（双色调）、image/pixelate-tool.html（像素画）、
// image/sharpen-tool.html（锐化）、image/dither-tool.html（抖动二值化）、image/histogram-tool.html（直方图）
//
// 用法：
//
//	lyntoolbox imgfilter 图片... -f gray [-f ...组合] [-bs 8] [-c1 #000000 -c2 #FFFFFF] [-amount 1.0] [-threshold 128] [-outdir 目录]
package imgfilter

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
	"golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const (
	Name  = "imgfilter"
	Desc  = "图片滤镜套件：灰度/马赛克/素描/双色调/像素画/锐化/抖动二值化/反色/直方图，可组合"
	Usage = `用法:
  lyntoolbox imgfilter 图片... -f 滤镜[,滤镜2,...] [-bs 8] [-c1 #000000] [-c2 #FFFFFF] [-amount 1.0] [-threshold 128] [-outdir 目录] [-o 输出]
  lyntoolbox imgfilter -dir 目录 [同上参数]

滤镜（-f 逗号分隔，按序应用）:
  gray       灰度
  mosaic     马赛克（块大小 -bs，默认 8）
  pixelate   像素画（块大小 -bs）
  sketch     素描（简化算法：灰度→反色→高斯近似模糊→颜色减淡）
  duotone    双色调映射（-c1 暗部颜色，-c2 亮部颜色）
  sharpen    锐化（3x3 卷积，强度 -amount）
  dither     Floyd-Steinberg 抖动二值化（初始阈值 -threshold）
  invert     反色
  histogram  直方图：打印 RGB 通道分布统计，不修改图像

参数:
  -bs        mosaic/pixelate 块大小（默认 8）
  -c1/-c2    duotone 颜色（默认 #1A1A2E / #E94560）
  -amount    sharpen 强度（默认 1.0）
  -threshold dither 初始阈值 0-255（默认 128）
  -dir       输入目录（批量）
  -o         单文件输出路径（仅单个输入时可用）
  -outdir    批量输出目录（默认输出到原目录，文件名加 .flt 后缀）`
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

func parseHexColor(s string) (color.RGBA, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return color.RGBA{}, fmt.Errorf("无法解析颜色 %q（应为 #RRGGBB）", s)
	}
	v, err := parseHexUint(s)
	if err != nil {
		return color.RGBA{}, err
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, nil
}

func parseHexUint(s string) (uint32, error) {
	var v uint32
	for i := 0; i < len(s); i++ {
		c := s[i]
		var d uint32
		switch {
		case c >= '0' && c <= '9':
			d = uint32(c - '0')
		case c >= 'a' && c <= 'f':
			d = uint32(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = uint32(c-'A') + 10
		default:
			return 0, fmt.Errorf("非法十六进制字符 %q", c)
		}
		v = v*16 + d
	}
	return v, nil
}

// toRGBA 转为可写的 RGBA 工作副本。
func toRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}

func clamp(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// lum 计算亮度（0-255）。
func lum(r, g, b uint8) int {
	return int(0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b))
}

// ---- 各滤镜实现 ----

func filterGray(img *image.RGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := img.PixOffset(x, y)
			l := clamp(lum(img.Pix[i], img.Pix[i+1], img.Pix[i+2]))
			img.Pix[i], img.Pix[i+1], img.Pix[i+2] = l, l, l
		}
	}
}

func filterInvert(img *image.RGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := img.PixOffset(x, y)
			img.Pix[i] = 255 - img.Pix[i]
			img.Pix[i+1] = 255 - img.Pix[i+1]
			img.Pix[i+2] = 255 - img.Pix[i+2]
		}
	}
}

// blockFilter 马赛克（均值）与像素画（取中心像素）共用骨架。
func blockFilter(img *image.RGBA, bs int, average bool) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if bs < 1 {
		bs = 1
	}
	for by := 0; by < h; by += bs {
		for bx := 0; bx < w; bx += bs {
			var rs, gs, bs_, n int
			var cr, cg, cb uint8
			for y := by; y < by+bs && y < h; y++ {
				for x := bx; x < bx+bs && x < w; x++ {
					i := img.PixOffset(b.Min.X+x, b.Min.Y+y)
					r, g, bl := img.Pix[i], img.Pix[i+1], img.Pix[i+2]
					if average {
						rs, gs, bs_ = rs+int(r), gs+int(g), bs_+int(bl)
						n++
					} else if x == bx+bs/2 && y == by+bs/2 {
						cr, cg, cb = r, g, bl
					}
				}
			}
			var fr, fg, fb uint8
			if average {
				fr, fg, fb = uint8(rs/n), uint8(gs/n), uint8(bs_/n)
			} else {
				fr, fg, fb = cr, cg, cb
			}
			for y := by; y < by+bs && y < h; y++ {
				for x := bx; x < bx+bs && x < w; x++ {
					i := img.PixOffset(b.Min.X+x, b.Min.Y+y)
					img.Pix[i], img.Pix[i+1], img.Pix[i+2] = fr, fg, fb
				}
			}
		}
	}
}

// gaussianBlur 3x3 高斯近似核（1 2 1 / 2 4 2 / 1 2 1），times 次叠加。
func gaussianBlur(img *image.RGBA, times int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	kernel := [9]int{1, 2, 1, 2, 4, 2, 1, 2, 1}
	for t := 0; t < times; t++ {
		src := image.NewRGBA(img.Bounds())
		copy(src.Pix, img.Pix)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				var acc [4]int
				ki := 0
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						sx, sy := x+dx, y+dy
						if sx < 0 {
							sx = 0
						}
						if sy < 0 {
							sy = 0
						}
						if sx >= w {
							sx = w - 1
						}
						if sy >= h {
							sy = h - 1
						}
						i := src.PixOffset(b.Min.X+sx, b.Min.Y+sy)
						acc[0] += int(src.Pix[i]) * kernel[ki]
						acc[1] += int(src.Pix[i+1]) * kernel[ki]
						acc[2] += int(src.Pix[i+2]) * kernel[ki]
						ki++
					}
				}
				o := img.PixOffset(b.Min.X+x, b.Min.Y+y)
				img.Pix[o] = clamp(acc[0] / 16)
				img.Pix[o+1] = clamp(acc[1] / 16)
				img.Pix[o+2] = clamp(acc[2] / 16)
			}
		}
	}
}

// filterSketch 简化素描：灰度 → 反色 → 高斯近似模糊 → 颜色减淡混合。
func filterSketch(img *image.RGBA) {
	filterGray(img)
	filterInvert(img)
	gaussianBlur(img, 2)
	// 现在图为“模糊的反色灰度”，与原灰度做颜色减淡: out = base * 255 / (255 - blend)
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := img.PixOffset(x, y)
			for c := 0; c < 3; c++ {
				out := 255
				if img.Pix[i+c] < 255 {
					out = int(clamp((int(img.Pix[i+c]) * 255) / (255 - int(img.Pix[i+c]))))
				}
				img.Pix[i+c] = clamp(out)
			}
		}
	}
}

func filterDuotone(img *image.RGBA, c1, c2 color.RGBA) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := img.PixOffset(x, y)
			t := float64(lum(img.Pix[i], img.Pix[i+1], img.Pix[i+2])) / 255
			img.Pix[i] = clamp(int(float64(c1.R) + t*float64(int(c2.R)-int(c1.R))))
			img.Pix[i+1] = clamp(int(float64(c1.G) + t*float64(int(c2.G)-int(c1.G))))
			img.Pix[i+2] = clamp(int(float64(c1.B) + t*float64(int(c2.B)-int(c1.B))))
		}
	}
}

// filterSharpen 3x3 锐化卷积，amount 控制强度。
func filterSharpen(img *image.RGBA, amount float64) {
	if amount <= 0 {
		amount = 1
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	src := image.NewRGBA(img.Bounds())
	copy(src.Pix, img.Pix)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			oi := src.PixOffset(b.Min.X+x, b.Min.Y+y)
			o := img.PixOffset(b.Min.X+x, b.Min.Y+y)
			for c := 0; c < 3; c++ {
				// 卷积核 [0,-1,0; -1,5,-1; 0,-1,0]
				var acc int
				neigh := func(dx, dy int) int {
					sx, sy := x+dx, y+dy
					if sx < 0 {
						sx = 0
					}
					if sy < 0 {
						sy = 0
					}
					if sx >= w {
						sx = w - 1
					}
					if sy >= h {
						sy = h - 1
					}
					i := src.PixOffset(b.Min.X+sx, b.Min.Y+sy)
					return int(src.Pix[i+c])
				}
				conv := 5*neigh(0, 0) - neigh(-1, 0) - neigh(1, 0) - neigh(0, -1) - neigh(0, 1)
				acc = int(src.Pix[oi+c]) + int(math.Round(amount*float64(conv-int(src.Pix[oi+c]))))
				img.Pix[o+c] = clamp(acc)
			}
		}
	}
}

// filterDither Floyd-Steinberg 抖动二值化。
func filterDither(img *image.RGBA, threshold int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if threshold < 0 {
		threshold = 0
	}
	if threshold > 255 {
		threshold = 255
	}
	// 灰度矩阵
	gray := make([]int, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			i := img.PixOffset(b.Min.X+x, b.Min.Y+y)
			gray[y*w+x] = lum(img.Pix[i], img.Pix[i+1], img.Pix[i+2])
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			old := gray[y*w+x]
			nv := 0
			if old >= threshold {
				nv = 255
			}
			err := old - nv
			if x+1 < w {
				gray[y*w+x+1] += err * 7 / 16
			}
			if y+1 < h {
				if x > 0 {
					gray[(y+1)*w+x-1] += err * 3 / 16
				}
				gray[(y+1)*w+x] += err * 5 / 16
				if x+1 < w {
					gray[(y+1)*w+x+1] += err * 1 / 16
				}
			}
			v := uint8(nv)
			i := img.PixOffset(b.Min.X+x, b.Min.Y+y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2] = v, v, v
		}
	}
}

// printHistogram 输出 RGB 通道 0-255 分布统计与 ASCII 柱状图。
func printHistogram(img *image.RGBA, label string) {
	const bins = 16 // 每桶 16 级
	var hist [3][bins]int
	var sum, cnt int64
	var means [3]float64
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			i := img.PixOffset(x, y)
			for c := 0; c < 3; c++ {
				hist[c][img.Pix[i+c]/bins]++
				means[c] += float64(img.Pix[i+c])
			}
			sum += int64(lum(img.Pix[i], img.Pix[i+1], img.Pix[i+2]))
			cnt++
		}
	}
	names := [3]string{"R", "G", "B"}
	var max int
	for c := 0; c < 3; c++ {
		for _, v := range hist[c] {
			if v > max {
				max = v
			}
		}
	}
	fmt.Printf("=== %s 直方图（%d 像素，平均亮度 %.1f，均值 R %.0f / G %.0f / B %.0f）===\n",
		label, cnt, float64(sum)/float64(cnt), means[0]/float64(cnt), means[1]/float64(cnt), means[2]/float64(cnt))
	for c := 0; c < 3; c++ {
		for bi := 0; bi < bins; bi++ {
			barLen := 0
			if max > 0 {
				barLen = hist[c][bi] * 40 / max
			}
			fmt.Printf("  %s %3d-%3d |%-40s %d\n", names[c], bi*bins, bi*bins+bins-1,
				strings.Repeat("#", barLen), hist[c][bi])
		}
	}
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
	knownFlags = map[string]bool{"f": true, "bs": true, "c1": true, "c2": true, "amount": true, "threshold": true, "dir": true, "o": true, "outdir": true}
	boolFlags  = map[string]bool{}
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	filters := fs.String("f", "", "滤镜列表，逗号分隔（gray,mosaic,sketch,duotone,pixelate,sharpen,dither,invert,histogram）")
	bs := fs.Int("bs", 8, "马赛克/像素画块大小")
	c1s := fs.String("c1", "#1A1A2E", "duotone 暗部颜色")
	c2s := fs.String("c2", "#E94560", "duotone 亮部颜色")
	amount := fs.Float64("amount", 1.0, "锐化强度")
	threshold := fs.Int("threshold", 128, "抖动初始阈值 0-255")
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
	if strings.TrimSpace(*filters) == "" {
		fmt.Fprintln(os.Stderr, "错误: 需要 -f 指定滤镜")
		return 2
	}
	if *bs < 1 {
		fmt.Fprintln(os.Stderr, "错误: -bs 需 ≥ 1")
		return 2
	}
	if *threshold < 0 || *threshold > 255 {
		fmt.Fprintln(os.Stderr, "错误: -threshold 需在 0-255 之间")
		return 2
	}
	c1, err1 := parseHexColor(*c1s)
	c2, err2 := parseHexColor(*c2s)
	if err1 != nil {
		fmt.Fprintln(os.Stderr, "错误:", err1)
		return 2
	}
	if err2 != nil {
		fmt.Fprintln(os.Stderr, "错误:", err2)
		return 2
	}

	// 解析滤镜链
	type step struct {
		name string
	}
	var chain []string
	for _, f := range strings.Split(*filters, ",") {
		f = strings.ToLower(strings.TrimSpace(f))
		if f == "" {
			continue
		}
		switch f {
		case "gray", "mosaic", "pixelate", "sketch", "duotone", "sharpen", "dither", "invert", "histogram":
			chain = append(chain, f)
		default:
			fmt.Fprintf(os.Stderr, "错误: 未知滤镜 %q（可选 gray,mosaic,pixelate,sketch,duotone,sharpen,dither,invert,histogram）\n", f)
			return 2
		}
	}
	if len(chain) == 0 {
		fmt.Fprintln(os.Stderr, "错误: -f 未包含有效滤镜")
		return 2
	}
	needOutput := false
	for _, f := range chain {
		if f != "histogram" {
			needOutput = true
		}
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
		work := toRGBA(img)
		for _, f := range chain {
			switch f {
			case "gray":
				filterGray(work)
			case "invert":
				filterInvert(work)
			case "mosaic":
				blockFilter(work, *bs, true)
			case "pixelate":
				blockFilter(work, *bs, false)
			case "sketch":
				filterSketch(work)
			case "duotone":
				filterDuotone(work, c1, c2)
			case "sharpen":
				filterSharpen(work, *amount)
			case "dither":
				filterDither(work, *threshold)
			case "histogram":
				printHistogram(work, src)
			}
		}
		if !needOutput {
			continue
		}
		ext := normalizeExt(format)
		if ext == "" {
			ext = ".png"
		}
		dst := *out
		if dst == "" {
			dst = outPath(src, ".flt", ext, *outdir)
		}
		fh, err := os.Create(dst)
		if err != nil {
			fmt.Fprintf(os.Stderr, "处理 %s 失败: %v\n", src, err)
			failed++
			continue
		}
		if err := encodeImage(fh, work, ext); err != nil {
			fh.Close()
			os.Remove(dst)
			fmt.Fprintf(os.Stderr, "处理 %s 失败: %v\n", src, err)
			failed++
			continue
		}
		fh.Close()
		fmt.Printf("%s: 已应用 [%s] → %s\n", src, strings.Join(chain, ","), dst)
	}
	if failed > 0 {
		return 1
	}
	return 0
}
