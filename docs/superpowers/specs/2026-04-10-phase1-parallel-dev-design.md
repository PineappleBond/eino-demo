# Phase 1 前后端并行开发设计

> **日期**: 2026-04-10
> **状态**: 待审批
> **范围**: Phase 1 — MVP: Core Chat + Basic Agent

## 概述

两人独立分工，前后端并行开发 Phase 1 全部功能。OpenAPI spec 为唯一真理来源，`openapi/generate.sh` 生成 Go 和 TypeScript 类型，确保接口一致。

## 协作架构

```
OpenAPI spec ──→ generate.sh ──→ Go types + TS types
                                      ↓           ↓
                                后端开发        前端开发
                                (独立)          (独立)
                                      ↓           ↓
                                HTTP endpoints ← API 调用
                                WebSocket push ← WS 连接
                                      ↓           ↓
                                联调验证 (curl + 浏览器)
```

**交接契约**（不变的文档）：
- `openapi/spec.yaml` — API 路径、请求/响应格式
- `docs/API.md` — 通信流程、Update 类型、错误处理
- `docs/DATABASE_POSTGRESQL.md` — 后端数据库 schema
- `docs/DATABASE_INDEXEDDB.md` — 前端 IndexedDB schema

只要 spec.yaml 不变，前后端可以完全独立开发。

## 开发环境

### 基础设施（Docker）

- PostgreSQL（含 pgvector 扩展）
- Redis
- 通过 `docker-compose.yml` 统一管理

### 环境变量（`.env.example`）

```
# OpenAI 兼容模型配置（三档，每档独立 BASE_URL + API_KEY + MODEL）
MODEL_HAIKU_BASE_URL=https://...
MODEL_HAIKU_API_KEY=...
MODEL_HAIKU_MODEL=gpt-4o-mini

MODEL_SONNET_BASE_URL=https://...
MODEL_SONNET_API_KEY=...
MODEL_SONNET_MODEL=gpt-4o

MODEL_OPUS_BASE_URL=https://...
MODEL_OPUS_API_KEY=...
MODEL_OPUS_MODEL=o1

# PostgreSQL
DATABASE_URL=postgres://user:pass@localhost:5432/eino_demo?sslmode=disable

# Redis
REDIS_ADDR=localhost:6379

# Server
SERVER_PORT=8080
```

## 阶段 0：公共基础设施（串行，1人完成）

### 1. `openapi/generate.sh`

从 `openapi/spec.yaml` 生成双向类型：
- **Go**: 使用 `oapi-codegen` → `server/internal/types/types.go`
- **TypeScript**: 使用 `openapi-typescript` → `web/src/types/api.d.ts`

依赖安装：
- Go: `github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen`
- Node: `openapi-typescript` (npm 包)

### 2. `docker-compose.yml`

```yaml
services:
  postgres:
    image: pgvector/pgvector:pg16
    ports: ["5432:5432"]
    environment:
      POSTGRES_USER: eino_user
      POSTGRES_PASSWORD: eino_pass
      POSTGRES_DB: eino_demo
    volumes: ["postgres_data:/var/lib/postgresql/data"]

  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]

volumes:
  postgres_data:
```

### 3. `.env.example`

如上所示的环境变量模板。

### 4. 运行类型生成

运行 `./openapi/generate.sh`，验证生成文件存在且无错误。

---

完成后，通知前后端开发者开始并行工作。

## 阶段 1：后端骨架（并行起点 A）

### 文件结构

```
server/
├── cmd/server/main.go          # 入口：flag + fx.New().Run()
├── go.mod
├── internal/
│   ├── config/config.go        # 配置：flag + env，三档模型
│   ├── di/
│   │   ├── module.go           # Fx 模块
│   │   └── logger.go           # Zap logger
│   ├── db/
│   │   ├── db.go               # GORM + AutoMigrate
│   │   └── pgvector.go         # pgvector 扩展
│   ├── model/                  # GORM 模型（6个文件）
│   │   ├── base.go             # BaseModel: ID, CreatedAt, UpdatedAt
│   │   ├── user.go             # User
│   │   ├── project.go          # Project
│   │   ├── conversation.go     # Conversation
│   │   ├── message.go          # Message
│   │   └── user_update.go      # UserUpdate (seq 日志)
│   ├── service/                # 业务逻辑（4个文件）
│   │   ├── user.go             # 用户服务
│   │   ├── project.go          # 项目 + 模板服务
│   │   ├── conversation.go     # 对话服务
│   │   └── chat.go             # 聊天 + Eino Agent 执行
│   ├── handler/                # HTTP 处理器（4个文件）
│   │   ├── http.go             # Gin 路由
│   │   ├── auth.go             # 认证中间件
│   │   ├── user.go             # 用户端点
│   │   ├── project.go          # 项目端点
│   │   ├── conversation.go     # 对话端点
│   │   └── chat.go             # 聊天端点
│   ├── ws/                     # WebSocket（4个文件）
│   │   ├── server.go           # WS handler
│   │   ├── manager.go          # 连接管理
│   │   ├── queue.go            # seq 队列
│   │   └── protocol.go         # 协议定义
│   ├── eino/                   # Eino 集成
│   │   ├── model.go            # 模型工厂（三档）
│   │   ├── tools/              # 工具注册
│   │   │   ├── registry.go
│   │   │   └── weather.go      # 示例工具
│   │   └── agents/
│   │       ├── registry.go
│   │       └── basic.go        # Template 01 Agent
│   └── templates/              # 模板注册
│       ├── registry.go
│       └── 01_basic_agent.go
```

### 后端依赖清单

| 库 | 用途 |
|---|---|
| `github.com/gin-gonic/gin` | HTTP 路由 |
| `github.com/coder/websocket` | WebSocket |
| `go.uber.org/fx` | 依赖注入 |
| `gorm.io/gorm` + `gorm.io/driver/postgres` | ORM |
| `github.com/redis/go-redis/v9` | Redis 客户端 |
| `go.uber.org/zap` | 日志 |
| `github.com/cloudwego/eino` | AI 框架 |
| `github.com/cloudwego/eino-ext` | Eino 扩展 |
| `github.com/oapi-codegen/runtime` | OpenAPI 运行时 |

### 后端开发顺序

1. **config + db + model** — 数据库连通，AutoMigrate 成功
2. **auth 中间件** — demo 模式认证
3. **service 层** — 用户、项目、对话 CRUD
4. **handler 层** — HTTP 端点
5. **ws 层** — WebSocket 连接管理 + seq 队列
6. **eino 层** — 模型工厂 + Basic Agent + 工具注册
7. **chat service** — 连接 HTTP → Service → Eino → WS 推送
8. **测试** — 按 API 文档逐端点 curl 验证

### HTTP 端点清单（Phase 1）

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/api/v1/users/me` | 当前用户信息 |
| `GET` | `/api/v1/templates` | 模板列表 |
| `GET` | `/api/v1/projects` | 用户项目列表 |
| `POST` | `/api/v1/projects` | 从模板创建项目 |
| `GET` | `/api/v1/projects/:id` | 项目详情 |
| `DELETE` | `/api/v1/projects/:id` | 删除项目 |
| `GET` | `/api/v1/projects/:id/conversations` | 对话列表 |
| `POST` | `/api/v1/projects/:id/conversations` | 创建对话 |
| `DELETE` | `/api/v1/conversations/:id` | 删除对话 |
| `POST` | `/api/v1/conversations/:id/messages` | 发送消息 |
| `POST` | `/api/v1/conversations/:id/stop` | 停止流式响应 |
| `GET` | `/api/v1/users/me/updates` | 轮询更新 |
| `GET` | `/api/v1/settings` | 获取设置 |
| `PUT` | `/api/v1/settings` | 更新设置 |
| `GET` | `/api/v1/models` | 可用模型列表 |
| `GET` | `/ws?token=&last_seq=` | WebSocket 连接 |

---

## 阶段 2：前端骨架（并行起点 B）

### 文件结构

```
web/
├── package.json
├── next.config.ts
├── tsconfig.json
├── src/
│   ├── app/
│   │   ├── [locale]/
│   │   │   ├── layout.tsx          # Providers: theme, i18n, auth
│   │   │   ├── page.tsx            # 首页 / 模板浏览器
│   │   │   ├── not-found.tsx
│   │   │   ├── chat/
│   │   │   │   ├── layout.tsx
│   │   │   │   ├── page.tsx        # 对话列表
│   │   │   │   └── [convId]/
│   │   │   │       └── page.tsx    # 聊天界面
│   │   │   └── settings/
│   │   │       └── page.tsx        # 设置页
│   ├── components/
│   │   ├── chat/
│   │   │   ├── MessageList.tsx     # 消息列表
│   │   │   ├── MessageBubble.tsx   # 单条消息
│   │   │   ├── ToolCallCard.tsx    # 工具调用卡片
│   │   │   ├── StreamingText.tsx   # pretext 流式文本
│   │   │   └── ChatInput.tsx       # 输入框 + 停止按钮
│   │   ├── template/
│   │   │   ├── TemplateCard.tsx    # 模板卡片
│   │   │   └── TemplateList.tsx    # 模板列表
│   │   └── layout/
│   │       ├── Sidebar.tsx
│   │       ├── Header.tsx
│   │       └── ThemeSwitch.tsx
│   ├── hooks/
│   │   ├── useWebSocket.ts         # WS 连接 + 重连
│   │   ├── useStream.ts            # 流式状态
│   │   └── useTheme.ts             # 主题切换
│   ├── lib/
│   │   ├── api.ts                  # 类型化 HTTP 客户端
│   │   └── updateDispatcher.ts     # applyUpdates() 单例
│   ├── store/
│   │   ├── indexedDB.ts            # IndexedDB 读写
│   │   └── topic.ts                # 路由 → topic 推导
│   ├── providers/
│   │   ├── ThemeProvider.tsx       # Ant Design 主题
│   │   ├── WSProvider.tsx          # WebSocket 连接
│   │   ├── UpdateProvider.tsx      # UpdateDispatcher 上下文
│   │   └── ChatProvider.tsx        # 聊天状态
│   └── locales/
│       ├── en.json
│       └── zh.json
```

### 前端依赖清单

| 库 | 用途 |
|---|---|
| `next` | Next.js App Router |
| `react` + `react-dom` | React |
| `antd` | UI 组件库 |
| `@ant-design/icons` | 图标 |
| `next-intl` | 国际化 |
| `openapi-typescript` | 类型生成 |
| `@chenglou/pretext` | 流式文本布局 |
| `react-markdown` + `remark-gfm` | Markdown 渲染 |
| `react-syntax-highlighter` | 代码高亮 |

### 前端开发顺序

1. **初始化 Next.js 项目** — `package.json`, `next.config.ts`, 基础路由
2. **运行 generate.sh** — 生成 TypeScript 类型
3. **updateDispatcher.ts** — `applyUpdates()` 核心逻辑
4. **IndexedDB + topic** — 数据存储 + 路由推导
5. **Providers** — Theme, WS, Update, Chat Context
6. **布局 + 导航** — Sidebar, Header, ThemeSwitch
7. **模板浏览器** — TemplateList + TemplateCard
8. **聊天 UI** — MessageList, MessageBubble, ChatInput
9. **流式 + 工具卡片** — StreamingText, ToolCallCard, useStream
10. **设置页** — 模型切换、语言、主题
11. **WebSocket 重连** — 指数退避 + last_seq 恢复

### UpdateDispatcher 核心流程

```
source (WS/HTTP) → applyUpdates()
  → 分片: persistable (seq>0) vs ephemeral (seq=0)
  → persistable: 检查 seq 连续性
    → 有缺口: 中止, 触发 HTTP 拉取
    → 连续: bump settings.latest_seq
  → 从 payload 提取 conversation_id → 推导 topic
  → 通知 topic 订阅者
  → 订阅者触发 HTTP sync → IndexedDB upsert → 重渲染
```

---

## 错误处理约定

所有端点统一错误格式：

```json
{ "error": { "code": "ERROR_CODE", "message": "human readable" } }
```

| HTTP 状态 | Code | 触发条件 |
|-----------|------|----------|
| 400 | `INVALID_REQUEST` | 请求体格式错误、缺少必填字段 |
| 401 | `UNAUTHORIZED` | 缺少或空的 Authorization |
| 404 | `NOT_FOUND` | 资源不存在 |
| 500 | `INTERNAL_ERROR` | 服务端内部错误 |

前端处理：
- `INVALID_REQUEST` → `message.error(message)`
- `NOT_FOUND` → `Result` 页面或 `message.warning`
- `UNAUTHORIZED` → 重定向登录，清除 token
- `INTERNAL_ERROR` → `message.error('Something went wrong')`

## WebSocket 协议

### 连接

```
GET /ws?token=<token>&last_seq=<number>
```

服务端推送帧格式：
```json
{ "type": "connected", "payload": { "user_id", "server_time", "max_seq" } }
{ "type": "updates", "payload": [ /* Update[] */ ] }
```

客户端仅发送心跳：
```json
{ "type": "ping", "payload": {} }
```

所有业务操作（发消息、停止、重连）通过 HTTP 端点，不通过 WebSocket 发送。

## 测试策略

### 后端测试
1. 启动服务器 (`go run ./server/cmd/server/main.go`)
2. 用 `curl` 逐个端点验证
3. 检查实际 HTTP 响应（状态码、body 结构、字段名）
4. 对比 [docs/API.md](docs/API.md) 判断通过/失败
5. WebSocket 连接 + 发送/接收至少一帧
6. 测试错误路径：无效请求体、缺认证、坏参数

### 前端测试
1. 手动浏览各页面验证路由
2. 模板列表 → 创建项目 → 创建对话 → 发消息 → 收回复
3. 流式文本渲染 + 停止按钮
4. 工具调用卡片折叠/展开
5. 主题切换 + 语言切换
6. WebSocket 断线重连

## 成功标准

- [ ] `docker-compose up` 启动 PostgreSQL + Redis
- [ ] `openapi/generate.sh` 成功生成 Go + TS 类型
- [ ] 后端启动无报错，AutoMigrate 成功
- [ ] 所有 HTTP 端点返回正确状态码和响应体
- [ ] WebSocket 连接成功，推送 updates
- [ ] 前端可浏览模板、创建项目、创建对话
- [ ] 发送消息后收到流式 AI 回复
- [ ] 工具调用以卡片形式显示
- [ ] 停止按钮可中断流式响应
- [ ] 主题/语言切换生效
- [ ] 模型切换（haiku/sonnet/opus）生效
- [ ] 设置页可修改并持久化偏好
