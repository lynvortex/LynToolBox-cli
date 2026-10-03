// Package datecalc 实现日期计算命令。
// 对应网页版：daily/date-calc-tool.html（日期计算）、daily/rrule-tool.html（重复日程）、
// text/second-time-change-tool.html（秒数换算）
//
// 用法：
//
//	lyntoolbox datecalc diff 2026-10-01 2026-10-08
//	lyntoolbox datecalc add 2024-01-31 1 m
//	lyntoolbox datecalc weekday 2026-10-03
//	lyntoolbox datecalc rrule -rule "FREQ=WEEKLY;INTERVAL=1;BYDAY=MO,WE;COUNT=10" -from 2026-10-05
//	lyntoolbox datecalc secs 90061
//	lyntoolbox datecalc secs -rev "1天2时3分"
package datecalc

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	Name  = "datecalc"
	Desc  = "日期差/加减/星期/重复日程(rrule)/秒数换算"
	Usage = `用法: lyntoolbox datecalc <子命令> [参数]

子命令:
  diff <日期1> <日期2>
      相差天数（日期2-日期1，带符号）与工作日数（闭区间含首尾，周六日不计）
  add <日期> <±N><y|m|d>
      日期加减；月份溢出规整到目标月最后一天（如 2024-01-31 +1m → 2024-02-29）
  weekday <日期>
      输出星期几（中文）
  rrule -rule "FREQ=...;INTERVAL=n;COUNT=n;UNTIL=日期;BYDAY=MO,WE;BYMONTHDAY=1,-1" [-from 日期]
      重复日程展开，输出日期列表（每行一个，YYYY-MM-DD）
      FREQ   DAILY|WEEKLY|MONTHLY|YEARLY（必填）
      COUNT/UNTIL 至少提供其一，限制生成条数/截止日期
      BYDAY 周几过滤（DAILY/WEEKLY 有效）；WEEKLY 无 BYDAY 时用起始日星期
      BYMONTHDAY 每月几号（MONTHLY 有效，负数表示倒数）
      MONTHLY/YEARLY 末日溢出规整；YEARLY 的 2/29 仅闰年生成
  secs <秒数> | secs -rev "1天2时3分"
      秒 ↔ 天/时/分/秒 互转（数字走正向，含单位词走反向）

日期格式: YYYY-MM-DD / YYYY/MM/DD / YYYYMMDD`
)

var weekdayNames = [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}

func parseDate(s string) (time.Time, error) {
	for _, layout := range []string{"2006-01-02", "2006/01/02", "20060102"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("无效日期 %q（应为 YYYY-MM-DD）", s)
}

func lastDay(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.Local).Day()
}

// addUnits 按单位加减并规整溢出。
func addUnits(t time.Time, n int, unit byte) time.Time {
	y, m, d := t.Date()
	switch unit {
	case 'y':
		ny := y + n
		return time.Date(ny, m, minInt(d, lastDay(ny, m)), 0, 0, 0, 0, time.Local)
	case 'm':
		total := int(m) - 1 + n
		ny := y + total/12
		nm := time.Month(total%12 + 1)
		return time.Date(ny, nm, minInt(d, lastDay(ny, nm)), 0, 0, 0, 0, time.Local)
	default: // 'd'
		return t.AddDate(0, 0, n)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// countWorkdays 闭区间 [a,b] 内的周一至周五天数。
func countWorkdays(a, b time.Time) int {
	if a.After(b) {
		a, b = b, a
	}
	n := 0
	for d := a; !d.After(b); d = d.AddDate(0, 0, 1) {
		if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
			n++
		}
	}
	return n
}

var bydayNames = map[string]time.Weekday{
	"SU": time.Sunday, "MO": time.Monday, "TU": time.Tuesday,
	"WE": time.Wednesday, "TH": time.Thursday, "FR": time.Friday, "SA": time.Saturday,
}

type rule struct {
	freq        string
	interval    int
	count       int
	until       time.Time
	hasUntil    bool
	byday       map[time.Weekday]bool
	bymonthday  []int
	hasByday    bool
	hasMonthday bool
}

func parseRule(s string) (*rule, error) {
	r := &rule{interval: 1, byday: map[time.Weekday]bool{}}
	freqSet := false
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("无效规则片段 %q（应为 KEY=VALUE）", part)
		}
		k := strings.ToUpper(strings.TrimSpace(kv[0]))
		v := strings.TrimSpace(kv[1])
		switch k {
		case "FREQ":
			f := strings.ToUpper(v)
			if f != "DAILY" && f != "WEEKLY" && f != "MONTHLY" && f != "YEARLY" {
				return nil, fmt.Errorf("FREQ 仅支持 DAILY/WEEKLY/MONTHLY/YEARLY，得到 %q", v)
			}
			r.freq, freqSet = f, true
		case "INTERVAL":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("INTERVAL 应为正整数，得到 %q", v)
			}
			r.interval = n
		case "COUNT":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("COUNT 应为正整数，得到 %q", v)
			}
			r.count = n
		case "UNTIL":
			t, err := parseDate(v)
			if err != nil {
				return nil, err
			}
			r.until, r.hasUntil = t, true
		case "BYDAY":
			for _, d := range strings.Split(strings.ToUpper(v), ",") {
				wd, ok := bydayNames[strings.TrimSpace(d)]
				if !ok {
					return nil, fmt.Errorf("BYDAY 值无效: %q（应为 MO/TU/WE/TH/FR/SA/SU）", d)
				}
				r.byday[wd] = true
			}
			r.hasByday = true
		case "BYMONTHDAY":
			for _, d := range strings.Split(v, ",") {
				n, err := strconv.Atoi(strings.TrimSpace(d))
				if err != nil || n == 0 || n > 31 || n < -31 {
					return nil, fmt.Errorf("BYMONTHDAY 值无效: %q（±1~31）", d)
				}
				r.bymonthday = append(r.bymonthday, n)
			}
			r.hasMonthday = true
		default:
			return nil, fmt.Errorf("不支持的规则键: %s", k)
		}
	}
	if !freqSet {
		return nil, fmt.Errorf("缺少 FREQ（DAILY/WEEKLY/MONTHLY/YEARLY）")
	}
	if r.count == 0 && !r.hasUntil {
		return nil, fmt.Errorf("必须提供 COUNT 或 UNTIL 之一，避免无限生成")
	}
	return r, nil
}

// expandRRule 展开重复规则。
func expandRRule(r *rule, from time.Time) []string {
	var out []string
	start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.Local)
	const maxScan = 400000 // 安全上限，约 1000 年
	for i := 0; i < maxScan && len(out) < 100000; i++ {
		cur := start.AddDate(0, 0, i)
		if r.hasUntil && cur.After(r.until) {
			break
		}
		if !matchesRule(r, start, cur) {
			continue
		}
		out = append(out, cur.Format("2006-01-02"))
		if r.count > 0 && len(out) >= r.count {
			break
		}
	}
	return out
}

// daysBetween 按日历日差计算天数：先把两端归一化为 UTC 零点，
// 避免跨夏令时日期的 23/25 小时天被 /24 截断成差一天。
func daysBetween(a, b time.Time) int {
	au := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	bu := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(bu.Sub(au).Hours() / 24)
}

func matchesRule(r *rule, start, cur time.Time) bool {
	dayDiff := daysBetween(start, cur)
	weekDiff := dayDiff / 7
	monthDiff := (cur.Year()-start.Year())*12 + int(cur.Month()) - int(start.Month())
	yearDiff := cur.Year() - start.Year()

	switch r.freq {
	case "DAILY":
		if dayDiff%r.interval != 0 {
			return false
		}
		return !r.hasByday || r.byday[cur.Weekday()]
	case "WEEKLY":
		if weekDiff%r.interval != 0 {
			return false
		}
		if r.hasByday {
			return r.byday[cur.Weekday()]
		}
		return cur.Weekday() == start.Weekday()
	case "MONTHLY":
		if monthDiff%r.interval != 0 {
			return false
		}
		if r.hasMonthday {
			days := lastDay(cur.Year(), cur.Month())
			for _, md := range r.bymonthday {
				d := md
				if d < 0 {
					d = days + 1 + d
				}
				if d >= 1 && d <= days && cur.Day() == d {
					return true
				}
			}
			return false
		}
		if r.hasByday {
			return r.byday[cur.Weekday()]
		}
		return cur.Day() == minInt(start.Day(), lastDay(cur.Year(), cur.Month()))
	case "YEARLY":
		if yearDiff%r.interval != 0 {
			return false
		}
		if r.hasMonthday {
			// BYMONTHDAY 仅对 MONTHLY 有效；YEARLY 下限定为起始月份中的该日，
			// 避免 "FREQ=YEARLY;BYMONTHDAY=1" 被误判为每月 1 号
			if cur.Month() != start.Month() {
				return false
			}
			for _, md := range r.bymonthday {
				if cur.Day() == md {
					return true
				}
			}
			return false
		}
		// 2/29 仅闰年命中（非闰年日期不存在，自动跳过）
		return cur.Month() == start.Month() && cur.Day() == start.Day()
	}
	return false
}

var revDurRe = regexp.MustCompile(`(?i)(\d+)\s*(天|d|时|小时|h|分|m|min|秒|s|sec)`)

// parseDurationCN 解析 "1天2时3分" 形式的时长为秒。
func parseDurationCN(s string) (int64, error) {
	mult := map[string]int64{
		"天": 86400, "d": 86400,
		"时": 3600, "小时": 3600, "h": 3600,
		"分": 60, "m": 60, "min": 60,
		"秒": 1, "s": 1, "sec": 1,
	}
	total := int64(0)
	found := false
	for _, m := range revDurRe.FindAllStringSubmatch(s, -1) {
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return 0, err
		}
		total += n * mult[strings.ToLower(m[2])]
		found = true
	}
	if !found {
		return 0, fmt.Errorf("无法解析时长 %q（示例: 1天2时3分 / 2h30m）", s)
	}
	return total, nil
}

// formatDurationCN 秒 → 天/时/分/秒，前导零单位省略。
func formatDurationCN(total int64) string {
	if total == 0 {
		return "0秒"
	}
	neg := ""
	if total < 0 {
		neg = "-"
		total = -total
	}
	parts := []int64{86400, 3600, 60, 1}
	names := []string{"天", "时", "分", "秒"}
	var sb strings.Builder
	sb.WriteString(neg)
	started := false
	for i, p := range parts {
		v := total / p
		total %= p
		if v > 0 || (started && i < len(parts)-1) {
			started = true
			fmt.Fprintf(&sb, "%d%s", v, names[i])
		}
	}
	return sb.String()
}

// reorderFlags 将 flag 参数挪到位置参数之前，使 "位置参数 -flag" 与 "-flag 位置参数" 两种顺序均可解析。
// valueFlags 为需要消费下一个参数的 flag 名集合。
func reorderFlags(args []string, valueFlags map[string]bool) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 1 && a[0] == '-' {
			if _, err := strconv.ParseFloat(a, 64); err == nil { // 负数视为位置参数
				pos = append(pos, a)
				continue
			}
			name := strings.TrimLeft(a, "-")
			if !strings.Contains(name, "=") && valueFlags[name] && i+1 < len(args) {
				flags = append(flags, a, args[i+1])
				i++
				continue
			}
			flags = append(flags, a)
			continue
		}
		pos = append(pos, a)
	}
	return append(flags, pos...)
} // Run 执行命令，返回退出码。
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, "缺少子命令\n"+Usage+"\n")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "diff":
		return runDiff(rest)
	case "add":
		return runAdd(rest)
	case "weekday":
		return runWeekday(rest)
	case "rrule":
		return runRRule(rest)
	case "secs":
		return runSecs(rest)
	case "-h", "-help", "help":
		fmt.Print(Usage + "\n")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n可用: diff add weekday rrule secs\n", sub)
		return 2
	}
}

func runDiff(args []string) int {
	fs := flag.NewFlagSet(Name+" diff", flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox datecalc diff <日期1> <日期2>")
		return 2
	}
	d1, err := parseDate(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	d2, err := parseDate(fs.Arg(1))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	days := daysBetween(d1, d2)
	fmt.Printf("相差天数: %d\n", days)
	fmt.Printf("工作日数: %d\n", countWorkdays(d1, d2))
	return 0
}

func runAdd(args []string) int {
	fs := flag.NewFlagSet(Name+" add", flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 2 && fs.NArg() != 3 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox datecalc add <日期> <±N><y|m|d>（或 <日期> <±N> <y|m|d>）")
		return 2
	}
	d, err := parseDate(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	spec := strings.ToLower(strings.TrimSpace(fs.Arg(1)))
	if fs.NArg() == 3 {
		spec += strings.ToLower(strings.TrimSpace(fs.Arg(2)))
	}
	if len(spec) < 2 {
		fmt.Fprintf(os.Stderr, "无效的加减表达式: %q（示例: 3d / -1m / +2y）\n", spec)
		return 2
	}
	unit := spec[len(spec)-1]
	if unit != 'y' && unit != 'm' && unit != 'd' {
		fmt.Fprintf(os.Stderr, "单位必须是 y/m/d，得到 %q\n", spec[len(spec)-1:])
		return 2
	}
	numStr := strings.TrimSuffix(strings.TrimSuffix(spec, string(unit)), "+")
	n, err := strconv.Atoi(numStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无效的加减数值: %q\n", spec[:len(spec)-1])
		return 2
	}
	fmt.Println(addUnits(d, n, unit).Format("2006-01-02"))
	return 0
}

func runWeekday(args []string) int {
	fs := flag.NewFlagSet(Name+" weekday", flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox datecalc weekday <日期>")
		return 2
	}
	d, err := parseDate(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Println(weekdayNames[d.Weekday()])
	return 0
}

func runRRule(args []string) int {
	args = reorderFlags(args, map[string]bool{"rule": true, "from": true})
	fs := flag.NewFlagSet(Name+" rrule", flag.ContinueOnError)
	ruleStr := fs.String("rule", "", "重复规则，如 FREQ=WEEKLY;BYDAY=MO,WE;COUNT=10")
	fromStr := fs.String("from", "", "起始日期（默认今天）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *ruleStr == "" {
		fmt.Fprintln(os.Stderr, "缺少 -rule 参数")
		return 2
	}
	r, err := parseRule(*ruleStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "规则错误: %v\n", err)
		return 2
	}
	from := time.Now()
	if *fromStr != "" {
		from, err = parseDate(*fromStr)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	}
	for _, d := range expandRRule(r, from) {
		fmt.Println(d)
	}
	return 0
}

func runSecs(args []string) int {
	args = reorderFlags(args, map[string]bool{"rev": true})
	fs := flag.NewFlagSet(Name+" secs", flag.ContinueOnError)
	rev := fs.Bool("rev", false, "反向：时长描述 → 秒")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, `用法: lyntoolbox datecalc secs <秒数> 或 secs -rev "1天2时3分"`)
		return 2
	}
	input := strings.TrimSpace(fs.Arg(0))
	if *rev {
		n, err := parseDurationCN(input)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println(n)
		return 0
	}
	if n, err := strconv.ParseInt(input, 10, 64); err == nil {
		fmt.Println(formatDurationCN(n))
		return 0
	}
	// 数字解析失败时自动按反向解析
	n, err := parseDurationCN(input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println(n)
	return 0
}
