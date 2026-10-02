// Package dockerfile 实现 Dockerfile 生成命令。
// 对应网页版：work/dockerfile-gen-tool.html（Dockerfile生成器）
package dockerfile

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

const (
	Name  = "dockerfile"
	Desc  = "按技术栈生成 Dockerfile（支持多阶段构建）"
	Usage = `用法: lyntoolbox dockerfile -stack go|node|python|java|static [-port 8080] [-multistage] [-o 输出]

参数:
  -stack       技术栈（必需）
  -port        应用监听端口（默认 8080）
  -multistage  多阶段构建（go/node/java 支持）
  -build-cmd   覆盖默认构建命令
  -run-cmd     覆盖默认启动命令
  -o           写入文件（默认打印）`
)

func template(stack string, port int, multistage bool, buildCmd, runCmd string) (string, bool) {
	defBuild := map[string]string{
		"go": "go build -o app .", "node": "npm run build", "java": "mvn -q package -DskipTests",
	}[stack]
	defRun := map[string]string{
		"go": "./app", "node": "npm start", "python": "python main.py",
		"java": "java -jar app.jar", "static": "nginx -g 'daemon off;'",
	}[stack]
	if buildCmd == "" {
		buildCmd = defBuild
	}
	if runCmd == "" {
		runCmd = defRun
	}

	switch stack {
	case "go":
		if multistage {
			return fmt.Sprintf(`# ---- 构建阶段 ----
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN %s

# ---- 运行阶段 ----
FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/app .
EXPOSE %d
CMD ["%s"]
`, buildCmd, port, strings.ReplaceAll(runCmd, "./", "")), true
		}
		return fmt.Sprintf(`FROM golang:1.24-alpine
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN %s
EXPOSE %d
CMD ["%s"]
`, buildCmd, port, strings.ReplaceAll(runCmd, "./", "")), true
	case "node":
		if multistage {
			return fmt.Sprintf(`# ---- 构建阶段 ----
FROM node:22-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN %s

# ---- 运行阶段 ----
FROM node:22-alpine
WORKDIR /app
ENV NODE_ENV=production
COPY --from=builder /app ./
EXPOSE %d
CMD %s
`, buildCmd, port, toJSONCmd(runCmd)), true
		}
		return fmt.Sprintf(`FROM node:22-alpine
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
EXPOSE %d
CMD %s
`, port, toJSONCmd(runCmd)), true
	case "python":
		return fmt.Sprintf(`FROM python:3.13-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
EXPOSE %d
CMD %s
`, port, toJSONCmd(runCmd)), true
	case "java":
		if multistage {
			return fmt.Sprintf(`# ---- 构建阶段 ----
FROM maven:3.9-eclipse-temurin-21 AS builder
WORKDIR /app
COPY pom.xml .
RUN mvn dependency:go-offline
COPY src ./src
RUN %s

# ---- 运行阶段 ----
FROM eclipse-temurin:21-jre-alpine
WORKDIR /app
COPY --from=builder /app/target/*.jar app.jar
EXPOSE %d
CMD %s
`, buildCmd, port, toJSONCmd(runCmd)), true
		}
		return fmt.Sprintf(`FROM eclipse-temurin:21-jre
WORKDIR /app
COPY target/*.jar app.jar
EXPOSE %d
CMD %s
`, port, toJSONCmd(runCmd)), true
	case "static":
		return fmt.Sprintf(`FROM nginx:1.27-alpine
COPY . /usr/share/nginx/html
EXPOSE %d
CMD %s
`, port, toJSONCmd(runCmd)), true
	}
	return "", false
}

// toJSONCmd 把 shell 命令转成 CMD 列表形式（简化：单命令字符串形式）。
func toJSONCmd(cmd string) string {
	return fmt.Sprintf(`["/bin/sh", "-c", "%s"]`, strings.ReplaceAll(cmd, `"`, `\"`))
}

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	stack := fs.String("stack", "", "技术栈 go|node|python|java|static")
	port := fs.Int("port", 8080, "监听端口")
	multi := fs.Bool("multistage", false, "多阶段构建")
	buildCmd := fs.String("build-cmd", "", "覆盖构建命令")
	runCmd := fs.String("run-cmd", "", "覆盖启动命令")
	out := fs.String("o", "", "输出文件")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *stack == "" {
		fmt.Fprintln(os.Stderr, "缺少 -stack 参数")
		return 2
	}
	tpl, ok := template(*stack, *port, *multi, *buildCmd, *runCmd)
	if !ok {
		fmt.Fprintf(os.Stderr, "未知技术栈: %s\n", *stack)
		return 2
	}
	if *out != "" {
		if err := os.WriteFile(*out, []byte(tpl), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "写入失败:", err)
			return 1
		}
		fmt.Printf("已写入 %s\n", *out)
		return 0
	}
	fmt.Print(tpl)
	return 0
}
