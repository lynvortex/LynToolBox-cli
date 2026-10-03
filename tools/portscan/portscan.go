// Package portscan 实现 TCP 端口检测/扫描命令。
// 对应网页版：network/host-tcp-tool.html（TCP端口检测）、network/host-port-tool.html（端口扫描）
//
// 用法：
//
//	lyntoolbox portscan 主机 -p 80,443,8080
//	lyntoolbox portscan 主机 -range 1-1024
//	lyntoolbox portscan 主机 -common
package portscan

import (
	"flag"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	Name  = "portscan"
	Desc  = "TCP 端口检测与扫描（200 并发），支持端口列表/范围/常用端口三种方式"
	Usage = `用法: lyntoolbox portscan 主机 [-p 端口列表] [-range 范围] [-common] [-timeout 毫秒]

参数:
  -p        逗号分隔的端口列表，如 "80,443,8080"；仅一个端口时输出详细结果
  -range    端口范围，如 "1-1024"
  -common   扫描内置常用端口表（约 100 个）
  -timeout  单次 TCP 连接超时毫秒数（默认 1500）
  主机       目标主机名或 IP 地址

说明: -p / -range / -common 三选一（均省略时默认扫描常用端口）；TCP connect 扫描。`
)

// commonPorts 内置常用端口表（top100）
var commonPorts = []int{
	20, 21, 22, 23, 25, 53, 67, 68, 69, 80,
	81, 88, 110, 111, 123, 135, 137, 138, 139, 143,
	161, 162, 179, 389, 443, 445, 465, 500, 514, 515,
	540, 548, 554, 587, 593, 623, 631, 636, 873, 990,
	993, 995, 1080, 1099, 1194, 1433, 1434, 1521, 1723, 1883,
	2049, 2082, 2083, 2181, 2375, 2376, 2483, 2484, 3000, 3128,
	3306, 3389, 4848, 5000, 5353, 5432, 5601, 5672, 5900, 5901,
	5984, 5985, 5986, 6379, 6443, 6667, 7001, 7002, 8000, 8008,
	8009, 8080, 8081, 8443, 8500, 8649, 8888, 9000, 9090, 9091,
	9200, 9300, 9999, 10000, 11211, 15672, 27017, 27018, 28017, 50070,
	61616,
}

// serviceNames 常见端口服务名猜测
var serviceNames = map[int]string{
	20: "ftp-data", 21: "ftp", 22: "ssh", 23: "telnet", 25: "smtp",
	53: "domain", 67: "dhcp-server", 68: "dhcp-client", 69: "tftp", 80: "http",
	81: "http", 88: "kerberos", 110: "pop3", 111: "rpcbind", 123: "ntp",
	135: "msrpc", 137: "netbios-ns", 138: "netbios-dgm", 139: "netbios-ssn", 143: "imap",
	161: "snmp", 162: "snmptrap", 179: "bgp", 389: "ldap", 443: "https",
	445: "microsoft-ds", 465: "smtps", 500: "isakmp", 514: "syslog", 515: "printer",
	540: "uucp", 548: "afp", 554: "rtsp", 587: "submission", 593: "http-rpc-epmap",
	623: "ipmi", 631: "ipp", 636: "ldaps", 873: "rsync", 990: "ftps",
	993: "imaps", 995: "pop3s", 1080: "socks", 1099: "java-rmi", 1194: "openvpn",
	1433: "ms-sql", 1434: "ms-sql-d", 1521: "oracle", 1723: "pptp", 1883: "mqtt",
	2049: "nfs", 2082: "cpanel", 2083: "cpanel-ssl", 2181: "zookeeper", 2375: "docker",
	2376: "docker-tls", 2483: "oracle-oci", 2484: "oracle-oci-ssl", 3000: "http-alt", 3128: "squid",
	3306: "mysql", 3389: "rdp", 4848: "glassfish", 5000: "upnp", 5353: "mdns",
	5432: "postgresql", 5601: "kibana", 5672: "amqp", 5900: "vnc", 5901: "vnc",
	5984: "couchdb", 5985: "winrm", 5986: "winrm-ssl", 6379: "redis", 6443: "kubernetes",
	6667: "irc", 7001: "weblogic", 7002: "weblogic-ssl", 8000: "http-alt", 8008: "http-alt",
	8009: "ajp", 8080: "http-proxy", 8081: "http-alt", 8443: "https-alt", 8500: "consul",
	8649: "ganglia", 8888: "http-alt", 9000: "php-fpm", 9090: "prometheus", 9091: "http-alt",
	9200: "elasticsearch", 9300: "elasticsearch", 10000: "webmin", 11211: "memcached", 15672: "rabbitmq-mgmt",
	27017: "mongodb", 27018: "mongodb", 28017: "mongodb-web", 50070: "hdfs", 61616: "activemq",
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	plist := fs.String("p", "", "端口列表，逗号分隔")
	rng := fs.String("range", "", "端口范围，如 1-1024")
	common := fs.Bool("common", false, "扫描常用端口")
	timeoutMs := fs.Int("timeout", 1500, "连接超时毫秒数")
	fs.Usage = func() { fmt.Print(Usage + "\n") }

	rest := args
	var positionals []string
	for {
		if err := fs.Parse(rest); err != nil {
			if err == flag.ErrHelp {
				return 0
			}
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
		fmt.Fprintln(os.Stderr, "错误: 需要且只需要一个目标主机参数")
		fmt.Fprintln(os.Stderr, "（lyntoolbox portscan -h 查看帮助）")
		return 2
	}
	if *timeoutMs < 1 {
		fmt.Fprintln(os.Stderr, "错误: -timeout 必须 ≥ 1 毫秒")
		return 2
	}

	modes := 0
	if *plist != "" {
		modes++
	}
	if *rng != "" {
		modes++
	}
	if *common {
		modes++
	}
	if modes > 1 {
		fmt.Fprintln(os.Stderr, "错误: -p、-range、-common 只能三选一")
		return 2
	}

	var ports []int
	var err error
	switch {
	case *plist != "":
		ports, err = parsePortList(*plist)
	case *rng != "":
		ports, err = parseRange(*rng)
	case *common:
		ports = commonPorts
	default:
		ports = commonPorts
		fmt.Fprintln(os.Stderr, "提示: 未指定端口方式，默认扫描常用端口（可用 -p / -range / -common 指定）")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 2
	}
	ports = dedupeSort(ports)
	host := positionals[0]

	ip, err := resolveHost(host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法解析主机 %s: %v\n", host, err)
		return 1
	}
	timeout := time.Duration(*timeoutMs) * time.Millisecond

	if len(ports) == 1 {
		return probeSingle(host, ip, ports[0], timeout)
	}
	return scan(host, ip, ports, timeout)
}

// probeSingle 单端口模式：详细输出开/关/拒绝
func probeSingle(host, ip string, port int, timeout time.Duration) int {
	fmt.Printf("目标: %s (%s)\n端口: %d/tcp\n超时: %s\n检测中...\n", host, ip, port, timeout)
	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, err := d.Dial("tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
	elapsed := time.Since(start).Round(time.Millisecond)
	if err == nil {
		conn.Close()
		fmt.Printf("结果: 开放（TCP 连接成功，用时 %s）\n", elapsed)
		return 0
	}
	fmt.Printf("结果: %s\n（%v）\n", classifyErr(err), err)
	return 1
}

// scan 多端口扫描：200 并发 goroutine
func scan(host, ip string, ports []int, timeout time.Duration) int {
	fmt.Printf("目标: %s (%s)\n端口数: %d  并发: 200  超时: %s\n扫描中...\n", host, ip, len(ports), timeout)

	type scanRes struct {
		port int
		err  error
	}
	jobs := make(chan int)
	resCh := make(chan scanRes, len(ports))
	workers := 200
	if workers > len(ports) {
		workers = len(ports)
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(p)), timeout)
				if err == nil {
					conn.Close()
				}
				resCh <- scanRes{port: p, err: err}
			}
		}()
	}
	for _, p := range ports {
		jobs <- p
	}
	close(jobs)
	wg.Wait()
	close(resCh)

	var open []int
	for r := range resCh {
		if r.err == nil {
			open = append(open, r.port)
		}
	}
	sort.Ints(open)

	if len(open) == 0 {
		fmt.Printf("\n未发现开放端口（共扫描 %d 个）\n", len(ports))
		return 0
	}
	fmt.Printf("\n开放端口（%d/%d）:\n", len(open), len(ports))
	fmt.Println("  端口      服务(猜测)")
	for _, p := range open {
		svc, ok := serviceNames[p]
		if !ok {
			svc = "-"
		}
		fmt.Printf("  %-8d %s\n", p, svc)
	}
	return 0
}

// classifyErr 将拨号错误归类为中文描述
func classifyErr(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "refused"):
		return "关闭（连接被拒绝）"
	case strings.Contains(msg, "i/o timeout"), strings.Contains(msg, "timed out"):
		return "超时或被过滤"
	case strings.Contains(msg, "unreachable"):
		return "网络不可达"
	default:
		return "无法连接"
	}
}

// resolveHost 解析目标为 IP 地址
func resolveHost(host string) (string, error) {
	if ip := net.ParseIP(host); ip != nil {
		return ip.String(), nil
	}
	addrs, err := net.LookupHost(host)
	if err != nil {
		return "", err
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("无解析结果")
	}
	return addrs[0], nil
}

// parsePortList 解析 "80,443,8080"
func parsePortList(s string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("端口不是数字: %q", part)
		}
		if n < 1 || n > 65535 {
			return nil, fmt.Errorf("端口超出范围 1-65535: %d", n)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("端口列表为空")
	}
	return out, nil
}

// parseRange 解析 "1-1024"
func parseRange(s string) ([]int, error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("范围格式应为 起始-结束，如 1-1024")
	}
	start, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	end, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("范围格式应为 起始-结束，如 1-1024")
	}
	if start < 1 || end > 65535 || start > end {
		return nil, fmt.Errorf("范围非法（需 1 ≤ 起始 ≤ 结束 ≤ 65535）")
	}
	out := make([]int, 0, end-start+1)
	for p := start; p <= end; p++ {
		out = append(out, p)
	}
	return out, nil
}

// dedupeSort 去重并升序
func dedupeSort(ports []int) []int {
	seen := make(map[int]bool, len(ports))
	out := make([]int, 0, len(ports))
	for _, p := range ports {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Ints(out)
	return out
}
