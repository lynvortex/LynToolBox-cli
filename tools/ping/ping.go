// Package ping 实现 Ping 连通性测试命令。
// 对应网页版：network/ping-tool.html（Ping测试）
//
// 用法：
//
//	lyntoolbox ping [-c 次数] [-w 超时毫秒] 主机
package ping

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
)

const (
	Name  = "ping"
	Desc  = "调用系统 ping 进行网络连通性测试，实时回显系统输出"
	Usage = `用法: lyntoolbox ping [-c 次数] [-w 超时毫秒] 主机

参数:
  -c   发送回显请求的次数（默认 4）
  -w   每次等待应答的超时毫秒数（默认 3000）
  主机  目标主机名或 IP 地址

说明: 调用各平台的系统 ping（Windows: ping 主机 -n 次数 -w 毫秒；Linux: -c 次数 -W 秒；
macOS: -c 次数 -t 秒），输出跟随系统语言。`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	count := fs.Int("c", 4, "发送次数")
	timeout := fs.Int("w", 3000, "超时毫秒数")
	fs.Usage = func() { fmt.Print(Usage + "\n") }

	// 允许位置参数后仍跟选项（如: ping 1.2.3.4 -c 2）
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
	if len(positionals) != 1 {
		fmt.Fprintln(os.Stderr, "错误: 需要且只需要一个目标主机参数")
		fmt.Fprintln(os.Stderr, "（lyntoolbox ping -h 查看帮助）")
		return 2
	}
	if *count < 1 {
		fmt.Fprintln(os.Stderr, "错误: -c 次数必须 ≥ 1")
		return 2
	}
	if *timeout < 1 {
		fmt.Fprintln(os.Stderr, "错误: -w 超时必须 ≥ 1 毫秒")
		return 2
	}
	host := positionals[0]

	// 各平台系统 ping 的参数语义不同：
	// Windows: -n 次数 -w 毫秒；Linux(iputils): -c 次数 -w 秒(整体deadline)；
	// macOS/BSD: -c 次数 -t 秒(整体超时)，-W 是单次应答等待毫秒。
	sec := strconv.Itoa((*timeout + 999) / 1000)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("ping", host, "-n", strconv.Itoa(*count), "-w", strconv.Itoa(*timeout))
	case "darwin", "freebsd", "openbsd", "netbsd":
		cmd = exec.Command("ping", host, "-c", strconv.Itoa(*count), "-t", sec)
	default: // linux 及其他类 unix
		cmd = exec.Command("ping", host, "-c", strconv.Itoa(*count), "-W", sec)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			fmt.Fprintf(os.Stderr, "ping 进程退出码 %d（目标可能不可达）\n", ee.ExitCode())
			return 1
		}
		fmt.Fprintf(os.Stderr, "无法启动 ping 命令: %v\n", err)
		return 1
	}
	return 0
}
