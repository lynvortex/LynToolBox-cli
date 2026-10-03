// Package unitconv 实现单位换算命令。
// 对应网页版：daily/unit-convert-tool.html（单位换算）
//
// 用法：
//
//	lyntoolbox unitconv 100 -from m -to km
//	lyntoolbox unitconv 98.6 -from f -to c
//	lyntoolbox unitconv -list
package unitconv

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	Name  = "unitconv"
	Desc  = "长度/质量/面积/体积/速度/数据/温度/时间单位换算"
	Usage = `用法: lyntoolbox unitconv <数值> -from 单位 -to 单位
      lyntoolbox unitconv -list

参数:
  -from  源单位（见 -list；大小写不敏感）
  -to    目标单位
  -list  列出全部类别与单位换算因子

说明:
  数据大小区分十进制（KB=1000B）与二进制（KiB=1024B）；
  温度按公式换算（°C/°F/K）；1 gal/qt/cup/floz 为美制。`
)

type unit struct {
	name   string
	factor float64 // 相对类别基准单位的因子
	note   string
}

type category struct {
	name  string
	base  string
	units []unit
	// temp 为 true 时使用温度公式
	temp bool
}

var categories = []category{
	{name: "长度", base: "m", units: []unit{
		{"mm", 0.001, "毫米"}, {"cm", 0.01, "厘米"}, {"m", 1, "米"},
		{"km", 1000, "千米"}, {"in", 0.0254, "英寸"}, {"ft", 0.3048, "英尺"},
		{"yd", 0.9144, "码"}, {"mi", 1609.344, "英里"}, {"nmi", 1852, "海里"},
	}},
	{name: "质量", base: "kg", units: []unit{
		{"mg", 1e-6, "毫克"}, {"g", 0.001, "克"}, {"kg", 1, "千克"},
		{"t", 1000, "吨"}, {"oz", 0.028349523125, "盎司"}, {"lb", 0.45359237, "磅"},
	}},
	{name: "面积", base: "m²", units: []unit{
		{"mm2", 1e-6, "平方毫米"}, {"cm2", 1e-4, "平方厘米"}, {"m2", 1, "平方米"},
		{"ha", 1e4, "公顷"}, {"km2", 1e6, "平方千米"}, {"mu", 2000.0 / 3, "亩"},
		{"ft2", 0.09290304, "平方英尺"},
	}},
	{name: "体积", base: "l", units: []unit{
		{"ml", 0.001, "毫升"}, {"l", 1, "升"}, {"m3", 1000, "立方米"},
		{"gal", 3.785411784, "美制加仑"}, {"qt", 0.946352946, "美制夸脱"},
		{"cup", 0.2365882365, "美制杯"}, {"floz", 0.0295735295625, "美制液量盎司"},
	}},
	{name: "速度", base: "m/s", units: []unit{
		{"m/s", 1, "米每秒"}, {"km/h", 1.0 / 3.6, "千米每小时"},
		{"mph", 0.44704, "英里每小时"}, {"kn", 1852.0 / 3600, "节"},
	}},
	{name: "数据", base: "B", units: []unit{
		{"B", 1, "字节"},
		{"KB", 1e3, "千字节(十进制)"}, {"MB", 1e6, "兆字节(十进制)"},
		{"GB", 1e9, "吉字节(十进制)"}, {"TB", 1e12, "太字节(十进制)"},
		{"KiB", 1024, "千字节(二进制)"}, {"MiB", 1 << 20, "兆字节(二进制)"},
		{"GiB", 1 << 30, "吉字节(二进制)"}, {"TiB", 1 << 40, "太字节(二进制)"},
	}},
	{name: "温度", base: "°C", temp: true, units: []unit{
		{"c", 1, "摄氏度"}, {"f", 1, "华氏度"}, {"k", 1, "开尔文"},
	}},
	{name: "时间", base: "s", units: []unit{
		{"ms", 0.001, "毫秒"}, {"s", 1, "秒"}, {"min", 60, "分钟"},
		{"h", 3600, "小时"}, {"day", 86400, "天"}, {"week", 604800, "周"},
	}},
}

var tempAliases = map[string]string{
	"c": "c", "°c": "c", "℃": "c", "celsius": "c",
	"f": "f", "°f": "f", "℉": "f", "fahrenheit": "f",
	"k": "k", "kelvin": "k",
}

// findUnit 在全部类别中查找单位，返回类别与因子。
func findUnit(name string) (*category, *unit, bool) {
	for ci := range categories {
		cat := &categories[ci]
		for i := range cat.units {
			u := &cat.units[i]
			if u.name == name {
				return cat, u, true
			}
		}
	}
	// 大小写不敏感回退（数据类别 KB/kB 等先按精确匹配处理）
	for ci := range categories {
		cat := &categories[ci]
		for i := range cat.units {
			u := &cat.units[i]
			if strings.EqualFold(u.name, name) {
				return cat, u, true
			}
		}
	}
	return nil, nil, false
}

// tempConvert 温度互转（内部先转摄氏度）。
func tempConvert(v float64, from, to string) float64 {
	toC := func() float64 {
		switch from {
		case "f":
			return (v - 32) * 5 / 9
		case "k":
			return v - 273.15
		}
		return v
	}
	c := toC()
	switch to {
	case "f":
		return c*9/5 + 32
	case "k":
		return c + 273.15
	}
	return c
}

func formatNum(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// reorderFlags 将 flag 参数挪到位置参数之前，使 "数值 -from 单位" 与 "-from 单位 数值" 两种顺序均可解析。
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
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	args = reorderFlags(args, map[string]bool{"from": true, "to": true})
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	from := fs.String("from", "", "源单位")
	to := fs.String("to", "", "目标单位")
	list := fs.Bool("list", false, "列出全部单位")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *list {
		for _, cat := range categories {
			fmt.Printf("[%s]（基准 %s）\n", cat.name, cat.base)
			for _, u := range cat.units {
				if cat.temp {
					fmt.Printf("  %-8s %s\n", u.name, u.note)
				} else {
					fmt.Printf("  %-8s 1 %s = %s %s（%s）\n", u.name, cat.base, formatNum(u.factor), cat.base, u.note)
				}
			}
		}
		return 0
	}

	if fs.NArg() != 1 || *from == "" || *to == "" {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox unitconv <数值> -from 单位 -to 单位")
		return 2
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(fs.Arg(0)), 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无效数值: %s\n", fs.Arg(0))
		return 2
	}

	fromName := strings.TrimSpace(*from)
	toName := strings.TrimSpace(*to)

	// 温度单位别名归一
	if n, ok := tempAliases[strings.ToLower(fromName)]; ok && fromName != "" {
		fromName = n
	}
	if n, ok := tempAliases[strings.ToLower(toName)]; ok {
		toName = n
	}

	fc, fu, ok1 := findUnit(fromName)
	tc, tu, ok2 := findUnit(toName)
	if !ok1 {
		fmt.Fprintf(os.Stderr, "未知源单位: %s（用 -list 查看）\n", *from)
		return 2
	}
	if !ok2 {
		fmt.Fprintf(os.Stderr, "未知目标单位: %s（用 -list 查看）\n", *to)
		return 2
	}
	if fc.name != tc.name {
		fmt.Fprintf(os.Stderr, "类别不匹配: %s 属于[%s]，%s 属于[%s]\n", fu.name, fc.name, tu.name, tc.name)
		return 1
	}

	var result float64
	if fc.temp {
		result = tempConvert(value, fu.name, tu.name)
	} else {
		result = value * fu.factor / tu.factor
	}

	fmt.Printf("%s %s = %s %s\n", formatNum(value), fu.name, formatNum(result), tu.name)
	return 0
}
