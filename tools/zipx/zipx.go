// Package zipx 实现 ZIP 打包/解包/列表命令。
// 对应网页版：text/zip-tool.html（ZIP打包解包）
package zipx

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	Name  = "zipx"
	Desc  = "ZIP 打包/解包/查看（纯本地，支持目录递归）"
	Usage = `用法:
  lyntoolbox zipx pack -o out.zip 文件或目录...
  lyntoolbox zipx unpack in.zip [-d 目标目录] [-files a.txt,b/]
  lyntoolbox zipx list in.zip`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, Usage+"\n")
		return 2
	}
	mode := args[0]
	switch mode {
	case "pack":
		return runPack(args[1:])
	case "unpack":
		return runUnpack(args[1:])
	case "list":
		return runList(args[1:])
	}
	fmt.Fprintf(os.Stderr, "未知子命令: %s（pack|unpack|list）\n", mode)
	return 2
}

func runPack(args []string) int {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	out := fs.String("o", "", "输出 zip 路径（必需）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *out == "" || fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "用法: zipx pack -o out.zip 文件或目录...")
		return 2
	}
	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "创建文件失败:", err)
		return 1
	}
	defer f.Close()
	w := zip.NewWriter(f)
	defer w.Close()

	count, files := 0, int64(0)
	// add 把单个文件写入 zip；base 为计算相对路径的基准目录
	add := func(base, path string, info os.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		if base != "" {
			if rel, err := filepath.Rel(base, path); err == nil {
				name = filepath.ToSlash(rel)
			}
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = name
		hdr.Method = zip.Deflate
		hw, err := w.CreateHeader(hdr)
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		n, err := io.Copy(hw, src)
		if err != nil {
			return err
		}
		count++
		files += n
		return nil
	}

	for _, p := range fs.Args() {
		info, err := os.Stat(p)
		if err != nil {
			fmt.Fprintln(os.Stderr, "跳过", p, ":", err)
			continue
		}
		if !info.IsDir() {
			if err := add("", p, info); err != nil {
				fmt.Fprintln(os.Stderr, "打包失败:", err)
				return 1
			}
			continue
		}
		// 目录内文件按「参数目录」为根存相对路径
		base := filepath.Dir(filepath.Clean(p))
		err = filepath.Walk(p, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			return add(base, path, fi)
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "遍历目录失败:", err)
			return 1
		}
	}
	fmt.Printf("已写入 %s：%d 个文件，原始共 %d 字节\n", *out, count, files)
	return 0
}

func runUnpack(args []string) int {
	fs := flag.NewFlagSet("unpack", flag.ContinueOnError)
	dest := fs.String("d", ".", "目标目录")
	var only stringSlice
	fs.Var(&only, "files", "仅解压指定前缀（逗号分隔）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: zipx unpack in.zip [-d 目标目录]")
		return 2
	}
	r, err := zip.OpenReader(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开 zip 失败:", err)
		return 1
	}
	defer r.Close()

	count := 0
	for _, zf := range r.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		if len(only) > 0 {
			match := false
			for _, p := range only {
				if strings.HasPrefix(zf.Name, p) {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		// 防 Zip Slip：拒绝绝对路径、盘符与 ".." 逃逸段（按路径分段判断，不误伤 a..b.txt）
		if filepath.IsAbs(zf.Name) || filepath.VolumeName(zf.Name) != "" || hasDotDotSegment(zf.Name) {
			fmt.Fprintln(os.Stderr, "跳过非法路径:", zf.Name)
			continue
		}
		target := filepath.Join(*dest, filepath.FromSlash(zf.Name))
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			fmt.Fprintln(os.Stderr, "创建目录失败:", err)
			return 1
		}
		src, err := zf.Open()
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取条目失败:", err)
			return 1
		}
		dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, zf.Mode())
		if err != nil {
			src.Close()
			fmt.Fprintln(os.Stderr, "写出失败:", err)
			return 1
		}
		_, err = io.Copy(dst, src)
		src.Close()
		dst.Close()
		if err != nil {
			fmt.Fprintln(os.Stderr, "写出失败:", err)
			return 1
		}
		count++
		fmt.Println("  " + zf.Name)
	}
	fmt.Printf("共解压 %d 个文件到 %s\n", count, *dest)
	return 0
}

func runList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: zipx list in.zip")
		return 2
	}
	r, err := zip.OpenReader(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开 zip 失败:", err)
		return 1
	}
	defer r.Close()

	fmt.Printf("%-40s %12s %12s %8s\n", "名称", "原始", "压缩后", "压缩率")
	// int64 累加，32 位平台上 >2GB 不会溢出
	var orig, comp int64
	n := 0
	for _, zf := range r.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		n++
		orig += int64(zf.UncompressedSize64)
		comp += int64(zf.CompressedSize64)
		rate := 0.0
		if zf.UncompressedSize64 > 0 {
			rate = 100 * (1 - float64(zf.CompressedSize64)/float64(zf.UncompressedSize64))
		}
		fmt.Printf("%-40s %12d %12d %7.1f%%\n", truncateName(zf.Name, 40), zf.UncompressedSize64, zf.CompressedSize64, rate)
	}
	fmt.Printf("\n共 %d 个文件，原始 %d 字节，压缩后 %d 字节\n", n, orig, comp)
	return 0
}

// hasDotDotSegment 判断 zip 条目名是否含 ".." 路径段（同时识别 / 与 \ 分隔）。
func hasDotDotSegment(name string) bool {
	for _, seg := range strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' }) {
		if seg == ".." {
			return true
		}
	}
	return false
}

// truncateName 按 rune 截断，避免切断 UTF-8 序列。
func truncateName(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return ".." + string(runes[len(runes)-n+2:])
}

// stringSlice 支持 -flag 可重复。
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}
