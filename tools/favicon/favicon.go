// Package favicon 实现 Favicon/ICO 生成命令（支持多尺寸 PNG 内嵌 ICO）。
// 对应网页版：work/favicon-tool.html（Favicon生成器）
//
// 用法：
//
//	lyntoolbox favicon 图片 [-sizes 16,32,48,64,128,256] [-o favicon.ico] [-pngs]
//	lyntoolbox favicon -from-text A [-bg #4a90e2] [-fg #ffffff] [-sizes ...] [-o favicon.ico]
package favicon

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	_ "golang.org/x/image/webp"
)

const (
	Name  = "favicon"
	Desc  = "生成多尺寸 Favicon/ICO（16-256），支持图片或文字生成"
	Usage = `用法:
  lyntoolbox favicon 输入图片 [-sizes 16,32,48,64,128,256] [-o favicon.ico] [-pngs]
  lyntoolbox favicon -from-text 文字 [-bg #4a90e2] [-fg #ffffff] [-sizes ...] [-o favicon.ico] [-pngs]

参数:
  -sizes      逗号分隔的尺寸列表（默认 16,32,48,64,128,256，单个 ≤256）
  -from-text  用文字生成图标（内嵌字体仅支持 ASCII）
  -bg         文字模式背景色（默认 #4a90e2）
  -fg         文字模式前景色（默认 #ffffff）
  -o          输出 ICO 路径（默认 favicon.ico）
  -pngs       同时输出各尺寸 PNG（favicon_16.png 等）

说明: 采用现代 ICO 格式，每尺寸内嵌 PNG 数据。`
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

// renderText 用 basicfont 渲染文字为 1x 小图。
func renderText(text string, col color.RGBA) (*image.RGBA, error) {
	for _, r := range text {
		if r > 127 {
			return nil, fmt.Errorf("内嵌字体仅支持 ASCII 字符，检测到非 ASCII 字符 %q", r)
		}
	}
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

// makeTextIcon 生成正方形文字图标（256x256）。
func makeTextIcon(text string, bg, fg color.RGBA) (*image.RGBA, error) {
	const size = 256
	canvas := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	t1x, err := renderText(text, fg)
	if err != nil {
		return nil, err
	}
	tw, th := t1x.Bounds().Dx(), t1x.Bounds().Dy()
	k := int(float64(size) * 0.5 / float64(th))
	kw := int(float64(size) * 0.6 / float64(tw))
	if kw < k {
		k = kw
	}
	if k < 1 {
		k = 1
	}
	if k > 32 {
		k = 32
	}
	scaled := image.NewRGBA(image.Rect(0, 0, tw*k, th*k))
	xdraw.NearestNeighbor.Scale(scaled, scaled.Bounds(), t1x, t1x.Bounds(), xdraw.Src, nil)
	ox := (size - tw*k) / 2
	oy := (size - th*k) / 2
	draw.Draw(canvas, image.Rect(ox, oy, ox+tw*k, oy+th*k), scaled, image.Point{}, draw.Over)
	return canvas, nil
}

// resizeTo 把源图缩放为 size×size（保持比例，居中放在透明画布上）。
func resizeTo(src image.Image, size int) *image.NRGBA {
	canvas := image.NewNRGBA(image.Rect(0, 0, size, size))
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw == 0 || sh == 0 {
		return canvas
	}
	// 缩放到恰好填满画布（favicon 需要正方形；非正方形按短边适配居中）
	k := float64(size) / float64(sw)
	dw, dh := size, int(math.Round(float64(sh)*k))
	if dh > size {
		k = float64(size) / float64(sh)
		dw = int(math.Round(float64(sw) * k))
		dh = size
	}
	scaled := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), src, b, xdraw.Src, nil)
	ox := (size - dw) / 2
	oy := (size - dh) / 2
	draw.Draw(canvas, image.Rect(ox, oy, ox+dw, oy+dh), scaled, image.Point{}, draw.Src)
	return canvas
}

// writeICO 写出多尺寸 PNG 内嵌 ICO 容器。
func writeICO(w *bufio.Writer, sizes []int, blobs [][]byte) error {
	if len(sizes) != len(blobs) {
		return errors.New("sizes 与数据数量不一致")
	}
	var hdr [6]byte
	binary.LittleEndian.PutUint16(hdr[0:], 0) // reserved
	binary.LittleEndian.PutUint16(hdr[2:], 1) // type = icon
	binary.LittleEndian.PutUint16(hdr[4:], uint16(len(sizes)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	offset := uint32(6 + 16*len(sizes))
	for i, s := range sizes {
		var e [16]byte
		wb := byte(s % 256) // 256 表示为 0
		e[0] = wb
		e[1] = wb
		e[2] = 0                                 // 调色板色数
		e[3] = 0                                 // 保留
		binary.LittleEndian.PutUint16(e[4:], 1)  // planes
		binary.LittleEndian.PutUint16(e[6:], 32) // bpp
		binary.LittleEndian.PutUint32(e[8:], uint32(len(blobs[i])))
		binary.LittleEndian.PutUint32(e[12:], offset)
		if _, err := w.Write(e[:]); err != nil {
			return err
		}
		offset += uint32(len(blobs[i]))
	}
	for _, b := range blobs {
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	return w.Flush()
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
	knownFlags = map[string]bool{"sizes": true, "from-text": true, "bg": true, "fg": true, "o": true, "pngs": true}
	boolFlags  = map[string]bool{"pngs": true}
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	sizesStr := fs.String("sizes", "16,32,48,64,128,256", "逗号分隔的尺寸列表")
	fromText := fs.String("from-text", "", "用文字生成图标（ASCII）")
	bgHex := fs.String("bg", "#4a90e2", "文字模式背景色")
	fgHex := fs.String("fg", "#ffffff", "文字模式前景色")
	out := fs.String("o", "", "输出 ICO 路径（默认 favicon.ico）")
	pngs := fs.Bool("pngs", false, "同时输出各尺寸 PNG")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	args = reorderArgs(args, boolFlags, knownFlags)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	var sizes []int
	for _, s := range strings.Split(*sizesStr, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		v, err := strconv.Atoi(s)
		if err != nil || v < 1 || v > 256 {
			fmt.Fprintf(os.Stderr, "错误: 无效尺寸 %q（1-256）\n", s)
			return 2
		}
		sizes = append(sizes, v)
	}
	if len(sizes) == 0 {
		fmt.Fprintln(os.Stderr, "错误: 尺寸列表为空")
		return 2
	}

	var src image.Image
	if *fromText != "" {
		bg, err1 := parseHexColor(*bgHex)
		fg, err2 := parseHexColor(*fgHex)
		if err1 != nil {
			fmt.Fprintln(os.Stderr, "错误:", err1)
			return 2
		}
		if err2 != nil {
			fmt.Fprintln(os.Stderr, "错误:", err2)
			return 2
		}
		img, err := makeTextIcon(*fromText, bg, fg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "错误:", err)
			return 2
		}
		src = img
	} else {
		in := fs.Arg(0)
		if in == "" {
			fmt.Fprintln(os.Stderr, "错误: 需要指定输入图片或 -from-text 文字")
			return 2
		}
		f, err := os.Open(in)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取失败:", err)
			return 1
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "解码失败:", err)
			return 1
		}
		src = img
	}

	dst := *out
	if dst == "" {
		dst = "favicon.ico"
	}
	outDir := filepath.Dir(dst)
	base := strings.TrimSuffix(filepath.Base(dst), filepath.Ext(dst))

	f, err := os.Create(dst)
	if err != nil {
		fmt.Fprintln(os.Stderr, "创建输出失败:", err)
		return 1
	}
	bw := bufio.NewWriter(f)
	var blobs [][]byte
	for _, s := range sizes {
		img := resizeTo(src, s)
		var mem bytes.Buffer
		if err := png.Encode(&mem, img); err != nil {
			f.Close()
			os.Remove(dst)
			fmt.Fprintln(os.Stderr, "编码 PNG 失败:", err)
			return 1
		}
		blobs = append(blobs, append([]byte(nil), mem.Bytes()...))
		if *pngs {
			pngPath := filepath.Join(outDir, fmt.Sprintf("%s_%d.png", base, s))
			if err := os.WriteFile(pngPath, mem.Bytes(), 0644); err != nil {
				fmt.Fprintln(os.Stderr, "写入 PNG 失败:", err)
			} else {
				fmt.Printf("  %s（%d 字节）\n", pngPath, mem.Len())
			}
		}
	}
	if err := writeICO(bw, sizes, blobs); err != nil {
		f.Close()
		fmt.Fprintln(os.Stderr, "写 ICO 失败:", err)
		return 1
	}
	f.Close()
	st, _ := os.Stat(dst)
	fmt.Printf("已生成 %s（尺寸 %v，%d 字节）\n", dst, sizes, st.Size())
	return 0
}
