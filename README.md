# LynToolBox-cli

绘萤工具箱命令行版。**双击 exe（或无参数运行）即进入交互菜单**：显示分组工具序号列表，输入序号、按提示填内容即可出结果；同时保留命令行参数模式供脚本调用。

- 网页版：<https://github.com/lynvortex/LynToolBox>
- 在线使用：<https://box.lynvortex.top>

**纯离线**：全部 108 个工具均为本地计算（内嵌 IEEE OUI 离线库等数据），除"访问你自己指定的网络目标"类工具（ping/HTTP 请求/证书查询等）外不产生任何网络请求。

## 使用

```
lyntoolbox.exe            ← 双击打开：交互式菜单（推荐）
lyntoolbox.exe <命令> ...  ← 命令行模式（脚本/进阶）
```

交互模式：输入序号 → 按提示逐项填写（有默认值的直接回车）→ 出结果 → 回车返回菜单，`q` 退出。也支持直接输入命令名。两种模式均支持"位置参数在前、选项在后"的书写习惯。

## 构建

```bash
go build -ldflags "-s -w" -o lyntoolbox.exe .
```

纯 Go 无 cgo；国内网络建议 `go env -w GOPROXY=https://goproxy.cn,direct`。

## 全部命令（108 个）

| 分组 | 命令 | 说明 |
|---|---|---|
| **A 编码·加密·哈希（17）** | `hash` | MD5/SHA/CRC32，文本与文件 |
| | `base64` | Base64 互转，URL 安全字母表 |
| | `urlenc` | URL 百分号编解码 |
| | `hexenc` | 文本↔十六进制 |
| | `base58` | Base58 / Base58Check |
| | `htmlent` | HTML 实体编解码 |
| | `gzipx` | gzip 压缩解压（Base64/二进制） |
| | `shesc` | POSIX/cmd/PowerShell 转义 |
| | `morse` | 摩斯密码 |
| | `aes` | AES CBC/ECB/CFB + PKCS7 |
| | `smcrypto` | 国密 SM3/SM4/SM2 套件 |
| | `hmac` | HMAC-SHA 签名 |
| | `jwt` | JWT 解码/过期检查/HS256 验签 |
| | `rsagen` | RSA 密钥对（PKCS#1/#8） |
| | `pbkdf2` | PBKDF2 密钥派生 |
| | `totp` | TOTP/HOTP 动态口令 |
| | `randgen` | UUID/密码/令牌/随机数 |
| **B 数据格式（15）** | `jsonfmt` | JSON 格式化/压缩/校验 |
| | `jsonpath` | JSONPath 查询 |
| | `json2code` | JSON→TS/Go/Java/C#/Python 类型 |
| | `jsonschema` | JSON Schema 校验 |
| | `yamlconv` | YAML 格式化与互转 |
| | `tomlconv` | TOML 格式化与互转 |
| | `xmlfmt` | XML 格式化/压缩/转 JSON |
| | `csvjson` | CSV↔JSON |
| | `csv2sql` | CSV→SQL INSERT |
| | `envconv` | .env↔JSON/YAML |
| | `sqlfmt` | SQL 格式化 |
| | `codefmt` | HTML/CSS/JS 美化 |
| | `minify` | CSS/HTML/JS 压缩 |
| | `cloc` | 代码行数统计 |
| | `mdconv` | Markdown↔HTML |
| **C 文本批处理（8）** | `textdiff` | 文本 Diff 对比 |
| | `sortuniq` | 行排序去重 |
| | `textrepl` | 批量查找替换 |
| | `extract` | 提取 URL/邮箱/手机号/IP/身份证 |
| | `maskdata` | 敏感信息打码 |
| | `tabspace` | Tab↔空格 |
| | `slug` | URL Slug（中文转拼音） |
| | `zhconv` | 简繁转换/拼音 |
| **D 生成器（8）** | `gitignore` | .gitignore 生成 |
| | `robots` | robots.txt 生成 |
| | `nginxconf` | nginx.conf 生成 |
| | `dockerfile` | Dockerfile 生成 |
| | `metatags` | Meta/OG 标签生成 |
| | `lorem` | 假文/Mock 数据 |
| | `figlet` | ASCII 横幅 |
| | `qrcode` | 二维码生成/识别/WiFi |
| **E 网络工具（12）** | `ping` | 连通性测试 |
| | `portscan` | TCP 端口扫描 |
| | `httpreq` | HTTP 请求模拟 |
| | `httphead` | 响应头安全分析 |
| | `dnsq` | DNS 查询 / DoH |
| | `subnet` | IPv4/IPv6 子网计算 |
| | `ipchk` | IP 校验分类 |
| | `urlparse` | URL 解析 |
| | `uaparse` | UA 解析 |
| | `mime` | MIME 互查 |
| | `macloc` | MAC 厂商（内嵌 OUI 库） |
| | `tsconv` | 时间戳互转 |
| **F 计算·校验（8）** | `basecalc` | 进制转换/位运算 |
| | `bignum` | 大数高精度计算 |
| | `calc` | 科学计算器 |
| | `snowflake` | 雪花 ID 生成解析 |
| | `semver` | SemVer 比较 |
| | `regext` | 正则测试+速查 |
| | `cronx` | Cron 解析生成 |
| | `stats` | 统计计算器 |
| **G 网络补充（9）** | `redirchain` | 重定向链追踪 |
| | `sslcert` | SSL 证书查询 |
| | `whois` | WHOIS 查询 |
| | `wsclient` | WebSocket 调试 |
| | `mqttcli` | MQTT 调试 |
| | `vsrc` | 网页源码查看 |
| | `httpcode` | 状态码速查（离线） |
| | `cookiep` | Cookie 解析 |
| | `corschk` | CORS 检测 |
| **H 文件·文档（7）** | `zipx` | ZIP 打包解包 |
| | `srt` | SRT 字幕时间轴 |
| | `docx2md` | Word 提取转 Markdown |
| | `epubx` | EPUB 处理 |
| | `xlsxtool` | Excel 处理 |
| | `pdf` | PDF 合并拆分旋转提取 |
| | `pdfop` | PDF 优化与水印 |
| **I 图片批处理（13）** | `imgconv` | 压缩/缩放/格式转换 |
| | `imgwm` | 文字/图片水印 |
| | `exift` | EXIF 查看/清除 |
| | `imggeom` | 裁剪/旋转/翻转/拼接/宫格 |
| | `imgfilter` | 滤镜套件（9 种可组合） |
| | `phash` | 感知哈希相似度 |
| | `imgascii` | 图片转 ASCII |
| | `stegano` | LSB 隐写 |
| | `imgcolor` | 主色提取 |
| | `placeholder` | 占位图生成 |
| | `favicon` | 多尺寸 ICO 生成 |
| | `barcode` | 条形码生成 |
| | `bgremove` | 背景移除/换底色 |
| **J 文本·计算补充（11）** | `wordcount` | 字数统计 |
| | `shufflelines` | 文本打乱 |
| | `encconv` | 编码转换（GBK/Big5/Shift-JIS） |
| | `hexdump` | 十六进制查看 |
| | `validate` | 格式校验套件 |
| | `datecalc` | 日期计算/RRULE |
| | `unitconv` | 单位换算 |
| | `colorcalc` | 颜色计算 |
| | `geodist` | 经纬度距离 |
| | `wavtool` | WAV 音频处理 |
| | `svgmin` | SVG 压缩 |

每个命令详情：`lyntoolbox <命令名> -h`。

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

- 新增工具：在 `tools/<命令名>/` 新建包，导出 `Name`/`Desc`/`Usage` 常量与 `Run(args []string) int`，在 `main.go` 的 `registerAll()` 注册；交互模式在 `interactive.go` 的 `interactivePrompts` 补一条输入定义。
- 退出码：0 成功 / 1 运行错误 / 2 用法错误。
- 依赖白名单见 `internal/deps/deps.go`；新增依赖需同步更新。

## 状态

v1.0.0 · 108 个命令全部可用，11 个分组。加密类实现经标准向量验证（SM3/HMAC/PBKDF2/RFC 6238 TOTP），图片/文档工具经往返实测。
