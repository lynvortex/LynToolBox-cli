// Package metatags 实现 Meta/OG 标签生成命令。
// 对应网页版：work/meta-tag-tool.html（Meta/OG标签生成器）
package metatags

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

const (
	Name  = "metatags"
	Desc  = "生成 HTML meta / Open Graph / Twitter 卡片标签块"
	Usage = `用法: lyntoolbox metatags -title 标题 -desc 描述 [-url URL] [-image 图片] [-site 站点名]
                        [-type website|article] [-kw 关键词,逗号分隔] [-locale zh_CN]

参数:
  -title    页面标题（必需）
  -desc     页面描述（必需）
  -url      页面地址（生成 canonical / og:url）
  -image    分享封面图
  -site     站点名称
  -type     og:type（默认 website）
  -kw       关键词（逗号分隔）
  -locale   og:locale（默认 zh_CN）`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	title := fs.String("title", "", "页面标题")
	desc := fs.String("desc", "", "页面描述")
	url := fs.String("url", "", "页面地址")
	image := fs.String("image", "", "封面图")
	site := fs.String("site", "", "站点名")
	typ := fs.String("type", "website", "og:type")
	kw := fs.String("kw", "", "关键词")
	locale := fs.String("locale", "zh_CN", "og:locale")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *title == "" || *desc == "" {
		fmt.Fprintln(os.Stderr, "-title 与 -desc 必需")
		return 2
	}

	var b strings.Builder
	w := func(line string) { b.WriteString("    " + line + "\n") }
	w(fmt.Sprintf(`<meta name="description" content="%s" />`, *desc))
	if *kw != "" {
		w(fmt.Sprintf(`<meta name="keywords" content="%s" />`, *kw))
	}
	if *url != "" {
		w(fmt.Sprintf(`<link rel="canonical" href="%s" />`, *url))
	}
	b.WriteString("\n")
	w(fmt.Sprintf(`<meta property="og:title" content="%s" />`, *title))
	w(fmt.Sprintf(`<meta property="og:description" content="%s" />`, *desc))
	w(fmt.Sprintf(`<meta property="og:type" content="%s" />`, *typ))
	if *url != "" {
		w(fmt.Sprintf(`<meta property="og:url" content="%s" />`, *url))
	}
	if *site != "" {
		w(fmt.Sprintf(`<meta property="og:site_name" content="%s" />`, *site))
	}
	if *image != "" {
		w(fmt.Sprintf(`<meta property="og:image" content="%s" />`, *image))
	}
	w(fmt.Sprintf(`<meta property="og:locale" content="%s" />`, *locale))
	b.WriteString("\n")
	w(`<meta name="twitter:card" content="summary_large_image" />`)
	w(fmt.Sprintf(`<meta name="twitter:title" content="%s" />`, *title))
	w(fmt.Sprintf(`<meta name="twitter:description" content="%s" />`, *desc))
	if *image != "" {
		w(fmt.Sprintf(`<meta name="twitter:image" content="%s" />`, *image))
	}
	b.WriteString("\n")
	w(fmt.Sprintf(`<meta itemprop="name" content="%s" />`, *title))
	w(fmt.Sprintf(`<meta itemprop="description" content="%s" />`, *desc))

	fmt.Print(strings.TrimRight(b.String(), "\n") + "\n")
	return 0
}
