// LynToolBox-cli 绘萤工具箱命令行版。
// 单文件 exe、多子命令；每个工具的实现在 tools/<命令名>/ 下的一个单独源文件中。
// 源码仓库：https://github.com/lynvortex/LynToolBox-cli
package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/lynvortex/LynToolBox-cli/internal/toolreg"

	"github.com/lynvortex/LynToolBox-cli/tools/base64"
	"github.com/lynvortex/LynToolBox-cli/tools/dockerfile"
	"github.com/lynvortex/LynToolBox-cli/tools/figlet"
	"github.com/lynvortex/LynToolBox-cli/tools/gitignore"
	"github.com/lynvortex/LynToolBox-cli/tools/hash"
	"github.com/lynvortex/LynToolBox-cli/tools/lorem"
	"github.com/lynvortex/LynToolBox-cli/tools/metatags"
	"github.com/lynvortex/LynToolBox-cli/tools/nginxconf"
	"github.com/lynvortex/LynToolBox-cli/tools/qrcode"
	"github.com/lynvortex/LynToolBox-cli/tools/robots"

	"github.com/lynvortex/LynToolBox-cli/tools/dnsq"
	"github.com/lynvortex/LynToolBox-cli/tools/httphead"
	"github.com/lynvortex/LynToolBox-cli/tools/httpreq"
	"github.com/lynvortex/LynToolBox-cli/tools/ipchk"
	"github.com/lynvortex/LynToolBox-cli/tools/macloc"
	"github.com/lynvortex/LynToolBox-cli/tools/mime"
	"github.com/lynvortex/LynToolBox-cli/tools/ping"
	"github.com/lynvortex/LynToolBox-cli/tools/portscan"
	"github.com/lynvortex/LynToolBox-cli/tools/subnet"
	"github.com/lynvortex/LynToolBox-cli/tools/tsconv"
	"github.com/lynvortex/LynToolBox-cli/tools/uaparse"
	"github.com/lynvortex/LynToolBox-cli/tools/urlparse"
)

func registerAll() {
	toolreg.Register(&toolreg.Tool{Name: base64.Name, Group: gA, Desc: base64.Desc, Usage: base64.Usage, Run: base64.Run})
	toolreg.Register(&toolreg.Tool{Name: hash.Name, Group: gA, Desc: hash.Desc, Usage: hash.Usage, Run: hash.Run})

	toolreg.Register(&toolreg.Tool{Name: gitignore.Name, Group: gD, Desc: gitignore.Desc, Usage: gitignore.Usage, Run: gitignore.Run})
	toolreg.Register(&toolreg.Tool{Name: robots.Name, Group: gD, Desc: robots.Desc, Usage: robots.Usage, Run: robots.Run})
	toolreg.Register(&toolreg.Tool{Name: nginxconf.Name, Group: gD, Desc: nginxconf.Desc, Usage: nginxconf.Usage, Run: nginxconf.Run})
	toolreg.Register(&toolreg.Tool{Name: dockerfile.Name, Group: gD, Desc: dockerfile.Desc, Usage: dockerfile.Usage, Run: dockerfile.Run})
	toolreg.Register(&toolreg.Tool{Name: metatags.Name, Group: gD, Desc: metatags.Desc, Usage: metatags.Usage, Run: metatags.Run})
	toolreg.Register(&toolreg.Tool{Name: lorem.Name, Group: gD, Desc: lorem.Desc, Usage: lorem.Usage, Run: lorem.Run})
	toolreg.Register(&toolreg.Tool{Name: figlet.Name, Group: gD, Desc: figlet.Desc, Usage: figlet.Usage, Run: figlet.Run})
	toolreg.Register(&toolreg.Tool{Name: qrcode.Name, Group: gD, Desc: qrcode.Desc, Usage: qrcode.Usage, Run: qrcode.Run})

	toolreg.Register(&toolreg.Tool{Name: ping.Name, Group: gE, Desc: ping.Desc, Usage: ping.Usage, Run: ping.Run})
	toolreg.Register(&toolreg.Tool{Name: portscan.Name, Group: gE, Desc: portscan.Desc, Usage: portscan.Usage, Run: portscan.Run})
	toolreg.Register(&toolreg.Tool{Name: httpreq.Name, Group: gE, Desc: httpreq.Desc, Usage: httpreq.Usage, Run: httpreq.Run})
	toolreg.Register(&toolreg.Tool{Name: httphead.Name, Group: gE, Desc: httphead.Desc, Usage: httphead.Usage, Run: httphead.Run})
	toolreg.Register(&toolreg.Tool{Name: dnsq.Name, Group: gE, Desc: dnsq.Desc, Usage: dnsq.Usage, Run: dnsq.Run})
	toolreg.Register(&toolreg.Tool{Name: subnet.Name, Group: gE, Desc: subnet.Desc, Usage: subnet.Usage, Run: subnet.Run})
	toolreg.Register(&toolreg.Tool{Name: ipchk.Name, Group: gE, Desc: ipchk.Desc, Usage: ipchk.Usage, Run: ipchk.Run})
	toolreg.Register(&toolreg.Tool{Name: urlparse.Name, Group: gE, Desc: urlparse.Desc, Usage: urlparse.Usage, Run: urlparse.Run})
	toolreg.Register(&toolreg.Tool{Name: uaparse.Name, Group: gE, Desc: uaparse.Desc, Usage: uaparse.Usage, Run: uaparse.Run})
	toolreg.Register(&toolreg.Tool{Name: mime.Name, Group: gE, Desc: mime.Desc, Usage: mime.Usage, Run: mime.Run})
	toolreg.Register(&toolreg.Tool{Name: macloc.Name, Group: gE, Desc: macloc.Desc, Usage: macloc.Usage, Run: macloc.Run})
	toolreg.Register(&toolreg.Tool{Name: tsconv.Name, Group: gE, Desc: tsconv.Desc, Usage: tsconv.Usage, Run: tsconv.Run})
}

const (
	gA = "A 编码·加密·哈希"
	gB = "B 数据格式"
	gC = "C 文本批处理"
	gD = "D 生成器"
	gE = "E 网络工具"
	gF = "F 计算·校验"
	gG = "G 网络补充"
	gH = "H 文件·文档"
	gI = "I 图片批处理"
	gJ = "J 文本·计算补充"
	gK = "K 联网查询"
)

var version = "1.0.0"

func groupOrder(g string) int {
	names := []string{gA, gB, gC, gD, gE, gF, gG, gH, gI, gJ, gK}
	for i, n := range names {
		if n == g {
			return i
		}
	}
	return len(names)
}

func printList() {
	byGroup := map[string][]*toolreg.Tool{}
	for _, t := range toolreg.All {
		byGroup[t.Group] = append(byGroup[t.Group], t)
	}
	groups := make([]string, 0, len(byGroup))
	for g := range byGroup {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return groupOrder(groups[i]) < groupOrder(groups[j]) })

	fmt.Printf("绘萤工具箱命令行版 v%s — 共 %d 个工具\n\n", version, len(toolreg.All))
	fmt.Println("用法: lyntoolbox <命令名> [参数]    查看某命令详情: lyntoolbox <命令名> -h")
	fmt.Println()
	for _, g := range groups {
		fmt.Printf("【%s】\n", g)
		for _, t := range byGroup[g] {
			fmt.Printf("  %-12s %s\n", t.Name, t.Desc)
		}
		fmt.Println()
	}
	fmt.Println("项目地址: https://github.com/lynvortex/LynToolBox-cli")
}

func main() {
	initConsole()
	registerAll()
	if len(os.Args) < 2 {
		// 无参数：交互式菜单模式（双击 exe 或直接运行）
		os.Exit(runInteractive())
	}
	name := os.Args[1]
	if name == "-v" || name == "--version" {
		fmt.Println("lyntoolbox v" + version)
		return
	}
	if name == "help" || name == "list" || name == "--help" || name == "-h" {
		if len(os.Args) > 2 {
			name = os.Args[2] // help <cmd>
		} else {
			printList()
			return
		}
	}
	for _, t := range toolreg.All {
		if t.Name == name {
			os.Exit(dispatch(t, os.Args[2:]))
			return
		}
	}
	fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", strings.TrimSpace(name))
	printList()
	os.Exit(2)
}

// dispatch 兼容"位置参数在前、选项在后"的书写习惯：
// 检测到乱序时优先按"选项提前"重排静默执行，子命令式工具再尝试保持首参的变体，
// 均失败则按原始顺序正式执行以给出标准错误信息。失败尝试的输出会被丢弃，避免噪音。
func dispatch(t *toolreg.Tool, args []string) int {
	if hasDisorder(args) {
		alt := partition(args)
		if !equalArgs(alt, args) {
			if code, out, errS := runQuiet(t, alt); code != 2 {
				fmt.Print(out)
				fmt.Fprint(os.Stderr, errS)
				return code
			}
		}
		if len(args) > 1 && !strings.HasPrefix(args[0], "-") {
			alt2 := append([]string{args[0]}, partition(args[1:])...)
			if !equalArgs(alt2, args) {
				if code, out, errS := runQuiet(t, alt2); code != 2 {
					fmt.Print(out)
					fmt.Fprint(os.Stderr, errS)
					return code
				}
			}
		}
	}
	return t.Run(args)
}

// runQuiet 静默执行一次命令，捕获其 stdout/stderr。
func runQuiet(t *toolreg.Tool, args []string) (int, string, string) {
	oldOut, oldErr := os.Stdout, os.Stderr
	rOut, wOut, err1 := os.Pipe()
	rErr, wErr, err2 := os.Pipe()
	if err1 != nil || err2 != nil {
		return t.Run(args), "", ""
	}
	os.Stdout, os.Stderr = wOut, wErr
	code := t.Run(args)
	wOut.Close()
	wErr.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	out, _ := io.ReadAll(rOut)
	errS, _ := io.ReadAll(rErr)
	return code, string(out), string(errS)
}

func equalArgs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// hasDisorder 判断是否存在"位置参数之后再出现选项"的乱序。
func hasDisorder(args []string) bool {
	seenPos := false
	for _, a := range args {
		if a == "--" {
			return false
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			if seenPos {
				return true
			}
		} else {
			seenPos = true
		}
	}
	return false
}

// partition 稳定重排：选项（连同其值）提前，位置参数保持相对顺序在后。
func partition(args []string) []string {
	var flags, pos []string
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			flags = append(flags, a)
			i++
			if i < len(args) && !strings.HasPrefix(args[i], "-") && args[i] != "--" {
				flags = append(flags, args[i])
				i++
			}
		} else {
			pos = append(pos, a)
			i++
		}
	}
	return append(flags, pos...)
}
