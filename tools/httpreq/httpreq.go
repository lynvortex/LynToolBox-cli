// Package httpreq 实现 HTTP 请求模拟器命令。
// 对应网页版：network/http-test-tool.html（在线HTTP请求模拟）
//
// 用法：
//
//	lyntoolbox httpreq https://example.com/api
//	lyntoolbox httpreq -X POST -H "Content-Type: application/json" -d '{"a":1}' https://example.com
//	lyntoolbox httpreq -d @body.json -o resp.txt -k https://example.com
package httpreq

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	Name  = "httpreq"
	Desc  = "HTTP 请求模拟器，自定义方法/请求头/请求体，格式化输出响应"
	Usage = `用法: lyntoolbox httpreq [-X 方法] [-H "K: V"]... [-d 数据] [-query 查询串] [选项] URL

参数:
  -X        请求方法（默认 GET；带 -d 且未显式指定时自动改为 POST）
  -H        请求头，可重复，如 -H "Content-Type: application/json"
  -d        请求体；@文件路径 表示从文件读取
  -query    附加查询串，如 "a=1&b=2"
  -timeout  超时秒数（默认 30）
  -k        跳过 HTTPS 证书校验
  -o        将响应体保存到文件（不再打印 body）
  -v        打印请求详情（请求行、请求头、请求体）
  -i        打印响应头（默认开启，-i=false 关闭）
  -UA       无；未指定 User-Agent 时默认使用 LynToolBox/1.0
  URL       目标地址`
)

// headerFlags 支持重复出现的 -H
type headerFlags []string

func (h *headerFlags) String() string { return strings.Join(*h, " | ") }
func (h *headerFlags) Set(v string) error {
	*h = append(*h, v)
	return nil
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	method := fs.String("X", "GET", "请求方法")
	var headers headerFlags
	fs.Var(&headers, "H", "请求头，可重复")
	data := fs.String("d", "", "请求体，@文件 表示读文件")
	query := fs.String("query", "", "附加查询串")
	timeoutS := fs.Int("timeout", 30, "超时秒数")
	insecure := fs.Bool("k", false, "跳过证书校验")
	out := fs.String("o", "", "响应体保存文件")
	verbose := fs.Bool("v", false, "打印请求详情")
	showHeaders := fs.Bool("i", true, "打印响应头")
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
		fmt.Fprintln(os.Stderr, "（lyntoolbox httpreq -h 查看帮助）")
		return 2
	}
	if *timeoutS < 1 {
		fmt.Fprintln(os.Stderr, "错误: -timeout 必须 ≥ 1 秒")
		return 2
	}
	target := positionals[0]

	// 方法：显式指定优先；否则 -d 时自动 POST
	methodExplicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "X" {
			methodExplicit = true
		}
	})
	m := strings.ToUpper(*method)
	if *data != "" && !methodExplicit && m == "GET" {
		m = "POST"
	}

	if *query != "" {
		q := strings.TrimPrefix(*query, "?")
		if q == "" {
			fmt.Fprintln(os.Stderr, "错误: -query 查询串为空")
			return 2
		}
		if strings.Contains(target, "?") {
			target += "&" + q
		} else {
			target += "?" + q
		}
	}

	var body []byte
	if *data != "" {
		if strings.HasPrefix(*data, "@") {
			b, err := os.ReadFile(strings.TrimPrefix(*data, "@"))
			if err != nil {
				fmt.Fprintf(os.Stderr, "读取请求体文件失败: %v\n", err)
				return 1
			}
			body = b
		} else {
			body = []byte(*data)
		}
	}

	req, err := http.NewRequest(m, target, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造请求失败（URL 或方法非法）: %v\n", err)
		return 2
	}

	hostOverride := ""
	for _, h := range headers {
		i := strings.Index(h, ":")
		if i <= 0 {
			fmt.Fprintf(os.Stderr, "错误: 请求头格式应为 \"K: V\"，收到: %q\n", h)
			return 2
		}
		k := strings.TrimSpace(h[:i])
		v := strings.TrimSpace(h[i+1:])
		if strings.EqualFold(k, "Host") {
			hostOverride = v
			continue
		}
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "LynToolBox/1.0")
	}
	if len(body) > 0 && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if hostOverride != "" {
		req.Host = hostOverride
	}

	client := &http.Client{
		Timeout: time.Duration(*timeoutS) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: *insecure},
		},
	}

	if *verbose {
		fmt.Printf("> %s %s\n", m, req.URL.RequestURI())
		host := req.Host
		if host == "" {
			host = req.URL.Host
		}
		fmt.Printf("> Host: %s\n", host)
		keys := make([]string, 0, len(req.Header))
		for k := range req.Header {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("> %s: %s\n", k, strings.Join(req.Header[k], ", "))
		}
		if len(body) > 0 {
			fmt.Printf("> \n> %s\n", string(body))
		}
		if *insecure {
			fmt.Println("> （已跳过证书校验）")
		}
		fmt.Println()
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "请求失败: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	elapsed := time.Since(start).Round(time.Millisecond)

	// 状态行
	fmt.Printf("%s %s\n", resp.Proto, resp.Status)
	if *showHeaders {
		keys := make([]string, 0, len(resp.Header))
		for k := range resp.Header {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("%s: %s\n", k, strings.Join(resp.Header[k], ", "))
		}
		fmt.Println()
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取响应体失败: %v\n", err)
		return 1
	}

	if *out != "" {
		if err := os.WriteFile(*out, raw, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "写入文件失败: %v\n", err)
			return 1
		}
		fmt.Printf("响应体已写入 %s（%d 字节，用时 %s）\n", *out, len(raw), elapsed)
		return 0
	}

	outBytes := raw
	if t := bytes.TrimSpace(raw); len(t) > 0 && (t[0] == '{' || t[0] == '[') {
		var buf bytes.Buffer
		if err := json.Indent(&buf, t, "", "  "); err == nil {
			outBytes = buf.Bytes()
			fmt.Fprintln(os.Stderr, "提示: 响应体为 JSON，已自动格式化缩进")
		}
	}
	os.Stdout.Write(outBytes)
	if len(outBytes) > 0 && outBytes[len(outBytes)-1] != '\n' {
		fmt.Println()
	}
	fmt.Fprintf(os.Stderr, "（用时 %s，响应体 %d 字节）\n", elapsed, len(raw))
	return 0
}
