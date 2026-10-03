// Package wavtool 实现 WAV 音频处理命令（仅 PCM WAV）。
// 对应网页版：media/audio-cut-tool.html（音频剪辑）、media/audio-merge-tool.html（音频合并）、
// media/audio-normalize-tool.html（音量归一化）、media/audio-pan-tool.html（声道处理）、
// media/audio-speed-tool.html（变速）——WAV 简化版
//
// 用法：
//
//	lyntoolbox wavtool info in.wav
//	lyntoolbox wavtool cut in.wav -from 1 -to 3.5 -o out.wav
//	lyntoolbox wavtool concat a.wav b.wav -o out.wav
//	lyntoolbox wavtool gain in.wav 6 -o out.wav
//	lyntoolbox wavtool normalize in.wav -peak -3 -o out.wav
//	lyntoolbox wavtool pan in.wav -balance 0.5 -o out.wav
//	lyntoolbox wavtool speed in.wav -factor 2 -o out.wav
//	lyntoolbox wavtool fade in.wav -in 2 -out 3 -o out.wav
package wavtool

import (
	"encoding/binary"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
)

const (
	Name  = "wavtool"
	Desc  = "WAV 音频处理：信息/剪辑/拼接/增益/归一化/声道/变速/淡入淡出（仅 PCM）"
	Usage = `用法: lyntoolbox wavtool <子命令> [参数]

子命令（除 info 外均需 -o 输出文件）:
  info <文件>
      打印时长/声道/采样率/位深/数据大小/峰值
  cut <文件> [-from 秒] [-to 秒] -o 输出
      剪辑指定区间；-to 省略表示到末尾
  concat <文件1> <文件2>... -o 输出
      拼接多个 WAV（声道/采样率/位深必须一致）
  gain <文件> <±dB> -o 输出
      增益；超界采样削波保护并提示削波数量
  normalize <文件> [-peak -3] -o 输出
      峰值归一化到指定 dBFS（默认 -1）
  pan <文件> [-balance -1~1] 或 [-mono l|r|avg] -o 输出
      立体声平衡调节；或提取左/右声道、多声道平均为单声道
  speed <文件> -factor 倍率 -o 输出
      线性插值重采样变速：时长与音高同步改变（未做变调补偿）
  fade <文件> [-in 秒] [-out 秒] -o 输出
      线性淡入/淡出

支持格式: PCM WAV（格式码 1），位深 8/16/24/32bit；32bit 亦接受 IEEE float（格式码 3）。
多声道按交错采样解析，输出保持输入的位深/采样率/声道数。`
)

// wavAudio 解析后的 WAV 音频，采样值归一到 [-1, 1]。
type wavAudio struct {
	formatCode int // 1=PCM, 3=IEEE float
	channels   int
	sampleRate int
	bits       int
	frames     int
	chans      [][]float64 // 按声道拆分，每路长度为 frames
}

// ---------- 解析 ----------

func parseWAV(data []byte) (*wavAudio, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, fmt.Errorf("不是有效的 RIFF/WAVE 文件")
	}
	var (
		formatCode, channels, bits int
		sampleRate                 int
		raw                        []byte
		hasFmt, hasData            bool
	)
	// 块位置全程用 int64：chunk size 是 uint32，32 位平台上转 int 会回绕成负数，
	// 导致负索引切片 panic 或循环回退。
	i := int64(12)
	for i+8 <= int64(len(data)) {
		id := string(data[i : i+4])
		size := int64(binary.LittleEndian.Uint32(data[i+4 : i+8]))
		bodyStart, bodyEnd := i+8, i+8+size
		if bodyEnd > int64(len(data)) {
			bodyEnd = int64(len(data))
		}
		if bodyEnd < bodyStart {
			return nil, fmt.Errorf("块 %q 声明大小非法: %d", id, size)
		}
		body := data[bodyStart:bodyEnd]
		switch id {
		case "fmt ":
			if len(body) < 16 {
				return nil, fmt.Errorf("fmt 块过短")
			}
			formatCode = int(binary.LittleEndian.Uint16(body[0:2]))
			channels = int(binary.LittleEndian.Uint16(body[2:4]))
			sampleRate = int(binary.LittleEndian.Uint32(body[4:8]))
			bits = int(binary.LittleEndian.Uint16(body[14:16]))
			if formatCode == 0xFFFE && len(body) >= 26 { // WAVE_FORMAT_EXTENSIBLE
				formatCode = int(binary.LittleEndian.Uint16(body[24:26]))
			}
			hasFmt = true
		case "data":
			raw = body
			hasData = true
		}
		i = bodyStart + size
		if size%2 == 1 {
			i++ // 块按 2 字节对齐
		}
	}
	if !hasFmt {
		return nil, fmt.Errorf("缺少 fmt 块")
	}
	if !hasData {
		return nil, fmt.Errorf("缺少 data 块")
	}
	if channels < 1 || channels > 64 {
		return nil, fmt.Errorf("不支持的声道数: %d", channels)
	}
	if sampleRate <= 0 {
		return nil, fmt.Errorf("无效采样率: %d", sampleRate)
	}
	switch formatCode {
	case 1:
		if bits != 8 && bits != 16 && bits != 24 && bits != 32 {
			return nil, fmt.Errorf("不支持的 PCM 位深: %d（仅 8/16/24/32）", bits)
		}
	case 3:
		if bits != 32 {
			return nil, fmt.Errorf("float 格式仅支持 32bit（当前 %d）", bits)
		}
	default:
		return nil, fmt.Errorf("仅支持 PCM(1)/float(3) WAV，当前格式码 %d", formatCode)
	}

	blockAlign := channels * bits / 8
	// WAV 头的 byteRate 字段是 uint32，提前拦截会溢出的组合
	if int64(sampleRate)*int64(blockAlign) > 0xFFFFFFFF {
		return nil, fmt.Errorf("采样率 %d × 块对齐 %d 超出 WAV 头字段上限", sampleRate, blockAlign)
	}
	if len(raw)%blockAlign != 0 {
		trimmed := len(raw) - len(raw)%blockAlign
		fmt.Fprintf(os.Stderr, "警告: data 块大小非块对齐，截断 %d 字节\n", len(raw)-trimmed)
		raw = raw[:trimmed]
	}
	frames := len(raw) / blockAlign

	a := &wavAudio{
		formatCode: formatCode, channels: channels, sampleRate: sampleRate,
		bits: bits, frames: frames,
		chans: make([][]float64, channels),
	}
	for c := range a.chans {
		a.chans[c] = make([]float64, frames)
	}
	for f := 0; f < frames; f++ {
		base := f * blockAlign
		for c := 0; c < channels; c++ {
			off := base + c*bits/8
			a.chans[c][f] = decodeSample(raw[off:], bits, formatCode)
		}
	}
	return a, nil
}

func decodeSample(b []byte, bits, formatCode int) float64 {
	switch bits {
	case 8:
		return (float64(b[0]) - 128) / 128
	case 16:
		return float64(int16(binary.LittleEndian.Uint16(b))) / 32768
	case 24:
		v := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
		if v&0x800000 != 0 {
			v |= ^int32(0xFFFFFF) // 符号扩展
		}
		return float64(v) / 8388608
	default: // 32
		if formatCode == 3 {
			return float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
		}
		return float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648
	}
}

// ---------- 写出 ----------

func (a *wavAudio) encodeSample(v float64) []byte {
	if v > 1 {
		v = 1
	} else if v < -1 {
		v = -1
	}
	buf := make([]byte, a.bits/8)
	switch a.bits {
	case 8:
		idx := int(math.Round(v*128)) + 128
		if idx > 255 {
			idx = 255
		}
		buf[0] = byte(idx)
	case 16:
		binary.LittleEndian.PutUint16(buf, uint16(int16(math.Round(v*32767))))
	case 24:
		n := int32(math.Round(v * 8388607))
		buf[0] = byte(n)
		buf[1] = byte(n >> 8)
		buf[2] = byte(n >> 16)
	default:
		if a.formatCode == 3 {
			binary.LittleEndian.PutUint32(buf, math.Float32bits(float32(v)))
		} else {
			binary.LittleEndian.PutUint32(buf, uint32(int32(math.Round(v*2147483647))))
		}
	}
	return buf
}

func (a *wavAudio) writeWAV(path string) error {
	blockAlign := a.channels * a.bits / 8
	dataSize := a.frames * blockAlign
	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+dataSize))
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], uint16(a.formatCode))
	binary.LittleEndian.PutUint16(header[22:24], uint16(a.channels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(a.sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(a.sampleRate*blockAlign))
	binary.LittleEndian.PutUint16(header[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(header[34:36], uint16(a.bits))
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataSize))

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(header); err != nil {
		return err
	}
	buf := make([]byte, 0, dataSize)
	for fr := 0; fr < a.frames; fr++ {
		for c := 0; c < a.channels; c++ {
			buf = append(buf, a.encodeSample(a.chans[c][fr])...)
		}
	}
	_, err = f.Write(buf)
	return err
}

func (a *wavAudio) durationSec() float64 {
	if a.sampleRate == 0 {
		return 0
	}
	return float64(a.frames) / float64(a.sampleRate)
}

func (a *wavAudio) peak() float64 {
	p := 0.0
	for _, ch := range a.chans {
		for _, v := range ch {
			if a := math.Abs(v); a > p {
				p = a
			}
		}
	}
	return p
}

func (a *wavAudio) formatName() string {
	if a.formatCode == 3 {
		return "IEEE float"
	}
	return "PCM"
}

// ---------- 变换 ----------

// cutFrames 按秒截取 [from, to)。
func cutFrames(a *wavAudio, from, to float64) (*wavAudio, error) {
	start := int(math.Floor(from * float64(a.sampleRate)))
	end := a.frames
	if to >= 0 {
		end = int(math.Ceil(to * float64(a.sampleRate)))
	}
	if start < 0 {
		start = 0
	}
	if end > a.frames {
		end = a.frames
	}
	if start >= end {
		return nil, fmt.Errorf("剪辑区间为空（%.3f~%.3f 秒，总时长 %.3f 秒）", from, to, a.durationSec())
	}
	out := *a
	out.frames = end - start
	out.chans = make([][]float64, a.channels)
	for c := range a.chans {
		out.chans[c] = append([]float64(nil), a.chans[c][start:end]...)
	}
	return &out, nil
}

// applyGain 应用增益，返回削波采样点个数。
func applyGain(a *wavAudio, db float64) int {
	g := math.Pow(10, db/20)
	clipped := 0
	for c := range a.chans {
		for i, v := range a.chans[c] {
			nv := v * g
			if nv > 1 || nv < -1 {
				clipped++
			}
			a.chans[c][i] = nv
		}
	}
	return clipped
}

// normalizePeak 归一化峰值到目标 dBFS。
func normalizePeak(a *wavAudio, dbfs float64) error {
	p := a.peak()
	if p == 0 {
		return fmt.Errorf("输入为纯静音，无法归一化")
	}
	g := math.Pow(10, dbfs/20) / p
	for c := range a.chans {
		for i := range a.chans[c] {
			a.chans[c][i] *= g
		}
	}
	return nil
}

// applyBalance 立体声平衡。
func applyBalance(a *wavAudio, b float64) error {
	if a.channels != 2 {
		return fmt.Errorf("-balance 需要立体声输入（当前 %d 声道）", a.channels)
	}
	theta := (b + 1) * math.Pi / 4
	gl, gr := math.Cos(theta), math.Sin(theta)
	for i := range a.chans[0] {
		a.chans[0][i] *= gl
		a.chans[1][i] *= gr
	}
	return nil
}

// toMono 提取声道（l/r）或平均（avg）为单声道。
func toMono(a *wavAudio, mode string) error {
	var out []float64
	switch mode {
	case "l":
		out = a.chans[0]
	case "r":
		if a.channels < 2 {
			return fmt.Errorf("输入为单声道，无法提取右声道")
		}
		out = a.chans[1]
	case "avg":
		out = make([]float64, a.frames)
		for c := range a.chans {
			for i := range a.chans[c] {
				out[i] += a.chans[c][i] / float64(a.channels)
			}
		}
	default:
		return fmt.Errorf("无效的 -mono 模式: %s（可选 l|r|avg）", mode)
	}
	a.channels = 1
	a.chans = [][]float64{out}
	return nil
}

// applySpeed 线性插值重采样变速。
func applySpeed(a *wavAudio, factor float64) {
	newFrames := int(math.Round(float64(a.frames) / factor))
	out := make([][]float64, a.channels)
	for c := range a.chans {
		dst := make([]float64, newFrames)
		for i := 0; i < newFrames; i++ {
			pos := float64(i) * factor
			i0 := int(math.Floor(pos))
			if i0 >= a.frames-1 {
				dst[i] = a.chans[c][a.frames-1]
				continue
			}
			frac := pos - float64(i0)
			dst[i] = a.chans[c][i0]*(1-frac) + a.chans[c][i0+1]*frac
		}
		out[c] = dst
	}
	a.frames = newFrames
	a.chans = out
}

// applyFade 线性淡入/淡出。
func applyFade(a *wavAudio, inSec, outSec float64) {
	sr := float64(a.sampleRate)
	if inSec > 0 {
		n := int(inSec * sr)
		if n > a.frames {
			n = a.frames
		}
		den := float64(n - 1)
		if den == 0 {
			den = 1
		}
		for i := 0; i < n; i++ {
			g := float64(i) / den
			for c := range a.chans {
				a.chans[c][i] *= g
			}
		}
	}
	if outSec > 0 {
		n := int(outSec * sr)
		if n > a.frames {
			n = a.frames
		}
		den := float64(n - 1)
		if den == 0 {
			den = 1
		}
		for i := 0; i < n; i++ {
			g := float64(n-1-i) / den
			idx := a.frames - n + i
			for c := range a.chans {
				a.chans[c][idx] *= g
			}
		}
	}
}

// reorderFlags 将 flag 参数挪到位置参数之前，使 "文件 -flag" 与 "-flag 文件" 两种顺序均可解析。
// valueFlags 为需要消费下一个参数的 flag 名集合。
func reorderFlags(args []string, valueFlags map[string]bool) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 1 && a[0] == '-' {
			if _, err := strconv.ParseFloat(a, 64); err == nil { // 负数（如增益 -3）视为位置参数
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

// ---------- 命令入口 ----------

// Run 执行命令，返回退出码。
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, "缺少子命令\n"+Usage+"\n")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "info":
		return runInfo(rest)
	case "cut":
		return runCut(rest)
	case "concat":
		return runConcat(rest)
	case "gain":
		return runGain(rest)
	case "normalize":
		return runNormalize(rest)
	case "pan":
		return runPan(rest)
	case "speed":
		return runSpeed(rest)
	case "fade":
		return runFade(rest)
	case "-h", "-help", "help":
		fmt.Print(Usage + "\n")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未知子命令: %s\n可用: info cut concat gain normalize pan speed fade\n", sub)
		return 2
	}
}

func loadWAV(path string) (*wavAudio, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取文件失败:", err)
		return nil, 1
	}
	a, err := parseWAV(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "解析 WAV 失败: %v\n", err)
		return nil, 1
	}
	return a, 0
}

func dbfs(v float64) string {
	if v <= 0 {
		return "-Inf"
	}
	return strconv.FormatFloat(20*math.Log10(v), 'f', 2, 64)
}

func summary(a *wavAudio, path string) string {
	return fmt.Sprintf("已写入 %s（%.2f 秒，%d 声道，%d Hz，%dbit %s，%d 字节）",
		path, a.durationSec(), a.channels, a.sampleRate, a.bits, a.formatName(), 44+a.frames*a.channels*a.bits/8)
}

func requireOut(out string) (string, int) {
	if out == "" {
		fmt.Fprintln(os.Stderr, "缺少 -o 输出文件")
		return "", 2
	}
	return out, 0
}

func runInfo(args []string) int {
	args = reorderFlags(args, nil)
	fs := flag.NewFlagSet(Name+" info", flag.ContinueOnError)
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox wavtool info <文件>")
		return 2
	}
	a, code := loadWAV(fs.Arg(0))
	if code != 0 {
		return code
	}
	fmt.Printf("文件: %s\n", fs.Arg(0))
	fmt.Printf("格式: %s（位深 %d）\n", a.formatName(), a.bits)
	fmt.Printf("声道: %d\n", a.channels)
	fmt.Printf("采样率: %d Hz\n", a.sampleRate)
	fmt.Printf("总帧数: %d\n", a.frames)
	fmt.Printf("时长: %.3f 秒\n", a.durationSec())
	fmt.Printf("数据大小: %d 字节\n", a.frames*a.channels*a.bits/8)
	fmt.Printf("峰值: %.6f（%s dBFS）\n", a.peak(), dbfs(a.peak()))
	return 0
}

func runCut(args []string) int {
	args = reorderFlags(args, map[string]bool{"from": true, "to": true, "o": true})
	fs := flag.NewFlagSet(Name+" cut", flag.ContinueOnError)
	from := fs.Float64("from", 0, "起始秒")
	to := fs.Float64("to", -1, "结束秒（-1 到末尾）")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox wavtool cut <文件> -from 秒 -to 秒 -o 输出")
		return 2
	}
	a, code := loadWAV(fs.Arg(0))
	if code != 0 {
		return code
	}
	res, err := cutFrames(a, *from, *to)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	path, code := requireOut(*out)
	if code != 0 {
		return code
	}
	if err := res.writeWAV(path); err != nil {
		fmt.Fprintln(os.Stderr, "写入文件失败:", err)
		return 1
	}
	fmt.Println(summary(res, path))
	return 0
}

func runConcat(args []string) int {
	args = reorderFlags(args, map[string]bool{"o": true})
	fs := flag.NewFlagSet(Name+" concat", flag.ContinueOnError)
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox wavtool concat <文件1> <文件2>... -o 输出")
		return 2
	}
	var base *wavAudio
	var all [][]float64 // 每路声道累计
	for _, p := range fs.Args() {
		a, code := loadWAV(p)
		if code != 0 {
			return code
		}
		if base == nil {
			base = a
			all = make([][]float64, a.channels)
			for c := range all {
				all[c] = append(all[c], a.chans[c]...)
			}
			continue
		}
		if a.channels != base.channels || a.sampleRate != base.sampleRate ||
			a.bits != base.bits || a.formatCode != base.formatCode {
			fmt.Fprintf(os.Stderr, "格式不一致，无法拼接: %s（%d 声道/%d Hz/%dbit）与首个文件（%d 声道/%d Hz/%dbit）\n",
				p, a.channels, a.sampleRate, a.bits, base.channels, base.sampleRate, base.bits)
			return 1
		}
		for c := range all {
			all[c] = append(all[c], a.chans[c]...)
		}
	}
	res := &wavAudio{
		formatCode: base.formatCode, channels: base.channels, sampleRate: base.sampleRate,
		bits: base.bits, frames: len(all[0]), chans: all,
	}
	path, code := requireOut(*out)
	if code != 0 {
		return code
	}
	if err := res.writeWAV(path); err != nil {
		fmt.Fprintln(os.Stderr, "写入文件失败:", err)
		return 1
	}
	fmt.Println(summary(res, path))
	return 0
}

func runGain(args []string) int {
	args = reorderFlags(args, map[string]bool{"o": true})
	fs := flag.NewFlagSet(Name+" gain", flag.ContinueOnError)
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox wavtool gain <文件> <±dB> -o 输出")
		return 2
	}
	db, err := strconv.ParseFloat(fs.Arg(1), 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无效的增益值: %s\n", fs.Arg(1))
		return 2
	}
	a, code := loadWAV(fs.Arg(0))
	if code != 0 {
		return code
	}
	clipped := applyGain(a, db)
	path, code := requireOut(*out)
	if code != 0 {
		return code
	}
	if err := a.writeWAV(path); err != nil {
		fmt.Fprintln(os.Stderr, "写入文件失败:", err)
		return 1
	}
	if clipped > 0 {
		fmt.Fprintf(os.Stderr, "警告: 增益后 %d 个采样点超界，已削波保护（限制到 ±1.0）；建议改用 normalize\n", clipped)
	}
	fmt.Println(summary(a, path))
	return 0
}

func runNormalize(args []string) int {
	args = reorderFlags(args, map[string]bool{"peak": true, "o": true})
	fs := flag.NewFlagSet(Name+" normalize", flag.ContinueOnError)
	peak := fs.Float64("peak", -1, "目标峰值 dBFS")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox wavtool normalize <文件> [-peak -3] -o 输出")
		return 2
	}
	if *peak > 0 {
		fmt.Fprintln(os.Stderr, "-peak 应为不大于 0 的 dBFS 值")
		return 2
	}
	a, code := loadWAV(fs.Arg(0))
	if code != 0 {
		return code
	}
	origPeak := dbfs(a.peak())
	if err := normalizePeak(a, *peak); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	path, code := requireOut(*out)
	if code != 0 {
		return code
	}
	if err := a.writeWAV(path); err != nil {
		fmt.Fprintln(os.Stderr, "写入文件失败:", err)
		return 1
	}
	fmt.Printf("峰值 %s dBFS → %.2f dBFS\n", origPeak, *peak)
	fmt.Println(summary(a, path))
	return 0
}

func runPan(args []string) int {
	args = reorderFlags(args, map[string]bool{"balance": true, "mono": true, "o": true})
	fs := flag.NewFlagSet(Name+" pan", flag.ContinueOnError)
	balance := fs.Float64("balance", 0, "平衡 -1(全左)~1(全右)")
	mono := fs.String("mono", "", "单声道模式 l|r|avg")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox wavtool pan <文件> -balance 值 或 -mono l|r|avg -o 输出")
		return 2
	}
	if *balance < -1 || *balance > 1 {
		fmt.Fprintln(os.Stderr, "-balance 必须在 -1~1 之间")
		return 2
	}
	if *mono != "" && *mono != "l" && *mono != "r" && *mono != "avg" {
		fmt.Fprintf(os.Stderr, "无效的 -mono 模式: %s\n", *mono)
		return 2
	}
	a, code := loadWAV(fs.Arg(0))
	if code != 0 {
		return code
	}
	if *mono != "" {
		if err := toMono(a, *mono); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
	} else {
		if err := applyBalance(a, *balance); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			return 1
		}
	}
	path, code := requireOut(*out)
	if code != 0 {
		return code
	}
	if err := a.writeWAV(path); err != nil {
		fmt.Fprintln(os.Stderr, "写入文件失败:", err)
		return 1
	}
	fmt.Println(summary(a, path))
	return 0
}

func runSpeed(args []string) int {
	args = reorderFlags(args, map[string]bool{"factor": true, "o": true})
	fs := flag.NewFlagSet(Name+" speed", flag.ContinueOnError)
	factor := fs.Float64("factor", 1, "倍速（>0）")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox wavtool speed <文件> -factor 倍率 -o 输出")
		return 2
	}
	if *factor <= 0 {
		fmt.Fprintln(os.Stderr, "-factor 必须大于 0")
		return 2
	}
	a, code := loadWAV(fs.Arg(0))
	if code != 0 {
		return code
	}
	origDur := a.durationSec()
	applySpeed(a, *factor)
	path, code := requireOut(*out)
	if code != 0 {
		return code
	}
	if err := a.writeWAV(path); err != nil {
		fmt.Fprintln(os.Stderr, "写入文件失败:", err)
		return 1
	}
	fmt.Printf("变速 %.2fx：时长 %.2f 秒 → %.2f 秒（线性插值重采样，音高同步改变）\n",
		*factor, origDur, a.durationSec())
	fmt.Println(summary(a, path))
	return 0
}

func runFade(args []string) int {
	args = reorderFlags(args, map[string]bool{"in": true, "out": true, "o": true})
	fs := flag.NewFlagSet(Name+" fade", flag.ContinueOnError)
	in := fs.Float64("in", 0, "淡入秒数")
	outSec := fs.Float64("out", 0, "淡出秒数")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox wavtool fade <文件> -in 秒 -out 秒 -o 输出")
		return 2
	}
	if *in < 0 || *outSec < 0 {
		fmt.Fprintln(os.Stderr, "-in/-out 不能为负")
		return 2
	}
	a, code := loadWAV(fs.Arg(0))
	if code != 0 {
		return code
	}
	if *in+*outSec > a.durationSec() {
		fmt.Fprintf(os.Stderr, "警告: 淡入+淡出（%.2f 秒）超过总时长（%.2f 秒），区间将重叠\n", *in+*outSec, a.durationSec())
	}
	applyFade(a, *in, *outSec)
	path, code := requireOut(*out)
	if code != 0 {
		return code
	}
	if err := a.writeWAV(path); err != nil {
		fmt.Fprintln(os.Stderr, "写入文件失败:", err)
		return 1
	}
	fmt.Println(summary(a, path))
	return 0
}
