// Package uaparse 实现 User-Agent 解析命令。
// 对应网页版：network/ua-parse-tool.html（UA解析）
//
// 用法：
//
//	lyntoolbox uaparse "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
//	lyntoolbox uaparse -f ua.txt
package uaparse

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	Name  = "uaparse"
	Desc  = "User-Agent 解析：浏览器/引擎/操作系统/设备类型（含爬虫识别）"
	Usage = `用法: lyntoolbox uaparse "UA 字符串"
   或: lyntoolbox uaparse -f 文件        （每行一个 UA）

参数:
  -f     从文件批量解析，每行一个 UA
  -json  以 JSON 格式输出（批量时逐行输出一个 JSON 对象）
  UA     待解析的 User-Agent 字符串（建议加引号）`
)

var (
	reEdge    = regexp.MustCompile(`Edg(?:e|A|iOS)?/([0-9.]+)`)
	reOPR     = regexp.MustCompile(`OPR/([0-9.]+)`)
	reOpera   = regexp.MustCompile(`Opera[ /]([0-9.]+)`)
	reSamsung = regexp.MustCompile(`SamsungBrowser/([0-9.]+)`)
	reWeChat  = regexp.MustCompile(`MicroMessenger/([0-9.]+)`)
	reQQ      = regexp.MustCompile(`QQBrowser/([0-9.]+)`)
	reMQQ     = regexp.MustCompile(`MQQBrowser/([0-9.]+)`)
	reUC      = regexp.MustCompile(`(?:UCBrowser|UCWEB)/([0-9.]+)`)
	reFirefox = regexp.MustCompile(`Firefox/([0-9.]+)`)
	reMSIE    = regexp.MustCompile(`MSIE ([0-9.]+)`)
	reRV      = regexp.MustCompile(`rv:([0-9.]+)`)
	reChrome  = regexp.MustCompile(`Chrome/([0-9.]+)`)
	reSafari  = regexp.MustCompile(`Version/([0-9.]+).*Safari/`)
	reTrident = regexp.MustCompile(`Trident/([0-9.]+)`)
	reWinNT   = regexp.MustCompile(`Windows NT ([0-9.]+)`)
	reMacOSX  = regexp.MustCompile(`Mac OS X[ /]([0-9_.]+)`)
	reAndroid = regexp.MustCompile(`Android[ /]([0-9.]+)`)
	reIOS     = regexp.MustCompile(`OS ([0-9_]+) like Mac OS X`)
	reHarm    = regexp.MustCompile(`(?:Open)?HarmonyOS[ /]?([0-9.]+)?`)
	reCurl    = regexp.MustCompile(`(?i)\b(curl|wget|python-requests|okhttp|Go-http-client|libwww|axios)/([0-9.]+)`)
)

// winNames Windows NT 版本映射
var winNames = map[string]string{
	"10.0": "Windows 10/11", "6.3": "Windows 8.1", "6.2": "Windows 8",
	"6.1": "Windows 7", "6.0": "Windows Vista", "5.2": "Windows XP x64/2003",
	"5.1": "Windows XP", "5.0": "Windows 2000",
}

// bots 已知爬虫名单（按顺序匹配）
var bots = []struct {
	re   *regexp.Regexp
	name string
}{
	{regexp.MustCompile(`(?i)googlebot|google-inspectiontool|apis-google|mediapartners-google`), "Googlebot"},
	{regexp.MustCompile(`(?i)bingbot|bingpreview`), "Bingbot"},
	{regexp.MustCompile(`(?i)baiduspider`), "Baiduspider"},
	{regexp.MustCompile(`(?i)sogou`), "Sogou spider"},
	{regexp.MustCompile(`(?i)360spider|qihoo`), "360Spider"},
	{regexp.MustCompile(`(?i)yisouspider`), "YisouSpider"},
	{regexp.MustCompile(`(?i)bytespider`), "Bytespider"},
	{regexp.MustCompile(`(?i)petalbot`), "PetalBot"},
	{regexp.MustCompile(`(?i)yandexbot|yandeximages`), "YandexBot"},
	{regexp.MustCompile(`(?i)duckduckbot`), "DuckDuckBot"},
	{regexp.MustCompile(`(?i)applebot`), "Applebot"},
	{regexp.MustCompile(`(?i)ahrefsbot`), "AhrefsBot"},
	{regexp.MustCompile(`(?i)semrushbot`), "SemrushBot"},
	{regexp.MustCompile(`(?i)mj12bot`), "MJ12bot"},
	{regexp.MustCompile(`(?i)dotbot`), "DotBot"},
	{regexp.MustCompile(`(?i)facebookexternalhit|meta-externalagent`), "Facebook/Meta crawler"},
	{regexp.MustCompile(`(?i)twitterbot`), "Twitterbot"},
	{regexp.MustCompile(`(?i)linkedinbot`), "LinkedInBot"},
	{regexp.MustCompile(`(?i)slackbot`), "Slackbot"},
	{regexp.MustCompile(`(?i)whatsapp`), "WhatsApp crawler"},
	{regexp.MustCompile(`(?i)telegrambot`), "TelegramBot"},
	{regexp.MustCompile(`(?i)discordbot`), "Discordbot"},
	{regexp.MustCompile(`(?i)scrapy`), "Scrapy"},
	{regexp.MustCompile(`(?i)spider|crawler|crawl|slurp|archiver`), "通用爬虫（spider/crawler/slurp）"},
	{regexp.MustCompile(`(?i)python|java/|httpclient|libwww|node-fetch|dart:io`), "程序化 HTTP 客户端"},
}

// result 解析结果
type result struct {
	Browser     string `json:"browser"`
	BrowserVer  string `json:"browserVersion,omitempty"`
	Engine      string `json:"engine,omitempty"`
	OS          string `json:"os,omitempty"`
	OSVersion   string `json:"osVersion,omitempty"`
	Device      string `json:"device"`
	Crawler     bool   `json:"crawler"`
	CrawlerName string `json:"crawlerName,omitempty"`
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	file := fs.String("f", "", "批量解析文件（每行一个 UA）")
	asJSON := fs.Bool("json", false, "JSON 输出")
	fs.Usage = func() { fmt.Print(Usage + "\n") }

	// 允许位置参数后仍跟选项（如: uaparse "Mozilla/..." -json）
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

	var uas []string
	switch {
	case *file != "":
		b, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取文件失败: %v\n", err)
			return 1
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				uas = append(uas, line)
			}
		}
		if len(uas) == 0 {
			fmt.Fprintln(os.Stderr, "错误: 文件中没有非空行")
			return 1
		}
	case len(positionals) > 0:
		uas = []string{strings.Join(positionals, " ")}
	default:
		fmt.Fprintln(os.Stderr, "错误: 需要一个 UA 字符串或 -f 文件")
		fmt.Fprintln(os.Stderr, "（lyntoolbox uaparse -h 查看帮助）")
		return 2
	}

	if len(uas) > 1 {
		for i, ua := range uas {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("== 第 %d 行 ==\n", i+1)
			parseOne(ua, *asJSON)
		}
		return 0
	}
	parseOne(uas[0], *asJSON)
	return 0
}

// parseOne 解析并输出单个 UA
func parseOne(ua string, asJSON bool) {
	r := parseUA(ua)
	if asJSON {
		b, err := json.MarshalIndent(r, "", "  ")
		if err == nil {
			fmt.Println(string(b))
			return
		}
	}
	fmt.Printf("UA:      %s\n", ua)
	browser := r.Browser
	if r.BrowserVer != "" {
		browser += " " + r.BrowserVer
	}
	fmt.Printf("浏览器:  %s\n", browser)
	fmt.Printf("引擎:    %s\n", orDash(r.Engine))
	fmt.Printf("操作系统: %s\n", orDash(r.OS))
	fmt.Printf("设备类型: %s\n", r.Device)
	if r.Crawler {
		fmt.Printf("爬虫:    是（%s）\n", r.CrawlerName)
	}
}

// parseUA 解析 UA 字符串
func parseUA(ua string) result {
	r := result{}

	// 爬虫识别优先
	for _, b := range bots {
		if b.re.MatchString(ua) {
			r.Crawler = true
			r.CrawlerName = b.name
			break
		}
	}

	r.parseBrowser(ua)
	r.parseEngine(ua)
	r.parseOS(ua)
	r.Device = deviceType(ua, r.Crawler)
	return r
}

func (r *result) parseBrowser(ua string) {
	switch {
	case reWeChat.MatchString(ua):
		r.Browser, r.BrowserVer = "微信内置浏览器", reWeChat.FindStringSubmatch(ua)[1]
		return
	case reMQQ.MatchString(ua):
		r.Browser, r.BrowserVer = "手机QQ浏览器", reMQQ.FindStringSubmatch(ua)[1]
		return
	case reQQ.MatchString(ua):
		r.Browser, r.BrowserVer = "QQ浏览器", reQQ.FindStringSubmatch(ua)[1]
		return
	case reUC.MatchString(ua):
		r.Browser, r.BrowserVer = "UC浏览器", reUC.FindStringSubmatch(ua)[1]
		return
	case reEdge.MatchString(ua):
		r.Browser, r.BrowserVer = "Microsoft Edge", reEdge.FindStringSubmatch(ua)[1]
		return
	case reOPR.MatchString(ua):
		r.Browser, r.BrowserVer = "Opera", reOPR.FindStringSubmatch(ua)[1]
		return
	case reOpera.MatchString(ua):
		r.Browser, r.BrowserVer = "Opera", reOpera.FindStringSubmatch(ua)[1]
		return
	case reSamsung.MatchString(ua):
		r.Browser, r.BrowserVer = "三星浏览器", reSamsung.FindStringSubmatch(ua)[1]
		return
	case reFirefox.MatchString(ua):
		r.Browser, r.BrowserVer = "Firefox", reFirefox.FindStringSubmatch(ua)[1]
		return
	case reMSIE.MatchString(ua):
		r.Browser, r.BrowserVer = "Internet Explorer", reMSIE.FindStringSubmatch(ua)[1]
		return
	case reTrident.MatchString(ua) && reRV.MatchString(ua):
		r.Browser, r.BrowserVer = "Internet Explorer", reRV.FindStringSubmatch(ua)[1]
		return
	case reChrome.MatchString(ua):
		r.Browser, r.BrowserVer = "Chrome", reChrome.FindStringSubmatch(ua)[1]
		return
	case reSafari.MatchString(ua):
		r.Browser, r.BrowserVer = "Safari", reSafari.FindStringSubmatch(ua)[1]
		return
	case strings.Contains(ua, "Safari/"):
		r.Browser = "Safari（旧版）"
		return
	}
	if m := reCurl.FindStringSubmatch(ua); m != nil {
		r.Browser, r.BrowserVer = "命令行客户端 "+m[1], m[2]
		return
	}
	r.Browser = "未知"
}

func (r *result) parseEngine(ua string) {
	switch {
	case reTrident.MatchString(ua):
		r.Engine = "Trident"
	case reFirefox.MatchString(ua):
		r.Engine = "Gecko"
	case reChrome.MatchString(ua) || reEdge.MatchString(ua) || reOPR.MatchString(ua):
		r.Engine = "Blink"
	case strings.Contains(ua, "AppleWebKit/"):
		r.Engine = "WebKit"
	case strings.Contains(ua, "Gecko/"):
		r.Engine = "Gecko"
	}
}

func (r *result) parseOS(ua string) {
	if m := reHarm.FindStringSubmatch(ua); m != nil && strings.Contains(ua, "HarmonyOS") {
		r.OS = "HarmonyOS"
		r.OSVersion = m[1]
		return
	}
	if m := reWinNT.FindStringSubmatch(ua); m != nil {
		r.OS = winNames[m[1]]
		if r.OS == "" {
			r.OS = "Windows（NT " + m[1] + "）"
		}
		r.OSVersion = "NT " + m[1]
		return
	}
	isAppleMobile := strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad") || strings.Contains(ua, "iPod")
	if isAppleMobile {
		if m := reIOS.FindStringSubmatch(ua); m != nil {
			ver := strings.ReplaceAll(m[1], "_", ".")
			if strings.Contains(ua, "iPad") {
				r.OS = "iPadOS"
			} else {
				r.OS = "iOS"
			}
			r.OSVersion = ver
			return
		}
		r.OS = "iOS/iPadOS"
		return
	}
	if strings.Contains(ua, "Macintosh") || reMacOSX.MatchString(ua) {
		if m := reMacOSX.FindStringSubmatch(ua); m != nil {
			r.OS = "macOS"
			r.OSVersion = strings.ReplaceAll(m[1], "_", ".")
		} else {
			r.OS = "macOS"
		}
		return
	}
	if m := reAndroid.FindStringSubmatch(ua); m != nil {
		r.OS = "Android"
		r.OSVersion = m[1]
		return
	}
	if strings.Contains(ua, "Linux") {
		r.OS = "Linux"
		return
	}
	if strings.Contains(ua, "Windows Phone") {
		r.OS = "Windows Phone"
		return
	}
}

// deviceType 判定设备类型
func deviceType(ua string, isBot bool) string {
	if isBot {
		return "爬虫"
	}
	if strings.Contains(ua, "iPad") || strings.Contains(ua, "Tablet") ||
		strings.Contains(ua, "Kindle") || strings.Contains(ua, "Silk/") || strings.Contains(ua, "PlayBook") {
		return "平板"
	}
	if strings.Contains(ua, "Android") {
		if strings.Contains(ua, "Mobile") {
			return "手机"
		}
		return "平板"
	}
	if strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPod") ||
		strings.Contains(ua, "Mobile") || strings.Contains(ua, "Windows Phone") || strings.Contains(ua, "IEMobile") {
		return "手机"
	}
	if strings.Contains(ua, "Smart-TV") || strings.Contains(ua, "SmartTV") ||
		strings.Contains(ua, "AppleTV") || strings.Contains(ua, "GoogleTV") {
		return "电视"
	}
	if strings.Contains(ua, "Windows NT") || strings.Contains(ua, "Macintosh") ||
		strings.Contains(ua, "X11") || strings.Contains(ua, "Linux") || strings.Contains(ua, "CrOS") {
		return "桌面"
	}
	return "未知"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
