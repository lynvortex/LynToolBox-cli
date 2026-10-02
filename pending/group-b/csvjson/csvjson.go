//go:build ignore

// 暂存区：未集成/未完成实现的工具源码，不参与编译。

// Package csvjson 实现 CSV 与 JSON 双向转换命令。
// 对应网页版：text/csv-json-tool.html（CSV与JSON互转）
//
// 用法：
//
//	lyntoolbox csvjson -f data.csv
//	lyntoolbox csvjson -to csv -f data.json
//	lyntoolbox csvjson -noheader -delim ';' -f data.csv
package csvjson

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	Name  = "csvjson"
	Desc  = "CSV 与 JSON 双向转换（表头对象数组、嵌套展平、自定义分隔符与引号）"
	Usage = `用法: lyntoolbox csvjson [-to csv] [-noheader] [-delim 字符] [-quote 字符] [-f 文件] [文本]

参数:
  -to csv    把 JSON 转为 CSV（默认 CSV 转 JSON）
  -noheader  CSV→JSON 时输出数组的数组；-to csv 时不输出表头行（输入为数组的数组）
  -delim     分隔符（默认 ,；多字节字符取首字符）
  -quote     引号字符（默认 "）
  -f         从文件读取
  文本       待处理内容；省略且无 -f 时从 stdin 读取

说明:
  CSV→JSON 首行作表头生成对象数组，单元格保持字符串；
  JSON→CSV 取全部对象字段的并集为列，嵌套对象展平为 a.b 点路径，
  数组与无法展平的值序列化为 JSON 字符串。`
)

// parseCSV 手写解析器，支持自定义分隔符/引号与引号内换行。
func parseCSV(data string, delim, quote rune) [][]string {
	var rows [][]string
	var row []string
	var field strings.Builder
	inQuote := false
	fieldStarted := false

	appendField := func() {
		row = append(row, field.String())
		field.Reset()
		fieldStarted = false
	}
	endRow := func() {
		appendField()
		rows = append(rows, row)
		row = nil
	}

	i := 0
	for i < len(data) {
		c, size := utf8.DecodeRuneInString(data[i:])
		switch {
		case inQuote:
			if c == quote {
				if i+size < len(data) {
					n, ns := utf8.DecodeRuneInString(data[i+size:])
					if n == quote {
						field.WriteRune(quote)
						i += size + ns
						continue
					}
				}
				inQuote = false
				i += size
			} else {
				field.WriteString(data[i : i+size])
				i += size
			}
		case c == quote && !fieldStarted:
			inQuote = true
			fieldStarted = true
			i += size
		case c == delim:
			appendField()
			i += size
		case c == '\r' || c == '\n':
			if c == '\r' && i+1 < len(data) && data[i+1] == '\n' {
				i++
			}
			endRow()
			i++
		default:
			field.WriteRune(c)
			fieldStarted = true
			i += size
		}
	}
	if fieldStarted || field.Len() > 0 || len(row) > 0 {
		endRow()
	}
	return rows
}

// fieldNeedsQuote 判断单元格是否需要引号包裹。
func fieldNeedsQuote(s string, delim, quote rune) bool {
	if s == "" {
		return false
	}
	if strings.ContainsAny(s, "\r\n") {
		return true
	}
	return strings.ContainsRune(s, delim) || strings.ContainsRune(s, quote)
}

// writeCSV 输出 CSV 文本。
func writeCSV(rows [][]string, delim, quote string) string {
	var b bytes.Buffer
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, c := range row {
			if fieldNeedsQuote(c, []rune(delim)[0], []rune(quote)[0]) {
				q, _ := utf8.DecodeRuneInString(quote)
				cells[i] = quote + strings.ReplaceAll(c, string(q), string(q)+string(q)) + quote
			} else {
				cells[i] = c
			}
		}
		b.WriteString(strings.Join(cells, delim))
		b.WriteString("\n")
	}
	return b.String()
}

// cellString 把 JSON 值转为单元格文本。
func cellString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	}
}

// flatten 把嵌套对象展平为点路径键值。
func flatten(prefix string, v map[string]interface{}, out map[string]string, order *[]string) {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sortStrings(keys) // 保证列顺序稳定
	for _, k := range keys {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if sub, ok := v[k].(map[string]interface{}); ok {
			flatten(key, sub, out, order)
			continue
		}
		if !containsStr(*order, key) {
			*order = append(*order, key)
		}
		out[key] = cellString(v[k])
	}
}

// containsStr 判断切片中是否包含指定字符串。
func containsStr(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func firstRune(s string) (rune, error) {
	if s == "" {
		return 0, fmt.Errorf("不能为空")
	}
	r, _ := utf8.DecodeRuneInString(s)
	return r, nil
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	to := fs.String("to", "", "目标格式 csv（JSON 输入）")
	noHeader := fs.Bool("noheader", false, "无表头模式")
	delim := fs.String("delim", ",", "分隔符")
	quote := fs.String("quote", `"`, "引号字符")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dr, err := firstRune(*delim)
	if err != nil || dr == '\n' || dr == '\r' {
		fmt.Fprintln(os.Stderr, "-delim 必须是单个非换行字符")
		return 2
	}
	qr, err := firstRune(*quote)
	if err != nil || qr == '\n' || qr == '\r' || qr == dr {
		fmt.Fprintln(os.Stderr, "-quote 必须是单个字符且不与分隔符相同")
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

	if *to == "csv" {
		return jsonToCSV(data, *noHeader, *delim, *quote, qr)
	}
	return csvToJSON(string(data), *noHeader, dr, qr)
}

// csvToJSON CSV 转 JSON。
func csvToJSON(text string, noHeader bool, delim, quote rune) int {
	rows := parseCSV(text, delim, quote)
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "没有可解析的 CSV 行")
		return 1
	}
	var out interface{}
	if noHeader {
		out = rows
	} else {
		header := rows[0]
		// 处理重复表头：第二次出现加序号
		seen := map[string]int{}
		cols := make([]string, len(header))
		for i, h := range header {
			h = strings.TrimSpace(h)
			seen[h]++
			if seen[h] > 1 {
				h = fmt.Sprintf("%s_%d", h, seen[h])
			}
			cols[i] = h
		}
		records := make([]map[string]string, 0, len(rows)-1)
		for _, row := range rows[1:] {
			obj := map[string]string{}
			for i, c := range cols {
				v := ""
				if i < len(row) {
					v = row[i]
				}
				obj[c] = v
			}
			records = append(records, obj)
		}
		out = records
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "生成 JSON 失败:", err)
		return 1
	}
	fmt.Println(string(b))
	return 0
}

// jsonToCSV JSON 转 CSV。
func jsonToCSV(data []byte, noHeader bool, delim, quote string, qr rune) int {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		fmt.Fprintln(os.Stderr, "JSON 非法:", err)
		return 1
	}

	var rows [][]string
	if arr, ok := v.([]interface{}); ok {
		if noHeader {
			// 数组的数组按行输出
			for _, e := range arr {
				switch row := e.(type) {
				case []interface{}:
					cells := make([]string, len(row))
					for i, c := range row {
						cells[i] = cellString(c)
					}
					rows = append(rows, cells)
				case map[string]interface{}:
					// 对象则按“键=值”输出
					cells := make([]string, 0, len(row))
					keys := make([]string, 0, len(row))
					for k := range row {
						keys = append(keys, k)
					}
					sortStrings(keys)
					for _, k := range keys {
						cells = append(cells, k+"="+cellString(row[k]))
					}
					rows = append(rows, cells)
				default:
					rows = append(rows, []string{cellString(e)})
				}
			}
		} else {
			// 对象数组：取并集列，嵌套对象展平
			var order []string
			flatRows := make([]map[string]string, 0, len(arr))
			for _, e := range arr {
				obj, ok := e.(map[string]interface{})
				if !ok {
					fmt.Fprintln(os.Stderr, "-to csv 需要对象数组，或配合 -noheader 使用数组的数组")
					return 1
				}
				flat := map[string]string{}
				flatten("", obj, flat, &order)
				flatRows = append(flatRows, flat)
			}
			rows = append(rows, order)
			for _, fr := range flatRows {
				cells := make([]string, len(order))
				for i, c := range order {
					cells[i] = fr[c]
				}
				rows = append(rows, cells)
			}
		}
	} else if noHeader {
		fmt.Fprintln(os.Stderr, "-noheader 需要 JSON 数组输入")
		return 1
	} else {
		fmt.Fprintln(os.Stderr, "-to csv 需要 JSON 数组输入")
		return 1
	}

	os.Stdout.WriteString(writeCSV(rows, delim, quote))
	return 0
}
