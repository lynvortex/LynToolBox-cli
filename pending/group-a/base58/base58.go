//go:build ignore

// 暂存区：未集成/未完成实现的工具源码，不参与编译。

// Package base58 实现 Base58 编解码命令。
// 对应网页版：work/base58-tool.html（Base58编解码）
//
// 用法：
//
//	lyntoolbox base58 [文本]
//	lyntoolbox base58 -d "2NEpo7tGJmPZ"
//	lyntoolbox base58 -btc -d "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa"
package base58

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"
)

const (
	Name  = "base58"
	Desc  = "Base58 与 Base58Check（比特币校验格式）编解码"
	Usage = `用法: lyntoolbox base58 [-d] [-btc] [文本]

参数:
  -d    解码模式（默认编码）
  -btc  使用 Base58Check 格式：版本字节 0x00 + 数据 + sha256 双哈希前 4 字节校验
  文本  待处理的文本；省略时从 stdin 读取`
)

const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// encode 将数据编码为 Base58 字符串。
func encode(data []byte) string {
	// 前导 0 字节映射为 '1'
	zeros := 0
	for zeros < len(data) && data[zeros] == 0 {
		zeros++
	}
	num := new(big.Int).SetBytes(data)
	base := big.NewInt(58)
	mod := new(big.Int)
	var out []byte
	for num.Sign() > 0 {
		num.DivMod(num, base, mod)
		out = append(out, alphabet[mod.Int64()])
	}
	for i := 0; i < zeros; i++ {
		out = append(out, '1')
	}
	// 反转
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

// decode 解析 Base58 字符串。
func decode(s string) ([]byte, error) {
	num := new(big.Int)
	base := big.NewInt(58)
	for _, r := range s {
		idx := strings.IndexRune(alphabet, r)
		if idx < 0 {
			return nil, fmt.Errorf("非法 Base58 字符: %q", string(r))
		}
		num.Mul(num, base)
		num.Add(num, big.NewInt(int64(idx)))
	}
	decoded := num.Bytes()
	// 前导 '1' 还原为 0 字节
	zeros := 0
	for zeros < len(s) && s[zeros] == '1' {
		zeros++
	}
	return append(make([]byte, zeros), decoded...), nil
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	decodeMode := fs.Bool("d", false, "解码模式")
	btc := fs.Bool("btc", false, "Base58Check 格式")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		return 2
	}

	var data []byte
	if fs.NArg() > 0 {
		data = []byte(strings.Join(fs.Args(), " "))
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 stdin 失败:", err)
			return 1
		}
		data = b
	}

	if *decodeMode {
		payload, err := decode(strings.TrimSpace(string(data)))
		if err != nil {
			fmt.Fprintln(os.Stderr, "解码失败:", err)
			return 1
		}
		if *btc {
			if len(payload) < 5 {
				fmt.Fprintln(os.Stderr, "数据过短，无法包含校验和")
				return 1
			}
			body, checksum := payload[:len(payload)-4], payload[len(payload)-4:]
			sum := sha256.Sum256(sha256.Sum256(body)[:])
			if !bytes.Equal(sum[:4], checksum) {
				fmt.Fprintln(os.Stderr, "校验和不匹配")
				return 1
			}
			if len(body) < 1 || body[0] != 0x00 {
				fmt.Fprintln(os.Stderr, "版本字节不是 0x00")
				return 1
			}
			payload = body[1:]
		}
		os.Stdout.Write(payload)
		fmt.Println()
		return 0
	}

	// 编码
	payload := data
	if *btc {
		versioned := append([]byte{0x00}, payload...)
		sum := sha256.Sum256(sha256.Sum256(versioned)[:])
		payload = append(versioned, sum[:4]...)
	}
	fmt.Println(encode(payload))
	return 0
}
