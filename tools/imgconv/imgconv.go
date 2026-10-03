// Package imgconv 实现图片压缩/缩放/格式转换命令。
// 对应网页版：image/compress-tool.html（图片压缩）、image/format-convert-tool.html（图片格式转换）、image/resize-tool.html（图片缩放）
//
// 用法：
//
//	lyntoolbox imgconv 图片... [-to png] [-w 800] [-q 85] [-outdir 目录]
//	lyntoolbox imgconv -dir 目录 [-scale 50] [-to jpg]
package imgconv

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/bmp"
	"golang.org/x/image/draw"
	"golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const (
	Name  = "imgconv"
	Desc  = "图片压缩/缩放/格式批量转换，支持 png/jpg/gif/bmp/tiff/webp 读取"
	Usage = `用法:
  lyntoolbox imgconv 图片... [-to png|jpg|gif|bmp|tiff] [-w N] [-h N] [-scale N] [-q N] [-o 输出 | -outdir 目录]
  lyntoolbox imgconv -dir 目录 [同上参数]

参数:
  -to       目标格式 png|jpg|gif|bmp|tiff（省略保持原格式；webp 仅支持解码不支持输出）
  -w/-h     目标宽/高，只给一边则等比缩放（与 -scale 同给时 -w/-h 优先）
  -scale    等比缩放百分比，如 50 表示缩小一半
  -q        JPEG 质量 1-100（默认 85）
  -dir      输入目录，批量处理目录内全部图片
  -o        单文件输出路径（仅单个输入时可用）
  -outdir   批量输出目录（默认输出到原目录，文件名加 .conv 后缀）`
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

// encodeImage 按扩展名/格式名编码图片，ext 形如 ".png" 或 "png"。
func encodeImage(w draw2writer, img image.Image, ext string, quality int) error {
	switch strings.TrimPrefix(strings.ToLower(ext), ".") {
	case "png":
		return png.Encode(w, img)
	case "jpg", "jpeg":
		return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
	case "gif":
		return gif.Encode(w, img, nil)
	case "bmp":
		return bmp.Encode(w, img)
	case "tif", "tiff":
		return tiff.Encode(w, img, nil)
	case "webp":
		return errors.New("暂不支持编码 WebP 输出（webp 仅支持解码），请改用 png/jpg/gif/bmp/tiff")
	}
	return fmt.Errorf("不支持的输出格式: %s", ext)
}

type draw2writer interface {
	Write(p []byte) (int, error)
}

// normalizeExt 归一化输出扩展名（jpg/jpeg 统一为 .jpg）。
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

// outPath 生成输出路径：原名加后缀（或 -outdir 下同名）。
func outPath(src, suffix, ext, outdir string) string {
	base := filepath.Base(src)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	name := stem + suffix + ext
	if outdir != "" {
		return filepath.Join(outdir, name)
	}
	return filepath.Join(filepath.Dir(src), name)
}

func scaleImage(src image.Image, w, h int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst
}

func humanSize(n int64) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.2fMB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	}
	return fmt.Sprintf("%dB", n)
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
	knownFlags = map[string]bool{"to": true, "w": true, "h": true, "scale": true, "q": true, "dir": true, "o": true, "outdir": true}
	boolFlags  = map[string]bool{}
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	to := fs.String("to", "", "目标格式 png|jpg|gif|bmp|tiff")
	w := fs.Int("w", 0, "目标宽度")
	h := fs.Int("h", 0, "目标高度")
	scale := fs.Int("scale", 100, "等比缩放百分比")
	quality := fs.Int("q", 85, "JPEG 质量 1-100")
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
	if *quality < 1 || *quality > 100 {
		fmt.Fprintln(os.Stderr, "错误: -q 需在 1-100 之间")
		return 2
	}
	if *scale <= 0 {
		fmt.Fprintln(os.Stderr, "错误: -scale 需为正数")
		return 2
	}
	if *w < 0 || *h < 0 {
		fmt.Fprintln(os.Stderr, "错误: -w/-h 不能为负")
		return 2
	}
	if *to != "" && normalizeExt(*to) == "" {
		if strings.ToLower(*to) == "webp" {
			fmt.Fprintln(os.Stderr, "错误: 暂不支持编码 WebP 输出（webp 仅支持解码），请改用 png/jpg/gif/bmp/tiff")
		} else {
			fmt.Fprintf(os.Stderr, "错误: 不支持的目标格式 %s（可选 png|jpg|gif|bmp|tiff）\n", *to)
		}
		return 2
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
		info, err := os.Stat(a)
		if err != nil {
			fmt.Fprintln(os.Stderr, "无法访问", a+":", err)
			return 1
		}
		if info.IsDir() {
			_ = filepath.Walk(a, func(p string, fi os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if fi.Mode().IsRegular() && isImageFile(p) {
					inputs = append(inputs, p)
				}
				return nil
			})
		} else if isImageFile(a) {
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
		if err := convert(src, *to, *w, *h, *scale, *quality, *out, *outdir); err != nil {
			fmt.Fprintf(os.Stderr, "处理 %s 失败: %v\n", src, err)
			failed++
		}
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "共 %d 个文件处理失败\n", failed)
		return 1
	}
	return 0
}

func convert(src, to string, w, h, scale, quality int, out, outdir string) error {
	img, format, err := loadImage(src)
	if err != nil {
		return fmt.Errorf("解码失败 %v", err)
	}
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	beforeSize := st.Size()

	// 计算目标尺寸
	sw, sh := img.Bounds().Dx(), img.Bounds().Dy()
	tw, th := sw, sh
	if w > 0 && h > 0 {
		tw, th = w, h
	} else if w > 0 {
		tw = w
		th = int(math.Round(float64(sh) * float64(w) / float64(sw)))
	} else if h > 0 {
		th = h
		tw = int(math.Round(float64(sw) * float64(h) / float64(sh)))
	} else if scale != 100 {
		tw = int(math.Round(float64(sw) * float64(scale) / 100))
		th = int(math.Round(float64(sh) * float64(scale) / 100))
	}
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}

	// 确定输出格式
	outExt := ""
	if to != "" {
		outExt = normalizeExt(to)
	} else if out != "" {
		outExt = normalizeExt(filepath.Ext(out))
	}
	if outExt == "" {
		outExt = normalizeExt(format)
	}
	if outExt == "" { // webp 等无法编码的格式回退为 png
		outExt = ".png"
	}

	if tw != sw || th != sh {
		img = scaleImage(img, tw, th)
	}

	dst := out
	if dst == "" {
		dst = outPath(src, ".conv", outExt, outdir)
	}

	var size int64
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	wt := bufio.NewWriter(f)
	if err := encodeImage(wt, img, outExt, quality); err != nil {
		f.Close()
		os.Remove(dst)
		return err
	}
	if err := wt.Flush(); err != nil {
		f.Close()
		return err
	}
	if info, err := f.Stat(); err == nil {
		size = info.Size()
	}
	f.Close()

	change := ""
	if beforeSize > 0 {
		pct := float64(size-beforeSize) / float64(beforeSize) * 100
		change = fmt.Sprintf("（体积 %+.1f%%）", pct)
	}
	fmt.Printf("%s: %dx%d %s → %dx%d %s  输出 %s %s\n",
		src, sw, sh, humanSize(beforeSize), tw, th, humanSize(size), dst, change)
	return nil
}
