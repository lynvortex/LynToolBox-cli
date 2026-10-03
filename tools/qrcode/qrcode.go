// Package qrcode 实现二维码生成与识别命令。
// 对应网页版：image/erweima-tool.html（二维码工具）、daily/wifi-qrcode-tool.html（WiFi二维码）
package qrcode

import (
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"

	"github.com/makiuchi-d/gozxing"
	zxingqrcode "github.com/makiuchi-d/gozxing/qrcode"
	"github.com/skip2/go-qrcode"
)

const (
	Name  = "qrcode"
	Desc  = "二维码生成（终端/PNG）与识别，支持 WiFi 编码"
	Usage = `用法:
  lyntoolbox qrcode [文本] [-o out.png] [-size 256] [-level M]
  lyntoolbox qrcode wifi -ssid 名称 -pass 密码 [-auth WPA|WEP|nopass] [-hidden] [-o out.png]
  lyntoolbox qrcode decode 图片路径

参数:
  -o       输出 PNG 文件（省略则终端打印二维码）
  -size    PNG 尺寸像素（默认 256）
  -level   纠错级别 L|M|Q|H（默认 M）`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	if len(args) > 0 && args[0] == "wifi" {
		return runWifi(args[1:])
	}
	if len(args) > 0 && args[0] == "decode" {
		return runDecode(args[1:])
	}

	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	out := fs.String("o", "", "输出 PNG 文件")
	size := fs.Int("size", 256, "PNG 尺寸")
	level := fs.String("level", "M", "纠错级别 L|M|Q|H")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "缺少要编码的文本")
		return 2
	}
	text := strings.Join(fs.Args(), " ")
	return encode(text, *out, *size, *level)
}

func parseLevel(s string) qrcode.RecoveryLevel {
	switch strings.ToUpper(s) {
	case "L":
		return qrcode.Low
	case "M":
		return qrcode.Medium
	case "Q":
		return qrcode.High
	case "H":
		return qrcode.Highest
	}
	return qrcode.Medium
}

func encode(text, out string, size int, level string) int {
	qc, err := qrcode.New(text, parseLevel(level))
	if err != nil {
		fmt.Fprintln(os.Stderr, "生成失败:", err)
		return 1
	}
	if out == "" {
		printTerminal(qc)
		return 0
	}
	if err := qc.WriteFile(size, out); err != nil {
		fmt.Fprintln(os.Stderr, "写入 PNG 失败:", err)
		return 1
	}
	fmt.Printf("已写入 %s\n", out)
	return 0
}

// printTerminal 用 Unicode 半块字符在终端打印二维码（黑码白底）。
func printTerminal(qc *qrcode.QRCode) {
	grid := qc.Bitmap() // grid[y][x] true=黑
	h := len(grid)
	w := len(grid[0])
	white := func(x, y int) bool { return x < 0 || y < 0 || x >= w || y >= h || !grid[y][x] }
	for y := -1; y < h; y += 2 {
		var b strings.Builder
		for x := 0; x < w; x++ {
			top := white(x, y)   // 上半格为白
			bot := white(x, y+1) // 下半格为白
			switch {
			case top && bot:
				b.WriteString(" ")
			case top && !bot:
				b.WriteString("▄") // 下黑上白
			case !top && bot:
				b.WriteString("▀") // 上黑下白
			default:
				b.WriteString("█")
			}
		}
		fmt.Println(b.String())
	}
}

func runWifi(args []string) int {
	fs := flag.NewFlagSet("wifi", flag.ContinueOnError)
	ssid := fs.String("ssid", "", "WiFi 名称（必需）")
	pass := fs.String("pass", "", "密码")
	auth := fs.String("auth", "WPA", "加密方式 WPA|WEP|nopass")
	hidden := fs.Bool("hidden", false, "隐藏网络")
	out := fs.String("o", "", "输出 PNG 文件")
	size := fs.Int("size", 256, "PNG 尺寸")
	level := fs.String("level", "M", "纠错级别")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *ssid == "" {
		fmt.Fprintln(os.Stderr, "缺少 -ssid")
		return 2
	}
	escape := func(s string) string {
		return strings.NewReplacer("\\", "\\\\", ";", "\\;", ",", "\\,", ":", "\\:", "\"", "\\\"").Replace(s)
	}
	var b strings.Builder
	b.WriteString("WIFI:")
	if strings.EqualFold(*auth, "nopass") {
		b.WriteString("T:nopass;")
	} else {
		b.WriteString("T:" + strings.ToUpper(*auth) + ";")
		b.WriteString("P:" + escape(*pass) + ";")
	}
	b.WriteString("S:" + escape(*ssid) + ";")
	if *hidden {
		b.WriteString("H:true;")
	}
	b.WriteString(";")
	return encode(b.String(), *out, *size, *level)
}

func runDecode(args []string) int {
	fs := flag.NewFlagSet("decode", flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox qrcode decode 图片路径")
		return 2
	}
	f, err := os.Open(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开图片失败:", err)
		return 1
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "解析图片失败:", err)
		return 1
	}
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		fmt.Fprintln(os.Stderr, "位图转换失败:", err)
		return 1
	}
	res, err := zxingqrcode.NewQRCodeReader().Decode(bmp, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "未识别到二维码:", err)
		return 1
	}
	fmt.Println(res.GetText())
	return 0
}
