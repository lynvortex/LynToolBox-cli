// Package srt 实现 SRT 字幕时间轴调整命令。
// 对应网页版：media/subtitle-shift-tool.html（字幕时间轴调整）
package srt

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

const (
	Name  = "srt"
	Desc  = "SRT 字幕时间轴调整（整体偏移/倍速缩放/重编号）"
	Usage = `用法: lyntoolbox srt 字幕.srt [-shift +1.5s|-00:00:02,000] [-scale 1.04] [-renumber] [-o 输出]

参数:
  -shift     整体偏移：秒数（+1.5 / -2）或完整时间（-00:00:02,500）
  -scale     时间倍速缩放（如 1.04；帧率修正常用 25/23.976）
  -renumber  序号重新从 1 编号
  -o         输出文件（默认打印到终端）`
)

// 小时允许 1~2 位：野生字幕常见 0:00:01,000 写法
var timeRe = regexp.MustCompile(`(\d{1,2}):(\d{2}):(\d{2})[,.](\d{3})`)

// parseTS 解析 SRT 时间为毫秒。
func parseTS(s string) (int64, error) {
	m := timeRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("无法识别时间: %s", s)
	}
	h, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	sec, _ := strconv.Atoi(m[3])
	ms, _ := strconv.Atoi(m[4])
	return int64(h)*3600000 + int64(mi)*60000 + int64(sec)*1000 + int64(ms), nil
}

// formatTS 毫秒转 SRT 时间格式。
func formatTS(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	h := ms / 3600000
	ms %= 3600000
	mi := ms / 60000
	ms %= 60000
	sec := ms / 1000
	ms %= 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, mi, sec, ms)
}

// parseShift 解析 -shift 参数为毫秒偏移。
func parseShift(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	sign := int64(1)
	if strings.HasPrefix(s, "+") {
		s = s[1:]
	} else if strings.HasPrefix(s, "-") {
		sign = -1
		s = s[1:]
	}
	// 含冒号按时间戳解析（HH:MM:SS,mmm 或 MM:SS,mmm）；否则按秒数（可带小数）
	if strings.Contains(s, ":") {
		ms, err := parseTS(s)
		if err != nil {
			ms, err = parseTS("00:" + s)
		}
		if err != nil {
			return 0, err
		}
		return sign * ms, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("无法识别偏移量: %s", s)
	}
	return sign * int64(f*1000), nil
}

type block struct {
	index string
	start int64
	end   int64
	lines []string
	raw   string
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	shift := fs.String("shift", "", "整体偏移")
	scale := fs.Float64("scale", 1, "时间缩放倍率")
	renumber := fs.Bool("renumber", false, "重新编号")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox srt 字幕.srt")
		return 2
	}

	offset, err := parseShift(*shift)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	f, err := os.Open(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "打开文件失败:", err)
		return 1
	}
	defer f.Close()

	// 解析块：序号行 + 时间行 + 文本行
	var blocks []block
	var cur *block
	var rawBuf []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	firstLine := true
	flush := func() {
		if cur != nil {
			cur.raw = strings.Join(rawBuf, "\n")
			blocks = append(blocks, *cur)
			cur = nil
			rawBuf = nil
		}
	}
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if firstLine {
			line = strings.TrimPrefix(line, "\ufeff")
			firstLine = false
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			continue
		}
		if cur == nil {
			if isIndexLine(trimmed) {
				cur = &block{index: trimmed}
				rawBuf = append(rawBuf, line)
				continue
			}
			// 无序号块：时间行直接开始
			if strings.Contains(trimmed, "-->") {
				cur = &block{index: ""}
				rawBuf = append(rawBuf, line)
				if s, e, ok := parseTimeLine(trimmed); ok {
					cur.start, cur.end = s, e
				}
				continue
			}
			// 孤立文本，原样输出
			blocks = append(blocks, block{raw: trimmed})
			continue
		}
		rawBuf = append(rawBuf, line)
		if strings.Contains(trimmed, "-->") && cur.start == 0 && cur.end == 0 {
			if s, e, ok := parseTimeLine(trimmed); ok {
				cur.start, cur.end = s, e
			}
			continue
		}
		cur.lines = append(cur.lines, line)
	}
	flush()

	// 变换并输出
	var b strings.Builder
	idx := 0
	warned := 0
	for _, blk := range blocks {
		if blk.start == 0 && blk.end == 0 && len(blk.lines) == 0 {
			b.WriteString(blk.raw + "\n\n")
			continue
		}
		if blk.start == 0 && blk.end == 0 {
			b.WriteString(blk.raw + "\n\n")
			warned++
			continue
		}
		idx++
		ns := int64(float64(blk.start)*(*scale)) + offset
		ne := int64(float64(blk.end)*(*scale)) + offset
		if ns < 0 {
			ns = 0
		}
		if ne < ns {
			ne = ns
		}
		num := blk.index
		if *renumber || num == "" {
			num = strconv.Itoa(idx)
		}
		b.WriteString(num + "\n")
		b.WriteString(formatTS(ns) + " --> " + formatTS(ne) + "\n")
		for _, l := range blk.lines {
			b.WriteString(l + "\n")
		}
		b.WriteString("\n")
	}

	result := strings.TrimRight(b.String(), "\n") + "\n"
	if warned > 0 {
		fmt.Fprintf(os.Stderr, "警告: %d 个块缺少时间轴，已原样保留\n", warned)
	}
	if *out != "" {
		if err := os.WriteFile(*out, []byte(result), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "写入失败:", err)
			return 1
		}
		fmt.Fprintf(os.Stderr, "已写入 %s（%d 个字幕块）\n", *out, idx)
		return 0
	}
	fmt.Print(result)
	fmt.Fprintf(os.Stderr, "\n（%d 个字幕块）\n", idx)
	return 0
}

func isIndexLine(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func parseTimeLine(s string) (int64, int64, bool) {
	parts := strings.SplitN(s, "-->", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	st, err1 := parseTS(parts[0])
	en, err2 := parseTS(timeRe.FindString(parts[1]))
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return st, en, true
}
