// Package smcrypto 实现国密算法套件命令：SM3 摘要、SM4 加解密、SM2 密钥/加解密/签名验签。
// 对应网页版：text/sm3-tool.html、text/sm4-tool.html、text/sm2-tool.html
//
// 用法：
//
//	lyntoolbox smcrypto sm3 "abc"
//	lyntoolbox smcrypto sm4 -key 0123456789abcdef "hello"
//	lyntoolbox smcrypto sm2 keygen
//	lyntoolbox smcrypto sm2 sign -privhex <私钥> "消息"
package smcrypto

import (
	"bytes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"

	"github.com/tjfoc/gmsm/sm2"
	"github.com/tjfoc/gmsm/sm3"
	"github.com/tjfoc/gmsm/sm4"
)

const (
	Name  = "smcrypto"
	Desc  = "国密算法套件：SM3 摘要、SM4 加解密、SM2 密钥/加解密/签名验签"
	Usage = `用法: lyntoolbox smcrypto <子命令> [参数] [文本]

子命令:
  sm3          SM3 摘要（文本/-file/stdin，输出十六进制）
  sm4          SM4 加解密（-mode ecb|cbc，默认 ecb，PKCS7 填充，-d 解密）
  sm2 keygen   生成 SM2 密钥对（输出十六进制公私钥）
  sm2 encrypt  SM2 加密（-pubhex 公钥）
  sm2 decrypt  SM2 解密（-privhex 私钥）
  sm2 sign     SM2 签名（-privhex 私钥，输出 r||s 十六进制）
  sm2 verify   SM2 验签（-pubhex 公钥，-sighex r||s；验证失败退出码为 1）

sm3 参数:
  -file    对文件计算摘要；省略文本与 -file 时从 stdin 读取

sm4 参数:
  -mode    ecb|cbc（默认 ecb）
  -key     UTF-8 密钥（与 -keyhex 二选一），必须恰好 16 字节
  -keyhex  十六进制密钥
  -iv      UTF-8 初始向量（仅 cbc，默认全零）；-ivhex 十六进制
  -d       解密模式
  -file    从文件读取输入
  -outfmt  加密输出 base64|hex|raw（默认 base64；raw 需配合 -o）
  -infmt   解密输入 base64|hex|auto（默认 auto）
  -o       输出到文件

sm2 参数:
  -mode    密文排列 c1c3c2|c1c2c3（默认 c1c3c2）
  -privhex 私钥（十六进制标量 D）
  -pubhex  公钥（十六进制，04||X||Y 非压缩 65 字节或压缩 33 字节）
  -sighex  签名（r||s 共 128 个十六进制字符）
  -uid     签名用户 ID（默认 1234567812345678）
  -outfmt  加密/签名输出 hex|base64（默认 hex）
  -infmt   解密输入 hex|base64|auto（默认 auto）
  -file    从文件读取输入
  文本     待处理文本；省略且无 -file 时从 stdin 读取`
)

// options 汇总全部子命令共用的旗标。
type options struct {
	mode    string
	key     string
	keyHex  string
	iv      string
	ivHex   string
	file    string
	outfmt  string
	infmt   string
	out     string
	privHex string
	pubHex  string
	sigHex  string
	uid     string
	decrypt bool
}

// ---------- 通用辅助 ----------

func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	return append(data, bytes.Repeat([]byte{byte(pad)}, pad)...)
}

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

func isHex(s string) bool {
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

// decodeText 将密文文本还原为字节（auto：严格十六进制优先，其次 base64）。
func decodeText(s, infmt string) ([]byte, error) {
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
	default:
		if len(s)%2 == 0 && len(s) > 0 && isHex(s) {
			if b, err := hex.DecodeString(s); err == nil {
				return b, nil
			}
		}
		return base64.StdEncoding.DecodeString(s)
	}
}

// readInput 按优先级读取输入：-file > 位置参数 > stdin。
func readInput(fs *flag.FlagSet, file string) ([]byte, error) {
	if file != "" {
		return os.ReadFile(file)
	}
	if fs.NArg() > 0 {
		return []byte(strings.Join(fs.Args(), " ")), nil
	}
	return io.ReadAll(os.Stdin)
}

// getKey 解析 -key / -keyhex 并校验长度。
func getKey(keyStr, keyHex string, wantLen int) ([]byte, error) {
	var key []byte
	switch {
	case keyStr != "" && keyHex != "":
		return nil, fmt.Errorf("-key 与 -keyhex 只能二选一")
	case keyStr != "":
		key = []byte(keyStr)
	case keyHex != "":
		k, err := hex.DecodeString(strings.TrimSpace(keyHex))
		if err != nil {
			return nil, fmt.Errorf("-keyhex 不是合法十六进制: %v", err)
		}
		key = k
	default:
		return nil, fmt.Errorf("必须通过 -key 或 -keyhex 提供密钥")
	}
	if len(key) != wantLen {
		return nil, fmt.Errorf("密钥长度 %d 字节无效，必须为 %d 字节", len(key), wantLen)
	}
	return key, nil
}

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
		return make([]byte, sm4.BlockSize), nil
	}
}

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

// encodeOut 将结果按 outfmt 输出到终端或文件。
func encodeOut(result []byte, outfmt, out string, allowRaw bool) int {
	var payload []byte
	switch outfmt {
	case "hex":
		payload = []byte(hex.EncodeToString(result))
	case "base64":
		payload = []byte(base64.StdEncoding.EncodeToString(result))
	case "raw":
		if !allowRaw {
			fmt.Fprintln(os.Stderr, "该子命令不支持 raw 输出格式")
			return 2
		}
		payload = result
	default:
		fmt.Fprintf(os.Stderr, "不支持的输出格式: %s\n", outfmt)
		return 2
	}
	if out != "" {
		if err := os.WriteFile(out, payload, 0644); err != nil {
			fmt.Fprintln(os.Stderr, "写入文件失败:", err)
			return 1
		}
		fmt.Printf("已写入 %s（%d 字节）\n", out, len(payload))
		return 0
	}
	if outfmt == "raw" {
		os.Stdout.Write(payload)
		fmt.Println()
	} else {
		fmt.Println(string(payload))
	}
	return 0
}

// ---------- sm3 ----------

func runSM3(fs *flag.FlagSet, file string) int {
	data, err := readInput(fs, file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取输入失败:", err)
		return 1
	}
	fmt.Printf("%x\n", sm3.Sm3Sum(data))
	return 0
}

// ---------- sm4 ----------

func runSM4(fs *flag.FlagSet, o *options) int {
	o.mode = strings.ToLower(o.mode)
	if o.mode == "" {
		o.mode = "ecb"
	}
	if o.mode != "ecb" && o.mode != "cbc" {
		fmt.Fprintf(os.Stderr, "sm4 不支持的模式: %s（可选 ecb|cbc）\n", o.mode)
		return 2
	}
	o.outfmt = strings.ToLower(o.outfmt)
	if o.outfmt == "" {
		o.outfmt = "base64"
	}
	if o.outfmt != "base64" && o.outfmt != "hex" && o.outfmt != "raw" {
		fmt.Fprintf(os.Stderr, "不支持的输出格式: %s（可选 base64|hex|raw）\n", o.outfmt)
		return 2
	}
	if o.outfmt == "raw" && o.out == "" {
		fmt.Fprintln(os.Stderr, "raw 输出为二进制数据，必须配合 -o 指定输出文件")
		return 2
	}
	o.infmt = strings.ToLower(o.infmt)
	if o.infmt != "base64" && o.infmt != "hex" && o.infmt != "auto" {
		fmt.Fprintf(os.Stderr, "不支持的输入格式: %s（可选 base64|hex|raw|auto）\n", o.infmt)
		return 2
	}
	key, err := getKey(o.key, o.keyHex, sm4.BlockSize)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	block, err := sm4.NewCipher(key)
	if err != nil {
		fmt.Fprintln(os.Stderr, "初始化 SM4 失败:", err)
		return 1
	}
	bs := block.BlockSize()

	data, err := readInput(fs, o.file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取输入失败:", err)
		return 1
	}

	var result []byte
	if o.decrypt {
		ciphertext, err := decodeText(string(data), o.infmt)
		if err != nil {
			fmt.Fprintln(os.Stderr, "密文格式解析失败:", err)
			return 1
		}
		if len(ciphertext) == 0 || len(ciphertext)%bs != 0 {
			fmt.Fprintf(os.Stderr, "解密失败: 密文长度不是 %d 字节的整数倍\n", bs)
			return 1
		}
		switch o.mode {
		case "ecb":
			result, err = pkcs7Unpad(ecbCrypt(block, ciphertext, false), bs)
		case "cbc":
			iv, e := getIV(o.iv, o.ivHex)
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
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "解密失败:", err)
			return 1
		}
	} else {
		switch o.mode {
		case "ecb":
			result = ecbCrypt(block, pkcs7Pad(data, bs), true)
		case "cbc":
			iv, e := getIV(o.iv, o.ivHex)
			if e != nil {
				fmt.Fprintln(os.Stderr, e)
				return 2
			}
			if len(iv) != bs {
				fmt.Fprintf(os.Stderr, "IV 长度 %d 字节无效，必须为 %d 字节\n", len(iv), bs)
				return 2
			}
			padded := pkcs7Pad(data, bs)
			result = make([]byte, len(padded))
			cipher.NewCBCEncrypter(block, iv).CryptBlocks(result, padded)
		}
	}
	if o.decrypt {
		// 解密输出原始明文，不做编码
		if o.out != "" {
			if err := os.WriteFile(o.out, result, 0644); err != nil {
				fmt.Fprintln(os.Stderr, "写入文件失败:", err)
				return 1
			}
			fmt.Fprintf(os.Stderr, "已写入 %s（%d 字节）\n", o.out, len(result))
			return 0
		}
		os.Stdout.Write(result)
		fmt.Println()
		return 0
	}
	return encodeOut(result, o.outfmt, o.out, true)
}

// ---------- sm2 ----------

// parsePubHex 解析十六进制公钥（04||X||Y 非压缩 65 字节，或压缩 33 字节）。
func parsePubHex(pubHex string) (*sm2.PublicKey, error) {
	b, err := hex.DecodeString(strings.TrimSpace(pubHex))
	if err != nil {
		return nil, fmt.Errorf("-pubhex 不是合法十六进制: %v", err)
	}
	switch {
	case len(b) == 65 && b[0] == 0x04:
		pub := &sm2.PublicKey{
			Curve: sm2.P256Sm2(),
			X:     new(big.Int).SetBytes(b[1:33]),
			Y:     new(big.Int).SetBytes(b[33:65]),
		}
		if !pub.Curve.IsOnCurve(pub.X, pub.Y) {
			return nil, fmt.Errorf("公钥坐标不在 SM2 曲线上")
		}
		return pub, nil
	case len(b) == 33 && (b[0] == 0x02 || b[0] == 0x03):
		return sm2.Decompress(b), nil
	default:
		return nil, fmt.Errorf("公钥格式无效：应为 65 字节非压缩（04||X||Y）或 33 字节压缩格式")
	}
}

// parsePrivHex 解析十六进制私钥（标量 D）。
func parsePrivHex(privHex string) (*sm2.PrivateKey, error) {
	b, err := hex.DecodeString(strings.TrimSpace(privHex))
	if err != nil {
		return nil, fmt.Errorf("-privhex 不是合法十六进制: %v", err)
	}
	d := new(big.Int).SetBytes(b)
	if d.Sign() == 0 {
		return nil, fmt.Errorf("私钥不能为 0")
	}
	priv := new(sm2.PrivateKey)
	priv.Curve = sm2.P256Sm2()
	if d.Cmp(priv.Curve.Params().N) >= 0 {
		return nil, fmt.Errorf("私钥超出曲线阶范围")
	}
	priv.D = d
	priv.X, priv.Y = priv.Curve.ScalarBaseMult(d.Bytes())
	return priv, nil
}

// sm2Mode 解析 -mode 为 gmsm 密文排列常量。
func sm2Mode(mode string) (int, error) {
	switch strings.ToLower(mode) {
	case "", "c1c3c2":
		return sm2.C1C3C2, nil
	case "c1c2c3":
		return sm2.C1C2C3, nil
	default:
		return 0, fmt.Errorf("sm2 不支持的模式: %s（可选 c1c3c2|c1c2c3）", mode)
	}
}

func runSM2Keygen() int {
	priv, err := sm2.GenerateKey(rand.Reader)
	if err != nil {
		fmt.Fprintln(os.Stderr, "生成密钥失败:", err)
		return 1
	}
	d := make([]byte, 32)
	priv.D.FillBytes(d)
	x := make([]byte, 32)
	y := make([]byte, 32)
	priv.X.FillBytes(x)
	priv.Y.FillBytes(y)
	fmt.Printf("私钥: %x\n", d)
	fmt.Printf("公钥: 04%x%x\n", x, y)
	return 0
}

func runSM2Encrypt(fs *flag.FlagSet, o *options) int {
	if o.pubHex == "" {
		fmt.Fprintln(os.Stderr, "缺少 -pubhex 公钥")
		return 2
	}
	pub, err := parsePubHex(o.pubHex)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	mode, err := sm2Mode(o.mode)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	data, err := readInput(fs, o.file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取输入失败:", err)
		return 1
	}
	ct, err := sm2.Encrypt(pub, data, rand.Reader, mode)
	if err != nil {
		fmt.Fprintln(os.Stderr, "加密失败:", err)
		return 1
	}
	outfmt := strings.ToLower(o.outfmt)
	if outfmt == "" {
		outfmt = "hex"
	}
	return encodeOut(ct, outfmt, o.out, false)
}

func runSM2Decrypt(fs *flag.FlagSet, o *options) int {
	if o.privHex == "" {
		fmt.Fprintln(os.Stderr, "缺少 -privhex 私钥")
		return 2
	}
	priv, err := parsePrivHex(o.privHex)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	mode, err := sm2Mode(o.mode)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	data, err := readInput(fs, o.file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取输入失败:", err)
		return 1
	}
	ct, err := decodeText(string(data), strings.ToLower(o.infmt))
	if err != nil {
		fmt.Fprintln(os.Stderr, "密文格式解析失败:", err)
		return 1
	}
	plain, err := sm2.Decrypt(priv, ct, mode)
	if err != nil {
		fmt.Fprintln(os.Stderr, "解密失败:", err)
		return 1
	}
	os.Stdout.Write(plain)
	if len(plain) > 0 && plain[len(plain)-1] != '\n' {
		fmt.Println()
	}
	return 0
}

func runSM2Sign(fs *flag.FlagSet, o *options) int {
	if o.privHex == "" {
		fmt.Fprintln(os.Stderr, "缺少 -privhex 私钥")
		return 2
	}
	priv, err := parsePrivHex(o.privHex)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	data, err := readInput(fs, o.file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取输入失败:", err)
		return 1
	}
	r, s, err := sm2.Sm2Sign(priv, data, []byte(o.uid), rand.Reader)
	if err != nil {
		fmt.Fprintln(os.Stderr, "签名失败:", err)
		return 1
	}
	rb := make([]byte, 32)
	sb := make([]byte, 32)
	r.FillBytes(rb)
	s.FillBytes(sb)
	outfmt := strings.ToLower(o.outfmt)
	switch outfmt {
	case "", "hex":
		fmt.Printf("%x%x\n", rb, sb)
	case "base64":
		fmt.Println(base64.StdEncoding.EncodeToString(append(rb, sb...)))
	default:
		fmt.Fprintf(os.Stderr, "不支持的输出格式: %s（可选 hex|base64）\n", outfmt)
		return 2
	}
	return 0
}

func runSM2Verify(fs *flag.FlagSet, o *options) int {
	if o.pubHex == "" {
		fmt.Fprintln(os.Stderr, "缺少 -pubhex 公钥")
		return 2
	}
	if o.sigHex == "" {
		fmt.Fprintln(os.Stderr, "缺少 -sighex 签名")
		return 2
	}
	pub, err := parsePubHex(o.pubHex)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	sig, err := decodeText(o.sigHex, "hex")
	if err != nil {
		fmt.Fprintln(os.Stderr, "-sighex 不是合法十六进制:", err)
		return 2
	}
	if len(sig) != 64 {
		fmt.Fprintf(os.Stderr, "签名长度 %d 字节无效，应为 r||s 共 64 字节（128 个十六进制字符）\n", len(sig))
		return 2
	}
	data, err := readInput(fs, o.file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取输入失败:", err)
		return 1
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if sm2.Sm2Verify(pub, data, []byte(o.uid), r, s) {
		fmt.Println("签名有效")
		return 0
	}
	fmt.Fprintln(os.Stderr, "签名无效")
	return 1
}

// valueFlags 记录需要取值的旗标名（用于在解析前定位位置参数）。
var valueFlags = map[string]bool{
	"file": true, "mode": true, "key": true, "keyhex": true, "iv": true,
	"ivhex": true, "outfmt": true, "infmt": true, "o": true,
	"privhex": true, "pubhex": true, "sighex": true, "uid": true,
}

// splitArgs 从参数中提取前 n 个位置参数（自动跳过旗标及其取值），
// 返回提取出的位置参数与剩余参数。用于子命令式工具：
// Go 的 flag 包遇到首个非旗标参数即停止解析，因此需先剥离子命令。
func splitArgs(args []string, n int) (pos []string, rest []string) {
	rest = []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" { // 其后的参数全部按位置参数处理
			for j := i + 1; j < len(args); j++ {
				if len(pos) < n {
					pos = append(pos, args[j])
				} else {
					rest = append(rest, args[j])
				}
			}
			return pos, rest
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			name := strings.ToLower(strings.TrimLeft(a, "-"))
			if !strings.Contains(name, "=") && valueFlags[name] && i+1 < len(args) {
				rest = append(rest, a, args[i+1])
				i++
				continue
			}
			rest = append(rest, a)
			continue
		}
		if len(pos) < n {
			pos = append(pos, a)
			continue
		}
		rest = append(rest, a)
	}
	return pos, rest
}

func runSM2(fs *flag.FlagSet, o *options, op string) int {
	switch strings.ToLower(op) {
	case "keygen":
		return runSM2Keygen()
	case "encrypt":
		return runSM2Encrypt(fs, o)
	case "decrypt":
		return runSM2Decrypt(fs, o)
	case "sign":
		return runSM2Sign(fs, o)
	case "verify":
		return runSM2Verify(fs, o)
	default:
		if op == "" {
			fmt.Fprintln(os.Stderr, "缺少 sm2 操作（keygen|encrypt|decrypt|sign|verify）")
		} else {
			fmt.Fprintf(os.Stderr, "未知 sm2 操作: %s（可选 keygen|encrypt|decrypt|sign|verify）\n", op)
		}
		return 2
	}
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	o := &options{}
	fs.StringVar(&o.mode, "mode", "", "sm4: ecb|cbc（默认 ecb）；sm2: c1c3c2|c1c2c3（默认 c1c3c2）")
	fs.StringVar(&o.key, "key", "", "UTF-8 文本密钥")
	fs.StringVar(&o.keyHex, "keyhex", "", "十六进制密钥")
	fs.StringVar(&o.iv, "iv", "", "UTF-8 初始向量")
	fs.StringVar(&o.ivHex, "ivhex", "", "十六进制初始向量")
	fs.StringVar(&o.file, "file", "", "输入文件")
	fs.StringVar(&o.outfmt, "outfmt", "", "输出格式（sm4 默认 base64，sm2 默认 hex）")
	fs.StringVar(&o.infmt, "infmt", "auto", "解密输入格式")
	fs.StringVar(&o.out, "o", "", "输出文件")
	fs.StringVar(&o.privHex, "privhex", "", "SM2 私钥（十六进制）")
	fs.StringVar(&o.pubHex, "pubhex", "", "SM2 公钥（十六进制）")
	fs.StringVar(&o.sigHex, "sighex", "", "SM2 签名（r||s 十六进制）")
	fs.StringVar(&o.uid, "uid", "1234567812345678", "SM2 签名用户 ID")
	fs.BoolVar(&o.decrypt, "d", false, "解密模式")
	fs.Usage = func() { fmt.Print(Usage + "\n") }

	subPos, rest := splitArgs(args, 1)
	sub := ""
	if len(subPos) > 0 {
		sub = subPos[0]
	}
	if err := fs.Parse(rest); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	switch strings.ToLower(sub) {
	case "sm3":
		return runSM3(fs, o.file)
	case "sm4":
		return runSM4(fs, o)
	case "sm2":
		opPos, rest2 := splitArgs(fs.Args(), 1)
		op := ""
		if len(opPos) > 0 {
			op = opPos[0]
		}
		if err := fs.Parse(rest2); err != nil {
			if err == flag.ErrHelp {
				return 0
			}
			return 2
		}
		return runSM2(fs, o, op)
	default:
		if sub == "" {
			fmt.Fprintln(os.Stderr, "缺少子命令（sm3|sm4|sm2）")
		} else {
			fmt.Fprintf(os.Stderr, "未知子命令: %s（可选 sm3|sm4|sm2）\n", sub)
		}
		return 2
	}
}
