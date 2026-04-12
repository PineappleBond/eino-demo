# 工具权限网关设计文档

> **状态**: 待评审
> **日期**: 2026-04-12

## 1. 概述

为 Eino Agent 的 Tool 调用增加**多层权限网关**，在敏感操作执行前自动拦截，通过白名单匹配 + 轻量 Agent 评估 + 人类确认（HITL）的三层递进机制，确保 Agent 不会在用户不知情时执行高风险操作。

### 目标

- 所有 Tool 调用在**执行前**经过权限评估
- 已有白名单授权的操作**零延迟放行**
- 未知操作由轻量 Agent（haiku）做**安全评级**
- 超出阈值的操作**中断并询问**用户，支持四种决策
- 用户决策**持久化到 DB**，同一 Project 自动记忆

### 非目标

- 不做细粒度的参数级校验（如"只能读 /tmp 目录"），那是后续 V2 的事
- 不做跨 Project 的权限共享

## 2. 核心流程

```
LLM 发起 Tool 调用
  │
  ▼
┌──────────────────────────────────────────────────────┐
│ ① ToolPermissionMiddleware 拦截                       │
│  - 断言工具是否实现 NeedPermissioner 接口              │
│  - 未实现 → 直接放行                                  │
│  - 已实现 → 调用 NeedPermission(input)                │
│    拿到 PermissionRequest（action, content, 工具信息等） │
└──────────────────────────────────────────────────────┘
  │
  ▼
┌──────────────────────────────────────────────────────┐
│ ② 查 DB 白名单 (project_id 维度)                      │
│  - 精确匹配: tool_name + action + pattern             │
│  - 通配符匹配: pattern 使用 glob 语法 (*, ?)          │
│  - 命中 → 直接放行                                    │
│  - 未命中 → 进入安全评估                               │
└──────────────────────────────────────────────────────┘
  │
  ▼
┌──────────────────────────────────────────────────────┐
│ ③ 安全评估 Agent (haiku 模型)                         │
│  - 输入: tool_name, tool_desc, action, content,       │
│    args_summary, tool_self_level                     │
│  - 输出: safety_level (1-4) + reason                 │
│  - 比较 project 阈值（默认 2）                         │
│  - level ≤ threshold → 放行                           │
│  - level > threshold → 进入中断流程                    │
└──────────────────────────────────────────────────────┘
  │
  ▼
┌──────────────────────────────────────────────────────┐
│ ④ Interrupt + HITL (ask_user_question 风格)           │
│  - 推送 permission.pending Update                     │
│  - 写入 HumanInPermission 记录                        │
│  - 等待用户四选一:                                    │
│    1. 同意（仅此一次）                                 │
│    2. 同意 + 加入精确白名单                            │
│    3. 同意 + 加入通配符白名单                          │
│    4. 拒绝                                            │
└──────────────────────────────────────────────────────┘
  │
  ▼
┌──────────────────────────────────────────────────────┐
│ ⑤ Resume                                             │
│  - 同意 → 执行原 Tool 调用，返回结果                   │
│  - 拒绝 → 返回错误，Agent 得知调用被拒                 │
│  - 推送 permission.decided Update                     │
└──────────────────────────────────────────────────────┘
```

## 3. 安全等级定义

| 等级 | 含义 | 示例 | 默认行为（阈值=2） |
|------|------|------|-------------------|
| **1 - Safe** | 纯只读、无副作用 | 天气查询、维基百科搜索 | 放行 |
| **2 - Low** | 读取敏感信息 | 读取本地文件、列出目录 | 放行 |
| **3 - Medium** | 写入/修改操作 | 创建/编辑文件、修改配置 | 中断问人类 |
| **4 - High** | 执行命令、网络外发、不可逆 | shell 执行、HTTP POST/DELETE | 中断问人类 |

**默认阈值为 2**：等级 1-2 放行，3-4 中断问人类。可在 Project Config 中配置。

## 4. 数据模型

### 4.1 ProjectToolPermission（新增）

```go
// ProjectToolPermission stores per-project tool permission whitelists.
type ProjectToolPermission struct {
    BaseModel
    ProjectID uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`
    ToolName  string    `gorm:"type:varchar(128);not null;index:idx_perm_tool_action"`
    Action    string    `gorm:"type:varchar(64);not null;index:idx_perm_tool_action"`
    Pattern   string    `gorm:"type:text;not null"`                    // glob 模式: "*" 匹配全部, "*.pdf" 匹配所有PDF
    GrantedBy uuid.UUID `gorm:"type:uuid;not null"`                    // 授权用户
    Level     string    `gorm:"type:varchar(16);not null;default:'exact'"` // "exact" | "wildcard"
}

func (ProjectToolPermission) TableName() string { return "project_tool_permissions" }
```

### 4.2 HumanInPermission（新增）

记录待审批的权限请求，类似 HumanInTheLoop 但针对权限网关。

```go
// HumanInPermission represents a pending tool permission request awaiting user approval.
type HumanInPermission struct {
    BaseModel
    ConversationID uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`
    CheckpointID   string    `gorm:"type:varchar(255);not null"`
    InterruptID    string    `gorm:"type:varchar(255);not null"`
    ToolName       string    `gorm:"type:varchar(128);not null"`
    Action         string    `gorm:"type:varchar(64);not null"`
    Content        string    `gorm:"type:text;not null"`
    ToolDesc       string    `gorm:"type:text;not null"`
    ArgsSummary    string    `gorm:"type:text;not null"`
    SafetyLevel    int       `gorm:"not null"`                      // Agent 评估的安全等级 1-4
    SafetyReason   string    `gorm:"type:text;not null"`            // Agent 给出的理由
    Decision       string    `gorm:"type:varchar(20);default:null"` // "approved" | "approved_exact" | "approved_wildcard" | "denied"
    Status         string    `gorm:"type:varchar(20);not null;default:'pending'"`
    CreatedAt      time.Time `gorm:"autoCreateTime"`
}

func (HumanInPermission) TableName() string { return "human_in_permissions" }
```

## 5. Go 接口定义

### 5.1 NeedPermissioner

工具实现此接口以声明权限需求：

```go
// PermissionRequest carries all context needed for permission evaluation.
type PermissionRequest struct {
    // Action is the operation type: "read", "write", "execute", "search", "network".
    Action string
    // Content is a human-readable summary of what the tool will do (used for whitelist matching).
    Content string
    // ToolName is the tool's registered name.
    ToolName string
    // ToolDesc is the tool's description (why it exists).
    ToolDesc string
    // ArgsSummary is a brief summary of the input arguments for this invocation.
    ArgsSummary string
    // ToolLevel is the tool's self-assessed risk level (1-4). 0 means no self-assessment.
    ToolLevel int
}

// NeedPermissioner is implemented by tools that require permission checks before execution.
type NeedPermissioner interface {
    // NeedPermission returns a PermissionRequest for the given input.
    // Returns nil if the tool determines this specific invocation needs no permission check.
    NeedPermission(input any) *PermissionRequest
}
```

### 5.2 哪些工具需要实现

| 工具 | 实现? | Action | 示例 Content |
|------|-------|--------|-------------|
| `weather` | 否 | - | 只读公开 API，无需权限 |
| `ask_user_question` | 否 | - | 本身就是 HITL |
| `todo_read/write` | 否 | - | Project 内部数据操作，范围可控 |
| `http_get` | 是 | `search` | "GET https://api.example.com/data" |
| `http_post/put/delete` | 是 | `network` | "POST https://api.example.com/update" |
| `wikipedia_search` | 否 | - | 只读公开知识 |
| `sequential_thinking` | 否 | - | 纯推理，无外部影响 |
| `local_backend.Read` | 是 | `read` | "读取 /path/to/file.txt" |
| `local_backend.Write` | 是 | `write` | "创建 /path/to/new.txt" |
| `local_backend.Edit` | 是 | `write` | "修改 /path/to/config.yml" |
| `local_backend.Execute` | 是 | `execute` | "执行: ls -la /tmp" |
| `local_backend.GrepRaw` | 是 | `read` | "搜索 /path/*.go 中的 pattern" |

## 6. 中间件实现

### 6.1 ToolPermissionMiddleware

```go
type ToolPermissionMiddleware struct {
    db          *gorm.DB
    projectID   uuid.UUID
    userID      uuid.UUID
    model       model.ToolCallingChatModel  // haiku，用于安全评估
    threshold   int                          // 默认 2
    pushUpdate  PushUpdateFunc
    nextSeq     NextSeqFunc
}
```

**BeforeTool 拦截流程**：

1. 检查工具是否实现 `NeedPermissioner`
2. 未实现 → 放行
3. 已实现 → 调用 `NeedPermission(input)` 拿到 `PermissionRequest`
4. 查 DB 白名单：`SELECT * FROM project_tool_permissions WHERE project_id = ? AND tool_name = ? AND action = ?`
   - 精确匹配：`pattern == content`
   - 通配符匹配：`filepath.Match(pattern, content)` 或自定义 glob
5. 命中 → 放行
6. 未命中 → 调 haiku Agent 做安全评估
7. haiku 输出 `{"safety_level": 3, "reason": "..."}`
8. `level <= threshold` → 放行
9. `level > threshold` → Interrupt（见第 7 节）

### 6.2 安全评估 Agent 的 System Prompt

```
你是一个安全评估助手。你的任务是评估一个 AI Agent 即将执行的工具调用是否需要人类事先知情同意。

## 评估规则

安全等级分为 4 级：
- 等级 1 (Safe): 纯只读操作，访问公开信息，无任何副作用
- 等级 2 (Low): 读取操作，但可能涉及用户私人数据（如读取文件内容、列出目录）
- 等级 3 (Medium): 写入或修改操作（如创建/编辑文件、修改配置、发送只读请求以外的 HTTP）
- 等级 4 (High): 执行系统命令、不可逆操作、可能造成数据丢失的操作

## 输出格式

只输出 JSON，不要输出其他内容：
{"safety_level": <1-4>, "reason": "<一句话说明理由>"}

## 上下文

工具名称: {{.ToolName}}
工具用途: {{.ToolDesc}}
操作类型: {{.Action}}
操作内容: {{.Content}}
调用参数: {{.ArgsSummary}}
工具自评: {{.ToolLevel}} (0表示不自评)
```

## 7. 中断与恢复

### 7.1 Interrupt

当安全等级 > 阈值时，复用 eino 的 `tool.Interrupt()` 机制：

```go
tool.Interrupt(ctx, map[string]any{
    "type":           "permission_request",
    "tool_name":      req.ToolName,
    "action":         req.Action,
    "content":        req.Content,
    "tool_desc":      req.ToolDesc,
    "args_summary":   req.ArgsSummary,
    "safety_level":   level,
    "safety_reason":  reason,
    "question":       fmt.Sprintf("Agent 想要调用 %s，是否允许？\n\n操作: %s\n详情: %s", req.ToolName, req.Action, req.Content),
    "choices": []HitlChoice{
        {Title: "同意", Desc: "允许此次操作"},
        {Title: "同意并记住", Desc: "允许，且同一 Project 下不再询问（精确匹配）"},
        {Title: "同意并通配记住", Desc: "允许，且同类操作不再询问（通配符匹配）"},
        {Title: "拒绝", Desc: "不允许此次操作"},
    },
    "answer_type": "single",
})
```

### 7.2 DB 写入

拦截时创建 `HumanInPermission` 记录，状态 `pending`。同时推送 `permission.pending` Update：

```go
update := model.UserUpdate{
    UserID: userID,
    Seq:    seq,
    Type:   "permission.pending",
    Payload: model.JSONMap{
        "permission_id": perm.ID.String(),
        "tool_name":     req.ToolName,
        "action":        req.Action,
        "content":       req.Content,
        "safety_level":  level,
        "safety_reason": reason,
        "seq":           seq,
    },
}
```

### 7.3 Resume

用户回答后，`AnswerPermission` 方法处理：

1. 更新 `HumanInPermission.Status` → `answered`，`Decision` → 用户选择
2. 根据用户选择写白名单（选择 2 或 3 时）
3. 推送 `permission.decided` Update
4. 设置 `ResumeParams`，重启 Agent
5. 中间件 Resume 后，根据 Decision 决定放行还是返回错误

Resume 时的决策映射：

| 用户选择 | Decision | 中间件行为 |
|---------|----------|-----------|
| 同意 | `approved` | 放行，执行原调用 |
| 同意并记住 | `approved_exact` | 写入精确白名单 → 放行 |
| 同意并通配记住 | `approved_wildcard` | 写入通配符白名单 → 放行 |
| 拒绝 | `denied` | 返回错误，Agent 得知调用被拒 |

## 8. 与现有架构的集成点

### 8.1 RootRunnerConfig 新增字段

```go
type RootRunnerConfig struct {
    // ... 现有字段不变
    PermissionMW *ToolPermissionMiddleware // nil 表示不启用权限网关
}
```

### 8.2 NewRootRunner 集成

权限中间件作为 DeepAgent 的 Handler 链的一环，插入在 Summarization 之后、Context Injection 之前：

```go
// handlers 链: Summarization → Permission → Context Injection → Reduction
if cfg.PermissionMW != nil {
    handlers = append(handlers, cfg.PermissionMW)
}
```

### 8.3 ChatService 变更

- `runAgent` 方法中构建 `PermissionMW` 实例（需要 projectID）
- 新增 `AnswerPermission` 方法，类似 `AnswerQuestion` 的流程
- 新增 `ListPendingPermissions` 方法

### 8.4 Handler 层新增

- `POST /api/conversations/:id/permissions/:permId/answer` — 回答权限请求
- `GET /api/conversations/:id/permissions?status=pending` — 列出权限请求

### 8. OpenAPI Spec 更新（协议优先）

**所有前后端通信协议必须先写入 `openapi/spec.yaml`**，然后运行 `openapi/generate.sh` 生成 Go (`server/internal/types/types.go`) 和 TypeScript (`web/src/types/api.d.ts`) 类型。禁止手写已有类型的 Request/Response。

### 8.1 新增 HTTP 端点

```yaml
# 权限请求列表
/conversations/{id}/permissions:
  get:
    summary: List permission requests for a conversation
    parameters:
      - name: id
        in: path
        required: true
        schema: { type: string }
      - name: status
        in: query
        required: false
        schema: { type: string, enum: [pending, answered] }
    responses:
      "200":
        content:
          application/json:
            schema:
              type: array
              items: { $ref: "#/components/schemas/HumanInPermission" }

# 回答权限请求
/conversations/{id}/permissions/{permId}/answer:
  post:
    summary: Answer a permission request
    parameters:
      - name: id
        in: path
        required: true
        schema: { type: string }
      - name: permId
        in: path
        required: true
        schema: { type: string }
    requestBody:
      required: true
      content:
        application/json:
          schema:
            type: object
            required: [checkpoint_id, interrupt_id, decision]
            properties:
              checkpoint_id:
                type: string
              interrupt_id:
                type: string
              decision:
                type: string
                enum: [approved, approved_exact, approved_wildcard, denied]
    responses:
      "200": { description: Permission answered, agent resumed }
      "404": { description: Permission request not found }
      "400": { description: Already answered or invalid request }
```

### 8.2 新增 Schemas

```yaml
HumanInPermission:
  type: object
  required: [id, conversation_id, tool_name, action, content, safety_level, status, created_at]
  properties:
    id: { type: string, format: uuid }
    conversation_id: { type: string, format: uuid }
    checkpoint_id: { type: string }
    interrupt_id: { type: string }
    tool_name: { type: string }
    action: { type: string }
    content: { type: string }
    tool_desc: { type: string }
    args_summary: { type: string }
    safety_level: { type: integer, minimum: 1, maximum: 4 }
    safety_reason: { type: string }
    decision:
      type: string
      nullable: true
      enum: [approved, approved_exact, approved_wildcard, denied]
    status: { type: string, enum: [pending, answered] }
    created_at: { type: string, format: date-time }
```

### 8.3 新增 Update 类型

在 Update.type enum 中追加：

- `permission.pending` — 新的权限请求
- `permission.decided` — 用户已做出决策

新增对应 Payload：

```yaml
PermissionPendingPayload:
  type: object
  required: [conversation_id, permission_id, tool_name, action, content, safety_level, safety_reason, seq]
  properties:
    conversation_id: { type: string }
    permission_id: { type: string }
    tool_name: { type: string }
    action: { type: string }
    content: { type: string }
    tool_desc: { type: string }
    args_summary: { type: string }
    safety_level: { type: integer }
    safety_reason: { type: string }
    checkpoint_id: { type: string }
    interrupt_id: { type: string }
    seq: { type: integer, format: int64 }

PermissionDecidedPayload:
  type: object
  required: [conversation_id, permission_id, decision, seq]
  properties:
    conversation_id: { type: string }
    permission_id: { type: string }
    decision: { type: string, enum: [approved, approved_exact, approved_wildcard, denied] }
    seq: { type: integer, format: int64 }
```

### 8.4 集成到 spec

- 在 Update.payload 的 oneOf 中加入 `PermissionPendingPayload` 和 `PermissionDecidedPayload`
- 在 `Update.type` 的 enum 列表中追加两个新值

---

## 9. 前端变更

### 9.1 类型生成

运行 `openapi/generate.sh` 后自动生成：

- Go: `server/internal/types/types.go` 中的 `HumanInPermission`, `PermissionPendingPayload`, `PermissionDecidedPayload`
- TypeScript: `web/src/types/api.d.ts` 中对应接口

**禁止手动编写这些类型。**

### 9.2 Update 类型处理

前端 `applyUpdates()` 需处理两种新 Update 类型：

- `permission.pending` — 显示权限请求卡片（Tool 名、操作、安全等级、Agent 理由、四选一按钮）
- `permission.decided` — 更新权限请求状态（已批准/已拒绝）

### 9.3 UI 组件

新增 `PermissionRequestCard` 组件，类似 `ToolCallCard` 但带有审批按钮：

- 显示工具名称、操作类型、内容摘要
- 显示安全等级（颜色标识）和评估理由
- 四个操作按钮（Ant Design Button 组）

## 10. 错误处理

| 场景 | 行为 |
|------|------|
| 白名单查询 DB 失败 | 降级为 Agent 评估（不阻断） |
| Agent 评估失败（超时/错误） | 默认按等级 3 处理（中断问人类） |
| Resume 时白名单写入失败 | 仅此次放行（不阻断 Agent），log 记录 |
| 用户长时间不回答 | 保持 pending 状态，不自动超时 |
| Tool 未实现 NeedPermissioner | 直接放行 |

## 11. 文件变更清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `server/internal/model/project_tool_permission.go` | **新增** | DB 模型 |
| `server/internal/model/human_in_permission.go` | **新增** | DB 模型 |
| `server/internal/eino/permission/middleware.go` | **新增** | 权限中间件核心 |
| `server/internal/eino/permission/evaluator.go` | **新增** | 安全评估 Agent |
| `server/internal/eino/permission/prompt.go` | **新增** | Agent prompt |
| `server/internal/eino/permission/types.go` | **新增** | 接口 + 类型定义 |
| `server/internal/eino/runner/rootrunner.go` | **修改** | RootRunnerConfig 新增 PermissionMW |
| `server/internal/service/chat.go` | **修改** | 新增 AnswerPermission 等方法 |
| `server/internal/service/permission.go` | **新增** | 权限管理 service |
| `server/internal/handler/permission.go` | **新增** | 权限 HTTP 端点 |
| `server/internal/handler/http.go` | **修改** | 注册权限路由 |
| `server/internal/db/db.go` | **修改** | AutoMigrate 新表 |
| `openapi/spec.yaml` | **修改** | 新增 Permission 类型和端点 |
| `web/src/components/chat/PermissionRequestCard.tsx` | **新增** | 权限请求卡片 |
| `web/src/types/api.d.ts` | **自动生成** | 运行 openapi/generate.sh |

## 12. 实施步骤（概要）

1. 新增 DB 模型 + AutoMigrate
2. 定义 NeedPermissioner 接口 + PermissionRequest 类型
3. 实现 ToolPermissionMiddleware（白名单匹配 + Agent 评估）
4. 实现安全评估 Agent + Prompt
5. 集成到 RootRunner handler 链
6. 实现 Interrupt + Resume 流程（类似 ask_user_question）
7. 为 httprequest / local_backend 工具实现 NeedPermissioner
8. 新增 Service + Handler 层
9. OpenAPI spec 更新 + 类型生成
10. 前端 PermissionRequestCard 组件 + Update 处理
11. 集成测试（DB + 真实 LLM）
