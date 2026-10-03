// Package stats 实现统计计算器命令。
// 对应网页版：daily/stats-tool.html（统计计算器）
//
// 用法：
//
//	lyntoolbox stats 1 2 3 4 5
//	lyntoolbox stats -f data.txt
//	cat data.txt | lyntoolbox stats
//	lyntoolbox stats -xy -f pairs.txt
package stats

import (
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

const (
	Name  = "stats"
	Desc  = "统计计算器：均值/中位数/众数/方差/标准差/四分位数，支持两列相关与回归"
	Usage = `用法: lyntoolbox stats [-xy] [-f 文件] [数字...]

说明:
  数字可由任意非数字字符分隔（空格、逗号、换行、分号等），支持科学计数法。
  -xy 时输入为两列数据（x y 交替出现），输出皮尔逊相关系数与线性回归 y=ax+b。
  四分位数采用线性插值法（Q1 位置 = 0.25*(n-1)）。
参数:
  -xy    按两列 x y 数据处理，输出相关系数 r、r² 与回归方程
  -f     从文件读取；省略数字与 -f 时从 stdin 读取
示例:
  lyntoolbox stats 1 2 3 4 5
  lyntoolbox stats -xy "1 2, 2 4, 3 5.1, 4 7.9"`
)

// splitArgs 将本命令的已知旗标记号提前到参数列表最前端，
// 使旗标可以写在任意位置，同时保证以 - 开头的数值
// （如 -3）不会被 flag 包误认为旗标。
func splitArgs(args []string) ([]string, []string) {
	spec := map[string]bool{"xy": false, "f": true}
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
	xy := fs.Bool("xy", false, "两列 x y 数据模式")
	file := fs.String("f", "", "输入文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	flags, pos := splitArgs(args)
	if err := fs.Parse(flags); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	var raw string
	if *file != "" {
		b, err := os.ReadFile(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取文件失败:", err)
			return 1
		}
		raw = string(b)
	} else if len(pos) > 0 {
		raw = strings.Join(pos, " ")
	} else {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "读取 stdin 失败:", err)
			return 1
		}
		raw = string(b)
	}
	nums, err := parseNumbers(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	if len(nums) == 0 {
		fmt.Fprintln(os.Stderr, "未提供数据")
		return 1
	}
	if *xy {
		return runXY(nums)
	}
	return runStats(nums)
}

// parseNumbers 从文本中提取全部数值（任意非数字字符均可作分隔符）。
func parseNumbers(raw string) ([]float64, error) {
	toks := strings.FieldsFunc(raw, func(r rune) bool {
		return !strings.ContainsRune("0123456789+-.eE", r)
	})
	nums := make([]float64, 0, len(toks))
	for _, t := range toks {
		v, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return nil, fmt.Errorf("无法解析数值 %q", t)
		}
		nums = append(nums, v)
	}
	return nums, nil
}

// fnum 以 12 位有效数字输出数值（消除浮点尾差噪声）；极大/极小值回退到完整精度。
func fnum(v float64) string {
	s := strconv.FormatFloat(v, 'g', 12, 64)
	if strings.ContainsAny(s, "eE") {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return s
}

// runStats 输出单列统计结果。
func runStats(nums []float64) int {
	n := len(nums)
	sorted := append([]float64(nil), nums...)
	sort.Float64s(sorted)

	var sum float64
	for _, v := range nums {
		sum += v
	}
	mean := sum / float64(n)

	var m2 float64 // 平方和偏差
	for _, v := range nums {
		d := v - mean
		m2 += d * d
	}
	varSamp, varPop := math.NaN(), m2/float64(n)
	if n >= 2 {
		varSamp = m2 / float64(n-1)
	}

	fmt.Printf("n: %d\n", n)
	fmt.Printf("总和: %s\n", fnum(sum))
	fmt.Printf("均值: %s\n", fnum(mean))
	fmt.Printf("中位数: %s\n", fnum(quantile(sorted, 0.5)))
	fmt.Printf("众数: %s\n", modes(nums))
	fmt.Printf("样本方差: %s\n", na(varSamp))
	fmt.Printf("总体方差: %s\n", fnum(varPop))
	fmt.Printf("样本标准差: %s\n", na(math.Sqrt(varSamp)))
	fmt.Printf("总体标准差: %s\n", fnum(math.Sqrt(varPop)))
	fmt.Printf("最小: %s\n", fnum(sorted[0]))
	fmt.Printf("最大: %s\n", fnum(sorted[n-1]))
	fmt.Printf("Q1: %s\n", fnum(quantile(sorted, 0.25)))
	fmt.Printf("Q3: %s\n", fnum(quantile(sorted, 0.75)))
	fmt.Printf("极差: %s\n", fnum(sorted[n-1]-sorted[0]))
	return 0
}

// runXY 输出两列数据的皮尔逊相关系数与线性回归。
func runXY(nums []float64) int {
	if len(nums)%2 != 0 {
		fmt.Fprintln(os.Stderr, "-xy 模式数据必须成对（x y 交替），实际数值个数 ", len(nums))
		return 1
	}
	n := len(nums) / 2
	if n < 2 {
		fmt.Fprintln(os.Stderr, "-xy 模式至少需要 2 对数据")
		return 1
	}
	var sx, sy float64
	for i := 0; i < n; i++ {
		sx += nums[2*i]
		sy += nums[2*i+1]
	}
	mx, my := sx/float64(n), sy/float64(n)
	var sxx, syy, sxy float64
	for i := 0; i < n; i++ {
		dx, dy := nums[2*i]-mx, nums[2*i+1]-my
		sxx += dx * dx
		syy += dy * dy
		sxy += dx * dy
	}
	fmt.Printf("n: %d\n", n)
	if sxx == 0 || syy == 0 {
		fmt.Println("相关系数 r: N/A（x 或 y 无变化）")
		fmt.Println("r²: N/A")
		fmt.Println("回归方程: N/A（x 或 y 无变化）")
		return 0
	}
	r := sxy / math.Sqrt(sxx*syy)
	a := sxy / sxx
	b := my - a*mx
	fmt.Printf("相关系数 r: %s\n", fnum(r))
	fmt.Printf("r²: %s\n", fnum(r*r))
	fmt.Printf("回归方程: y = %sx + %s\n", fnum(a), fnum(b))
	fmt.Printf("斜率 a: %s\n", fnum(a))
	fmt.Printf("截距 b: %s\n", fnum(b))
	return 0
}

// na 数值无效时输出 N/A。
func na(v float64) string {
	if math.IsNaN(v) {
		return "N/A"
	}
	return fnum(v)
}

// quantile 线性插值法分位数（p 取 0.25/0.5/0.75）。
func quantile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 1 {
		return sorted[0]
	}
	pos := p * float64(n-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	frac := pos - float64(lo)
	return sorted[lo] + frac*(sorted[hi]-sorted[lo])
}

// modes 众数：出现次数最多的值（并列全部列出；全部只出现一次则无众数）。
func modes(nums []float64) string {
	freq := map[float64]int{}
	maxCnt := 0
	for _, v := range nums {
		freq[v]++
		if freq[v] > maxCnt {
			maxCnt = freq[v]
		}
	}
	if maxCnt <= 1 {
		return "无"
	}
	var vals []float64
	for v, c := range freq {
		if c == maxCnt {
			vals = append(vals, v)
		}
	}
	sort.Float64s(vals)
	parts := make([]string, 0, len(vals))
	for _, v := range vals {
		parts = append(parts, fnum(v))
	}
	return strings.Join(parts, "、") + fmt.Sprintf("（各出现 %d 次）", maxCnt)
}
