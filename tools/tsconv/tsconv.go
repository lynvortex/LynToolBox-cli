// Package tsconv 实现时间戳/时间格式转换命令。
// 对应网页版：network/timestamp-tool.html（时间戳转换）
//
// 用法：
//
//	lyntoolbox tsconv                      # 当前时间
//	lyntoolbox tsconv 1780000000
//	lyntoolbox tsconv "2026-01-02 15:04:05" -tz Asia/Shanghai
//	lyntoolbox tsconv -diff 1780000000 "2026-06-05 12:00:00"
package tsconv

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	_ "time/tzdata" // 内嵌时区数据库，保证 LoadLocation 离线可用
)

const (
	Name  = "tsconv"
	Desc  = "时间戳与日期时间互转（秒/毫秒/纳秒/RFC3339/常用格式），支持时区与差值计算"
	Usage = `用法: lyntoolbox tsconv [时间] [-tz 时区]
   或: lyntoolbox tsconv -diff 时间1 时间2

参数:
  时间  自动识别: 10位Unix秒 / 13位毫秒 / 19位纳秒 / RFC3339 /
        "2006-01-02 15:04:05" / "2006-01-02"（本地格式按 -tz 或本地时区解释）
  -tz   IANA 时区名，如 Asia/Shanghai、UTC
  -diff 输出两个时间的差值（天/时/分/秒）
  无参数 输出当前时间对照表`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	tz := fs.String("tz", "", "IANA 时区，如 Asia/Shanghai")
	diff := fs.Bool("diff", false, "计算两个时间的差值")
	fs.Usage = func() { fmt.Print(Usage + "\n") }

	// 允许位置参数后仍跟选项（如: tsconv 1780000000 -tz Asia/Shanghai）
	rest := args
	var positionals []string
	for {
		if err := fs.Parse(rest); err != nil {
			if err == flag.ErrHelp {
				return 0
			}
			return 2
		}
		rem := fs.Args()
		if len(rem) == 0 {
			break
		}
		positionals = append(positionals, rem[0])
		rest = rem[1:]
	}

	loc := time.Local
	if *tz != "" {
		l, err := time.LoadLocation(*tz)
		if err != nil {
			fmt.Fprintf(os.Stderr, "加载时区 %s 失败: %v\n（程序已内嵌 tzdata，请检查时区名拼写，如 Asia/Shanghai）\n", *tz, err)
			return 2
		}
		loc = l
	}

	if *diff {
		if len(positionals) != 2 {
			fmt.Fprintln(os.Stderr, "错误: -diff 需要且只需要两个时间参数")
			return 2
		}
		return runDiff(positionals[0], positionals[1], loc)
	}

	if len(positionals) == 0 {
		printTable(time.Now(), "当前时间", loc)
		return 0
	}
	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, "错误: 最多一个时间参数（计算差值请用 -diff）")
		return 2
	}
	t, desc, err := parseTime(strings.TrimSpace(positionals[0]), loc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法识别的时间 %q: %v\n", positionals[0], err)
		return 2
	}
	printTable(t, desc, loc)
	return 0
}

// parseTime 自动识别时间格式
func parseTime(s string, loc *time.Location) (time.Time, string, error) {
	if s == "" {
		return time.Time{}, "", fmt.Errorf("空字符串")
	}
	// 纯数字时间戳
	if isAllDigits(s) {
		switch len(s) {
		case 10:
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return time.Time{}, "", err
			}
			return time.Unix(n, 0), "Unix 秒时间戳", nil
		case 13:
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return time.Time{}, "", err
			}
			return time.UnixMilli(n), "Unix 毫秒时间戳", nil
		case 19:
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil {
				return time.Time{}, "", err
			}
			return time.Unix(0, n), "Unix 纳秒时间戳", nil
		default:
			return time.Time{}, "", fmt.Errorf("数字长度 %d 不是 10（秒）/13（毫秒）/19（纳秒）位", len(s))
		}
	}
	// RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, "RFC3339", nil
	}
	// 本地格式
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, "日期时间（" + layout + "）", nil
		}
	}
	return time.Time{}, "", fmt.Errorf("支持 10/13/19 位时间戳、RFC3339、2006-01-02 [15:04:05]")
}

// runDiff 输出两个时间的差值
func runDiff(s1, s2 string, loc *time.Location) int {
	t1, d1, err := parseTime(strings.TrimSpace(s1), loc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法识别的时间 %q: %v\n", s1, err)
		return 2
	}
	t2, d2, err := parseTime(strings.TrimSpace(s2), loc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法识别的时间 %q: %v\n", s2, err)
		return 2
	}
	d := t1.Sub(t2)
	fmt.Printf("t1: %s（%s）\nt2: %s（%s）\n", t1.Format(time.RFC3339Nano), d1, t2.Format(time.RFC3339Nano), d2)
	if d == 0 {
		fmt.Println("相差: 两个时间相同")
		return 0
	}
	sign := "晚"
	if d < 0 {
		sign = "早"
	}
	abs := d.Abs()
	days := int64(abs.Hours()) / 24
	hours := int64(abs.Hours()) % 24
	mins := int64(abs.Minutes()) % 60
	secs := int64(abs.Seconds()) % 60
	fmt.Printf("相差: %d 天 %d 小时 %d 分 %d 秒（t1 比 t2 %s %s 秒）\n",
		days, hours, mins, secs, sign, strconv.FormatInt(int64(abs.Seconds()), 10))
	return 0
}

// printTable 输出时间对照表
func printTable(t time.Time, desc string, loc *time.Location) {
	fmt.Printf("输入:      %s（%s）\n", t.Format(time.RFC3339Nano), desc)
	fmt.Printf("本地时间:  %s\n", t.Local().Format("2006-01-02 15:04:05 MST (-07:00)"))
	if loc != time.Local {
		fmt.Printf("指定时区:  %s\n", t.In(loc).Format("2006-01-02 15:04:05 MST (-07:00)"))
	}
	fmt.Printf("UTC:       %s\n", t.UTC().Format("2006-01-02 15:04:05 MST (-07:00)"))
	fmt.Printf("RFC3339:   %s\n", t.Format(time.RFC3339Nano))
	fmt.Printf("Unix 秒:   %d\n", t.Unix())
	fmt.Printf("Unix 毫秒: %d\n", t.UnixMilli())
	fmt.Printf("星期:      %s\n", weekdayCN(t.Weekday()))
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

func weekdayCN(w time.Weekday) string {
	return [...]string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}[w]
}
