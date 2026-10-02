package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/lynvortex/LynToolBox-cli/internal/toolreg"
)

// iPrompt 描述一个工具在交互模式下的输入项。
type iPrompt struct {
	flag   string // 对应的命令行 flag 名；"" 表示位置参数
	msg    string // 提示语
	def    string // 默认值
	req    bool   // 必填
	isBool bool   // 布尔开关（输入 y/是 开启）
	first  bool   // 作为第一个位置参数（子命令）
}

// interactivePrompts 每个命令的交互输入定义；未定义的命令在交互模式下提示用命令行方式使用。
var interactivePrompts = map[string][]iPrompt{
	"hash":       {{flag: "", msg: "要计算哈希的文本", req: true}, {flag: "file", msg: "或改算文件路径（填了此项忽略上文）"}},
	"base64":     {{flag: "", msg: "要编码/解码的内容", req: true}, {flag: "d", msg: "解码模式", isBool: true}, {flag: "url", msg: "URL 安全字母表", isBool: true}, {flag: "file", msg: "改为读取文件路径"}},
	"urlenc":     {{flag: "", msg: "要编码/解码的文本", req: true}, {flag: "d", msg: "解码模式", isBool: true}},
	"hexenc":     {{flag: "", msg: "要转换的文本", req: true}, {flag: "d", msg: "从十六进制解码", isBool: true}, {flag: "upper", msg: "输出大写", isBool: true}},
	"base58":     {{flag: "", msg: "要转换的文本", req: true}, {flag: "btc", msg: "Base58Check（比特币格式）", isBool: true}},
	"htmlent":    {{flag: "", msg: "要处理的 HTML 片段", req: true}, {flag: "d", msg: "解码模式", isBool: true}, {flag: "num", msg: "非 ASCII 输出数字实体", isBool: true}},
	"gzipx":      {{flag: "", msg: "要压缩的文本", req: true}, {flag: "d", msg: "解压模式（输入 Base64）", isBool: true}},
	"shesc":      {{flag: "", msg: "要转义的字符串", req: true}, {flag: "mode", msg: "转义风格 posix|cmd|powershell", def: "posix"}},
	"morse":      {{flag: "", msg: "要转换的文本", req: true}, {flag: "d", msg: "解码模式", isBool: true}},
	"aes":        {{flag: "", msg: "要加密/解密的文本", req: true}, {flag: "key", msg: "密钥（UTF-8）", req: true}, {flag: "iv", msg: "IV（CBC/CFB 必填）"}, {flag: "mode", msg: "模式 cbc|ecb|cfb", def: "cbc"}, {flag: "d", msg: "解密模式", isBool: true}},
	"smcrypto":   {{flag: "", msg: "子命令 sm3|sm4|sm2", req: true, first: true}, {flag: "", msg: "要处理的文本（sm3 哈希 / sm4 加解密）"}},
	"hmac":       {{flag: "", msg: "要签名的文本", req: true}, {flag: "k", msg: "密钥", req: true}, {flag: "algo", msg: "算法（默认 sha256）"}},
	"jwt":        {{flag: "", msg: "JWT Token", req: true}, {flag: "exp", msg: "检查过期时间", isBool: true}, {flag: "key", msg: "HS256 验签密钥（可选）"}},
	"rsagen":     {{flag: "bits", msg: "密钥位数 1024|2048|4096", def: "2048"}, {flag: "pkcs8", msg: "私钥用 PKCS#8 格式", isBool: true}},
	"pbkdf2":     {{flag: "pass", msg: "密码", req: true}, {flag: "iter", msg: "迭代次数", def: "10000"}, {flag: "len", msg: "派生长度（字节）", def: "32"}, {flag: "algo", msg: "算法 sha256|sha512|sha1", def: "sha256"}},
	"totp":       {{flag: "secret", msg: "Base32 密钥", req: true}, {flag: "digits", msg: "位数 6|8", def: "6"}, {flag: "period", msg: "周期（秒）", def: "30"}},
	"randgen":    {{flag: "", msg: "类型 uuid|password|token|number", req: true, first: true}, {flag: "len", msg: "长度（默认 16）"}, {flag: "n", msg: "数量（默认 5）"}},
	"gitignore":  {{flag: "stack", msg: "技术栈（逗号分隔，如 go,node）", def: "go,node"}},
	"robots":     {{flag: "disallow", msg: "禁止路径", def: "/"}, {flag: "allow", msg: "允许路径"}, {flag: "sitemap", msg: "Sitemap 地址"}},
	"nginxconf":  {{flag: "domain", msg: "域名（_ 表示全部）", def: "_"}, {flag: "proxy", msg: "反代地址（如 http://127.0.0.1:3000）"}, {flag: "root", msg: "静态站点根目录"}, {flag: "gzip", msg: "启用 gzip", isBool: true}},
	"dockerfile": {{flag: "stack", msg: "技术栈 go|node|python|java|static", req: true}, {flag: "port", msg: "监听端口", def: "8080"}, {flag: "multistage", msg: "多阶段构建", isBool: true}},
	"metatags":   {{flag: "title", msg: "页面标题", req: true}, {flag: "desc", msg: "页面描述", req: true}, {flag: "url", msg: "页面地址"}, {flag: "image", msg: "分享封面图"}},
	"lorem":      {{flag: "what", msg: "类型 text|name|phone|email|idcard", def: "text"}, {flag: "n", msg: "数量", def: "3"}},
	"figlet":     {{flag: "", msg: "横幅文本（A-Z 0-9）", req: true}},
	"qrcode":     {{flag: "", msg: "二维码内容", req: true}, {flag: "o", msg: "输出 PNG 路径（留空终端显示）"}},
	"ping":       {{flag: "", msg: "目标域名或 IP", req: true}, {flag: "c", msg: "次数", def: "4"}},
	"portscan":   {{flag: "", msg: "目标主机", req: true}, {flag: "p", msg: "端口列表（如 80,443；留空扫常用端口）"}},
	"httpreq":    {{flag: "", msg: "请求 URL", req: true}, {flag: "X", msg: "请求方法", def: "GET"}, {flag: "d", msg: "请求体"}},
	"httphead":   {{flag: "", msg: "目标 URL", req: true}},
	"dnsq":       {{flag: "", msg: "域名", req: true}, {flag: "type", msg: "记录类型 A|AAAA|CNAME|MX|NS|TXT|SRV", def: "A"}},
	"subnet":     {{flag: "", msg: "CIDR（如 192.168.1.0/24）", req: true}},
	"ipchk":      {{flag: "", msg: "IP 地址", req: true}},
	"urlparse":   {{flag: "", msg: "URL", req: true}},
	"uaparse":    {{flag: "", msg: "User-Agent 字符串", req: true}},
	"mime":       {{flag: "", msg: "扩展名（如 pdf）或 MIME 类型", req: true}},
	"macloc":     {{flag: "", msg: "MAC 地址（任意分隔格式）", req: true}},
	"tsconv":     {{flag: "", msg: "时间戳或日期（留空=当前时间）"}},
}

// runInteractive 交互式主循环：打开即显示菜单，输序号运行工具。
func runInteractive() int {
	reader := bufio.NewReader(os.Stdin)
	for {
		printMenu()
		fmt.Print("请输入序号运行工具（q 退出）: ")
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println()
			return 0
		}
		input := strings.TrimSpace(line)
		switch input {
		case "":
			continue
		case "q", "quit", "exit", "退出":
			fmt.Println("再见！萤火虽微，愿为其芒。")
			return 0
		case "help", "h", "?":
			continue
		}

		var tool *toolreg.Tool
		if idx := indexOfNumber(input); idx >= 1 && idx <= len(numberedTools) {
			tool = numberedTools[idx-1]
		} else {
			for _, t := range toolreg.All {
				if t.Name == strings.ToLower(input) {
					tool = t
					break
				}
			}
		}
		if tool == nil {
			fmt.Println("无效输入，请输入菜单中的序号或命令名。")
			continue
		}
		runToolInteractive(reader, tool)
	}
}

// numberedTools 与菜单显示顺序一致。
var numberedTools []*toolreg.Tool

func printMenu() {
	numberedTools = nil
	byGroup := map[string][]*toolreg.Tool{}
	for _, t := range toolreg.All {
		byGroup[t.Group] = append(byGroup[t.Group], t)
	}
	groups := make([]string, 0, len(byGroup))
	for g := range byGroup {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return groupOrder(groups[i]) < groupOrder(groups[j]) })

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════")
	fmt.Println("  绘萤工具箱 · 命令行版 v" + version)
	fmt.Println("  开源地址: https://github.com/lynvortex/LynToolBox-cli")
	fmt.Printf("  共 %d 个工具 | 输入序号运行，q 退出\n", len(toolreg.All))
	fmt.Println("══════════════════════════════════════════════")
	for _, g := range groups {
		fmt.Println("【" + g + "】")
		for _, t := range byGroup[g] {
			numberedTools = append(numberedTools, t)
			fmt.Printf("  %2d. %-12s %s\n", len(numberedTools), t.Name, t.Desc)
		}
	}
	fmt.Println("══════════════════════════════════════════════")
}

func indexOfNumber(s string) int {
	n := 0
	if s == "" {
		return -1
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// runToolInteractive 按交互提示收集输入，拼装参数后执行工具。
func runToolInteractive(reader *bufio.Reader, tool *toolreg.Tool) {
	prompts, ok := interactivePrompts[tool.Name]
	if !ok {
		fmt.Println()
		fmt.Println("该命令需要命令行参数，交互模式暂不支持：")
		fmt.Println(tool.Usage)
		fmt.Print("按回车返回菜单...")
		_, _ = reader.ReadString('\n')
		return
	}

	var flags, positionals, firstPos []string
	for _, p := range prompts {
		var val string
		for {
			hint := ""
			if p.def != "" {
				hint = "（默认 " + p.def + "）"
			}
			reqMark := ""
			if p.req {
				reqMark = "[必填] "
			}
			fmt.Printf("%s%s%s: ", reqMark, p.msg, hint)
			line, err := reader.ReadString('\n')
			if err != nil {
				val = ""
			} else {
				val = strings.TrimSpace(line)
			}
			if val == "" && p.req && p.def == "" {
				fmt.Println("  此项必填，请重新输入。")
				continue
			}
			if val == "" {
				val = p.def
			}
			break
		}
		if val == "" {
			continue
		}
		switch {
		case p.isBool:
			if val == "y" || val == "Y" || val == "是" || val == "1" || val == "true" {
				flags = append(flags, "-"+p.flag)
			}
		case p.flag == "":
			if p.first {
				firstPos = append(firstPos, val)
			} else {
				positionals = append(positionals, val)
			}
		default:
			flags = append(flags, "-"+p.flag, val)
		}
	}

	args := append(firstPos, flags...)
	args = append(args, positionals...)
	if len(args) == 0 {
		fmt.Println("未输入任何内容，返回菜单。")
		return
	}

	fmt.Println("────────────────────────────────────────────")
	code := dispatch(tool, args)
	fmt.Println("────────────────────────────────────────────")
	if code != 0 {
		fmt.Printf("（命令退出码 %d）\n", code)
	}
	fmt.Print("按回车返回菜单（q 退出）...")
	line, _ := reader.ReadString('\n')
	if strings.TrimSpace(line) == "q" {
		fmt.Println("再见！萤火虽微，愿为其芒。")
		os.Exit(0)
	}
}
