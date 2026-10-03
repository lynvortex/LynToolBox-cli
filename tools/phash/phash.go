// Package phash 实现图片感知哈希（pHash）命令，用于找相似图片。
// 对应网页版：image/phash-tool.html（图片相似度/pHash）
//
// 用法：
//
//	lyntoolbox phash 图片                          输出该图 64 位 pHash
//	lyntoolbox phash 图A 图B                       对比两图相似度
//	lyntoolbox phash 图A -dir 目录                 与目录内全部图片按相似度排序
package phash

import (
	"encoding/binary"
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
	Name  = "phash"
	Desc  = "图片感知哈希（64 位 DCT pHash）与相似图对比，支持目录批量比对"
	Usage = `用法:
  lyntoolbox phash 图片                    输出该图的 64 位 pHash（16 位十六进制）
  lyntoolbox phash 图A 图B                 输出两图 pHash、汉明距离与相似度
  lyntoolbox phash 图A -dir 目录           与目录内全部图片对比，按相似度降序排列

说明:
  pHash 将图片缩放为 32x32 灰度 → 2D DCT → 取左上 8x8 低频系数 →
  以中位数为阈值二值化为 64bit；汉明距离越小图片越相似（≤10 通常高度相似）`
)

var imgExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".bmp": true, ".tif": true, ".tiff": true, ".webp": true,
}

func isImageFile(p string) bool { return imgExts[strings.ToLower(filepath.Ext(p))] }

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

// toGray32 缩放到 32x32 灰度矩阵。
func toGray32(img image.Image) *[32][32]float64 {
	dst := image.NewGray(image.Rect(0, 0, 32, 32))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
	var m [32][32]float64
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			m[y][x] = float64(dst.GrayAt(x, y).Y)
		}
	}
	return &m
}

// dct1D 对长度 N 的向量做 DCT-II（预计算余弦表，可复用）。
func dctTables(n int) (cos [][]float64, kscale []float64) {
	cos = make([][]float64, n)
	kscale = make([]float64, n)
	for k := 0; k < n; k++ {
		cos[k] = make([]float64, n)
		for i := 0; i < n; i++ {
			cos[k][i] = math.Cos(math.Pi/float64(n)*((float64(i)+0.5)*float64(k))) * 2.0
		}
		if k == 0 {
			kscale[k] = math.Sqrt(1.0 / float64(n))
		} else {
			kscale[k] = math.Sqrt(2.0 / float64(n))
		}
	}
	return cos, kscale
}

// phash 计算 64 位感知哈希。
func phash(img image.Image) (uint64, error) {
	gray := toGray32(img)

	// 预乘缩放系数后的行/列变换
	cos, ks := dctTables(32)
	tmp := make([]float64, 32*32)
	// 先对每行做 DCT
	for y := 0; y < 32; y++ {
		for k := 0; k < 32; k++ {
			var s float64
			for i := 0; i < 32; i++ {
				s += gray[y][i] * cos[k][i]
			}
			tmp[y*32+k] = s * ks[k]
		}
	}
	// 再对每列做 DCT
	freq := make([]float64, 32*32)
	for k := 0; k < 32; k++ {
		for x := 0; x < 32; x++ {
			var s float64
			for i := 0; i < 32; i++ {
				s += tmp[i*32+x] * cos[k][i]
			}
			freq[k*32+x] = s * ks[k]
		}
	}
	// 左上 8x8 低频系数
	coef := make([]float64, 64)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			coef[y*8+x] = freq[y*32+x]
		}
	}
	// 中位数（剔除直流分量 DC 更符合感知）
	sorted := append([]float64(nil), coef[1:]...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	median := sorted[len(sorted)/2]
	var hash uint64
	for i := 0; i < 64; i++ {
		if i == 0 {
			continue // DC 不参与
		}
		if coef[i] > median {
			hash |= 1 << (63 - uint(i))
		}
	}
	return hash, nil
}

func hashHex(h uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], h)
	return fmt.Sprintf("%x", buf)
}

func hamming(a, b uint64) int {
	return bitsOn(a ^ b)
}

func bitsOn(v uint64) int {
	n := 0
	for v != 0 {
		v &= v - 1
		n++
	}
	return n
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
	knownFlags = map[string]bool{"dir": true}
	boolFlags  = map[string]bool{}
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	dir := fs.String("dir", "", "对比目录：计算目标图与目录内全部图片的相似度")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	args = reorderArgs(args, boolFlags, knownFlags)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	rest := fs.Args()
	var targets []string
	if *dir != "" {
		err := filepath.Walk(*dir, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() && isImageFile(p) {
				targets = append(targets, p)
			}
			return nil
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "遍历目录失败:", err)
			return 1
		}
	}
	targets = append(targets, rest...)

	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 未指定输入图片")
		return 2
	}

	load := func(p string) (image.Image, error) {
		img, err := loadImage(p)
		if err != nil {
			return nil, fmt.Errorf("%s: 解码失败 %v", p, err)
		}
		return img, nil
	}

	// 单图：输出 pHash
	if len(targets) == 1 && *dir == "" {
		img, err := load(targets[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, "错误:", err)
			return 1
		}
		h, err := phash(img)
		if err != nil {
			fmt.Fprintln(os.Stderr, "错误:", err)
			return 1
		}
		fmt.Printf("%s  %s\n", hashHex(h), targets[0])
		return 0
	}

	// 两图对比 或 目标图 vs 目录
	base := targets[0]
	if *dir == "" && len(targets) != 2 {
		fmt.Fprintln(os.Stderr, "错误: 对比模式需要恰好 2 张图片（或用 -dir 指定对比目录）")
		return 2
	}
	baseImg, err := load(base)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		return 1
	}
	baseHash, err := phash(baseImg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		return 1
	}
	fmt.Printf("目标图 %s pHash: %s\n", base, hashHex(baseHash))

	var others []string
	if *dir == "" {
		others = targets[1:2]
	} else {
		// 排除目标图自身（同一路径）
		absBase, _ := filepath.Abs(base)
		for _, t := range targets[1:] {
			absT, _ := filepath.Abs(t)
			if absT != absBase {
				others = append(others, t)
			}
		}
	}
	type result struct {
		path   string
		hash   uint64
		dist   int
		sim    float64
		failed bool
	}
	var results []result
	for _, o := range others {
		img, err := load(o)
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告: %v\n", err)
			results = append(results, result{path: o, failed: true})
			continue
		}
		h, err := phash(img)
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告: %s: %v\n", o, err)
			results = append(results, result{path: o, failed: true})
			continue
		}
		d := hamming(baseHash, h)
		results = append(results, result{path: o, hash: h, dist: d, sim: float64(64-d) / 64 * 100})
	}
	if *dir != "" {
		// 按相似度降序
		for i := 1; i < len(results); i++ {
			for j := i; j > 0; j-- {
				a, b := results[j-1], results[j]
				if (a.failed && !b.failed) || (!a.failed && !b.failed && a.dist > b.dist) {
					results[j-1], results[j] = b, a
				} else {
					break
				}
			}
		}
	}
	for _, r := range results {
		if r.failed {
			fmt.Printf("  %s  [读取失败]\n", r.path)
			continue
		}
		fmt.Printf("  %s  pHash %s  汉明距离 %d/64  相似度 %.1f%%\n", r.path, hashHex(r.hash), r.dist, r.sim)
	}
	return 0
}
