// Package httpcode 实现 HTTP 状态码离线速查命令。
// 对应网页版：network/http-status-tool.html（HTTP状态码速查）
package httpcode

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

const (
	Name  = "httpcode"
	Desc  = "HTTP 状态码离线速查（中文说明与处理建议）"
	Usage = `用法: lyntoolbox httpcode [状态码|类码|all]

参数:
  404   查询单个状态码
  4xx   查询某类全部状态码（1xx-5xx）
  all   输出全部
  省略  输出分类目录`
)

type entry struct {
	name string
	desc string
	tip  string
}

var table = map[int]entry{
	// 1xx
	100: {"Continue", "服务器已收到请求头，客户端应继续发送请求体", "大文件上传时的临时确认"},
	101: {"Switching Protocols", "服务器同意切换协议", "WebSocket 握手成功即此码"},
	// 2xx
	200: {"OK", "请求成功", "最常见成功状态码"},
	201: {"Created", "请求成功并创建了新资源", "POST 创建资源的标准响应"},
	202: {"Accepted", "请求已接受但尚未处理完成", "异步任务场景"},
	204: {"No Content", "成功但无返回内容", "DELETE 成功常用"},
	206: {"Partial Content", "返回部分内容", "Range 断点续传"},
	// 3xx
	300: {"Multiple Choices", "多个可选资源", "较少见"},
	301: {"Moved Permanently", "资源永久迁移", "SEO 权重转移；客户端应缓存新地址"},
	302: {"Found", "资源临时迁移", "不缓存；临时跳转"},
	303: {"See Other", "用 GET 访问另一地址", "PRG 模式防表单重复提交"},
	304: {"Not Modified", "资源未变化，用本地缓存", "配合 ETag/If-None-Match"},
	307: {"Temporary Redirect", "临时重定向且保持原方法", "不会把 POST 变 GET"},
	308: {"Permanent Redirect", "永久重定向且保持原方法", "301 的严格版"},
	// 4xx
	400: {"Bad Request", "请求语法错误，服务器无法理解", "检查参数格式与 JSON 合法性"},
	401: {"Unauthorized", "未认证", "缺少或错误的凭据；带 WWW-Authenticate 头"},
	402: {"Payment Required", "保留字段", "实际用于付费墙等场景"},
	403: {"Forbidden", "服务器拒绝执行", "已认证但无权限，与登录无关"},
	404: {"Not Found", "资源不存在", "检查 URL 拼写与路由注册"},
	405: {"Method Not Allowed", "方法不被允许", "对 GET 路由发了 POST；看 Allow 头"},
	406: {"Not Acceptable", "无法满足 Accept 头要求", "内容协商失败"},
	408: {"Request Timeout", "请求超时", "客户端发送太慢"},
	409: {"Conflict", "请求与资源当前状态冲突", "并发修改/唯一约束冲突"},
	410: {"Gone", "资源已永久删除", "比 404 更明确的失效"},
	413: {"Payload Too Large", "请求体过大", "调整服务器上传限制"},
	414: {"URI Too Long", "URL 过长", "改用 POST 传递数据"},
	415: {"Unsupported Media Type", "不支持的媒体类型", "检查 Content-Type"},
	422: {"Unprocessable Entity", "请求格式正确但语义错误", "校验失败常用（WebDAV/Laravel）"},
	429: {"Too Many Requests", "请求过于频繁", "限流触发；看 Retry-After 头"},
	451: {"Unavailable For Legal Reasons", "因法律原因不可用", "合规下架"},
	// 5xx
	500: {"Internal Server Error", "服务器内部错误", "查服务端日志与异常堆栈"},
	501: {"Not Implemented", "功能未实现", "服务器不支持该方法"},
	502: {"Bad Gateway", "网关收到无效上游响应", "反代后端挂了；排查 upstream"},
	503: {"Service Unavailable", "服务暂不可用", "过载或维护；看 Retry-After"},
	504: {"Gateway Timeout", "网关等待上游超时", "后端响应太慢；调大 proxy 超时"},
	505: {"HTTP Version Not Supported", "HTTP 版本不支持", "罕见"},
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	arg := ""
	if fs.NArg() > 0 {
		arg = strings.ToLower(fs.Arg(0))
	}

	switch {
	case arg == "all":
		printClass("1xx 信息", 100, 199)
		printClass("2xx 成功", 200, 299)
		printClass("3xx 重定向", 300, 399)
		printClass("4xx 客户端错误", 400, 499)
		printClass("5xx 服务器错误", 500, 599)
		return 0
	case strings.HasSuffix(arg, "xx") && len(arg) == 3 && arg[0] >= '1' && arg[0] <= '5':
		lo := int(arg[0]-'0') * 100
		printClass(arg+" "+className(lo), lo, lo+99)
		return 0
	case arg == "":
		fmt.Println("用法示例: lyntoolbox httpcode 404 | 4xx | all")
		fmt.Println()
		fmt.Println("分类目录:")
		for _, c := range []int{100, 200, 300, 400, 500} {
			fmt.Printf("  %dxx  %s\n", c/100, className(c))
		}
		return 0
	default:
		var code int
		if _, err := fmt.Sscanf(arg, "%d", &code); err != nil || code < 100 || code > 599 {
			fmt.Fprintf(os.Stderr, "无效输入: %s（示例 404 / 4xx / all）\n", arg)
			return 2
		}
		e, ok := table[code]
		if !ok {
			fmt.Printf("%d  （未收录的非标准状态码）\n", code)
			return 0
		}
		fmt.Printf("%d %s\n  含义: %s\n  场景: %s\n", code, e.name, e.desc, e.tip)
		return 0
	}
}

func className(lo int) string {
	switch lo {
	case 100:
		return "信息响应"
	case 200:
		return "成功"
	case 300:
		return "重定向"
	case 400:
		return "客户端错误"
	case 500:
		return "服务器错误"
	}
	return ""
}

func printClass(title string, lo, hi int) {
	fmt.Println("【" + title + "】")
	codes := make([]int, 0)
	for c := range table {
		if c >= lo && c <= hi {
			codes = append(codes, c)
		}
	}
	sort.Ints(codes)
	for _, c := range codes {
		e := table[c]
		fmt.Printf("  %d %-24s %s\n", c, e.name, e.desc)
	}
	fmt.Println()
}
