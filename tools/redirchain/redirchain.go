// Package redirchain 实现重定向链追踪命令。
// 对应网页版：network/html-congdingxiang-tool.html（网址重定向查询）
package redirchain

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	Name  = "redirchain"
	Desc  = "追踪 URL 重定向链路，查看每一跳与最终落地地址"
	Usage = `用法: lyntoolbox redirchain URL [-max 10] [-k]

参数:
  -max   最大跳数（默认 10）
  -k     跳过 TLS 证书校验`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	max := fs.Int("max", 10, "最大跳数")
	insecure := fs.Bool("k", false, "跳过证书校验")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox redirchain URL")
		return 2
	}
	current := fs.Arg(0)
	if !strings.HasPrefix(current, "http://") && !strings.HasPrefix(current, "https://") {
		current = "https://" + current
	}

	client := &http.Client{
		Timeout: 20 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: *insecure},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	fmt.Printf("起始地址: %s\n\n", current)
	hops := 0
	for hops < *max {
		req, err := http.NewRequest("GET", current, nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, "无效 URL:", err)
			return 2
		}
		req.Header.Set("User-Agent", "LynToolBox/1.0")
		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintln(os.Stderr, "请求失败:", err)
			return 1
		}
		hops++
		loc := resp.Header.Get("Location")
		fmt.Printf("第 %d 跳: %s\n", hops, current)
		fmt.Printf("  状态:   %s\n", resp.Status)
		if loc != "" {
			ref, _ := url.Parse(current)
			next, err := ref.Parse(loc)
			if err != nil {
				io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
				resp.Body.Close()
				fmt.Fprintf(os.Stderr, "错误: 无法解析跳转目标 %q（%v），停止跟踪\n", loc, err)
				return 1
			}
			cross := ""
			if next.Host != "" && ref.Host != next.Host {
				cross = "  ← 跨域跳转"
			}
			fmt.Printf("  跳转至: %s%s\n", next.String(), cross)
			io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			current = next.String()
			continue
		}
		resp.Body.Close()
		fmt.Printf("\n最终落地: %s（共 %d 跳，状态 %s）\n", current, hops, resp.Status)
		return 0
	}
	fmt.Printf("\n超过最大跳数 %d，链路可能存在循环重定向\n", *max)
	return 1
}
