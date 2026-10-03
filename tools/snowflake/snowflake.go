// Package snowflake 实现雪花 ID 生成与解析命令。
// 对应网页版：work/snowflake-tool.html（雪花ID生成器）
//
// 用法：
//
//	lyntoolbox snowflake generate [-n 数量] [-epoch 毫秒] [-worker 值] [-dc 值]
//	lyntoolbox snowflake parse [-epoch 毫秒] [-utc] <ID>...
package snowflake

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	Name  = "snowflake"
	Desc  = "雪花 ID（Snowflake）生成与解析：41 位时间戳 + 10 位机器 + 12 位序列"
	Usage = `用法: lyntoolbox snowflake generate [-n 数量] [-epoch 毫秒] [-worker 值] [-dc 值]
      lyntoolbox snowflake parse [-epoch 毫秒] [-utc] <ID>...

说明:
  标准 Twitter 布局：41 位毫秒时间戳 + 10 位机器号（数据中心 5 位 + 机器 5 位）+ 12 位序列。
  -epoch 默认 1288834974657（Twitter 纪元）。
参数:
  -n       生成数量（1-100000，默认 1）
  -epoch   自定义纪元（毫秒时间戳）
  -worker  机器 ID（0-31，默认 0）
  -dc      数据中心 ID（0-31，默认 0）
  -utc     parse 输出日期使用 UTC（默认本地时区）
示例:
  lyntoolbox snowflake generate -n 3
  lyntoolbox snowflake parse 7266510327367225345`
)

const (
	epochBits      = 41
	dcBits         = 5
	workerBits     = 5
	seqBits        = 12
	maxSequence    = 1<<seqBits - 1 // 4095
	defaultEpoch   = int64(1288834974657)
	timeShift      = dcBits + workerBits + seqBits // 22
	dcShift        = workerBits + seqBits          // 17
	workerShift    = seqBits                       // 12
	maxElapsedTime = int64(1)<<epochBits - 1
)

// splitArgs 将本命令的已知旗标记号提前到参数列表最前端，
// 使旗标可以写在子命令之后（如 generate -n 3）。
func splitArgs(args []string) ([]string, []string) {
	spec := map[string]bool{"n": true, "epoch": true, "worker": true, "dc": true, "utc": false}
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
	n := fs.Int("n", 1, "生成数量")
	epoch := fs.Int64("epoch", defaultEpoch, "纪元毫秒时间戳")
	worker := fs.Int64("worker", 0, "机器 ID（0-31）")
	dc := fs.Int64("dc", 0, "数据中心 ID（0-31）")
	utc := fs.Bool("utc", false, "parse 日期用 UTC 输出")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	flags, pos := splitArgs(args)
	if err := fs.Parse(flags); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *epoch < 0 {
		fmt.Fprintln(os.Stderr, "-epoch 不能为负")
		return 2
	}
	if *worker < 0 || *worker > 1<<workerBits-1 {
		fmt.Fprintln(os.Stderr, "-worker 必须在 0-31 之间")
		return 2
	}
	if *dc < 0 || *dc > 1<<dcBits-1 {
		fmt.Fprintln(os.Stderr, "-dc 必须在 0-31 之间")
		return 2
	}

	rest := pos
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "缺少子命令（generate|parse）")
		return 2
	}
	switch strings.ToLower(rest[0]) {
	case "generate":
		return runGenerate(*n, *epoch, *worker, *dc)
	case "parse":
		if len(rest) < 2 {
			fmt.Fprintln(os.Stderr, "parse 需要至少一个 ID")
			return 2
		}
		return runParse(rest[1:], *epoch, *utc)
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s（可选 generate|parse）\n", rest[0])
		return 2
	}
}

// runGenerate 生成 n 个雪花 ID，每行一个。
func runGenerate(n int, epoch, worker, dc int64) int {
	if n < 1 || n > 100000 {
		fmt.Fprintln(os.Stderr, "-n 必须在 1-100000 之间")
		return 2
	}
	var last int64 = -1
	var seq int64
	for i := 0; i < n; i++ {
		now := time.Now().UnixMilli()
		if now == last {
			seq++
			if seq > maxSequence {
				// 当前毫秒序列用尽，自旋等待下一毫秒
				for time.Now().UnixMilli() <= last {
					time.Sleep(50 * time.Microsecond)
				}
				last = time.Now().UnixMilli()
				seq = 0
			}
		} else {
			last = now
			seq = 0
		}
		delta := last - epoch
		if delta < 0 {
			fmt.Fprintln(os.Stderr, "系统时间早于纪元，无法生成")
			return 1
		}
		if delta > maxElapsedTime {
			fmt.Fprintln(os.Stderr, "超出 41 位时间戳上限")
			return 1
		}
		id := delta<<timeShift | dc<<dcShift | worker<<workerShift | seq
		fmt.Println(id)
	}
	return 0
}

// runParse 解析一个或多个雪花 ID。
func runParse(ids []string, epoch int64, utc bool) int {
	for k, raw := range ids {
		id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || id < 0 {
			fmt.Fprintf(os.Stderr, "非法 ID: %s\n", raw)
			return 1
		}
		delta := id >> timeShift
		ts := delta + epoch
		dcID := id >> dcShift & (1<<dcBits - 1)
		workerID := id >> workerShift & (1<<workerBits - 1)
		seq := id & maxSequence
		t := time.UnixMilli(ts)
		if utc {
			t = t.UTC()
		}
		if k > 0 {
			fmt.Println()
		}
		fmt.Printf("ID:       %d\n", id)
		fmt.Printf("时间戳:   %d（自纪元 %d，毫秒）\n", ts, epoch)
		fmt.Printf("日期:     %s\n", t.Format("2006-01-02 15:04:05.000 -07:00"))
		fmt.Printf("数据中心: %d\n", dcID)
		fmt.Printf("机器:     %d\n", workerID)
		fmt.Printf("序列:     %d\n", seq)
	}
	return 0
}
