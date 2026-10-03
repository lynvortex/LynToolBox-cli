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
	"hash":         {{flag: "", msg: "要计算哈希的文本", req: true}, {flag: "file", msg: "或改算文件路径（填了此项忽略上文）"}},
	"base64":       {{flag: "", msg: "要编码/解码的内容", req: true}, {flag: "d", msg: "解码模式", isBool: true}, {flag: "url", msg: "URL 安全字母表", isBool: true}, {flag: "file", msg: "改为读取文件路径"}},
	"urlenc":       {{flag: "", msg: "要编码/解码的文本", req: true}, {flag: "d", msg: "解码模式", isBool: true}},
	"hexenc":       {{flag: "", msg: "要转换的文本", req: true}, {flag: "d", msg: "从十六进制解码", isBool: true}, {flag: "upper", msg: "输出大写", isBool: true}},
	"base58":       {{flag: "", msg: "要转换的文本", req: true}, {flag: "btc", msg: "Base58Check（比特币格式）", isBool: true}},
	"htmlent":      {{flag: "", msg: "要处理的 HTML 片段", req: true}, {flag: "d", msg: "解码模式", isBool: true}, {flag: "num", msg: "非 ASCII 输出数字实体", isBool: true}},
	"gzipx":        {{flag: "", msg: "要压缩的文本", req: true}, {flag: "d", msg: "解压模式（输入 Base64）", isBool: true}},
	"shesc":        {{flag: "", msg: "要转义的字符串", req: true}, {flag: "mode", msg: "转义风格 posix|cmd|powershell", def: "posix"}},
	"morse":        {{flag: "", msg: "要转换的文本", req: true}, {flag: "d", msg: "解码模式", isBool: true}},
	"aes":          {{flag: "", msg: "要加密/解密的文本", req: true}, {flag: "key", msg: "密钥（UTF-8）", req: true}, {flag: "iv", msg: "IV（CBC/CFB 必填）"}, {flag: "mode", msg: "模式 cbc|ecb|cfb", def: "cbc"}, {flag: "d", msg: "解密模式", isBool: true}},
	"smcrypto":     {{flag: "", msg: "子命令 sm3|sm4|sm2", req: true, first: true}, {flag: "", msg: "要处理的文本（sm3 哈希 / sm4 加解密）"}},
	"hmac":         {{flag: "", msg: "要签名的文本", req: true}, {flag: "k", msg: "密钥", req: true}, {flag: "algo", msg: "算法（默认 sha256）"}},
	"jwt":          {{flag: "", msg: "JWT Token", req: true}, {flag: "exp", msg: "检查过期时间", isBool: true}, {flag: "key", msg: "HS256 验签密钥（可选）"}},
	"rsagen":       {{flag: "bits", msg: "密钥位数 1024|2048|4096", def: "2048"}, {flag: "pkcs8", msg: "私钥用 PKCS#8 格式", isBool: true}},
	"pbkdf2":       {{flag: "pass", msg: "密码", req: true}, {flag: "iter", msg: "迭代次数", def: "10000"}, {flag: "len", msg: "派生长度（字节）", def: "32"}, {flag: "algo", msg: "算法 sha256|sha512|sha1", def: "sha256"}},
	"totp":         {{flag: "secret", msg: "Base32 密钥", req: true}, {flag: "digits", msg: "位数 6|8", def: "6"}, {flag: "period", msg: "周期（秒）", def: "30"}},
	"randgen":      {{flag: "", msg: "类型 uuid|password|token|number", req: true, first: true}, {flag: "len", msg: "长度（默认 16）"}, {flag: "n", msg: "数量（默认 5）"}},
	"gitignore":    {{flag: "stack", msg: "技术栈（逗号分隔，如 go,node）", def: "go,node"}},
	"robots":       {{flag: "disallow", msg: "禁止路径", def: "/"}, {flag: "allow", msg: "允许路径"}, {flag: "sitemap", msg: "Sitemap 地址"}},
	"nginxconf":    {{flag: "domain", msg: "域名（_ 表示全部）", def: "_"}, {flag: "proxy", msg: "反代地址（如 http://127.0.0.1:3000）"}, {flag: "root", msg: "静态站点根目录"}, {flag: "gzip", msg: "启用 gzip", isBool: true}},
	"dockerfile":   {{flag: "stack", msg: "技术栈 go|node|python|java|static", req: true}, {flag: "port", msg: "监听端口", def: "8080"}, {flag: "multistage", msg: "多阶段构建", isBool: true}},
	"metatags":     {{flag: "title", msg: "页面标题", req: true}, {flag: "desc", msg: "页面描述", req: true}, {flag: "url", msg: "页面地址"}, {flag: "image", msg: "分享封面图"}},
	"lorem":        {{flag: "what", msg: "类型 text|name|phone|email|idcard", def: "text"}, {flag: "n", msg: "数量", def: "3"}},
	"figlet":       {{flag: "", msg: "横幅文本（A-Z 0-9）", req: true}},
	"qrcode":       {{flag: "", msg: "二维码内容", req: true}, {flag: "o", msg: "输出 PNG 路径（留空终端显示）"}},
	"ping":         {{flag: "", msg: "目标域名或 IP", req: true}, {flag: "c", msg: "次数", def: "4"}},
	"portscan":     {{flag: "", msg: "目标主机", req: true}, {flag: "p", msg: "端口列表（如 80,443；留空扫常用端口）"}},
	"httpreq":      {{flag: "", msg: "请求 URL", req: true}, {flag: "X", msg: "请求方法", def: "GET"}, {flag: "d", msg: "请求体（@文件 读文件）"}, {flag: "query", msg: "附加查询串"}, {flag: "timeout", msg: "超时秒数", def: "30"}, {flag: "o", msg: "响应体保存文件"}, {flag: "k", msg: "跳过证书校验", isBool: true}, {flag: "v", msg: "打印请求详情", isBool: true}},
	"httphead":     {{flag: "", msg: "目标 URL", req: true}},
	"dnsq":         {{flag: "", msg: "域名", req: true}, {flag: "type", msg: "记录类型 A|AAAA|CNAME|MX|NS|TXT|SRV", def: "A"}},
	"subnet":       {{flag: "", msg: "CIDR（如 192.168.1.0/24）", req: true}},
	"ipchk":        {{flag: "", msg: "IP 地址", req: true}},
	"urlparse":     {{flag: "", msg: "URL", req: true}},
	"uaparse":      {{flag: "", msg: "User-Agent 字符串", req: true}},
	"mime":         {{flag: "", msg: "扩展名（如 pdf）或 MIME 类型", req: true}},
	"macloc":       {{flag: "", msg: "MAC 地址（任意分隔格式）", req: true}},
	"tsconv":       {{flag: "", msg: "时间戳或日期（留空=当前时间）"}},
	"jsonfmt":      {{flag: "", msg: "JSON 文本", req: true}, {flag: "i", msg: "缩进空格数", def: "2"}, {flag: "c", msg: "压缩为单行", isBool: true}, {flag: "k", msg: "只校验不输出", isBool: true}},
	"jsonpath":     {{flag: "", msg: "JSONPath 表达式（如 $.a.b[0]）", req: true, first: true}, {flag: "", msg: "JSON 文本", req: true}},
	"json2code":    {{flag: "", msg: "JSON 文本", req: true}, {flag: "lang", msg: "目标语言 ts|go|java|csharp|python", def: "ts"}, {flag: "name", msg: "根类型名", def: "Root"}},
	"jsonschema":   {{flag: "s", msg: "Schema 文件路径", req: true}, {flag: "", msg: "待校验 JSON 文本", req: true}},
	"yamlconv":     {{flag: "", msg: "YAML 或 JSON 文本", req: true}, {flag: "to", msg: "转换目标 json|yaml（留空=格式化校验）"}, {flag: "c", msg: "只校验", isBool: true}},
	"tomlconv":     {{flag: "", msg: "TOML 或 JSON 文本", req: true}, {flag: "to", msg: "转换目标 json|toml（留空=格式化校验）"}, {flag: "c", msg: "只校验", isBool: true}},
	"xmlfmt":       {{flag: "", msg: "XML 文本", req: true}, {flag: "i", msg: "缩进空格数", def: "2"}, {flag: "c", msg: "压缩单行", isBool: true}, {flag: "k", msg: "只校验", isBool: true}, {flag: "to", msg: "转 JSON（填 json）"}},
	"csvjson":      {{flag: "", msg: "CSV 或 JSON 文本", req: true}, {flag: "to", msg: "转 CSV（填 csv，输入为 JSON 对象数组）"}, {flag: "noheader", msg: "无表头模式", isBool: true}, {flag: "delim", msg: "分隔符", def: ","}},
	"csv2sql":      {{flag: "table", msg: "目标表名", req: true}, {flag: "", msg: "CSV 文本（首行表头）", req: true}, {flag: "dialect", msg: "方言 mysql|pg", def: "mysql"}, {flag: "batch", msg: "合并多 VALUES", isBool: true}, {flag: "create", msg: "附加建表语句", isBool: true}},
	"envconv":      {{flag: "", msg: ".env / JSON / YAML 文本", req: true}, {flag: "from", msg: "输入格式 env|json（默认 env）"}, {flag: "to", msg: "输出格式 json|yaml（留空=.env）"}},
	"sqlfmt":       {{flag: "", msg: "SQL 语句", req: true}, {flag: "i", msg: "缩进空格数", def: "2"}, {flag: "c", msg: "压缩单行", isBool: true}},
	"codefmt":      {{flag: "", msg: "代码（HTML/CSS/JS）", req: true}, {flag: "lang", msg: "语言 html|css|js|auto", def: "auto"}, {flag: "i", msg: "缩进空格数", def: "2"}},
	"minify":       {{flag: "", msg: "代码（CSS/HTML/JS）", req: true}, {flag: "lang", msg: "语言 css|html|js|auto", def: "auto"}},
	"cloc":         {{flag: "", msg: "要统计的文件或目录路径", req: true}, {flag: "exclude", msg: "排除的路径子串", def: "node_modules,.git,vendor,dist"}},
	"mdconv":       {{flag: "", msg: "Markdown 或 HTML 文本", req: true}, {flag: "to", msg: "目标格式 html|md", def: "html"}, {flag: "x", msg: "输出完整 HTML 文档", isBool: true}},
	"redirchain":   {{flag: "", msg: "起始 URL", req: true}, {flag: "max", msg: "最大跳数", def: "10"}},
	"sslcert":      {{flag: "", msg: "域名", req: true}, {flag: "port", msg: "TLS 端口", def: "443"}, {flag: "k", msg: "跳过证书校验", isBool: true}},
	"whois":        {{flag: "", msg: "域名", req: true}, {flag: "clean", msg: "摘要模式（过滤注释）", isBool: true}},
	"wsclient":     {{flag: "", msg: "ws(s):// 地址", req: true}, {flag: "send", msg: "要发送的消息"}, {flag: "listen", msg: "纯监听秒数"}},
	"mqttcli":      {{flag: "", msg: "子命令 sub|pub", req: true, first: true}, {flag: "url", msg: "broker 地址", def: "tcp://127.0.0.1:1883"}, {flag: "sub", msg: "订阅主题（sub 模式）"}, {flag: "pub", msg: "发布主题（pub 模式）"}, {flag: "m", msg: "发布消息（pub 模式）"}},
	"vsrc":         {{flag: "", msg: "网页 URL", req: true}, {flag: "meta", msg: "摘要模式", isBool: true}, {flag: "fmt", msg: "重缩进美化", isBool: true}},
	"httpcode":     {{flag: "", msg: "状态码 / 类码 / all（留空看目录）"}},
	"cookiep":      {{flag: "", msg: "Cookie 字符串", req: true}, {flag: "sc", msg: "Set-Cookie 响应头模式", isBool: true}},
	"corschk":      {{flag: "", msg: "目标 URL", req: true}, {flag: "origin", msg: "自定义 Origin（默认 evil.example.com）"}},
	"zipx":         {{flag: "", msg: "子命令 pack|unpack|list", req: true, first: true}, {flag: "o", msg: "pack 输出 zip 路径"}, {flag: "d", msg: "unpack 目标目录"}},
	"srt":          {{flag: "", msg: "SRT 字幕文件路径", req: true}, {flag: "shift", msg: "整体偏移（如 +1.5 或 -00:00:02,500）"}, {flag: "scale", msg: "时间缩放倍率（如 1.04）", def: "1"}, {flag: "renumber", msg: "重新编号", isBool: true}, {flag: "o", msg: "输出文件（留空打印）"}},
	"docx2md":      {{flag: "", msg: "Word(.docx) 文件路径", req: true}, {flag: "plain", msg: "纯文本模式", isBool: true}, {flag: "o", msg: "输出文件（留空打印）"}},
	"epubx":        {{flag: "", msg: "子命令 unpack|list|text|meta", req: true, first: true}, {flag: "", msg: "EPUB 文件路径", req: true}, {flag: "d", msg: "unpack 目标目录"}, {flag: "o", msg: "text 输出文件（留空打印）"}},
	"xlsxtool":     {{flag: "", msg: "子命令 j2x|x2c|info", req: true, first: true}, {flag: "", msg: "JSON 数组（j2x）或 xlsx 路径", req: true}, {flag: "o", msg: "输出文件路径"}, {flag: "sheet", msg: "工作表名", def: "Sheet1"}},
	"pdf":          {{flag: "", msg: "子命令 merge|split|rotate|extract|info", req: true, first: true}, {flag: "", msg: "输入 PDF 路径", req: true}, {flag: "o", msg: "输出路径"}, {flag: "pages", msg: "页面选择（如 1-3,5）"}, {flag: "deg", msg: "rotate 旋转角度", def: "90"}},
	"pdfop":        {{flag: "", msg: "子命令 optimize|watermark", req: true, first: true}, {flag: "", msg: "输入 PDF 路径", req: true}, {flag: "o", msg: "输出路径"}, {flag: "text", msg: "水印文字（仅 ASCII）"}, {flag: "opacity", msg: "水印透明度 0-1", def: "0.3"}},
	"textdiff":     {{flag: "", msg: "文本 A（文件路径或直接内容，- 为 stdin）", req: true}, {flag: "", msg: "文本 B", req: true}, {flag: "u", msg: "上下文行数（0=全量）", def: "3"}, {flag: "color", msg: "彩色输出", isBool: true}},
	"sortuniq":     {{flag: "", msg: "文本（每行一条）", req: true}, {flag: "u", msg: "去重", isBool: true}, {flag: "n", msg: "按数值排序", isBool: true}, {flag: "r", msg: "反序", isBool: true}, {flag: "i", msg: "忽略大小写", isBool: true}},
	"textrepl":     {{flag: "", msg: "查找内容", req: true}, {flag: "", msg: "替换为", req: true}, {flag: "regex", msg: "正则模式", isBool: true}, {flag: "i", msg: "忽略大小写", isBool: true}},
	"extract":      {{flag: "", msg: "待提取文本", req: true}, {flag: "what", msg: "类型 url|email|phone|ipv4|ipv6|idcard|all", def: "all"}},
	"maskdata":     {{flag: "", msg: "待打码文本", req: true}, {flag: "mode", msg: "模式 partial|full|keep1", def: "partial"}},
	"tabspace":     {{flag: "", msg: "文本", req: true}, {flag: "to", msg: "目标 spaces|tabs", def: "spaces"}, {flag: "w", msg: "宽度", def: "4"}},
	"slug":         {{flag: "", msg: "标题文本（支持中文）", req: true}, {flag: "sep", msg: "分隔符", def: "-"}, {flag: "max", msg: "最大长度"}},
	"zhconv":       {{flag: "", msg: "子命令 s2t|t2s|pinyin", req: true, first: true}, {flag: "", msg: "文本", req: true}},
	"basecalc":     {{flag: "", msg: "子命令：直接转换填空，位运算填 and|or|xor|not|shl|shr", first: true}, {flag: "", msg: "数值（第二个参数）"}, {flag: "from", msg: "输入进制 2-36", def: "10"}, {flag: "to", msg: "输出进制 2-36", def: "10"}},
	"bignum":       {{flag: "", msg: "表达式（如 0.1+0.2）", req: true}, {flag: "prec", msg: "除法小数位", def: "20"}},
	"calc":         {{flag: "", msg: "表达式（如 sin(pi/2)+2^10）", req: true}, {flag: "deg", msg: "角度制", isBool: true}},
	"snowflake":    {{flag: "", msg: "子命令 generate|parse", req: true, first: true}, {flag: "", msg: "雪花 ID（parse 模式）"}, {flag: "n", msg: "生成数量", def: "5"}, {flag: "epoch", msg: "纪元毫秒（默认 Twitter）", def: "1288834974657"}},
	"semver":       {{flag: "", msg: "子命令 parse|compare|check|sort", req: true, first: true}, {flag: "", msg: "版本号（空格分隔多个）", req: true}},
	"regext":       {{flag: "p", msg: "正则表达式", req: true}, {flag: "", msg: "测试文本", req: true}, {flag: "replace", msg: "替换串（支持 $1）"}},
	"cronx":        {{flag: "", msg: "子命令 parse|gen", req: true, first: true}, {flag: "", msg: "Cron 表达式（parse）或 -min 等参数（gen）"}, {flag: "next", msg: "未来 N 次触发", def: "5"}},
	"stats":        {{flag: "", msg: "数字（空格分隔）", req: true}, {flag: "xy", msg: "按两列做相关与回归", isBool: true}},
	"wordcount":    {{flag: "", msg: "文本（或 -f 文件路径）", req: true}},
	"shufflelines": {{flag: "", msg: "文本（每行一条）", req: true}, {flag: "mode", msg: "模式 line|word|char", def: "line"}, {flag: "seed", msg: "随机种子（可复现）"}, {flag: "n", msg: "抽取条数"}},
	"encconv":      {{flag: "", msg: "输入文件路径（留空读终端输入）"}, {flag: "from", msg: "源编码 auto|utf-8|gbk|gb18030|big5|shift-jis|euc-jp", def: "auto"}, {flag: "to", msg: "目标编码", def: "utf-8"}},
	"hexdump":      {{flag: "", msg: "文件路径（留空读终端输入）"}, {flag: "o", msg: "起始偏移"}, {flag: "l", msg: "读取长度"}},
	"validate":     {{flag: "", msg: "待校验内容（每行一条）", req: true}, {flag: "t", msg: "类型 email|ipv4|phone|idcard|creditcard|url|uuid|auto", def: "auto"}, {flag: "info", msg: "身份证解析详情", isBool: true}},
	"datecalc":     {{flag: "", msg: "子命令 diff|add|weekday|rrule|secs", req: true, first: true}},
	"unitconv":     {{flag: "", msg: "数值", req: true}, {flag: "from", msg: "源单位（如 m）", req: true}, {flag: "to", msg: "目标单位（如 km）", req: true}},
	"colorcalc":    {{flag: "", msg: "子命令 conv|contrast|mix|shades|lighten|darken", req: true, first: true}},
	"geodist":      {{flag: "", msg: "点1（纬度,经度）", req: true}, {flag: "", msg: "点2（纬度,经度）", req: true}},
	"wavtool":      {{flag: "", msg: "子命令 info|cut|concat|gain|normalize|pan|speed|fade", req: true, first: true}, {flag: "", msg: "WAV 文件路径", req: true}, {flag: "o", msg: "输出文件路径", req: true}},
	"svgmin":       {{flag: "", msg: "SVG 文件路径或内容", req: true}, {flag: "prec", msg: "数值精度", def: "3"}},
	"imgconv":      {{flag: "", msg: "图片文件路径（与 -dir 二选一）", req: true}, {flag: "to", msg: "目标格式 png|jpg|gif|bmp|tiff"}, {flag: "w", msg: "目标宽度（等比）"}, {flag: "h", msg: "目标高度（等比）"}, {flag: "scale", msg: "等比缩放百分比", def: "100"}, {flag: "q", msg: "JPEG 质量 1-100", def: "85"}, {flag: "dir", msg: "输入目录（批量）"}, {flag: "outdir", msg: "批量输出目录"}, {flag: "o", msg: "单文件输出路径"}},
	"imgwm":        {{flag: "", msg: "图片文件路径", req: true}, {flag: "text", msg: "水印文字（仅 ASCII）"}, {flag: "wm", msg: "水印图片路径"}, {flag: "pos", msg: "位置 tl~br/mc/tile", def: "br"}, {flag: "o", msg: "输出路径", req: true}},
	"exift":        {{flag: "", msg: "子命令 view|strip", req: true, first: true}, {flag: "", msg: "图片路径", req: true}},
	"imggeom":      {{flag: "", msg: "子命令 crop|rotate|flip|concat|grid", req: true, first: true}, {flag: "", msg: "图片路径", req: true}, {flag: "o", msg: "输出路径", req: true}},
	"imgfilter":    {{flag: "", msg: "图片路径", req: true}, {flag: "f", msg: "滤镜 gray,mosaic,pixelate,sketch,duotone,sharpen,dither,invert,histogram", req: true}, {flag: "o", msg: "输出路径"}},
	"phash":        {{flag: "", msg: "单图路径算哈希；或 compare 图1 图2", req: true}},
	"imgascii":     {{flag: "", msg: "图片路径", req: true}, {flag: "w", msg: "宽度（字符列数）", def: "80"}, {flag: "invert", msg: "反相", isBool: true}},
	"stegano":      {{flag: "", msg: "子命令 hide|extract", req: true, first: true}, {flag: "", msg: "PNG 载体路径", req: true}, {flag: "msg", msg: "要隐藏的文字（hide）"}, {flag: "pass", msg: "口令（可选加密）"}, {flag: "o", msg: "输出路径（hide 必需）"}},
	"imgcolor":     {{flag: "", msg: "图片路径", req: true}, {flag: "n", msg: "主色数量", def: "8"}},
	"placeholder":  {{flag: "size", msg: "尺寸（如 800x600）", def: "800x600"}, {flag: "text", msg: "文字（ASCII）"}, {flag: "bg", msg: "背景色", def: "#EEEEEE"}, {flag: "fg", msg: "文字色", def: "#AAAAAA"}, {flag: "o", msg: "输出路径"}},
	"favicon":      {{flag: "", msg: "源图片路径（或 -from-text）", req: true}, {flag: "o", msg: "输出 ICO 路径", def: "favicon.ico"}, {flag: "sizes", msg: "尺寸列表", def: "16,32,48,64,128,256"}},
	"barcode":      {{flag: "", msg: "条码内容", req: true}, {flag: "type", msg: "类型 code128|ean13|upca", def: "code128"}, {flag: "o", msg: "输出 PNG 路径", req: true}},
	"bgremove":     {{flag: "", msg: "图片路径", req: true}, {flag: "key", msg: "背景色 #RRGGBB（留空自动取样）"}, {flag: "tol", msg: "容差 0-100", def: "18"}, {flag: "bg", msg: "换底色 #RRGGBB（留空输出透明）"}, {flag: "o", msg: "输出路径", req: true}},
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
