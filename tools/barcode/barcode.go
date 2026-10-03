// Package barcode 实现条形码生成命令（Code128/EAN-13/UPC-A）。
// 对应网页版：image/barcode-tool.html（条形码生成）
//
// 用法：
//
//	lyntoolbox barcode 文本 [-type code128|ean13|upca] [-w 100] [-scale 2] [-margin 10] [-o out.png]
package barcode

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strconv"
	"strings"

	bcode "github.com/boombuler/barcode"
	"github.com/boombuler/barcode/code128"
	"github.com/boombuler/barcode/ean"
)

const (
	Name  = "barcode"
	Desc  = "条形码生成（Code128/EAN-13/UPC-A），输出 PNG"
	Usage = `用法:
  lyntoolbox barcode 文本 [-type code128|ean13|upca] [-w 100] [-scale 2] [-margin 10] [-o out.png]

参数:
  -type    条码类型 code128|ean13|upca（默认 code128）
  -w       条码高度像素（默认 100）
  -scale   横向模块放大倍数（默认 2，越大越宽）
  -margin  左右留白像素（默认 10）
  -o       输出 PNG 路径（默认 barcode.png）

校验规则:
  code128  任意可打印 ASCII 文本
  ean13    12 位数字自动补校验位；13 位数字则校验
  upca     11 位数字自动补校验位；12 位数字则校验（编码为前导 0 的 EAN-13）`
)

// ean13CheckDigit 计算 EAN-13/UPC-A 校验位（对不含校验位的数字串）。
func ean13CheckDigit(digits string) (int, error) {
	sum := 0
	for i, c := range digits {
		d, err := strconv.Atoi(string(c))
		if err != nil {
			return 0, errors.New("必须为数字")
		}
		// 从左起第 1 位权重 1，第 2 位权重 3，交替
		if i%2 == 0 {
			sum += d
		} else {
			sum += d * 3
		}
	}
	return (10 - sum%10) % 10, nil
}

// normalizeEAN13 校验并归一化 EAN-13 输入，返回 13 位数字串。
func normalizeEAN13(text string) (string, error) {
	if len(text) == 12 {
		c, err := ean13CheckDigit(text)
		if err != nil {
			return "", err
		}
		text += strconv.Itoa(c)
	}
	if len(text) != 13 {
		return "", fmt.Errorf("EAN-13 需为 12 位（自动补校验）或 13 位数字，当前 %d 位", len(text))
	}
	c, err := ean13CheckDigit(text[:12])
	if err != nil {
		return "", err
	}
	if strconv.Itoa(c) != text[12:] {
		return "", fmt.Errorf("校验位错误: 末位应为 %s 而非 %s", strconv.Itoa(c), text[12:])
	}
	return text, nil
}

// upcaCheckDigit 计算 UPC-A 校验位（对 11 位数据串）：
// 自左起第 1、3、5…位（1 基）权重 3，第 2、4…位权重 1。
func upcaCheckDigit(digits string) (int, error) {
	sum := 0
	for i, c := range digits {
		d, err := strconv.Atoi(string(c))
		if err != nil {
			return 0, errors.New("必须为数字")
		}
		if i%2 == 0 {
			sum += d * 3
		} else {
			sum += d
		}
	}
	return (10 - sum%10) % 10, nil
}

// normalizeUPCA 校验并归一化 UPC-A 输入，返回编码用的 13 位 EAN 串。
// 注：EAN-13 对 "0"+11 位数据计算出的校验位与 UPC-A 校验位一致。
func normalizeUPCA(text string) (string, error) {
	if len(text) == 11 {
		c, err := upcaCheckDigit(text)
		if err != nil {
			return "", err
		}
		text += strconv.Itoa(c)
	}
	if len(text) != 12 {
		return "", fmt.Errorf("UPC-A 需为 11 位（自动补校验）或 12 位数字，当前 %d 位", len(text))
	}
	c, err := upcaCheckDigit(text[:11])
	if err != nil {
		return "", err
	}
	if strconv.Itoa(c) != text[11:] {
		return "", fmt.Errorf("校验位错误: 末位应为 %s 而非 %s", strconv.Itoa(c), text[11:])
	}
	return "0" + text, nil
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
	knownFlags = map[string]bool{"type": true, "w": true, "scale": true, "margin": true, "o": true}
	boolFlags  = map[string]bool{}
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	typ := fs.String("type", "code128", "条码类型 code128|ean13|upca")
	height := fs.Int("w", 100, "条码高度像素")
	scale := fs.Int("scale", 2, "横向模块放大倍数")
	margin := fs.Int("margin", 10, "左右留白像素")
	out := fs.String("o", "", "输出 PNG 路径（默认 barcode.png）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	args = reorderArgs(args, boolFlags, knownFlags)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	text := strings.Join(fs.Args(), " ")
	if text == "" {
		fmt.Fprintln(os.Stderr, "错误: 需要提供要编码的文本")
		return 2
	}
	if *height < 10 || *height > 2000 {
		fmt.Fprintln(os.Stderr, "错误: -w 需在 10-2000 之间")
		return 2
	}
	if *scale < 1 || *scale > 20 {
		fmt.Fprintln(os.Stderr, "错误: -scale 需在 1-20 之间")
		return 2
	}
	if *margin < 0 || *margin > 500 {
		fmt.Fprintln(os.Stderr, "错误: -margin 需在 0-500 之间")
		return 2
	}

	var bc bcode.Barcode
	var err error
	var display string
	switch strings.ToLower(*typ) {
	case "code128":
		for _, r := range text {
			if r < 32 || r > 126 {
				fmt.Fprintln(os.Stderr, "错误: code128 仅支持可打印 ASCII 字符")
				return 2
			}
		}
		bc, err = code128.Encode(text)
		display = text
	case "ean13":
		norm, nerr := normalizeEAN13(text)
		if nerr != nil {
			fmt.Fprintln(os.Stderr, "错误:", nerr)
			return 2
		}
		bc, err = ean.Encode(norm)
		display = norm
	case "upca":
		norm, nerr := normalizeUPCA(text)
		if nerr != nil {
			fmt.Fprintln(os.Stderr, "错误:", nerr)
			return 2
		}
		bc, err = ean.Encode(norm)
		display = norm
	default:
		fmt.Fprintf(os.Stderr, "错误: 未知类型 %q（可选 code128|ean13|upca）\n", *typ)
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "编码失败:", err)
		return 1
	}

	// 缩放：横向按模块倍数放大，高度取 -w
	bb := bc.Bounds()
	if bb.Dx() < 1 {
		fmt.Fprintln(os.Stderr, "编码结果为空")
		return 1
	}
	targetW := bb.Dx() * *scale
	scaled, err := bcode.Scale(bc, targetW, *height)
	if err != nil {
		fmt.Fprintln(os.Stderr, "缩放失败:", err)
		return 1
	}
	sb := scaled.Bounds()

	// 白底画布 + 左右留白
	canvas := image.NewRGBA(image.Rect(0, 0, sb.Dx()+2**margin, sb.Dy()+2**margin))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(*margin, *margin, *margin+sb.Dx(), *margin+sb.Dy()), scaled, sb.Min, draw.Src)

	dst := *out
	if dst == "" {
		dst = "barcode.png"
	}
	f, err := os.Create(dst)
	if err != nil {
		fmt.Fprintln(os.Stderr, "创建输出失败:", err)
		return 1
	}
	if err := png.Encode(f, canvas); err != nil {
		f.Close()
		os.Remove(dst)
		fmt.Fprintln(os.Stderr, "编码 PNG 失败:", err)
		return 1
	}
	f.Close()
	fmt.Printf("已生成 %s 条码（内容 %s，尺寸 %dx%d）→ %s\n",
		strings.ToLower(*typ), display, canvas.Bounds().Dx(), canvas.Bounds().Dy(), dst)
	return 0
}
