// Package mime 实现 MIME 类型查询命令。
// 对应网页版：network/mime-tool.html（MIME类型查询）
//
// 用法：
//
//	lyntoolbox mime txt
//	lyntoolbox mime .pdf
//	lyntoolbox mime "text/html"
//	lyntoolbox mime -all
package mime

import (
	"flag"
	"fmt"
	"mime"
	"os"
	"sort"
	"strings"
)

const (
	Name  = "mime"
	Desc  = "MIME 类型与扩展名互查（内置常见类型表）"
	Usage = `用法: lyntoolbox mime 扩展名|MIME类型
   或: lyntoolbox mime -all

参数:
  扩展名     正向查询: txt / .txt -> text/plain
  MIME 类型  反向查询: 含 "/" 时按类型查扩展名，如 text/html
  -all       列出内置 MIME 类型表

说明: 正向查询优先使用标准库 mime.TypeByExtension，缺失时用内置补充表（约 90 条）。`
)

// builtin 内置补充表（扩展名 -> MIME）
var builtin = map[string]string{
	".3g2": "video/3gpp2", ".3gp": "video/3gpp", ".7z": "application/x-7z-compressed",
	".aac": "audio/aac", ".apk": "application/vnd.android.package-archive", ".atom": "application/atom+xml",
	".avif": "image/avif", ".avi": "video/x-msvideo", ".bin": "application/octet-stream",
	".bmp": "image/bmp", ".bz2": "application/x-bzip2", ".c": "text/x-c",
	".css": "text/css", ".csv": "text/csv", ".dll": "application/x-msdownload",
	".doc": "application/msword", ".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".eot": "application/vnd.ms-fontobject", ".epub": "application/epub+zip", ".exe": "application/x-msdownload",
	".flac": "audio/flac", ".flv": "video/x-flv", ".gif": "image/gif",
	".gz": "application/gzip", ".htm": "text/html", ".html": "text/html",
	".ico": "image/x-icon", ".ics": "text/calendar", ".iso": "application/x-iso9660-image",
	".jar": "application/java-archive", ".java": "text/x-java-source", ".jpeg": "image/jpeg",
	".jpg": "image/jpeg", ".js": "text/javascript", ".json": "application/json",
	".jsonld": "application/ld+json", ".log": "text/plain", ".m3u8": "application/vnd.apple.mpegurl",
	".m4a": "audio/mp4", ".m4v": "video/x-m4v", ".md": "text/markdown",
	".mid": "audio/midi", ".midi": "audio/midi", ".mkv": "video/x-matroska",
	".mov": "video/quicktime", ".mp3": "audio/mpeg", ".mp4": "video/mp4",
	".mpeg": "video/mpeg", ".mpg": "video/mpeg", ".msi": "application/x-msi",
	".odp": "application/vnd.oasis.opendocument.presentation", ".ods": "application/vnd.oasis.opendocument.spreadsheet",
	".odt": "application/vnd.oasis.opendocument.text", ".ogg": "audio/ogg", ".ogv": "video/ogg",
	".otf": "font/otf", ".pdf": "application/pdf", ".png": "image/png",
	".ppt": "application/vnd.ms-powerpoint", ".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".ps": "application/postscript", ".psd": "image/vnd.adobe.photoshop", ".py": "text/x-python",
	".rar": "application/vnd.rar", ".rtf": "application/rtf", ".sh": "application/x-sh",
	".so": "application/x-sharedlib", ".svg": "image/svg+xml", ".swf": "application/x-shockwave-flash",
	".tar": "application/x-tar", ".tif": "image/tiff", ".tiff": "image/tiff",
	".toml": "application/toml", ".ts": "text/typescript", ".ttf": "font/ttf",
	".txt": "text/plain", ".vsd": "application/vnd.visio", ".wav": "audio/wav",
	".weba": "audio/webm", ".webm": "video/webm", ".webp": "image/webp",
	".wma": "audio/x-ms-wma", ".wmv": "video/x-ms-wmv", ".woff": "font/woff",
	".woff2": "font/woff2", ".xhtml": "application/xhtml+xml", ".xls": "application/vnd.ms-excel",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xml": "application/xml",
	".xsl": "application/xslt+xml", ".yaml": "application/yaml", ".yml": "application/yaml",
	".zip": "application/zip",
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	all := fs.Bool("all", false, "列出内置 MIME 表")
	fs.Usage = func() { fmt.Print(Usage + "\n") }

	// 允许位置参数后仍跟选项
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

	if *all {
		keys := make([]string, 0, len(builtin))
		for k := range builtin {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Printf("内置 MIME 类型表（%d 条）:\n", len(keys))
		fmt.Println("  扩展名     MIME 类型")
		for _, k := range keys {
			fmt.Printf("  %-10s %s\n", k, builtin[k])
		}
		return 0
	}

	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, "错误: 需要且只需要一个参数（扩展名或 MIME 类型），或使用 -all")
		fmt.Fprintln(os.Stderr, "（lyntoolbox mime -h 查看帮助）")
		return 2
	}
	arg := strings.TrimSpace(positionals[0])

	if strings.Contains(arg, "/") {
		return reverse(arg)
	}
	return forward(arg)
}

// forward 扩展名 -> MIME
func forward(arg string) int {
	ext := strings.ToLower(arg)
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	std := mime.TypeByExtension(ext)
	bd, hasBd := builtin[ext]
	switch {
	case std != "":
		fmt.Printf("扩展名:    %s\nMIME 类型: %s\n", ext, std)
		if hasBd && trimParams(bd) != trimParams(std) {
			fmt.Printf("（内置补充表登记为: %s）\n", bd)
		}
		return 0
	case hasBd:
		fmt.Printf("扩展名:    %s\nMIME 类型: %s（内置补充表）\n", ext, bd)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未找到扩展名 %s 对应的 MIME 类型\n", ext)
		return 1
	}
}

// reverse MIME -> 扩展名
func reverse(arg string) int {
	mt := strings.ToLower(strings.TrimSpace(strings.Split(arg, ";")[0]))
	set := map[string]bool{}
	if exts, err := mime.ExtensionsByType(mt); err == nil {
		for _, e := range exts {
			set[e] = true
		}
	}
	for e, t := range builtin {
		if trimParams(t) == mt {
			set[e] = true
		}
	}
	if len(set) == 0 {
		fmt.Fprintf(os.Stderr, "未找到 MIME 类型 %s 对应的扩展名\n", mt)
		return 1
	}
	exts := make([]string, 0, len(set))
	for e := range set {
		exts = append(exts, e)
	}
	sort.Strings(exts)
	fmt.Printf("MIME 类型: %s\n扩展名:    %s\n", mt, strings.Join(exts, " "))
	return 0
}

// trimParams 去掉 MIME 参数部分
func trimParams(s string) string {
	return strings.TrimSpace(strings.ToLower(strings.Split(s, ";")[0]))
}
