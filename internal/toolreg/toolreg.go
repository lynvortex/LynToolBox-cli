// Package toolreg 定义命令行工具的统一注册结构。
// 每个工具位于 tools/<命令名>/ 下的一个单独源文件，
// 通过导出的 Tool 变量暴露元信息，由 main.go 汇总注册。
package toolreg

// Tool 描述一个子命令。
type Tool struct {
	Name  string                  // 命令名，如 "hash"
	Group string                  // 分组名，如 "A 编码·加密·哈希"
	Desc  string                  // 一句话中文描述
	Usage string                  // 详细用法（多行）
	Run   func(args []string) int // 执行入口；返回进程退出码：0 成功 / 1 运行错误 / 2 用法错误
}

// All 保存全部已注册的命令。
var All []*Tool

// Register 追加一个命令。
func Register(t *Tool) { All = append(All, t) }
