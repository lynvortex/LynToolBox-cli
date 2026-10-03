// Package cookiep 实现 Cookie 字符串解析命令。
// 对应网页版：network/cookie-viewer-tool.html（Cookie查看解析）
package cookiep

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

const (
	Name  = "cookiep"
	Desc  = "解析 Cookie 请求头 / Set-Cookie 响应头，检查安全属性"
	Usage = `用法: lyntoolbox cookiep "k1=v1; k2=v2" [-sc] [-json]

参数:
  Cookie 串  请求头格式（k=v; k2=v2）；或配合 -sc 传入 Set-Cookie 头
  -sc        Set-Cookie 响应头模式（可解析 -f 文件中的多行 Set-Cookie）
  -f         从文件读取
  -json      JSON 输出`
)

type cookieAttr struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain"`
	Path     string `json:"path"`
	Expires  string `json:"expires"`
	MaxAge   string `json:"maxAge"`
	HttpOnly bool   `json:"httpOnly"`
	Secure   bool   `json:"secure"`
	SameSite string `json:"sameSite"`
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	sc := fs.Bool("sc", false, "Set-Cookie 响应头模式")
	file := fs.String("f", "", "从文件读取")
	raw := fs.Bool("json", false, "JSON 输出")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	var input string
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取文件失败:", err)
			return 1
		}
		input = string(b)
	} else if fs.NArg() > 0 {
		input = strings.Join(fs.Args(), " ")
	} else {
		fmt.Fprintln(os.Stderr, "缺少 Cookie 字符串")
		return 2
	}

	var cookies []cookieAttr
	if *sc {
		for _, line := range strings.Split(input, "\n") {
			line = strings.TrimSpace(line)
			line = strings.TrimPrefix(line, "Set-Cookie:")
			if line == "" {
				continue
			}
			if c, ok := parseSetCookie(line); ok {
				cookies = append(cookies, c)
			}
		}
	} else {
		for _, part := range strings.Split(input, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			k, v, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			cookies = append(cookies, cookieAttr{Name: strings.TrimSpace(k), Value: strings.TrimSpace(v)})
		}
	}

	if len(cookies) == 0 {
		fmt.Fprintln(os.Stderr, "未解析到任何 Cookie")
		return 1
	}

	if *raw {
		b, _ := json.MarshalIndent(cookies, "", "  ")
		fmt.Println(string(b))
		return 0
	}

	if *sc {
		for _, c := range cookies {
			fmt.Printf("● %s = %s\n", c.Name, truncate(c.Value, 40))
			fmt.Printf("    Domain=%s Path=%s Expires=%s MaxAge=%s\n",
				orDash(c.Domain), orDash(c.Path), orDash(c.Expires), orDash(c.MaxAge))
			fmt.Printf("    HttpOnly=%v Secure=%v SameSite=%s\n", c.HttpOnly, c.Secure, orDash(c.SameSite))
			warn := []string{}
			if !c.Secure {
				warn = append(warn, "缺少 Secure（非 HTTPS 不传输，但 HTTPS 下也应设置）")
			}
			if !c.HttpOnly {
				warn = append(warn, "缺少 HttpOnly（可被 JS 读取，XSS 可窃取）")
			}
			if c.SameSite == "" {
				warn = append(warn, "缺少 SameSite（CSRF 防护建议 Lax/Strict）")
			}
			for _, w := range warn {
				fmt.Printf("    ⚠ %s\n", w)
			}
			fmt.Println()
		}
		return 0
	}

	fmt.Printf("%-4s %-28s %s\n", "#", "名称", "值")
	for i, c := range cookies {
		fmt.Printf("%-4d %-28s %s\n", i+1, c.Name, truncate(c.Value, 60))
	}
	fmt.Printf("\n共 %d 个\n", len(cookies))
	return 0
}

func parseSetCookie(line string) (cookieAttr, bool) {
	var c cookieAttr
	parts := strings.Split(line, ";")
	if len(parts) == 0 {
		return c, false
	}
	k, v, ok := strings.Cut(strings.TrimSpace(parts[0]), "=")
	if !ok {
		return c, false
	}
	c.Name, c.Value = strings.TrimSpace(k), strings.TrimSpace(v)
	for _, p := range parts[1:] {
		p = strings.TrimSpace(p)
		name, val, _ := strings.Cut(p, "=")
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "domain":
			c.Domain = strings.TrimSpace(val)
		case "path":
			c.Path = strings.TrimSpace(val)
		case "expires":
			c.Expires = strings.TrimSpace(val)
		case "max-age":
			c.MaxAge = strings.TrimSpace(val)
		case "httponly":
			c.HttpOnly = true
		case "secure":
			c.Secure = true
		case "samesite":
			c.SameSite = strings.TrimSpace(val)
		}
	}
	return c, true
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
