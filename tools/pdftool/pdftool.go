// Package pdftool 实现 PDF 页面操作命令。
// 对应网页版：document/pdf-merge-tool.html、pdf-split-tool.html、pdf-rotate-tool.html、pdf-pages-tool.html
package pdftool

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

const (
	Name  = "pdf"
	Desc  = "PDF 页面操作：merge/split/rotate/extract/info（纯本地）"
	Usage = `用法:
  lyntoolbox pdf merge -o out.pdf in1.pdf in2.pdf ...
  lyntoolbox pdf split in.pdf -o 输出目录 [-n 1]      每 N 页拆一个文件
  lyntoolbox pdf rotate in.pdf -pages 1-3,5 -deg 90 [-o out.pdf]
  lyntoolbox pdf extract in.pdf -pages 1-5 -o out.pdf  页面提取为新 PDF
  lyntoolbox pdf info in.pdf`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, Usage+"\n")
		return 2
	}
	mode := args[0]
	switch mode {
	case "merge":
		return runMerge(args[1:])
	case "split":
		return runSplit(args[1:])
	case "rotate":
		return runRotate(args[1:])
	case "extract":
		return runExtract(args[1:])
	case "info":
		return runInfo(args[1:])
	}
	fmt.Fprintf(os.Stderr, "未知子命令: %s（merge|split|rotate|extract|info）\n", mode)
	return 2
}

func runMerge(args []string) int {
	fs := flag.NewFlagSet("merge", flag.ContinueOnError)
	out := fs.String("o", "", "输出文件（必需）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *out == "" || fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "用法: pdf merge -o out.pdf in1.pdf in2.pdf ...")
		return 2
	}
	if err := api.MergeCreateFile(context.Background(), fs.Args(), *out, false, nil); err != nil {
		fmt.Fprintln(os.Stderr, "合并失败:", err)
		return 1
	}
	st, _ := os.Stat(*out)
	fmt.Printf("已合并 %d 个文件 → %s（%d 字节）\n", fs.NArg(), *out, st.Size())
	return 0
}

func runSplit(args []string) int {
	fs := flag.NewFlagSet("split", flag.ContinueOnError)
	outDir := fs.String("o", ".", "输出目录")
	span := fs.Int("n", 1, "每个文件包含的页数")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: pdf split in.pdf -o 输出目录 [-n 页数]")
		return 2
	}
	if err := api.SplitFile(context.Background(), fs.Arg(0), *outDir, *span, nil); err != nil {
		fmt.Fprintln(os.Stderr, "拆分失败:", err)
		return 1
	}
	fmt.Printf("已拆分到 %s（每 %d 页一个文件）\n", *outDir, *span)
	return 0
}

func runRotate(args []string) int {
	fs := flag.NewFlagSet("rotate", flag.ContinueOnError)
	pages := fs.String("pages", "", "页面选择（如 1-3,5；空=全部）")
	deg := fs.Int("deg", 90, "旋转角度 90|180|270")
	out := fs.String("o", "", "输出文件（默认覆盖输入）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 || *deg%90 != 0 {
		fmt.Fprintln(os.Stderr, "用法: pdf rotate in.pdf -pages 1-3,5 -deg 90")
		return 2
	}
	in := fs.Arg(0)
	outF := *out
	if outF == "" {
		outF = in
	}
	rel := *deg
	if rel < 0 {
		rel += 360
	}
	var selected []string
	if strings.TrimSpace(*pages) != "" {
		selected = []string{*pages}
	}
	if err := api.RotateFile(context.Background(), in, outF, rel, selected, nil); err != nil {
		fmt.Fprintln(os.Stderr, "旋转失败:", err)
		return 1
	}
	fmt.Printf("已旋转 %s → %s（%d°，页面 %s）\n", in, outF, *deg, orAll(*pages))
	return 0
}

func runExtract(args []string) int {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	pages := fs.String("pages", "", "页面选择（如 1-5；空=全部）")
	out := fs.String("o", "", "输出文件（必需）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 || *out == "" {
		fmt.Fprintln(os.Stderr, "用法: pdf extract in.pdf -pages 1-5 -o out.pdf")
		return 2
	}
	var selected []string
	if strings.TrimSpace(*pages) != "" {
		selected = []string{*pages}
	}
	if err := api.TrimFile(context.Background(), fs.Arg(0), *out, selected, nil); err != nil {
		fmt.Fprintln(os.Stderr, "提取失败:", err)
		return 1
	}
	st, _ := os.Stat(*out)
	fmt.Printf("已提取页面 %s → %s（%d 字节）\n", orAll(*pages), *out, st.Size())
	return 0
}

func runInfo(args []string) int {
	fs := flag.NewFlagSet("info", flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: pdf info in.pdf")
		return 2
	}
	in := fs.Arg(0)
	f, err := os.Open(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开失败:", err)
		return 1
	}
	defer f.Close()
	info, err := api.PDFInfo(context.Background(), f, in, nil, false, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取失败:", err)
		return 1
	}
	st, _ := os.Stat(in)
	fmt.Printf("文件:     %s\n", in)
	fmt.Printf("大小:     %d 字节\n", st.Size())
	fmt.Printf("版本:     %s\n", info.Version)
	fmt.Printf("页数:     %d\n", info.PageCount)
	fmt.Printf("加密:     %v\n", info.Encrypted)
	if len(info.Dimensions) > 0 {
		d := info.Dimensions[0]
		fmt.Printf("页面尺寸: %.0f × %.0f pt（首页）\n", d.Width, d.Height)
	}
	if info.Title != "" {
		fmt.Printf("标题:     %s\n", info.Title)
	}
	if info.Author != "" {
		fmt.Printf("作者:     %s\n", info.Author)
	}
	return 0
}

func orAll(p string) string {
	if strings.TrimSpace(p) == "" {
		return "全部"
	}
	return p
}
