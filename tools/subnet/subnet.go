// Package subnet 实现子网计算器命令。
// 对应网页版：network/subnet-calc-tool.html（子网掩码计算）、network/ipv6-subnet-tool.html（IPv6子网计算）
//
// 用法：
//
//	lyntoolbox subnet 192.168.1.0/24
//	lyntoolbox subnet 2001:db8::/32
//	lyntoolbox subnet 192.168.1.0/24 -in 192.168.1.55
package subnet

import (
	"flag"
	"fmt"
	"math/big"
	"net/netip"
	"os"
	"strings"
)

const (
	Name  = "subnet"
	Desc  = "子网计算器（IPv4/IPv6），输出网络地址/掩码/可用范围/总数等"
	Usage = `用法: lyntoolbox subnet CIDR [-in IP]

参数:
  CIDR  网段，如 192.168.1.0/24 或 2001:db8::/32；裸 IP 按 /32 或 /128 处理
  -in   判定指定 IP 是否属于该网段

说明: IPv4 输出网络/广播地址、掩码、反掩码、可用范围与主机数、二进制表示；
IPv6 输出前缀、地址范围、总数、完整展开与压缩形式。`
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
		fmt.Fprintln(os.Stderr, "错误: 需要且只需要一个 CIDR 参数（如 192.168.1.0/24）")
		fmt.Fprintln(os.Stderr, "（lyntoolbox subnet -h 查看帮助）")
		return 2
	}

	arg := strings.TrimSpace(positionals[0])
	p, perr := netip.ParsePrefix(arg)
	if perr != nil {
		if a, aerr := netip.ParseAddr(arg); aerr == nil {
			if a.Is4() {
				p = netip.PrefixFrom(a, 32)
			} else {
				p = netip.PrefixFrom(a, 128)
			}
			fmt.Fprintf(os.Stderr, "提示: 输入为裸 IP，按 /%d 处理\n", p.Bits())
		} else {
			fmt.Fprintf(os.Stderr, "无法解析 CIDR %q: %v\n", arg, perr)
			return 2
		}
	}

	if p.Addr().Is4() {
		calcV4(p)
	} else {
		calcV6(p)
	}

	if *in != "" {
		a, err := netip.ParseAddr(strings.TrimSpace(*in))
		if err != nil {
			fmt.Fprintf(os.Stderr, "无法解析 IP %q: %v\n", *in, err)
			return 2
		}
		switch {
		case a.Is4() != p.Addr().Is4():
			fmt.Printf("\n归属判定: %s 不属于 %s（地址族不同）\n", a, p)
		case p.Contains(a):
			fmt.Printf("\n归属判定: %s 属于 %s: 是\n", a, p)
		default:
			fmt.Printf("\n归属判定: %s 属于 %s: 否\n", a, p)
		}
	}
	return 0
}

// calcV4 IPv4 子网计算
func calcV4(p netip.Prefix) {
	bits := p.Bits()
	network := p.Masked()
	na := network.Addr()
	nb := na.As4()

	var mask [4]byte
	for i := 0; i < 4; i++ {
		var m byte
		for b := 0; b < 8; b++ {
			if i*8+b < bits {
				m |= 1 << (7 - b)
			}
		}
		mask[i] = m
	}
	var bcast [4]byte
	for i := range nb {
		bcast[i] = nb[i] | ^mask[i]
	}
	ba := netip.AddrFrom4(bcast)

	var wild [4]byte
	for i := range mask {
		wild[i] = ^mask[i]
	}

	fmt.Println("协议版本:       IPv4")
	fmt.Printf("输入:           %s\n", p)
	fmt.Printf("网络地址:       %s/%d\n", na, bits)
	fmt.Printf("广播地址:       %s\n", ba)
	fmt.Printf("子网掩码:       %s\n", dotted(mask))
	fmt.Printf("反掩码(通配符): %s\n", dotted(wild))

	var first, last netip.Addr
	var usable uint64
	switch bits {
	case 32:
		first, last, usable = na, na, 1
	case 31:
		first, last, usable = na, ba, 2
	default:
		first, last = na.Next(), ba.Prev()
		usable = (uint64(1) << (32 - bits)) - 2
	}
	fmt.Printf("可用地址范围:   %s ~ %s\n", first, last)
	fmt.Printf("可用主机数:     %d\n", usable)
	fmt.Printf("总地址数:       %d\n", uint64(1)<<(32-bits))
	fmt.Printf("网络地址二进制: %s\n", binDotted(nb))
	fmt.Printf("掩码二进制:     %s\n", binDotted(mask))
}

// calcV6 IPv6 子网计算
func calcV6(p netip.Prefix) {
	bits := p.Bits()
	network := p.Masked()
	first := network.Addr()

	total := new(big.Int).Lsh(big.NewInt(1), uint(128-bits))
	hi := addrToBig(first)
	hi.Add(hi, total)
	hi.Sub(hi, big.NewInt(1))
	last := bigToAddr(hi)

	fmt.Println("协议版本:  IPv6")
	fmt.Printf("输入:      %s\n", p)
	fmt.Printf("网络前缀:  %s\n", network.String())
	fmt.Printf("首地址:    %s\n", first)
	fmt.Printf("末地址:    %s\n", last)
	fmt.Printf("总地址数:  2^%d = %s\n", 128-bits, total.String())
	fmt.Printf("完整展开:  %s\n", first.StringExpanded())
	fmt.Printf("压缩形式:  %s\n", first.String())
}

// dotted 点分十进制
func dotted(b [4]byte) string {
	return fmt.Sprintf("%d.%d.%d.%d", b[0], b[1], b[2], b[3])
}

// binDotted 二进制点分表示
func binDotted(b [4]byte) string {
	parts := make([]string, 4)
	for i, v := range b {
		parts[i] = fmt.Sprintf("%08b", v)
	}
	return strings.Join(parts, ".")
}

// addrToBig IPv6 地址转大整数
func addrToBig(a netip.Addr) *big.Int {
	b := a.As16()
	return new(big.Int).SetBytes(b[:])
}

// bigToAddr 大整数转 IPv6 地址
func bigToAddr(x *big.Int) netip.Addr {
	var b [16]byte
	xb := x.Bytes()
	copy(b[16-len(xb):], xb)
	return netip.AddrFrom16(b)
}
