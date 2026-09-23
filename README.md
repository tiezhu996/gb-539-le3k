# KilnCurve 木材窑干燥优化台

KilnCurve 是面向木材加工企业干燥工艺团队的离线决策支持工作台。它维护窑炉安全边界、工艺批次和含水率读数，基于可复核规则生成干燥曲线仿真建议。系统不会连接或控制窑炉、风机、蒸汽阀或喷淋设备，也不包含订单、库存、财务或社区功能。

## 立即启动

```bash
cp .env.example .env
docker compose up -d --build
```

启动后在内置浏览器访问 `http://127.0.0.1:18539` 。

演示账号：`admin@kilncurve.local / admin123`。审核账号：`reviewer@kilncurve.local / reviewer123`。其他角色还有 `kiln_engineer`、`quality_analyst`、`auditor`，密码分别为 `engineer123`、`analyst123`、`auditor123`。

## 功能与页面

- `/kilns`：窑炉档案、安全边界和编辑操作。
- `/lots`：登记木材工艺批次，按 `queued -> conditioning -> drying -> equalizing -> completed` 迁移，任意运行阶段可中止。
- `/readings`：导入含水率和干湿球读数，展示平均含水率曲线。
- `/schedules`：按树种、厚度、读数和窑炉快照计算建议，展示风险、规则证据和人工审核。读数补录、作废或修正会自动把同批次旧计划标为 `superseded`（过期），冻结旧计划仅保留快照；过期计划不能再审核或冻结；重新计算的计划指向被替代的旧计划。批次从均衡进入完成前要求最新计划已冻结。两个页面展示过期原因（读数补录/作废/修正）与替代关系。
- `/audit`：按 request ID、实体和操作者查看前后快照。

共享组件为 `MoistureStageBadge`、`DryingCurveChart`、`RuleEvidenceDrawer`；曲线计算交互封装在 `useScheduleSimulation`。四个实体在数据库、Go model/dto/repository/service/handler/router 与 Angular type/api/store/page 中均保持独立文件。

## 架构

前端为 Angular 17 依赖 + Vite 工作台，使用 TypeScript、Angular Signals、Angular Material 依赖和 ECharts-ready 曲线组件；Nginx 提供 SPA 与 `/api/v1` 反向代理。后端为 Go 1.22、Gin、GORM、JWT；生产数据库 PostgreSQL 16，runtime smoke 支持 SQLite。API 前缀为 `/api/v1`，健康检查为 `/healthz` 与代理路径 `/api/healthz`。

```text
backend/cmd/server       HTTP 入口、优雅停机
backend/internal/model   GORM 实体和迁移
backend/internal/repository  四实体独立持久化
backend/internal/service 事务边界、状态机、审计
backend/internal/algorithm 阶段、梯度、风险和规则建议
backend/internal/handler + router + middleware
frontend/src/api stores types components/common hooks pages router utils
database/init.sql        PostgreSQL 初始化
```

## 安全与算法边界

JWT + RBAC 角色为 `admin`、`kiln_engineer`、`quality_analyst`、`reviewer`、`auditor`。计划创建者不能接受自己的建议；写操作记录 request ID、操作者和前后快照；无效读数返回 422、非法状态/版本返回 409、越权返回 403。算法版本为 `curve-v2.0`，同一输入哈希幂等复用，建议永远限制在窑炉边界内。阶段枚举 `green | fiber_saturation | bound_water | target` 位于 `backend/internal/constants` 与 `frontend/src/types/enums`；批次状态枚举位于同处，README 与各 model、dto、algorithm、service、store、组件和页面共享。

## 环境、端口与本地开发

`.env.example` 包含 `COMPOSE_PROJECT_NAME=timber-kiln-drying-optimizer`、端口 `18539/19539/57539`、PostgreSQL 和 JWT 配置；不要提交 `.env`。本地后端：`PORT=20539 DB_DRIVER=sqlite DB_DSN='file:runtime-smoke?mode=memory&cache=shared' JWT_SECRET=runtime-smoke-only-change-me go run ./cmd/server`。本地前端：`npm --prefix frontend ci && npm --prefix frontend run dev`。

## 验证

```bash
go work sync
go build ./backend/...
go vet ./backend/...
go test ./backend/...
go test -race ./backend/...
npm --prefix frontend ci
npm --prefix frontend run build
npm --prefix frontend run typecheck


docker compose config --quiet
./scripts/api_smoke.sh
```

Browser 验证仅使用 Codex 内置 Browser：登录后依次检查五个页面，创建/编辑窑炉、登记批次、导入读数、迁移状态、运行仿真并检查 console/network、移动端溢出。停止服务：`docker compose down -v --remove-orphans`。

## License

内部工艺工具示例，按组织内部许可使用。
