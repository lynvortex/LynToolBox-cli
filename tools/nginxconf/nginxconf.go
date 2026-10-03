// Package nginxconf 实现 nginx.conf 生成命令。
// 对应网页版：work/nginx-gen-tool.html（Nginx配置生成器）
package nginxconf

import (
	"flag"
	"fmt"
	"strings"
)

const (
	Name  = "nginxconf"
	Desc  = "按需生成 nginx.conf（反代/SSL/静态/Gzip）"
	Usage = `用法: lyntoolbox nginxconf [-domain example.com] [-port 80] [-proxy http://127.0.0.1:3000]
                      [-root /var/www] [-ssl 证书路径 -sslkey 密钥路径] [-gzip] [-cache]

参数:
  -domain   域名（默认 _）
  -port     HTTP 端口（默认 80）
  -proxy    反向代理上游地址（如 http://127.0.0.1:3000）
  -root     静态站点根目录
  -ssl      SSL 证书路径（配合 -sslkey 生成 443 块与 80 跳转）
  -sslkey   SSL 私钥路径
  -gzip     启用 gzip
  -cache    静态资源缓存头`
)

// Run 执行命令，返回退出码。
func Run(args []string) int {
	fs := flag.NewFlagSet(Name, flag.ContinueOnError)
	domain := fs.String("domain", "_", "域名")
	port := fs.Int("port", 80, "HTTP 端口")
	proxy := fs.String("proxy", "", "反代上游地址")
	root := fs.String("root", "", "静态根目录")
	ssl := fs.String("ssl", "", "SSL 证书路径")
	sslkey := fs.String("sslkey", "", "SSL 私钥路径")
	gzip := fs.Bool("gzip", false, "启用 gzip")
	cache := fs.Bool("cache", false, "静态资源缓存")
	fs.Usage = func() { fmt.Print(Usage + "\n") }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *proxy == "" && *root == "" {
		*root = "/var/www/html"
	}

	var b strings.Builder
	b.WriteString("# 由 LynToolBox 生成（lyntoolbox nginxconf）\n")
	b.WriteString("user  www-data;\nworker_processes  auto;\nevents { worker_connections 1024; }\n\nhttp {\n    include       mime.types;\n    default_type  application/octet-stream;\n    sendfile      on;\n    keepalive_timeout 65;\n\n")
	if *gzip {
		b.WriteString("    gzip on;\n    gzip_types text/plain text/css application/json application/javascript image/svg+xml;\n    gzip_min_length 1k;\n    gzip_comp_level 5;\n\n")
	}

	if *ssl != "" && *sslkey != "" {
		fmt.Fprintf(&b, "    # HTTP 跳转 HTTPS\n    server {\n        listen %d;\n        server_name %s;\n        return 301 https://$host$request_uri;\n    }\n\n", *port, *domain)
		fmt.Fprintf(&b, "    # HTTPS 站点\n    server {\n        listen 443 ssl http2;\n        server_name %s;\n\n        ssl_certificate     %s;\n        ssl_certificate_key %s;\n        ssl_protocols TLSv1.2 TLSv1.3;\n        ssl_session_cache shared:SSL:10m;\n\n", *domain, *ssl, *sslkey)
	} else {
		fmt.Fprintf(&b, "    server {\n        listen %d;\n        server_name %s;\n\n", *port, *domain)
	}

	if *proxy != "" {
		b.WriteString("        location / {\n")
		fmt.Fprintf(&b, "            proxy_pass %s;\n", *proxy)
		b.WriteString("            proxy_set_header Host $host;\n            proxy_set_header X-Real-IP $remote_addr;\n            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n            proxy_set_header X-Forwarded-Proto $scheme;\n")
		if *cache {
			b.WriteString("            proxy_cache_valid 200 10m;\n")
		}
		b.WriteString("        }\n")
	} else {
		fmt.Fprintf(&b, "        root %s;\n        index index.html index.htm;\n\n", *root)
		if *cache {
			b.WriteString("        location ~* \\.(jpg|jpeg|png|gif|ico|css|js|svg|woff2)$ {\n            expires 30d;\n            add_header Cache-Control \"public\";\n        }\n\n")
		}
		b.WriteString("        location / {\n            try_files $uri $uri/ =404;\n        }\n")
	}
	b.WriteString("    }\n}\n")

	fmt.Print(strings.TrimRight(b.String(), "\n") + "\n")
	return 0
}
