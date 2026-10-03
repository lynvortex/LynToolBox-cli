// Package corschk 实现 CORS 跨域策略检测命令。
// 对应网页版：network/cors-check-tool.html（CORS 检测）
package corschk

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	Name  = "corschk"
	Desc  = "CORS 跨域策略检测（反射/通配/凭据组合判定）"
	Usage = `用法: lyntoolbox corschk URL [-origin https://evil.example] [-method OPTIONS]

参数:
  -origin  自定义 Origin（默认 https://evil.example.com）
  -method  探测方法（默认 OPTIONS，失败回退 GET）`
)

type result struct {
	method string
	status int
	acao   string
	acam   string
	acah   string
	acac   string
	acma   string
	vary   string
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	origin := fs.String("origin", "https://evil.example.com", "自定义 Origin")
	method := fs.String("method", "OPTIONS", "探测方法")
	insecure := fs.Bool("k", false, "跳过证书校验")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox corschk URL")
		return 2
	}
	target := fs.Arg(0)
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = "https://" + target
	}

	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: *insecure},
	}}

	do := func(m string) (*result, error) {
		req, err := http.NewRequest(m, target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "LynToolBox/1.0")
		req.Header.Set("Origin", *origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return &result{
			method: m, status: resp.StatusCode,
			acao: resp.Header.Get("Access-Control-Allow-Origin"),
			acam: resp.Header.Get("Access-Control-Allow-Methods"),
			acah: resp.Header.Get("Access-Control-Allow-Headers"),
			acac: resp.Header.Get("Access-Control-Allow-Credentials"),
			acma: resp.Header.Get("Access-Control-Max-Age"),
			vary: resp.Header.Get("Vary"),
		}, nil
	}

	r, lastErr := do(*method)
	fallback := false
	if lastErr != nil || (r != nil && r.status >= 400 && r.status != 403) {
		r2, err2 := do("GET")
		if err2 != nil {
			lastErr = err2
		}
		if r2 != nil && (r == nil || r2.status < 400) {
			r = r2
			fallback = true
			lastErr = nil
		}
	}
	if r == nil {
		fmt.Fprintf(os.Stderr, "请求失败: %v\n", lastErr)
		return 1
	}

	fmt.Printf("目标:     %s\n", target)
	fmt.Printf("Origin:   %s\n", *origin)
	fmt.Printf("方法:     %s（状态 %d）%s\n", r.method, r.status, map[bool]string{true: "（回退 GET）", false: ""}[fallback])
	fmt.Println()
	fmt.Printf("Access-Control-Allow-Origin:      %s\n", orDash(r.acao))
	fmt.Printf("Access-Control-Allow-Methods:     %s\n", orDash(r.acam))
	fmt.Printf("Access-Control-Allow-Headers:     %s\n", orDash(r.acah))
	fmt.Printf("Access-Control-Allow-Credentials: %s\n", orDash(r.acac))
	fmt.Printf("Access-Control-Max-Age:           %s\n", orDash(r.acma))
	fmt.Printf("Vary:                             %s\n", orDash(r.vary))
	fmt.Println()

	// 判定
	switch {
	case r.acao == "":
		fmt.Println("判定: 未启用 CORS —— 浏览器将拦截跨域读取（服务端自身 API 调用不受影响）")
	case r.acao == "*":
		if r.acac == "true" {
			fmt.Println("判定: ⚠ ACAO=* 且允许凭据 —— 该组合被浏览器禁止，凭据请求实际会失败")
		} else {
			fmt.Println("判定: 通配符 * —— 任意站点均可读取响应（公开 API 可接受，私有接口不建议）")
		}
	case r.acao == *origin:
		if r.acac == "true" {
			fmt.Println("判定: 🔴 高危 —— Origin 反射 + Allow-Credentials=true")
			fmt.Println("      任意来源都可携带用户 Cookie 读取响应，等同于 CSRF + 数据窃取组合")
		} else {
			fmt.Println("判定: ⚠ Origin 反射 —— 跨域可读，配合凭据接口风险中等")
		}
	default:
		fmt.Printf("判定: 白名单模式 —— 仅允许 %s\n", r.acao)
	}
	return 0
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
