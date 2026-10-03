// Package pdfop 实现 PDF 优化与水印命令。
// 对应网页版：document/pdf-compress-tool.html（PDF压缩）、pdf-watermark-tool.html（PDF水印）
package pdfop

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

const (
	Name  = "pdfop"
	Desc  = "PDF 优化压缩与文字水印（纯本地）"
	Usage = `用法:
  lyntoolbox pdfop optimize in.pdf [-o out.pdf]        优化压缩，输出前后体积
  lyntoolbox pdfop watermark in.pdf -text "文字" [-o out.pdf] [-size 48] [-opacity 0.3]
                    [-rot 45] [-pos c|tl|tr|bl|br] [-color #999999]

参数:
  -pos      位置 tl/tc/tr/ml/mc/mr/bl/bc/br（默认 c 居中）
  -opacity  透明度 0-1（默认 0.3）
  -rot      旋转角度（默认 45）
  注意      pdfcpu 内置字体仅支持 ASCII，中文水印会报错`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, Usage+"\n")
		return 2
	}
	mode := args[0]
	switch mode {
	case "optimize":
		return runOptimize(args[1:])
	case "watermark":
		return runWatermark(args[1:])
	}
	fmt.Fprintf(os.Stderr, "未知子命令: %s（optimize|watermark）\n", mode)
	return 2
}

func runOptimize(args []string) int {
	fs := flag.NewFlagSet("optimize", flag.ContinueOnError)
	out := fs.String("o", "", "输出文件（默认 out_optimized.pdf）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: pdfop optimize in.pdf [-o out.pdf]")
		return 2
	}
	in := fs.Arg(0)
	outF := *out
	if outF == "" {
		outF = strings.TrimSuffix(in, ".pdf") + "_optimized.pdf"
	}
	inSt, err := os.Stat(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取失败:", err)
		return 1
	}
	if err := api.OptimizeFile(context.Background(), in, outF, nil, nil); err != nil {
		fmt.Fprintln(os.Stderr, "优化失败:", err)
		return 1
	}
	outSt, _ := os.Stat(outF)
	fmt.Printf("原始:   %d 字节\n优化后: %d 字节（%.1f%%）\n输出:   %s\n",
		inSt.Size(), outSt.Size(), 100*float64(outSt.Size())/float64(inSt.Size()), outF)
	return 0
}

func runWatermark(args []string) int {
	fs := flag.NewFlagSet("watermark", flag.ContinueOnError)
	text := fs.String("text", "", "水印文字（必需，仅 ASCII）")
	out := fs.String("o", "", "输出文件（默认 out_wm.pdf）")
	size := fs.Int("size", 48, "字号")
	opacity := fs.Float64("opacity", 0.3, "透明度 0-1")
	rot := fs.Int("rot", 45, "旋转角度")
	pos := fs.String("pos", "c", "位置 tl|tc|tr|ml|mc|mr|bl|bc|br")
	color := fs.String("color", "#999999", "颜色 #RRGGBB")
	pages := fs.String("pages", "", "页面选择（空=全部）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 || *text == "" {
		fmt.Fprintln(os.Stderr, "用法: pdfop watermark in.pdf -text \"文字\"")
		return 2
	}
	for _, r := range *text {
		if r > 127 {
			fmt.Fprintln(os.Stderr, "水印文字暂仅支持 ASCII（pdfcpu 内置字体不含中文），请换用英文/数字")
			return 1
		}
	}
	in := fs.Arg(0)
	outF := *out
	if outF == "" {
		outF = strings.TrimSuffix(in, ".pdf") + "_wm.pdf"
	}
	desc := fmt.Sprintf("font:Helvetica, points:%d, color:%s, rotation:%d, opacity:%.2f, pos:%s, scalefactor:1",
		*size, hexToRGBFloats(*color), *rot, *opacity, *pos)
	var selected []string
	if *pages != "" {
		selected = []string{*pages}
	}
	if err := api.AddTextWatermarksFile(context.Background(), in, outF, selected, true, *text, desc, nil); err != nil {
		fmt.Fprintln(os.Stderr, "加水印失败:", err)
		return 1
	}
	st, _ := os.Stat(outF)
	fmt.Printf("已加水印 → %s（%d 字节）\n", outF, st.Size())
	return 0
}

// hexToRGBFloats 把 #RRGGBB 转成 pdfcpu 描述符的 "r g b"（0-1 浮点）。
func hexToRGBFloats(hex string) string {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return "0.6 0.6 0.6"
	}
	v := make([]float64, 3)
	for i := 0; i < 3; i++ {
		var n int
		fmt.Sscanf(hex[i*2:i*2+2], "%02x", &n)
		v[i] = float64(n) / 255
	}
	return fmt.Sprintf("%.3f %.3f %.3f", v[0], v[1], v[2])
}
