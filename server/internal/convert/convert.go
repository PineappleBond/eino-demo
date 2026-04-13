// Package convert provides helpers to translate between GORM models and
// OpenAPI-generated types (server/internal/types).
package convert

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/PineappleBond/eino-demo-dev/server/internal/model"
	"github.com/PineappleBond/eino-demo-dev/server/internal/templates"
	"github.com/PineappleBond/eino-demo-dev/server/internal/types"
)

func toUUID(id uuid.UUID) openapi_types.UUID {
	return id // openapi_types.UUID is a type alias for uuid.UUID
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtr(i int) *int {
	if i == 0 {
		return nil
	}
	return &i
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func toMapPtr(m model.JSONMap) *map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return &result
}

// ToUser converts a GORM User model to the OpenAPI User type.
func ToUser(m *model.User) *types.User {
	if m == nil {
		return nil
	}
	return &types.User{
		Id:        toUUID(m.ID),
		Name:      strPtr(m.Name),
		CreatedAt: timePtr(m.CreatedAt),
	}
}

// ToProject converts a GORM Project model to the OpenAPI Project type.
func ToProject(m model.Project) types.Project {
	cfg := make(map[string]interface{}, len(m.Config))
	for k, v := range m.Config {
		cfg[k] = v
	}
	return types.Project{
		Id:         toUUID(m.ID),
		UserId:     toUUID(m.UserID),
		TemplateId: m.TemplateID,
		Name:       m.Name,
		Config:     &cfg,
		CreatedAt:  timePtr(m.CreatedAt),
	}
}

// ToConversation converts a GORM Conversation model to the OpenAPI Conversation type.
func ToConversation(m model.Conversation) types.Conversation {
	return types.Conversation{
		Id:              toUUID(m.ID),
		ProjectId:       toUUID(m.ProjectID),
		UserId:          toUUID(m.UserID),
		Title:           strPtr(m.Title),
		Summary:         strPtr(m.Summary),
		Status:          strPtr(m.Status),
		Mode:            (*types.ConversationMode)(strPtr(m.Mode)),
		LastPreview:     strPtr(m.LastMessagePreview),
		MessageCount:    intPtr(m.MessageCount),
		LatestSeq:       intPtr(int(m.LatestMessageSeq)),
		MemberCount:     intPtr(m.MemberCount),
		TokenPrompt:     intPtr(int(m.TokenPrompt)),
		TokenCompletion: intPtr(int(m.TokenCompletion)),
		CreatedAt:       timePtr(m.CreatedAt),
		UpdatedAt:       timePtr(m.UpdatedAt),
	}
}

// treeEntry holds a conversation and its linked children during tree building.
type treeEntry struct {
	conv     model.Conversation
	children []model.Conversation
}

// BuildConversationTree converts a flat list of conversations into a tree structure
// based on ParentConversationID. Orphaned nodes (parent not in list) become roots.
// Circular references are detected and broken by promoting the node to root.
func BuildConversationTree(conversations []model.Conversation) []types.Conversation {
	// Index all conversations
	index := make(map[uuid.UUID]*treeEntry, len(conversations))
	for i := range conversations {
		conv := &conversations[i]
		index[conv.ID] = &treeEntry{conv: *conv}
	}

	// Link children to parents
	for _, entry := range index {
		if entry.conv.ParentConversationID != nil && *entry.conv.ParentConversationID != uuid.Nil {
			if parent, ok := index[*entry.conv.ParentConversationID]; ok {
				// Cycle detection: skip if targetID is already an ancestor
				if !hasAncestor(parent, entry.conv.ID, index) {
					parent.children = append(parent.children, entry.conv)
				}
			}
		}
	}

	// Collect roots
	var roots []types.Conversation
	for _, entry := range index {
		// Root if: no parent, parent is nil UUID, parent not in index, or cycle prevented
		if entry.conv.ParentConversationID == nil || *entry.conv.ParentConversationID == uuid.Nil {
			roots = append(roots, convertTreeEntry(*entry, index))
		} else if _, exists := index[*entry.conv.ParentConversationID]; !exists {
			roots = append(roots, convertTreeEntry(*entry, index))
		} else {
			// Check if we were linked as a child (if not, it was a cycle — become root)
			parent := index[*entry.conv.ParentConversationID]
			linked := false
			for _, c := range parent.children {
				if c.ID == entry.conv.ID {
					linked = true
					break
				}
			}
			if !linked {
				roots = append(roots, convertTreeEntry(*entry, index))
			}
		}
	}
	return roots
}

// hasAncestor checks if any ancestor of entryID eventually reaches targetID.
// Used for cycle detection during tree building.
func hasAncestor(entry *treeEntry, targetID uuid.UUID, index map[uuid.UUID]*treeEntry) bool {
	visited := make(map[uuid.UUID]bool)
	current := entry
	for current != nil {
		if current.conv.ID == targetID {
			return true
		}
		if visited[current.conv.ID] {
			return false // cycle in traversal, stop
		}
		visited[current.conv.ID] = true
		// Move up to parent
		if current.conv.ParentConversationID == nil {
			break
		}
		parentID := *current.conv.ParentConversationID
		current = index[parentID]
	}
	return false
}

// convertTreeEntry recursively converts a treeEntry and its children to types.Conversation.
func convertTreeEntry(entry treeEntry, index map[uuid.UUID]*treeEntry) types.Conversation {
	result := ToConversation(entry.conv)
	count := len(entry.children)
	result.ChildrenCount = &count
	if count > 0 {
		childTypes := make([]types.Conversation, 0, count)
		for _, child := range entry.children {
			if childEntry, ok := index[child.ID]; ok {
				childTypes = append(childTypes, convertTreeEntry(*childEntry, index))
			}
		}
		result.Children = &childTypes
	}
	return result
}

// ToMessage converts a GORM Message model to the OpenAPI Message type.
func ToMessage(m model.Message) types.Message {
	msg := types.Message{
		Id:              m.ID.String(),
		ConversationId:  m.ConversationID.String(),
		Seq:             int(m.Seq),
		SenderRole:      types.MessageSenderRole(m.SenderRole),
		SenderId:        strPtr(m.SenderID),
		Content:         m.Content,
		ReasonContent:   strPtr(m.ReasonContent),
		TokenPrompt:     intPtr(int(m.TokenPrompt)),
		TokenCompletion: intPtr(int(m.TokenCompletion)),
		CreatedAt:       timePtr(m.CreatedAt),
		Metadata:        toMapPtr(m.Metadata),
	}
	if m.ReplyToSeq != nil {
		v := int(*m.ReplyToSeq)
		msg.ReplyToSeq = &v
	}
	if len(m.MentionedMembers) > 0 {
		msg.MentionedMembers = &m.MentionedMembers
	}
	if m.FinishReason != nil {
		msg.FinishReason = m.FinishReason
	}
	if m.ErrorMessage != nil {
		msg.ErrorMessage = m.ErrorMessage
	}
	if m.DurationMs != nil {
		msg.DurationMs = m.DurationMs
	}
	if len(m.ToolCalling) > 0 {
		toolCalling := &struct {
			Input  *map[string]interface{} `json:"input,omitempty"`
			Output *string                 `json:"output,omitempty"`
		}{}
		if input, ok := m.ToolCalling["input"].(map[string]interface{}); ok {
			toolCalling.Input = toMapPtr(input)
		}
		if output, ok := m.ToolCalling["output"].(string); ok {
			toolCalling.Output = strPtr(output)
		}
		msg.ToolCalling = toolCalling
	}
	if !m.UpdatedAt.IsZero() {
		msg.UpdatedAt = &m.UpdatedAt
	}
	return msg
}

// ToSettings converts a GORM Settings model to the OpenAPI Settings type.
func ToSettings(m model.Settings) types.Settings {
	return types.Settings{
		ModelTier: (*types.SettingsModelTier)(strPtr(m.ModelTier)),
		Locale:    (*types.SettingsLocale)(strPtr(m.Locale)),
		Theme:     (*types.SettingsTheme)(strPtr(m.Theme)),
		UpdatedAt: timePtr(m.UpdatedAt),
	}
}

// ToTemplate converts a template TemplateInfo to the OpenAPI Template type.
func ToTemplate(t templates.TemplateInfo) types.Template {
	tpl := types.Template{
		Id:          t.ID,
		Name:        t.Name,
		Description: t.Description,
	}
	if len(t.Tags) > 0 {
		tpl.Tags = &t.Tags
	}
	if t.Difficulty != "" {
		d := types.TemplateDifficulty(t.Difficulty)
		tpl.Difficulty = &d
	}
	return tpl
}

// ToTodo converts a GORM Todo model to the OpenAPI Todo type.
func ToTodo(m model.Todo) types.Todo {
	return types.Todo{
		Id:             toUUID(m.ID),
		ConversationId: toUUID(m.ConversationID),
		Content:        m.Content,
		Completed:      m.Completed,
		Metadata:       toMapPtr(m.Metadata),
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

// ToCronTask converts a GORM CronTask model to the OpenAPI CronTask type.
func ToCronTask(m model.CronTask) types.CronTask {
	return types.CronTask{
		Id:             toUUID(m.ID),
		ConversationId: toUUID(m.ConversationID),
		Content:        m.Content,
		SenderRole:     types.CronTaskSenderRole(m.SenderRole),
		Schedule:       m.Schedule,
		NextRunAt:      m.NextRunAt,
		Status:         m.Status,
		CreatedAt:      m.CreatedAt,
	}
}

// ToMember converts a GORM ConversationMember model to the OpenAPI Member type.
func ToMember(m model.ConversationMember) types.Member {
	return types.Member{
		Id:             toUUID(m.ID),
		ConversationId: toUUID(m.ConversationID),
		MemberType:     m.MemberType,
		MemberId:       m.MemberID,
		MemberName:     strPtr(m.MemberName),
		IsOwner:        &m.IsOwner,
	}
}

// ToHumanInTheLoop converts a GORM HumanInTheLoop model to the OpenAPI HumanInTheLoop type.
func ToHumanInTheLoop(m model.HumanInTheLoop) types.HumanInTheLoop {
	var choices *[]struct {
		Desc  *string `json:"desc,omitempty"`
		Title string  `json:"title"`
	}
	if m.Choices != nil {
		if raw, ok := m.Choices["choices"].([]interface{}); ok {
			// Marshal to JSON then unmarshal to match the generated types exactly.
			b, _ := json.Marshal(raw)
			var parsed []struct {
				Desc  *string `json:"desc,omitempty"`
				Title string  `json:"title"`
			}
			_ = json.Unmarshal(b, &parsed)
			if len(parsed) > 0 {
				choices = &parsed
			}
		}
	}
	return types.HumanInTheLoop{
		Id:             toUUID(m.ID),
		ConversationId: toUUID(m.ConversationID),
		CheckpointId:   strPtr(m.CheckpointID),
		InterruptId:    strPtr(m.InterruptID),
		Question:       m.Question,
		Choices:        choices,
		AnswerType:     types.HumanInTheLoopAnswerType(m.AnswerType),
		Answer:         nil,
		Status:         types.HumanInTheLoopStatus(m.Status),
		CreatedAt:      m.CreatedAt,
	}
}

// ToHumanInPermission converts a GORM HumanInPermission model to the OpenAPI HumanInPermission type.
func ToHumanInPermission(m model.HumanInPermission) types.HumanInPermission {
	var decision *types.HumanInPermissionDecision
	if m.Decision != "" {
		d := types.HumanInPermissionDecision(m.Decision)
		decision = &d
	}
	return types.HumanInPermission{
		Id:             toUUID(m.ID),
		ConversationId: toUUID(m.ConversationID),
		CheckpointId:   strPtr(m.CheckpointID),
		InterruptId:    strPtr(m.InterruptID),
		ToolName:       m.ToolName,
		Action:         m.Action,
		Content:        m.Content,
		ToolDesc:       strPtr(m.ToolDesc),
		ArgsSummary:    strPtr(m.ArgsSummary),
		SafetyLevel:    m.SafetyLevel,
		SafetyReason:   strPtr(m.SafetyReason),
		Decision:       decision,
		Status:         types.HumanInPermissionStatus(m.Status),
		CreatedAt:      m.CreatedAt,
	}
}

// ToUpdatePayload converts a model.JSONMap to the generated Update_Payload union type.
// Used when reading persisted updates from the database.
func ToUpdatePayload(payload model.JSONMap) types.Update_Payload {
	var p types.Update_Payload
	if len(payload) == 0 {
		p.FromEmptyPayload(model.JSONMap{})
		return p
	}
	b, _ := json.Marshal(payload)
	_ = json.Unmarshal(b, &p) // safe: JSON was produced from valid Go types
	return p
}

// ToUpdate converts a GORM UserUpdate model to the OpenAPI Update type.
func ToUpdate(m model.UserUpdate) types.Update {
	return types.Update{
		Seq:     m.Seq,
		Type:    types.UpdateType(m.Type),
		Payload: ToUpdatePayload(m.Payload),
	}
}

// ── Composite response types that are inline in the OpenAPI spec ──

// MeResponse is the body returned by GET /users/me.
type MeResponse struct {
	ID        openapi_types.UUID `json:"id"`
	Name      *string            `json:"name,omitempty"`
	CreatedAt *time.Time         `json:"created_at,omitempty"`
	Settings  types.Settings     `json:"settings"`
}

// ModelsResponse is the body returned by GET /models.
type ModelInfo struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

type ModelsResponse struct {
	Models []ModelInfo `json:"models"`
}

// UpdatesResponse is the body returned by GET /users/me/updates.
type UpdatesResponse struct {
	Updates []types.Update `json:"updates"`
	MaxSeq  int64          `json:"max_seq"`
	HasMore bool           `json:"has_more"`
}

// MessageSendResponse is the body returned by POST /conversations/:id/messages.
type MessageSendResponse struct {
	ConversationID openapi_types.UUID `json:"conversation_id"`
	MessageID      openapi_types.UUID `json:"message_id"`
	Seq            int64              `json:"seq"`
}
