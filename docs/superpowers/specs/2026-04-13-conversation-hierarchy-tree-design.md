# 对话层级树形展示设计文档

## 日期
2026-04-13

## 概述

在侧边栏对话列表中实现层级树形展示。对话已有 `ParentConversationID` 字段支持无限层级嵌套，但前端侧边栏仍为平铺列表。本设计实现手风琴式树形列表，使用户能一目了然地看到一级对话，按需展开查看子对话。

## 已确认的设计决策

| 决策 | 选择 | 理由 |
| --- | --- | --- |
| 展示方案 | 手风琴（Accordion） | 一级列表整洁，按需展开，实现简单 |
| 展开深度 | 完全递归 | 深层嵌套虽少见但不限制 |
| 手风琴行为 | 只保持一个展开 | 列表不会变太长 |
| 状态持久化 | React 状态记忆 | 路由切换不丢失展开状态 |
| 后端返回格式 | 树形结构（`children` 字段） | 前端无需自己构建树 |
| 最大层级 | 不限制 | 理论无限，实际少见 |

## 架构

### 数据流

```text
GET /projects/{id}/conversations
  → 后端递归查询 conversations，组装成树形结构
  → 返回 [{ root_conv, children: [{ child, children: [...] }] }]
  → 前端 ChatSider 接收树形数据
  → 手风琴组件渲染：一级展开/折叠，子对话递归缩进
  → 用户点击展开箭头 → 展开唯一父对话的子节点 → 点击另一个 → 自动收起前一个
```

### 层级深度视觉策略

240px 侧边栏，每层缩进 16px。深层（4+ 层）时文字空间变窄，但：

- 99% 场景只有 0-1 层子对话
- 极少数深层场景仍可阅读（7 层时仍有约 128px 文字宽度）
- 不做限制，不做水平滚动

## 后端改动

### 1. OpenAPI Spec 更新

`Conversation` schema 新增两个字段：

```yaml
Conversation:
  properties:
    # ... 已有字段 ...
    parent_conversation_id:
      type: string
      format: uuid
      nullable: true
      description: "Parent conversation ID for hierarchical conversations"
    children_count:
      type: integer
      description: "Number of direct children (for badge display when children array is not included)"
    children:
      type: array
      items:
        $ref: "#/components/schemas/Conversation"
      description: "Nested child conversations (recursive)"
```

### 2. 后端 `ListConversations` 返回树形结构

当前 `ListConversations` 返回平铺列表。改为：

1. 查询所有对话（仍用一条 SQL）
2. 在内存中根据 `ParentConversationID` 构建树
3. 只返回根级对话（`ParentConversationID == nil`），每个附带 `children` 数组
4. `children` 递归填充所有层级

```go
// service/conversation.go
type ConversationTreeNode struct {
    model.Conversation
    Children []ConversationTreeNode `json:"children,omitempty"`
}

func (s *ConversationService) ListConversationsTree(
    userID, projectID uuid.UUID,
) ([]ConversationTreeNode, error) {
    var conversations []model.Conversation
    // ... 现有查询不变 ...

    // Build tree in memory
    index := make(map[uuid.UUID]ConversationTreeNode)
    var roots []ConversationTreeNode

    // First pass: create nodes
    for _, conv := range conversations {
        index[conv.ID] = ConversationTreeNode{Conversation: conv}
    }

    // Second pass: link children to parents
    for _, conv := range conversations {
        node := index[conv.ID]
        if conv.ParentConversationID != nil {
            if parent, ok := index[*conv.ParentConversationID]; ok {
                parent.Children = append(parent.Children, node)
                index[*conv.ParentConversationID] = parent
            }
        } else {
            roots = append(roots, node)
        }
    }
    return roots, nil
}
```

### 3. Handler 适配

`GET /projects/:id/conversations` handler 改为调用 `ListConversationsTree`，返回树形 JSON。

### 4. Convert 函数

`convert.ToConversation` 新增 `Children` 字段转换：

```go
func ToConversationTree(m model.Conversation, children []types.Conversation) types.Conversation {
    conv := ToConversation(m)
    conv.ParentConversationId = ...
    conv.ChildrenCount = len(children)
    conv.Children = children
    return conv
}
```

## 前端改动

### 1. ChatSider 改造

当前 `ChatSider` 使用 `useState<Conversation[]>` 平铺数组。改为：

```tsx
// 树形数据
interface ConversationNode extends Conversation {
    children?: ConversationNode[];
}

const [conversations, setConversations] = useState<ConversationNode[]>([]);
const [expandedConvIds, setExpandedConvIds] = useState<Set<string>>(new Set());
```

### 2. 手风琴展开逻辑

```tsx
const toggleExpand = useCallback((convId: string) => {
    setExpandedConvIds((prev) => {
        const next = new Set(prev);
        if (next.has(convId)) {
            next.delete(convId);
        } else {
            // Accordion: only one expanded at a time
            next.clear();
            next.add(convId);
        }
        return next;
    });
}, []);
```

### 3. 递归渲染

```tsx
function renderConversation(node: ConversationNode, depth: number) {
    const isExpanded = expandedConvIds.has(node.id);
    const hasChildren = node.children_count > 0;

    return (
        <div key={node.id}>
            <Link ... onClick={() => hasChildren && toggleExpand(node.id)}>
                {hasChildren && <span>{isExpanded ? '▼' : '▶'}</span>}
                <span style={{ paddingLeft: depth * 16 }}>{node.title}</span>
            </Link>
            {isExpanded && node.children?.map((child) =>
                renderConversation(child, depth + 1)
            )}
        </div>
    );
}
```

### 4. 子对话高亮

当前选中的对话（`selectedKey`）高亮。子对话被选中时，父对话自动展开并高亮。

### 5. URL 不变

导航 URL 仍为 `/${locale}/project/${projectId}/chat/${convId}`，子对话与普通对话共用相同 URL 模式。

### 6. WebSocket 事件处理

现有的 `conversation.created` / `conversation.deleted` 事件仍触发 `fetchRef.current()` 全量拉取。由于后端返回的是树形结构，前端无需额外处理层级关系，刷新即可。

## 需要改动的文件

### 后端

1. `openapi/spec.yaml` — Conversation schema 新增字段
2. `server/internal/types/types.go` — 重新生成
3. `server/internal/model/conversation.go` — 无需改动（已有 ParentConversationID）
4. `server/internal/service/conversation.go` — 新增 `ListConversationsTree` 方法
5. `server/internal/handler/conversation.go` — 修改 GET list handler
6. `server/internal/convert/convert.go` — 转换函数增加 children 字段

### 前端

1. `web/src/types/api.d.ts` — 重新生成
2. `web/src/lib/api.ts` — 无需改动（使用 components schemas）
3. `web/src/components/layout/ChatSider.tsx` — 主要改动：树形渲染 + 手风琴逻辑

## 风险与边界情况

1. **循环引用** — 理论上 `A.parent = B, B.parent = A` 会无限递归。后端构建树时需要检测循环（用 visited set）。
2. **孤儿节点** — 如果父对话被删除，`parent_conversation_id` 为已不存在的 ID。后端应将其视为根节点。
3. **大量子对话** — 一个父对话有 100+ 个子对话时，展开后会很长。这是合理的（手风琴本身就是为此设计的），但可考虑加个"查看更多"的虚拟滚动优化（后续）。
4. **性能** — 对话数量多时，树构建在内存中完成，O(n) 复杂度，不会有性能问题。
