// Package ipchk 实现 IP 地址校验与分类解析命令。
// 对应网页版：network/ip-validate-tool.html（IP地址校验）
//
// 用法：
//
//	lyntoolbox ipchk 192.168.1.1
//	lyntoolbox ipchk 2001:db8::1 -in 2001:db8::/32
package ipchk

import (
	"encoding/binary"
	"flag"
	"fmt"
	"math/big"
	"net/netip"
	"os"
	"strings"
)

const (
	Name  = "ipchk"
	Desc  = "IP 地址校验与分类解析（合法性/环回/私有/组播/保留等）"
	Usage = `用法: lyntoolbox ipchk IP [-in CIDR]

参数:
  IP    待校验的 IPv4/IPv6 地址
  -in   判定该 IP 是否属于指定网段（如 192.168.1.0/24）

说明: 输出版本、合法性、各类别归属（环回/私有/组播/链路本地/保留等）、
IPv4 的整数值与十六进制表示。`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	in := fs.String("in", "", "判定 IP 是否属于该网段")
	fs.Usage = func() { fmt.Print(Usage + "\n") }

	rest := args
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
		fmt.Fprintln(os.Stderr, "错误: 需要且只需要一个 IP 参数")
		fmt.Fprintln(os.Stderr, "（lyntoolbox ipchk -h 查看帮助）")
		return 2
	}
	input := strings.TrimSpace(positionals[0])

	a, err := netip.ParseAddr(input)
	if err != nil {
		fmt.Printf("IP:   %s\n合法: 否\n错误: %v\n", input, err)
		return 1
	}

	isV4 := a.Is4()
	fmt.Printf("IP:   %s\n", a)
	fmt.Println("合法: 是")
	fmt.Printf("版本: IPv%d\n", map[bool]int{true: 4, false: 6}[isV4])
	if a.Is4In6() && !isV4 {
		fmt.Printf("提示: IPv4 映射地址（::ffff:x.x.x.x），实际承载 IPv4 %s\n", netip.AddrFrom4(a.As4()))
	}

	fmt.Println("分类:")
	for _, c := range classify(a) {
		mark := "否"
		if c.yes {
			mark = "是"
		}
		fmt.Printf("  %-6s %s\n", mark, c.label)
	}

	if isV4 {
		b := a.As4()
		n := binary.BigEndian.Uint32(b[:])
		fmt.Printf("整数值: %d\n", n)
		fmt.Printf("十六进制: 0x%08X\n", n)
	} else {
		b16 := a.As16()
		x := new(big.Int).SetBytes(b16[:])
		fmt.Printf("整数值: %s（IPv6 大整数，128 位）\n", x.String())
	}

	if *in != "" {
		p, perr := netip.ParsePrefix(strings.TrimSpace(*in))
		if perr != nil {
			fmt.Fprintf(os.Stderr, "无法解析 CIDR %q: %v\n", *in, perr)
			return 2
		}
		switch {
		case p.Addr().Is4() != isV4:
			fmt.Printf("归属判定: %s 属于 %s: 否（地址族不同）\n", a, p)
		case p.Contains(a):
			fmt.Printf("归属判定: %s 属于 %s: 是\n", a, p)
		default:
			fmt.Printf("归属判定: %s 属于 %s: 否\n", a, p)
		}
	}
	return 0
}

type category struct {
	label string
	yes   bool
}

// classify 依据 netip.Addr.Is* 与手写补充段进行分类
func classify(a netip.Addr) []category {
	isV4 := a.Is4()

	loopback := a.IsLoopback()
	private := a.IsPrivate()
	multicast := a.IsMulticast()
	linkLocal := a.IsLinkLocalUnicast()
	llMulticast := a.IsLinkLocalMulticast()
	unspecified := a.IsUnspecified()

	var extraSpecial bool
	var extras []category
	if isV4 {
		cgnat := inV4(a, "100.64.0.0/10")
		zeroNet := inV4(a, "0.0.0.0/8")
		reserved := inV4(a, "240.0.0.0/4")
		broadcast := a == netip.AddrFrom4([4]byte{255, 255, 255, 255})
		testnet := inV4(a, "192.0.2.0/24") || inV4(a, "198.51.100.0/24") || inV4(a, "203.0.113.0/24")
		benchmark := inV4(a, "198.18.0.0/15")
		ietf := inV4(a, "192.0.0.0/24")
		extraSpecial = cgnat || zeroNet || reserved || broadcast || testnet || benchmark || ietf
		extras = []category{
			{"运营商级 NAT (100.64.0.0/10)", cgnat},
			{"本网络段 (0.0.0.0/8)", zeroNet},
			{"保留地址 (240.0.0.0/4)", reserved},
			{"受限广播 (255.255.255.255)", broadcast},
			{"文档专用 TEST-NET", testnet},
			{"基准测试 (198.18.0.0/15)", benchmark},
			{"IETF 协议分配 (192.0.0.0/24)", ietf},
		}
	} else {
		ula := inV6(a, "fc00::/7")
		doc := inV6(a, "2001:db8::/32")
		extraSpecial = ula || doc
		extras = []category{
			{"唯一本地 ULA (fc00::/7)", ula},
			{"文档专用 (2001:db8::/32)", doc},
			{"IPv4 映射 (::ffff:0:0/96)", a.Is4In6()},
		}
	}

	global := !loopback && !private && !multicast && !linkLocal && !llMulticast && !unspecified && !extraSpecial

	out := []category{
		{"环回地址", loopback},
		{"私有地址", private},
		{"组播地址", multicast},
		{"链路本地单播", linkLocal},
		{"链路本地组播", llMulticast},
		{"未指定地址", unspecified},
		{"公网全球单播", global},
	}
	return append(out, extras...)
}

// inV4 判断 IPv4 是否属于 CIDR
func inV4(a netip.Addr, cidr string) bool {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return false
	}
	return p.Contains(a)
}

// inV6 判断 IPv6 是否属于 CIDR
func inV6(a netip.Addr, cidr string) bool {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return false
	}
	return p.Contains(a)
}
