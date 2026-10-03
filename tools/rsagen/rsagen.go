// Package rsagen 实现 RSA 密钥对生成命令，输出 PEM 格式。
// 对应网页版：work/rsa-tool.html（RSA生成）
//
// 用法：
//
//	lyntoolbox rsagen
//	lyntoolbox rsagen -bits 4096 -pkcs8
//	lyntoolbox rsagen -o ./keys
package rsagen

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const (
	Name  = "rsagen"
	Desc  = "生成 RSA 密钥对，输出 PEM（PKCS#1/PKCS#8 私钥 + SPKI 公钥）"
	Usage = `用法: lyntoolbox rsagen [-bits 位数] [-pkcs8] [-o 目录]

参数:
  -bits   密钥位数 1024|2048|4096（默认 2048）
  -pkcs8  私钥使用 PKCS#8 格式（默认 PKCS#1）；公钥固定为 SPKI 格式
  -o      输出目录：写入 id_rsa / id_rsa.pub；省略时打印到终端`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	bits := fs.Int("bits", 2048, "密钥位数")
	usePKCS8 := fs.Bool("pkcs8", false, "私钥使用 PKCS#8 格式")
	outDir := fs.String("o", "", "输出目录")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *bits != 1024 && *bits != 2048 && *bits != 4096 {
		fmt.Fprintf(os.Stderr, "不支持的密钥位数: %d（可选 1024|2048|4096）\n", *bits)
		return 2
	}

	fmt.Fprintf(os.Stderr, "正在生成 %d 位 RSA 密钥对，请稍候...\n", *bits)
	key, err := rsa.GenerateKey(rand.Reader, *bits)
	if err != nil {
		fmt.Fprintln(os.Stderr, "生成密钥失败:", err)
		return 1
	}

	var privPEM *pem.Block
	if *usePKCS8 {
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			fmt.Fprintln(os.Stderr, "编码 PKCS#8 私钥失败:", err)
			return 1
		}
		privPEM = &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	} else {
		privPEM = &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, "编码公钥失败:", err)
		return 1
	}
	pubPEM := &pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}

	if *outDir != "" {
		if err := os.MkdirAll(*outDir, 0755); err != nil {
			fmt.Fprintln(os.Stderr, "创建输出目录失败:", err)
			return 1
		}
		privPath := filepath.Join(*outDir, "id_rsa")
		pubPath := filepath.Join(*outDir, "id_rsa.pub")
		if err := os.WriteFile(privPath, pem.EncodeToMemory(privPEM), 0600); err != nil {
			fmt.Fprintln(os.Stderr, "写入私钥文件失败:", err)
			return 1
		}
		if err := os.WriteFile(pubPath, pem.EncodeToMemory(pubPEM), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "写入公钥文件失败:", err)
			return 1
		}
		fmt.Printf("已写入 %s\n已写入 %s\n", privPath, pubPath)
		return 0
	}

	if err := pem.Encode(os.Stdout, privPEM); err != nil {
		fmt.Fprintln(os.Stderr, "输出私钥失败:", err)
		return 1
	}
	if err := pem.Encode(os.Stdout, pubPEM); err != nil {
		fmt.Fprintln(os.Stderr, "输出公钥失败:", err)
		return 1
	}
	return 0
}
