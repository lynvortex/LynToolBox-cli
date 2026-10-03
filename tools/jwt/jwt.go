// Package jwt 实现 JWT（JSON Web Token）解码与 HS256/384/512 验签命令。
// 对应网页版：work/jwt-tool.html（JWT解码）
//
// 用法：
//
//	lyntoolbox jwt <token>
//	lyntoolbox jwt -exp <token>
//	lyntoolbox jwt -key 秘密 <token>
package jwt

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"hash"
	"io"
	"os"
	"strings"
	"time"
)

const (
	Name  = "jwt"
	Desc  = "JWT 解码（格式化 header/payload）、exp 过期检查与 HS256/384/512 验签"
	Usage = `用法: lyntoolbox jwt [-exp] [-key 秘密] [token]

参数:
  -exp   检查 payload 中的 exp 声明，输出剩余/超时秒数
  -key   对 HS256/HS384/HS512 做 HMAC 签名比对验证（提供即验证）
  token  待解析的 JWT；省略时从 stdin 读取

说明:
  解码仅为 Base64 还原，不校验签名；需要验证时请配合 -key。
  验签失败退出码为 1。`
)

// decodeSegment 解码 JWT 的 base64url 段（容忍带填充）。
func decodeSegment(seg string) ([]byte, error) {
	if b, err := base64.RawURLEncoding.DecodeString(seg); err == nil {
		return b, nil
	}
	return base64.URLEncoding.DecodeString(seg)
}

// prettyJSON 将解码后的 JSON 缩进输出，保留原有键顺序。
func prettyJSON(raw []byte) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		// 不是合法 JSON 时原样输出
		return string(raw)
	}
	return buf.String()
}

// checkExp 检查 payload 的 exp 声明，返回提示文本。
func checkExp(payload []byte) (string, error) {
	var claims map[string]any
	dec := json.NewDecoder(strings.NewReader(string(payload)))
	dec.UseNumber()
	if err := dec.Decode(&claims); err != nil {
		return "", fmt.Errorf("payload 不是合法 JSON")
	}
	v, ok := claims["exp"]
	if !ok {
		return "", fmt.Errorf("payload 中没有 exp 声明")
	}
	var exp int64
	switch n := v.(type) {
	case json.Number:
		var err error
		if exp, err = n.Int64(); err != nil {
			return "", fmt.Errorf("exp 声明不是整数: %s", n.String())
		}
	case string:
		var err error
		if _, err = fmt.Sscanf(n, "%d", &exp); err != nil {
			return "", fmt.Errorf("exp 声明不是整数: %s", n)
		}
	default:
		return "", fmt.Errorf("exp 声明类型无效")
	}
	now := time.Now().Unix()
	remain := exp - now
	expTime := time.Unix(exp, 0).Format("2006-01-02 15:04:05")
	if remain >= 0 {
		return fmt.Sprintf("exp 未过期，剩余 %d 秒（到期时间 %s）", remain, expTime), nil
	}
	return fmt.Sprintf("exp 已过期 %d 秒（到期时间 %s）", -remain, expTime), nil
}

// verifyHS 用密钥对 HS256/HS384/HS512 做签名比对。
func verifyHS(alg, signingInput string, sig []byte, secret []byte) bool {
	var h func() hash.Hash
	switch alg {
	case "HS256":
		h = sha256.New
	case "HS384":
		h = sha512.New384
	case "HS512":
		h = sha512.New
	default:
		return false
	}
	mac := hmac.New(h, secret)
	mac.Write([]byte(signingInput))
	return hmac.Equal(mac.Sum(nil), sig)
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	checkExpFlag := fs.Bool("exp", false, "检查 exp 是否过期")
	key := fs.String("key", "", "验签密钥（HS256/HS384/HS512）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	var token string
	if fs.NArg() > 0 {
		token = strings.Join(fs.Args(), " ")
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 stdin 失败:", err)
			return 1
		}
		token = string(b)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		fmt.Fprintln(os.Stderr, "缺少 token 参数")
		return 2
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		fmt.Fprintln(os.Stderr, "token 格式无效：应为 header.payload.signature 三段")
		return 1
	}

	headerRaw, err := decodeSegment(parts[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "header 解码失败:", err)
		return 1
	}
	payloadRaw, err := decodeSegment(parts[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "payload 解码失败:", err)
		return 1
	}
	sigRaw, err := decodeSegment(parts[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, "signature 解码失败:", err)
		return 1
	}

	var header struct {
		Alg string `json:"alg"`
	}
	if err := json.Unmarshal(headerRaw, &header); err != nil {
		fmt.Fprintln(os.Stderr, "header 不是合法 JSON:", err)
		return 1
	}

	fmt.Println("== header ==")
	fmt.Println(prettyJSON(headerRaw))
	fmt.Println("== payload ==")
	fmt.Println(prettyJSON(payloadRaw))

	if *checkExpFlag {
		msg, err := checkExp(payloadRaw)
		if err != nil {
			fmt.Fprintln(os.Stderr, "exp 检查失败:", err)
			return 1
		}
		fmt.Println("== exp ==")
		fmt.Println(msg)
	}

	if *key == "" {
		fmt.Fprintln(os.Stderr, "提示: 未验证签名（如需验证请使用 -key 指定密钥）")
		return 0
	}

	if header.Alg != "HS256" && header.Alg != "HS384" && header.Alg != "HS512" {
		fmt.Fprintf(os.Stderr, "不支持的签名算法: %q（仅支持 HS256/HS384/HS512）\n", header.Alg)
		return 1
	}
	ok := verifyHS(header.Alg, parts[0]+"."+parts[1], sigRaw, []byte(*key))
	if ok {
		fmt.Println("== 签名 ==")
		fmt.Println("签名验证通过")
		return 0
	}
	fmt.Fprintln(os.Stderr, "签名验证失败（密钥错误或 token 被篡改）")
	return 1
}
