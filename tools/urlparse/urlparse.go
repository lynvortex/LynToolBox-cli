// Package urlparse 实现 URL 解析命令。
// 对应网页版：work/url-parse-tool.html（URL解析）
//
// 用法：
//
//	lyntoolbox urlparse "https://user:pass@example.com:8443/a/b?q=中文&x=1#sec"
//	lyntoolbox urlparse "https://example.com/path?a=1" -json
package urlparse

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
)

const (
	Name  = "urlparse"
	Desc  = "URL 解析：协议/主机/端口/路径分段/查询参数/锚点，支持 JSON 输出"
	Usage = `用法: lyntoolbox urlparse URL [-json]

参数:
  URL    待解析的 URL（建议加引号）
  -json  以 JSON 格式输出解析结果

说明: 端口未显式给出时按协议推断默认端口（http/https/ws/wss/ftp/ssh）；
查询参数自动 URL 解码后逐行输出。`
)

// defaultPorts 常见协议默认端口
var defaultPorts = map[string]string{
	"http": "80", "https": "443", "ws": "80", "wss": "443", "ftp": "21", "ssh": "22",
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "JSON 输出")
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
		fmt.Fprintln(os.Stderr, "错误: 需要且只需要一个 URL 参数")
		fmt.Fprintln(os.Stderr, "（lyntoolbox urlparse -h 查看帮助）")
		return 2
	}
	raw := strings.TrimSpace(positionals[0])

	u, err := url.Parse(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "URL 解析失败: %v\n", err)
		return 2
	}

	port := u.Port()
	portExplicit := port != ""
	inferred := ""
	if !portExplicit && u.Scheme != "" && u.Host != "" {
		if dp, ok := defaultPorts[strings.ToLower(u.Scheme)]; ok {
			inferred = dp
		}
	}

	// 路径分段
	var segments []string
	if clean := strings.Trim(u.Path, "/"); clean != "" {
		for _, s := range strings.Split(clean, "/") {
			if d, err := url.PathUnescape(s); err == nil {
				segments = append(segments, d)
			} else {
				segments = append(segments, s)
			}
		}
	}

	if *asJSON {
		out := struct {
			Scheme       string    `json:"scheme"`
			User         string    `json:"user,omitempty"`
			HasPassword  bool      `json:"hasPassword"`
			Host         string    `json:"host"`
			Port         string    `json:"port"`
			PortExplicit bool      `json:"portExplicit"`
			DefaultPort  string    `json:"defaultPort,omitempty"`
			Path         string    `json:"path"`
			Segments     []string  `json:"segments"`
			RawQuery     string    `json:"rawQuery,omitempty"`
			Query        []queryKv `json:"query"`
			Fragment     string    `json:"fragment,omitempty"`
		}{
			Scheme:       u.Scheme,
			Host:         u.Hostname(),
			Port:         port,
			PortExplicit: portExplicit,
			DefaultPort:  inferred,
			Path:         u.Path,
			Segments:     segments,
			RawQuery:     u.RawQuery,
			Query:        parseQueryKv(u.RawQuery),
			Fragment:     u.Fragment,
		}
		if u.User != nil {
			out.User = u.User.Username()
			_, hasPw := u.User.Password()
			out.HasPassword = hasPw
		}
		if out.Segments == nil {
			out.Segments = []string{}
		}
		if out.Query == nil {
			out.Query = []queryKv{}
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(out); err != nil {
			fmt.Fprintf(os.Stderr, "JSON 编码失败: %v\n", err)
			return 1
		}
		os.Stdout.Write(buf.Bytes())
		return 0
	}

	fmt.Printf("URL:      %s\n", raw)
	if u.Scheme == "" {
		fmt.Println("提示:     输入未包含协议（scheme），已按相对引用解析")
	}
	fmt.Printf("scheme:   %s\n", orDash(u.Scheme))
	if u.User != nil {
		if pw, ok := u.User.Password(); ok {
			fmt.Printf("userinfo: %s（含密码）\n", u.User.Username()+":"+pw)
		} else {
			fmt.Printf("userinfo: %s\n", u.User.Username())
		}
	} else {
		fmt.Println("userinfo: -")
	}
	fmt.Printf("host:     %s\n", orDash(u.Hostname()))
	switch {
	case portExplicit:
		fmt.Printf("port:     %s（显式指定）\n", port)
	case inferred != "":
		fmt.Printf("port:     %s（按 %s 协议默认推断）\n", inferred, u.Scheme)
	default:
		fmt.Println("port:     -")
	}
	fmt.Printf("path:     %s\n", orDash(u.Path))
	if len(segments) > 0 {
		fmt.Printf("  分段:   %s\n", strings.Join(segments, " / "))
	}
	if u.RawQuery != "" {
		fmt.Printf("query:    %s\n", u.RawQuery)
		kvs := parseQueryKv(u.RawQuery)
		if len(kvs) == 0 {
			fmt.Println("  （查询串无法解析为键值对）")
		}
		for _, kv := range kvs {
			fmt.Printf("  %s = %s\n", kv.K, kv.V)
		}
	} else {
		fmt.Println("query:    -")
	}
	fmt.Printf("fragment: %s\n", orDash(u.Fragment))
	return 0
}

type queryKv struct {
	K string `json:"k"`
	V string `json:"v"`
}

// parseQueryKv 按原始顺序解析查询串并解码
func parseQueryKv(raw string) []queryKv {
	if raw == "" {
		return nil
	}
	var out []queryKv
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" {
			continue
		}
		k, v, hasV := strings.Cut(pair, "=")
		kd, err1 := url.QueryUnescape(k)
		if err1 != nil {
			kd = k + "（解码失败）"
		}
		vd := "（无值）"
		if hasV {
			vd = v
			if d, err := url.QueryUnescape(v); err == nil {
				vd = d
			} else {
				vd = v + "（解码失败）"
			}
		}
		out = append(out, queryKv{K: kd, V: vd})
	}
	return out
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
