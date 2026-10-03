// Package encconv 实现文本编码转换命令。
// 对应网页版：text/encoding-tool.html（文本编码转换）
//
// 用法：
//
//	lyntoolbox encconv [-from auto] [-to utf-8] [-f 文件] [-o 输出]
//	cat gbk.txt | lyntoolbox encconv
//	lyntoolbox encconv -f in.txt -from auto -to shift-jis -o out.txt
package encconv

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
)

const (
	Name  = "encconv"
	Desc  = "文本文件编码检测与转换（UTF-8/GBK/GB18030/Big5/Shift-JIS/EUC-JP）"
	Usage = `用法: lyntoolbox encconv [-from 编码] [-to 编码] [-f 文件] [-o 输出]

参数:
  -from  源编码：auto|utf-8|gbk|gb18030|big5|shift-jis|euc-jp（默认 auto）
         auto 先检查 UTF-8 合法性，再依次严格尝试 gb18030→big5→shift-jis→euc-jp
  -to    目标编码：utf-8|gbk|gb18030|big5|shift-jis|euc-jp（默认 utf-8）
  -f     输入文件（按原始字节读取）；省略时从 stdin 读取
  -o     输出文件（二进制安全写出）；省略时写到 stdout

说明:
  解码失败的替换字符 U+FFFD 用于严格判定；转换为目标编码时
  不支持的字符会被编码器替换字节代替。检测信息输出到 stderr。`
)

var encodings = map[string]encoding.Encoding{
	"gb18030":   simplifiedchinese.GB18030,
	"gbk":       simplifiedchinese.GBK,
	"big5":      traditionalchinese.Big5,
	"shift-jis": japanese.ShiftJIS,
	"euc-jp":    japanese.EUCJP,
}

var autoOrder = []string{"gb18030", "big5", "shift-jis", "euc-jp"}

// normalize 统一编码名（小写、去下划线、常见别名）。
func normalize(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	n = strings.ReplaceAll(n, "_", "-")
	switch n {
	case "utf8", "utf-8":
		return "utf-8"
	case "cp936", "gb2312":
		return "gbk"
	case "cp950":
		return "big5"
	case "sjis", "ms932":
		return "shift-jis"
	}
	return n
}

// decode 严格解码：出现 U+FFFD 视为失败（valid=false），但仍有解码结果。
func decode(name string, data []byte) (out []byte, valid bool, err error) {
	if name == "utf-8" {
		return stripBOM(data), utf8.Valid(data), nil
	}
	dec, err := encodings[name].NewDecoder().Bytes(data)
	if err != nil {
		return nil, false, err
	}
	return dec, !containsReplacementRune(dec), nil
}

// encode 将 UTF-8 文本编码为目标编码。
func encode(name string, data []byte) ([]byte, error) {
	if name == "utf-8" {
		return data, nil
	}
	return encodings[name].NewEncoder().Bytes(data)
}

func stripBOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

func containsReplacementRune(b []byte) bool {
	return strings.Contains(string(b), "\uFFFD")
}

// autoDetect 自动检测编码。
func autoDetect(data []byte) (string, error) {
	if len(data) == 0 {
		return "utf-8", nil
	}
	if len(data) >= 2 && ((data[0] == 0xFF && data[1] == 0xFE) || (data[0] == 0xFE && data[1] == 0xFF)) {
		return "", fmt.Errorf("检测到 UTF-16 BOM，请先用其他工具转成 UTF-8 再处理")
	}
	if utf8.Valid(data) {
		return "utf-8", nil
	}
	for _, name := range autoOrder {
		_, valid, err := decode(name, data)
		if err == nil && valid {
			return name, nil
		}
	}
	return "", fmt.Errorf("无法检测源编码（utf-8/gb18030/big5/shift-jis/euc-jp 均未命中）")
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
	args = reorderFlags(args, map[string]bool{"from": true, "to": true, "f": true, "o": true})
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	from := fs.String("from", "auto", "源编码")
	to := fs.String("to", "utf-8", "目标编码")
	file := fs.String("f", "", "输入文件")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	srcName := normalize(*from)
	dstName := normalize(*to)
	if srcName != "auto" {
		if _, ok := getEncoding(srcName); !ok {
			fmt.Fprintf(os.Stderr, "不支持的源编码: %s\n", srcName)
			return 2
		}
	}
	if dstName != "utf-8" {
		if _, ok := getEncoding(dstName); !ok {
			fmt.Fprintf(os.Stderr, "不支持的目标编码: %s\n", dstName)
			return 2
		}
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

	if srcName == "auto" {
		detected, err := autoDetect(data)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		srcName = detected
		fmt.Fprintf(os.Stderr, "检测到源编码: %s\n", srcName)
	}

	decoded, valid, err := decode(srcName, data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "解码失败:", err)
		return 1
	}
	if !valid {
		fmt.Fprintf(os.Stderr, "警告: 输入含 %s 无效字节，已替换为 U+FFFD\n", srcName)
	}

	result, err := encode(dstName, decoded)
	if err != nil {
		fmt.Fprintln(os.Stderr, "编码失败:", err)
		return 1
	}

	if *out != "" {
		if err := os.WriteFile(*out, result, 0644); err != nil {
			fmt.Fprintln(os.Stderr, "写入文件失败:", err)
			return 1
		}
		fmt.Printf("已写入 %s（%d 字节，%s → %s）\n", *out, len(result), srcName, dstName)
		return 0
	}
	if _, err := os.Stdout.Write(result); err != nil {
		fmt.Fprintln(os.Stderr, "写出失败:", err)
		return 1
	}
	return 0
}

func getEncoding(name string) (encoding.Encoding, bool) {
	e, ok := encodings[name]
	return e, ok
}
