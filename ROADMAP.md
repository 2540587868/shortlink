# Roadmap: ShortLink 短链接服务

> 最后更新: 2026-05-16 | 版本: v0.3

## 项目现状

- 代码文件: 21 个 (.go: 14 个, _test.go: 7 个)
- 测试覆盖率: 8 个内部包中 6 个有测试 (75%)
- 已知问题: 0 个
- 技术债: 0 项
- 架构: Snowflake + Base62 slug → Bloom Filter → 分片 LRU 缓存 → SQLite 存储 → 异步统计 → Prometheus 监控

## 🔴 P0 — 紧急（立即处理）

| # | 类别 | 描述 | 影响 | 工作量 | 关联文件 |
|---|------|------|------|--------|----------|
| 1 | 🔧 techdebt | **API 层 HTTP Handler 测试** — `router.go` 486 行零测试，服务入口无保障 | 所有 API 行为无回归保护，重构和扩展风险极高 | 半天-1天 | `internal/api/router.go` | ✅ 已完成

## 🟠 P1 — 高优先级（本版本）

| # | 类别 | 描述 | 影响 | 工作量 | 关联文件 |
|---|------|------|------|--------|----------|

## 🟡 P2 — 中优先级（下个版本）

| # | 类别 | 描述 | 影响 | 工作量 | 关联文件 |
|---|------|------|------|--------|----------|
| 2 | 🚀 ops | **CI/CD Pipeline** — 添加 GitHub Actions (lint + test + build + push) | 无自动化质检，PR 依赖人工 review | 1-4小时 | `.github/workflows/ci.yml` | ✅ 已完成
| 3 | 🚀 ops | **配置热加载接入** — `Reload()` 已实现但未接入 SIGHUP 监听 | 每次配置变更需重启服务 | 1-4小时 | `cmd/shortlink/main.go`, `internal/config/config.go` | ✅ 已完成
| 4 | ✨ feature | **短链接密码验证** — 密码确认页 + 验证 API + Cookie 会话 | 隐私链接无法保护，功能不完整 | 1-4小时 | `internal/core/service.go`, `internal/api/router.go` | ✅ 已完成
| 5 | 📊 observability | **Prometheus Metrics** — 暴露 HTTP 请求计数/延迟/状态码/队列积压 | 无运行时指标，无法监控服务状态 | 半天-1天 | `internal/api/router.go`, `internal/metrics/` | ✅ 已完成
| 6 | 📊 observability | **pprof 调试端点** — 注册 `/debug/pprof/` 到独立端口 | 无内存/goroutine 分析能力，问题排查困难 | <1小时 | `cmd/shortlink/main.go` | ✅ 已完成

## 🔵 P3 — 低优先级（排期待定）

| # | 类别 | 描述 | 影响 | 工作量 | 关联文件 |
|---|------|------|------|--------|----------|
| 7 | 🔧 techdebt | **config 包测试** — config 加载/热更新/默认值测试 | 配置错误可能导致运行时异常 | 1-4小时 | `internal/config/config.go` | ✅ 已完成
| 8 | 🔧 techdebt | **API Benchmark 测试** — 重定向热路径、创建短链的性能基线 | 无性能基准，优化无从衡量 | <1小时 | `internal/api/` | ✅ 已完成
| 9 | 🐛 bug | **writeJSON 错误忽略** — json.NewEncoder 返回值未检查 | 写入失败时静默丢失响应体 | <1小时 | `internal/api/router.go` | ✅ 已完成

## ⚪ P4 — 可选（有空再做）

| # | 类别 | 描述 | 影响 | 工作量 | 关联文件 |
|---|------|------|------|--------|----------|
| 10 | ✨ feature | **Admin SPA 管理后台** — Vite + TypeScript 内嵌管理界面 | 当前仅 API 操作，无图形化管理入口 | 1周+ | `web/admin/` | ⏭️ 跳过 (工作量大，延后到 v0.4)
| 11 | ⚡ perf | **Bloom Filter 防 slug 碰撞** — 创建短链时快速排除已用 slug | 减少 DB 查询，对于万级短链有明显收益 | 1-4小时 | `internal/bloom/` | ✅ 已完成
| 12 | ✨ feature | **二维码生成** — POST 创建时附带 `generate_qr: true` 返回二维码 base64 PNG | 移动端分享场景高频需求 | 1-4小时 | `internal/core/`, `internal/api/` | ✅ 已完成
| 13 | ✨ feature | **点击来源 Top N 排名** — 统计 API 返回 Top Referer / Top UA | 统计功能当前仅计数，无分析能力 | 1-4小时 | `internal/store/store.go`, `internal/api/router.go` | ✅ 已完成

## 版本规划

| 版本 | 目标 | 包含项目 | 状态 |
|------|------|----------|------|
| v0.1 | MVP — 核心短链 CRUD + 重定向 + 统计 + 缓存 | 全部基础功能 | ✅ 已完成 |
| v0.2 | 稳定版 — 测试覆盖 + CI/CD + 热加载 + 密码保护 + 可观测 | #1, #2, #3, #4, #5, #6, #7, #8, #9 | ✅ 已完成 |
| v0.3 | 体验版 — Bloom Filter + QR Code + Top N 来源分析 | #11, #12, #13 | ✅ 已完成 |
| v0.4 | 可视化版 — Admin SPA 管理后台 | #10 | ⬜ 计划中 |

## 变更记录

| 日期 | 变更 |
|------|------|
| 2026-05-15 | 初始创建，基于项目代码五维分析模型扫描生成 |
| 2026-05-16 | ✅ P0#1 API 测试完成 (32 个测试覆盖所有 handler) |
| 2026-05-16 | ✅ P2#2 CI/CD Pipeline (GitHub Actions + Docker) |
| 2026-05-16 | ✅ P2#3 配置热加载 (SIGHUP 信号监听) |
| 2026-05-16 | ✅ P2#4 密码验证 (密码确认页 + Cookie 会话 + 验证 API) |
| 2026-05-16 | ✅ P2#5 Prometheus Metrics (请求计数/延迟/短链/重定向/队列) |
| 2026-05-16 | ✅ P2#6 pprof 调试端点 (独立端口) |
| 2026-05-16 | ✅ P3#7 config 包测试 (加载/热更新/默认值) |
| 2026-05-16 | ✅ P3#8 API Benchmark (重定向/创建/健康检查) |
| 2026-05-16 | ✅ P3#9 writeJSON 错误检查 (slog.Error) |
| 2026-05-16 | ⏭️ P4#10 Admin SPA 跳过 (工作量 1周+, 延后到 v0.4) |
| 2026-05-16 | ✅ P4#11 Bloom Filter (slug 碰撞快速排除) |
| 2026-05-16 | ✅ P4#12 QR Code 生成 (base64 PNG) |
| 2026-05-16 | ✅ P4#13 Top N 来源分析 (Top Referers + Top User Agents) |