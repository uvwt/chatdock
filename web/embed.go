package webui

import "embed"

// Dist 是生产构建后的前端静态资源。Go 构建前必须先执行 web 构建生成 web/dist。
//
//go:embed dist
var Dist embed.FS
