// Package api 存放 HTTP 契约：openapi.yaml。
//
// 契约随代码一起演进；修改路由或响应 schema 后请同步更新 openapi.yaml，
// 然后跑 `bazel test //backend/api:api_test`（或 `go test ./backend/api/...`）
// 以确保 YAML 解析、关键路径与 $ref 完整性仍成立。
//
// 前端类型生成：
//   pnpm --filter @nexusapi/shared run api:gen
package api
