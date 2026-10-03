// Package totp 实现 RFC 6238 TOTP 与 RFC 4226 HOTP 一次性密码计算（手写实现）。
// 对应网页版：work/totp-tool.html（TOTP动态口令）
//
// 用法：
//
//	lyntoolbox totp -secret GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ
//	lyntoolbox totp -secret XXXX -at "2026-01-02 15:04:05"
//	lyntoolbox totp -gen
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"flag"
	"fmt"
	"hash"
	"os"
	"strings"
	"time"
)

const (
	Name  = "totp"
	Desc  = "TOTP/HOTP 动态口令计算（RFC 6238/4226，SHA1/256/512，6/8 位）"
	Usage = `用法: lyntoolbox totp -secret 密钥 [-algo SHA1] [-digits 6] [-period 30] [-at 时间]
       lyntoolbox totp -hotp -secret 密钥 -counter N
       lyntoolbox totp -gen

参数:
  -secret   Base32 密钥（容忍空格与小写；-gen 模式下省略）
  -algo     哈希算法 SHA1|SHA256|SHA512（默认 SHA1）
  -digits   口令位数 6|8（默认 6）
  -period   TOTP 时间步长，秒（默认 30）
  -at       指定计算时刻："2006-01-02 15:04:05"（本地时区）或 Unix 秒数（默认现在）
  -hotp     使用 HOTP 计数器模式（忽略 -at/-period）
  -counter  HOTP 计数器值（配合 -hotp，默认 0）
  -gen      随机生成 Base32 密钥（20 字节）并打印`
)

// decodeSecret 容错解析 Base32 密钥：去空格、转大写、去填充。
func decodeSecret(s string) ([]byte, error) {
	clean := strings.ToUpper(strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, s))
	clean = strings.TrimRight(clean, "=")
	if clean == "" {
		return nil, fmt.Errorf("密钥为空")
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(clean)
}

// hotp 计算 RFC 4226 HOTP：HMAC + 动态截断。
func hotp(key []byte, counter uint64, algo string, digits int) (string, error) {
	var h func() hash.Hash
	switch strings.ToUpper(algo) {
	case "SHA1", "":
		h = sha1.New
	case "SHA256":
		h = sha256.New
	case "SHA512":
		h = sha512.New
	default:
		return "", fmt.Errorf("不支持的算法: %s（可选 SHA1|SHA256|SHA512）", algo)
	}
	mac := hmac.New(h, key)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac.Write(buf[:])
	sum := mac.Sum(nil)
	// 动态截断：取最后一字节低 4 位为偏移
	offset := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, code%mod), nil
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	secretStr := fs.String("secret", "", "Base32 密钥")
	algo := fs.String("algo", "SHA1", "哈希算法")
	digits := fs.Int("digits", 6, "口令位数")
	period := fs.Int("period", 30, "时间步长（秒）")
	at := fs.String("at", "", "指定计算时刻")
	hotpMode := fs.Bool("hotp", false, "HOTP 计数器模式")
	counter := fs.Uint64("counter", 0, "HOTP 计数器值")
	gen := fs.Bool("gen", false, "随机生成 Base32 密钥")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if strings.ToUpper(*algo) != "SHA1" && strings.ToUpper(*algo) != "SHA256" && strings.ToUpper(*algo) != "SHA512" {
		fmt.Fprintf(os.Stderr, "不支持的算法: %s（可选 SHA1|SHA256|SHA512）\n", *algo)
		return 2
	}
	if *digits != 6 && *digits != 8 {
		fmt.Fprintf(os.Stderr, "不支持的位数: %d（可选 6|8）\n", *digits)
		return 2
	}

	if *gen {
		raw := make([]byte, 20)
		if _, err := rand.Read(raw); err != nil {
			fmt.Fprintln(os.Stderr, "生成随机密钥失败:", err)
			return 1
		}
		fmt.Println(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw))
		return 0
	}

	if *secretStr == "" {
		fmt.Fprintln(os.Stderr, "缺少 -secret 密钥（或使用 -gen 随机生成）")
		return 2
	}
	key, err := decodeSecret(*secretStr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "-secret 不是合法 Base32:", err)
		return 2
	}

	if *period <= 0 {
		fmt.Fprintln(os.Stderr, "-period 必须为正整数")
		return 2
	}

	var ctr uint64
	if *hotpMode {
		ctr = *counter
	} else {
		var t time.Time
		if *at == "" {
			t = time.Now()
		} else if isAllDigits(*at) {
			// 允许直接给 Unix 秒数，便于精确复现测试向量
			var sec int64
			if _, err := fmt.Sscanf(*at, "%d", &sec); err != nil {
				fmt.Fprintln(os.Stderr, "-at 时间戳无效:", err)
				return 2
			}
			t = time.Unix(sec, 0)
		} else {
			parsed, err := time.ParseInLocation("2006-01-02 15:04:05", *at, time.Local)
			if err != nil {
				fmt.Fprintln(os.Stderr, "-at 时间格式无效（应为 2006-01-02 15:04:05 或 Unix 秒数）:", err)
				return 2
			}
			t = parsed
		}
		if t.Unix() < 0 {
			fmt.Fprintln(os.Stderr, "-at 时间早于 1970-01-01，无法计算")
			return 2
		}
		ctr = uint64(t.Unix()) / uint64(*period)
	}

	code, err := hotp(key, ctr, *algo, *digits)
	if err != nil {
		fmt.Fprintln(os.Stderr, "计算失败:", err)
		return 1
	}
	fmt.Println(code)
	return 0
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
