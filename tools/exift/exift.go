// Package exift 实现图片 EXIF 查看与清除命令。
// 对应网页版：image/exif-view-tool.html（EXIF 查看）、image/exif-clean-tool.html（EXIF 清除）
//
// 用法：
//
//	lyntoolbox exift view 图片... [-dir 目录]
//	lyntoolbox exift strip 图片... [-to png|jpg|gif|bmp|tiff] [-q N] [-outdir 目录]
package exift

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/rwcarlsen/goexif/exif"
	gotiff "github.com/rwcarlsen/goexif/tiff"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const (
	Name  = "exift"
	Desc  = "图片 EXIF 查看与清除（重编码去元数据），支持批量"
	Usage = `用法:
  lyntoolbox exift view 图片... [-dir 目录]           查看 EXIF 信息
  lyntoolbox exift strip 图片... [-to 格式] [-q N] [-outdir 目录] [-dir 目录]
                                                      重编码清除全部元数据

strip 参数:
  -to       输出格式 png|jpg|gif|bmp|tiff（默认保持原格式；webp 源输出 png）
  -q        JPEG 质量 1-100（默认 92）
  -outdir   批量输出目录（默认输出到原目录，文件名加 .clean 后缀）`
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

func encodeImage(w interface{ Write(p []byte) (int, error) }, img image.Image, ext string, quality int) error {
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

func humanSize(n int64) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.2fMB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	}
	return fmt.Sprintf("%dB", n)
}

// collectInputs 收集输入文件（位置参数 + -dir 目录批量）。
func collectInputs(args []string, dir string) ([]string, int) {
	var list []string
	if dir != "" {
		err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() && isImageFile(p) {
				list = append(list, p)
			}
			return nil
		})
		if err != nil {
			return nil, -1
		}
	}
	for _, a := range args {
		if isImageFile(a) {
			list = append(list, a)
		}
	}
	return list, 0
}

func orientationDesc(o int) string {
	switch o {
	case 1:
		return "正常"
	case 2:
		return "水平翻转"
	case 3:
		return "旋转 180°"
	case 4:
		return "垂直翻转"
	case 5:
		return "转置"
	case 6:
		return "顺时针旋转 90°"
	case 7:
		return "转置反转"
	case 8:
		return "逆时针旋转 90°"
	}
	return "未知"
}

// Rat 辅助：取标签第 i 个有理数。
func tagRat(t *gotiff.Tag, i int) *big.Rat {
	r, err := t.Rat(i)
	if err != nil {
		return nil
	}
	return r
}

func printExif(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	x, err := exif.Decode(f)
	if err != nil {
		// 无 EXIF 数据（含 EOF/未找到 APP1 段等情况）不算失败，明确提示
		fmt.Printf("=== %s ===\n  无 EXIF 信息\n", path)
		return nil
	}

	get := func(name exif.FieldName) *gotiff.Tag {
		t, err := x.Get(name)
		if err != nil {
			return nil
		}
		return t
	}
	str := func(name exif.FieldName) string {
		if t := get(name); t != nil {
			if s, err := t.StringVal(); err == nil {
				return strings.Trim(s, "\x00 ")
			}
		}
		return ""
	}

	fmt.Printf("=== %s ===\n", path)
	if v := str(exif.Make); v != "" {
		fmt.Printf("  相机厂商: %s\n", v)
	}
	if v := str(exif.Model); v != "" {
		fmt.Printf("  相机型号: %s\n", v)
	}
	if v := str(exif.Software); v != "" {
		fmt.Printf("  软件: %s\n", v)
	}
	if v := str(exif.DateTime); v != "" {
		fmt.Printf("  拍摄时间: %s\n", v)
	}
	if t := get(exif.Orientation); t != nil {
		if o, err := t.Int(0); err == nil {
			fmt.Printf("  方向: %d（%s）\n", o, orientationDesc(o))
		}
	}
	if t := get(exif.ExposureTime); t != nil {
		if r := tagRat(t, 0); r != nil {
			fmt.Printf("  曝光时间: %s s\n", r.RatString())
		}
	}
	if t := get(exif.FNumber); t != nil {
		if r := tagRat(t, 0); r != nil {
			fv, _ := r.Float64()
			fmt.Printf("  光圈: f/%.1f\n", fv)
		}
	}
	if t := get(exif.ISOSpeedRatings); t != nil {
		if iso, err := t.Int(0); err == nil {
			fmt.Printf("  ISO: %d\n", iso)
		}
	}
	if t := get(exif.FocalLength); t != nil {
		if r := tagRat(t, 0); r != nil {
			fv, _ := r.Float64()
			if fv == float64(int64(fv)) {
				fmt.Printf("  焦距: %dmm\n", int64(fv))
			} else {
				fmt.Printf("  焦距: %.1fmm\n", fv)
			}
		}
	}
	// GPS
	latT, lonT := get(exif.GPSLatitude), get(exif.GPSLongitude)
	latRefT, lonRefT := get(exif.GPSLatitudeRef), get(exif.GPSLongitudeRef)
	if latT != nil && lonT != nil {
		dms := func(t *gotiff.Tag) float64 {
			var v float64
			mul := []float64{1, 1.0 / 60, 1.0 / 3600}
			for i := 0; i < 3; i++ {
				if r := tagRat(t, i); r != nil {
					f, _ := r.Float64()
					v += f * mul[i]
				}
			}
			return v
		}
		lat, lon := dms(latT), dms(lonT)
		latRef, _ := latRefT.StringVal()
		lonRef, _ := lonRefT.StringVal()
		latRef = strings.Trim(strings.ToUpper(latRef), "\x00 ")
		lonRef = strings.Trim(strings.ToUpper(lonRef), "\x00 ")
		if strings.HasPrefix(latRef, "S") {
			lat = -lat
		}
		if strings.HasPrefix(lonRef, "W") {
			lon = -lon
		}
		fmt.Printf("  GPS: %.6f°%s, %.6f°%s\n", math.Abs(lat), ns(lat), math.Abs(lon), ew(lon))
	}
	return nil
}

func ns(lat float64) string {
	if lat < 0 {
		return "S"
	}
	return "N"
}
func ew(lon float64) string {
	if lon < 0 {
		return "W"
	}
	return "E"
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
	knownFlags = map[string]bool{"dir": true, "to": true, "q": true, "outdir": true}
	boolFlags  = map[string]bool{}
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, "错误: 缺少子命令 view 或 strip\n\n")
		fmt.Fprint(os.Stderr, Usage+"\n")
		return 2
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "view":
		return runView(rest)
	case "strip":
		return runStrip(rest)
	default:
		fmt.Fprintf(os.Stderr, "错误: 未知子命令 %q（可用: view, strip）\n\n", sub)
		fmt.Fprint(os.Stderr, Usage+"\n")
		return 2
	}
}

func runView(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	dir := fs.String("dir", "", "输入目录（批量）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	args = reorderArgs(args, boolFlags, knownFlags)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	inputs, code := collectInputs(fs.Args(), *dir)
	if code != 0 {
		fmt.Fprintln(os.Stderr, "遍历目录失败")
		return 1
	}
	if len(inputs) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 未指定输入图片（可传文件参数或用 -dir）")
		return 2
	}
	failed := 0
	for _, p := range inputs {
		if err := printExif(p); err != nil {
			fmt.Fprintf(os.Stderr, "读取 %s 失败: %v\n", p, err)
			failed++
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}

func runStrip(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	to := fs.String("to", "", "输出格式 png|jpg|gif|bmp|tiff")
	quality := fs.Int("q", 92, "JPEG 质量 1-100")
	outdir := fs.String("outdir", "", "批量输出目录")
	dir := fs.String("dir", "", "输入目录（批量）")
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
	if *to != "" && normalizeExt(*to) == "" {
		fmt.Fprintf(os.Stderr, "错误: 不支持的目标格式 %s（可选 png|jpg|gif|bmp|tiff）\n", *to)
		return 2
	}
	inputs, code := collectInputs(fs.Args(), *dir)
	if code != 0 {
		fmt.Fprintln(os.Stderr, "遍历目录失败")
		return 1
	}
	if len(inputs) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 未指定输入图片（可传文件参数或用 -dir）")
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
		st, err := os.Stat(src)
		if err != nil {
			failed++
			continue
		}
		outExt := ""
		if *to != "" {
			outExt = normalizeExt(*to)
		} else {
			outExt = normalizeExt(format)
		}
		if outExt == "" { // webp 等无法编码的格式回退为 png
			outExt = ".png"
		}
		dst := outPath(src, ".clean", outExt, *outdir)
		f, err := os.Create(dst)
		if err != nil {
			fmt.Fprintf(os.Stderr, "处理 %s 失败: %v\n", src, err)
			failed++
			continue
		}
		if err := encodeImage(f, img, outExt, *quality); err != nil {
			f.Close()
			os.Remove(dst)
			fmt.Fprintf(os.Stderr, "处理 %s 失败: %v\n", src, err)
			failed++
			continue
		}
		f.Close()
		after, _ := os.Stat(dst)
		beforeSize, afterSize := st.Size(), int64(0)
		if after != nil {
			afterSize = after.Size()
		}
		fmt.Printf("%s: %s → %s（EXIF 已清除）\n", src, humanSize(beforeSize), dst+" "+humanSize(afterSize))
	}
	if failed > 0 {
		return 1
	}
	return 0
}
