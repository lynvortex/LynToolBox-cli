// Package tomlconv 实现 TOML 校验/格式化/与 JSON 互转命令。
// 对应网页版：text/toml-format-tool.html（TOML格式化）
//
// 用法：
//
//	lyntoolbox tomlconv -f config.toml
//	lyntoolbox tomlconv -c -f config.toml
//	lyntoolbox tomlconv -to json -f config.toml
//	lyntoolbox tomlconv -to toml -f data.json
package tomlconv

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	Name  = "tomlconv"
	Desc  = "TOML 校验、格式化，以及 TOML 与 JSON 双向互转"
	Usage = `用法: lyntoolbox tomlconv [-to json|toml] [-c] [-f 文件] [文本]

参数:
  -to json  转为 JSON 输出（TOML 输入）
  -to toml  转为 TOML 输出（JSON 输入）
  -c        只校验合法性，不输出转换结果
  -f        从文件读取
  文本      待处理内容；省略且无 -f 时从 stdin 读取

说明:
  默认解析后用 TOML 编码器重新输出格式化 TOML。JSON→TOML 时顶层必须是对象。`
)

// normalize 把 json.Number 转为 int64/float64，便于 TOML 编码输出数字。
func normalize(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, e := range t {
			t[k] = normalize(e)
		}
		return t
	case []interface{}:
		for i, e := range t {
			t[i] = normalize(e)
		}
		return t
	case json.Number:
		s := t.String()
		if !strings.ContainsAny(s, ".eE") {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return n
			}
		}
		f, err := t.Float64()
		if err != nil {
			return s
		}
		return f
	}
	return v
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	to := fs.String("to", "", "目标格式 json|toml")
	checkOnly := fs.Bool("c", false, "只校验")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	*to = strings.ToLower(*to)
	switch *to {
	case "", "json", "toml":
	default:
		fmt.Fprintf(os.Stderr, "不支持的 -to 目标: %s（可选 json|toml）\n", *to)
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
	} else if fs.NArg() > 0 {
		data = []byte(strings.Join(fs.Args(), " "))
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 stdin 失败:", err)
			return 1
		}
		data = b
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		fmt.Fprintln(os.Stderr, "输入为空")
		return 1
	}

	switch *to {
	case "json":
		// TOML → JSON
		var v map[string]interface{}
		if _, err := toml.Decode(string(data), &v); err != nil {
			fmt.Fprintln(os.Stderr, "TOML 非法:", err)
			return 1
		}
		if *checkOnly {
			fmt.Println("TOML 合法")
			return 0
		}
		out, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "转 JSON 失败:", err)
			return 1
		}
		fmt.Println(string(out))
	case "toml":
		// JSON → TOML
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.UseNumber()
		var v interface{}
		if err := dec.Decode(&v); err != nil {
			fmt.Fprintln(os.Stderr, "JSON 非法:", err)
			return 1
		}
		m, ok := normalize(v).(map[string]interface{})
		if !ok {
			fmt.Fprintln(os.Stderr, "JSON 顶层必须是对象才能转为 TOML")
			return 1
		}
		if *checkOnly {
			fmt.Println("JSON 合法")
			return 0
		}
		if err := toml.NewEncoder(os.Stdout).Encode(m); err != nil {
			fmt.Fprintln(os.Stderr, "转 TOML 失败:", err)
			return 1
		}
	default:
		// TOML 校验 / 格式化
		var v map[string]interface{}
		if _, err := toml.Decode(string(data), &v); err != nil {
			fmt.Fprintln(os.Stderr, "TOML 非法:", err)
			return 1
		}
		if *checkOnly {
			fmt.Println("TOML 合法")
			return 0
		}
		if err := toml.NewEncoder(os.Stdout).Encode(v); err != nil {
			fmt.Fprintln(os.Stderr, "格式化失败:", err)
			return 1
		}
	}
	return 0
}
