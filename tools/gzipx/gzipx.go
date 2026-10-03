// Package gzipx 实现文本 gzip 压缩解压命令。
// 对应网页版：text/gzip-tool.html（gzip压缩解压）
//
// 用法：
//
//	lyntoolbox gzipx [文本]
//	lyntoolbox gzipx -d "H4sIAAAAAAAA..."
//	lyntoolbox gzipx -raw -o out.gz [文本]
package gzipx

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const (
	Name  = "gzipx"
	Desc  = "gzip 压缩与解压文本，输出 Base64 或原始二进制"
	Usage = `用法: lyntoolbox gzipx [-d] [-level 1-9] [-raw] [-o 文件] [-file 文件] [文本]

参数:
  -d      解压模式（默认压缩）
  -level  压缩级别 1-9（默认 6；-d 时忽略）
  -raw    压缩结果输出原始 .gz 二进制到 -o 指定文件（默认输出 Base64）
  -file   从文件读取输入（压缩读文本，解压读原始 .gz 二进制）
  -o      输出到文件
  文本    待处理的文本；省略且无 -file 时从 stdin 读取`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	decode := fs.Bool("d", false, "解压模式")
	level := fs.Int("level", 6, "压缩级别 1-9")
	raw := fs.Bool("raw", false, "输出原始二进制")
	file := fs.String("file", "", "输入文件")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *level < 1 || *level > 9 {
		fmt.Fprintln(os.Stderr, "压缩级别必须是 1-9")
		return 2
	}
	if *raw && *out == "" {
		fmt.Fprintln(os.Stderr, "-raw 模式必须指定 -o 输出文件")
		return 2
	}

	var data []byte
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取文件失败:", err)
			return 1
		}
		data = b
	} else if fs.NArg() > 0 {
		data = []byte(strings.Join(fs.Args(), " "))
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 stdin 失败:", err)
			return 1
		}
		data = b
	}

	var result []byte
	if *decode {
		gzBytes := data
		// 输入为 Base64 文本时先解码；已是原始 gzip 数据（魔数 1f 8b）则直接使用
		if !(len(gzBytes) >= 2 && gzBytes[0] == 0x1f && gzBytes[1] == 0x8b) {
			clean := strings.NewReplacer("\r", "", "\n", "", "\t", "", " ", "").Replace(strings.TrimSpace(string(gzBytes)))
			dec, err := base64.StdEncoding.DecodeString(clean)
			if err != nil {
				dec, err = base64.RawStdEncoding.DecodeString(clean)
			}
			if err != nil {
				dec, err = base64.URLEncoding.DecodeString(clean)
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "Base64 解码失败:", err)
				return 1
			}
			gzBytes = dec
		}
		r, err := gzip.NewReader(bytes.NewReader(gzBytes))
		if err != nil {
			fmt.Fprintln(os.Stderr, "gzip 数据无效:", err)
			return 1
		}
		// 1 GiB 上限：恶意 gzip 解压炸弹会撑爆内存，超限直接中止
		const maxDec = 1 << 30
		dec, err := io.ReadAll(io.LimitReader(r, maxDec+1))
		if err != nil {
			fmt.Fprintln(os.Stderr, "解压失败:", err)
			return 1
		}
		if len(dec) > maxDec {
			fmt.Fprintln(os.Stderr, "错误: 解压后数据超过 1 GiB，已中止")
			return 1
		}
		result = dec
	} else {
		var buf bytes.Buffer
		w, err := gzip.NewWriterLevel(&buf, *level)
		if err != nil {
			fmt.Fprintln(os.Stderr, "创建压缩器失败:", err)
			return 1
		}
		if _, err := w.Write(data); err != nil {
			fmt.Fprintln(os.Stderr, "压缩失败:", err)
			return 1
		}
		if err := w.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "压缩失败:", err)
			return 1
		}
		gz := buf.Bytes()
		fmt.Fprintf(os.Stderr, "压缩前 %s 字节 → 压缩后 %s 字节（压缩率 %.1f%%）\n",
			strconv.Itoa(len(data)), strconv.Itoa(len(gz)),
			100*float64(len(gz))/float64(max(len(data), 1)))
		if *raw {
			result = gz
		} else {
			result = []byte(base64.StdEncoding.EncodeToString(gz))
		}
	}

	if *out != "" {
		if err := os.WriteFile(*out, result, 0644); err != nil {
			fmt.Fprintln(os.Stderr, "写入文件失败:", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "已写入 %s（%d 字节）\n", *out, len(result))
		return 0
	}
	os.Stdout.Write(result)
	fmt.Println()
	return 0
}
