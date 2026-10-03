// Package pbkdf2 实现 PBKDF2 密钥派生命令（基于 crypto/hmac 手写实现）。
// 对应网页版：work/pbkdf2-tool.html（PBKDF2）
//
// 用法：
//
//	lyntoolbox pbkdf2 -pass 123456 -salt mysalt
//	lyntoolbox pbkdf2 -pass 123456 -salthex 73616c74 -iter 10000 -len 32
package pbkdf2

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
)

const (
	Name  = "pbkdf2"
	Desc  = "PBKDF2 密钥派生（HMAC-SHA1/SHA256/SHA512），输出十六进制"
	Usage = `用法: lyntoolbox pbkdf2 (-pass 密码) [-salt 盐|-salthex 十六进制盐] [-iter 次数] [-len 长度] [-algo 算法]

参数:
  -pass     派生密码（必需；省略时从 stdin 读取，自动去除行尾换行）
  -salt     UTF-8 盐（与 -salthex 二选一）；两者都省略时随机生成 16 字节盐并打印
  -salthex  十六进制盐
  -iter     迭代次数（默认 10000）
  -len      派生密钥长度，字节数（默认 32）
  -algo     sha256|sha512|sha1（默认 sha256）

输出:
  派生密钥以十六进制打印到 stdout；随机盐的十六进制值打印到 stderr`
)

// deriveKey 手写 PBKDF2（RFC 8018）。
func deriveKey(password, salt []byte, iter, keyLen int, h func() hash.Hash) []byte {
	prf := hmac.New(h, password)
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen
	dk := make([]byte, 0, numBlocks*hashLen)
	u := make([]byte, hashLen)
	var blockBuf [4]byte
	for block := 1; block <= numBlocks; block++ {
		// U1 = PRF(password, salt || INT_32_BE(block))
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(blockBuf[:], uint32(block))
		prf.Write(blockBuf[:])
		dk = prf.Sum(dk)
		t := dk[len(dk)-hashLen:]
		copy(u, t)
		// U2..Uc 逐步异或进 T
		for n := 2; n <= iter; n++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for i := range u {
				t[i] ^= u[i]
			}
		}
	}
	return dk[:keyLen]
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	pass := fs.String("pass", "", "派生密码")
	saltStr := fs.String("salt", "", "UTF-8 盐")
	saltHex := fs.String("salthex", "", "十六进制盐")
	iter := fs.Int("iter", 10000, "迭代次数")
	keyLen := fs.Int("len", 32, "派生密钥长度（字节）")
	algo := fs.String("algo", "sha256", "哈希算法")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	var h func() hash.Hash
	switch strings.ToLower(*algo) {
	case "sha256":
		h = sha256.New
	case "sha512":
		h = sha512.New
	case "sha1":
		h = sha1.New
	default:
		fmt.Fprintf(os.Stderr, "不支持的算法: %s（可选 sha256|sha512|sha1）\n", *algo)
		return 2
	}
	if *iter <= 0 {
		fmt.Fprintln(os.Stderr, "-iter 必须为正整数")
		return 2
	}
	if *keyLen <= 0 {
		fmt.Fprintln(os.Stderr, "-len 必须为正整数")
		return 2
	}

	var password []byte
	if *pass != "" {
		password = []byte(*pass)
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 stdin 失败:", err)
			return 1
		}
		s := string(b)
		s = strings.TrimSuffix(s, "\n")
		s = strings.TrimSuffix(s, "\r")
		password = []byte(s)
	}

	var salt []byte
	switch {
	case *saltStr != "" && *saltHex != "":
		fmt.Fprintln(os.Stderr, "-salt 与 -salthex 只能二选一")
		return 2
	case *saltStr != "":
		salt = []byte(*saltStr)
	case *saltHex != "":
		b, err := hex.DecodeString(strings.TrimSpace(*saltHex))
		if err != nil {
			fmt.Fprintln(os.Stderr, "-salthex 不是合法十六进制:", err)
			return 2
		}
		salt = b
	default:
		salt = make([]byte, 16)
		if _, err := rand.Read(salt); err != nil {
			fmt.Fprintln(os.Stderr, "生成随机盐失败:", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "盐(salt): %s\n", hex.EncodeToString(salt))
	}

	dk := deriveKey(password, salt, *iter, *keyLen, h)
	fmt.Printf("%x\n", dk)
	return 0
}
