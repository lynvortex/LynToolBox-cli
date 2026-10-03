// LynToolBox-cli 绘萤工具箱命令行版。
// 单文件 exe、多子命令；每个工具的实现在 tools/<命令名>/ 下的一个单独源文件中。
// 源码仓库：https://github.com/lynvortex/LynToolBox-cli
package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/lynvortex/LynToolBox-cli/internal/toolreg"

	"github.com/lynvortex/LynToolBox-cli/tools/base58"
	"github.com/lynvortex/LynToolBox-cli/tools/base64"
	"github.com/lynvortex/LynToolBox-cli/tools/cloc"
	"github.com/lynvortex/LynToolBox-cli/tools/codefmt"
	"github.com/lynvortex/LynToolBox-cli/tools/csv2sql"
	"github.com/lynvortex/LynToolBox-cli/tools/csvjson"
	"github.com/lynvortex/LynToolBox-cli/tools/dockerfile"
	"github.com/lynvortex/LynToolBox-cli/tools/docx2md"
	"github.com/lynvortex/LynToolBox-cli/tools/envconv"
	"github.com/lynvortex/LynToolBox-cli/tools/epubx"
	"github.com/lynvortex/LynToolBox-cli/tools/exift"
	"github.com/lynvortex/LynToolBox-cli/tools/extract"
	"github.com/lynvortex/LynToolBox-cli/tools/favicon"
	"github.com/lynvortex/LynToolBox-cli/tools/figlet"
	"github.com/lynvortex/LynToolBox-cli/tools/gitignore"
	"github.com/lynvortex/LynToolBox-cli/tools/gzipx"
	"github.com/lynvortex/LynToolBox-cli/tools/hash"
	"github.com/lynvortex/LynToolBox-cli/tools/hexenc"
	"github.com/lynvortex/LynToolBox-cli/tools/htmlent"
	"github.com/lynvortex/LynToolBox-cli/tools/json2code"
	"github.com/lynvortex/LynToolBox-cli/tools/jsonfmt"
	"github.com/lynvortex/LynToolBox-cli/tools/jsonpath"
	"github.com/lynvortex/LynToolBox-cli/tools/jsonschema"
	"github.com/lynvortex/LynToolBox-cli/tools/lorem"
	"github.com/lynvortex/LynToolBox-cli/tools/maskdata"
	"github.com/lynvortex/LynToolBox-cli/tools/mdconv"
	"github.com/lynvortex/LynToolBox-cli/tools/metatags"
	"github.com/lynvortex/LynToolBox-cli/tools/minify"
	"github.com/lynvortex/LynToolBox-cli/tools/nginxconf"
	"github.com/lynvortex/LynToolBox-cli/tools/phash"
	"github.com/lynvortex/LynToolBox-cli/tools/placeholder"
	"github.com/lynvortex/LynToolBox-cli/tools/qrcode"
	"github.com/lynvortex/LynToolBox-cli/tools/robots"
	"github.com/lynvortex/LynToolBox-cli/tools/shesc"
	"github.com/lynvortex/LynToolBox-cli/tools/slug"
	"github.com/lynvortex/LynToolBox-cli/tools/sortuniq"
	"github.com/lynvortex/LynToolBox-cli/tools/sqlfmt"
	"github.com/lynvortex/LynToolBox-cli/tools/srt"
	"github.com/lynvortex/LynToolBox-cli/tools/stegano"
	"github.com/lynvortex/LynToolBox-cli/tools/tabspace"
	"github.com/lynvortex/LynToolBox-cli/tools/textdiff"
	"github.com/lynvortex/LynToolBox-cli/tools/textrepl"
	"github.com/lynvortex/LynToolBox-cli/tools/tomlconv"
	"github.com/lynvortex/LynToolBox-cli/tools/urlenc"
	"github.com/lynvortex/LynToolBox-cli/tools/xmlfmt"
	"github.com/lynvortex/LynToolBox-cli/tools/yamlconv"
	"github.com/lynvortex/LynToolBox-cli/tools/zhconv"
	"github.com/lynvortex/LynToolBox-cli/tools/zipx"

	"github.com/lynvortex/LynToolBox-cli/tools/pdfop"
	"github.com/lynvortex/LynToolBox-cli/tools/pdftool"
	"github.com/lynvortex/LynToolBox-cli/tools/xlsxtool"

	"github.com/lynvortex/LynToolBox-cli/tools/basecalc"
	"github.com/lynvortex/LynToolBox-cli/tools/bignum"
	"github.com/lynvortex/LynToolBox-cli/tools/calc"
	"github.com/lynvortex/LynToolBox-cli/tools/cronx"
	"github.com/lynvortex/LynToolBox-cli/tools/regext"
	"github.com/lynvortex/LynToolBox-cli/tools/semver"
	"github.com/lynvortex/LynToolBox-cli/tools/snowflake"
	"github.com/lynvortex/LynToolBox-cli/tools/stats"

	"github.com/lynvortex/LynToolBox-cli/tools/barcode"
	"github.com/lynvortex/LynToolBox-cli/tools/bgremove"
	"github.com/lynvortex/LynToolBox-cli/tools/colorcalc"
	"github.com/lynvortex/LynToolBox-cli/tools/datecalc"
	"github.com/lynvortex/LynToolBox-cli/tools/encconv"
	"github.com/lynvortex/LynToolBox-cli/tools/geodist"
	"github.com/lynvortex/LynToolBox-cli/tools/hexdump"
	"github.com/lynvortex/LynToolBox-cli/tools/imgascii"
	"github.com/lynvortex/LynToolBox-cli/tools/imgcolor"
	"github.com/lynvortex/LynToolBox-cli/tools/imgconv"
	"github.com/lynvortex/LynToolBox-cli/tools/imgfilter"
	"github.com/lynvortex/LynToolBox-cli/tools/imggeom"
	"github.com/lynvortex/LynToolBox-cli/tools/imgwm"
	"github.com/lynvortex/LynToolBox-cli/tools/shufflelines"
	"github.com/lynvortex/LynToolBox-cli/tools/svgmin"
	"github.com/lynvortex/LynToolBox-cli/tools/unitconv"
	"github.com/lynvortex/LynToolBox-cli/tools/validate"
	"github.com/lynvortex/LynToolBox-cli/tools/wavtool"
	"github.com/lynvortex/LynToolBox-cli/tools/wordcount"

	"github.com/lynvortex/LynToolBox-cli/tools/aes"
	"github.com/lynvortex/LynToolBox-cli/tools/hmac"
	"github.com/lynvortex/LynToolBox-cli/tools/jwt"
	"github.com/lynvortex/LynToolBox-cli/tools/morse"
	"github.com/lynvortex/LynToolBox-cli/tools/pbkdf2"
	"github.com/lynvortex/LynToolBox-cli/tools/randgen"
	"github.com/lynvortex/LynToolBox-cli/tools/rsagen"
	"github.com/lynvortex/LynToolBox-cli/tools/smcrypto"
	"github.com/lynvortex/LynToolBox-cli/tools/totp"

	"github.com/lynvortex/LynToolBox-cli/tools/dnsq"
	"github.com/lynvortex/LynToolBox-cli/tools/httphead"
	"github.com/lynvortex/LynToolBox-cli/tools/httpreq"
	"github.com/lynvortex/LynToolBox-cli/tools/ipchk"
	"github.com/lynvortex/LynToolBox-cli/tools/macloc"
	"github.com/lynvortex/LynToolBox-cli/tools/mime"
	"github.com/lynvortex/LynToolBox-cli/tools/ping"
	"github.com/lynvortex/LynToolBox-cli/tools/portscan"
	"github.com/lynvortex/LynToolBox-cli/tools/subnet"
	"github.com/lynvortex/LynToolBox-cli/tools/tsconv"
	"github.com/lynvortex/LynToolBox-cli/tools/uaparse"
	"github.com/lynvortex/LynToolBox-cli/tools/urlparse"

	"github.com/lynvortex/LynToolBox-cli/tools/cookiep"
	"github.com/lynvortex/LynToolBox-cli/tools/corschk"
	"github.com/lynvortex/LynToolBox-cli/tools/httpcode"
	"github.com/lynvortex/LynToolBox-cli/tools/mqttcli"
	"github.com/lynvortex/LynToolBox-cli/tools/redirchain"
	"github.com/lynvortex/LynToolBox-cli/tools/sslcert"
	"github.com/lynvortex/LynToolBox-cli/tools/vsrc"
	"github.com/lynvortex/LynToolBox-cli/tools/whois"
	"github.com/lynvortex/LynToolBox-cli/tools/wsclient"
)

func registerAll() {
	toolreg.Register(&toolreg.Tool{Name: base64.Name, Group: gA, Desc: base64.Desc, Usage: base64.Usage, Run: base64.Run})
	toolreg.Register(&toolreg.Tool{Name: hash.Name, Group: gA, Desc: hash.Desc, Usage: hash.Usage, Run: hash.Run})
	toolreg.Register(&toolreg.Tool{Name: urlenc.Name, Group: gA, Desc: urlenc.Desc, Usage: urlenc.Usage, Run: urlenc.Run})
	toolreg.Register(&toolreg.Tool{Name: hexenc.Name, Group: gA, Desc: hexenc.Desc, Usage: hexenc.Usage, Run: hexenc.Run})
	toolreg.Register(&toolreg.Tool{Name: base58.Name, Group: gA, Desc: base58.Desc, Usage: base58.Usage, Run: base58.Run})
	toolreg.Register(&toolreg.Tool{Name: htmlent.Name, Group: gA, Desc: htmlent.Desc, Usage: htmlent.Usage, Run: htmlent.Run})
	toolreg.Register(&toolreg.Tool{Name: gzipx.Name, Group: gA, Desc: gzipx.Desc, Usage: gzipx.Usage, Run: gzipx.Run})
	toolreg.Register(&toolreg.Tool{Name: shesc.Name, Group: gA, Desc: shesc.Desc, Usage: shesc.Usage, Run: shesc.Run})
	toolreg.Register(&toolreg.Tool{Name: morse.Name, Group: gA, Desc: morse.Desc, Usage: morse.Usage, Run: morse.Run})
	toolreg.Register(&toolreg.Tool{Name: aes.Name, Group: gA, Desc: aes.Desc, Usage: aes.Usage, Run: aes.Run})
	toolreg.Register(&toolreg.Tool{Name: smcrypto.Name, Group: gA, Desc: smcrypto.Desc, Usage: smcrypto.Usage, Run: smcrypto.Run})
	toolreg.Register(&toolreg.Tool{Name: hmac.Name, Group: gA, Desc: hmac.Desc, Usage: hmac.Usage, Run: hmac.Run})
	toolreg.Register(&toolreg.Tool{Name: jwt.Name, Group: gA, Desc: jwt.Desc, Usage: jwt.Usage, Run: jwt.Run})
	toolreg.Register(&toolreg.Tool{Name: rsagen.Name, Group: gA, Desc: rsagen.Desc, Usage: rsagen.Usage, Run: rsagen.Run})
	toolreg.Register(&toolreg.Tool{Name: pbkdf2.Name, Group: gA, Desc: pbkdf2.Desc, Usage: pbkdf2.Usage, Run: pbkdf2.Run})
	toolreg.Register(&toolreg.Tool{Name: totp.Name, Group: gA, Desc: totp.Desc, Usage: totp.Usage, Run: totp.Run})
	toolreg.Register(&toolreg.Tool{Name: randgen.Name, Group: gA, Desc: randgen.Desc, Usage: randgen.Usage, Run: randgen.Run})

	toolreg.Register(&toolreg.Tool{Name: jsonfmt.Name, Group: gB, Desc: jsonfmt.Desc, Usage: jsonfmt.Usage, Run: jsonfmt.Run})
	toolreg.Register(&toolreg.Tool{Name: jsonpath.Name, Group: gB, Desc: jsonpath.Desc, Usage: jsonpath.Usage, Run: jsonpath.Run})
	toolreg.Register(&toolreg.Tool{Name: json2code.Name, Group: gB, Desc: json2code.Desc, Usage: json2code.Usage, Run: json2code.Run})
	toolreg.Register(&toolreg.Tool{Name: jsonschema.Name, Group: gB, Desc: jsonschema.Desc, Usage: jsonschema.Usage, Run: jsonschema.Run})
	toolreg.Register(&toolreg.Tool{Name: yamlconv.Name, Group: gB, Desc: yamlconv.Desc, Usage: yamlconv.Usage, Run: yamlconv.Run})
	toolreg.Register(&toolreg.Tool{Name: tomlconv.Name, Group: gB, Desc: tomlconv.Desc, Usage: tomlconv.Usage, Run: tomlconv.Run})
	toolreg.Register(&toolreg.Tool{Name: xmlfmt.Name, Group: gB, Desc: xmlfmt.Desc, Usage: xmlfmt.Usage, Run: xmlfmt.Run})
	toolreg.Register(&toolreg.Tool{Name: csvjson.Name, Group: gB, Desc: csvjson.Desc, Usage: csvjson.Usage, Run: csvjson.Run})
	toolreg.Register(&toolreg.Tool{Name: csv2sql.Name, Group: gB, Desc: csv2sql.Desc, Usage: csv2sql.Usage, Run: csv2sql.Run})
	toolreg.Register(&toolreg.Tool{Name: envconv.Name, Group: gB, Desc: envconv.Desc, Usage: envconv.Usage, Run: envconv.Run})
	toolreg.Register(&toolreg.Tool{Name: sqlfmt.Name, Group: gB, Desc: sqlfmt.Desc, Usage: sqlfmt.Usage, Run: sqlfmt.Run})
	toolreg.Register(&toolreg.Tool{Name: codefmt.Name, Group: gB, Desc: codefmt.Desc, Usage: codefmt.Usage, Run: codefmt.Run})
	toolreg.Register(&toolreg.Tool{Name: minify.Name, Group: gB, Desc: minify.Desc, Usage: minify.Usage, Run: minify.Run})
	toolreg.Register(&toolreg.Tool{Name: cloc.Name, Group: gB, Desc: cloc.Desc, Usage: cloc.Usage, Run: cloc.Run})
	toolreg.Register(&toolreg.Tool{Name: mdconv.Name, Group: gB, Desc: mdconv.Desc, Usage: mdconv.Usage, Run: mdconv.Run})

	toolreg.Register(&toolreg.Tool{Name: textdiff.Name, Group: gC, Desc: textdiff.Desc, Usage: textdiff.Usage, Run: textdiff.Run})
	toolreg.Register(&toolreg.Tool{Name: sortuniq.Name, Group: gC, Desc: sortuniq.Desc, Usage: sortuniq.Usage, Run: sortuniq.Run})
	toolreg.Register(&toolreg.Tool{Name: textrepl.Name, Group: gC, Desc: textrepl.Desc, Usage: textrepl.Usage, Run: textrepl.Run})
	toolreg.Register(&toolreg.Tool{Name: extract.Name, Group: gC, Desc: extract.Desc, Usage: extract.Usage, Run: extract.Run})
	toolreg.Register(&toolreg.Tool{Name: maskdata.Name, Group: gC, Desc: maskdata.Desc, Usage: maskdata.Usage, Run: maskdata.Run})
	toolreg.Register(&toolreg.Tool{Name: tabspace.Name, Group: gC, Desc: tabspace.Desc, Usage: tabspace.Usage, Run: tabspace.Run})
	toolreg.Register(&toolreg.Tool{Name: slug.Name, Group: gC, Desc: slug.Desc, Usage: slug.Usage, Run: slug.Run})
	toolreg.Register(&toolreg.Tool{Name: zhconv.Name, Group: gC, Desc: zhconv.Desc, Usage: zhconv.Usage, Run: zhconv.Run})

	toolreg.Register(&toolreg.Tool{Name: gitignore.Name, Group: gD, Desc: gitignore.Desc, Usage: gitignore.Usage, Run: gitignore.Run})
	toolreg.Register(&toolreg.Tool{Name: robots.Name, Group: gD, Desc: robots.Desc, Usage: robots.Usage, Run: robots.Run})
	toolreg.Register(&toolreg.Tool{Name: nginxconf.Name, Group: gD, Desc: nginxconf.Desc, Usage: nginxconf.Usage, Run: nginxconf.Run})
	toolreg.Register(&toolreg.Tool{Name: dockerfile.Name, Group: gD, Desc: dockerfile.Desc, Usage: dockerfile.Usage, Run: dockerfile.Run})
	toolreg.Register(&toolreg.Tool{Name: metatags.Name, Group: gD, Desc: metatags.Desc, Usage: metatags.Usage, Run: metatags.Run})
	toolreg.Register(&toolreg.Tool{Name: lorem.Name, Group: gD, Desc: lorem.Desc, Usage: lorem.Usage, Run: lorem.Run})
	toolreg.Register(&toolreg.Tool{Name: figlet.Name, Group: gD, Desc: figlet.Desc, Usage: figlet.Usage, Run: figlet.Run})
	toolreg.Register(&toolreg.Tool{Name: qrcode.Name, Group: gD, Desc: qrcode.Desc, Usage: qrcode.Usage, Run: qrcode.Run})

	toolreg.Register(&toolreg.Tool{Name: ping.Name, Group: gE, Desc: ping.Desc, Usage: ping.Usage, Run: ping.Run})
	toolreg.Register(&toolreg.Tool{Name: portscan.Name, Group: gE, Desc: portscan.Desc, Usage: portscan.Usage, Run: portscan.Run})
	toolreg.Register(&toolreg.Tool{Name: httpreq.Name, Group: gE, Desc: httpreq.Desc, Usage: httpreq.Usage, Run: httpreq.Run})
	toolreg.Register(&toolreg.Tool{Name: httphead.Name, Group: gE, Desc: httphead.Desc, Usage: httphead.Usage, Run: httphead.Run})
	toolreg.Register(&toolreg.Tool{Name: dnsq.Name, Group: gE, Desc: dnsq.Desc, Usage: dnsq.Usage, Run: dnsq.Run})
	toolreg.Register(&toolreg.Tool{Name: subnet.Name, Group: gE, Desc: subnet.Desc, Usage: subnet.Usage, Run: subnet.Run})
	toolreg.Register(&toolreg.Tool{Name: ipchk.Name, Group: gE, Desc: ipchk.Desc, Usage: ipchk.Usage, Run: ipchk.Run})
	toolreg.Register(&toolreg.Tool{Name: urlparse.Name, Group: gE, Desc: urlparse.Desc, Usage: urlparse.Usage, Run: urlparse.Run})
	toolreg.Register(&toolreg.Tool{Name: uaparse.Name, Group: gE, Desc: uaparse.Desc, Usage: uaparse.Usage, Run: uaparse.Run})
	toolreg.Register(&toolreg.Tool{Name: mime.Name, Group: gE, Desc: mime.Desc, Usage: mime.Usage, Run: mime.Run})
	toolreg.Register(&toolreg.Tool{Name: macloc.Name, Group: gE, Desc: macloc.Desc, Usage: macloc.Usage, Run: macloc.Run})
	toolreg.Register(&toolreg.Tool{Name: tsconv.Name, Group: gE, Desc: tsconv.Desc, Usage: tsconv.Usage, Run: tsconv.Run})

	toolreg.Register(&toolreg.Tool{Name: redirchain.Name, Group: gG, Desc: redirchain.Desc, Usage: redirchain.Usage, Run: redirchain.Run})
	toolreg.Register(&toolreg.Tool{Name: sslcert.Name, Group: gG, Desc: sslcert.Desc, Usage: sslcert.Usage, Run: sslcert.Run})
	toolreg.Register(&toolreg.Tool{Name: whois.Name, Group: gG, Desc: whois.Desc, Usage: whois.Usage, Run: whois.Run})
	toolreg.Register(&toolreg.Tool{Name: wsclient.Name, Group: gG, Desc: wsclient.Desc, Usage: wsclient.Usage, Run: wsclient.Run})
	toolreg.Register(&toolreg.Tool{Name: mqttcli.Name, Group: gG, Desc: mqttcli.Desc, Usage: mqttcli.Usage, Run: mqttcli.Run})
	toolreg.Register(&toolreg.Tool{Name: vsrc.Name, Group: gG, Desc: vsrc.Desc, Usage: vsrc.Usage, Run: vsrc.Run})
	toolreg.Register(&toolreg.Tool{Name: httpcode.Name, Group: gG, Desc: httpcode.Desc, Usage: httpcode.Usage, Run: httpcode.Run})
	toolreg.Register(&toolreg.Tool{Name: cookiep.Name, Group: gG, Desc: cookiep.Desc, Usage: cookiep.Usage, Run: cookiep.Run})
	toolreg.Register(&toolreg.Tool{Name: corschk.Name, Group: gG, Desc: corschk.Desc, Usage: corschk.Usage, Run: corschk.Run})

	toolreg.Register(&toolreg.Tool{Name: basecalc.Name, Group: gF, Desc: basecalc.Desc, Usage: basecalc.Usage, Run: basecalc.Run})
	toolreg.Register(&toolreg.Tool{Name: bignum.Name, Group: gF, Desc: bignum.Desc, Usage: bignum.Usage, Run: bignum.Run})
	toolreg.Register(&toolreg.Tool{Name: calc.Name, Group: gF, Desc: calc.Desc, Usage: calc.Usage, Run: calc.Run})
	toolreg.Register(&toolreg.Tool{Name: snowflake.Name, Group: gF, Desc: snowflake.Desc, Usage: snowflake.Usage, Run: snowflake.Run})
	toolreg.Register(&toolreg.Tool{Name: semver.Name, Group: gF, Desc: semver.Desc, Usage: semver.Usage, Run: semver.Run})
	toolreg.Register(&toolreg.Tool{Name: regext.Name, Group: gF, Desc: regext.Desc, Usage: regext.Usage, Run: regext.Run})
	toolreg.Register(&toolreg.Tool{Name: cronx.Name, Group: gF, Desc: cronx.Desc, Usage: cronx.Usage, Run: cronx.Run})
	toolreg.Register(&toolreg.Tool{Name: stats.Name, Group: gF, Desc: stats.Desc, Usage: stats.Usage, Run: stats.Run})

	toolreg.Register(&toolreg.Tool{Name: wordcount.Name, Group: gJ, Desc: wordcount.Desc, Usage: wordcount.Usage, Run: wordcount.Run})
	toolreg.Register(&toolreg.Tool{Name: shufflelines.Name, Group: gJ, Desc: shufflelines.Desc, Usage: shufflelines.Usage, Run: shufflelines.Run})
	toolreg.Register(&toolreg.Tool{Name: encconv.Name, Group: gJ, Desc: encconv.Desc, Usage: encconv.Usage, Run: encconv.Run})
	toolreg.Register(&toolreg.Tool{Name: hexdump.Name, Group: gJ, Desc: hexdump.Desc, Usage: hexdump.Usage, Run: hexdump.Run})
	toolreg.Register(&toolreg.Tool{Name: validate.Name, Group: gJ, Desc: validate.Desc, Usage: validate.Usage, Run: validate.Run})
	toolreg.Register(&toolreg.Tool{Name: datecalc.Name, Group: gJ, Desc: datecalc.Desc, Usage: datecalc.Usage, Run: datecalc.Run})
	toolreg.Register(&toolreg.Tool{Name: unitconv.Name, Group: gJ, Desc: unitconv.Desc, Usage: unitconv.Usage, Run: unitconv.Run})
	toolreg.Register(&toolreg.Tool{Name: colorcalc.Name, Group: gJ, Desc: colorcalc.Desc, Usage: colorcalc.Usage, Run: colorcalc.Run})
	toolreg.Register(&toolreg.Tool{Name: geodist.Name, Group: gJ, Desc: geodist.Desc, Usage: geodist.Usage, Run: geodist.Run})
	toolreg.Register(&toolreg.Tool{Name: wavtool.Name, Group: gJ, Desc: wavtool.Desc, Usage: wavtool.Usage, Run: wavtool.Run})
	toolreg.Register(&toolreg.Tool{Name: svgmin.Name, Group: gJ, Desc: svgmin.Desc, Usage: svgmin.Usage, Run: svgmin.Run})

	toolreg.Register(&toolreg.Tool{Name: imgconv.Name, Group: gI, Desc: imgconv.Desc, Usage: imgconv.Usage, Run: imgconv.Run})
	toolreg.Register(&toolreg.Tool{Name: imgwm.Name, Group: gI, Desc: imgwm.Desc, Usage: imgwm.Usage, Run: imgwm.Run})
	toolreg.Register(&toolreg.Tool{Name: exift.Name, Group: gI, Desc: exift.Desc, Usage: exift.Usage, Run: exift.Run})
	toolreg.Register(&toolreg.Tool{Name: imggeom.Name, Group: gI, Desc: imggeom.Desc, Usage: imggeom.Usage, Run: imggeom.Run})
	toolreg.Register(&toolreg.Tool{Name: imgfilter.Name, Group: gI, Desc: imgfilter.Desc, Usage: imgfilter.Usage, Run: imgfilter.Run})
	toolreg.Register(&toolreg.Tool{Name: phash.Name, Group: gI, Desc: phash.Desc, Usage: phash.Usage, Run: phash.Run})
	toolreg.Register(&toolreg.Tool{Name: imgascii.Name, Group: gI, Desc: imgascii.Desc, Usage: imgascii.Usage, Run: imgascii.Run})
	toolreg.Register(&toolreg.Tool{Name: stegano.Name, Group: gI, Desc: stegano.Desc, Usage: stegano.Usage, Run: stegano.Run})
	toolreg.Register(&toolreg.Tool{Name: imgcolor.Name, Group: gI, Desc: imgcolor.Desc, Usage: imgcolor.Usage, Run: imgcolor.Run})
	toolreg.Register(&toolreg.Tool{Name: placeholder.Name, Group: gI, Desc: placeholder.Desc, Usage: placeholder.Usage, Run: placeholder.Run})
	toolreg.Register(&toolreg.Tool{Name: favicon.Name, Group: gI, Desc: favicon.Desc, Usage: favicon.Usage, Run: favicon.Run})
	toolreg.Register(&toolreg.Tool{Name: barcode.Name, Group: gI, Desc: barcode.Desc, Usage: barcode.Usage, Run: barcode.Run})
	toolreg.Register(&toolreg.Tool{Name: bgremove.Name, Group: gI, Desc: bgremove.Desc, Usage: bgremove.Usage, Run: bgremove.Run})

	toolreg.Register(&toolreg.Tool{Name: zipx.Name, Group: gH, Desc: zipx.Desc, Usage: zipx.Usage, Run: zipx.Run})
	toolreg.Register(&toolreg.Tool{Name: srt.Name, Group: gH, Desc: srt.Desc, Usage: srt.Usage, Run: srt.Run})
	toolreg.Register(&toolreg.Tool{Name: docx2md.Name, Group: gH, Desc: docx2md.Desc, Usage: docx2md.Usage, Run: docx2md.Run})
	toolreg.Register(&toolreg.Tool{Name: epubx.Name, Group: gH, Desc: epubx.Desc, Usage: epubx.Usage, Run: epubx.Run})
	toolreg.Register(&toolreg.Tool{Name: xlsxtool.Name, Group: gH, Desc: xlsxtool.Desc, Usage: xlsxtool.Usage, Run: xlsxtool.Run})
	toolreg.Register(&toolreg.Tool{Name: pdftool.Name, Group: gH, Desc: pdftool.Desc, Usage: pdftool.Usage, Run: pdftool.Run})
	toolreg.Register(&toolreg.Tool{Name: pdfop.Name, Group: gH, Desc: pdfop.Desc, Usage: pdfop.Usage, Run: pdfop.Run})
}

const (
	gA = "A 编码·加密·哈希"
	gB = "B 数据格式"
	gC = "C 文本批处理"
	gD = "D 生成器"
	gE = "E 网络工具"
	gF = "F 计算·校验"
	gG = "G 网络补充"
	gH = "H 文件·文档"
	gI = "I 图片批处理"
	gJ = "J 文本·计算补充"
)

var version = "1.0.0"

func groupOrder(g string) int {
	names := []string{gA, gB, gC, gD, gE, gF, gG, gH, gI, gJ}
	for i, n := range names {
		if n == g {
			return i
		}
	}
	return len(names)
}

func printList() {
	byGroup := map[string][]*toolreg.Tool{}
	for _, t := range toolreg.All {
		byGroup[t.Group] = append(byGroup[t.Group], t)
	}
	groups := make([]string, 0, len(byGroup))
	for g := range byGroup {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool { return groupOrder(groups[i]) < groupOrder(groups[j]) })

	fmt.Printf("绘萤工具箱命令行版 v%s — 共 %d 个工具\n\n", version, len(toolreg.All))
	fmt.Println("用法: lyntoolbox <命令名> [参数]    查看某命令详情: lyntoolbox <命令名> -h")
	fmt.Println()
	for _, g := range groups {
		fmt.Printf("【%s】\n", g)
		for _, t := range byGroup[g] {
			fmt.Printf("  %-12s %s\n", t.Name, t.Desc)
		}
		fmt.Println()
	}
	fmt.Println("项目地址: https://github.com/lynvortex/LynToolBox-cli")
}

func main() {
	initConsole()
	registerAll()
	if len(os.Args) < 2 {
		// 无参数：交互式菜单模式（双击 exe 或直接运行）
		os.Exit(runInteractive())
	}
	name := os.Args[1]
	if name == "-v" || name == "--version" {
		fmt.Println("lyntoolbox v" + version)
		return
	}
	if name == "help" || name == "list" || name == "--help" || name == "-h" {
		if len(os.Args) > 2 {
			for _, t := range toolreg.All {
				if t.Name == os.Args[2] {
					fmt.Println(strings.TrimRight(t.Usage, "\n"))
					return
				}
			}
			fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", strings.TrimSpace(os.Args[2]))
			printList()
			os.Exit(2)
		}
		printList()
		return
	}
	for _, t := range toolreg.All {
		if t.Name == name {
			os.Exit(dispatch(t, os.Args[2:]))
			return
		}
	}
	fmt.Fprintf(os.Stderr, "未知命令: %s\n\n", strings.TrimSpace(name))
	printList()
	os.Exit(2)
}

// dispatch 兼容"位置参数在前、选项在后"的书写习惯：
// 检测到乱序时按"选项提前"重排静默执行（探测），子命令式工具再尝试保持首参的变体，
// 均失败则按原始顺序正式执行以给出标准错误信息。
// 探测必须先行：hash 等工具会把多余位置参数拼接为输入（原始顺序"假成功"），
// 只有让合法的重排先执行才能得到正确语义。副作用方面是安全的：
// 约定退出码 2（用法错误）发生在任何副作用之前，因此失败探测不产生效果，
// 成功路径从第一个探测成功即返回，实际产生副作用的执行最多一次。
func dispatch(t *toolreg.Tool, args []string) int {
	if hasDisorder(args) {
		alt := partition(args)
		if !equalArgs(alt, args) {
			if code, out, errS := runQuiet(t, alt); code != 2 {
				fmt.Print(out)
				fmt.Fprint(os.Stderr, errS)
				return code
			}
		}
		if len(args) > 1 && !strings.HasPrefix(args[0], "-") {
			alt2 := append([]string{args[0]}, partition(args[1:])...)
			if !equalArgs(alt2, args) {
				if code, out, errS := runQuiet(t, alt2); code != 2 {
					fmt.Print(out)
					fmt.Fprint(os.Stderr, errS)
					return code
				}
			}
		}
	}
	return t.Run(args)
}

// runQuiet 静默执行一次命令，捕获其 stdout/stderr。
// 仅在 main 启动早期、任何工具 goroutine 尚未创建时调用。
func runQuiet(t *toolreg.Tool, args []string) (int, string, string) {
	rOut, wOut, err1 := os.Pipe()
	rErr, wErr, err2 := os.Pipe()
	if err1 != nil || err2 != nil {
		return t.Run(args), "", ""
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wOut, wErr
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	code := t.Run(args)
	// 先关写端再读，避免 io.ReadAll 因未关闭的管道阻塞
	wOut.Close()
	wErr.Close()
	out, _ := io.ReadAll(rOut)
	errS, _ := io.ReadAll(rErr)
	os.Stdout, os.Stderr = oldOut, oldErr
	return code, string(out), string(errS)
}

func equalArgs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// hasDisorder 判断是否存在"位置参数之后再出现选项"的乱序。
func hasDisorder(args []string) bool {
	seenPos := false
	for _, a := range args {
		if a == "--" {
			return false
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			if seenPos {
				return true
			}
		} else {
			seenPos = true
		}
	}
	return false
}

// partition 稳定重排：选项（连同其值）提前，位置参数保持相对顺序在后。
func partition(args []string) []string {
	var flags, pos []string
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			flags = append(flags, a)
			i++
			if i < len(args) && !strings.HasPrefix(args[i], "-") && args[i] != "--" {
				flags = append(flags, args[i])
				i++
			}
		} else {
			pos = append(pos, a)
			i++
		}
	}
	return append(flags, pos...)
}
