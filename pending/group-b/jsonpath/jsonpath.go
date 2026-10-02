//go:build ignore

// 暂存区：未集成/未完成实现的工具源码，不参与编译。

// Package jsonpath 实现 JSONPath 查询命令（手写子集）。
// 对应网页版：work/jsonpath-tool.html（JSONPath查询）
//
// 支持: $ 根、.key、["key"]、[0]、[-1]、[start:end]、.*、..key 递归下降
//
// 用法：
//
//	lyntoolbox jsonpath '$.store.book[0].title' -f data.json
//	echo '{"a":{"b":[1,2,3]}}' | lyntoolbox jsonpath '$.a.b[-1]'
package jsonpath

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	Name  = "jsonpath"
	Desc  = "用 JSONPath 表达式查询 JSON 数据（支持 $ .key [0] [-1] [start:end] .* ..key）"
	Usage = `用法: lyntoolbox jsonpath '表达式' [-f 文件] [JSON文本]

参数:
  表达式    JSONPath 查询表达式，如 $.a.b[0]、$["key"]、$.a[*]、$..name
  -f        从文件读取 JSON；省略 JSON 文本与 -f 时从 stdin 读取
  JSON文本  待查询的 JSON；有 -f 时忽略

支持的语法:
  $          根对象
  .key       子字段
  ["key"]    子字段（括号写法）
  [0] [−1]   数组下标（负数从末尾计数）
  [start:end] 数组切片（Python 风格，负数/省略均可）
  .*  [*]    通配所有子值
  ..key      递归下降，取所有层级的 key`
)

// step 表示解析后的单个选择步骤。
type step struct {
	kind       string // "name" | "index" | "slice" | "wild" | "desc"（递归下降）
	key        string
	index      int
	sc, ec     int
	hasS, hasE bool
}

// parsePath 解析 JSONPath 表达式为步骤序列。
func parsePath(expr string) ([]step, error) {
	expr = strings.TrimSpace(expr)
	if !strings.HasPrefix(expr, "$") {
		return nil, fmt.Errorf("表达式必须以 $ 开头")
	}
	var steps []step
	i := 1
	for i < len(expr) {
		switch {
		case strings.HasPrefix(expr[i:], ".."):
			i += 2
			if i < len(expr) && expr[i] == '*' {
				steps = append(steps, step{kind: "descAll"})
				i++
				break
			}
			if i < len(expr) && expr[i] == '[' {
				// ..["key"] / ..[0] 形式
				j := strings.IndexByte(expr[i:], ']')
				if j < 0 {
					return nil, fmt.Errorf("第 %d 字符附近: 括号未闭合", i+1)
				}
				inner := strings.TrimSpace(expr[i+1 : i+j])
				i += j + 1
				if strings.HasPrefix(inner, `"`) || strings.HasPrefix(inner, `'`) {
					steps = append(steps, step{kind: "desc", key: trimQuote(inner)})
				} else {
					return nil, fmt.Errorf("递归下降仅支持 ..key 或 ..[\"key\"] 形式")
				}
				break
			}
			// ..key：读取连续的名称字符
			j := i
			for j < len(expr) && isNameChar(expr[j]) {
				j++
			}
			if j == i {
				return nil, fmt.Errorf(".. 之后缺少字段名")
			}
			steps = append(steps, step{kind: "desc", key: expr[i:j]})
			i = j
		case expr[i] == '.':
			i++
			if i < len(expr) && expr[i] == '*' {
				steps = append(steps, step{kind: "wild"})
				i++
				break
			}
			j := i
			for j < len(expr) && isNameChar(expr[j]) {
				j++
			}
			if j == i {
				return nil, fmt.Errorf("第 %d 字符附近: '.' 之后缺少字段名", i)
			}
			steps = append(steps, step{kind: "name", key: expr[i:j]})
			i = j
		case expr[i] == '[':
			end := strings.IndexByte(expr[i:], ']')
			if end < 0 {
				return nil, fmt.Errorf("第 %d 字符附近: 括号未闭合", i+1)
			}
			inner := strings.TrimSpace(expr[i+1 : i+end])
			i += end + 1
			if strings.HasPrefix(inner, `"`) || strings.HasPrefix(inner, `'`) {
				steps = append(steps, step{kind: "name", key: trimQuote(inner)})
				break
			}
			if inner == "*" {
				steps = append(steps, step{kind: "wild"})
				break
			}
			if strings.Contains(inner, ":") {
				parts := strings.SplitN(inner, ":", 2)
				st := step{kind: "slice", sc: 0, ec: math.MaxInt32}
				if p := strings.TrimSpace(parts[0]); p != "" {
					n, err := strconv.Atoi(p)
					if err != nil {
						return nil, fmt.Errorf("切片起点 %q 不是整数", p)
					}
					st.sc, st.hasS = n, true
				}
				if p := strings.TrimSpace(parts[1]); p != "" {
					n, err := strconv.Atoi(p)
					if err != nil {
						return nil, fmt.Errorf("切片终点 %q 不是整数", p)
					}
					st.ec, st.hasE = n, true
				}
				steps = append(steps, st)
				break
			}
			n, err := strconv.Atoi(inner)
			if err != nil {
				return nil, fmt.Errorf("无法识别的选择器 [%s]", inner)
			}
			steps = append(steps, step{kind: "index", index: n})
		default:
			return nil, fmt.Errorf("第 %d 字符附近: 无法识别的字符 %q", i+1, expr[i])
		}
	}
	return steps, nil
}

func isNameChar(c byte) bool {
	return c == '_' || c == '-' || c >= '0' && c <= '9' ||
		c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func trimQuote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// evalSteps 依次应用各步骤。
func evalSteps(root interface{}, steps []step) []interface{} {
	cur := []interface{}{root}
	for _, st := range steps {
		var next []interface{}
		for _, v := range cur {
			next = append(next, applyStep(v, st)...)
		}
		cur = next
	}
	return cur
}

func applyStep(v interface{}, st step) []interface{} {
	switch st.kind {
	case "name":
		if m, ok := v.(map[string]interface{}); ok {
			if c, ok := m[st.key]; ok {
				return []interface{}{c}
			}
		}
	case "index":
		if arr, ok := v.([]interface{}); ok {
			idx := st.index
			if idx < 0 {
				idx += len(arr)
			}
			if idx >= 0 && idx < len(arr) {
				return []interface{}{arr[idx]}
			}
		}
	case "slice":
		if arr, ok := v.([]interface{}); ok {
			lo, hi := st.sc, st.ec
			if lo < 0 {
				lo += len(arr)
			}
			if hi < 0 {
				hi += len(arr)
			}
			if !st.hasS {
				lo = 0
			}
			if !st.hasE {
				hi = len(arr)
			}
			if lo < 0 {
				lo = 0
			}
			if hi > len(arr) {
				hi = len(arr)
			}
			if lo <= hi {
				sub := make([]interface{}, hi-lo)
				copy(sub, arr[lo:hi])
				return []interface{}{sub}
			}
		}
	case "wild":
		switch t := v.(type) {
		case map[string]interface{}:
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			out := make([]interface{}, 0, len(t))
			for _, k := range keys {
				out = append(out, t[k])
			}
			return out
		case []interface{}:
			return append([]interface{}{}, t...)
		}
	case "desc":
		var out []interface{}
		var walk func(n interface{})
		walk = func(n interface{}) {
			switch t := n.(type) {
			case map[string]interface{}:
				keys := make([]string, 0, len(t))
				for k := range t {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					if k == st.key {
						out = append(out, t[k])
					}
					walk(t[k])
				}
			case []interface{}:
				for _, e := range t {
					walk(e)
				}
			}
		}
		walk(v)
		return out
	case "descAll":
		var out []interface{}
		var walk func(n interface{})
		walk = func(n interface{}) {
			switch t := n.(type) {
			case map[string]interface{}:
				keys := make([]string, 0, len(t))
				for k := range t {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					out = append(out, t[k])
					walk(t[k])
				}
			case []interface{}:
				for _, e := range t {
					out = append(out, e)
					walk(e)
				}
			}
		}
		walk(v)
		return out
	}
	return nil
}

// formatValue 标量原样输出，对象/数组输出紧凑 JSON。
func formatValue(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		if t {
			return "true"
		}
		return "false"
	case json.Number:
		return t.String()
	case string:
		return t
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	file := fs.String("f", "", "JSON 数据文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }

	// 表达式通常在最前，先把标志参数提取出来再解析
	var flagArgs, posArgs []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && a != "-" && a != "--" {
			flagArgs = append(flagArgs, a)
			if a == "-f" && i+1 < len(args) {
				i++
				flagArgs = append(flagArgs, args[i])
			}
			continue
		}
		posArgs = append(posArgs, a)
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	rest := posArgs
	if len(rest) < 1 {
		fmt.Fprintln(os.Stderr, "缺少 JSONPath 表达式")
		return 2
	}
	expr := rest[0]
	var data []byte
	if len(rest) > 1 {
		data = []byte(strings.Join(rest[1:], " "))
	} else if *file != "" {
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

	steps, err := parsePath(expr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "表达式错误:", err)
		return 2
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	var root interface{}
	if err := dec.Decode(&root); err != nil {
		fmt.Fprintln(os.Stderr, "JSON 解析失败:", err)
		return 1
	}

	matches := evalSteps(root, steps)
	if len(matches) == 0 {
		return 1
	}
	for _, m := range matches {
		fmt.Println(formatValue(m))
	}
	return 0
}
