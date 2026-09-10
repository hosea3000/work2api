// Package web 将管理台前端构建产物嵌入二进制。
// 构建顺序：cd web && pnpm install && pnpm run build:bundle → 生成 web/dist → go build。
// 若 dist 缺失，编译报错提示先构建前端。
package web

import "embed"

//go:embed all:dist
var DistFS embed.FS
