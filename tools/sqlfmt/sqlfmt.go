// Package sqlfmt 实现 SQL 格式化/压缩命令。
// 对应网页版：work/sql-format-tool.html（SQL格式化）
//
// 用法：
//
//	lyntoolbox sqlfmt -f query.sql
//	lyntoolbox sqlfmt -i 4 'select id,name from users where id=1'
//	lyntoolbox sqlfmt -c 'SELECT a FROM t'
package sqlfmt

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	Name  = "sqlfmt"
	Desc  = "SQL 格式化：关键字大写、主子句换行缩进、字段逗号对齐（字符串与括号内不改动）"
	Usage = `用法: lyntoolbox sqlfmt [-i 缩进数] [-c] [-f 文件] [文本]

参数:
  -i        缩进空格数（默认 2）
  -c        压缩为单行（去除注释与多余空白，不做变量改名）
  -f        从文件读取
  文本      待处理 SQL；省略且无 -f 时从 stdin 读取

说明:
  基础美化：识别常见关键字大写并在主子句处换行；顶层逗号字段换行缩进；
  字符串字面量与括号内内容不做重排（仅规范化空白）；注释在格式化时尽量
  保留、压缩时删除。与关键字同名的标识符可能被误大写。`
)

type tokKind int

const (
	tkWord tokKind = iota
	tkString
	tkIdent
	tkPunct
	tkComment
	tkGroup
)

type token struct {
	kind tokKind
	text string
}

var keywordSet = map[string]bool{
	"SELECT": true, "FROM": true, "WHERE": true, "GROUP": true, "BY": true,
	"ORDER": true, "HAVING": true, "LIMIT": true, "OFFSET": true,
	"JOIN": true, "LEFT": true, "RIGHT": true, "INNER": true, "OUTER": true,
	"FULL": true, "CROSS": true, "NATURAL": true, "ON": true, "USING": true,
	"AS": true, "AND": true, "OR": true, "NOT": true, "IN": true, "IS": true,
	"NULL": true, "LIKE": true, "BETWEEN": true, "EXISTS": true,
	"UNION": true, "ALL": true, "ANY": true, "DISTINCT": true,
	"CASE": true, "WHEN": true, "THEN": true, "ELSE": true, "END": true,
	"INSERT": true, "INTO": true, "VALUES": true, "UPDATE": true, "SET": true,
	"DELETE": true, "CREATE": true, "TABLE": true, "DROP": true, "ALTER": true,
	"ADD": true, "PRIMARY": true, "FOREIGN": true, "KEY": true, "REFERENCES": true,
	"DEFAULT": true, "UNIQUE": true, "INDEX": true, "ASC": true, "DESC": true,
	"TRUNCATE": true, "TRUE": true, "FALSE": true, "INTERSECT": true,
	"EXCEPT": true, "COLLATE": true, "RETURNING": true,
}

func isIdentChar(c byte) bool {
	return c == '_' || c == '$' || c == '#' || c >= '0' && c <= '9' ||
		c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

// skipString 从 i 处的引号开始跳过字符串体，返回结束位置（引号之后）。
func skipString(src string, i int) int {
	q := src[i]
	i++
	for i < len(src) {
		if src[i] == q {
			if i+1 < len(src) && src[i+1] == q { // '' 转义
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return i
}

// tokenize 把 SQL 拆为 token；括号内容整体作为 group 原样保留。
func tokenize(src string) []token {
	var toks []token
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '-' && i+1 < len(src) && src[i+1] == '-':
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				j = len(src)
			} else {
				j += i
			}
			toks = append(toks, token{tkComment, src[i:j]})
			i = j
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			j := strings.Index(src[i+2:], "*/")
			if j < 0 {
				toks = append(toks, token{tkComment, src[i:]})
				i = len(src)
			} else {
				end := i + 2 + j + 2
				toks = append(toks, token{tkComment, src[i:end]})
				i = end
			}
		case c == '\'' || c == '"' || c == '`':
			j := skipString(src, i)
			kind := tkString
			if c != '\'' {
				kind = tkIdent
			}
			toks = append(toks, token{kind, src[i:j]})
			i = j
		case c == '[':
			j := strings.IndexByte(src[i:], ']')
			if j < 0 {
				toks = append(toks, token{tkIdent, src[i:]})
				i = len(src)
			} else {
				j += i + 1
				toks = append(toks, token{tkIdent, src[i:j]})
				i = j
			}
		case c == '(':
			// 括号内容原样复制（考虑嵌套、字符串与注释）
			depth := 1
			j := i + 1
			for j < len(src) && depth > 0 {
				switch {
				case src[j] == '\'' || src[j] == '"' || src[j] == '`':
					j = skipString(src, j)
					continue
				case src[j] == '-' && j+1 < len(src) && src[j+1] == '-':
					k := strings.IndexByte(src[j:], '\n')
					if k < 0 {
						j = len(src)
					} else {
						j += k
					}
					continue
				case src[j] == '/' && j+1 < len(src) && src[j+1] == '*':
					k := strings.Index(src[j+2:], "*/")
					if k < 0 {
						j = len(src)
					} else {
						j = j + 2 + k + 2
					}
					continue
				case src[j] == '(':
					depth++
				case src[j] == ')':
					depth--
				}
				j++
			}
			toks = append(toks, token{tkGroup, src[i:j]})
			i = j
		case isIdentChar(c):
			j := i
			for j < len(src) && isIdentChar(src[j]) {
				j++
			}
			toks = append(toks, token{tkWord, src[i:j]})
			i = j
		default:
			// 运算符：合并常见双字符写法
			if i+1 < len(src) {
				two := src[i : i+2]
				if two == "<=" || two == ">=" || two == "<>" || two == "!=" ||
					two == "||" || two == "::" || two == "->" || two == "=>" {
					toks = append(toks, token{tkPunct, two})
					i += 2
					continue
				}
			}
			toks = append(toks, token{tkPunct, src[i : i+1]})
			i++
		}
	}
	return toks
}

func collapseWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// needSpace 两个 token 之间是否需要空格。
func needSpace(prev, cur token) bool {
	if prev.kind == 0 && prev.text == "" {
		return false
	}
	if cur.kind == tkPunct && (cur.text == "," || cur.text == ";" || cur.text == ")" || cur.text == ".") {
		return false
	}
	if prev.kind == tkPunct && (prev.text == "(" || prev.text == ".") {
		return false
	}
	// 括号组紧贴前面的函数名/关键字：count(a)
	if cur.kind == tkGroup && strings.HasPrefix(cur.text, "(") {
		return false
	}
	if prev.kind == tkGroup && strings.HasPrefix(prev.text, "(") && strings.HasSuffix(prev.text, ")") &&
		cur.kind == tkPunct && cur.text == "(" {
		return false
	}
	return true
}

func appendTok(line *strings.Builder, prev *token, t token) {
	if line.Len() > 0 && needSpace(*prev, t) {
		line.WriteString(" ")
	}
	line.WriteString(t.text)
	*prev = t
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	indent := fs.Int("i", 2, "缩进空格数")
	compact := fs.Bool("c", false, "压缩单行")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
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
	toks := tokenize(string(data))
	if len(toks) == 0 {
		fmt.Fprintln(os.Stderr, "输入为空")
		return 1
	}

	var out strings.Builder
	if *compact {
		formatCompact(&out, toks)
	} else {
		formatPretty(&out, toks, *indent)
	}
	fmt.Print(out.String())
	return 0
}

// formatPretty 主子句换行缩进的格式化输出。
func formatPretty(out *strings.Builder, toks []token, indent int) {
	indStr := strings.Repeat(" ", indent)
	var line strings.Builder
	prev := token{}
	curIndent := ""  // 当前累积行的缩进（行开始时确定）
	prevClause := "" // 上一个主子句关键字（DELETE 特判 FROM）

	// flush 按行开始时的缩进输出当前行（换行后重置 prev，避免行首多余空格）
	flush := func() {
		if line.Len() > 0 {
			out.WriteString(curIndent + line.String() + "\n")
			line.Reset()
		}
		prev = token{}
	}
	// setCont 更新延续行状态与下一行缩进
	setCont := func(v bool) {
		if v {
			curIndent = indStr
		} else {
			curIndent = ""
		}
	}

	i := 0
	for i < len(toks) {
		t := toks[i]
		switch t.kind {
		case tkGroup:
			g := token{tkGroup, collapseWS(t.text)}
			appendTok(&line, &prev, g)
		case tkString, tkIdent:
			appendTok(&line, &prev, t)
		case tkPunct:
			switch t.text {
			case ",":
				line.WriteString(",")
				prev = t
				flush()
				setCont(true)
			case ";":
				line.WriteString(";")
				prev = t
				flush()
				setCont(false)
				prevClause = ""
			default:
				appendTok(&line, &prev, t)
			}
		case tkComment:
			// 注释独占一行（单行块注释保持行内）
			if !strings.Contains(t.text, "\n") && strings.HasPrefix(t.text, "/*") {
				appendTok(&line, &prev, t)
			} else {
				flush()
				out.WriteString(t.text + "\n")
			}
		case tkWord:
			up := strings.ToUpper(t.text)
			if !keywordSet[up] {
				appendTok(&line, &prev, t)
				break
			}
			// 多词关键字与主子句处理
			switch up {
			case "SELECT", "VALUES":
				flush()
				setCont(false)
				line.WriteString(up)
				prev = token{tkWord, up}
				flush() // 关键字独占一行，字段进入延续行
				setCont(true)
				prevClause = up
			case "INSERT", "UPDATE", "DELETE", "CREATE", "TRUNCATE":
				flush()
				setCont(false)
				line.WriteString(up)
				prev = token{tkWord, up}
				prevClause = up
			case "WHERE", "HAVING", "LIMIT", "OFFSET", "SET":
				flush()
				setCont(false)
				line.WriteString(up)
				prev = token{tkWord, up}
				prevClause = up
			case "FROM":
				if prevClause != "DELETE" {
					flush()
					setCont(false)
					line.WriteString(up)
					prev = token{tkWord, up}
				} else {
					// DELETE FROM 保持同行
					appendTok(&line, &prev, token{tkWord, up})
				}
				prevClause = "FROM"
			case "GROUP", "ORDER":
				flush()
				setCont(false)
				// 消费跟在后面的 BY
				if i+1 < len(toks) && toks[i+1].kind == tkWord &&
					strings.EqualFold(toks[i+1].text, "BY") {
					line.WriteString(up + " BY")
					i++
				} else {
					line.WriteString(up)
				}
				prev = token{tkWord, "BY"}
				prevClause = up
			case "UNION":
				flush()
				setCont(false)
				if i+1 < len(toks) && toks[i+1].kind == tkWord &&
					strings.EqualFold(toks[i+1].text, "ALL") {
					line.WriteString("UNION ALL")
					i++
				} else {
					line.WriteString("UNION")
				}
				prev = token{tkWord, "UNION"}
				prevClause = "UNION"
			case "JOIN":
				flush()
				setCont(false)
				line.WriteString("JOIN")
				prev = token{tkWord, "JOIN"}
				prevClause = "JOIN"
			case "LEFT", "RIGHT", "INNER", "FULL", "CROSS", "NATURAL":
				// [OUTER] JOIN 序列整体作为子句开头
				j := i + 1
				if j < len(toks) && toks[j].kind == tkWord &&
					strings.EqualFold(toks[j].text, "OUTER") {
					j++
				}
				if j < len(toks) && toks[j].kind == tkWord &&
					strings.EqualFold(toks[j].text, "JOIN") {
					flush()
					setCont(false)
					seq := up
					for k := i + 1; k <= j; k++ {
						seq += " " + strings.ToUpper(toks[k].text)
					}
					line.WriteString(seq)
					prev = token{tkWord, "JOIN"}
					i = j
					prevClause = "JOIN"
				} else {
					appendTok(&line, &prev, token{tkWord, up})
				}
			default:
				// 其余关键字：仅大写
				appendTok(&line, &prev, token{tkWord, up})
			}
		}
		i++
	}
	flush()
}

// formatCompact 压缩为单行（删除注释）。
func formatCompact(out *strings.Builder, toks []token) {
	var line strings.Builder
	prev := token{}
	for _, t := range toks {
		if t.kind == tkComment {
			continue
		}
		if t.kind == tkGroup {
			t = token{tkGroup, collapseWS(t.text)}
		}
		appendTok(&line, &prev, t)
	}
	out.WriteString(line.String() + "\n")
}
