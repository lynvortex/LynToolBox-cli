// Package imgascii 实现图片转 ASCII 字符画命令。
// 对应网页版：image/img-ascii-tool.html（图片转ASCII）
//
// 用法：
//
//	lyntoolbox imgascii 图片 [-w 80] [-ramp default|simple|blocks] [-invert] [-o 输出.txt]
package imgascii

import (
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	Name  = "imgascii"
	Desc  = "图片转 ASCII 字符画，支持多种字符梯度与反相"
	Usage = `用法:
  lyntoolbox imgascii 图片... [-w 80] [-ramp default|simple|blocks] [-invert] [-o 输出.txt]

参数:
  -w       输出列数（默认 80；行数按字符宽高比 0.5 自动压缩）
  -ramp    字符梯度:
             default  " .:-=+*#%@"（深→浅，适配深色终端）
             simple   " .oO0@"
             blocks   " ░▒▓█"
  -invert  反相（亮色背景终端使用）
  -o       输出到文本文件（默认打印到终端）
  图片     支持多个文件；-o 仅对单个输入有效`
)

var imgExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".bmp": true, ".tif": true, ".tiff": true, ".webp": true,
}

func loadImage(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	return image.Decode(f)
}

var ramps = map[string]string{
	"default": " .:-=+*#%@",
	"simple":  " .oO0@",
	"blocks":  " ░▒▓█",
}

// asciiArt 将图片转换为 ASCII 字符画。
func asciiArt(img image.Image, cols int, ramp string, invert bool) string {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	rows := int(math.Round(float64(sh) / float64(sw) * float64(cols) * 0.5))
	if rows < 1 {
		rows = 1
	}
	if cols > sw {
		cols = sw
	}
	// 缩放到 cols x rows 的灰度图
	gray := image.NewGray(image.Rect(0, 0, cols, rows))
	draw.CatmullRom.Scale(gray, gray.Bounds(), img, img.Bounds(), draw.Src, nil)

	n := len(ramp)
	var sb strings.Builder
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			lum := int(gray.GrayAt(x, y).Y)
			if invert {
				lum = 255 - lum
			}
			idx := lum * (n - 1) / 255
			sb.WriteByte(ramp[idx])
		}
		sb.WriteByte('\n')
	}
	return sb.String()
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
	knownFlags = map[string]bool{"w": true, "ramp": true, "invert": true, "o": true, "ramp-chars": true}
	boolFlags  = map[string]bool{"invert": true}
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	cols := fs.Int("w", 80, "输出列数")
	rampName := fs.String("ramp", "default", "字符梯度 default|simple|blocks")
	invert := fs.Bool("invert", false, "反相")
	out := fs.String("o", "", "输出文本文件")
	rampCustom := fs.String("ramp-chars", "", "自定义字符梯度（从暗到亮），优先于 -ramp")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	args = reorderArgs(args, boolFlags, knownFlags)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *cols < 4 || *cols > 1000 {
		fmt.Fprintln(os.Stderr, "错误: -w 需在 4-1000 之间")
		return 2
	}
	ramp := ""
	if *rampCustom != "" {
		ramp = *rampCustom
	} else {
		r, ok := ramps[strings.ToLower(*rampName)]
		if !ok {
			fmt.Fprintf(os.Stderr, "错误: 未知字符梯度 %q（可选 default|simple|blocks）\n", *rampName)
			return 2
		}
		ramp = r
	}
	if len(ramp) < 2 {
		fmt.Fprintln(os.Stderr, "错误: 字符梯度至少需要 2 个字符")
		return 2
	}

	var inputs []string
	for _, a := range fs.Args() {
		if info, err := os.Stat(a); err == nil && info.IsDir() {
			_ = filepath.Walk(a, func(p string, fi os.FileInfo, err error) error {
				if err == nil && fi.Mode().IsRegular() && imgExts[strings.ToLower(filepath.Ext(p))] {
					inputs = append(inputs, p)
				}
				return nil
			})
		} else if imgExts[strings.ToLower(filepath.Ext(a))] {
			inputs = append(inputs, a)
		}
	}
	if len(inputs) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 未指定输入图片")
		return 2
	}
	if *out != "" && len(inputs) > 1 {
		fmt.Fprintln(os.Stderr, "错误: -o 仅支持单个输入")
		return 2
	}

	// 各格式解码器已通过空白导入注册
	for _, src := range inputs {
		img, _, err := loadImage(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "处理 %s 失败: 解码失败 %v\n", src, err)
			continue
		}
		art := asciiArt(img, *cols, ramp, *invert)
		if *out != "" {
			if err := os.WriteFile(*out, []byte(art), 0644); err != nil {
				fmt.Fprintln(os.Stderr, "写入文件失败:", err)
				return 1
			}
			fmt.Printf("已输出 %s（%d 行）\n", *out, strings.Count(art, "\n"))
			return 0
		}
		if len(inputs) > 1 {
			fmt.Printf("=== %s ===\n", src)
		}
		fmt.Print(art)
	}
	return 0
}
