# LynToolBox-cli

绘萤工具箱命令行版。**双击 exe（或无参数运行）即进入交互菜单**：显示工具序号列表，输入序号、按提示填内容即可出结果；同时保留命令行参数模式供脚本调用。

- 网页版：<https://github.com/lynvortex/LynToolBox>
- 在线使用：<https://box.lynvortex.top>

## 使用

```
lyntoolbox.exe            ← 双击打开：交互式菜单（推荐）
lyntoolbox.exe <命令> ...  ← 命令行模式（脚本/进阶）
```

交互模式：输入序号 → 按提示逐项填写（有默认值的直接回车）→ 出结果 → 回车返回菜单，`q` 退出。也支持直接输入命令名（如 `hash`）。

## 构建

```bash
go build -o lyntoolbox.exe .
```

依赖为纯 Go 库，无 cgo；国内网络建议 `go env -w GOPROXY=https://goproxy.cn,direct`。

## 当前包含的命令（22 个）

| 分组 | 命令 | 说明 |
|---|---|---|
| 编码·加密·哈希 | `hash` | MD5/SHA/CRC32 哈希，支持文本与文件 |
| | `base64` | 文本/文件 Base64 互转，支持 URL 安全字母表 |
| 生成器 | `gitignore` | 按语言/框架组合生成 .gitignore |
| | `robots` | 生成 robots.txt |
| | `nginxconf` | 生成 nginx.conf（反代/SSL/静态/Gzip） |
| | `dockerfile` | 按技术栈生成 Dockerfile（支持多阶段构建） |
| | `metatags` | 生成 meta / Open Graph / Twitter 标签块 |
| | `lorem` | 假文与 Mock 数据（姓名/手机号/邮箱/身份证） |
| | `figlet` | ASCII 块状字符横幅 |
| | `qrcode` | 二维码生成（终端/PNG）与识别，支持 WiFi 编码 |
| 网络工具 | `ping` | 系统 ping 连通性测试 |
| | `portscan` | TCP 端口检测与扫描（并发） |
| | `httpreq` | HTTP 请求模拟器 |
| | `httphead` | HTTP 响应头分析与安全项检查 |
| | `dnsq` | DNS 查询（系统解析器 / DoH） |
| | `subnet` | IPv4/IPv6 子网计算器 |
| | `ipchk` | IP 校验与分类解析 |
| | `urlparse` | URL 结构解析 |
| | `uaparse` | User-Agent 解析 |
| | `mime` | MIME 类型与扩展名互查 |
| | `macloc` | MAC 厂商查询（内嵌 IEEE OUI 离线库） |
| | `tsconv` | 时间戳与日期时间互转 |

命令行模式详情：`lyntoolbox <命令名> -h`。两种模式均支持"位置参数在前、选项在后"的书写习惯。

## 仓库结构

```
main.go                  入口：交互菜单 / 命令分发（兼容乱序参数）
interactive.go           交互式菜单框架与各工具的输入提示定义
internal/toolreg/        命令注册结构
internal/deps/           第三方依赖锁定（空白导入）
tools/<命令名>/<命令名>.go   每个工具一个单文件实现
pending/                 暂存区：未集成的工具源码（不参与编译）
```

## 约定

- 新增工具：在 `tools/<命令名>/` 新建包，导出 `Name`/`Desc`/`Usage` 常量与 `Run(args []string) int`，在 `main.go` 的 `registerAll()` 注册；若需交互模式，在 `interactive.go` 的 `interactivePrompts` 中补一条输入定义。
- 退出码：0 成功 / 1 运行错误 / 2 用法错误。
- 依赖白名单见 `internal/deps/deps.go`；新增依赖需同步更新。

## 状态

v1.0.0 · 已发布 22 个命令；更多工具源码见 `pending/`，整理后可随时集成。
