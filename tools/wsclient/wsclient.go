// Package wsclient 实现 WebSocket 客户端调试命令。
// 对应网页版：network/websocket-tool.html（WebSocket客户端）
package wsclient

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	Name  = "wsclient"
	Desc  = "WebSocket 服务器连接调试（收发消息）"
	Usage = `用法: lyntoolbox wsclient ws(s)://地址 [-send 消息]... [-listen 秒] [-hdr "K: V"]... [-t 30]

参数:
  -send     发送的消息（可重复；省略则只监听）
  -listen   纯监听秒数
  -hdr      自定义请求头（可重复，格式 "K: V"）
  -t        总超时秒（默认 30）`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	var sends, hdrs stringSlice
	listen := fs.Int("listen", 0, "纯监听秒数")
	timeout := fs.Int("t", 30, "总超时秒")
	fs.Var(&sends, "send", "发送消息（可重复）")
	fs.Var(&hdrs, "hdr", "自定义请求头（可重复）")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "用法: lyntoolbox wsclient ws(s)://地址")
		return 2
	}
	wsURL := fs.Arg(0)
	if !strings.HasPrefix(wsURL, "ws://") && !strings.HasPrefix(wsURL, "wss://") {
		wsURL = "wss://" + wsURL
	}

	header := make(http.Header)
	for _, h := range hdrs {
		if i := strings.Index(h, ":"); i > 0 {
			header.Set(strings.TrimSpace(h[:i]), strings.TrimSpace(h[i+1:]))
		}
	}

	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, resp, err := dialer.Dial(wsURL, header)
	if err != nil {
		if resp != nil {
			fmt.Fprintf(os.Stderr, "连接失败: %v（HTTP %d）\n", err, resp.StatusCode)
		} else {
			fmt.Fprintln(os.Stderr, "连接失败:", err)
		}
		return 1
	}
	defer conn.Close()
	fmt.Printf("已连接: %s\n", wsURL)

	total := time.Duration(*timeout) * time.Second
	if *listen > 0 {
		total = time.Duration(*listen) * time.Second
	}
	deadline := time.Now().Add(total)
	conn.SetReadDeadline(deadline)

	// 发送
	for _, msg := range sends {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			fmt.Fprintln(os.Stderr, "发送失败:", err)
			return 1
		}
		fmt.Printf("[%s] [↑] %s\n", time.Now().Format("15:04:05"), msg)
	}

	// 接收（有 -listen 或无 -send 时进入循环；有 -send 时也读一次响应）
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	readLoop := true
	if len(sends) > 0 && *listen == 0 {
		// 发送后读取响应直至超时或一条消息
		for i := 0; i < len(sends); i++ {
			if !readOne(conn, deadline) {
				readLoop = false
				break
			}
		}
		readLoop = false
	}
	if readLoop {
		fmt.Println("监听中（Ctrl+C 退出）...")
		go func() {
			<-interrupt
			conn.Close()
			fmt.Println("\n已断开")
			os.Exit(0)
		}()
		for {
			if !readOne(conn, deadline) {
				break
			}
		}
	}
	if len(sends) > 0 || *listen > 0 {
		return 0
	}
	// 纯连接测试
	fmt.Println("连接正常（未发送/监听任何消息）")
	return 0
}

func readOne(conn *websocket.Conn, deadline time.Time) bool {
	conn.SetReadDeadline(deadline)
	mt, data, err := conn.ReadMessage()
	now := time.Now().Format("15:04:05")
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			fmt.Println("\n监听超时")
		} else {
			fmt.Printf("\n连接关闭: %v\n", err)
		}
		return false
	}
	if mt == websocket.TextMessage {
		fmt.Printf("[%s] [↓] %s\n", now, string(data))
	} else {
		fmt.Printf("[%s] [↓] <二进制 %d 字节>\n", now, len(data))
	}
	return true
}

// stringSlice 支持 -flag 可重复。
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}
