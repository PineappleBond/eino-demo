# API & WebSocket 设计评审 — 多角色会议记录

**日期**: 2026-04-10
**参与角色**: 后端架构师、前端架构师、DBA、产品经理、测试工程师
**评审目标**: 审查现有 API 接口和 WebSocket 协议定义，发现 gaps、不一致处和改进点

---

## 一、现状概述

### 已定义的 4 个端点

| # | 方法 | 路径 | 用途 |
|---|------|------|------|
| 1 | POST | `/api/v1/projects/{id}/conversations` | 创建对话 |
| 2 | POST | `/api/v1/conversations/{id}/messages` | 发送消息 |
| 3 | GET | `/api/v1/users/me/updates?last_seq=N` | 轮询回退 |
| 4 | WS | `/ws?token=&last_seq=` | WebSocket 连接 |

### OpenAPI 状态
`openapi/spec.yaml` 的 `paths: {}` 为空 — **零个 HTTP 端点被正式定义**。类型生成仅产出 WebSocket 信封类型和 Update 类型。

### 各角色一致认为的核心问题
> **当前 API 覆盖率仅 6%**。需求文档暗示至少需要 24 个端点，目前仅定义 4 个。

---

## 二、缺失端点清单（全员共识）

### 2.1 P0 — MVP 阻塞项（13 个）

| # | 方法 | 路径 | 用途 | 提出者 |
|---|------|------|------|--------|
| 1 | GET | `/api/v1/templates` | 模板列表（首页） | 全员 |
| 2 | GET | `/api/v1/templates/{id}` | 模板详情 | 全员 |
| 3 | POST | `/api/v1/templates/{id}/projects` | 从模板创建项目 | 全员 |
| 4 | GET | `/api/v1/projects` | 用户项目列表 | 全员 |
| 5 | GET | `/api/v1/projects/{id}` | 项目详情（含 agents + relationships） | 全员 |
| 6 | PATCH | `/api/v1/projects/{id}` | 更新项目（重命名/配置） | 前+产 |
| 7 | DELETE | `/api/v1/projects/{id}` | 删除项目 | 全员 |
| 8 | GET | `/api/v1/projects/{id}/conversations` | 对话列表（左侧栏） | 全员 |
| 9 | GET | `/api/v1/conversations/{id}` | 单个对话详情 | 前+产 |
| 10 | PATCH | `/api/v1/conversations/{id}` | 重命名/归档对话 | 前+产 |
| 11 | DELETE | `/api/v1/conversations/{id}` | 删除对话 | 全员 |
| 12 | GET | `/api/v1/conversations/{id}/messages` | 消息历史（分页） | 全员 |
| 13 | POST | `/api/v1/conversations/{id}/stop` | 停止流式响应 | 全员 |

### 2.2 P0 — 用户与设置（3 个）

| # | 方法 | 路径 | 用途 |
|---|------|------|------|
| 14 | GET | `/api/v1/users/me` | 获取用户信息（引导加载） |
| 15 | GET | `/api/v1/users/me/settings` | 获取用户设置 |
| 16 | PATCH | `/api/v1/users/me/settings` | 更新设置（触发 settings.changed） |

### 2.3 P1 — 辅助数据（4 个）

| # | 方法 | 路径 | 用途 |
|---|------|------|------|
| 17 | GET | `/api/v1/projects/{id}/agents` | Agent 列表（@提及自动完成） |
| 18 | GET | `/api/v1/projects/{id}/agent-relationships` | Agent 关系树 |
| 19 | GET | `/api/v1/conversations/{id}/members` | 对话成员列表 |
| 20 | POST | `/api/v1/conversations/{id}/branches` | 分支对话 |

### 2.4 Phase 2-3 — 模板特定（7 个）

| # | 方法 | 路径 | 用途 | 模板 |
|---|------|------|------|------|
| 21 | POST | `/api/v1/projects/{id}/documents` | RAG 文档上传 | 05 |
| 22 | GET | `/api/v1/projects/{id}/documents` | RAG 文档列表 | 05 |
| 23 | DELETE | `/api/v1/projects/{id}/documents/{doc_id}` | RAG 文档删除 | 05 |
| 24 | POST | `/api/v1/conversations/{id}/compact` | 手动上下文压缩 | 10 |
| 25 | GET | `/api/v1/conversations/{id}/checkpoints` | 检查点列表 | 10 |
| 26 | POST | `/api/v1/conversations/{id}/checkpoints/{cp_id}/resume` | 恢复中断 | 10 |
| 27 | GET | `/api/v1/conversations/{id}/trace` | 回调/追踪可视化 | 11 |

---

## 三、各角色详细分析

### 3.1 后端架构师 — 关键发现

#### WebSocket 协议缺陷
1. **缺少 `error` 帧类型**: 当前只有 `updates` 和 `connected`。WS 级错误（限流、内部错误）无法传达。
   - 建议新增: `{ "type": "error", "payload": { "code", "message", "retry_after_ms" } }`

2. **多连接竞态风险**: 多 Tab 连接同一用户时，重复推送浪费带宽，seq 乱序触发客户端回退。
   - 建议: 指定"主连接"（最先连接），优先推送主连接，再扇出到副连接。

3. **seq 窗口期丢消息**: `connected` 帧包含 `max_seq`，但从读取到发送之间可能产生新更新。
   - 建议: 发送 `connected` 后立即补推 `seq > client.last_seq` 的更新批次。

4. **WS 推送批量策略未定义**:
   - 建议: seq>0 更新立即推送（低延迟），seq=0 流式 token 缓存 50ms 或 10 个 token 后批量推送。
   - 单帧上限 64KB，避免 WS 分片。
   - 不同对话的更新不要混在同一批次。

#### 服务层设计
需要 7 个 Service: `UserService`, `ProjectService`, `TemplateService`, `ConversationService`, `ChatService`, `SettingsService`, `AgentService`。

**关键不变量**: 每个写操作必须在同一个 DB 事务中同时写入 `user_updates` 行。

```
BEGIN;
  INSERT INTO messages (...) VALUES (...);
  UPDATE conversations SET latest_message_seq = ... WHERE id = ...;
  INSERT INTO user_updates (user_id, seq, type, payload) VALUES (...);
COMMIT;
```

#### 错误码扩展

| HTTP 状态 | 错误码 | 场景 |
|-----------|--------|------|
| 400 | `CONVERSATION_INACTIVE` | 向已归档/压缩的对话发消息 |
| 400 | `NO_ACTIVE_STREAM` | 无活跃流式时调用 stop |
| 400 | `INVALID_ENUM_VALUE` | 设置字段枚举值非法 |
| 403 | `FORBIDDEN` | 资源存在但非当前用户所有 |
| 409 | `CONFLICT` | 并发修改（如同时触发两次压缩） |
| 429 | `RATE_LIMITED` | 请求频率超限 |
| 503 | `SERVICE_UNAVAILABLE` | LLM 提供商下线 / Redis 不可用 |

#### 不一致问题
- **`NOT_FOUND` 同时出现在 400 和 404**: 400 的应改名为 `RESOURCE_NOT_FOUND` 或直接用 `INVALID_REQUEST`。
- **OpenAPI spec 的 `paths: {}` 为空**: 违背了"一份 spec，两端代码"原则。

---

### 3.2 前端架构师 — 关键发现

#### 引导加载流程缺失

当前文档没有定义 App 初始化流程。建议序列:

```
1. 检查 IndexedDB settings.auth_token
   ├── 存在 → 使用
   └── 不存在 → 生成 crypto.randomUUID()，存入 settings

2. GET /api/v1/users/me  → 填充 users + settings IndexedDB 表

3. WS 连接: ws://host/ws?token=<token>&last_seq=<settings.latest_seq>
   ├── connected 帧: 比较 max_seq 与 local latest_seq
   └── 有 gap → 启动 HTTP 拉取

4. GET /api/v1/templates （与步骤 3 并行）

5. 按路由加载数据:
   ├── / (首页) → GET /api/v1/projects → 渲染模板网格
   ├── /project/[id] → GET /api/v1/projects/[id] → 项目详情 + Agent 树
   └── /project/[id]/chat → GET /api/v1/projects/[id]/conversations → 对话列表
```

#### 状态存储分层

| 数据 | 存储位置 | 理由 |
|------|----------|------|
| 用户信息 | IndexedDB `users` + React Context | 极少变化，全局需要 |
| 设置 | IndexedDB `settings` + React Context | 用户可修改，全局需要 |
| `latest_seq` | IndexedDB `settings.latest_seq` | 同步游标，跨标签页存活 |
| 流式 delta (seq=0) | React `useReducer` 状态 | 临时数据，`message.done` 时清除 |
| 草稿 | IndexedDB `drafts` | 跨标签页关闭存活 |
| 模板列表 | 内存（React Context） | 永不变化，不需要 IndexedDB |
| UI 状态（面板开关等） | 组件状态 / URL 参数 | 不持久化 |

#### 乐观 UI 发消息流程

```
1. 用户点击发送
2. 生成 client_id = crypto.randomUUID()
3. 立即插入 IndexedDB: { client_id, status: 'sending', ... } → 渲染 + 旋转图标
4. POST /api/v1/conversations/{id}/messages
5. 200 响应: 更新 { id: message_id, seq: response.seq, status: 'sent' }
6. WS 推送 delta → 追加到流式缓冲区
7. WS 推送 message.done → 更新最终数据，status='delivered'，清除流式缓冲区
```

#### 关键不一致

**`applyUpdates()` 实体持久化矛盾**:
- API.md 说 `applyUpdates()` 只处理 Update 事件流（seq>0 写入 updates store）
- 但 IndexedDB 文档说**没有** updates store，只有 `settings.latest_seq` 游标
- 建议: `applyUpdates()` 只 bump `latest_seq` + 通知订阅者。实体数据从 Update payload 中直接写入 IndexedDB（无需 HTTP 往返），HTTP sync 仅用于页面挂载恢复。

**`send_message` 响应不完整**:
- 当前响应只有 `{ conversation_id, message_id, seq }`
- 客户端需要从 HTTP 响应构造 Update 对象传给 `applyUpdates()`
- 建议: HTTP 响应返回完整 Update 对象，客户端直接 `applyUpdates([response.update])`

---

### 3.3 DBA — 关键发现

#### 缺失索引

| 表 | 缺失索引 | 影响查询 |
|---|----------|----------|
| `conversations` | `(user_id, updated_at DESC)` | 对话列表按活动时间排序 — **PostgreSQL 缺失，IndexedDB 有** |
| `messages` | `(conversation_id, seq DESC)` | 获取最新 N 条消息 |
| `checkpoints` | `(conversation_id)` | 查询对话的所有检查点 |
| `documents` | `(user_id, status)`, `(project_id)` | Phase 3 文档管理 |

#### N+1 查询风险

**项目详情** 需要 3 次查询（project + agents + relationships）。如果关系表需要解析 agent 名称，需要 JOIN:

```sql
SELECT ar.*, a_parent.agent_name AS parent_name, a_child.agent_name AS child_name
FROM agent_relationships ar
JOIN agents a_parent ON ar.parent_id = a_parent.id
JOIN agents a_child ON ar.child_id = a_child.id
WHERE ar.project_id = $1
ORDER BY ar.sort_order;
```

#### Update 类型覆盖缺口

**缺失 `checkpoint.created` 和 `checkpoint.resolved`**: 模板 10（中断与恢复）需要实时通知前端显示中断提示。

**可选新增 `conversation.updated`**: 当对话元数据（标题自动生成、token 统计）变化但没有新消息时，对话列表视图无法感知。

#### Seq 系统 — Redis 崩溃恢复

推荐惰性恢复策略:

```go
func (s *SeqService) GetNextSeq(userID uuid.UUID) (int64, error) {
    seq, err := s.redis.Incr(ctx, "seq:"+userID.String()).Result()
    if err != nil { // Redis 重启
        maxSeq, _ := s.db.GetMaxUserSeq(userID)
        s.redis.Set(ctx, "seq:"+userID.String(), maxSeq, 0)
        seq, _ = s.redis.Incr(ctx, "seq:"+userID.String()).Result()
    }
    return seq, err
}
```

#### user_updates 表增长预估

- 单次会话: ~20 消息 → ~40 条更新
- 每日 10 次会话 → 400 条/天/用户
- 30 天 → 12,000 条/用户
- 建议定期清理: 保留每用户最近 1,000 条，删除 30 天前的旧数据

---

### 3.4 产品经理 — 关键发现

#### 用户故事覆盖缺口

| 用户故事 | 端点覆盖 | 状态 |
|----------|----------|------|
| 浏览模板 | 无 | **完全缺失** |
| 从模板创建项目 | 无 | **完全缺失** |
| 查看项目详情 | 无 | **完全缺失** |
| 删除项目 | 无 | **完全缺失** |
| 对话列表 | 无 | **完全缺失** |
| 删除对话 | 无 | **完全缺失** |
| 停止流式 | 无 | **完全缺失** |
| 获取/更新设置 | 无 | **完全缺失** |
| 查看个人资料 | 无 | **完全缺失** |

#### "5 分钟内完成首次对话" 流程

| 步骤 | 用户操作 | 所需 API | 状态 |
|------|----------|----------|------|
| 1 | 打开 App | 前端自动生成 token | 未定义 |
| 2 | 看到模板网格 | `GET /api/v1/templates` | **缺失** |
| 3 | 点击"Basic Agent" | `GET /api/v1/templates/01` | **缺失** |
| 4 | 点击"使用此模板" | `POST /api/v1/templates/01/projects` | **缺失** |
| 5 | 进入项目 | `GET /api/v1/projects/{id}` | **缺失** |
| 6 | 点击"开始聊天" | `POST /api/v1/projects/{id}/conversations` | 已覆盖 |
| 7 | 输入消息发送 | `POST /api/v1/conversations/{id}/messages` | 已覆盖 |
| 8 | 观看流式响应 | WS `/ws` | 已覆盖 |

**结论: 步骤 1-5 完全没有 API 支持。用户无法发现模板或创建项目。**

#### 错误码 UX 映射

| 场景 | 当前 | 建议 |
|------|------|------|
| 限流 / 模型配额 | 无 | `RATE_LIMITED` (429) — UI: "请稍后重试" |
| Agent 执行超时 | 无 | `TIMEOUT` (504) — UI: "重试" |
| 对话已归档 | 无 | `CONVERSATION_ARCHIVED` (400) — UI: 只读徽章 |
| 上下文过大 | 无 | `CONTEXT_TOO_LARGE` (400) — UI: "立即压缩" |
| 模板不存在 | 无 | `TEMPLATE_NOT_FOUND` (404) |
| 重复停止 | 无 | `ALREADY_STOPPED` (400) |
| 项目有活跃对话 | 无 | `PROJECT_HAS_ACTIVE_CONVERSATIONS` (400) |

#### 引导体验

Demo 模式下零摩擦登录:
1. 前端自动生成 UUID token → 存入 localStorage
2. 首次 API 调用触发 Auth Middleware `FirstOrCreate`
3. 如果 `user.created_at == now`，显示引导提示: "欢迎！选择一个模板开始。"
4. 无需显式登录。

---

### 3.5 测试工程师 — 关键发现

#### 端点测试矩阵

每个端点需要覆盖的测试维度:
- **Happy path**: 合法请求 → 验证 HTTP 状态码 + 响应形状 + DB 副作用 + WS 推送
- **Error path**: 缺 auth、空 body、非法类型、不存在的资源 → 验证统一错误格式
- **Edge cases**: 最大长度、特殊字符、并发请求、rapid-fire (100 条/10 秒)

**关键测试用例**:
| 测试场景 | 验证点 |
|----------|--------|
| 并发创建对话 (10 次) | 每个对话唯一 ID，无竞态 |
| 并发发消息 (5 次同对话) | seq 唯一且递增，无丢失 |
| Gap 填充 (seq 1-20，仅 5,10,15,20 有数据) | 返回 20 条更新，15 条 empty 填充 |
| Redis 重启恢复 | Flush Redis → 从 DB 恢复 MAX(seq) → 新 seq 不冲突 |
| WS 多连接 (3 Tab) | 所有连接收到全部推送 |
| 心跳超时 (61 秒无 ping) | 服务端主动关闭 (code 1001) |
| 归档对话发消息 | 400 错误 |
| 压缩中对话发消息 | 400 错误 |

#### API 可测试性缺口

| 缺口 | 影响 | 建议 |
|------|------|------|
| 无测试数据重置端点 | 测试间状态污染 | 新增 `POST /__test__/reset` (仅测试模式) |
| 无 Mock LLM | 测试慢、贵、不稳定 | 环境变量 `EINO_TEST_MODE=true` 切换 |
| Update payload 形状未定义 | 无法验证 payload 结构 | OpenAPI spec 中为每种 type 定义具体 schema |
| 列表端点无分页 | 无法测试大数据量 | `?limit=N&cursor=<id>` |
| 无限流 | 高频请求无保护 | 文档化预期行为，至少加速率限制头 |

#### 推荐测试架构

```
server/test/
├── integration/
│   ├── auth_test.go           # Auth 中间件
│   ├── projects_test.go       # 项目 CRUD + 模板流程
│   ├── conversations_test.go  # 对话生命周期
│   ├── messages_test.go       # 消息发送 + AI 响应
│   ├── settings_test.go       # 设置获取/更新
│   ├── updates_test.go        # Seq 系统 + Gap 填充
│   ├── websocket_test.go      # WS 连接生命周期
│   └── streaming_test.go      # 完整聊天 (Mock LLM)
├── e2e/
│   └── full_flow_test.go      # 端到端 (真实 LLM)
└── fixtures/
    └── seed.go                # 测试数据助手
```

#### Seq 系统 — 最需要测试覆盖的组件

测试工程师标记 Seq 系统为最高风险组件，有最多边界条件:
- Gap 填充正确性
- Redis 崩溃恢复
- 并发 INCR
- 离线恢复
- 连续性检查
- 多用户独立 seq

---

## 四、优先级行动计划

### 第一阶段 — 立即执行（P0）

1. **[最高优先级] 在 `openapi/spec.yaml` 中定义全部 16 个 MVP 端点** — 这是影响力最大的单一行动。所有端点的请求/响应类型必须进入 spec，确保 `openapi/generate.sh` 产出准确的 Go + TypeScript 类型。

2. **补全引导加载流程文档** — 在 API.md 新增 Flow 7: App 初始化序列。

3. **明确 `applyUpdates()` 行为** — 选择一种策略并全局一致:
   - 方案 A: `applyUpdates()` 直接从 payload 写入 IndexedDB 实体（更低延迟，推荐）
   - 方案 B: `applyUpdates()` 仅 bump `latest_seq`，HTTP sync 写入实体（更简单，多一次往返）

4. **修改 `send_message` 响应** — 返回完整 Update 对象而非 `{ conversation_id, message_id, seq }`。

5. **添加 `error` WS 帧类型** — 处理 WS 级错误。

6. **修正 400/404 `NOT_FOUND` 冲突** — 400 改为 `INVALID_REQUEST` 或 `RESOURCE_NOT_FOUND`。

7. **定义 WS 推送批量策略** — 刷写时机、最大批次大小、seq>0 与 seq=0 分离。

### 第二阶段 — 近期执行（P1）

8. **新增 `checkpoint.created` / `checkpoint.resolved` Update 类型** — 模板 10 需要。

9. **考虑新增 `conversation.updated` Update 类型** — 对话元数据变更通知。

10. **添加 PostgreSQL 缺失索引** — `(user_id, updated_at DESC)` on conversations, `(conversation_id)` on checkpoints。

11. **定义 Redis 崩溃恢复流程** — 惰性恢复策略。

12. **定义项目详情嵌入响应** — `GET /projects/{id}` 应包含 agents + relationships，避免瀑布请求。

### 第三阶段 — Phase 2-3 规划

13. **RAG 文档管理端点** (3 个)
14. **检查点/中断恢复端点** (3 个)
15. **回调追踪端点** (1 个)

---

## 五、待讨论议题

以下是需要逐项讨论的具体议题:

1. **OpenAPI spec 填充策略** — 一次写完 16 个端点还是分批定义？
2. **`applyUpdates()` 实体持久化方案** — 方案 A（直接写入）还是方案 B（HTTP sync）？
3. **`send_message` 响应形状** — 是否返回完整 Update 对象？
4. **WS 推送批量策略** — 刷写时机、批次大小、分离规则
5. **`error` WS 帧类型的 payload 设计**
6. **多 Tab 连接管理** — 主/副连接还是全部平等？
7. **引导体验** — 是否需要 `POST /api/v1/auth/refresh` token 轮换？
8. **消息分页策略** — `?after`（向前）+ `?before`（向后）双方向？
9. **分支对话设计** — 独立端点还是复用创建对话？
10. **错误码扩展** — 是否需要全部 7 个新错误码？

---

## 六、议题讨论决议

| # | 议题 | 决议 |
|---|------|------|
| 1 | OpenAPI spec 填充策略 | 一次定义全部 16 个 P0 端点。`openapi/spec.yaml` 已更新。 |
| 2 | `applyUpdates()` 实体持久化 | **方案 B** — `applyUpdates()` 仅 bump `settings.latest_seq`，实体数据通过 HTTP sync (`GET /users/me/updates`) 拉取后写入 IndexedDB。 |
| 3 | `send_message` 响应形状 | **返回完整 `Update[]` 对象**，直接传给 `applyUpdates()`。当前实现中 applyUpdates 会触发 HTTP sync，但响应携带 Update 为未来优化留好入口。 |
| 4 | WS 推送批量策略 | **简单策略** — 所有更新立即推送，不缓存，不区分 seq，不同对话可同批次。不过度设计。 |
| 5 | `error` WS 帧类型 | **删除**。不加 `WSErrorPayload`，不加 `error` 帧类型。WS 级错误靠连接断开 + 客户端重连处理。已从 spec 移除。 |
| 6 | 多 Tab 连接管理 | **不考虑**。默认用户只开一个标签页。 |
| 7 | 引导体验 / Auth refresh | **不需要 token 轮换**。保持最简单的 UUID token 设计，零摩擦体验。 |
| 8 | 消息分页策略 | **保持当前设计** — `after_seq`（向前同步）+ `before_seq`（向后翻页）+ `order`（排序方向）。 |
| 9 | 分支对话设计 | **复用创建对话端点** — `POST /api/v1/projects/{id}/conversations` 通过 `parent_conversation_id` 字段表达分支关系，不需要独立端点。 |
| 10 | 错误码扩展 | **保持当前 11 个错误码**，对 demo 项目完全够用。 |
