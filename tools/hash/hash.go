// Package hash 实现哈希计算命令。
// 对应网页版：text/hash-tool.html（哈希编解码）、work/file-hash-tool.html（文件哈希校验）
//
// 用法：
//
//	lyntoolbox hash [-algo sha256] [文本]
//	lyntoolbox hash -file 文件路径 [-algo md5]
//	lyntoolbox hash -all -file 文件路径
package hash

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"flag"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"os"
	"strings"
)

const (
	Name  = "hash"
	Desc  = "计算 MD5/SHA/CRC32 哈希，支持文本与文件（合并：哈希编解码+文件哈希校验）"
	Usage = `用法: lyntoolbox hash [-algo 算法] [-all] [-file 路径] [文本]

参数:
  -algo    md5|sha1|sha224|sha256|sha384|sha512|crc32（默认 sha256）
  -all     同时输出全部算法结果
  -file    对文件计算哈希；省略文本与 -file 时从 stdin 读取`
)

func newHash(algo string) (hash.Hash, bool) {
	switch strings.ToLower(algo) {
	case "md5":
		return md5.New(), true
	case "sha1":
		return sha1.New(), true
	case "sha224":
		return sha256.New224(), true
	case "sha256":
		return sha256.New(), true
	case "sha384":
		return sha512.New384(), true
	case "sha512":
		return sha512.New(), true
	case "crc32":
		return crc32.NewIEEE(), true
	}
	return nil, false
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	algo := fs.String("algo", "sha256", "哈希算法")
	all := fs.Bool("all", false, "输出全部算法")
	file := fs.String("file", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	algos := []string{strings.ToLower(*algo)}
	if *all {
		algos = []string{"md5", "sha1", "sha224", "sha256", "sha384", "sha512", "crc32"}
	}
	for _, a := range algos {
		if _, ok := newHash(a); !ok {
			fmt.Fprintf(os.Stderr, "不支持的算法: %s\n", a)
			return 2
		}
	}

	// 输入统一为一次性读取的 byte 切片，保证多算法可重复摘要
	var data []byte
	src := "stdin"
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取文件失败:", err)
			return 1
		}
		data = b
		src = *file
	} else if fs.NArg() > 0 {
		data = []byte(strings.Join(fs.Args(), " "))
		src = "text"
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 stdin 失败:", err)
			return 1
		}
		data = b
	}

	for _, a := range algos {
		h, _ := newHash(a)
		h.Write(data)
		if *all {
			fmt.Printf("%-8s %s\n", a+":", fmt.Sprintf("%x", h.Sum(nil)))
		} else {
			fmt.Printf("%s  %s\n", fmt.Sprintf("%x", h.Sum(nil)), src)
		}
	}
	return 0
}
