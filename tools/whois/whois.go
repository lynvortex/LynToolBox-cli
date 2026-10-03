// Package whois 实现 WHOIS 查询命令。
// 对应网页版：network/whois-state-tool.html（网站注册状态与WHOIS查询）
package whois

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const (
	Name  = "whois"
	Desc  = "域名 WHOIS 注册信息查询（原生 43 端口协议）"
	Usage = `用法: lyntoolbox whois 域名 [-clean]

参数:
  -clean  过滤注释与空行，并摘要常用字段`
)

// 直连注册局表（常用 TLD）
var tldServers = map[string]string{
	"com": "whois.verisign-grs.com", "net": "whois.verisign-grs.com",
	"org": "whois.pir.org", "cn": "whois.cnnic.cn",
	"info": "whois.afilias.net", "biz": "whois.nic.biz",
	"top": "whois.nic.top", "xyz": "whois.nic.xyz",
	"io": "whois.nic.io", "me": "whois.nic.me",
	"cc": "whois.nic.cc", "tv": "whois.nic.tv",
	"jp": "whois.jprs.jp", "kr": "whois.kr",
	"edu": "whois.educause.edu", "gov": "whois.dotgov.gov",
}

func query(server, domain string) (string, error) {
	conn, err := net.DialTimeout("tcp", server+":43", 15*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_, _ = conn.Write([]byte(domain + "\r\n"))
	var b strings.Builder
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	deadline := time.Now().Add(20 * time.Second)
	if err := conn.SetReadDeadline(deadline); err != nil {
		return "", err
	}
	for scanner.Scan() {
		b.WriteString(scanner.Text())
		b.WriteString("\n")
	}
	if err := scanner.Err(); err != nil {
		return b.String(), fmt.Errorf("读取响应中断: %w", err)
	}
	return b.String(), nil
}

// referral 从 IANA/注册局响应中提取指向的 whois 服务器。
func referral(resp string) string {
	for _, line := range strings.Split(resp, "\n") {
		l := strings.ToLower(line)
		if strings.HasPrefix(l, "refer:") || strings.HasPrefix(l, "whois server:") || strings.HasPrefix(l, "registrar whois server:") {
			if i := strings.Index(line, ":"); i >= 0 {
				v := strings.TrimSpace(line[i+1:])
				if v != "" {
					return v
				}
			}
		}
	}
	return ""
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	clean := fs.Bool("clean", false, "过滤注释并摘要")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox whois 域名")
		return 2
	}
	domain := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(fs.Arg(0), "https://"), "http://"))
	if i := strings.Index(domain, "/"); i >= 0 {
		domain = domain[:i]
	}
	if domain == "" || strings.ContainsAny(domain, "\r\n\t ") {
		fmt.Fprintln(os.Stderr, "错误: 无效的域名")
		return 2
	}
	parts := strings.Split(domain, ".")
	tld := strings.ToLower(parts[len(parts)-1])

	server, ok := tldServers[tld]
	if !ok {
		server = "whois.iana.org"
	}

	resp, err := query(server, domain)
	if err != nil {
		fmt.Fprintf(os.Stderr, "查询 %s 失败: %v\n", server, err)
		return 1
	}
	if ref := referral(resp); ref != "" && ref != server {
		if r2, err2 := query(ref, domain); err2 == nil && strings.TrimSpace(r2) != "" {
			resp = r2
			server = ref
		}
	}

	if !*clean {
		fmt.Printf("# 查询服务器: %s\n\n", server)
		fmt.Print(resp)
		return 0
	}

	fmt.Printf("查询服务器: %s\n\n", server)
	keys := []string{"domain name", "registrar:", "registrar whois", "creation date", "created", "registry expiry", "expiry", "updated date", "name server", "status", "registrant"}
	seen := map[string]bool{}
	for _, line := range strings.Split(resp, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "%") || strings.HasPrefix(t, "#") || strings.HasPrefix(t, ">") {
			continue
		}
		low := strings.ToLower(t)
		matched := false
		for _, k := range keys {
			if strings.HasPrefix(low, k) && !seen[low] {
				seen[low] = true
				matched = true
				break
			}
		}
		if matched {
			fmt.Println(t)
		}
	}
	if strings.TrimSpace(resp) == "" {
		fmt.Println("（响应为空，该域名可能未注册或注册局不支持查询）")
	}
	return 0
}
