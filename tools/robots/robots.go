// Package robots 实现 robots.txt 生成命令。
// 对应网页版：work/robots-tool.html（robots.txt生成器）
package robots

import (
	"flag"
	"fmt"
	"strings"
)

const (
	Name  = "robots"
	Desc  = "可视化参数生成 robots.txt"
	Usage = `用法: lyntoolbox robots [-agent "*"] [-disallow /admin] [-allow /public] [-sitemap URL] [-delay 5]

参数:
  -agent     User-agent，逗号分隔多个（默认 *）
  -disallow  禁止路径，可重复（默认 /）
  -allow     允许路径，可重复
  -sitemap   Sitemap 地址，可重复
  -delay     Crawl-delay 秒数`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	agent := fs.String("agent", "*", "User-agent 列表")
	var disallows, allows, sitemaps stringSlice
	fs.Var(&disallows, "disallow", "禁止路径（可重复）")
	fs.Var(&allows, "allow", "允许路径（可重复）")
	fs.Var(&sitemaps, "sitemap", "Sitemap 地址（可重复）")
	delay := fs.Int("delay", 0, "Crawl-delay 秒")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if len(disallows) == 0 && len(allows) == 0 {
		disallows = []string{"/"}
	}

	var b strings.Builder
	for _, a := range strings.Split(*agent, ",") {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		b.WriteString("User-agent: " + a + "\n")
		for _, d := range disallows {
			b.WriteString("Disallow: " + d + "\n")
		}
		for _, al := range allows {
			b.WriteString("Allow: " + al + "\n")
		}
		if *delay > 0 {
			fmt.Fprintf(&b, "Crawl-delay: %d\n", *delay)
		}
		b.WriteString("\n")
	}
	for _, s := range sitemaps {
		b.WriteString("Sitemap: " + s + "\n")
	}
	fmt.Print(b.String())
	return 0
}

// stringSlice 支持 -flag 可重复。
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}
