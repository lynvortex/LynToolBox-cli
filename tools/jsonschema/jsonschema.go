// Package jsonschema 实现 JSON Schema 校验命令（手写核心子集）。
// 对应网页版：work/jsonschema-tool.html（JSON Schema校验）
//
// 支持关键字: type(含数组)、required、properties、items、enum、const、
// minimum/maximum/exclusiveMinimum/exclusiveMaximum、minLength/maxLength、
// pattern、minItems/maxItems、additionalProperties(bool)
//
// 用法：
//
//	lyntoolbox jsonschema -s schema.json -f data.json
//	lyntoolbox jsonschema -s schema.json '{"id":-1}'
//	cat data.json | lyntoolbox jsonschema -s schema.json
package jsonschema

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strings"
)

const (
	Name  = "jsonschema"
	Desc  = "按 JSON Schema 校验 JSON 数据，逐条列出错误与 JSON Path"
	Usage = `用法: lyntoolbox jsonschema -s schema文件 [-f 数据文件] [数据文本]

参数:
  -s        Schema 文件路径（必需）
  -f        从文件读取待校验 JSON
  数据文本  待校验的 JSON；省略且无 -f 时从 stdin 读取

说明:
  支持核心子集: type/required/properties/items/enum/const/数值与字符串长度/
  pattern/minItems/maxItems/additionalProperties。错误按 JSON Path（$.a[0].b）
  逐行列出。校验失败退出码 1。`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	schemaFile := fs.String("s", "", "Schema 文件")
	file := fs.String("f", "", "数据文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *schemaFile == "" {
		fmt.Fprintln(os.Stderr, "缺少 -s schema 文件")
		return 2
	}

	sb, err := os.ReadFile(*schemaFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取 schema 失败:", err)
		return 1
	}
	var schema interface{}
	dec := json.NewDecoder(strings.NewReader(string(sb)))
	dec.UseNumber()
	if err := dec.Decode(&schema); err != nil {
		fmt.Fprintln(os.Stderr, "Schema 不是合法 JSON:", err)
		return 1
	}
	if _, ok := schema.(map[string]interface{}); !ok {
		fmt.Fprintln(os.Stderr, "Schema 必须是 JSON 对象")
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
	var value interface{}
	dec = json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		fmt.Fprintln(os.Stderr, "数据不是合法 JSON:", err)
		return 1
	}

	var errs []string
	validate(value, schema, "$", &errs)
	if len(errs) == 0 {
		fmt.Println("校验通过")
		return 0
	}
	for _, e := range errs {
		fmt.Println(e)
	}
	fmt.Printf("共 %d 处错误\n", len(errs))
	return 1
}

// validate 递归校验，path 为当前值的 JSON Path。
func validate(v interface{}, schema interface{}, path string, errs *[]string) {
	s, ok := schema.(map[string]interface{})
	if !ok {
		return // 布尔 schema 或非法 schema 跳过
	}

	// type：字符串或数组
	if tv, ok := s["type"]; ok {
		var types []string
		switch t := tv.(type) {
		case string:
			types = []string{t}
		case []interface{}:
			for _, e := range t {
				if es, ok := e.(string); ok {
					types = append(types, es)
				}
			}
		}
		if len(types) > 0 {
			matched := false
			for _, ty := range types {
				if typeMatches(v, ty) {
					matched = true
					break
				}
			}
			if !matched {
				*errs = append(*errs, fmt.Sprintf("%s: 类型应为 %s，实际为 %s",
					path, strings.Join(types, " 或 "), typeName(v)))
			}
		}
	}

	// enum / const
	if ev, ok := s["enum"]; ok {
		if arr, ok := ev.([]interface{}); ok && !containsValue(arr, v) {
			*errs = append(*errs, fmt.Sprintf("%s: 值不在枚举范围内（enum）", path))
		}
	}
	if cv, ok := s["const"]; ok && !jsonEqual(cv, v) {
		*errs = append(*errs, fmt.Sprintf("%s: 值必须等于 const 指定的常量", path))
	}

	switch t := v.(type) {
	case map[string]interface{}:
		// required
		if rv, ok := s["required"]; ok {
			if arr, ok := rv.([]interface{}); ok {
				for _, e := range arr {
					if k, ok := e.(string); ok {
						if _, exists := t[k]; !exists {
							*errs = append(*errs, fmt.Sprintf("%s: 缺少必需字段 %q", path, k))
						}
					}
				}
			}
		}
		// properties
		props, _ := s["properties"].(map[string]interface{})
		for k, pv := range props {
			if child, exists := t[k]; exists {
				validate(child, pv, path+"."+k, errs)
			}
		}
		// additionalProperties: false
		if ap, ok := s["additionalProperties"]; ok {
			if ab, ok := ap.(bool); ok && !ab {
				for k := range t {
					if _, declared := props[k]; !declared {
						*errs = append(*errs, fmt.Sprintf("%s.%s: 不允许的额外字段", path, k))
					}
				}
			}
		}
	case []interface{}:
		if iv, ok := s["items"]; ok {
			for i, e := range t {
				validate(e, iv, fmt.Sprintf("%s[%d]", path, i), errs)
			}
		}
		if mi, ok := s["minItems"]; ok {
			if n, ok := toNumber(mi); ok && float64(len(t)) < n {
				*errs = append(*errs, fmt.Sprintf("%s: 数组元素数 %d 少于 minItems %d", path, len(t), int(n)))
			}
		}
		if ma, ok := s["maxItems"]; ok {
			if n, ok := toNumber(ma); ok && float64(len(t)) > n {
				*errs = append(*errs, fmt.Sprintf("%s: 数组元素数 %d 超过 maxItems %d", path, len(t), int(n)))
			}
		}
	case json.Number:
		n, _ := t.Float64()
		if mv, ok := s["minimum"]; ok {
			if m, ok := toNumber(mv); ok && n < m {
				*errs = append(*errs, fmt.Sprintf("%s: 数值 %s 小于 minimum %s", path, t.String(), numStr(mv)))
			}
		}
		if mv, ok := s["maximum"]; ok {
			if m, ok := toNumber(mv); ok && n > m {
				*errs = append(*errs, fmt.Sprintf("%s: 数值 %s 大于 maximum %s", path, t.String(), numStr(mv)))
			}
		}
		if mv, ok := s["exclusiveMinimum"]; ok {
			if m, ok := toNumber(mv); ok && n <= m {
				*errs = append(*errs, fmt.Sprintf("%s: 数值 %s 未大于 exclusiveMinimum %s", path, t.String(), numStr(mv)))
			}
		}
		if mv, ok := s["exclusiveMaximum"]; ok {
			if m, ok := toNumber(mv); ok && n >= m {
				*errs = append(*errs, fmt.Sprintf("%s: 数值 %s 未小于 exclusiveMaximum %s", path, t.String(), numStr(mv)))
			}
		}
	case string:
		runeLen := len([]rune(t))
		if mv, ok := s["minLength"]; ok {
			if m, ok := toNumber(mv); ok && float64(runeLen) < m {
				*errs = append(*errs, fmt.Sprintf("%s: 字符串长度 %d 少于 minLength %d", path, runeLen, int(m)))
			}
		}
		if mv, ok := s["maxLength"]; ok {
			if m, ok := toNumber(mv); ok && float64(runeLen) > m {
				*errs = append(*errs, fmt.Sprintf("%s: 字符串长度 %d 超过 maxLength %d", path, runeLen, int(m)))
			}
		}
		if pv, ok := s["pattern"]; ok {
			if ps, ok := pv.(string); ok {
				re, err := regexp.Compile(ps)
				if err != nil {
					*errs = append(*errs, fmt.Sprintf("%s: pattern 正则无效（Go RE2 语法）: %v", path, err))
				} else if !re.MatchString(t) {
					*errs = append(*errs, fmt.Sprintf("%s: 字符串不匹配 pattern %q", path, ps))
				}
			}
		}
	}
}

// typeMatches 判断值是否符合单个类型名。
func typeMatches(v interface{}, ty string) bool {
	switch ty {
	case "object":
		_, ok := v.(map[string]interface{})
		return ok
	case "array":
		_, ok := v.([]interface{})
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "number":
		_, ok := v.(json.Number)
		return ok
	case "integer":
		n, ok := v.(json.Number)
		if !ok {
			return false
		}
		return !strings.ContainsAny(n.String(), ".eE")
	case "null":
		return v == nil
	}
	return false
}

func typeName(v interface{}) string {
	switch t := v.(type) {
	case map[string]interface{}:
		return "object"
	case []interface{}:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number:
		if !strings.ContainsAny(t.String(), ".eE") {
			return "integer"
		}
		return "number"
	default:
		return "null"
	}
}

// toNumber 把 schema 中的数值取出（json.Number 或 float64）。
func toNumber(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case float64:
		return t, true
	case int:
		return float64(t), true
	}
	return 0, false
}

func numStr(v interface{}) string {
	if n, ok := v.(json.Number); ok {
		return n.String()
	}
	return fmt.Sprint(v)
}

// containsValue 判断值是否在枚举列表中。
func containsValue(arr []interface{}, v interface{}) bool {
	for _, e := range arr {
		if jsonEqual(e, v) {
			return true
		}
	}
	return false
}

// jsonEqual 深比较两个解码后的 JSON 值。
func jsonEqual(a, b interface{}) bool {
	af, aok := toNumber(a)
	bf, bok := toNumber(b)
	if aok && bok {
		return af == bf || (math.IsNaN(af) && math.IsNaN(bf))
	}
	if aok != bok {
		return false
	}
	switch at := a.(type) {
	case nil:
		return b == nil
	case string:
		bs, ok := b.(string)
		return ok && at == bs
	case bool:
		bb, ok := b.(bool)
		return ok && at == bb
	case []interface{}:
		bar, ok := b.([]interface{})
		if !ok || len(at) != len(bar) {
			return false
		}
		for i := range at {
			if !jsonEqual(at[i], bar[i]) {
				return false
			}
		}
		return true
	case map[string]interface{}:
		bm, ok := b.(map[string]interface{})
		if !ok || len(at) != len(bm) {
			return false
		}
		for k, av := range at {
			bv, exists := bm[k]
			if !exists || !jsonEqual(av, bv) {
				return false
			}
		}
		return true
	}
	return false
}
