//go:build ignore

// 暂存区：未集成/未完成实现的工具源码，不参与编译。

// Package envconv 实现 .env 与 JSON/YAML 互转命令。
// 对应网页版：work/env-convert-tool.html（.env转换）
//
// 用法：
//
//	lyntoolbox envconv -f .env
//	lyntoolbox envconv -to yaml -f .env
//	lyntoolbox envconv -from json -f config.json
package envconv

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	Name  = "envconv"
	Desc  = ".env 与 JSON/YAML 互转（引号剥离、注释过滤、嵌套路径展平）"
	Usage = `用法: lyntoolbox envconv [-to json|yaml] [-from env|json] [-f 文件] [文本]

参数:
  -to json|yaml  输出格式；默认 json；-from json 时默认输出 .env
  -from env|json 输入格式；默认 env
  -f             从文件读取
  文本           待处理内容；省略且无 -f 时从 stdin 读取

说明:
  .env 解析支持 KEY=VALUE、单双引号剥离、# 注释与行内注释、export 前缀、
  双引号内 \n \t \" \\ 转义。JSON→.env 时对象展平为下划线大写路径 A_B_C，
  布尔/数字原样，数组序列化为 JSON 字符串。`
)

var keyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)

// parseEnv 解析 .env 文本为键值对（保持出现顺序）。
func parseEnv(text string) ([][2]string, error) {
	var out [][2]string
	for lineNo, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.Index(line, "=")
		if eq < 0 {
			return nil, fmt.Errorf("第 %d 行缺少 '=': %s", lineNo+1, raw)
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if key == "" || !keyRe.MatchString(key) {
			return nil, fmt.Errorf("第 %d 行的键名不合法: %s", lineNo+1, key)
		}
		val = stripValue(val)
		out = append(out, [2]string{key, val})
	}
	return out, nil
}

// stripValue 剥离引号与行内注释。
func stripValue(val string) string {
	if len(val) >= 2 {
		if val[0] == '"' {
			if end := strings.Index(val[1:], `"`); end >= 0 {
				return unescapeDouble(val[1 : end+1])
			}
			// 未闭合引号：取全部内容
			return unescapeDouble(val[1:])
		}
		if val[0] == '\'' {
			if end := strings.Index(val[1:], `'`); end >= 0 {
				return val[1 : end+1]
			}
			return val[1:]
		}
	}
	// 非引号值：去掉 " #" 起始的行内注释
	if idx := strings.Index(val, " #"); idx >= 0 {
		val = val[:idx]
	}
	return strings.TrimSpace(val)
}

func unescapeDouble(s string) string {
	r := strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\r`, "\r", `\"`, `"`, `\\`, `\`)
	return r.Replace(s)
}

// escapeDouble 输出双引号包裹的值。
func escapeDouble(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`, "\r", `\r`)
	return `"` + r.Replace(s) + `"`
}

// needsQuote 判断 .env 输出时是否需要引号。
func needsQuote(s string) bool {
	if s == "" {
		return true
	}
	return strings.ContainsAny(s, " \t\r\n#'\"=")
}

// envPairsToMap 转换为 map（键按字母序输出）。
func envPairsToMap(pairs [][2]string) map[string]string {
	m := make(map[string]string, len(pairs))
	for _, p := range pairs {
		m[p[0]] = p[1]
	}
	return m
}

// flattenJSON 把 JSON 对象展平为 KEY=VALUE 对。
func flattenJSON(prefix string, v interface{}, out *[][2]string) error {
	switch t := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			key := strings.ToUpper(k)
			if prefix != "" {
				key = prefix + "_" + key
			}
			key = strings.ReplaceAll(strings.ReplaceAll(key, ".", "_"), "-", "_")
			if err := flattenJSON(key, t[k], out); err != nil {
				return err
			}
		}
		return nil
	case string:
		*out = append(*out, [2]string{prefix, t})
		return nil
	case json.Number:
		*out = append(*out, [2]string{prefix, t.String()})
		return nil
	case bool:
		*out = append(*out, [2]string{prefix, strconv.FormatBool(t)})
		return nil
	case nil:
		*out = append(*out, [2]string{prefix, ""})
		return nil
	default:
		b, err := json.Marshal(t) // 数组等序列化为 JSON 字符串
		if err != nil {
			return err
		}
		*out = append(*out, [2]string{prefix, string(b)})
		return nil
	}
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	to := fs.String("to", "", "输出格式 json|yaml")
	from := fs.String("from", "", "输入格式 env|json")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	*from = strings.ToLower(*from)
	if *from == "" {
		*from = "env"
	}
	switch *from {
	case "env", "json":
	default:
		fmt.Fprintf(os.Stderr, "不支持的 -from 输入格式: %s（可选 env|json）\n", *from)
		return 2
	}
	*to = strings.ToLower(*to)
	if *to == "" {
		if *from == "json" {
			*to = "env"
		} else {
			*to = "json"
		}
	}
	switch *to {
	case "json", "yaml", "env":
	default:
		fmt.Fprintf(os.Stderr, "不支持的 -to 输出格式: %s（可选 json|yaml|env）\n", *to)
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

	switch {
	case *from == "env" && *to == "env":
		// 原样解析再输出，起规范化/校验作用
		pairs, err := parseEnv(string(data))
		if err != nil {
			fmt.Fprintln(os.Stderr, ".env 解析失败:", err)
			return 1
		}
		for _, p := range pairs {
			if needsQuote(p[1]) {
				fmt.Printf("%s=%s\n", p[0], escapeDouble(p[1]))
			} else {
				fmt.Printf("%s=%s\n", p[0], p[1])
			}
		}
	case *from == "env":
		pairs, err := parseEnv(string(data))
		if err != nil {
			fmt.Fprintln(os.Stderr, ".env 解析失败:", err)
			return 1
		}
		m := envPairsToMap(pairs)
		if *to == "yaml" {
			var buf strings.Builder
			enc := yaml.NewEncoder(&buf)
			if err := enc.Encode(m); err != nil {
				fmt.Fprintln(os.Stderr, "转 YAML 失败:", err)
				return 1
			}
			enc.Close()
			fmt.Print(buf.String())
		} else {
			b, err := json.MarshalIndent(m, "", "  ")
			if err != nil {
				fmt.Fprintln(os.Stderr, "转 JSON 失败:", err)
				return 1
			}
			fmt.Println(string(b))
		}
	default:
		// JSON → env（或 yaml）
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.UseNumber()
		var v interface{}
		if err := dec.Decode(&v); err != nil {
			fmt.Fprintln(os.Stderr, "JSON 非法:", err)
			return 1
		}
		if *to != "env" {
			// JSON → YAML 顺带支持
			out, err := yaml.Marshal(v)
			if err != nil {
				fmt.Fprintln(os.Stderr, "转 YAML 失败:", err)
				return 1
			}
			fmt.Print(string(out))
			return 0
		}
		obj, ok := v.(map[string]interface{})
		if !ok {
			fmt.Fprintln(os.Stderr, "JSON 顶层必须是对象")
			return 1
		}
		var pairs [][2]string
		if err := flattenJSON("", obj, &pairs); err != nil {
			fmt.Fprintln(os.Stderr, "展平失败:", err)
			return 1
		}
		for _, p := range pairs {
			if p[0] == "" {
				continue
			}
			if needsQuote(p[1]) {
				fmt.Printf("%s=%s\n", p[0], escapeDouble(p[1]))
			} else {
				fmt.Printf("%s=%s\n", p[0], p[1])
			}
		}
	}
	return 0
}
