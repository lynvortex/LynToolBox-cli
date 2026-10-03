// Package xlsxtool 实现 Excel 处理命令。
// 对应网页版：document/json-to-excel-tool.html（JSON转Excel）、document/xlsx-view-tool.html（Excel查看器）
package xlsxtool

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/xuri/excelize/v2"
)

const (
	Name  = "xlsxtool"
	Desc  = "Excel 处理：JSON 转 xlsx / xlsx 转 CSV / 工作簿信息"
	Usage = `用法:
  lyntoolbox xlsxtool j2x -o out.xlsx 'JSON数组'    JSON 对象数组导出 Excel
  lyntoolbox xlsxtool x2c -sheet Sheet1 book.xlsx   xlsx 转 CSV
  lyntoolbox xlsxtool info book.xlsx                查看工作表信息`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, Usage+"\n")
		return 2
	}
	mode := args[0]
	switch mode {
	case "j2x":
		return runJ2X(args[1:])
	case "x2c":
		return runX2C(args[1:])
	case "info":
		return runInfo(args[1:])
	}
	fmt.Fprintf(os.Stderr, "未知子命令: %s（j2x|x2c|info）\n", mode)
	return 2
}

// j2x：JSON 对象数组 → xlsx
func runJ2X(args []string) int {
	fs := flag.NewFlagSet("j2x", flag.ContinueOnError)
	out := fs.String("o", "", "输出 xlsx 路径（必需）")
	sheet := fs.String("sheet", "Sheet1", "工作表名")
	file := fs.String("f", "", "从文件读取 JSON")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *out == "" {
		fmt.Fprintln(os.Stderr, "缺少 -o 输出路径")
		return 2
	}
	var data []byte
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取失败:", err)
			return 1
		}
		data = b
	} else if fs.NArg() > 0 {
		data = []byte(strings.Join(fs.Args(), " "))
	} else {
		fmt.Fprintln(os.Stderr, "缺少 JSON 输入")
		return 2
	}

	var rows []map[string]interface{}
	if err := json.Unmarshal(data, &rows); err != nil {
		fmt.Fprintln(os.Stderr, "JSON 解析失败（需对象数组）:", err)
		return 1
	}
	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "JSON 数组为空")
		return 1
	}

	// 表头取首对象键序
	f := excelize.NewFile()
	defer f.Close()
	// 目标名与默认表同名时不能 NewSheet+DeleteSheet（会删掉唯一工作表产出空文件）
	if *sheet != "Sheet1" {
		if _, err := f.NewSheet(*sheet); err != nil {
			fmt.Fprintln(os.Stderr, "创建工作表失败:", err)
			return 1
		}
		f.DeleteSheet("Sheet1") // 默认表替换为指定名
	}
	cols := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		cols = append(cols, k)
	}
	sortStrings(cols)
	for ci, name := range cols {
		cell, _ := excelize.CoordinatesToCellName(ci+1, 1)
		if err := f.SetCellValue(*sheet, cell, name); err != nil {
			fmt.Fprintln(os.Stderr, "写入表头失败:", err)
			return 1
		}
	}
	for ri, row := range rows {
		for ci, name := range cols {
			cell, _ := excelize.CoordinatesToCellName(ci+1, ri+2)
			if err := f.SetCellValue(*sheet, cell, row[name]); err != nil {
				fmt.Fprintln(os.Stderr, "写入单元格失败:", err)
				return 1
			}
		}
	}
	if err := f.SaveAs(*out); err != nil {
		fmt.Fprintln(os.Stderr, "保存失败:", err)
		return 1
	}
	fmt.Printf("已写入 %s：%d 行 × %d 列（工作表 %s）\n", *out, len(rows), len(cols), *sheet)
	return 0
}

func sortStrings(s []string) {
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}

// x2c：xlsx → CSV
func runX2C(args []string) int {
	fs := flag.NewFlagSet("x2c", flag.ContinueOnError)
	sheet := fs.String("sheet", "", "工作表名（默认第一个）")
	out := fs.String("o", "", "输出 CSV（默认打印）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: xlsxtool x2c book.xlsx")
		return 2
	}
	f, err := excelize.OpenFile(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开失败:", err)
		return 1
	}
	defer f.Close()
	name := *sheet
	if name == "" {
		list := f.GetSheetList()
		if len(list) == 0 {
			fmt.Fprintln(os.Stderr, "工作簿没有工作表")
			return 1
		}
		name = list[0]
	}
	rows, err := f.GetRows(name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取失败:", err)
		return 1
	}
	var b strings.Builder
	for _, row := range rows {
		cells := make([]string, len(row))
		for i, v := range row {
			cells[i] = csvEscape(v)
		}
		b.WriteString(strings.Join(cells, ",") + "\n")
	}
	result := b.String()
	if *out != "" {
		if err := os.WriteFile(*out, []byte(result), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "写入失败:", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "已写入 %s（%d 行）\n", *out, len(rows))
		return 0
	}
	fmt.Print(result)
	return 0
}

func csvEscape(s string) string {
	if strings.ContainsAny(s, ",\"\n\r") {
		return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
	}
	return s
}

// info：工作簿信息
func runInfo(args []string) int {
	fs := flag.NewFlagSet("info", flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: xlsxtool info book.xlsx")
		return 2
	}
	st, err := os.Stat(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取失败:", err)
		return 1
	}
	f, err := excelize.OpenFile(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开失败:", err)
		return 1
	}
	defer f.Close()
	fmt.Printf("文件:     %s\n", fs.Arg(0))
	fmt.Printf("大小:     %d 字节\n", st.Size())
	for i, name := range f.GetSheetList() {
		rows, err := f.GetRows(name)
		cols := 0
		if err == nil {
			for _, r := range rows {
				if len(r) > cols {
					cols = len(r)
				}
			}
		}
		fmt.Printf("工作表 %d: %s（%d 行 × %d 列）\n", i+1, name, len(rows), cols)
	}
	return 0
}
