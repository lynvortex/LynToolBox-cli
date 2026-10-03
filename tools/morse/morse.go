// Package morse 实现摩斯密码（Morse Code）编解码命令。
// 对应网页版：text/mosi-tool.html（摩斯密码）
//
// 用法：
//
//	lyntoolbox morse SOS
//	lyntoolbox morse -d "... --- ..."
//	lyntoolbox morse "SOS SOS" -sep "/"
package morse

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	Name  = "morse"
	Desc  = "摩斯密码编码与解码，支持字母数字常用标点"
	Usage = `用法: lyntoolbox morse [-d] [-sep 分隔符] [文本]

参数:
  -d     解码模式（默认编码）
  -sep   词（单词）分隔符，默认 "/ "，编码时输出形如 "... --- ... / ... --- ..."
  文本   待处理文本；省略时从 stdin 读取

说明:
  编码时字母间用单个空格分隔，单词间用分隔符连接；小写字母自动转大写。
  解码时自动识别分隔符与空格，输出大写原文。`
)

// morseTable 标准国际摩斯电码表：A-Z、0-9 及常用标点。
var morseTable = map[rune]string{
	'A': ".-", 'B': "-...", 'C': "-.-.", 'D': "-..", 'E': ".", 'F': "..-.",
	'G': "--.", 'H': "....", 'I': "..", 'J': ".---", 'K': "-.-", 'L': ".-..",
	'M': "--", 'N': "-.", 'O': "---", 'P': ".--.", 'Q': "--.-", 'R': ".-.",
	'S': "...", 'T': "-", 'U': "..-", 'V': "...-", 'W': ".--", 'X': "-..-",
	'Y': "-.--", 'Z': "--..",
	'0': "-----", '1': ".----", '2': "..---", '3': "...--", '4': "....-",
	'5': ".....", '6': "-....", '7': "--...", '8': "---..", '9': "----.",
	'.': ".-.-.-", ',': "--..--", '?': "..--..", '!': "-.-.--", '/': "-..-.",
	'(': "-.--.", ')': "-.--.-", '&': ".-...", ':': "---...", ';': "-.-.-.",
	'=': "-...-", '+': ".-.-", '-': "-....-", '_': "..--.-", '"': ".-..-.",
	'$': "...-..-", '@': ".--.-.", '\'': ".----.",
}

// revTable 解码用的反查表。
var revTable = func() map[string]rune {
	m := make(map[string]rune, len(morseTable))
	for k, v := range morseTable {
		m[v] = k
	}
	return m
}()

// encode 将明文编码为摩斯电码。
func encode(text, sep string) (string, error) {
	words := strings.Fields(text)
	if len(words) == 0 {
		return "", fmt.Errorf("输入为空")
	}
	outWords := make([]string, 0, len(words))
	for _, w := range words {
		var sb strings.Builder
		for _, r := range strings.ToUpper(w) {
			code, ok := morseTable[r]
			if !ok {
				return "", fmt.Errorf("不支持的字符: %q", r)
			}
			if sb.Len() > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(code)
		}
		outWords = append(outWords, sb.String())
	}
	return strings.Join(outWords, " "+sep+" "), nil
}

// decode 将摩斯电码解码为明文。
func decode(text, sep string) (string, error) {
	t := strings.TrimSpace(text)
	if t == "" {
		return "", fmt.Errorf("输入为空")
	}
	if sep != "/" {
		t = strings.ReplaceAll(t, sep, "/")
	}
	parts := strings.Split(t, "/")
	outWords := make([]string, 0, len(parts))
	for _, part := range parts {
		letters := strings.Fields(part)
		if len(letters) == 0 {
			continue
		}
		var sb strings.Builder
		for _, code := range letters {
			r, ok := revTable[code]
			if !ok {
				return "", fmt.Errorf("无法识别的摩斯码: %q", code)
			}
			sb.WriteRune(r)
		}
		outWords = append(outWords, sb.String())
	}
	if len(outWords) == 0 {
		return "", fmt.Errorf("输入为空")
	}
	return strings.Join(outWords, " "), nil
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	decodeMode := fs.Bool("d", false, "解码模式")
	sep := fs.String("sep", "/ ", "词分隔符")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	var data []byte
	if fs.NArg() > 0 {
		data = []byte(strings.Join(fs.Args(), " "))
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 stdin 失败:", err)
			return 1
		}
		data = b
	}

	wordSep := strings.TrimSpace(*sep)
	if wordSep == "" {
		wordSep = "/"
	}

	var result string
	var err error
	if *decodeMode {
		result, err = decode(string(data), wordSep)
	} else {
		result, err = encode(string(data), wordSep)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "处理失败:", err)
		return 1
	}
	fmt.Println(result)
	return 0
}
