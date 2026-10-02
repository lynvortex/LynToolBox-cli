// Package dnsq 实现 DNS 查询命令。
// 对应网页版：network/dns-tool.html（DNS查询）、network/dns-doh-tool.html（DoH查询）
//
// 用法：
//
//	lyntoolbox dnsq baidu.com
//	lyntoolbox dnsq baidu.com -type MX
//	lyntoolbox dnsq baidu.com -doh            # Cloudflare DoH
//	lyntoolbox dnsq baidu.com -doh google     # Google DoH
package dnsq

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	Name  = "dnsq"
	Desc  = "DNS 记录查询，支持系统解析器与 DoH（Cloudflare/Google）"
	Usage = `用法: lyntoolbox dnsq 域名 [-type 类型] [-doh [cloudflare|google]]

参数:
  -type  记录类型: A|AAAA|CNAME|MX|NS|TXT|SRV（默认 A；-doh 模式额外支持 SOA|PTR|CAA）
  -doh   使用 DoH JSON API 查询；可带值 cloudflare（默认，1.1.1.1）、google（dns.google）
         或 alidns（dns.alidns.com，补充）；也可单独写 -doh 不带值
  域名   待查询的域名

说明: 默认走系统 DNS（net.Resolver，不返回 TTL，以 - 表示）；
-doh 模式经 HTTPS 查询，返回含 TTL 的完整 Answer。`
)

// dohTypeNum DNS 记录类型编号 -> 名称
var dohTypeNum = map[int]string{
	1: "A", 2: "NS", 5: "CNAME", 6: "SOA", 12: "PTR", 15: "MX",
	16: "TXT", 28: "AAAA", 33: "SRV", 257: "CAA", 65: "HTTPS",
}

// rec 统一的记录输出
type rec struct {
	name  string
	typ   string
	ttl   string
	value string
}

// dohProviders 已知 DoH 服务名（用于识别 "-doh google" 中跟随的服务名）
var dohProviders = map[string]bool{
	"cloudflare": true, "cf": true, "1.1.1.1": true,
	"google": true, "g": true, "alidns": true, "ali": true,
}

// normalizeDohArgs 把 "-doh google" / "-doh" 预处理为 "-doh=google" / "-doh=cloudflare"，
// 使 -doh 既能不带值（默认 cloudflare）又能带服务名。
func normalizeDohArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-doh" || a == "--doh" {
			if i+1 < len(args) && dohProviders[strings.ToLower(args[i+1])] {
				out = append(out, "-doh="+strings.ToLower(args[i+1]))
				i++
			} else {
				out = append(out, "-doh=cloudflare")
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

type dohResponse struct {
	Status int `json:"Status"`
	Answer []struct {
		Name string `json:"name"`
		Type int    `json:"type"`
		TTL  int    `json:"TTL"`
		Data string `json:"data"`
	} `json:"Answer"`
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	dtype := fs.String("type", "A", "记录类型")
	doh := fs.String("doh", "", "DoH 服务: cloudflare(默认)|google|alidns")
	fs.Usage = func() { fmt.Print(Usage + "\n") }

	rest := normalizeDohArgs(args)
	var positionals []string
	for {
		if err := fs.Parse(rest); err != nil {
			return 2
		}
		rem := fs.Args()
		if len(rem) == 0 {
			break
		}
		positionals = append(positionals, rem[0])
		rest = rem[1:]
	}
	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, "错误: 需要且只需要一个域名参数")
		fmt.Fprintln(os.Stderr, "（lyntoolbox dnsq -h 查看帮助）")
		return 2
	}
	domain := strings.TrimSuffix(strings.TrimSpace(positionals[0]), ".")
	if domain == "" {
		fmt.Fprintln(os.Stderr, "错误: 域名不能为空")
		return 2
	}
	t := strings.ToUpper(*dtype)

	// 类型校验
	baseTypes := map[string]bool{"A": true, "AAAA": true, "CNAME": true, "MX": true, "NS": true, "TXT": true, "SRV": true}
	dohOnly := map[string]bool{"SOA": true, "PTR": true, "CAA": true}
	if !baseTypes[t] && !(dohOnly[t] && *doh != "") {
		if dohOnly[t] {
			fmt.Fprintf(os.Stderr, "错误: 类型 %s 仅在 -doh 模式下支持\n", t)
		} else {
			fmt.Fprintf(os.Stderr, "错误: 不支持的记录类型 %s（可选: A AAAA CNAME MX NS TXT SRV）\n", t)
		}
		return 2
	}

	var records []rec
	var err error
	if *doh != "" {
		records, err = queryDoH(domain, t, *doh)
	} else {
		records, err = querySystem(domain, t)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "查询失败: %v\n", err)
		return 1
	}

	mode := "系统 DNS"
	if *doh != "" {
		mode = "DoH(" + dohProviderName(*doh) + ")"
	}
	fmt.Printf("查询: %s  类型: %s  方式: %s\n", domain, t, mode)
	fmt.Println(strings.Repeat("-", 72))
	fmt.Printf("%-32s %-6s %-8s %s\n", "记录名", "类型", "TTL", "值")
	for _, r := range records {
		fmt.Printf("%-32s %-6s %-8s %s\n", r.name, r.typ, r.ttl, r.value)
	}
	fmt.Println(strings.Repeat("-", 72))
	fmt.Printf("共 %d 条记录\n", len(records))
	if len(records) == 0 {
		return 1
	}
	return 0
}

// querySystem 使用 net.Resolver（系统 DNS）
func querySystem(domain, t string) ([]rec, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r := net.DefaultResolver
	var records []rec
	var err error
	switch t {
	case "A":
		var ips []net.IP
		ips, err = r.LookupIP(ctx, "ip4", domain)
		for _, ip := range ips {
			records = append(records, rec{domain, "A", "—", ip.String()})
		}
	case "AAAA":
		var ips []net.IP
		ips, err = r.LookupIP(ctx, "ip6", domain)
		for _, ip := range ips {
			records = append(records, rec{domain, "AAAA", "—", ip.String()})
		}
	case "CNAME":
		var c string
		c, err = r.LookupCNAME(ctx, domain)
		records = append(records, rec{domain, "CNAME", "—", c})
	case "MX":
		var mxs []*net.MX
		mxs, err = r.LookupMX(ctx, domain)
		for _, m := range mxs {
			records = append(records, rec{domain, "MX", "—", fmt.Sprintf("%d %s", m.Pref, m.Host)})
		}
	case "NS":
		var nss []*net.NS
		nss, err = r.LookupNS(ctx, domain)
		for _, n := range nss {
			records = append(records, rec{domain, "NS", "—", n.Host})
		}
	case "TXT":
		var txts []string
		txts, err = r.LookupTXT(ctx, domain)
		for _, s := range txts {
			records = append(records, rec{domain, "TXT", "—", s})
		}
	case "SRV":
		var (
			_, srvs []*net.SRV
		)
		_, srvs, err = r.LookupSRV(ctx, "", "", domain)
		for _, s := range srvs {
			records = append(records, rec{domain, "SRV", "—",
				fmt.Sprintf("%d %d %d %s", s.Priority, s.Weight, s.Port, s.Target)})
		}
	}
	if err != nil {
		if de, ok := err.(*net.DNSError); ok && de.IsNotFound {
			return nil, fmt.Errorf("域名不存在（NXDOMAIN）: %s", domain)
		}
		return nil, err
	}
	return records, nil
}

// dohProviderName 归一化 DoH 服务显示名
func dohProviderName(provider string) string {
	switch strings.ToLower(provider) {
	case "google", "g":
		return "google"
	case "alidns", "ali":
		return "alidns"
	default:
		return "cloudflare"
	}
}

// queryDoH 通过 DoH JSON API 查询
func queryDoH(domain, t, provider string) ([]rec, error) {
	var endpoint string
	switch strings.ToLower(provider) {
	case "cloudflare", "cf", "cloudflare-dns", "1.1.1.1", "true":
		endpoint = "https://1.1.1.1/dns-query"
	case "google", "g":
		endpoint = "https://dns.google/resolve"
	case "alidns", "ali":
		endpoint = "https://dns.alidns.com/resolve"
	default:
		return nil, fmt.Errorf("未知 DoH 服务 %q（可选: cloudflare | google | alidns）", provider)
	}
	u := fmt.Sprintf("%s?name=%s&type=%s", endpoint, url.QueryEscape(domain), url.QueryEscape(t))
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/dns-json")
	req.Header.Set("User-Agent", "LynToolBox/1.0")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH 服务返回 HTTP %d", resp.StatusCode)
	}
	var dr dohResponse
	if err := json.Unmarshal(body, &dr); err != nil {
		return nil, fmt.Errorf("解析 DoH 响应失败: %v", err)
	}
	if dr.Status == 3 {
		return nil, fmt.Errorf("域名不存在（NXDOMAIN）: %s", domain)
	}
	if dr.Status != 0 {
		return nil, fmt.Errorf("DoH 返回错误状态码 RCODE=%d", dr.Status)
	}
	records := make([]rec, 0, len(dr.Answer))
	for _, a := range dr.Answer {
		tn, ok := dohTypeNum[a.Type]
		if !ok {
			tn = fmt.Sprintf("TYPE%d", a.Type)
		}
		records = append(records, rec{
			name:  strings.TrimSuffix(a.Name, "."),
			typ:   tn,
			ttl:   fmt.Sprintf("%d", a.TTL),
			value: strings.TrimSuffix(a.Data, "."),
		})
	}
	return records, nil
}
