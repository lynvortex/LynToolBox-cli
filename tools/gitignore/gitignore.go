// Package gitignore 实现 .gitignore 生成命令。
// 对应网页版：work/gitignore-tool.html（.gitignore生成器）
package gitignore

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

const (
	Name  = "gitignore"
	Desc  = "按语言/框架组合生成 .gitignore"
	Usage = `用法: lyntoolbox gitignore -stack go,node [-list] [-o 输出]

参数:
  -stack  逗号分隔的技术栈（默认 go,node）
  -list   列出全部可用模板
  -o      写入文件（默认打印）`
)

var templates = map[string]string{
	"go": `# Go
*.exe
*.exe~
*.dll
*.so
*.dylib
*.test
*.out
vendor/
go.sum
`,
	"node": `# Node
node_modules/
npm-debug.log*
yarn-debug.log*
yarn-error.log*
.npm
dist/
build/
.env
.env.local
coverage/
`,
	"python": `# Python
__pycache__/
*.py[cod]
*.egg-info/
.eggs/
venv/
.venv/
env/
build/
dist/
.pytest_cache/
.mypy_cache/
.coverage
htmlcov/
`,
	"java": `# Java
*.class
*.jar
*.war
target/
build/
.gradle/
.mvn/
*.log
hs_err_pid*
`,
	"rust": `# Rust
target/
Cargo.lock
*.rs.bk
`,
	"csharp": `# C#
bin/
obj/
*.user
*.suo
.vs/
packages/
TestResults/
`,
	"cpp": `# C/C++
*.o
*.obj
*.a
*.lib
*.so
*.dylib
*.exe
build/
cmake-build-*/
CMakeFiles/
`,
	"php": `# PHP
vendor/
composer.lock
*.log
.env
`,
	"ruby": `# Ruby
*.gem
.bundle/
vendor/bundle/
Gemfile.lock
coverage/
`,
	"vue": `# Vue
node_modules/
dist/
.npmrc
.DS_Store
*.local
`,
	"react": `# React
node_modules/
build/
dist/
.DS_Store
.env.local
coverage/
`,
	"flutter": `# Flutter
.dart_tool/
.flutter-plugins
.packages
build/
ios/Pods/
*.iml
`,
	"android": `# Android
*.apk
*.aab
.gradle/
build/
local.properties
.idea/
*.iml
captures/
`,
	"ios": `# iOS
xcuserdata/
*.xcuserstate
Pods/
DerivedData/
*.ipa
.DS_Store
`,
	"macos": `# macOS
.DS_Store
._*
.Spotlight-V100
.Trashes
Icon?
`,
	"windows": `# Windows
Thumbs.db
ehthumbs.db
Desktop.ini
$RECYCLE.BIN/
*.cab
*.msi
`,
	"linux": `# Linux
*~
.fuse_hidden*
.directory
.Trash-*
`,
	"idea": `# IntelliJ IDEA
.idea/
*.iml
*.ipr
*.iws
out/
`,
	"vscode": `# VS Code
.vscode/*
!.vscode/settings.json
!.vscode/tasks.json
!.vscode/launch.json
.history/
`,
	"eclipse": `# Eclipse
.project
.classpath
.settings/
bin/
`,
	"dotnetcore": `# .NET Core
bin/
obj/
*.user
artifacts/
global.json
`,
	"laravel": `# Laravel
vendor/
node_modules/
.env
storage/*.key
public/storage
`,
	"django": `# Django
*.pyc
__pycache__/
db.sqlite3
media/
staticfiles/
.env
`,
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	stack := fs.String("stack", "go,node", "逗号分隔的技术栈")
	list := fs.Bool("list", false, "列出可用模板")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	if *list {
		names := make([]string, 0, len(templates))
		for k := range templates {
			names = append(names, k)
		}
		for i := 0; i < len(names); i++ {
			for j := i + 1; j < len(names); j++ {
				if names[j] < names[i] {
					names[i], names[j] = names[j], names[i]
				}
			}
		}
		fmt.Println("可用模板：", strings.Join(names, ", "))
		return 0
	}

	var b strings.Builder
	seen := map[string]bool{}
	for _, s := range strings.Split(*stack, ",") {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		tpl, ok := templates[s]
		if !ok {
			fmt.Fprintf(os.Stderr, "未知模板: %s（用 -list 查看全部）\n", s)
			return 2
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		b.WriteString(tpl)
		b.WriteString("\n")
	}

	if *out != "" {
		if err := os.WriteFile(*out, []byte(b.String()), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "写入失败:", err)
			return 1
		}
		fmt.Printf("已写入 %s\n", *out)
		return 0
	}
	fmt.Print(b.String())
	return 0
}
