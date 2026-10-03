// Package csv2sql 实现 CSV 转 SQL INSERT 语句命令。
// 对应网页版：work/csv-sql-tool.html（CSV转SQL）
//
// 用法：
//
//	lyntoolbox csv2sql -table users -f data.csv
//	lyntoolbox csv2sql -table users -dialect pg -batch -create -f data.csv
package csv2sql

import (
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	Name  = "csv2sql"
	Desc  = "CSV 转 SQL INSERT 语句，支持方言、批量合并、类型推断与建表语句"
	Usage = `用法: lyntoolbox csv2sql -table 表名 [-dialect mysql|pg] [-batch [-batch-size N]] [-create] [-f 文件] [文本]

参数:
  -table        目标表名（必需）
  -dialect      方言: mysql(默认，反引号标识符)|pg(双引号标识符)
  -batch        多行合并为一条多 VALUES 的 INSERT
  -batch-size   -batch 时每条语句包含的行数（默认 500）
  -create       在最前附加 CREATE TABLE 建表语句（BIGINT/DOUBLE/TEXT 类型推断）
  -f            从文件读取 CSV
  文本          待处理内容；省略且无 -f 时从 stdin 读取

说明:
  首行为列名。值类型推断: 整数/浮点数字面量、true/false、空串→NULL、
  其余按字符串转义（' → ''）。`
)

// parseCSV 手写 CSV 解析（标准逗号/双引号，支持引号内换行）。
func parseCSV(data string) [][]string {
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
			if c == '"' {
				if i+size < len(data) && data[i+size] == '"' {
					field.WriteByte('"')
					i += size + 1
					continue
				}
				inQuote = false
				i += size
			} else {
				field.WriteString(data[i : i+size])
				i += size
			}
		case c == '"' && !fieldStarted:
			inQuote = true
			fieldStarted = true
			i += size
		case c == ',':
			appendField()
			i += size
		case c == '\r' || c == '\n':
			if c == '\r' && i+1 < len(data) && data[i+1] == '\n' {
				i++
			}
			endRow()
			i++
		default:
			field.WriteString(data[i : i+size])
			fieldStarted = true
			i += size
		}
	}
	if fieldStarted || field.Len() > 0 || len(row) > 0 {
		endRow()
	}
	return rows
}

// quoteIdent 按方言包裹标识符。
func quoteIdent(dialect, name string) string {
	if dialect == "pg" {
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// quoteLiteral 按推断类型与方言把单元格值转为 SQL 字面量。
func quoteLiteral(dialect, s string) string {
	if s == "" {
		return "NULL"
	}
	if strings.EqualFold(s, "true") || strings.EqualFold(s, "false") {
		return strings.ToUpper(s)
	}
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return s
	}
	// 拒绝 inf/NaN 等非有限数，避免拼出非法 SQL
	if f, err := strconv.ParseFloat(s, 64); err == nil && !math.IsInf(f, 0) && !math.IsNaN(f) {
		return s
	}
	if dialect == "pg" {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	// MySQL 默认启用反斜杠转义，必须先转义 \ 再转义 '，否则值可逃逸出字符串
r := strings.NewReplacer(`\`, `\\`, "'", `\'`)
	return "'" + r.Replace(s) + "'"
}

// inferType 按整列推断建表字段类型。
func inferType(values []string) string {
	allInt, allNum := true, true
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, err := strconv.ParseInt(v, 10, 64); err != nil {
			allInt = false
		}
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			allNum = false
		}
		if !allNum {
			break
		}
	}
	if allInt {
		return "BIGINT"
	}
	if allNum {
		return "DOUBLE"
	}
	return "TEXT"
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	table := fs.String("table", "", "目标表名（必需）")
	dialect := fs.String("dialect", "mysql", "SQL 方言 mysql|pg")
	batch := fs.Bool("batch", false, "合并为多 VALUES INSERT")
	batchSize := fs.Int("batch-size", 500, "批量大小")
	create := fs.Bool("create", false, "附加建表语句")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *table == "" {
		fmt.Fprintln(os.Stderr, "缺少 -table 表名")
		return 2
	}
	*dialect = strings.ToLower(*dialect)
	switch *dialect {
	case "mysql", "pg":
	default:
		fmt.Fprintf(os.Stderr, "不支持的方言: %s（可选 mysql|pg）\n", *dialect)
		return 2
	}
	if *batchSize <= 0 {
		fmt.Fprintln(os.Stderr, "-batch-size 必须为正整数")
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

	rows := parseCSV(string(data))
	if len(rows) < 2 {
		fmt.Fprintln(os.Stderr, "CSV 至少需要表头与一行数据")
		return 1
	}
	header := rows[0]
	for i, h := range header {
		header[i] = strings.TrimSpace(h)
		if header[i] == "" {
			header[i] = fmt.Sprintf("col%d", i+1)
		}
	}
	body := rows[1:]

	if *create {
		cols := make([]string, len(header))
		for i, h := range header {
			values := make([]string, 0, len(body))
			for _, r := range body {
				if i < len(r) {
					values = append(values, r[i])
				}
			}
			cols[i] = quoteIdent(*dialect, h) + " " + inferType(values)
		}
		fmt.Printf("CREATE TABLE %s (\n  %s\n);\n\n",
			quoteIdent(*dialect, *table), strings.Join(cols, ",\n  "))
	}

	tbl := quoteIdent(*dialect, *table)
	cols := make([]string, len(header))
	for i, h := range header {
		cols[i] = quoteIdent(*dialect, h)
	}
	colList := strings.Join(cols, ", ")

	rowSQL := func(r []string) string {
		vals := make([]string, len(header))
		for i := range header {
			v := ""
			if i < len(r) {
				v = r[i]
			}
			vals[i] = quoteLiteral(*dialect, v)
		}
		return "(" + strings.Join(vals, ", ") + ")"
	}

	if *batch {
		for start := 0; start < len(body); start += *batchSize {
			end := start + *batchSize
			if end > len(body) {
				end = len(body)
			}
			parts := make([]string, 0, end-start)
			for _, r := range body[start:end] {
				parts = append(parts, rowSQL(r))
			}
			fmt.Printf("INSERT INTO %s (%s) VALUES %s;\n", tbl, colList, strings.Join(parts, ", "))
		}
	} else {
		for _, r := range body {
			fmt.Printf("INSERT INTO %s (%s) VALUES %s;\n", tbl, colList, rowSQL(r))
		}
	}
	return 0
}
