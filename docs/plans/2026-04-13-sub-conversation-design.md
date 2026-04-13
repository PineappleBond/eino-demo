# 子对话系统设计文档

## 日期
2026-04-13

## 概述

实现三个独立功能：
1. **回调重构** — 消除 `cron_task` 和 `todo_write` 工具中的包级回调变量
2. **功能一：对话分支** — 用户通过 HTTP 创建，从现有对话复制历史消息
3. **功能二：Agent 子对话** — Agent 通过 subAgentTool 创建，异步执行任务

---

## 一、回调重构（优先级最高）

### 当前问题

`cron_task.go` 和 `todo_write.go` 使用包级 `var` 回调：
- `CronTaskRegisterFunc` — 注册任务到调度器
- `CronTaskSyncFunc` — 推送 `cron_task.sync` WS 事件
- `TodoSyncFunc` — 推送 `todo.sync` WS 事件

每个工具执行 sync 操作时，需要额外查 DB 获取 `userID`：
```go
var conv model.Conversation
db.Where("id = ?", t.conversationID).First(&conv)
TodoSyncFunc(ctx, conv.UserID, t.conversationID)
```

### 重构方案

#### 1. 新增 `SyncPushFunc` 类型

```go
// tools/sync.go
type SyncPushFunc func(ctx context.Context, userID uuid.UUID, conversationID uuid.UUID, updateType string)
```

#### 2. 修改工具构造函数签名

```go
func NewCronTaskTool(db *gorm.DB, conversationID uuid.UUID, syncFn SyncPushFunc)
func NewTodoWriteTool(db *gorm.DB, conversationID uuid.UUID, syncFn SyncPushFunc)
func NewTodoReadTool(db *gorm.DB, conversationID uuid.UUID) // 只读，不变
```

#### 3. 工具内部使用 `syncFn`

```go
// 替换前：
var conv model.Conversation
db.Where("id = ?", t.conversationID).First(&conv)
TodoSyncFunc(ctx, conv.UserID, t.conversationID)

// 替换后：
t.syncFn(ctx, t.userID, t.conversationID, "todo.sync")
```

#### 4. `ToolRegistry` 注入

```go
type ToolRegistry struct {
    // ... existing fields ...
    syncFn SyncPushFunc
}

func (r *ToolRegistry) SetSyncPushFn(syncFn SyncPushFunc) {
    r.syncFn = syncFn
}
```

#### 5. `GetBaseTools` 传入 syncFn

```go
todoWrite, _ := NewTodoWriteTool(r.db, r.conversationID, r.syncFn)
cronTask, _ := NewCronTaskTool(r.db, r.conversationID, r.syncFn)
```

#### 6. 删除模块注入

`module.go` 中删除以下赋值：
- `tools.CronTaskRegisterFunc = cronSvc.RegisterTask`
- `tools.CronTaskSyncFunc = func(...) {...}`
- `tools.TodoSyncFunc = func(...) {...}`

改为通过 `registry.SetSyncPushFn(...)` 注入。

#### 7. `CronTaskRegisterFunc` 处理

调度器注册逻辑移到 service 层：工具只负责 DB 创建，注册由 service 层在创建后调用。或者将 `RegisterTask` 也通过构造函数注入。

---

## 二、功能一：对话分支（HTTP 创建）

### 端点

```
POST /conversations/:id/branch
Content-Type: application/json
{ "input_seq": 10 }
```

### 逻辑

1. 查询原对话中 `seq <= input_seq` 的消息
2. 创建新对话（继承 Mode，不设置 `ParentConversationID`）
3. 将复制的消息插入新对话（保持相对顺序，重新分配 seq）
4. 返回新对话 ID

### 数据流

```
用户 → POST /conversations/:id/branch
  → handler: 解码 {input_seq}
  → service:
    1. 验证 input_seq <= LatestMessageSeq
    2. 查询 messages WHERE conversation_id = :id AND seq <= :input_seq ORDER BY seq ASC
    3. 创建新 conversation（继承 Mode，新 title）
    4. 插入消息（新 conversation_id，重新分配 seq）
    5. push conversation.created update
  → handler: 返回新对话 ID
```

### OpenAPI Spec 新增

```yaml
/conversations/{id}/branch:
  post:
    summary: Create a branched conversation
    requestBody:
      content:
        application/json:
          schema:
            type: object
            required: [input_seq]
            properties:
              input_seq:
                type: integer
                minimum: 0
                description: Branch from this message sequence number
    responses:
      200:
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/BranchConversationResponse'
```

```yaml
BranchConversationResponse:
  type: object
  properties:
    id:
      type: string
      format: uuid
    title:
      type: string
    mode:
      type: string
```

---

## 三、功能二：Agent 子对话（subAgentTool 创建）

### 3.1 Tool 定义

```go
// sub_agent_tool.go

type SubAgentInput struct {
    Prompt string `json:"prompt" jsonschema_description:"Task description for the sub-agent"`
}

type SubAgentOutput struct {
    Success     bool   `json:"success"`
    Desc        string `json:"desc"`
    TmpLogFile  string `json:"tmp_log_file"`
}
```

### 3.2 子对话创建流程

```
Agent 调用 subAgentTool
  → tool:
    1. 创建子对话（ParentConversationID = 父对话 ID，继承 Mode）
    2. 创建 JSONL 日志文件 /tmp/sub_conv_{uuid}.jsonl
    3. 写入日志 writer
    4. 异步执行 runAgent（与父对话独立 goroutine）
    5. 返回 {success: true, desc: "已启动", tmp_log_file: "/tmp/..."}
```

### 3.3 JSONL 日志格式

每行一个 JSON 对象：

```json
{"ts":"2026-04-13T10:00:00Z","seq":1,"type":"message.new","role":"user","content":"任务描述"}
{"ts":"2026-04-13T10:00:01Z","seq":2,"type":"message.delta","role":"assistant","delta":"开始分析..."}
{"ts":"2026-04-13T10:00:02Z","seq":2,"type":"message.tool_call","role":"assistant","tool":"read_file","status":"completed"}
{"ts":"2026-04-13T10:00:05Z","seq":2,"type":"message.done","role":"assistant","finish_reason":"stop","token_prompt":100,"token_completion":50}
```

### 3.4 结果回写机制

子对话完成后（或失败/错误），自动在父对话插入系统消息：

```go
func writeResultToParent(ctx context.Context, userID uuid.UUID, parentConvID uuid.UUID, childConvID uuid.UUID, success bool, summary string) {
    // 1. 分配 seq
    // 2. 创建 message (role=system, content=summary)
    // 3. 创建 user_update (message.new)
    // 4. push update
}
```

系统消息格式：
```
成功: "子对话 [ID] 已完成。摘要：{summary}"
失败: "子对话 [ID] 执行失败：{error_message}"
中断: "子对话 [ID] 需要您的确认：{permission_question}"（冒泡场景）
```

### 3.5 中断冒泡机制（方案：保留在子对话 + 聚合查询）

原设计是在父对话创建 HumanInPermission/Hitl 记录。但该方案会破坏 checkpoint/resume
机制——Eino 的 checkpoint 保存在子对话上，resume 必须在子对话上进行。如果将权限
记录创建在父对话，resume 时需要跨对话定位 checkpoint，逻辑复杂且容易出错。

**最终方案：权限记录保留在子对话，通过聚合 API 查询到父对话。**

```
子对话 Agent 触发中断
  → 权限中间件在子对话创建 HumanInPermission/Hitl 记录
     - conversation_id = 子对话 ID
     - source_conversation_id = 子对话 ID（标记来源）
  → Eino 保存 checkpoint 到子对话
  → 推送 permission.pending / human_in_the_loop.created WS 事件到子对话

前端在父对话页面
  → 调用 GET /conversations/:parentId/sub-interrupts 获取所有 pending 子中断
  → 在父对话页面渲染子中断卡片（复用现有组件）
  → 用户点击"同意" → POST /conversations/:childConvId/permissions/:permId/answer
  → 后端正常 resume 子对话（checkpoint 在同一对话，无需跨对话）
  → 子对话继续执行
```

**核心优势：** checkpoint 和 resume 逻辑完全不变，只需添加聚合查询接口。

**后端改动：**

1. **权限中间件** — 创建中断记录时赋值 `SourceConversationID`（设为当前对话 ID 即可，
   语义上标记"此中断来自这个对话"，前端据此区分父对话自身中断和子对话中断）

2. **新增聚合查询** — `GET /conversations/:id/sub-interrupts`
   - 查询所有 `source_conversation_id` 属于当前对话子代的 pending 中断
   - 返回 `HumanInPermission` 和 `HumanInTheLoop` 的联合列表
   - 每条记录附带 `conversation_id`（即子对话 ID，前端用于调用 resolve API）

3. **前端 resolve 不需要新端点** — 直接用子对话 ID 调用现有端点：
   - `POST /conversations/:childConvId/permissions/:permId/answer`
   - `POST /conversations/:childConvId/answer`

**前端改动：**

1. 页面加载时调用 `GET /conversations/:convId/sub-interrupts`
2. 子中断卡片渲染在父对话的输入框上方（与父对话自身的中断并列展示）
3. 子中断卡片标注来源子对话标题
4. resolve 时调用子对话的 resolve 端点

### 3.5.1 SourceConversationID 语义说明

`SourceConversationID` 在权限记录中的含义是"这个中断来自哪个对话"。
对于子对话触发中断的场景，`conversation_id` = `source_conversation_id` = 子对话 ID。
这个字段的主要价值在于：

- 前端通过 `conversation_id !== currentConvId` 识别这是子对话的中断
- 聚合查询通过 `source_conversation_id IN (子对话列表)` 批量获取
- 未来如果需要中断继续向上冒泡（多级子对话），此字段提供溯源能力

### 3.6 子对话与父对话的工具集

子对话继承父对话的全部工具集，不需要额外配置。

### 3.7 子对话生命周期

- 一个子对话 = 一次任务
- 执行完成后状态变为 `archived`
- 不再接受新消息
- 子对话 Agent 执行不暂停父对话

---

## 四、影响范围总结

### 回调重构影响文件

| 文件 | 修改 |
|------|------|
| `tools/sync.go` | 新增 SyncPushFunc 类型 |
| `tools/cron_task.go` | 移除全局回调，接收 syncFn |
| `tools/todo_write.go` | 移除全局回调，接收 syncFn |
| `tools/registry.go` | 新增 SetSyncPushFn，GetBaseTools 传入 syncFn |
| `di/module.go` | 移除全局变量赋值，改为 SetSyncPushFn |

### 功能一影响文件

| 文件 | 修改 |
|------|------|
| `openapi/spec.yaml` | 新增 /conversations/:id/branch 端点 |
| `server/internal/types/types.go` | 自动生成 |
| `web/src/types/api.d.ts` | 自动生成 |
| `service/conversation.go` | 新增 BranchConversation 方法 |
| `handler/conversation.go` | 新增 BranchConversation 端点 |

### 功能二影响文件

| 文件 | 修改 |
|------|------|
| `tools/sub_agent_tool.go` | 新增 subAgentTool |
| `service/chat.go` | 修改 runAgent 支持子对话上下文 |
| `eino/runner/rootrunner_callbacks.go` | 子对话日志写入 + 结果回写 |
| `model/human_in_permission.go` | 新增 SourceConversationID |
| `model/human_in_the_loop.go` | 新增 SourceConversationID |
| `eino/permission/middleware.go` | 创建中断时赋值 SourceConversationID |

### 功能三：中断冒泡（新）影响文件

| 文件 | 修改 |
|------|------|
| `openapi/spec.yaml` | 新增 GET /conversations/:id/sub-interrupts 端点 |
| `server/internal/types/types.go` | 自动生成 |
| `web/src/types/api.d.ts` | 自动生成 |
| `service/conversation.go` | 新增 GetSubInterrupts 方法 |
| `handler/conversation.go` | 新增 GET /conversations/:id/sub-interrupts 端点 |
| `eino/permission/middleware.go` | 创建中断时赋值 SourceConversationID |
| `eino/runner/rootrunner_callbacks.go` | 创建 HITL 记录时赋值 SourceConversationID |
| `web/src/app/.../chat/[convId]/page.tsx` | 加载子中断、渲染子中断卡片 |
