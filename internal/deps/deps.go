// Package deps 通过空白导入锁定全部第三方依赖，
// 保证 go.mod / go.sum 在并行开发期间保持稳定。
// 各工具只允许使用标准库与本文件列出的依赖。
package deps

import (
	_ "github.com/BurntSushi/toml"
	_ "github.com/boombuler/barcode"
	_ "github.com/boombuler/barcode/code128"
	_ "github.com/boombuler/barcode/ean"
	_ "github.com/boombuler/barcode/qr"
	_ "github.com/eclipse/paho.mqtt.golang"
	_ "github.com/gomarkdown/markdown"
	_ "github.com/gorilla/websocket"
	_ "github.com/longbridgeapp/opencc"
	_ "github.com/makiuchi-d/gozxing"
	_ "github.com/makiuchi-d/gozxing/qrcode"
	_ "github.com/mozillazg/go-pinyin"
	_ "github.com/pdfcpu/pdfcpu/pkg/api"
	_ "github.com/rwcarlsen/goexif/exif"
	_ "github.com/skip2/go-qrcode"
	_ "github.com/tjfoc/gmsm/sm2"
	_ "github.com/tjfoc/gmsm/sm3"
	_ "github.com/tjfoc/gmsm/sm4"
	_ "github.com/xuri/excelize/v2"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
	_ "golang.org/x/net/html"
	_ "golang.org/x/text/encoding/simplifiedchinese"
	_ "golang.org/x/text/encoding/traditionalchinese"
	_ "golang.org/x/text/encoding/unicode"
	_ "gopkg.in/yaml.v3"
)
