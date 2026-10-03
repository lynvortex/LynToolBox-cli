// Package hexdump 实现十六进制查看命令。
// 对应网页版：work/hex-viewer-tool.html（十六进制查看器）
//
// 用法：
//
//	lyntoolbox hexdump [-w 16] [-o 偏移] [-l 长度] [-f 文件]
//	lyntoolbox hexdump -f out.bin
//	cat data.bin | lyntoolbox hexdump -o 16 -l 64
package hexdump

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const (
	Name  = "hexdump"
	Desc  = "经典 hexdump -C 风格十六进制查看（偏移+十六进制+ASCII 列）"
	Usage = `用法: lyntoolbox hexdump [-w 每行字节] [-o 起始偏移] [-l 长度] [-f 文件]

参数:
  -w  每行显示字节数（默认 16，建议为 8 的倍数以便分组）
  -o  起始读取偏移（十进制或 0x 十六进制，默认 0）
  -l  最多读取的字节数（-1 表示读到末尾）
  -f  输入文件；省略时从 stdin 读取

输出格式: 8 位十六进制偏移 + 十六进制字节（每 8 字节一组分两段）+ ASCII 列，
末尾单独一行打印总字节数。`
)

// parseNum 解析十进制或 0x 前缀十六进制数值。
func parseNum(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(strings.ToLower(s), "0x") {
		return strconv.ParseInt(s[2:], 16, 64)
	}
	return strconv.ParseInt(s, 10, 64)
}

// reorderFlags 将 flag 参数挪到位置参数之前，使 "位置参数 -flag" 与 "-flag 位置参数" 两种顺序均可解析。
// valueFlags 为需要消费下一个参数的 flag 名集合。
func reorderFlags(args []string, valueFlags map[string]bool) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 1 && a[0] == '-' {
			if _, err := strconv.ParseFloat(a, 64); err == nil { // 负数视为位置参数
				pos = append(pos, a)
				continue
			}
			name := strings.TrimLeft(a, "-")
			if !strings.Contains(name, "=") && valueFlags[name] && i+1 < len(args) {
				flags = append(flags, a, args[i+1])
				i++
				continue
			}
			flags = append(flags, a)
			continue
		}
		pos = append(pos, a)
	}
	return append(flags, pos...)
} // Run 执行命令，返回退出码。
func Run(args []string) int {
	args = reorderFlags(args, map[string]bool{"w": true, "o": true, "l": true, "f": true})
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	width := fs.Int("w", 16, "每行字节数")
	offsetStr := fs.String("o", "0", "起始偏移")
	lengthStr := fs.String("l", "-1", "读取长度")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *width <= 0 || *width > 1024 {
		fmt.Fprintln(os.Stderr, "-w 必须在 1~1024 之间")
		return 2
	}
	offset, err := parseNum(*offsetStr)
	if err != nil || offset < 0 {
		fmt.Fprintf(os.Stderr, "无效的 -o 偏移: %s\n", *offsetStr)
		return 2
	}
	length, err := parseNum(*lengthStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无效的 -l 长度: %s\n", *lengthStr)
		return 2
	}

	var data []byte
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取文件失败:", err)
			return 1
		}
		data = b
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 stdin 失败:", err)
			return 1
		}
		data = b
	}

	if offset > int64(len(data)) {
		fmt.Fprintf(os.Stderr, "起始偏移 %d 超过数据长度 %d\n", offset, len(data))
		return 1
	}
	data = data[offset:]
	if length >= 0 && length < int64(len(data)) {
		data = data[:length]
	}

	w := *width
	half := w / 2
	total := len(data)
	// 十六进制区域宽度：每字节 "xx " + 半组处额外 1 空格
	hexWidth := w*3 + 1
	if half == 0 {
		hexWidth = w * 3
	}
	for i := 0; i < total; i += w {
		end := i + w
		if end > total {
			end = total
		}
		var hex strings.Builder
		for j := i; j < end; j++ {
			if j > i {
				if half > 0 && j-i == half {
					hex.WriteString("  ") // 8 字节分组间隔
				} else {
					hex.WriteByte(' ')
				}
			}
			fmt.Fprintf(&hex, "%02x", data[j])
		}
		var ascii strings.Builder
		for j := i; j < end; j++ {
			if c := data[j]; c >= 0x20 && c <= 0x7E {
				ascii.WriteByte(c)
			} else {
				ascii.WriteByte('.')
			}
		}
		fmt.Printf("%08x  %-*s |%s|\n", offset+int64(i), hexWidth, hex.String(), ascii.String())
	}
	fmt.Printf("%08x\n", offset+int64(total))
	return 0
}
