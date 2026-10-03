// Package imgcolor 实现图片主色提取命令。
// 对应网页版：image/color-extract-tool.html（颜色提取）
//
// 用法：
//
//	lyntoolbox imgcolor 图片... [-n 8] [-json]
//	lyntoolbox imgcolor -dir 目录 [-n 8] [-json]
package imgcolor

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	Name  = "imgcolor"
	Desc  = "提取图片主色调（量化+合并），输出 HEX/RGB 与占比"
	Usage = `用法:
  lyntoolbox imgcolor 图片... [-n 8] [-json]
  lyntoolbox imgcolor -dir 目录 [同上参数]

参数:
  -n      输出主色数量（默认 8）
  -json   以 JSON 格式输出
  -dir    输入目录（批量分析目录内全部图片）`
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

type domColor struct {
	Hex   string  `json:"hex"`
	R     int     `json:"r"`
	G     int     `json:"g"`
	B     int     `json:"b"`
	Ratio float64 `json:"ratio"`
}

// dominantColors 缩小到 ≤64px 后按 RGB 每通道 4bit 量化计数，合并相近桶。
func dominantColors(img image.Image, n int) []domColor {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	// 缩放到最长边 ≤64
	scale := 1.0
	long := sw
	if sh > sw {
		long = sh
	}
	if long > 64 {
		scale = 64.0 / float64(long)
	}
	dw := int(math.Round(float64(sw) * scale))
	dh := int(math.Round(float64(sh) * scale))
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	small := image.NewRGBA(image.Rect(0, 0, dw, dh))
	draw.CatmullRom.Scale(small, small.Bounds(), img, img.Bounds(), draw.Src, nil)

	type bucket struct {
		count            int
		sumR, sumG, sumB int64
	}
	buckets := map[uint16]*bucket{}
	total := 0
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			c := small.RGBAAt(x, y)
			key := uint16(c.R>>4)<<8 | uint16(c.G>>4)<<4 | uint16(c.B>>4)
			bk := buckets[key]
			if bk == nil {
				bk = &bucket{}
				buckets[key] = bk
			}
			bk.count++
			bk.sumR += int64(c.R)
			bk.sumG += int64(c.G)
			bk.sumB += int64(c.B)
			total++
		}
	}
	// 按数量降序排列桶
	type entry struct {
		key   uint16
		avgR  float64
		avgG  float64
		avgB  float64
		count int
	}
	var list []entry
	for k, v := range buckets {
		list = append(list, entry{
			key:   k,
			avgR:  float64(v.sumR) / float64(v.count),
			avgG:  float64(v.sumG) / float64(v.count),
			avgB:  float64(v.sumB) / float64(v.count),
			count: v.count,
		})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].count > list[j].count })

	// 合并距离小于阈值的相邻桶（以出现最多的桶为代表色）
	const mergeDist = 60
	var merged []entry
	used := make([]bool, len(list))
	for i := 0; i < len(list); i++ {
		if used[i] {
			continue
		}
		used[i] = true
		rep := list[i]
		for j := i + 1; j < len(list); j++ {
			if used[j] {
				continue
			}
			dr := list[j].avgR - rep.avgR
			dg := list[j].avgG - rep.avgG
			db := list[j].avgB - rep.avgB
			if math.Sqrt(dr*dr+dg*dg+db*db) < mergeDist {
				used[j] = true
				rep.count += list[j].count
				// 代表色保持不动（取最大桶均值）
			}
		}
		merged = append(merged, rep)
	}
	// 重新按合并后的数量排序
	sort.Slice(merged, func(i, j int) bool { return merged[i].count > merged[j].count })
	if n > 0 && len(merged) > n {
		merged = merged[:n]
	}

	out := make([]domColor, 0, len(merged))
	for _, m := range merged {
		r := int(math.Round(m.avgR))
		g := int(math.Round(m.avgG))
		bb := int(math.Round(m.avgB))
		out = append(out, domColor{
			Hex:   fmt.Sprintf("#%02X%02X%02X", r, g, bb),
			R:     r,
			G:     g,
			B:     bb,
			Ratio: float64(m.count) / float64(total),
		})
	}
	return out
}

func bar(pct float64) string {
	const width = 30
	l := int(math.Round(pct / 100 * width))
	if l < 1 {
		l = 1
	}
	return strings.Repeat("█", l) + strings.Repeat("·", width-l)
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
	knownFlags = map[string]bool{"n": true, "json": true, "dir": true}
	boolFlags  = map[string]bool{"json": true}
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	n := fs.Int("n", 8, "输出主色数量")
	jsonOut := fs.Bool("json", false, "JSON 输出")
	dir := fs.String("dir", "", "输入目录（批量）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	args = reorderArgs(args, boolFlags, knownFlags)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *n < 1 || *n > 64 {
		fmt.Fprintln(os.Stderr, "错误: -n 需在 1-64 之间")
		return 2
	}
	var inputs []string
	if *dir != "" {
		err := filepath.Walk(*dir, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() && imgExts[strings.ToLower(filepath.Ext(p))] {
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
		if imgExts[strings.ToLower(filepath.Ext(a))] {
			inputs = append(inputs, a)
		}
	}
	if len(inputs) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 未指定输入图片（可传文件参数或用 -dir）")
		return 2
	}

	failed := 0
	for _, src := range inputs {
		img, _, err := loadImage(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "处理 %s 失败: 解码失败 %v\n", src, err)
			failed++
			continue
		}
		colors := dominantColors(img, *n)
		if *jsonOut {
			data, err := json.MarshalIndent(colors, "", "  ")
			if err != nil {
				failed++
				continue
			}
			fmt.Printf("%s\n", data)
			continue
		}
		fmt.Printf("=== %s 主色 Top %d ===\n", src, len(colors))
		for i, c := range colors {
			fmt.Printf("  %2d. %s  rgb(%d,%d,%d)  %s %5.1f%%\n",
				i+1, c.Hex, c.R, c.G, c.B, bar(c.Ratio*100), c.Ratio*100)
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}
