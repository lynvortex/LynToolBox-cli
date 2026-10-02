// Package lorem 实现假文与 Mock 数据生成命令。
// 对应网页版：text/lorem-tool.html（假文生成器）
package lorem

import (
	"crypto/rand"
	"flag"
	"fmt"
	"math/big"
	"os"
	"strings"
)

const (
	Name  = "lorem"
	Desc  = "生成假文/随机姓名/手机号/邮箱/身份证等 Mock 数据"
	Usage = `用法: lyntoolbox lorem -what text|name|phone|email|idcard [-n 数量]

参数:
  -what    类型：text（默认）|name|phone|email|idcard|address
  -n       数量（text 默认 3 段，其余默认 5 条）
  -lang    text 模式语言 en|zh（默认 zh）
  -sent    每段句数（默认 4）`
)

var (
	enWords       = strings.Fields("lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua enim ad minim veniam quis nostrud exercitation ullamco laboris nisi aliquip ex ea commodo consequat duis aute irure in reprehenderit voluptate velit esse cillum eu fugiat nulla pariatur excepteur sint occaecat cupidatat non proident sunt culpa qui officia deserunt mollit anim id est laborum")
	zhWords       = strings.Fields("数据 处理 系统 网络 开发 设计 优化 平台 服务 用户 信息 结构 方法 功能 模块 配置 管理 分析 计算 存储 接口 协议 测试 部署 运行 维护 更新 版本 质量 效率 安全 可靠 灵活 简单 快速 稳定 支持 提供 实现 完成 检查 确认 选择 应用 场景 需求 方案 目标 结果 过程 内容 模式 标准 规则")
	surnames      = []rune("赵钱孙李周吴郑王冯陈褚卫蒋沈韩杨朱秦尤许何吕施张孔曹严华金魏陶姜戚谢邹喻柏水窦章云苏潘葛奚范彭郎鲁韦昌马苗凤花方俞任袁柳唐罗薛雷贺倪汤")
	givenNames    = strings.Fields("伟 芳 娜 敏 静 丽 强 磊 军 洋 勇 艳 杰 娟 涛 明 超 秀兰 霞 平 刚 桂英 华 建 文 辉 春梅 晓燕 天翔 宇轩 雨欣 子涵 浩然 思远 静怡 志强 建国 淑珍 明明 海燕 冬梅")
	emailDomains  = []string{"gmail.com", "qq.com", "163.com", "outlook.com", "foxmail.com", "126.com"}
	provinces     = []string{"110000 北京市", "310000 上海市", "440100 广州市", "440300 深圳市", "330100 杭州市", "510100 成都市", "420100 武汉市", "430100 长沙市", "610100 西安市", "320100 南京市"}
	provinceCodes = []string{"11", "31", "44", "44", "33", "51", "42", "43", "61", "32"}
	idWeights     = []int{7, 9, 10, 5, 8, 4, 2, 1, 6, 3, 7, 9, 10, 5, 8, 4, 2}
	idCheckMap    = []string{"1", "0", "X", "9", "8", "7", "6", "5", "4", "3", "2"}
)

func rnd(n int) int {
	v, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0
	}
	return int(v.Int64())
}

func pick(s []string) string { return s[rnd(len(s))] }

func genSentence(lang string) string {
	var b strings.Builder
	n := 6 + rnd(10)
	for i := 0; i < n; i++ {
		if lang == "en" {
			b.WriteString(enWords[rnd(len(enWords))])
		} else {
			b.WriteString(zhWords[rnd(len(zhWords))])
		}
		if lang == "en" && i < n-1 {
			b.WriteString(" ")
		}
	}
	if lang == "en" {
		b.WriteString(".")
	} else {
		b.WriteString("。")
	}
	return b.String()
}

func genIDCard() string {
	idx := rnd(len(provinceCodes))
	// 顺序：地区码(6) + 生日(8) + 顺序码(3) + 校验位
	seq := fmt.Sprintf("%s%04d%02d%02d%03d", provinceCodes[idx], 1970+rnd(35), 1+rnd(12), 1+rnd(28), rnd(1000))
	sum := 0
	for i, r := range seq[:17] {
		sum += int(r-'0') * idWeights[i]
	}
	return seq + idCheckMap[sum%11]
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	what := fs.String("what", "text", "类型 text|name|phone|email|idcard")
	n := fs.Int("n", 0, "数量")
	lang := fs.String("lang", "zh", "text 语言 en|zh")
	sent := fs.Int("sent", 4, "每段句数")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *n == 0 {
		if *what == "text" {
			*n = 3
		} else {
			*n = 5
		}
	}

	switch *what {
	case "text":
		for p := 0; p < *n; p++ {
			var b strings.Builder
			for s := 0; s < *sent; s++ {
				b.WriteString(genSentence(*lang))
				if *lang == "en" {
					b.WriteString(" ")
				}
			}
			fmt.Println(strings.TrimSpace(b.String()))
			fmt.Println()
		}
	case "name":
		for i := 0; i < *n; i++ {
			fmt.Printf("%c%s\n", surnames[rnd(len(surnames))], pick(givenNames))
		}
	case "phone":
		prefixes := []string{"130", "131", "135", "138", "139", "150", "155", "158", "166", "170", "176", "177", "180", "182", "185", "188", "189", "191", "198", "199"}
		for i := 0; i < *n; i++ {
			suffix := ""
			for d := 0; d < 8; d++ {
				suffix += fmt.Sprint(rnd(10))
			}
			fmt.Println(pick(prefixes) + suffix)
		}
	case "email":
		for i := 0; i < *n; i++ {
			user := ""
			for d := 0; d < 8+rnd(5); d++ {
				user += string(rune('a' + rnd(26)))
			}
			fmt.Printf("%s%d@%s\n", user, rnd(10000), pick(emailDomains))
		}
	case "idcard":
		for i := 0; i < *n; i++ {
			fmt.Println(genIDCard())
		}
	default:
		fmt.Fprintf(os.Stderr, "未知类型: %s\n", *what)
		return 2
	}
	return 0
}
