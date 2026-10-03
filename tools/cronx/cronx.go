// Package cronx 实现 Cron 表达式解析与生成命令。
// 对应网页版：work/cron-tool.html（Cron解析）、work/cron-gen-tool.html（Cron生成）
//
// 用法：
//
//	lyntoolbox cronx parse [-next 次数] "分 时 日 月 周"
//	lyntoolbox cronx gen [-min 字段] [-hour 字段] [-dom 字段] [-month 字段] [-dow 字段]
package cronx

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	Name  = "cronx"
	Desc  = "Cron 表达式（5 字段）解析为中文描述并推算未来触发时间，支持反向生成"
	Usage = `用法: lyntoolbox cronx parse [-next 次数] "分 时 日 月 周"
      lyntoolbox cronx gen [-min 字段] [-hour 字段] [-dom 字段] [-month 字段] [-dow 字段]

说明:
  5 字段依次为 分钟(0-59) 小时(0-23) 日(1-31) 月(1-12) 星期(0-7，0 与 7 为周日)。
  支持 * , - / 与月份/星期英文缩写（jan-dec、sun-sat，大小写不限）。
  日与星期同时受限时按标准 Cron 语义取"或"。
参数:
  -next   parse 输出未来触发次数（0-100，默认 5，最多向前扫描一年）
  -min    gen: 分钟字段（默认 *）
  -hour   gen: 小时字段（默认 *）
  -dom    gen: 日字段（默认 *）
  -month  gen: 月字段（默认 *）
  -dow    gen: 星期字段（默认 *）
示例:
  lyntoolbox cronx parse "30 8 * * 1-5"
  lyntoolbox cronx parse -next 3 "*/15 9-18 * * mon-fri"
  lyntoolbox cronx gen -min 0 -hour 9 -dow 1-5`
)

// fieldSpec 字段取值范围与英文缩写映射。
type fieldSpec struct {
	min, max int
	names    map[string]int
	label    string
}

var (
	minuteSpec = fieldSpec{0, 59, nil, "分钟"}
	hourSpec   = fieldSpec{0, 23, nil, "小时"}
	domSpec    = fieldSpec{1, 31, nil, "日"}
	monthSpec  = fieldSpec{1, 12, map[string]int{
		"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
		"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
	}, "月"}
	dowSpec = fieldSpec{0, 7, map[string]int{
		"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
	}, "星期"}
)

// cronField 解析后的字段。
type cronField struct {
	set      map[int]bool
	wildcard bool // 覆盖全范围（原始字段为 * 或等价全范围）
}

// effMax 返回字段的有效上界（星期 7 与 0 同义，有效范围 0-6）。
func (spec fieldSpec) effMax() int {
	if spec.max == 7 {
		return 6
	}
	return spec.max
}

// parseField 解析单个 Cron 字段表达式。
func parseField(s string, spec fieldSpec) (*cronField, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return nil, fmt.Errorf("%s字段为空", spec.label)
	}
	f := &cronField{set: map[int]bool{}}
	add := func(v int) {
		if v == 7 && spec.max == 7 { // 星期 7 视为周日
			v = 0
		}
		f.set[v] = true
	}
	resolve := func(tok string) (int, error) {
		if v, err := strconv.Atoi(tok); err == nil {
			return v, nil
		}
		if v, ok := spec.names[tok]; ok {
			return v, nil
		}
		return 0, fmt.Errorf("%s字段含非法值 %q", spec.label, tok)
	}
	for _, part := range strings.Split(s, ",") {
		if part == "" {
			return nil, fmt.Errorf("%s字段存在空的列表项", spec.label)
		}
		rangePart, stepStr := part, "1"
		if i := strings.IndexByte(part, '/'); i >= 0 {
			rangePart, stepStr = part[:i], part[i+1:]
		}
		step, err := strconv.Atoi(stepStr)
		if err != nil || step < 1 {
			return nil, fmt.Errorf("%s字段步长非法: %q", spec.label, stepStr)
		}
		lo, hi := spec.min, spec.max
		switch {
		case rangePart == "*":
			// 通配与否由下方“全范围覆盖”归一化判定（*/n 不等于 *）
		case strings.Contains(rangePart, "-"):
			ab := strings.SplitN(rangePart, "-", 2)
			if lo, err = resolve(ab[0]); err != nil {
				return nil, err
			}
			if hi, err = resolve(ab[1]); err != nil {
				return nil, err
			}
			if lo > hi {
				return nil, fmt.Errorf("%s字段范围起止颠倒: %q", spec.label, rangePart)
			}
		default:
			if lo, err = resolve(rangePart); err != nil {
				return nil, err
			}
			hi = lo // 单值
			if strings.Contains(part, "/") {
				hi = spec.max // "a/step" 从 a 起按步长到上限
			}
		}
		if lo < spec.min || hi > spec.max || lo > hi {
			return nil, fmt.Errorf("%s字段取值超出 %d-%d: %q", spec.label, spec.min, spec.max, rangePart)
		}
		for v := lo; v <= hi; v += step {
			add(v)
		}
	}
	// 全范围时按通配处理（如 0-59 等价于 *）
	span := spec.effMax() - spec.min + 1
	if len(f.set) == span {
		full := true
		for v := spec.min; v <= spec.effMax(); v++ {
			if !f.set[v] {
				full = false
				break
			}
		}
		f.wildcard = f.wildcard || full
	}
	return f, nil
}

// describeSet 将字段值集合转为中文片段（单值 / 连续区间 / 顿号列举）。
func describeSet(f *cronField, spec fieldSpec, pad bool, unit string) string {
	vals := sortedKeys(f.set)
	fmtVal := func(v int) string {
		if pad {
			return fmt.Sprintf("%02d", v)
		}
		return strconv.Itoa(v)
	}
	if len(vals) == 1 {
		return fmtVal(vals[0]) + unit
	}
	contig := true
	for i := 1; i < len(vals); i++ {
		if vals[i] != vals[i-1]+1 {
			contig = false
			break
		}
	}
	if contig {
		return fmtVal(vals[0]) + "至" + fmtVal(vals[len(vals)-1]) + unit
	}
	parts := make([]string, 0, len(vals))
	for i, v := range vals {
		if i >= 6 {
			parts = append(parts, "…")
			break
		}
		parts = append(parts, fmtVal(v))
	}
	return strings.Join(parts, "、") + unit
}

func sortedKeys(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

var dowNames = map[int]string{0: "日", 1: "一", 2: "二", 3: "三", 4: "四", 5: "五", 6: "六"}

// joinSpace 以单空格连接非空片段。
func joinSpace(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}

// describe 生成整条表达式的中文描述。
func describe(fields [5]*cronField) string {
	minF, hourF, domF, monF, dowF := fields[0], fields[1], fields[2], fields[3], fields[4]

	// —— 日期部分 ——
	monPart := ""
	if !monF.wildcard {
		monPart = describeSet(monF, monthSpec, false, "月")
	}
	domPart := ""
	if !domF.wildcard {
		domPart = describeSet(domF, domSpec, false, "号")
	}
	dowPart := ""
	if !dowF.wildcard {
		dowPart = "周" + describeDow(dowF)
	}
	var dateDesc string
	switch {
	case domF.wildcard && dowF.wildcard:
		if monF.wildcard {
			dateDesc = "每天"
		} else {
			dateDesc = monPart + "每天"
		}
	case domF.wildcard:
		dateDesc = monPart + dowPart
	case dowF.wildcard:
		if monF.wildcard {
			dateDesc = "每月" + domPart
		} else {
			dateDesc = monPart + domPart
		}
	default:
		if monF.wildcard {
			dateDesc = "每月" + domPart + "或" + dowPart
		} else {
			dateDesc = monPart + domPart + "或" + dowPart
		}
	}

	// —— 时间部分 ——
	var timeDesc string
	switch {
	case hourF.wildcard && minF.wildcard:
		timeDesc = "每分钟"
	case hourF.wildcard && isStepSet(minF.set, 0, 59):
		timeDesc = "每小时每隔 " + strconv.Itoa(stepOf(minF.set, 0)) + " 分"
	case hourF.wildcard:
		timeDesc = "每小时第 " + describeSet(minF, minuteSpec, true, "") + " 分"
	case minF.wildcard:
		timeDesc = describeSet(hourF, hourSpec, true, "时") + "内每分钟"
	case isStepSet(minF.set, 0, 59) && minF.set[0]:
		timeDesc = describeSet(hourF, hourSpec, true, "时") + "，每隔 " + strconv.Itoa(stepOf(minF.set, 0)) + " 分"
	default:
		timeDesc = joinHHMM(hourF, minF)
	}

	if dateDesc == "每天" && strings.HasPrefix(timeDesc, "每") {
		return timeDesc // “每天 每分钟”类冗余，保留“每小时/每分钟”粒度即可
	}
	if dateDesc == "每天" {
		return joinSpace(dateDesc, timeDesc)
	}
	return strings.TrimSpace(dateDesc + " " + timeDesc)
}

// describeDow 星期集合描述：连续区间用“一至周五”，否则顿号列举。
func describeDow(f *cronField) string {
	vals := sortedKeys(f.set)
	if len(vals) == 1 {
		return dowNames[vals[0]]
	}
	contig := true
	for i := 1; i < len(vals); i++ {
		if vals[i] != vals[i-1]+1 {
			contig = false
			break
		}
	}
	if contig {
		return dowNames[vals[0]] + "至周" + dowNames[vals[len(vals)-1]]
	}
	parts := make([]string, 0, len(vals))
	for i, v := range vals {
		if i >= 6 {
			parts = append(parts, "…")
			break
		}
		parts = append(parts, dowNames[v]) // 调用方已加“周”前缀
	}
	return strings.Join(parts, "、")
}

// isStepSet 判断集合是否为从 from 开始覆盖到上界的等差数列（还原 */n 语义）。
func isStepSet(set map[int]bool, from, to int) bool {
	n := len(set)
	if n < 2 {
		return false
	}
	step := stepOf(set, from)
	if step < 1 {
		return false
	}
	count := 0
	for v := from; v <= to; v += step {
		if !set[v] {
			return false
		}
		count++
	}
	return count == n
}

// stepOf 返回集合以 from 为起点的步长（假定存在）。
func stepOf(set map[int]bool, from int) int {
	for v := from + 1; v <= toMax(set, from); v++ {
		if set[v] {
			return v - from
		}
	}
	return 0
}

func toMax(set map[int]bool, from int) int {
	m := from
	for k := range set {
		if k > m {
			m = k
		}
	}
	return m
}

// joinHHMM 小时×分钟组合的时刻描述（最多列 6 个）。
func joinHHMM(hourF, minF *cronField) string {
	hours, mins := sortedKeys(hourF.set), sortedKeys(minF.set)
	total := len(hours) * len(mins)
	parts := make([]string, 0, total)
	for _, h := range hours {
		for _, m := range mins {
			if len(parts) >= 6 {
				parts = append(parts, fmt.Sprintf("…共 %d 个时刻", total))
				return strings.Join(parts, "、")
			}
			parts = append(parts, fmt.Sprintf("%02d:%02d", h, m))
		}
	}
	return strings.Join(parts, "、")
}

// matches 判断给定时刻是否命中表达式。
func matches(fields [5]*cronField, t time.Time) bool {
	if !fields[0].set[t.Minute()] || !fields[1].set[t.Hour()] || !fields[3].set[int(t.Month())] {
		return false
	}
	domOK := fields[2].wildcard
	dowOK := fields[4].wildcard
	if !domOK {
		domOK = fields[2].set[t.Day()]
	}
	if !dowOK {
		dowOK = fields[4].set[int(t.Weekday())]
	}
	if !fields[2].wildcard && !fields[4].wildcard {
		return domOK || dowOK // 标准 Cron：日与星期同时受限取或
	}
	return domOK && dowOK
}

// nextTimes 从 now 之后推算未来 n 次触发时间（最多扫描一年）。
func nextTimes(fields [5]*cronField, n int) []time.Time {
	var out []time.Time
	t := time.Now().Truncate(time.Minute).Add(time.Minute)
	limit := t.AddDate(1, 0, 0)
	for len(out) < n && t.Before(limit) {
		if matches(fields, t) {
			out = append(out, t)
		}
		t = t.Add(time.Minute)
	}
	return out
}

// splitArgs 将本命令的已知旗标记号提前到参数列表最前端，
// 使旗标可以写在子命令之后（如 parse "..." -next 3）。
func splitArgs(args []string) ([]string, []string) {
	spec := map[string]bool{
		"next": true, "min": true, "hour": true, "dom": true, "month": true, "dow": true,
	}
	var flags, pos []string
	i := 0
	for ; i < len(args); i++ {
		t := args[i]
		if t == "--" {
			i++
			break
		}
		if strings.HasPrefix(t, "-") && strings.TrimLeft(t, "-") != "" {
			name := strings.TrimLeft(t, "-")
			hasValue := false
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				name, hasValue = name[:eq], true
			}
			if takes, known := spec[name]; known {
				flags = append(flags, t)
				if !hasValue && takes && i+1 < len(args) {
					i++
					flags = append(flags, args[i])
				}
				continue
			}
			if name == "h" || name == "help" {
				flags = append(flags, t)
				continue
			}
		}
		pos = append(pos, t)
	}
	pos = append(pos, args[i:]...)
	return flags, pos
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	nextN := fs.Int("next", 5, "未来触发次数")
	fMin := fs.String("min", "*", "分钟字段")
	fHour := fs.String("hour", "*", "小时字段")
	fDom := fs.String("dom", "*", "日字段")
	fMonth := fs.String("month", "*", "月字段")
	fDow := fs.String("dow", "*", "星期字段")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	flags, pos := splitArgs(args)
	if err := fs.Parse(flags); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	rest := pos
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "缺少子命令（parse|gen）")
		return 2
	}
	switch strings.ToLower(rest[0]) {
	case "parse":
		if len(rest) != 2 {
			fmt.Fprintln(os.Stderr, `parse 需要一个 5 字段表达式，如 "30 8 * * 1-5"`)
			return 2
		}
		return runParse(rest[1], *nextN)
	case "gen":
		return runGen(*fMin, *fHour, *fDom, *fMonth, *fDow)
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s（可选 parse|gen）\n", rest[0])
		return 2
	}
}

// parseCron 解析 5 字段表达式。
func parseCron(expr string) ([5]*cronField, error) {
	raws := strings.Fields(expr)
	if len(raws) != 5 {
		return [5]*cronField{}, fmt.Errorf("表达式必须为 5 个字段（分 时 日 月 周），实际 %d 个", len(raws))
	}
	var fields [5]*cronField
	specs := [5]fieldSpec{minuteSpec, hourSpec, domSpec, monthSpec, dowSpec}
	for i, raw := range raws {
		f, err := parseField(raw, specs[i])
		if err != nil {
			return fields, err
		}
		fields[i] = f
	}
	return fields, nil
}

// runParse 解析并输出描述与未来触发时间。
func runParse(expr string, n int) int {
	if n < 0 || n > 100 {
		fmt.Fprintln(os.Stderr, "-next 必须在 0-100 之间")
		return 2
	}
	fields, err := parseCron(expr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	fmt.Printf("表达式: %s\n", strings.Join(strings.Fields(expr), " "))
	fmt.Printf("描述:   %s\n", describe(fields))
	if n == 0 {
		return 0
	}
	times := nextTimes(fields, n)
	fmt.Printf("未来触发（共 %d 次）:\n", len(times))
	for _, t := range times {
		fmt.Println(t.Format("  2006-01-02 15:04"))
	}
	return 0
}

// runGen 由各字段生成表达式与描述。
func runGen(fMin, fHour, fDom, fMonth, fDow string) int {
	expr := strings.Join([]string{fMin, fHour, fDom, fMonth, fDow}, " ")
	fields, err := parseCron(expr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	fmt.Printf("表达式: %s\n", expr)
	fmt.Printf("描述:   %s\n", describe(fields))
	return 0
}
