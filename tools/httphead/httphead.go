// Package httphead 实现 HTTP 响应头分析与安全检查命令。
// 对应网页版：network/http-header-tool.html（HTTP响应头分析）
//
// 用法：
//
//	lyntoolbox httphead https://example.com
package httphead

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	Name  = "httphead"
	Desc  = "获取并分析 HTTP 响应头，逐项解读安全头并提示缺失风险"
	Usage = `用法: lyntoolbox httphead URL

参数:
  URL  目标地址（需含 http:// 或 https://）

说明: 优先发送 HEAD 请求（失败或 4xx/5xx 时自动回退 GET），打印全部响应头，
并对 HSTS、CSP、X-Frame-Options 等安全相关头进行中文解读与缺失警告。`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
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
		fmt.Fprintln(os.Stderr, "错误: 需要且只需要一个 URL 参数")
		fmt.Fprintln(os.Stderr, "（lyntoolbox httphead -h 查看帮助）")
		return 2
	}
	target := positionals[0]
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		fmt.Fprintln(os.Stderr, "错误: URL 需包含 http:// 或 https:// 前缀")
		return 2
	}

	client := &http.Client{Timeout: 30 * time.Second}

	doReq := func(method string) (*http.Response, error) {
		req, err := http.NewRequest(method, target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "LynToolBox/1.0")
		return client.Do(req)
	}

	resp, err := doReq("HEAD")
	fellback := false
	if err != nil || resp.StatusCode >= 400 {
		if resp != nil {
			resp.Body.Close()
		}
		resp, err = doReq("GET")
		fellback = true
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "请求失败: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	fmt.Printf("目标: %s\n", target)
	if fellback {
		fmt.Println("(HEAD 请求失败或被拒绝，已回退 GET)")
	}
	fmt.Printf("最终 URL: %s\n", resp.Request.URL.String())
	fmt.Printf("状态: %s\n\n", resp.Status)

	fmt.Println("== 响应头 ==")
	keys := make([]string, 0, len(resp.Header))
	for k := range resp.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("%s: %s\n", k, strings.Join(resp.Header[k], ", "))
	}
	fmt.Println()

	fmt.Println("== 安全分析 ==")
	analyze(resp)
	return 0
}

// analyze 对安全相关响应头逐项解读
func analyze(resp *http.Response) {
	h := resp.Header
	get := func(k string) string { return strings.TrimSpace(h.Get(k)) }

	// Strict-Transport-Security
	if v := get("Strict-Transport-Security"); v != "" {
		note := "强制浏览器使用 HTTPS，可防止 SSL 剥离攻击"
		if age, ok := maxAge(v); ok && age < 15552000 {
			note += fmt.Sprintf("；当前 max-age=%d 偏短，建议 ≥ 15552000（180 天）", age)
		}
		fmt.Printf("[通过] Strict-Transport-Security: %s\n       %s。\n", v, note)
	} else {
		fmt.Println("[警告] 缺失 Strict-Transport-Security（HSTS）：建议启用，强制 HTTPS 并防降级攻击。")
	}

	// Content-Security-Policy
	if v := get("Content-Security-Policy"); v != "" {
		note := "已配置内容安全策略，可显著降低 XSS 风险"
		if strings.Contains(v, "unsafe-inline") || strings.Contains(v, "unsafe-eval") {
			note += "；但策略中包含 unsafe-inline/unsafe-eval，削弱了防护效果"
		}
		fmt.Printf("[通过] Content-Security-Policy: %s\n       %s。\n", v, note)
	} else {
		fmt.Println("[警告] 缺失 Content-Security-Policy（CSP）：未限制页面可加载的资源，XSS 风险较高。")
	}

	// X-Frame-Options
	switch v := get("X-Frame-Options"); {
	case v == "":
		fmt.Println("[警告] 缺失 X-Frame-Options：页面可被任意 iframe 嵌套，存在点击劫持风险（现代方案: CSP frame-ancestors）。")
	case strings.EqualFold(v, "DENY"):
		fmt.Println("[通过] X-Frame-Options: DENY —— 禁止任何 iframe 嵌套，可防点击劫持。")
	case strings.EqualFold(v, "SAMEORIGIN"):
		fmt.Println("[通过] X-Frame-Options: SAMEORIGIN —— 仅允许同源页面嵌套。")
	case strings.Contains(strings.ToUpper(v), "ALLOW-FROM"):
		fmt.Printf("[提示] X-Frame-Options: %s —— ALLOW-FROM 已废弃，现代浏览器忽略，建议改用 CSP frame-ancestors。\n", v)
	default:
		fmt.Printf("[提示] X-Frame-Options: %s —— 非标准取值，请确认为 DENY 或 SAMEORIGIN。\n", v)
	}

	// X-Content-Type-Options
	if v := get("X-Content-Type-Options"); v != "" {
		if strings.EqualFold(v, "nosniff") {
			fmt.Println("[通过] X-Content-Type-Options: nosniff —— 禁止浏览器 MIME 嗅探，防内容型攻击。")
		} else {
			fmt.Printf("[提示] X-Content-Type-Options: %s —— 建议取值 nosniff。\n", v)
		}
	} else {
		fmt.Println("[警告] 缺失 X-Content-Type-Options：建议设为 nosniff，防止浏览器猜测 MIME 类型。")
	}

	// Referrer-Policy
	if v := get("Referrer-Policy"); v != "" {
		fmt.Printf("[通过] Referrer-Policy: %s —— 控制 Referer 泄露；常用 strict-origin-when-cross-origin。\n", v)
	} else {
		fmt.Println("[警告] 缺失 Referrer-Policy：跳转到外站时默认携带完整 URL，可能泄露用户浏览路径。")
	}

	// Permissions-Policy
	if v := get("Permissions-Policy"); v != "" {
		fmt.Printf("[通过] Permissions-Policy: %s —— 已限制浏览器敏感能力（摄像头/定位等）。\n", v)
	} else {
		fmt.Println("[提示] 未设置 Permissions-Policy：建议按需禁用摄像头/麦克风/定位等敏感 API。")
	}

	// 缓存相关
	cc, exp, etag, lm := get("Cache-Control"), get("Expires"), get("ETag"), get("Last-Modified")
	switch {
	case cc == "" && exp == "" && etag == "" && lm == "":
		fmt.Println("[提示] 未返回缓存相关头（Cache-Control/Expires/ETag/Last-Modified），缓存行为由浏览器启发式决定。")
	default:
		fmt.Println("[提示] 缓存策略:")
		if cc != "" {
			fmt.Printf("         Cache-Control: %s\n", cc)
		}
		if exp != "" {
			fmt.Printf("         Expires: %s\n", exp)
		}
		if etag != "" {
			fmt.Printf("         ETag: %s\n", etag)
		}
		if lm != "" {
			fmt.Printf("         Last-Modified: %s\n", lm)
		}
		if strings.Contains(strings.ToLower(cc), "no-store") || strings.Contains(strings.ToLower(cc), "no-cache") {
			fmt.Println("         说明: 响应被要求不缓存/每次校验，适合敏感内容。")
		}
	}

	// 信息泄露
	if v := get("Server"); v != "" {
		fmt.Printf("[提示] Server: %s —— 暴露服务器软件信息，建议隐藏或弱化。\n", v)
	}
	if v := get("X-Powered-By"); v != "" {
		fmt.Printf("[提示] X-Powered-By: %s —— 暴露后端框架/语言版本，建议移除该响应头。\n", v)
	}

	// Set-Cookie 安全属性
	cookies := resp.Cookies()
	for _, c := range cookies {
		analyzeCookie(c)
	}
	fmt.Println("[完成] 安全头分析结束（[警告] 为缺失项，建议尽快补齐）。")
}

// analyzeCookie 解析单个 Set-Cookie 的安全属性
func analyzeCookie(c *http.Cookie) {
	raw := c.Raw
	parts := strings.Split(raw, ";")
	has := func(attr string) bool {
		for _, p := range parts[1:] {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(p)), attr) {
				return true
			}
		}
		return false
	}
	secure := has("secure")
	httpOnly := has("httponly")
	sameSite := has("samesite")
	fmt.Printf("[提示] Set-Cookie: %s=...\n", c.Name)
	if secure {
		fmt.Println("         Secure: 已设置（仅经 HTTPS 传输）")
	} else {
		fmt.Println("         Secure: 未设置 —— 建议设置，防止明文传输被窃听")
	}
	if httpOnly {
		fmt.Println("         HttpOnly: 已设置（JS 无法读取，防 XSS 窃取）")
	} else {
		fmt.Println("         HttpOnly: 未设置 —— 若非必要请设置，防止脚本读取 Cookie")
	}
	if sameSite {
		fmt.Println("         SameSite: 已设置（可限制跨站携带，防 CSRF）")
	} else {
		fmt.Println("         SameSite: 未设置 —— 现代浏览器默认按 Lax 处理，建议显式声明")
	}
	if has("domain") {
		fmt.Println("         Domain: 已指定作用域")
	}
	if has("max-age") || has("expires") {
		fmt.Println("         有效期: 已指定")
	} else {
		fmt.Println("         有效期: 未指定（会话 Cookie，关闭浏览器即失效）")
	}
}

// maxAge 从 HSTS 值中解析 max-age
func maxAge(v string) (int, bool) {
	lower := strings.ToLower(v)
	i := strings.Index(lower, "max-age=")
	if i < 0 {
		return 0, false
	}
	s := v[i+len("max-age="):]
	if j := strings.Index(s, ";"); j >= 0 {
		s = s[:j]
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return n, true
}
