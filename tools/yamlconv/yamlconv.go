// Package yamlconv 实现 YAML 校验/格式化/与 JSON 互转命令。
// 对应网页版：text/yaml-format-tool.html（YAML格式化）、work/yaml-json-tool.html（YAML与JSON互转）
//
// 用法：
//
//	lyntoolbox yamlconv -f config.yaml
//	lyntoolbox yamlconv -c -f config.yaml
//	lyntoolbox yamlconv -to json -f config.yaml
//	lyntoolbox yamlconv -to yaml -f data.json
package yamlconv

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	Name  = "yamlconv"
	Desc  = "YAML 校验、格式化，以及 YAML 与 JSON 双向互转"
	Usage = `用法: lyntoolbox yamlconv [-to json|yaml] [-c] [-f 文件] [文本]

参数:
  -to json  转为 JSON 输出（YAML 输入）
  -to yaml  转为 YAML 输出（JSON 输入）
  -c        只校验合法性，不输出转换结果
  -f        从文件读取
  文本      待处理内容；省略且无 -f 时从 stdin 读取

说明:
  默认解析后重新输出格式化 YAML（缩进 2）。数字在 YAML→JSON 时为
  int/float64。`
)

// normalize 把 json.Number 转为 int64/float64，便于 yaml.Marshal 输出数字。
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
	to := fs.String("to", "", "目标格式 json|yaml")
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
	case "", "json", "yaml":
	default:
		fmt.Fprintf(os.Stderr, "不支持的 -to 目标: %s（可选 json|yaml）\n", *to)
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
		// YAML → JSON
		var v interface{}
		if err := yaml.Unmarshal(data, &v); err != nil {
			fmt.Fprintln(os.Stderr, "YAML 非法:", err)
			return 1
		}
		if *checkOnly {
			fmt.Println("YAML 合法")
			return 0
		}
		out, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "转 JSON 失败:", err)
			return 1
		}
		fmt.Println(string(out))
	case "yaml":
		// JSON → YAML
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.UseNumber()
		var v interface{}
		if err := dec.Decode(&v); err != nil {
			fmt.Fprintln(os.Stderr, "JSON 非法:", err)
			return 1
		}
		if *checkOnly {
			fmt.Println("JSON 合法")
			return 0
		}
		var buf strings.Builder
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(normalize(v)); err != nil {
			fmt.Fprintln(os.Stderr, "转 YAML 失败:", err)
			return 1
		}
		enc.Close()
		fmt.Print(buf.String())
	default:
		// YAML 校验 / 格式化
		var v interface{}
		if err := yaml.Unmarshal(data, &v); err != nil {
			fmt.Fprintln(os.Stderr, "YAML 非法:", err)
			return 1
		}
		if *checkOnly {
			fmt.Println("YAML 合法")
			return 0
		}
		var buf strings.Builder
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(v); err != nil {
			fmt.Fprintln(os.Stderr, "格式化失败:", err)
			return 1
		}
		enc.Close()
		fmt.Print(buf.String())
	}
	return 0
}
