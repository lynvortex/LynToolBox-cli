// Package aes 实现 AES 加解密命令，支持 CBC/ECB/CFB 模式与 PKCS7 填充。
// 对应网页版：work/aes-tool.html（AES加解密）
//
// 用法：
//
//	lyntoolbox aes -key 1234567890abcdef "hello"
//	lyntoolbox aes -mode ecb -keyhex 31323334... -d <密文>
//	lyntoolbox aes -key 秘钥 -file 明文.txt -outfmt hex -o 密文.txt
package aes

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	Name  = "aes"
	Desc  = "AES 加解密，支持 CBC/ECB/CFB 模式与 PKCS7 填充"
	Usage = `用法: lyntoolbox aes [-mode 模式] (-key 文本密钥|-keyhex 十六进制密钥) [-d] [文本]

参数:
  -mode    加密模式 cbc|ecb|cfb（默认 cbc）
  -key     UTF-8 文本密钥（与 -keyhex 二选一），长度 16/24/32 字节对应 AES-128/192/256
  -keyhex  十六进制密钥（与 -key 二选一）
  -iv      UTF-8 初始向量（ECB 模式忽略），默认全零；可用 -ivhex 十六进制指定
  -d       解密模式（默认加密）
  -file    从文件读取输入
  -outfmt  加密输出格式 base64|hex|raw（默认 base64；raw 需配合 -o 写文件）
  -infmt   解密输入格式 base64|hex|raw|auto（默认 auto 自动识别；raw 为二进制密文）
  -o       输出到文件
  文本     待处理文本；省略且无 -file 时从 stdin 读取`
)

// getKey 解析 -key / -keyhex，返回密钥字节。
func getKey(keyStr, keyHex string) ([]byte, error) {
	switch {
	case keyStr != "" && keyHex != "":
		return nil, fmt.Errorf("-key 与 -keyhex 只能二选一")
	case keyStr != "":
		return []byte(keyStr), nil
	case keyHex != "":
		k, err := hex.DecodeString(strings.TrimSpace(keyHex))
		if err != nil {
			return nil, fmt.Errorf("-keyhex 不是合法十六进制: %v", err)
		}
		return k, nil
	default:
		return nil, fmt.Errorf("必须通过 -key 或 -keyhex 提供密钥")
	}
}

// getIV 解析 -iv / -ivhex，未提供时返回全零 IV。
func getIV(ivStr, ivHex string) ([]byte, error) {
	switch {
	case ivStr != "" && ivHex != "":
		return nil, fmt.Errorf("-iv 与 -ivhex 只能二选一")
	case ivStr != "":
		return []byte(ivStr), nil
	case ivHex != "":
		iv, err := hex.DecodeString(strings.TrimSpace(ivHex))
		if err != nil {
			return nil, fmt.Errorf("-ivhex 不是合法十六进制: %v", err)
		}
		return iv, nil
	default:
		return make([]byte, aes.BlockSize), nil
	}
}

// pkcs7Pad 按 PKCS7 规则填充。
func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	return append(data, bytes.Repeat([]byte{byte(pad)}, pad)...)
}

// pkcs7Unpad 去除 PKCS7 填充。
func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("数据长度不是 %d 字节的整数倍（密钥错误或数据损坏）", blockSize)
	}
	pad := int(data[len(data)-1])
	if pad <= 0 || pad > blockSize || pad > len(data) {
		return nil, fmt.Errorf("填充数据无效（密钥错误或数据损坏）")
	}
	for _, b := range data[len(data)-pad:] {
		if int(b) != pad {
			return nil, fmt.Errorf("填充数据无效（密钥错误或数据损坏）")
		}
	}
	return data[:len(data)-pad], nil
}

// ecbCrypt 手写 ECB 分块处理，encrypt 为 true 加密、false 解密。
func ecbCrypt(block cipher.Block, data []byte, encrypt bool) []byte {
	bs := block.BlockSize()
	out := make([]byte, len(data))
	for i := 0; i+bs <= len(data); i += bs {
		if encrypt {
			block.Encrypt(out[i:i+bs], data[i:i+bs])
		} else {
			block.Decrypt(out[i:i+bs], data[i:i+bs])
		}
	}
	return out
}

// decodeCiphertext 将密文文本还原为字节（auto：严格十六进制优先，其次 base64）。
func decodeCiphertext(s, infmt string) ([]byte, error) {
	switch infmt {
	case "raw":
		return []byte(s), nil // 二进制密文，不做任何处理
	}
	s = strings.TrimSpace(s)
	switch infmt {
	case "hex":
		return hex.DecodeString(s)
	case "base64":
		return base64.StdEncoding.DecodeString(s)
	default: // auto
		if len(s)%2 == 0 && len(s) > 0 && isHex(s) {
			if b, err := hex.DecodeString(s); err == nil {
				return b, nil
			}
		}
		return base64.StdEncoding.DecodeString(s)
	}
}

func isHex(s string) bool {
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	mode := fs.String("mode", "cbc", "加密模式 cbc|ecb|cfb")
	keyStr := fs.String("key", "", "UTF-8 文本密钥")
	keyHex := fs.String("keyhex", "", "十六进制密钥")
	ivStr := fs.String("iv", "", "UTF-8 初始向量")
	ivHex := fs.String("ivhex", "", "十六进制初始向量")
	decrypt := fs.Bool("d", false, "解密模式")
	file := fs.String("file", "", "输入文件")
	outfmt := fs.String("outfmt", "base64", "加密输出格式 base64|hex|raw")
	infmt := fs.String("infmt", "auto", "解密输入格式 base64|hex|auto")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	*mode = strings.ToLower(*mode)
	if *mode != "cbc" && *mode != "ecb" && *mode != "cfb" {
		fmt.Fprintf(os.Stderr, "不支持的模式: %s（可选 cbc|ecb|cfb）\n", *mode)
		return 2
	}
	*outfmt = strings.ToLower(*outfmt)
	if *outfmt != "base64" && *outfmt != "hex" && *outfmt != "raw" {
		fmt.Fprintf(os.Stderr, "不支持的输出格式: %s（可选 base64|hex|raw）\n", *outfmt)
		return 2
	}
	*infmt = strings.ToLower(*infmt)
	if *infmt != "base64" && *infmt != "hex" && *infmt != "raw" && *infmt != "auto" {
		fmt.Fprintf(os.Stderr, "不支持的输入格式: %s（可选 base64|hex|raw|auto）\n", *infmt)
		return 2
	}
	if *outfmt == "raw" && *out == "" {
		fmt.Fprintln(os.Stderr, "raw 输出为二进制数据，必须配合 -o 指定输出文件")
		return 2
	}

	key, err := getKey(*keyStr, *keyHex)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		fmt.Fprintf(os.Stderr, "密钥长度 %d 字节无效，必须为 16/24/32 字节（对应 AES-128/192/256）\n", len(key))
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

	block, err := aes.NewCipher(key)
	if err != nil {
		fmt.Fprintln(os.Stderr, "初始化 AES 失败:", err)
		return 1
	}
	bs := block.BlockSize()

	var result []byte
	if *decrypt {
		ciphertext, err := decodeCiphertext(string(data), *infmt)
		if err != nil {
			fmt.Fprintln(os.Stderr, "密文格式解析失败:", err)
			return 1
		}
		switch *mode {
		case "ecb":
			if len(ciphertext)%bs != 0 || len(ciphertext) == 0 {
				err = fmt.Errorf("密文长度不是 %d 字节的整数倍", bs)
				break
			}
			result, err = pkcs7Unpad(ecbCrypt(block, ciphertext, false), bs)
		case "cbc":
			if len(ciphertext)%bs != 0 || len(ciphertext) == 0 {
				err = fmt.Errorf("密文长度不是 %d 字节的整数倍", bs)
				break
			}
			iv, e := getIV(*ivStr, *ivHex)
			if e != nil {
				fmt.Fprintln(os.Stderr, e)
				return 2
			}
			if len(iv) != bs {
				fmt.Fprintf(os.Stderr, "IV 长度 %d 字节无效，必须为 %d 字节\n", len(iv), bs)
				return 2
			}
			plain := make([]byte, len(ciphertext))
			cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ciphertext)
			result, err = pkcs7Unpad(plain, bs)
		case "cfb":
			iv, e := getIV(*ivStr, *ivHex)
			if e != nil {
				fmt.Fprintln(os.Stderr, e)
				return 2
			}
			if len(iv) != bs {
				fmt.Fprintf(os.Stderr, "IV 长度 %d 字节无效，必须为 %d 字节\n", len(iv), bs)
				return 2
			}
			plain := make([]byte, len(ciphertext))
			cipher.NewCFBDecrypter(block, iv).XORKeyStream(plain, ciphertext)
			result, err = pkcs7Unpad(plain, bs)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "解密失败:", err)
			return 1
		}
	} else {
		switch *mode {
		case "ecb":
			fmt.Fprintln(os.Stderr, "警告: ECB 模式相同明文块产生相同密文，安全性弱，建议改用 -mode cbc 并指定随机 -iv")
			result = ecbCrypt(block, pkcs7Pad(data, bs), true)
		case "cbc":
			iv, e := getIV(*ivStr, *ivHex)
			if e != nil {
				fmt.Fprintln(os.Stderr, e)
				return 2
			}
			if len(iv) != bs {
				fmt.Fprintf(os.Stderr, "IV 长度 %d 字节无效，必须为 %d 字节\n", len(iv), bs)
				return 2
			}
			if *ivStr == "" && *ivHex == "" {
				fmt.Fprintln(os.Stderr, "警告: 未指定 -iv，使用全零 IV（确定性加密），建议提供随机 IV")
			}
			padded := pkcs7Pad(data, bs)
			result = make([]byte, len(padded))
			cipher.NewCBCEncrypter(block, iv).CryptBlocks(result, padded)
		case "cfb":
			iv, e := getIV(*ivStr, *ivHex)
			if e != nil {
				fmt.Fprintln(os.Stderr, e)
				return 2
			}
			if len(iv) != bs {
				fmt.Fprintf(os.Stderr, "IV 长度 %d 字节无效，必须为 %d 字节\n", len(iv), bs)
				return 2
			}
			if *ivStr == "" && *ivHex == "" {
				fmt.Fprintln(os.Stderr, "警告: 未指定 -iv，使用全零 IV（确定性加密），建议提供随机 IV")
			}
			padded := pkcs7Pad(data, bs)
			result = make([]byte, len(padded))
			cipher.NewCFBEncrypter(block, iv).XORKeyStream(result, padded)
		}
	}

	if *out != "" {
		var payload []byte
		if *decrypt {
			// 解密输出原始明文，不做编码
			payload = result
		} else {
			switch *outfmt {
			case "hex":
				payload = []byte(hex.EncodeToString(result))
			case "base64":
				payload = []byte(base64.StdEncoding.EncodeToString(result))
			default:
				payload = result
			}
		}
		if err := os.WriteFile(*out, []byte(payload), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "写入文件失败:", err)
			return 1
		}
		fmt.Printf("已写入 %s（%d 字节）\n", *out, len(payload))
		return 0
	}

	if *decrypt {
		// 解密输出原始明文，不做编码
		os.Stdout.Write(result)
		fmt.Println()
		return 0
	}
	switch *outfmt {
	case "hex":
		fmt.Println(hex.EncodeToString(result))
	case "base64":
		fmt.Println(base64.StdEncoding.EncodeToString(result))
	default: // raw 仅在加密配合 -o 时可达
		os.Stdout.Write(result)
		fmt.Println()
	}
	return 0
}
