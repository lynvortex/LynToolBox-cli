// Package sslcert 实现 SSL 证书查询命令。
// 对应网页版：network/html-ssl-tool.html（网站SSL证书查询）
package sslcert

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const (
	Name  = "sslcert"
	Desc  = "查询网站 SSL/TLS 证书详情与剩余有效期"
	Usage = `用法: lyntoolbox sslcert 域名 [-port 443] [-k]

参数:
  -port   TLS 端口（默认 443）
  -k      跳过证书链校验（自签名/过期证书仍可查看）`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	port := fs.Int("port", 443, "TLS 端口")
	insecure := fs.Bool("k", false, "跳过证书校验")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox sslcert 域名")
		return 2
	}
	host := strings.TrimSpace(fs.Arg(0))
	host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	if i := strings.Index(host, "/"); i >= 0 {
		host = host[:i]
	}
	if i := strings.Index(host, ":"); i >= 0 {
		host = host[:i]
	}
	addr := fmt.Sprintf("%s:%d", host, *port)

	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 15 * time.Second}, "tcp", addr, &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: *insecure,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "TLS 连接失败:", err)
		return 1
	}
	defer conn.Close()

	cs := conn.ConnectionState()
	fmt.Printf("目标:       %s\n", addr)
	fmt.Printf("TLS 版本:   %s\n", tlsVersion(cs.Version))
	fmt.Printf("加密套件:   %s\n", tlsSuite(cs.CipherSuite))
	if len(cs.PeerCertificates) == 0 {
		fmt.Fprintln(os.Stderr, "服务器未返回证书")
		return 1
	}
	cert := cs.PeerCertificates[0]
	fmt.Printf("主题 (CN):  %s\n", cert.Subject.CommonName)
	if len(cert.Subject.Organization) > 0 {
		fmt.Printf("颁发对象:   %s\n", strings.Join(cert.Subject.Organization, ", "))
	}
	fmt.Printf("颁发者:     %s\n", cert.Issuer.CommonName)
	if len(cert.DNSNames) > 0 {
		sans := cert.DNSNames
		if len(sans) > 10 {
			fmt.Printf("SAN 列表:   %s 等共 %d 条\n", strings.Join(sans[:10], ", "), len(sans))
		} else {
			fmt.Printf("SAN 列表:   %s\n", strings.Join(sans, ", "))
		}
	}
	fmt.Printf("序列号:     %s\n", cert.SerialNumber.Text(16))
	fmt.Printf("签名算法:   %s\n", cert.SignatureAlgorithm.String())
	fmt.Printf("生效时间:   %s\n", cert.NotBefore.Format("2006-01-02 15:04:05"))
	fmt.Printf("过期时间:   %s\n", cert.NotAfter.Format("2006-01-02 15:04:05"))
	remain := time.Until(cert.NotAfter)
	days := int(remain.Hours() / 24)
	if remain < 0 {
		fmt.Printf("剩余有效期: 已过期 %.1f 天！\n", -remain.Hours()/24)
		return 1
	}
	warn := ""
	if days < 30 {
		warn = "  ⚠ 即将过期，请尽快续签"
	}
	fmt.Printf("剩余有效期: %d 天%s\n", days, warn)
	if *insecure {
		fmt.Println("注意:       未校验证书链（-k）")
	}
	return 0
}

func tlsVersion(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	}
	return fmt.Sprintf("0x%04x", v)
}

func tlsSuite(c uint16) string {
	return tls.CipherSuiteName(c)
}
