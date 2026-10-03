// Package hmac 实现 HMAC 签名计算命令。
// 对应网页版：work/hmac-tool.html（HMAC签名）
//
// 用法：
//
//	lyntoolbox hmac -k 密钥 "消息"
//	lyntoolbox hmac -algo sha256 -khex 6b6579 -base64 -file 消息.txt
package hmac

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
)

const (
	Name  = "hmac"
	Desc  = "HMAC 签名计算，支持 SHA 家族与 MD5，输出 hex 或 Base64"
	Usage = `用法: lyntoolbox hmac (-k 密钥|-khex 十六进制密钥) [-algo 算法] [-base64] [-file 路径] [文本]

参数:
  -algo    sha1|sha224|sha256|sha384|sha512|md5（默认 sha256）
  -k       UTF-8 密钥（与 -khex 二选一，必需）
  -khex    十六进制密钥
  -base64  输出 Base64（默认输出十六进制）
  -file    从文件读取消息；省略文本与 -file 时从 stdin 读取
  文本     待签名文本；省略且无 -file 时从 stdin 读取`
)

func newHash(algo string) (func() hash.Hash, bool) {
	switch strings.ToLower(algo) {
	case "sha1":
		return sha1.New, true
	case "sha224":
		return sha256.New224, true
	case "sha256":
		return sha256.New, true
	case "sha384":
		return sha512.New384, true
	case "sha512":
		return sha512.New, true
	case "md5":
		return md5.New, true
	}
	return nil, false
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	algo := fs.String("algo", "sha256", "哈希算法")
	keyStr := fs.String("k", "", "UTF-8 密钥")
	keyHex := fs.String("khex", "", "十六进制密钥")
	useB64 := fs.Bool("base64", false, "输出 Base64")
	file := fs.String("file", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	newH, ok := newHash(*algo)
	if !ok {
		fmt.Fprintf(os.Stderr, "不支持的算法: %s（可选 sha1|sha224|sha256|sha384|sha512|md5）\n", *algo)
		return 2
	}

	var key []byte
	switch {
	case *keyStr != "" && *keyHex != "":
		fmt.Fprintln(os.Stderr, "-k 与 -khex 只能二选一")
		return 2
	case *keyStr != "":
		key = []byte(*keyStr)
	case *keyHex != "":
		b, err := hex.DecodeString(strings.TrimSpace(*keyHex))
		if err != nil {
			fmt.Fprintln(os.Stderr, "-khex 不是合法十六进制:", err)
			return 2
		}
		key = b
	default:
		fmt.Fprintln(os.Stderr, "必须通过 -k 或 -khex 提供密钥")
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

	mac := hmac.New(newH, key)
	mac.Write(data)
	sum := mac.Sum(nil)

	if *useB64 {
		fmt.Println(base64.StdEncoding.EncodeToString(sum))
	} else {
		fmt.Printf("%x\n", sum)
	}
	return 0
}
