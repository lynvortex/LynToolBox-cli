// Package stegano 实现 PNG 图片 LSB 隐写命令（隐藏/提取文字或文件）。
// 对应网页版：image/steganography-tool.html（图片隐写）
//
// 用法：
//
//	lyntoolbox stegano hide 载体.png [-msg 文本 | -file 附件] [-pass 密码] [-o 输出.png]
//	lyntoolbox stegano extract 载体.png [-pass 密码] [-o 输出文件]
package stegano

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	Name  = "stegano"
	Desc  = "PNG 图片 LSB 隐写：隐藏与提取文字/文件，支持口令加密"
	Usage = `用法:
  lyntoolbox stegano hide 载体图片 [-msg 文本 | -file 附件路径] [-pass 密码] [-o 输出.png]
  lyntoolbox stegano extract 载体图片 [-pass 密码] [-o 输出文件]

说明:
  载体建议使用 PNG（无损）；隐写数据写入像素 RGB 通道最低位（LSB），
  格式为 32 位长度头 + 数据字节，容量 = 宽 × 高 × 3 字节。
  -pass 指定时以 PBKDF2-HMAC-SHA256（21 万次迭代）派生密钥，
  用 AES-256-GCM 加密后嵌入（体部开销 48 字节）。
  旧版（sha256 密钥流 XOR）隐写图仍可正常提取，但强度不足，建议重新加密。`
)

const headerBytes = 4 // 32 位长度头

// v2 加密格式常量：头为明文长度，体部 = LTS2 魔数 + 盐 + nonce + AES-256-GCM 密文
const (
	v2Magic   = "LTS2"
	v2SaltLen = 16
	v2Iter    = 210000
)

// deriveKeystream 旧版（v1）格式以 sha256(pass) 为密钥生成密钥流（SHA256(key||counter) 级联）。
// 仅用于兼容读取旧隐写图；v1 无盐无认证，已知明文即可恢复密钥流，不应再用于写入。
func deriveKeystream(pass string, n int) []byte {
	key := sha256.Sum256([]byte(pass))
	out := make([]byte, 0, n)
	var counter uint32
	for len(out) < n {
		h := sha256.New()
		h.Write(key[:])
		var cb [4]byte
		binary.BigEndian.PutUint32(cb[:], counter)
		h.Write(cb[:])
		out = h.Sum(out)
		counter++
	}
	return out[:n]
}

// pbkdf2Key 按 RFC 8018 以 HMAC-SHA256 派生密钥（自实现，保持零第三方依赖）。
func pbkdf2Key(pass string, salt []byte, keyLen, iter int) []byte {
	prf := hmac.New(sha256.New, []byte(pass))
	hashLen := prf.Size()
	numBlocks := (keyLen + hashLen - 1) / hashLen
	out := make([]byte, 0, numBlocks*hashLen)
	var u, t []byte
	buf := make([]byte, 4)
	for block := 1; block <= numBlocks; block++ {
		prf.Reset()
		prf.Write(salt)
		binary.BigEndian.PutUint32(buf, uint32(block))
		prf.Write(buf)
		u = prf.Sum(u[:0])
		t = append(t[:0], u...)
		for i := 1; i < iter; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(u[:0])
			for j := range u {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:keyLen]
}

// encryptWithPass 以随机盐 + PBKDF2-HMAC-SHA256 派生密钥，AES-256-GCM 加密，
// 返回 v2 体部（LTS2 + salt + nonce + 密文，含认证标签）。
func encryptWithPass(payload []byte, pass string) ([]byte, error) {
	salt := make([]byte, v2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key := pbkdf2Key(pass, salt, 32, v2Iter)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	body := make([]byte, 0, len(v2Magic)+v2SaltLen+gcm.NonceSize()+len(payload)+gcm.Overhead())
	body = append(body, v2Magic...)
	body = append(body, salt...)
	body = append(body, nonce...)
	return gcm.Seal(body, nonce, payload, nil), nil
}

// decryptWithPass 解析并解密 v2 体部；口令错误时 GCM 认证失败返回错误。
func decryptWithPass(body []byte, pass string) ([]byte, error) {
	off := 0
	need := len(v2Magic) + v2SaltLen + 12 + 16 // 魔数+盐+nonce+最短GCM密文(纯标签)
	if len(body) < need {
		return nil, errors.New("加密数据过短，可能已损坏")
	}
	if string(body[off:len(v2Magic)]) != v2Magic {
		return nil, errors.New("加密格式魔数不符")
	}
	off += len(v2Magic)
	salt := body[off : off+v2SaltLen]
	off += v2SaltLen
	key := pbkdf2Key(pass, salt, 32, v2Iter)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := body[off : off+gcm.NonceSize()]
	off += gcm.NonceSize()
	plain, err := gcm.Open(nil, nonce, body[off:], nil)
	if err != nil {
		return nil, errors.New("解密失败：口令不正确或数据已损坏")
	}
	return plain, nil
}

// embedData 将数据逐位写入 NRGBA 像素 RGB 通道的最低位（MSB 先行，行主序）。
func embedData(img *image.NRGBA, data []byte) error {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	capacity := w * h * 3
	if len(data) > capacity {
		return fmt.Errorf("数据 %d 字节超出载体容量 %d 字节（%dx%d×3）", len(data), capacity, w, h)
	}
	bitIdx := 0
	for _, db := range data {
		for bit := 7; bit >= 0; bit-- {
			pix := bitIdx / 3
			ch := bitIdx % 3
			px, py := pix%w, pix/w
			i := img.PixOffset(b.Min.X+px, b.Min.Y+py)
			if db&(1<<uint(bit)) != 0 {
				img.Pix[i+ch] |= 1
			} else {
				img.Pix[i+ch] &^= 1
			}
			bitIdx++
		}
	}
	return nil
}

// readLSBBytes 从像素 LSB 顺序读取前 n 字节（MSB 先行，行主序 RGB 通道）。
func readLSBBytes(img *image.NRGBA, n int) []byte {
	b := img.Bounds()
	w := b.Dx()
	out := make([]byte, n)
	bitIdx := 0
	for bi := 0; bi < n*8; bi++ {
		pix := bitIdx / 3
		ch := bitIdx % 3
		px, py := pix%w, pix/w
		i := img.PixOffset(b.Min.X+px, b.Min.Y+py)
		if img.Pix[i+ch]&1 != 0 {
			out[bi/8] |= 1 << uint(7-bi%8)
		}
		bitIdx++
	}
	return out
}

// extractPayload 提取数据，按格式分派：
//   - v2（-pass 写入）：头为明文长度，体部以 LTS2 开头，AES-256-GCM 解密；
//   - v1 无口令：头与体部均为明文；
//   - v1 有口令：头与体部整体被旧密钥流 XOR 加密，头部非法时按此路径解。
func extractPayload(img *image.NRGBA, pass string) ([]byte, error) {
	b := img.Bounds()
	capacity := b.Dx()*b.Dy()*3 - headerBytes
	if capacity < 1 {
		return nil, errors.New("图片太小无法提取")
	}
	head := readLSBBytes(img, headerBytes)
	length := binary.BigEndian.Uint32(head)
	if length > 0 && int64(length) <= int64(capacity) {
		// readLSBBytes 从图首开始读，raw = 长度头 + 体部，须剥掉头部
		raw := readLSBBytes(img, headerBytes+int(length))
		body := raw[headerBytes:]
		if len(body) >= len(v2Magic) && string(body[:len(v2Magic)]) == v2Magic {
			if pass == "" {
				return nil, errors.New("该隐写图使用口令加密，请用 -pass 提供口令")
			}
			return decryptWithPass(body, pass)
		}
		return body, nil
	}
	// 头部不是合法明文长度：v1 口令加密（头也被加密）或不是隐写图
	if pass == "" {
		return nil, fmt.Errorf("未检测到有效嵌入数据（长度头 %d 非法）", length)
	}
	ksHead := deriveKeystream(pass, headerBytes)
	legHead := make([]byte, headerBytes)
	for i := range head {
		legHead[i] = head[i] ^ ksHead[i]
	}
	legLen := binary.BigEndian.Uint32(legHead)
	if legLen == 0 || int64(legLen) > int64(capacity) {
		return nil, errors.New("长度头非法，口令可能不正确或不是隐写图")
	}
	raw := readLSBBytes(img, headerBytes+int(legLen))
	ks := deriveKeystream(pass, headerBytes+int(legLen))
	for i := range raw {
		raw[i] ^= ks[i]
	}
	return raw[headerBytes:], nil
}

func loadImage(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	return image.Decode(f)
}

// reorderArgs 将已知选项移到最前，使文件参数可以放在任意位置。
func reorderArgs(args []string, bools, known map[string]bool) []string {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		tok := args[i]
		name := strings.TrimLeft(tok, "-")
		if len(tok) > 1 && tok[0] == '-' && known[name] {
			flags = append(flags, tok)
			if !bools[name] && i+1 < len(args) && !known[strings.TrimLeft(args[i+1], "-")] {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		rest = append(rest, tok)
	}
	return append(flags, rest...)
}

var (
	knownFlags = map[string]bool{"msg": true, "file": true, "pass": true, "o": true}
	boolFlags  = map[string]bool{}
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, "错误: 缺少子命令 hide 或 extract\n\n")
		fmt.Fprint(os.Stderr, Usage+"\n")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "hide":
		return runHide(rest)
	case "extract":
		return runExtract(rest)
	default:
		fmt.Fprintf(os.Stderr, "错误: 未知子命令 %q（可用: hide, extract）\n\n", sub)
		fmt.Fprint(os.Stderr, Usage+"\n")
		return 2
	}
}

func runHide(args []string) int {
	fs := flag.NewFlagSet("hide", flag.ContinueOnError)
	msg := fs.String("msg", "", "要隐藏的文本")
	file := fs.String("file", "", "要隐藏的文件路径")
	pass := fs.String("pass", "", "加密口令（可选）")
	out := fs.String("o", "", "输出 PNG 路径（默认原名加 .steg.png）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	args = reorderArgs(args, boolFlags, knownFlags)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	carrier := fs.Arg(0)
	if carrier == "" {
		fmt.Fprintln(os.Stderr, "错误: 需要指定载体图片")
		return 2
	}
	if *msg == "" && *file == "" {
		fmt.Fprintln(os.Stderr, "错误: 需要 -msg 或 -file 指定要隐藏的数据")
		return 2
	}
	if *msg != "" && *file != "" {
		fmt.Fprintln(os.Stderr, "错误: -msg 与 -file 只能二选一")
		return 2
	}

	var payload []byte
	if *msg != "" {
		payload = []byte(*msg)
	} else {
		b, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取附件失败:", err)
			return 1
		}
		payload = b
	}

	img, format, err := loadImage(carrier)
	if err != nil {
		fmt.Fprintln(os.Stderr, "解码载体失败:", err)
		return 1
	}
	if format != "png" {
		fmt.Fprintf(os.Stderr, "警告: 载体为 %s 格式，输出将转为 PNG（建议直接使用 PNG 载体）\n", format)
	}

	// 转为可写 NRGBA
	b := img.Bounds()
	nrgba := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(nrgba, nrgba.Bounds(), img, b.Min, draw.Src)

	var data []byte
	if *pass != "" {
		body, err := encryptWithPass(payload, *pass)
		if err != nil {
			fmt.Fprintln(os.Stderr, "加密失败:", err)
			return 1
		}
		data = make([]byte, headerBytes+len(body))
		binary.BigEndian.PutUint32(data[:headerBytes], uint32(len(body)))
		copy(data[headerBytes:], body)
	} else {
		data = make([]byte, headerBytes+len(payload))
		binary.BigEndian.PutUint32(data[:headerBytes], uint32(len(payload)))
		copy(data[headerBytes:], payload)
	}

	if err := embedData(nrgba, data); err != nil {
		fmt.Fprintln(os.Stderr, "错误:", err)
		return 1
	}

	dst := *out
	if dst == "" {
		stem := strings.TrimSuffix(filepath.Base(carrier), filepath.Ext(carrier))
		dst = filepath.Join(filepath.Dir(carrier), stem+".steg.png")
	}
	f, err := os.Create(dst)
	if err != nil {
		fmt.Fprintln(os.Stderr, "创建输出失败:", err)
		return 1
	}
	if err := png.Encode(f, nrgba); err != nil {
		f.Close()
		os.Remove(dst)
		fmt.Fprintln(os.Stderr, "编码 PNG 失败:", err)
		return 1
	}
	f.Close()

	capacity := b.Dx() * b.Dy() * 3
	fmt.Printf("已嵌入 %d 字节数据（容量 %d 字节，使用 %.2f%%）→ %s\n",
		len(payload), capacity, float64(len(data))/float64(capacity)*100, dst)
	return 0
}

func runExtract(args []string) int {
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	pass := fs.String("pass", "", "解密口令（与 hide 时一致）")
	out := fs.String("o", "", "提取结果写入文件（默认打印文本）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	args = reorderArgs(args, boolFlags, knownFlags)
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	carrier := fs.Arg(0)
	if carrier == "" {
		fmt.Fprintln(os.Stderr, "错误: 需要指定载体图片")
		return 2
	}
	img, format, err := loadImage(carrier)
	if err != nil {
		fmt.Fprintln(os.Stderr, "解码失败:", err)
		return 1
	}
	_ = format
	b := img.Bounds()
	nrgba := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(nrgba, nrgba.Bounds(), img, b.Min, draw.Src)

	data, err := extractPayload(nrgba, *pass)
	if err != nil {
		fmt.Fprintln(os.Stderr, "提取失败:", err)
		return 1
	}

	if *out != "" {
		if err := os.WriteFile(*out, data, 0644); err != nil {
			fmt.Fprintln(os.Stderr, "写入文件失败:", err)
			return 1
		}
		fmt.Printf("已提取 %d 字节 → %s\n", len(data), *out)
		return 0
	}
	if !utf8.Valid(data) {
		fmt.Fprintf(os.Stderr, "提示: 提取的 %d 字节不是有效文本（可能为二进制数据或口令错误），请用 -o 保存为文件\n", len(data))
		return 1
	}
	fmt.Printf("提取到 %d 字节:\n%s\n", len(data), string(data))
	return 0
}
