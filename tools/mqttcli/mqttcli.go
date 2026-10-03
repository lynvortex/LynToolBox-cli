// Package mqttcli 实现 MQTT 客户端调试命令。
// 对应网页版：network/mqtt-tool.html（MQTT客户端测试）
package mqttcli

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	Name  = "mqttcli"
	Desc  = "MQTT broker 调试（sub 订阅 / pub 发布）"
	Usage = `用法:
  lyntoolbox mqttcli sub -url tcp://host:1883 -sub 主题 [-sub 主题2] [-n 条数] [-t 秒]
  lyntoolbox mqttcli pub -url tcp://host:1883 -pub 主题 -m 消息 [-qos 0]

参数:
  -url      broker 地址（tcp:// 或 ssl://，默认 tcp://127.0.0.1:1883）
  -user     用户名
  -pass     密码
  -clientid 客户端 ID（默认随机）
  -sub      订阅主题（sub 模式，支持 # + 通配，可重复）
  -pub      发布主题（pub 模式）
  -m        发布消息内容
  -qos      QoS 0/1/2（默认 0）
  -n        sub 模式最多接收条数（默认 0=不限）
  -t        sub 模式总限时秒（默认 0=不限，Ctrl+C 退出）`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "缺少子命令 sub|pub")
		fmt.Fprint(os.Stderr, Usage+"\n")
		return 2
	}
	mode := args[0]
	if mode != "sub" && mode != "pub" {
		fmt.Fprintf(os.Stderr, "未知子命令: %s（可用 sub|pub）\n", mode)
		return 2
	}

	fs := flag.NewFlagSet(mode, flag.ContinueOnError)
	url := fs.String("url", "tcp://127.0.0.1:1883", "broker 地址")
	user := fs.String("user", "", "用户名")
	pass := fs.String("pass", "", "密码")
	clientID := fs.String("clientid", "", "客户端 ID")
	var subs stringSlice
	fs.Var(&subs, "sub", "订阅主题（可重复）")
	pubTopic := fs.String("pub", "", "发布主题")
	msg := fs.String("m", "", "发布消息")
	qos := fs.Int("qos", 0, "QoS 0/1/2")
	n := fs.Int("n", 0, "最多接收条数")
	total := fs.Int("t", 0, "总限时秒")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *clientID == "" {
		*clientID = fmt.Sprintf("lyntoolbox-%d", rand.Intn(1000000))
	}
	opts := mqtt.NewClientOptions().
		AddBroker(*url).
		SetClientID(*clientID).
		SetCleanSession(true).
		SetConnectTimeout(15 * time.Second)
	if *user != "" {
		opts.SetUsername(*user)
		opts.SetPassword(*pass)
	}
	lost := make(chan struct{})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		fmt.Fprintln(os.Stderr, "连接断开:", err)
		close(lost)
	})
	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.Wait()
	if err := token.Error(); err != nil {
		fmt.Fprintln(os.Stderr, "连接失败:", err)
		return 1
	}
	defer client.Disconnect(250)
	fmt.Printf("已连接 %s（clientid=%s）\n", *url, *clientID)

	switch mode {
	case "pub":
		if *pubTopic == "" || *msg == "" {
			fmt.Fprintln(os.Stderr, "pub 模式需要 -pub 主题 与 -m 消息")
			return 2
		}
		t := client.Publish(*pubTopic, byte(*qos), false, *msg)
		t.WaitTimeout(5 * time.Second)
		if err := t.Error(); err != nil {
			fmt.Fprintln(os.Stderr, "发布失败:", err)
			return 1
		}
		fmt.Printf("已发布到 %s（QoS %d）\n", *pubTopic, *qos)
		time.Sleep(300 * time.Millisecond)
		return 0

	case "sub":
		if len(subs) == 0 {
			fmt.Fprintln(os.Stderr, "sub 模式需要 -sub 主题")
			return 2
		}
		received := make(chan int, 1)
		var count int64
		handler := func(_ mqtt.Client, m mqtt.Message) {
			n := atomic.AddInt64(&count, 1)
			fmt.Printf("[↓ #%d] %s\n  %s\n", n, m.Topic(), string(m.Payload()))
			// 非阻塞投递：主循环未消费时不得卡住 paho 的消息路由 goroutine
			select {
			case received <- int(n):
			default:
			}
		}
		for _, s := range subs {
			t := client.Subscribe(s, byte(*qos), handler)
			t.Wait()
			if err := t.Error(); err != nil {
				fmt.Fprintf(os.Stderr, "订阅 %s 失败: %v\n", s, err)
				return 1
			}
			fmt.Printf("已订阅: %s\n", s)
		}
		fmt.Println("等待消息（Ctrl+C 退出）...")
		deadline := time.Time{}
		if *total > 0 {
			deadline = time.Now().Add(time.Duration(*total) * time.Second)
		}
		for {
			var timeoutCh <-chan time.Time
			if !deadline.IsZero() {
				timeoutCh = time.After(time.Until(deadline))
			}
			select {
			case c := <-received:
				if *n > 0 && c >= *n {
					fmt.Printf("已收到 %d 条，退出\n", c)
					return 0
				}
			case <-timeoutCh:
				fmt.Println("限时到达，退出")
				return 0
			case <-lost:
				return 1
			}
		}
	}
	return 0
}

// stringSlice 支持 -flag 可重复。
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}
