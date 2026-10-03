// Package geodist 实现经纬度计算命令。
// 对应网页版：daily/geo-distance-tool.html（经纬度距离计算）
//
// 用法：
//
//	lyntoolbox geodist "39.9042,116.4074" "31.2304,121.4737"
//	lyntoolbox geodist 39.9042 116.4074 31.2304 121.4737 -mi
//	lyntoolbox geodist -json "39.9042,116.4074" "31.2304,121.4737"
package geodist

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

const (
	Name  = "geodist"
	Desc  = "经纬度大圆距离/初始方位角/中点计算（Haversine）"
	Usage = `用法: lyntoolbox geodist [-json] [-mi] [-r 半径] <纬度,经度> <纬度,经度>
      lyntoolbox geodist [-json] <纬度1 经度1 纬度2 经度2>

参数:
  -json  以 JSON 输出结果
  -mi    距离单位使用英里（默认千米 km）
  -r     自定义地球半径（单位随 -mi 而定；默认 6371 km / 3958.7613 mi）

说明:
  距离采用 Haversine 大圆公式；方位角为从点 1 指向点 2 的初始方位角（正北为 0，顺时针 0~360）；
  中点为大圆中点坐标。输入格式为 "纬度,经度"（十进制度）。`
)

const (
	earthRadiusKm = 6371.0
	earthRadiusMi = 3958.7613
)

func rad(d float64) float64 { return d * math.Pi / 180 }

func deg(r float64) float64 { return r * 180 / math.Pi }

type point struct {
	lat, lng float64
}

func parsePoint(s string) (point, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return point{}, fmt.Errorf("坐标 %q 应为 纬度,经度 两段", s)
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return point{}, fmt.Errorf("纬度 %q 不是数字", parts[0])
	}
	lng, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return point{}, fmt.Errorf("经度 %q 不是数字", parts[1])
	}
	if lat < -90 || lat > 90 {
		return point{}, fmt.Errorf("纬度 %g 超出 [-90, 90]", lat)
	}
	if lng < -180 || lng > 180 {
		return point{}, fmt.Errorf("经度 %g 超出 [-180, 180]", lng)
	}
	return point{lat, lng}, nil
}

// distanceKM Haversine 大圆距离。
func distanceKM(a, b point, radius float64) float64 {
	dLat := rad(b.lat - a.lat)
	dLng := rad(b.lng - a.lng)
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(a.lat))*math.Cos(rad(b.lat))*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * radius * math.Asin(math.Sqrt(h))
}

// bearing 初始方位角（度，0~360）。
func bearing(a, b point) float64 {
	phi1, phi2 := rad(a.lat), rad(b.lat)
	dLng := rad(b.lng - a.lng)
	y := math.Sin(dLng) * math.Cos(phi2)
	x := math.Cos(phi1)*math.Sin(phi2) - math.Sin(phi1)*math.Cos(phi2)*math.Cos(dLng)
	bd := deg(math.Atan2(y, x))
	return math.Mod(bd+360, 360)
}

// midpoint 大圆中点。
func midpoint(a, b point) point {
	phi1, phi2 := rad(a.lat), rad(b.lat)
	dLng := rad(b.lng - a.lng)
	bx := math.Cos(phi2) * math.Cos(dLng)
	by := math.Cos(phi2) * math.Sin(dLng)
	phim := math.Atan2(math.Sin(phi1)+math.Sin(phi2),
		math.Sqrt((math.Cos(phi1)+bx)*(math.Cos(phi1)+bx)+by*by))
	lngm := rad(a.lng) + math.Atan2(by, math.Cos(phi1)+bx)
	return point{deg(phim), math.Mod(deg(lngm)+540, 360) - 180}
}

type jsonResult struct {
	Distance float64 `json:"distance"`
	Unit     string  `json:"unit"`
	Bearing  float64 `json:"bearing_deg"`
	Midpoint struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
	} `json:"midpoint"`
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
	args = reorderFlags(args, map[string]bool{"r": true})
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	jsonOut := fs.Bool("json", false, "JSON 输出")
	mi := fs.Bool("mi", false, "使用英里")
	radius := fs.Float64("r", 0, "自定义球半径（0 表示默认）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	var p1, p2 point
	switch fs.NArg() {
	case 2:
		var err error
		if p1, err = parsePoint(fs.Arg(0)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		if p2, err = parsePoint(fs.Arg(1)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	case 4:
		var err error
		if p1, err = parsePoint(fs.Arg(0) + "," + fs.Arg(1)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		if p2, err = parsePoint(fs.Arg(2) + "," + fs.Arg(3)); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	default:
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox geodist \"纬度,经度\" \"纬度,经度\"")
		return 2
	}

	unit := "km"
	r := earthRadiusKm
	if *mi {
		unit, r = "mi", earthRadiusMi
	}
	if *radius > 0 {
		r = *radius
	}

	dist := distanceKM(p1, p2, r)
	brg := bearing(p1, p2)
	mid := midpoint(p1, p2)

	if *jsonOut {
		var res jsonResult
		res.Distance = math.Round(dist*1000) / 1000
		res.Unit = unit
		res.Bearing = math.Round(brg*100) / 100
		res.Midpoint.Lat = math.Round(mid.lat*1e6) / 1e6
		res.Midpoint.Lng = math.Round(mid.lng*1e6) / 1e6
		b, err := json.Marshal(res)
		if err != nil {
			fmt.Fprintln(os.Stderr, "JSON 编码失败:", err)
			return 1
		}
		fmt.Println(string(b))
		return 0
	}

	fmt.Printf("距离: %.3f %s\n", dist, unit)
	fmt.Printf("初始方位角: %.2f°\n", brg)
	fmt.Printf("中点: %.6f, %.6f\n", mid.lat, mid.lng)
	return 0
}
