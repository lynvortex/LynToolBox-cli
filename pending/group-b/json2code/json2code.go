//go:build ignore

// 暂存区：未集成/未完成实现的工具源码，不参与编译。

// Package json2code 实现 JSON 到各语言类型定义的转换命令。
// 对应网页版：work/json-to-code-tool.html（JSON转代码）
//
// 用法：
//
//	lyntoolbox json2code -lang go -f data.json
//	echo '{"id":1,"name":"a"}' | lyntoolbox json2code
package json2code

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode"
)

const (
	Name  = "json2code"
	Desc  = "把 JSON 数据推断并生成 TS/Go/Java/C#/Python 类型定义"
	Usage = `用法: lyntoolbox json2code [-lang ts|go|java|csharp|python] [-name 类型名] [-f 文件] [文本]

参数:
  -lang     目标语言: ts(默认)|go|java|csharp|python
  -name     根类型名（默认 Root）
  -f        从文件读取 JSON
  文本      待处理的 JSON；省略且无 -f 时从 stdin 读取

说明:
  对象生成类型定义并递归展开；数组取首元素推断元素类型，空数组为 any/Object；
  数字区分整数/浮点；字段名按各语言命名惯例转换（TS camelCase、Go 导出 PascalCase
  带 json tag、Java 字段+getter、C# 属性、Python dataclass snake_case）。`
)

// fieldDef 一个类型的字段。
type fieldDef struct {
	orig    string // JSON 原始字段名（json tag 用）
	outName string // 转换后的字段名
	typ     string // 该语言下的类型
}

// typeDef 一个已生成的类型定义。
type typeDef struct {
	name   string
	fields []fieldDef
}

type generator struct {
	lang       string
	defs       []typeDef
	used       map[string]bool
	needTyping bool // Python 是否需要 from typing import ...
}

// uniqueType 生成不重复的类型名。
func (g *generator) uniqueType(base string) string {
	base = pascal(base)
	if base == "" {
		base = "Root"
	}
	if !unicode.IsLetter(rune(base[0])) {
		base = "Type" + base
	}
	name := base
	for i := 2; g.used[name]; i++ {
		name = base + fmt.Sprint(i)
	}
	g.used[name] = true
	return name
}

// genType 递归推断值的类型；对象会注册新的类型定义。
func (g *generator) genType(v interface{}, suggest string) string {
	switch t := v.(type) {
	case map[string]interface{}:
		typeName := g.uniqueType(suggest)
		def := typeDef{name: typeName}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			ft := g.genType(t[k], k)
			outName := k
			switch g.lang {
			case "ts":
				outName = camel(k)
			case "go":
				outName = pascal(k)
			case "python":
				outName = snake(k)
			case "java", "csharp":
				outName = camel(k)
			}
			def.fields = append(def.fields, fieldDef{orig: k, outName: outName, typ: ft})
		}
		g.defs = append(g.defs, def)
		return typeName
	case []interface{}:
		if len(t) == 0 {
			return g.emptyArrayType()
		}
		elem := g.genType(t[0], suggest+"Item")
		return g.arrayType(elem)
	case json.Number:
		s := t.String()
		if strings.ContainsAny(s, ".eE") {
			return g.scalarType("float")
		}
		return g.scalarType("int")
	case string:
		return g.scalarType("string")
	case bool:
		return g.scalarType("bool")
	default: // nil
		return g.scalarType("null")
	}
}

func (g *generator) emptyArrayType() string {
	switch g.lang {
	case "go":
		return "[]interface{}"
	case "java":
		return "List<Object>"
	case "csharp":
		return "List<object>"
	case "python":
		g.needTyping = true
		return "List[Any]"
	}
	return "any[]"
}

func (g *generator) arrayType(elem string) string {
	switch g.lang {
	case "go":
		return "[]" + elem
	case "java":
		return "List<" + elem + ">"
	case "csharp":
		return "List<" + elem + ">"
	case "python":
		g.needTyping = true
		return "List[" + elem + "]"
	}
	return elem + "[]"
}

func (g *generator) scalarType(kind string) string {
	switch g.lang {
	case "ts":
		switch kind {
		case "int", "float":
			return "number"
		case "string":
			return "string"
		case "bool":
			return "boolean"
		default:
			return "null"
		}
	case "go":
		switch kind {
		case "int":
			return "int64"
		case "float":
			return "float64"
		case "string":
			return "string"
		case "bool":
			return "bool"
		default:
			return "interface{}"
		}
	case "java":
		switch kind {
		case "int":
			return "long"
		case "float":
			return "double"
		case "string":
			return "String"
		case "bool":
			return "boolean"
		default:
			return "Object"
		}
	case "csharp":
		switch kind {
		case "int":
			return "long"
		case "float":
			return "double"
		case "string":
			return "string"
		case "bool":
			return "bool"
		default:
			return "object"
		}
	default: // python
		switch kind {
		case "int":
			return "int"
		case "float":
			return "float"
		case "string":
			return "str"
		case "bool":
			return "bool"
		default:
			return "None"
		}
	}
}

// render 按语言输出全部类型定义。
func (g *generator) render(root string) string {
	var b strings.Builder
	switch g.lang {
	case "ts":
		for _, d := range g.defs {
			fmt.Fprintf(&b, "export interface %s {\n", d.name)
			for _, f := range d.fields {
				fmt.Fprintf(&b, "  %s: %s;\n", f.outName, f.typ)
			}
			b.WriteString("}\n\n")
		}
	case "go":
		for _, d := range g.defs {
			// 计算对齐宽度
			nameW, typeW := 0, 0
			for _, f := range d.fields {
				if len(f.outName) > nameW {
					nameW = len(f.outName)
				}
				if len(f.typ) > typeW {
					typeW = len(f.typ)
				}
			}
			fmt.Fprintf(&b, "type %s struct {\n", d.name)
			for _, f := range d.fields {
				fmt.Fprintf(&b, "\t%-*s %-*s `json:\"%s\"`\n",
					nameW, f.outName, typeW, f.typ, f.orig)
			}
			b.WriteString("}\n\n")
		}
	case "java":
		for _, d := range g.defs {
			fmt.Fprintf(&b, "public class %s {\n", d.name)
			for _, f := range d.fields {
				fmt.Fprintf(&b, "    private %s %s;\n", f.typ, f.outName)
			}
			for _, f := range d.fields {
				b.WriteString("\n")
				getter := "get" + pascal(f.outName)
				if f.typ == "boolean" {
					getter = "is" + pascal(f.outName)
				}
				fmt.Fprintf(&b, "    public %s %s() {\n        return %s;\n    }\n", f.typ, getter, f.outName)
			}
			b.WriteString("}\n\n")
		}
	case "csharp":
		for _, d := range g.defs {
			fmt.Fprintf(&b, "public class %s\n{\n", d.name)
			for _, f := range d.fields {
				fmt.Fprintf(&b, "    public %s %s { get; set; }\n", f.typ, pascal(f.outName))
			}
			b.WriteString("}\n\n")
		}
	case "python":
		if g.needTyping {
			b.WriteString("from typing import Any, List\n\n")
		}
		b.WriteString("from dataclasses import dataclass\n\n")
		for _, d := range g.defs {
			fmt.Fprintf(&b, "@dataclass\nclass %s:\n", d.name)
			for _, f := range d.fields {
				fmt.Fprintf(&b, "    %s: %s\n", f.outName, f.typ)
			}
			b.WriteString("\n")
		}
	}
	_ = root
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// splitWords 把标识符按分隔符拆成单词片段。
func splitWords(s string) []string {
	var parts []string
	var cur strings.Builder
	for _, r := range s {
		switch {
		case r == '_' || r == '-' || r == ' ' || r == '.':
			if cur.Len() > 0 {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		case unicode.IsUpper(r) && cur.Len() > 0:
			rs := []rune(cur.String())
			// 大写开头的新词（前一个不是大写开头时才切分，如 parseURL 不切）
			if !unicode.IsUpper(rs[len(rs)-1]) {
				parts = append(parts, cur.String())
				cur.Reset()
			}
			cur.WriteRune(r)
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	if len(parts) == 0 {
		return []string{s}
	}
	return parts
}

func capFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// camel 转小驼峰。
func camel(s string) string {
	parts := splitWords(s)
	var b strings.Builder
	for i, p := range parts {
		if i == 0 {
			b.WriteString(lowerFirst(p))
		} else {
			b.WriteString(capFirst(lowerFirst(p)))
		}
	}
	return b.String()
}

// pascal 转大驼峰（Go 导出名）。
func pascal(s string) string {
	parts := splitWords(s)
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(capFirst(lowerFirst(p)))
	}
	return b.String()
}

// snake 转下划线小写。
func snake(s string) string {
	parts := splitWords(s)
	for i := range parts {
		parts[i] = strings.ToLower(parts[i])
	}
	return strings.Join(parts, "_")
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	lang := fs.String("lang", "ts", "目标语言 ts|go|java|csharp|python")
	root := fs.String("name", "Root", "根类型名")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	*lang = strings.ToLower(*lang)
	switch *lang {
	case "ts", "go", "java", "csharp", "python":
	default:
		fmt.Fprintf(os.Stderr, "不支持的语言: %s（可选 ts|go|java|csharp|python）\n", *lang)
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

	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		fmt.Fprintln(os.Stderr, "JSON 解析失败:", err)
		return 1
	}

	g := &generator{lang: *lang, used: map[string]bool{}}
	g.genType(v, *root)
	fmt.Print(g.render(*root))
	return 0
}
