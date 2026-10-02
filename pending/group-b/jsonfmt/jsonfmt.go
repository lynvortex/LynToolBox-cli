//go:build ignore

// 暂存区：未集成/未完成实现的工具源码，不参与编译。

// Package jsonfmt 实现 JSON 格式化/压缩/校验命令。
// 对应网页版：text/json-format-tool.html（JSON格式化）
//
// 用法：
//
//	lyntoolbox jsonfmt [-i 缩进数] [-o 输出] [文本]
//	echo {"a":1} | lyntoolbox jsonfmt -c
//	lyntoolbox jsonfmt -k -f data.json
package jsonfmt

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	Name  = "jsonfmt"
	Desc  = "JSON 格式化、压缩与合法性校验，错误提示带行列号"
	Usage = `用法: lyntoolbox jsonfmt [-i 缩进数] [-c] [-k] [-f 文件] [-o 输出] [文本]

参数:
  -i        自定义缩进空格数（默认 2）
  -c        压缩为单行
  -k        只校验不输出（合法打印 "JSON 合法"）
  -f        从文件读取 JSON
  -o        输出到文件（默认打印到终端）
  文本      待处理的 JSON 文本；省略且无 -f 时从 stdin 读取`
)

// lineCol 将字节偏移换算为 1 起始的行列号。
func lineCol(data []byte, offset int64) (int, int) {
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	line := 1
	col := 1
	for i := 0; i < int(offset); i++ {
		if data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}

// validate 校验 JSON，非法时返回带行列号的中文错误说明。
func validate(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return describeSyntax(data, err)
	}
	// 确保整个输入只有一个 JSON 值，无多余尾部内容
	if _, err := dec.Token(); err == io.EOF {
		return nil
	} else if err != nil {
		return describeSyntax(data, err)
	}
	_, line, col := trailingOffset(data)
	return fmt.Errorf("第 %d 行第 %d 列: JSON 值之后存在多余内容", line, col)
}

// describeSyntax 把 json 包错误转成带行列号的中文描述。
func describeSyntax(data []byte, err error) error {
	if se, ok := err.(*json.SyntaxError); ok {
		line, col := lineCol(data, se.Offset)
		return fmt.Errorf("第 %d 行第 %d 列: %s", line, col, se.Error())
	}
	return err
}

// trailingOffset 返回第一个 JSON 值结束的位置（用于多余内容提示）。
func trailingOffset(data []byte) (int64, int, int) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v interface{}
	_ = dec.Decode(&v)
	off := dec.InputOffset()
	line, col := lineCol(data, off+1)
	return off, line, col
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	indent := fs.Int("i", 2, "缩进空格数")
	compact := fs.Bool("c", false, "压缩单行")
	checkOnly := fs.Bool("k", false, "只校验")
	file := fs.String("f", "", "输入文件")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *indent < 0 {
		fmt.Fprintln(os.Stderr, "缩进空格数不能为负")
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
	if len(bytes.TrimSpace(data)) == 0 {
		fmt.Fprintln(os.Stderr, "输入为空")
		return 1
	}

	var result []byte
	if *compact {
		var buf bytes.Buffer
		if err := json.Compact(&buf, data); err != nil {
			fmt.Fprintln(os.Stderr, "JSON 非法:", describeSyntax(data, err))
			return 1
		}
		result = buf.Bytes()
	} else {
		var buf bytes.Buffer
		ind := strings.Repeat(" ", *indent)
		if err := json.Indent(&buf, data, "", ind); err != nil {
			fmt.Fprintln(os.Stderr, "JSON 非法:", describeSyntax(data, err))
			return 1
		}
		result = buf.Bytes()
	}

	if *checkOnly {
		if err := validate(data); err != nil {
			fmt.Fprintln(os.Stderr, "JSON 非法:", err)
			return 1
		}
		fmt.Println("JSON 合法")
		return 0
	}

	result = append(bytes.TrimRight(result, "\n"), '\n')
	if *out != "" {
		if err := os.WriteFile(*out, result, 0644); err != nil {
			fmt.Fprintln(os.Stderr, "写入文件失败:", err)
			return 1
		}
		fmt.Printf("已写入 %s（%d 字节）\n", *out, len(result))
		return 0
	}
	os.Stdout.Write(result)
	return 0
}
